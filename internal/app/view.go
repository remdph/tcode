package app

import (
	"github.com/charmbracelet/lipgloss"
)

var (
	focusedColor = lipgloss.Color("39")  // azul brillante
	dimColor     = lipgloss.Color("240") // gris
	titleColor   = lipgloss.Color("245")
)

// paneView envuelve el contenido de un panel en una caja con borde y un rótulo
// superior. El borde se resalta cuando el panel tiene el foco.
func paneView(content, title string, focused bool, innerW, innerH int) string {
	if innerW < 1 {
		innerW = 1
	}
	if innerH < 1 {
		innerH = 1
	}

	border := dimColor
	tcolor := titleColor
	if focused {
		border = focusedColor
		tcolor = focusedColor
	}

	titleBar := lipgloss.NewStyle().
		Foreground(tcolor).
		Bold(focused).
		Width(innerW).
		MaxWidth(innerW).
		Render(" " + title)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Width(innerW).
		Height(innerH).
		MaxHeight(innerH + 2).
		Render(content)

	return lipgloss.JoinVertical(lipgloss.Left, titleBar, box)
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
