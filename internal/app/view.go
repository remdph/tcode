package app

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	focusedColor = lipgloss.Color("39")  // azul brillante
	dimColor     = lipgloss.Color("240") // gris (costuras)
	titleColor   = lipgloss.Color("245")
)

// headerLabel dibuja la fila de rótulo de un panel (sin recuadro), ocupando
// todo el ancho. Cuando el panel tiene el foco se resalta en azul.
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

// horizSep dibuja el separador horizontal entre CLAUDE y TERMINAL, con el
// rótulo del panel inferior embebido (── TERMINAL ─────). Se resalta si el
// panel inferior tiene el foco.
func horizSep(label string, focused bool, w int) string {
	if w < 1 {
		w = 1
	}
	text := []rune("─ " + label + " ")
	if len(text) < w {
		text = append(text, []rune(strings.Repeat("─", w-len(text)))...)
	} else {
		text = text[:w]
	}
	st := lipgloss.NewStyle().Foreground(dimColor)
	if focused {
		st = st.Foreground(focusedColor).Bold(true)
	}
	return st.Render(string(text))
}

// seamColumn dibuja la costura vertical (│) de altura h entre el explorador y
// la columna derecha, con una ├ en la fila del separador horizontal.
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

// blockRect normaliza el contenido a un rectángulo exacto de w×h celdas,
// rellenando con espacios y recortando lo que sobre.
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

// statusBar dibuja la barra inferior con el foco actual y los atajos.
func (m *Model) statusBar() string {
	var focusName string
	switch m.focus {
	case focusSidebar:
		focusName = "EXPLORADOR"
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

	hints := " Ctrl+B explorador · Alt+1/2/3 foco · Ctrl+Q salir"
	if m.autoHidden {
		hints = " explorador oculto · Alt+2/3 foco · Ctrl+Q salir"
	}
	rest := lipgloss.NewStyle().
		Foreground(lipgloss.Color("250")).
		Background(lipgloss.Color("236")).
		Render(hints)

	bar := lipgloss.JoinHorizontal(lipgloss.Left, seg, rest)
	return lipgloss.NewStyle().
		Inline(true). // una sola línea: trunca en vez de envolver
		Width(m.width).
		MaxWidth(m.width).
		Background(lipgloss.Color("236")).
		Render(bar)
}
