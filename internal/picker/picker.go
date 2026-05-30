// Package picker implements the session selector shown in the CLAUDE panel
// before launching claude-cli.
package picker

import (
	"strings"

	"code-tui/internal/theme"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Item is an option in the selector.
type Item struct {
	ID       string // session ID to resume (empty when IsNew)
	Title    string // primary text
	Subtitle string // secondary text (e.g. the date)
	IsNew    bool   // true for the "New session" option
}

// Model is the selector state.
type Model struct {
	items         []Item
	cursor        int
	offset        int
	width, height int
}

var (
	headerStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	newStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("78")).Bold(true)
	titleStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	subtitleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	selTextStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Bold(true)
)

// accent-derived styles are built at render time so they pick up the theme
// loaded at startup.
func selBarStyle() lipgloss.Style { return lipgloss.NewStyle().Foreground(theme.Accent) }
func selSubStyle() lipgloss.Style { return lipgloss.NewStyle().Foreground(theme.Accent) }

// New creates the selector with the given options (the first one should be the
// "New session" option).
func New(items []Item) Model {
	return Model{items: items, width: 40, height: 10}
}

// SetSize sets the available area.
func (m *Model) SetSize(w, h int) {
	m.width, m.height = w, h
}

// Update handles navigation. It returns the chosen item when the user confirms
// with Enter (nil otherwise).
func (m Model) Update(k tea.KeyMsg) (Model, *Item) {
	switch k.String() {
	case "up", "k", "ctrl+p":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j", "ctrl+n":
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}
	case "home", "g":
		m.cursor = 0
	case "end", "G":
		m.cursor = len(m.items) - 1
	case "enter":
		if m.cursor >= 0 && m.cursor < len(m.items) {
			it := m.items[m.cursor]
			return m, &it
		}
	}
	m.ensureVisible()
	return m, nil
}

func (m *Model) ensureVisible() {
	rows := m.rowsPerItem()
	visible := m.height / rows
	if visible < 1 {
		visible = 1
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+visible {
		m.offset = m.cursor - visible + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

// rowsPerItem: each item takes 2 rows (title + subtitle).
func (m Model) rowsPerItem() int { return 2 }

// View renders the selector.
func (m Model) View() string {
	var b strings.Builder
	header := "Claude sessions in this directory — ↑/↓ and Enter:"
	b.WriteString(truncate(headerStyle.Render(header), m.width))
	b.WriteString("\n\n")

	rows := m.rowsPerItem()
	visible := (m.height - 2) / rows
	if visible < 1 {
		visible = 1
	}
	end := m.offset + visible
	if end > len(m.items) {
		end = len(m.items)
	}

	for i := m.offset; i < end; i++ {
		it := m.items[i]
		selected := i == m.cursor

		bar := "  "
		if selected {
			bar = selBarStyle().Render("▎ ")
		}

		// Primary line (the plain text is truncated before styling).
		mainText := it.Title
		if it.IsNew {
			mainText = "＋ New session"
		}
		mainText = truncate(mainText, m.width-2)
		var main string
		switch {
		case selected:
			main = selTextStyle.Render(mainText)
		case it.IsNew:
			main = newStyle.Render(mainText)
		default:
			main = titleStyle.Render(mainText)
		}
		b.WriteString(bar + main + "\n")

		// Secondary line (date / id).
		if !it.IsNew {
			subText := truncate(it.Subtitle, m.width-2)
			if selected {
				b.WriteString("  " + selSubStyle().Render(subText) + "\n")
			} else {
				b.WriteString("  " + subtitleStyle.Render(subText) + "\n")
			}
		} else {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// truncate trims plain (unstyled) text to a maximum visible width.
func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && lipgloss.Width(string(r))+1 > w {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}
