// Package app holds the root model of the TUI: it composes the side explorer,
// the claude-cli panel and the terminal panel, and manages layout and focus.
package app

import (
	"os"
	"os/exec"
	"path/filepath"

	"code-tui/internal/config"
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

// Model is the root Bubble Tea model.
type Model struct {
	dir           string
	width, height int

	sidebar sidebar.Model
	claude  *terminal.Model

	// terms holds the terminal tabs; activeTerm is the visible/focused one.
	terms      []*terminal.Model
	activeTerm int
	nextTermID int
	shell      string

	focus        focus
	sidebarPref  bool // what the user wants (Ctrl+B)
	showSidebar  bool // effective visibility (pref + available width)
	autoHidden   bool // hidden automatically because the window is too narrow
	sidebarWidth int  // explorer width in columns (persisted per project)
	started      bool
	prog         *tea.Program

	// picker is the session selector that occupies the CLAUDE panel until the
	// user chooses; nil when there are no past sessions or one was already chosen.
	picker *picker.Model

	// editor is the floating editor overlay (nano/vi) while a file is open;
	// nil otherwise.
	editor     *terminal.Model
	editorID   int
	editorName string
	editorHint string

	// Geometry computed by layout().
	sbInnerW                 int
	rightInnerW              int
	sbInnerH                 int
	claudeInnerH, termInnerH int
}

// New builds the root model pointing at dir. The terminal processes are not
// launched until the first window size is received.
func New(dir string) *Model {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}

	// Per-project explorer width (persisted), falling back to the default.
	sidebarWidth := config.Load(dir).SidebarWidth
	if sidebarWidth == 0 {
		sidebarWidth = defaultSidebarWidth
	}
	sidebarWidth = clamp(sidebarWidth, minSidebarWidth, maxSidebarWidth)

	m := &Model{
		dir:          dir,
		shell:        shell,
		sidebarPref:  false, // the explorer starts hidden (toggle with Ctrl+B)
		sidebarWidth: sidebarWidth,
		focus:        focusClaude,
		sidebar:      sidebar.New(dir),
		claude:       terminal.New(1, "CLAUDE", dir, claudeArgs(nil)),
		terms:        []*terminal.Model{terminal.New(2, "TERMINAL", dir, []string{shell})},
		nextTermID:   3,
	}

	// If there are past sessions in this directory, a selector is shown in the
	// CLAUDE panel; the first option is always to create a new session.
	if past := sessions.List(dir); len(past) > 0 {
		items := []picker.Item{{Title: "New session", IsNew: true}}
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

// claudeArgs builds the claude command. It always launches with
// --dangerously-skip-permissions; if resumeID is non-empty, that session is resumed.
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

// SetProgram stores the program reference so the terminals can notify new output.
func (m *Model) SetProgram(p *tea.Program) { m.prog = p }

func (m *Model) Init() tea.Cmd { return nil }

// Update implements tea.Model.
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

	case sidebar.OpenFileMsg:
		m.openEditor(msg.Path)
		return m, nil

	case terminal.ExitMsg:
		// Closing the editor (e.g. quitting nano) dismisses the overlay.
		if m.editor != nil && msg.ID == m.editorID {
			m.editor.Close()
			m.editor = nil
		}
		return m, nil

	case terminal.RefreshMsg:
		// A repaint is enough; Bubble Tea calls View after every Update.
		return m, nil
	}
	return m, nil
}

func (m *Model) handleKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	// While the floating editor is open, every key goes to it; it closes on its
	// own exit (Ctrl+X in nano, :q in vi).
	if m.editor != nil {
		m.editor.SendKey(k)
		return m, nil
	}

	// Global shortcuts (intercepted before forwarding to the focused panel).
	switch {
	case k.Type == tea.KeyCtrlQ:
		m.cleanup()
		return m, tea.Quit
	case k.String() == "ctrl+b" || k.String() == "super+b":
		m.toggleSidebar()
		return m, nil
	case k.Alt && runeIs(k, '1'):
		m.focus = focusClaude
		return m, nil
	case k.Alt && runeIs(k, '2'):
		m.focus = focusTerminal
		return m, nil
	case k.Alt && runeIs(k, '3'):
		if m.showSidebar {
			m.focus = focusSidebar
		}
		return m, nil
	case k.Alt && (runeIs(k, '+') || runeIs(k, '=')):
		m.newTerminal() // '=' shares the key with '+'
		return m, nil
	case k.Alt && runeIs(k, '-'):
		m.closeTerminal()
		return m, nil
	}

	// Forward to the focused panel.
	switch m.focus {
	case focusSidebar:
		// +/- resize the explorer (persisted per project).
		switch k.String() {
		case "+", "=":
			m.resizeSidebar(+2)
			return m, nil
		case "-":
			m.resizeSidebar(-2)
			return m, nil
		}
		var cmd tea.Cmd
		m.sidebar, cmd = m.sidebar.Update(k)
		return m, cmd
	case focusClaude:
		// While the session picker is active, keys go to it.
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
		// Switch tabs with Alt+Left/Right; everything else goes to the terminal.
		switch {
		case k.Alt && k.Type == tea.KeyLeft:
			m.switchTerm(-1)
		case k.Alt && k.Type == tea.KeyRight:
			m.switchTerm(1)
		default:
			m.activeTermModel().SendKey(k)
		}
	}
	return m, nil
}

func runeIs(k tea.KeyMsg, r rune) bool {
	return len(k.Runes) == 1 && k.Runes[0] == r
}

func (m *Model) toggleSidebar() {
	m.sidebarPref = !m.sidebarPref
	m.layout()
	// Showing the explorer focuses it; hiding it returns focus to CLAUDE.
	if m.showSidebar {
		m.focus = focusSidebar
	} else if m.focus == focusSidebar {
		m.focus = focusClaude
	}
}

func (m *Model) startTerminals() {
	for _, t := range m.terms {
		_ = t.Start(m.prog)
	}
	// The shell always starts; claude only if there is no pending selector.
	if m.picker == nil {
		m.startClaude(nil)
	}
}

// activeTermModel returns the terminal of the active tab.
func (m *Model) activeTermModel() *terminal.Model { return m.terms[m.activeTerm] }

// newTerminal opens a new terminal tab, makes it active and focuses it.
func (m *Model) newTerminal() {
	t := terminal.New(m.nextTermID, "TERMINAL", m.dir, []string{m.shell})
	m.nextTermID++
	t.SetSize(max(m.rightInnerW, 1), max(m.termInnerH, 1))
	if m.started {
		_ = t.Start(m.prog)
	}
	m.terms = append(m.terms, t)
	m.activeTerm = len(m.terms) - 1
	m.focus = focusTerminal
}

// closeTerminal closes the active tab, keeping at least one terminal open.
func (m *Model) closeTerminal() {
	if len(m.terms) <= 1 {
		return
	}
	m.terms[m.activeTerm].Close()
	m.terms = append(m.terms[:m.activeTerm], m.terms[m.activeTerm+1:]...)
	if m.activeTerm >= len(m.terms) {
		m.activeTerm = len(m.terms) - 1
	}
}

// switchTerm moves the active tab by delta (wrapping around).
func (m *Model) switchTerm(delta int) {
	n := len(m.terms)
	m.activeTerm = (m.activeTerm + delta + n) % n
}

// startClaude launches claude-cli (new session, or resuming resumeID).
func (m *Model) startClaude(resumeID *string) {
	if m.claude.Started() {
		return
	}
	m.claude.SetArgs(claudeArgs(resumeID))
	_ = m.claude.Start(m.prog)
}

func (m *Model) cleanup() {
	m.claude.Close()
	for _, t := range m.terms {
		t.Close()
	}
	if m.editor != nil {
		m.editor.Close()
	}
}

// openEditor opens path in a floating editor (nano, or vi as a fallback).
func (m *Model) openEditor(path string) {
	args, hint := editorCommand(path)
	if args == nil {
		return // neither nano nor vi available
	}
	m.editorID = m.nextTermID
	m.nextTermID++
	m.editor = terminal.New(m.editorID, filepath.Base(path), m.dir, args)
	m.editor.SetFocused(true) // the editor always shows its cursor
	m.editorName = filepath.Base(path)
	m.editorHint = hint
	m.layout() // size the editor before starting it
	if m.started {
		_ = m.editor.Start(m.prog)
	}
}

// editorCommand returns the command (and exit hint) for editing path, preferring
// nano and falling back to vi.
func editorCommand(path string) (args []string, hint string) {
	if p, err := exec.LookPath("nano"); err == nil {
		return []string{p, path}, "Ctrl+X to exit"
	}
	if p, err := exec.LookPath("vi"); err == nil {
		return []string{p, path}, ":q to exit"
	}
	return nil, ""
}

// editorDims returns the inner content size (width, height) of the floating
// editor box, which uses almost the whole screen.
func (m *Model) editorDims() (w, h int) {
	boxW, boxH := m.width-4, m.height-2
	if boxW < 10 {
		boxW = m.width
	}
	if boxH < 6 {
		boxH = m.height
	}
	// Box = border (2) on each axis + a title row inside.
	return max(boxW-2, 1), max(boxH-3, 1)
}

// minWidthForSidebar is the minimum window width (in columns) below which the
// explorer is hidden automatically to give the panels more room.
const minWidthForSidebar = 80

// Explorer width bounds and default (in columns).
const (
	defaultSidebarWidth = 24
	minSidebarWidth     = 12
	maxSidebarWidth     = 50
)

// resizeSidebar changes the explorer width by delta and persists it per project.
func (m *Model) resizeSidebar(delta int) {
	m.sidebarWidth = clamp(m.sidebarWidth+delta, minSidebarWidth, maxSidebarWidth)
	m.layout()
	_ = config.Save(m.dir, config.Project{SidebarWidth: m.sidebarWidth})
}

// layout computes the geometry of the panels and propagates it.
func (m *Model) layout() {
	if m.width <= 0 || m.height <= 0 {
		return
	}
	const statusH = 1     // bottom status bar
	const headerH = 1     // top label row (EXPLORER / CLAUDE)
	const termHeaderH = 1 // TERMINAL label row, between CLAUDE and TERMINAL
	const dividerH = 4    // dividers: above and below CLAUDE, above and below TERMINAL

	// Effective visibility: what the user wants, but only if the window is wide
	// enough. Otherwise it is hidden automatically (responsive).
	m.showSidebar = m.sidebarPref && m.width >= minWidthForSidebar
	m.autoHidden = m.sidebarPref && !m.showSidebar
	if !m.showSidebar && m.focus == focusSidebar {
		m.focus = focusClaude
	}

	bodyH := m.height - statusH

	// No outer borders: only 1 column is reserved for the vertical seam (│)
	// when the explorer is visible.
	sbW, seamW := 0, 0
	if m.showSidebar {
		// Use the persisted width, but never let it take more than half the window.
		sbW = clamp(m.sidebarWidth, 8, m.width/2)
		seamW = 1
	}
	rightW := m.width - sbW - seamW

	m.sbInnerW = sbW
	m.rightInnerW = rightW
	m.sbInnerH = bodyH - headerH

	rightContentH := bodyH - headerH - termHeaderH - dividerH
	if rightContentH < 2 {
		rightContentH = 2
	}
	// CLAUDE takes the larger share; the terminal starts a bit smaller.
	m.claudeInnerH = max(rightContentH*7/10, 1)
	m.termInnerH = max(rightContentH-m.claudeInnerH, 1)

	m.sidebar.SetSize(max(sbW, 1), max(m.sbInnerH, 1))
	m.claude.SetSize(max(rightW, 1), max(m.claudeInnerH, 1))
	for _, t := range m.terms {
		t.SetSize(max(rightW, 1), max(m.termInnerH, 1))
	}
	if m.editor != nil {
		ew, eh := m.editorDims()
		m.editor.SetSize(ew, eh)
	}
	if m.picker != nil {
		m.picker.SetSize(max(rightW, 1), max(m.claudeInnerH, 1))
	}
}

// View implements tea.Model.
func (m *Model) View() string {
	if m.width == 0 {
		return "Starting code-tui…"
	}

	// The floating editor takes over the whole screen when open.
	if m.editor != nil {
		return m.editorView()
	}

	bodyH := m.height - 1

	// Right column: CLAUDE on top, TERMINAL below. The TERMINAL label uses the
	// same header style as the others. If the session picker is active, it
	// takes over the CLAUDE area.
	m.claude.SetFocused(m.focus == focusClaude)
	claudeView := m.claude.View()
	if m.picker != nil {
		claudeView = m.picker.View()
	}
	at := m.activeTermModel()
	at.SetFocused(m.focus == focusTerminal)
	divider := hLine(m.rightInnerW)
	claudeHeader := headerLabel(m.claude.Name(), m.focus == focusClaude, m.rightInnerW)
	claudeContent := blockRect(claudeView, m.rightInnerW, m.claudeInnerH)
	termHeader := terminalHeader(len(m.terms), m.activeTerm, m.focus == focusTerminal, m.rightInnerW)
	termContent := blockRect(at.View(), m.rightInnerW, m.termInnerH)
	right := lipgloss.JoinVertical(lipgloss.Left,
		divider, // above CLAUDE title
		claudeHeader,
		divider, // below CLAUDE title
		claudeContent,
		divider, // above TERMINAL title
		termHeader,
		divider, // below TERMINAL title
		termContent,
	)

	body := right
	if m.showSidebar {
		sbHeader := headerLabel("EXPLORER", m.focus == focusSidebar, m.sbInnerW)
		sbContent := blockRect(m.sidebar.View(), m.sbInnerW, m.sbInnerH)
		left := lipgloss.JoinVertical(lipgloss.Left, sbHeader, sbContent)
		// ├ junctions on each divider row of the right column.
		seam := seamColumn(bodyH, 0, 2, 3+m.claudeInnerH, 5+m.claudeInnerH)
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
