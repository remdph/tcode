// Package app contiene el modelo raíz de la TUI: compone el explorador lateral,
// el panel de claude-cli y el panel de terminal, y gestiona layout y foco.
package app

import (
	"os"

	"code-tui/internal/sidebar"
	"code-tui/internal/terminal"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type focus int

const (
	focusSidebar focus = iota
	focusClaude
	focusTerminal
)

// Model es el modelo raíz de Bubble Tea.
type Model struct {
	dir           string
	width, height int

	sidebar sidebar.Model
	claude  *terminal.Model
	term    *terminal.Model

	focus       focus
	sidebarPref bool // lo que el usuario quiere (Ctrl+B)
	showSidebar bool // visibilidad efectiva (pref + ancho disponible)
	autoHidden  bool // oculto automáticamente por ancho insuficiente
	started     bool
	prog        *tea.Program

	// Geometría calculada por layout().
	sbInnerW                 int
	rightInnerW              int
	sbInnerH                 int
	claudeInnerH, termInnerH int
}

// New construye el modelo raíz apuntando a dir. Los procesos de los terminales
// no se lanzan hasta recibir el primer tamaño de ventana.
func New(dir string) *Model {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}
	return &Model{
		dir:         dir,
		sidebarPref: true,
		showSidebar: true,
		focus:       focusClaude,
		sidebar:     sidebar.New(dir),
		claude:      terminal.New(1, "CLAUDE", dir, []string{"claude"}),
		term:        terminal.New(2, "TERMINAL", dir, []string{shell}),
	}
}

// SetProgram guarda la referencia al programa para que los terminales puedan
// notificar nueva salida.
func (m *Model) SetProgram(p *tea.Program) { m.prog = p }

func (m *Model) Init() tea.Cmd { return nil }

// Update implementa tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		if !m.started {
			m.startTerminals()
			m.started = true
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case terminal.RefreshMsg, terminal.ExitMsg:
		// Basta con repintar; Bubble Tea llama a View tras cada Update.
		return m, nil
	}
	return m, nil
}

func (m *Model) handleKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Atajos globales (interceptados antes de reenviar al panel con foco).
	switch {
	case k.Type == tea.KeyCtrlQ:
		m.cleanup()
		return m, tea.Quit
	case k.String() == "ctrl+b" || k.String() == "super+b":
		m.toggleSidebar()
		return m, nil
	case k.Alt && runeIs(k, '1'):
		if m.showSidebar {
			m.focus = focusSidebar
		}
		return m, nil
	case k.Alt && runeIs(k, '2'):
		m.focus = focusClaude
		return m, nil
	case k.Alt && runeIs(k, '3'):
		m.focus = focusTerminal
		return m, nil
	}

	// Reenvío al panel con foco.
	switch m.focus {
	case focusSidebar:
		var cmd tea.Cmd
		m.sidebar, cmd = m.sidebar.Update(k)
		return m, cmd
	case focusClaude:
		m.claude.SendKey(k)
	case focusTerminal:
		m.term.SendKey(k)
	}
	return m, nil
}

func runeIs(k tea.KeyMsg, r rune) bool {
	return len(k.Runes) == 1 && k.Runes[0] == r
}

func (m *Model) toggleSidebar() {
	m.sidebarPref = !m.sidebarPref
	m.layout()
}

func (m *Model) startTerminals() {
	_ = m.claude.Start(m.prog)
	_ = m.term.Start(m.prog)
}

func (m *Model) cleanup() {
	m.claude.Close()
	m.term.Close()
}

// minWidthForSidebar es el ancho mínimo de ventana (en columnas) por debajo del
// cual el explorador se oculta automáticamente para dar espacio a los paneles.
const minWidthForSidebar = 80

// layout calcula la geometría de los paneles y la propaga.
func (m *Model) layout() {
	if m.width <= 0 || m.height <= 0 {
		return
	}
	const statusH = 1
	const titleH = 1   // rótulo encima de cada panel
	const borderH = 2  // borde superior + inferior
	const borderW = 2  // borde izquierdo + derecho

	// Visibilidad efectiva: lo que el usuario quiere, pero solo si la ventana es
	// suficientemente ancha. Si no, se oculta automáticamente (responsive).
	m.showSidebar = m.sidebarPref && m.width >= minWidthForSidebar
	m.autoHidden = m.sidebarPref && !m.showSidebar
	if !m.showSidebar && m.focus == focusSidebar {
		m.focus = focusClaude
	}

	bodyH := m.height - statusH

	sbOuterW := 0
	if m.showSidebar {
		sbOuterW = clamp(m.width/4, 22, 40)
	}
	rightOuterW := m.width - sbOuterW

	m.sbInnerW = sbOuterW - borderW
	m.rightInnerW = rightOuterW - borderW
	m.sbInnerH = bodyH - titleH - borderH

	claudeOuterH := bodyH * 3 / 5
	termOuterH := bodyH - claudeOuterH
	m.claudeInnerH = claudeOuterH - titleH - borderH
	m.termInnerH = termOuterH - titleH - borderH

	m.sidebar.SetSize(max(m.sbInnerW, 1), max(m.sbInnerH, 1))
	m.claude.SetSize(max(m.rightInnerW, 1), max(m.claudeInnerH, 1))
	m.term.SetSize(max(m.rightInnerW, 1), max(m.termInnerH, 1))
}

// View implementa tea.Model.
func (m *Model) View() string {
	if m.width == 0 {
		return "Iniciando code-tui…"
	}

	claudeBox := paneView(m.claude.View(), m.claude.Name(), m.focus == focusClaude, m.rightInnerW, m.claudeInnerH)
	termBox := paneView(m.term.View(), m.term.Name(), m.focus == focusTerminal, m.rightInnerW, m.termInnerH)
	right := lipgloss.JoinVertical(lipgloss.Left, claudeBox, termBox)

	body := right
	if m.showSidebar {
		sb := paneView(m.sidebar.View(), "EXPLORADOR", m.focus == focusSidebar, m.sbInnerW, m.sbInnerH)
		body = lipgloss.JoinHorizontal(lipgloss.Top, sb, right)
	}

	return lipgloss.JoinVertical(lipgloss.Left, body, m.statusBar())
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
