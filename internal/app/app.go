// Package app contiene el modelo raíz de la TUI: compone el explorador lateral,
// el panel de claude-cli y el panel de terminal, y gestiona layout y foco.
package app

import (
	"os"

	"code-tui/internal/picker"
	"code-tui/internal/sessions"
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

	// picker es el selector de sesión que ocupa el panel CLAUDE hasta que el
	// usuario elige; nil si no hay sesiones pasadas o ya se eligió.
	picker *picker.Model

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

	m := &Model{
		dir:         dir,
		sidebarPref: true,
		showSidebar: true,
		focus:       focusClaude,
		sidebar:     sidebar.New(dir),
		claude:      terminal.New(1, "CLAUDE", dir, claudeArgs(nil)),
		term:        terminal.New(2, "TERMINAL", dir, []string{shell}),
	}

	// Si hay sesiones pasadas en este directorio, se muestra un selector en el
	// panel CLAUDE; la primera opción siempre es crear una sesión nueva.
	if past := sessions.List(dir); len(past) > 0 {
		items := []picker.Item{{Title: "Nueva sesión", IsNew: true}}
		for _, s := range past {
			items = append(items, picker.Item{
				ID:       s.ID,
				Title:    s.Title,
				Subtitle: s.ModTime.Format("2006-01-02 15:04") + "  ·  " + shortID(s.ID),
			})
		}
		p := picker.New(items)
		m.picker = &p
	}
	return m
}

// claudeArgs construye el comando de claude. Siempre se lanza con
// --dangerously-skip-permissions; si resumeID no es vacío, se reanuda esa sesión.
func claudeArgs(resumeID *string) []string {
	args := []string{"claude", "--dangerously-skip-permissions"}
	if resumeID != nil && *resumeID != "" {
		args = append(args, "--resume", *resumeID)
	}
	return args
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
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
		// Mientras el selector de sesión esté activo, las teclas van a él.
		if m.picker != nil {
			updated, chosen := m.picker.Update(k)
			*m.picker = updated
			if chosen != nil {
				m.picker = nil
				if chosen.IsNew {
					m.startClaude(nil)
				} else {
					m.startClaude(&chosen.ID)
				}
			}
			return m, nil
		}
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
	_ = m.term.Start(m.prog)
	// La shell siempre arranca; claude solo si no hay selector pendiente.
	if m.picker == nil {
		m.startClaude(nil)
	}
}

// startClaude lanza claude-cli (nueva sesión o reanudando resumeID).
func (m *Model) startClaude(resumeID *string) {
	if m.claude.Started() {
		return
	}
	m.claude.SetArgs(claudeArgs(resumeID))
	_ = m.claude.Start(m.prog)
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
	const statusH = 1 // barra de estado inferior
	const headerH = 1 // fila de rótulos superior (EXPLORADOR / CLAUDE)
	const sepH = 1    // separador horizontal entre CLAUDE y TERMINAL

	// Visibilidad efectiva: lo que el usuario quiere, pero solo si la ventana es
	// suficientemente ancha. Si no, se oculta automáticamente (responsive).
	m.showSidebar = m.sidebarPref && m.width >= minWidthForSidebar
	m.autoHidden = m.sidebarPref && !m.showSidebar
	if !m.showSidebar && m.focus == focusSidebar {
		m.focus = focusClaude
	}

	bodyH := m.height - statusH

	// Sin bordes exteriores: solo se reserva 1 columna para la costura vertical
	// (│) cuando el explorador está visible.
	sbW, seamW := 0, 0
	if m.showSidebar {
		sbW = clamp(m.width/4, 20, 40)
		seamW = 1
	}
	rightW := m.width - sbW - seamW

	m.sbInnerW = sbW
	m.rightInnerW = rightW
	m.sbInnerH = bodyH - headerH

	rightContentH := bodyH - headerH - sepH
	m.claudeInnerH = rightContentH * 3 / 5
	m.termInnerH = rightContentH - m.claudeInnerH

	m.sidebar.SetSize(max(sbW, 1), max(m.sbInnerH, 1))
	m.claude.SetSize(max(rightW, 1), max(m.claudeInnerH, 1))
	m.term.SetSize(max(rightW, 1), max(m.termInnerH, 1))
	if m.picker != nil {
		m.picker.SetSize(max(rightW, 1), max(m.claudeInnerH, 1))
	}
}

// View implementa tea.Model.
func (m *Model) View() string {
	if m.width == 0 {
		return "Iniciando code-tui…"
	}

	bodyH := m.height - 1

	// Columna derecha: CLAUDE arriba, separador con rótulo, TERMINAL abajo.
	// Si el selector de sesión está activo, ocupa el área de CLAUDE.
	claudeView := m.claude.View()
	if m.picker != nil {
		claudeView = m.picker.View()
	}
	rightHeader := headerLabel(m.claude.Name(), m.focus == focusClaude, m.rightInnerW)
	claudeContent := blockRect(claudeView, m.rightInnerW, m.claudeInnerH)
	sep := horizSep(m.term.Name(), m.focus == focusTerminal, m.rightInnerW)
	termContent := blockRect(m.term.View(), m.rightInnerW, m.termInnerH)
	right := lipgloss.JoinVertical(lipgloss.Left, rightHeader, claudeContent, sep, termContent)

	body := right
	if m.showSidebar {
		sbHeader := headerLabel("EXPLORADOR", m.focus == focusSidebar, m.sbInnerW)
		sbContent := blockRect(m.sidebar.View(), m.sbInnerW, m.sbInnerH)
		left := lipgloss.JoinVertical(lipgloss.Left, sbHeader, sbContent)
		seam := seamColumn(bodyH, 1+m.claudeInnerH) // ├ en la fila del separador
		body = lipgloss.JoinHorizontal(lipgloss.Top, left, seam, right)
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
