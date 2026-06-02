// Package app holds the root model of the TUI: it composes the side explorer,
// the claude-cli panel and the terminal panel, and manages layout and focus.
package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"code-tui/internal/config"
	"code-tui/internal/gitpanel"
	"code-tui/internal/picker"
	"code-tui/internal/quickopen"
	"code-tui/internal/sessions"
	"code-tui/internal/sidebar"
	"code-tui/internal/terminal"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
)

type focus int

const (
	focusSidebar focus = iota
	focusClaude
	focusTerminal
	focusGit
)

// claudeTermID is the fixed terminal id of the CLAUDE panel (terminals use 2+).
const claudeTermID = 1

// Model is the root Bubble Tea model.
type Model struct {
	dir           string
	width, height int

	sidebar sidebar.Model
	claude  *terminal.Model

	// terms holds the terminal tabs; activeTerm is the visible/focused one.
	terms         []*terminal.Model
	activeTerm    int
	nextTermID    int
	shell         string
	showTerminals bool // the TERMINAL section is visible (Ctrl+T)

	focus        focus
	sidebarPref  bool // what the user wants (Ctrl+B)
	showSidebar  bool // effective visibility (pref + available width)
	autoHidden   bool // hidden automatically because the window is too narrow
	sidebarWidth int  // explorer width in columns (persisted per project)

	// Git panel (right side), toggled with Ctrl+G.
	gitPanel gitpanel.Model
	showGit  bool
	gitWidth int // git panel width in columns (persisted per project)
	started  bool
	prog     *tea.Program

	// picker is the session selector that occupies the CLAUDE panel until the
	// user chooses; nil when there are no past sessions or one was already chosen.
	picker *picker.Model

	// editor is the floating editor overlay (nano/vi) while a file is open;
	// nil otherwise.
	editor     *terminal.Model
	editorID   int
	editorName string
	editorHint string

	// quickOpen is the floating fuzzy file finder (Ctrl+P); nil when closed.
	quickOpen *quickopen.Model

	// Geometry computed by layout().
	sbInnerW                 int
	rightInnerW              int
	sbInnerH                 int
	claudeInnerH, termInnerH int
	gitInnerW, gitInnerH     int
}

// New builds the root model pointing at dir. The terminal processes are not
// launched until the first window size is received.
func New(dir string) *Model {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}

	// Per-project panel widths (persisted), falling back to the defaults.
	cfg := config.Load(dir)
	sidebarWidth := cfg.SidebarWidth
	if sidebarWidth == 0 {
		sidebarWidth = defaultSidebarWidth
	}
	gitWidth := cfg.GitWidth
	if gitWidth == 0 {
		gitWidth = defaultGitWidth
	}

	m := &Model{
		dir:           dir,
		shell:         shell,
		sidebarPref:   false, // the explorer starts hidden (toggle with Ctrl+B)
		showTerminals: true,
		sidebarWidth:  clamp(sidebarWidth, minSidebarWidth, maxSidebarWidth),
		gitWidth:      clamp(gitWidth, minGitWidth, maxGitWidth),
		focus:         focusClaude,
		sidebar:       sidebar.New(dir),
		gitPanel:      gitpanel.New(dir),
		claude:        terminal.New(claudeTermID, "CLAUDE", dir, claudeArgs(nil)),
		terms:         []*terminal.Model{terminal.New(2, "TERMINAL", dir, []string{shell})},
		nextTermID:    3,
	}

	// If there are past sessions in this directory, a selector is shown in the
	// CLAUDE panel; the first option is always to create a new session.
	if past := sessions.List(dir); len(past) > 0 {
		m.buildSessionPicker(past)
	}
	return m
}

// buildSessionPicker (re)builds the CLAUDE session selector from past.
func (m *Model) buildSessionPicker(past []sessions.Session) {
	items := []picker.Item{{Title: "New session", IsNew: true}}
	for _, s := range past {
		items = append(items, picker.Item{
			ID:       s.ID,
			Title:    s.Title,
			Subtitle: s.ModTime.Format("2006-01-02 15:04") + "  ·  " + shortID(s.ID),
		})
	}
	p := picker.New("Claude sessions in this directory — ↑/↓ and Enter:", items)
	m.picker = &p
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

	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case tea.PasteMsg:
		// Bracketed-paste content is routed to whatever currently takes input.
		switch {
		case m.editor != nil:
			m.editor.SendPaste(msg.Content)
		case m.quickOpen != nil:
			// the finder ignores pastes; nothing to do
		case m.focus == focusClaude && m.picker == nil:
			m.claude.ScrollToBottom()
			m.claude.SendPaste(msg.Content)
		case m.focus == focusTerminal:
			m.activeTermModel().ScrollToBottom()
			m.activeTermModel().SendPaste(msg.Content)
		}
		return m, nil

	case sidebar.OpenFileMsg:
		m.openEditor(msg.Path)
		return m, nil

	case terminal.ExitMsg:
		switch {
		case m.editor != nil && msg.ID == m.editorID:
			// Closing the editor (e.g. quitting nano) dismisses the overlay.
			m.editor.Close()
			m.editor = nil
		case msg.ID == claudeTermID:
			// claude-cli exited (e.g. Ctrl+C to quit): return to the session menu.
			m.reopenSessionMenu()
		}
		return m, nil

	case terminal.RefreshMsg:
		// A repaint is enough; Bubble Tea calls View after every Update.
		return m, nil
	}
	return m, nil
}

func (m *Model) handleKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	logKey(k) // no-op unless TCODE_DEBUG_KEYS is set

	// While the floating editor is open, every key goes to it; it closes on its
	// own exit (Ctrl+X in nano, :q in vi).
	if m.editor != nil {
		m.editor.SendKey(k)
		return m, nil
	}

	// While the quick-open finder is open, every key drives it. Confirming opens
	// the chosen file in the floating editor (just like the explorer); Esc closes.
	if m.quickOpen != nil {
		updated, open, cancelled := m.quickOpen.Update(k)
		*m.quickOpen = updated
		switch {
		case cancelled:
			m.quickOpen = nil
		case open != "":
			m.quickOpen = nil
			m.openEditor(open)
		}
		return m, nil
	}

	// Global shortcuts (intercepted before forwarding to the focused panel).
	// Ctrl/Super combos are matched by their keystroke string (their Text is
	// empty so String() is reliable); Alt combos are matched on Code+Mod because
	// some terminals populate Text for Alt+printable, which would shadow the
	// keystroke form.
	// altOnly is Alt without Shift/Ctrl, so the Alt+<digit> focus shortcuts don't
	// collide with the Alt+Shift+<digit> terminal-tab shortcuts.
	alt := altHeld(k.Mod)
	altOnly := alt && !k.Mod.Contains(tea.ModShift) && !k.Mod.Contains(tea.ModCtrl)
	switch {
	case k.String() == "ctrl+q":
		m.cleanup()
		return m, tea.Quit
	case k.String() == "ctrl+p", k.String() == "super+p":
		m.openQuickOpen()
		return m, nil
	case k.String() == "ctrl+b", k.String() == "super+b":
		m.toggleSidebar()
		return m, nil
	case k.String() == "ctrl+g":
		m.toggleGit()
		return m, nil
	case k.String() == "ctrl+t":
		m.toggleTerminals()
		return m, nil
	case altOnly && k.Code == '1':
		m.focus = focusClaude
		return m, nil
	case altOnly && k.Code == '2':
		if m.showTerminals {
			m.focus = focusTerminal
		}
		return m, nil
	case altOnly && k.Code == '3':
		if m.showSidebar {
			m.focus = focusSidebar
		}
		return m, nil
	case alt && (k.Code == '+' || k.Code == '='):
		m.newTerminal() // '+' is Shift+'='; accept either
		return m, nil
	case alt && k.Code == '-':
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
		// Insert a newline in claude's prompt (instead of submitting) on
		// Shift+Enter, Ctrl+J or Alt/Option+Enter. Plain Enter still submits.
		// With the kitty keyboard protocol (negotiated by Bubble Tea v2),
		// Shift+Enter is now distinguishable on any supporting terminal.
		switch k.String() {
		case "shift+enter", "ctrl+j", "alt+enter":
			m.claude.ScrollToBottom()
			m.claude.SendNewline()
			return m, nil
		}
		// PgUp/PgDn scroll the CLAUDE history (its scrollback buffer), unless an
		// alt-screen program is running, in which case it does its own paging.
		// Any other key snaps back to the live view before reaching the process.
		if !m.claude.AltScreen() {
			switch k.Code {
			case tea.KeyPgUp:
				m.claude.ScrollPage(+1)
				return m, nil
			case tea.KeyPgDown:
				m.claude.ScrollPage(-1)
				return m, nil
			}
		}
		m.claude.ScrollToBottom()
		m.claude.SendKey(k)
	case focusTerminal:
		// Alt+Shift+Left/Right cycle tabs, Alt+Shift+<n> jumps to tab n, PgUp/PgDn
		// scroll the history; every other key goes to the terminal (after
		// snapping back to the live view). Shift is required because many
		// terminals reserve a bare Alt+Left/Right for word navigation.
		shift := k.Mod.Contains(tea.ModShift)
		switch {
		case altHeld(k.Mod) && shift && k.Code == tea.KeyLeft:
			m.switchTerm(-1)
		case altHeld(k.Mod) && shift && k.Code == tea.KeyRight:
			m.switchTerm(1)
		case k.Code == tea.KeyPgUp && !m.activeTermModel().AltScreen():
			m.activeTermModel().ScrollPage(+1)
		case k.Code == tea.KeyPgDown && !m.activeTermModel().AltScreen():
			m.activeTermModel().ScrollPage(-1)
		default:
			if idx, ok := altTabIndex(k); ok {
				if idx < len(m.terms) {
					m.activeTerm = idx
				}
				return m, nil // swallow the combo even if that tab is absent
			}
			m.activeTermModel().ScrollToBottom()
			m.activeTermModel().SendKey(k)
		}
	case focusGit:
		// +/- resize the Git panel (persisted per project).
		switch k.String() {
		case "+", "=":
			m.resizeGit(+2)
			return m, nil
		case "-":
			m.resizeGit(-2)
			return m, nil
		}
		var cmd tea.Cmd
		m.gitPanel, cmd = m.gitPanel.Update(k)
		return m, cmd
	}
	return m, nil
}

// keyDebug enables logging of every received key press to a file, for
// diagnosing terminal-specific key encodings. Set TCODE_DEBUG_KEYS=<path> (or
// any value, which logs to /tmp/tcode-keys.log).
var keyDebug = os.Getenv("TCODE_DEBUG_KEYS")

func logKey(k tea.KeyPressMsg) {
	if keyDebug == "" {
		return
	}
	path := keyDebug
	if path == "1" || path == "true" {
		path = "/tmp/tcode-keys.log"
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "string=%-16q code=%d (%q) mod=%03b text=%q\n",
		k.String(), k.Code, string(k.Code), k.Mod, k.Text)
}

// altHeld reports whether the Alt modifier is active. Some terminals report the
// physical Alt key as Meta, so Meta counts too; lock keys (Num/Caps/Scroll Lock)
// set extra modifier bits under the kitty keyboard protocol and are ignored.
func altHeld(m tea.KeyMod) bool {
	m &^= tea.ModCapsLock | tea.ModNumLock | tea.ModScrollLock
	return m.Contains(tea.ModAlt) || m.Contains(tea.ModMeta)
}

// altTabIndex maps an Alt+Shift+<digit> press to a 0-based terminal tab index.
// With the kitty keyboard protocol the digit is reported directly (Code is the
// unshifted '1'..'0' with ModAlt|ModShift); on terminals without it the press
// arrives as Alt + the US-layout shifted symbol (!@#$…). ok is false otherwise.
func altTabIndex(k tea.KeyPressMsg) (int, bool) {
	if !altHeld(k.Mod) {
		return 0, false
	}
	if k.Mod.Contains(tea.ModShift) {
		switch k.Code {
		case '1', '2', '3', '4', '5', '6', '7', '8', '9':
			return int(k.Code - '1'), true
		case '0':
			return 9, true
		}
	}
	switch k.Code { // legacy fallback: Alt + shifted symbol
	case '!':
		return 0, true
	case '@':
		return 1, true
	case '#':
		return 2, true
	case '$':
		return 3, true
	case '%':
		return 4, true
	case '^':
		return 5, true
	case '&':
		return 6, true
	case '*':
		return 7, true
	case '(':
		return 8, true
	case ')':
		return 9, true
	}
	return 0, false
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
	m.showTerminals = true // creating a terminal reveals the section
	t := terminal.New(m.nextTermID, "TERMINAL", m.dir, []string{m.shell})
	m.nextTermID++
	m.terms = append(m.terms, t)
	m.activeTerm = len(m.terms) - 1
	m.focus = focusTerminal
	m.layout() // size the new terminal (and the rest) before starting it
	if m.started {
		_ = t.Start(m.prog)
	}
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

// reopenSessionMenu is called when claude-cli exits: it replaces the dead CLAUDE
// terminal with a fresh one and shows the session selector again.
func (m *Model) reopenSessionMenu() {
	m.claude.Close()
	m.claude = terminal.New(claudeTermID, "CLAUDE", m.dir, claudeArgs(nil))
	m.buildSessionPicker(sessions.List(m.dir))
	m.focus = focusClaude
	m.layout()
}

// toggleTerminals shows/hides the TERMINAL section (Ctrl+T). Revealing it moves
// focus to it; hiding it returns focus to CLAUDE. The shells keep running while
// hidden.
func (m *Model) toggleTerminals() {
	m.showTerminals = !m.showTerminals
	if m.showTerminals {
		m.focus = focusTerminal
	} else if m.focus == focusTerminal {
		m.focus = focusClaude
	}
	m.layout()
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

// openQuickOpen opens the floating fuzzy file finder over the current project.
func (m *Model) openQuickOpen() {
	qo := quickopen.New(m.dir)
	m.quickOpen = &qo
	m.layout() // size it before the first render
}

// quickOpenDims returns the inner content size (width, height) of the floating
// quick-open box, which covers most (but not all) of the screen.
func (m *Model) quickOpenDims() (w, h int) {
	boxW := clamp(m.width*3/4, 40, max(m.width-4, 10))
	boxH := clamp(m.height*2/3, 8, max(m.height-2, 6))
	// Box = border (2) on each axis + a title row inside.
	return max(boxW-2, 1), max(boxH-3, 1)
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

// Panel width bounds and defaults (in columns).
const (
	defaultSidebarWidth = 24
	minSidebarWidth     = 12
	maxSidebarWidth     = 50

	defaultGitWidth = 36
	minGitWidth     = 18
	maxGitWidth     = 70
)

// saveConfig persists the per-project panel widths.
func (m *Model) saveConfig() {
	_ = config.Save(m.dir, config.Project{
		SidebarWidth: m.sidebarWidth,
		GitWidth:     m.gitWidth,
	})
}

// resizeSidebar changes the explorer width by delta and persists it.
func (m *Model) resizeSidebar(delta int) {
	m.sidebarWidth = clamp(m.sidebarWidth+delta, minSidebarWidth, maxSidebarWidth)
	m.layout()
	m.saveConfig()
}

// resizeGit changes the Git panel width by delta and persists it.
func (m *Model) resizeGit(delta int) {
	m.gitWidth = clamp(m.gitWidth+delta, minGitWidth, maxGitWidth)
	m.layout()
	m.saveConfig()
}

// toggleGit shows/hides the Git panel. Showing it focuses it and refreshes its
// data; hiding it returns focus to CLAUDE.
func (m *Model) toggleGit() {
	m.showGit = !m.showGit
	if m.showGit {
		m.gitPanel.Refresh()
		m.focus = focusGit
	} else if m.focus == focusGit {
		m.focus = focusClaude
	}
	m.layout()
}

// layout computes the geometry of the panels and propagates it.
func (m *Model) layout() {
	if m.width <= 0 || m.height <= 0 {
		return
	}
	const statusH = 1 // bottom status bar
	const headerH = 1 // top label row (EXPLORER / CLAUDE)

	// Effective visibility: what the user wants, but only if the window is wide
	// enough. Otherwise it is hidden automatically (responsive).
	m.showSidebar = m.sidebarPref && m.width >= minWidthForSidebar
	m.autoHidden = m.sidebarPref && !m.showSidebar
	if !m.showSidebar && m.focus == focusSidebar {
		m.focus = focusClaude
	}

	bodyH := m.height - statusH

	// No outer borders: only 1 column is reserved for each vertical seam (│)
	// next to the explorer (left) and the Git panel (right).
	sbW, leftSeam := 0, 0
	if m.showSidebar {
		sbW = clamp(m.sidebarWidth, 8, m.width/3)
		leftSeam = 1
	}
	gitW, rightSeam := 0, 0
	if m.showGit {
		gitW = clamp(m.gitWidth, 12, m.width/3)
		rightSeam = 1
	}
	rightW := m.width - sbW - leftSeam - gitW - rightSeam

	m.sbInnerW = sbW
	m.rightInnerW = rightW
	m.sbInnerH = bodyH - headerH
	m.gitInnerW = gitW
	m.gitInnerH = bodyH - headerH // header row = tab bar

	if m.showTerminals {
		// CLAUDE header + TERMINAL header + 4 dividers (above/below each).
		rightContentH := bodyH - 2*headerH - 4
		if rightContentH < 2 {
			rightContentH = 2
		}
		// CLAUDE takes the larger share; the terminal starts a bit smaller.
		m.claudeInnerH = max(rightContentH*7/10, 1)
		m.termInnerH = max(rightContentH-m.claudeInnerH, 1)
	} else {
		// Only CLAUDE: its header plus a divider above and below it.
		m.claudeInnerH = max(bodyH-headerH-2, 1)
		m.termInnerH = 0
	}

	m.sidebar.SetSize(max(sbW, 1), max(m.sbInnerH, 1))
	m.claude.SetSize(max(rightW, 1), max(m.claudeInnerH, 1))
	if m.showTerminals {
		for _, t := range m.terms {
			t.SetSize(max(rightW, 1), max(m.termInnerH, 1))
		}
	}
	if m.editor != nil {
		ew, eh := m.editorDims()
		m.editor.SetSize(ew, eh)
	}
	if m.quickOpen != nil {
		qw, qh := m.quickOpenDims()
		m.quickOpen.SetSize(qw, qh)
	}
	if m.picker != nil {
		m.picker.SetSize(max(rightW, 1), max(m.claudeInnerH, 1))
	}
	if m.showGit {
		m.gitPanel.SetSize(max(gitW, 1), max(m.gitInnerH, 1))
	}
}

// View implements tea.Model. It requests the alternate screen and renders the
// current UI; AltScreen on the View is what puts the program in full-window mode
// in Bubble Tea v2 (there is no WithAltScreen program option).
func (m *Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	return v
}

// render builds the full-screen UI as a styled string.
func (m *Model) render() string {
	if m.width == 0 {
		return "Starting code-tui…"
	}

	// The floating editor takes over the whole screen when open.
	if m.editor != nil {
		return m.editorView()
	}

	// The quick-open finder floats over the screen until dismissed.
	if m.quickOpen != nil {
		return m.quickOpenView()
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
	divider := hLine(m.rightInnerW)
	claudeHeader := headerLabel(m.claude.Name(), m.focus == focusClaude, m.rightInnerW)
	claudeContent := blockRect(claudeView, m.rightInnerW, m.claudeInnerH)

	var right string
	var jr []int // divider rows of the middle column, where the seams branch
	if m.showTerminals {
		at := m.activeTermModel()
		at.SetFocused(m.focus == focusTerminal)
		termHeader := terminalHeader(len(m.terms), m.activeTerm, m.focus == focusTerminal, m.rightInnerW)
		termContent := blockRect(at.View(), m.rightInnerW, m.termInnerH)
		right = lipgloss.JoinVertical(lipgloss.Left,
			divider, // above CLAUDE title
			claudeHeader,
			divider, // below CLAUDE title
			claudeContent,
			divider, // above TERMINAL title
			termHeader,
			divider, // below TERMINAL title
			termContent,
		)
		jr = []int{0, 2, 3 + m.claudeInnerH, 5 + m.claudeInnerH}
	} else {
		right = lipgloss.JoinVertical(lipgloss.Left,
			divider, // above CLAUDE title
			claudeHeader,
			divider, // below CLAUDE title
			claudeContent,
		)
		jr = []int{0, 2}
	}

	// Compose the columns left-to-right: [explorer] [seam] middle [seam] [git].
	cols := []string{}
	if m.showSidebar {
		sbHeader := headerLabel("EXPLORER", m.focus == focusSidebar, m.sbInnerW)
		sbContent := blockRect(m.sidebar.View(), m.sbInnerW, m.sbInnerH)
		left := lipgloss.JoinVertical(lipgloss.Left, sbHeader, sbContent)
		cols = append(cols, left, seamColumn(bodyH, "├", jr...))
	}
	cols = append(cols, right)
	if m.showGit {
		gitHeader := m.gitPanel.TabBar(m.focus == focusGit, m.gitInnerW)
		gitContent := blockRect(m.gitPanel.View(), m.gitInnerW, m.gitInnerH)
		gitBlock := lipgloss.JoinVertical(lipgloss.Left, gitHeader, gitContent)
		cols = append(cols, seamColumn(bodyH, "┤", jr...), gitBlock)
	}

	body := lipgloss.JoinHorizontal(lipgloss.Top, cols...)
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
