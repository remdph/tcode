package app

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	focusedColor = lipgloss.Color("39")  // bright blue
	dimColor     = lipgloss.Color("240") // gray (seams)
	titleColor   = lipgloss.Color("245")
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
		st = st.Foreground(lipgloss.Color("231")).Background(focusedColor)
	} else {
		st = st.Foreground(titleColor)
	}
	return st.Render(" " + label)
}

// seamColumn draws the vertical seam (│) of height h between the explorer and
// the right column, with a ├ on the TERMINAL header row.
func seamColumn(h, sepRow int) string {
	lines := make([]string, h)
	for i := range lines {
		if i == sepRow {
			lines[i] = "├"
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
	}

	seg := lipgloss.NewStyle().
		Foreground(lipgloss.Color("231")).
		Background(focusedColor).
		Bold(true).
		Padding(0, 1).
		Render(focusName)

	hints := " Ctrl+B explorer · Alt+1 Claude · Alt+2 terminal · Ctrl+Q quit"
	if m.autoHidden {
		hints = " explorer hidden (window too narrow) · Alt+1/2 focus · Ctrl+Q quit"
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
