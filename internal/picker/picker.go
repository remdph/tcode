// Package picker implementa el selector de sesión que se muestra en el panel
// CLAUDE antes de lanzar claude-cli.
package picker

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Item es una opción del selector.
type Item struct {
	ID       string // ID de sesión a reanudar (vacío si IsNew)
	Title    string // texto principal
	Subtitle string // texto secundario (p. ej. la fecha)
	IsNew    bool   // true en la opción "Nueva sesión"
}

// Model es el estado del selector.
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
	selBarStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	selTextStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Bold(true)
	selSubStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("153"))
)

// New crea el selector con las opciones dadas (la primera debería ser la de
// "Nueva sesión").
func New(items []Item) Model {
	return Model{items: items, width: 40, height: 10}
}

// SetSize fija el área disponible.
func (m *Model) SetSize(w, h int) {
	m.width, m.height = w, h
}

// Update procesa la navegación. Devuelve el ítem elegido cuando el usuario
// confirma con Enter (nil en caso contrario).
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

// rowsPerItem: cada ítem ocupa 2 filas (título + subtítulo) salvo el "nuevo".
func (m Model) rowsPerItem() int { return 2 }

// View renderiza el selector.
func (m Model) View() string {
	var b strings.Builder
	header := "Sesiones de Claude en este directorio — ↑/↓ y Enter:"
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
			bar = selBarStyle.Render("▎ ")
		}

		// Línea principal (se recorta el texto plano antes de aplicar estilo).
		mainText := it.Title
		if it.IsNew {
			mainText = "＋ Nueva sesión"
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

		// Línea secundaria (fecha / id).
		if !it.IsNew {
			subText := truncate(it.Subtitle, m.width-2)
			if selected {
				b.WriteString("  " + selSubStyle.Render(subText) + "\n")
			} else {
				b.WriteString("  " + subtitleStyle.Render(subText) + "\n")
			}
		} else {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// truncate recorta texto plano (sin estilos) a un ancho visible máximo.
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
