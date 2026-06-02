package app

import (
	"fmt"
	"strings"

	"code-tui/internal/terminal"
	"code-tui/internal/theme"

	"github.com/charmbracelet/lipgloss"
)

var (
	dimColor   = lipgloss.Color("240") // gray (seams)
	titleColor = lipgloss.Color("245")
)

// headerLabel draws a panel's label row (no box), spanning the full width.
// When the panel is focused it is highlighted in blue.
func headerLabel(label string, focused bool, w int) string {
	if w < 1 {
		w = 1
	}
	st := lipgloss.NewStyle().
		Inline(true).
		Bold(true).
		Width(w).
		MaxWidth(w)
	if focused {
		st = st.Foreground(theme.OnAccent).Background(theme.Accent)
	} else {
		st = st.Foreground(titleColor)
	}
	return st.Render(" " + label)
}

// editorView renders the floating editor box centered over the screen.
func (m *Model) editorView() string {
	cw, ch := m.editorDims()

	title := lipgloss.NewStyle().
		Foreground(theme.OnAccent).
		Background(theme.Accent).
		Bold(true).
		Inline(true).
		Width(cw).
		MaxWidth(cw).
		Render(" " + m.editorName + "  —  " + m.editorHint)

	content := blockRect(m.editor.View(), cw, ch)
	inner := lipgloss.JoinVertical(lipgloss.Left, title, content)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Accent).
		Render(inner)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

// quickOpenView renders the floating fuzzy file finder centered over the screen.
func (m *Model) quickOpenView() string {
	cw, ch := m.quickOpenDims()

	title := lipgloss.NewStyle().
		Foreground(theme.OnAccent).
		Background(theme.Accent).
		Bold(true).
		Inline(true).
		Width(cw).
		MaxWidth(cw).
		Render(" Open file  —  type to search · ↑/↓ select · Enter open · Esc cancel")

	content := blockRect(m.quickOpen.View(), cw, ch)
	inner := lipgloss.JoinVertical(lipgloss.Left, title, content)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Accent).
		Render(inner)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

// terminalHeader draws the TERMINAL label followed by one tab chip per open
// terminal. The active tab is highlighted with the theme accent; the label is
// accent-colored when the panel is focused.
func terminalHeader(count, active int, focused bool, w int) string {
	if w < 1 {
		w = 1
	}
	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(titleColor)
	if focused {
		labelStyle = labelStyle.Foreground(theme.Accent)
	}
	parts := []string{labelStyle.Render(" TERMINAL ")}
	for i := 0; i < count; i++ {
		chip := fmt.Sprintf(" %d ", i+1)
		if i == active {
			parts = append(parts, lipgloss.NewStyle().
				Foreground(theme.OnAccent).Background(theme.Accent).Bold(true).Render(chip))
		} else {
			parts = append(parts, lipgloss.NewStyle().Foreground(dimColor).Render(chip))
		}
	}
	line := lipgloss.JoinHorizontal(lipgloss.Top, parts...)
	return lipgloss.NewStyle().Inline(true).Width(w).MaxWidth(w).Render(line)
}

// hLine draws a horizontal divider line of width w.
func hLine(w int) string {
	if w < 1 {
		w = 1
	}
	return lipgloss.NewStyle().Foreground(dimColor).Render(strings.Repeat("─", w))
}

// seamColumn draws a vertical seam (│) of height h, with the given junction
// glyph (├ for a seam to the left of dividers, ┤ to the right) on every row
// that has a horizontal divider.
func seamColumn(h int, junction string, junctions ...int) string {
	isJunction := make(map[int]bool, len(junctions))
	for _, j := range junctions {
		isJunction[j] = true
	}
	lines := make([]string, h)
	for i := range lines {
		if isJunction[i] {
			lines[i] = junction
		} else {
			lines[i] = "│"
		}
	}
	return lipgloss.NewStyle().Foreground(dimColor).Render(strings.Join(lines, "\n"))
}

// blockRect normalizes content into an exact w×h rectangle, padding with
// spaces and trimming any overflow.
func blockRect(content string, w, h int) string {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	return lipgloss.NewStyle().
		Width(w).
		Height(h).
		MaxWidth(w).
		MaxHeight(h).
		Render(content)
}

// focusedScroll returns the terminal panel that PgUp/PgDn currently scrolls
// (CLAUDE or the active terminal tab), or nil if neither is focused.
func (m *Model) focusedScroll() *terminal.Model {
	switch m.focus {
	case focusClaude:
		if m.picker == nil {
			return m.claude
		}
	case focusTerminal:
		return m.activeTermModel()
	}
	return nil
}

// statusBar draws the bottom bar with the current focus and the shortcuts.
func (m *Model) statusBar() string {
	var focusName string
	switch m.focus {
	case focusSidebar:
		focusName = "EXPLORER"
	case focusClaude:
		focusName = "CLAUDE"
	case focusTerminal:
		focusName = "TERMINAL"
	case focusGit:
		focusName = "GIT"
	}

	seg := lipgloss.NewStyle().
		Foreground(theme.OnAccent).
		Background(theme.Accent).
		Bold(true).
		Padding(0, 1).
		Render(focusName)

	// While a terminal panel is scrolled into its history, the hint shows how to
	// get back to the live view.
	if scrolled := m.focusedScroll(); scrolled != nil && scrolled.Scrolled() {
		hints := fmt.Sprintf(" SCROLLBACK ↑%d lines · PgUp/PgDn scroll · any key → live",
			scrolled.ScrollOffset())
		rest := lipgloss.NewStyle().
			Foreground(theme.OnAccent).
			Background(theme.Accent).
			Render(hints)
		bar := lipgloss.JoinHorizontal(lipgloss.Left, seg, rest)
		return lipgloss.NewStyle().
			Inline(true).
			Width(m.width).
			MaxWidth(m.width).
			Background(theme.Accent).
			Render(bar)
	}

	var hints string
	switch {
	case m.focus == focusSidebar:
		hints = " ↑/↓ move · Enter open · . dotfiles · +/- width · Ctrl+B hide · Ctrl+Q quit"
	case m.focus == focusTerminal:
		hints = " Alt++ new · Alt+- close · Alt+←/→ or Alt+Shift+n switch · PgUp/PgDn scroll · Alt+1 Claude"
	case m.focus == focusGit:
		hints = " ↑/↓ move · Tab CHANGES/HISTORY · r refresh · +/- width · Ctrl+G hide"
	case m.autoHidden:
		hints = " explorer hidden (window too narrow) · Alt+1/2 focus · Ctrl+Q quit"
	default:
		hints = " Ctrl+P open · Ctrl+B explorer · Ctrl+G git · Ctrl+T terminals · Ctrl+J newline · Alt+1/2 focus"
	}
	rest := lipgloss.NewStyle().
		Foreground(lipgloss.Color("250")).
		Background(lipgloss.Color("236")).
		Render(hints)

	bar := lipgloss.JoinHorizontal(lipgloss.Left, seg, rest)
	return lipgloss.NewStyle().
		Inline(true). // single line: truncate instead of wrapping
		Width(m.width).
		MaxWidth(m.width).
		Background(lipgloss.Color("236")).
		Render(bar)
}
