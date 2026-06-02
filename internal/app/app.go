// Package app holds the root model of the TUI: it composes the side explorer,
// the claude-cli panel and the terminal panel, and manages layout and focus.
package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"code-tui/internal/agents"
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

// pickMode is what selector (if any) a tab is currently showing in place of its
// running process.
type pickMode int

const (
	pickNone    pickMode = iota // the agent is running
	pickAgent                   // choosing which agent to run
	pickSession                 // choosing a Claude session to start/resume
)

// claudeTab is one tab of the agent section: an agent CLI running in a terminal
// plus, while picking, the agent or session selector shown in its place.
type claudeTab struct {
	agent  agents.Agent
	term   *terminal.Model
	picker *picker.Model // the active selector; nil only when mode == pickNone
	mode   pickMode
}

// Model is the root Bubble Tea model.
type Model struct {
	dir           string
	width, height int

	sidebar sidebar.Model

	// agentList is the set of agents detected on PATH; defaultAgent is the one
	// new tabs launch (empty Bin means "ask on first use").
	agentList    []agents.Agent
	defaultAgent agents.Agent

	// claudeTabs holds the agent tabs; activeClaude is the visible/focused one.
	claudeTabs   []*claudeTab
	activeClaude int

	// terms holds the terminal tabs; activeTerm is the visible/focused one.
	terms         []*terminal.Model
	activeTerm    int
	shell         string
	showTerminals bool // the TERMINAL section is visible (Ctrl+T)

	// nextID hands out unique terminal ids (claude tabs, terminal tabs, editor).
	nextID int

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
		nextID:        1,
	}

	// Detect installed agents and resolve the default. With a saved (still
	// installed) default, use it. With exactly one agent, adopt it silently.
	// Otherwise leave it unset so the first tab opens on the agent selector.
	m.agentList = agents.Detect()
	if len(m.agentList) == 0 {
		m.agentList = []agents.Agent{agents.Claude} // fallback: try plain claude
	}
	if a, ok := m.findAgent(config.LoadGlobal().DefaultAgent); ok {
		m.defaultAgent = a
	} else if len(m.agentList) == 1 {
		m.setDefaultAgent(m.agentList[0])
	}

	// One agent tab and one TERMINAL tab to start.
	m.claudeTabs = []*claudeTab{m.makeClaudeTab()}
	m.terms = []*terminal.Model{terminal.New(m.allocID(), "TERMINAL", dir, []string{shell})}
	return m
}

// findAgent returns the installed agent with the given Bin.
func (m *Model) findAgent(bin string) (agents.Agent, bool) {
	for _, a := range m.agentList {
		if a.Bin == bin {
			return a, true
		}
	}
	return agents.Agent{}, false
}

// setDefaultAgent records agent as the default for new tabs and persists it.
func (m *Model) setDefaultAgent(a agents.Agent) {
	m.defaultAgent = a
	_ = config.SaveGlobal(config.Global{DefaultAgent: a.Bin})
}

// allocID returns a fresh unique terminal id.
func (m *Model) allocID() int {
	id := m.nextID
	m.nextID++
	return id
}

// makeClaudeTab builds an agent tab. With no default agent yet (first run with
// several installed) it opens on the agent selector; otherwise it is configured
// for the default agent (Claude opens on its session selector).
func (m *Model) makeClaudeTab() *claudeTab {
	tab := &claudeTab{}
	if m.defaultAgent.Bin == "" {
		m.toAgentPicker(tab)
		return tab
	}
	m.configureTab(tab, m.defaultAgent)
	return tab
}

// toAgentPicker puts tab into agent-selection mode (with a placeholder terminal).
func (m *Model) toAgentPicker(tab *claudeTab) {
	tab.agent = agents.Agent{}
	tab.term = terminal.New(m.allocID(), "AGENT", m.dir, nil)
	tab.picker = newAgentPicker(m.agentList)
	tab.mode = pickAgent
}

// configureTab points tab at agent and chooses its starting state: Claude with
// past sessions opens on the session selector; everything else launches directly.
func (m *Model) configureTab(tab *claudeTab, agent agents.Agent) {
	tab.agent = agent
	tab.term = terminal.New(m.allocID(), strings.ToUpper(agent.Name), m.dir, agent.Args)
	tab.term.SetEnv(agent.Env)
	if agent.Bin == "claude" {
		if past := sessions.List(m.dir); len(past) > 0 {
			tab.picker = newSessionPicker(past)
			tab.mode = pickSession
			return
		}
	}
	tab.picker = nil
	tab.mode = pickNone
}

// activeClaudeTab returns the visible/focused agent tab.
func (m *Model) activeClaudeTab() *claudeTab { return m.claudeTabs[m.activeClaude] }

// agentLabel is the uppercased name of the active tab's agent, used as the
// section header (e.g. "CLAUDE", "CODEX"); "AGENT" while still selecting one.
func (m *Model) agentLabel() string {
	if a := m.activeClaudeTab().agent; a.Name != "" {
		return strings.ToUpper(a.Name)
	}
	return "AGENT"
}

// newAgentPicker builds the agent selector from the installed agents.
func newAgentPicker(list []agents.Agent) *picker.Model {
	items := make([]picker.Item, 0, len(list))
	for _, a := range list {
		items = append(items, picker.Item{ID: a.Bin, Title: a.Name, Subtitle: a.Bin})
	}
	p := picker.New("Select an AI agent — ↑/↓ and Enter:", items)
	return &p
}

// newSessionPicker builds the CLAUDE session selector from past sessions; the
// first option is always to create a new session.
func newSessionPicker(past []sessions.Session) *picker.Model {
	items := []picker.Item{{Title: "New session", IsNew: true}}
	for _, s := range past {
		items = append(items, picker.Item{
			ID:       s.ID,
			Title:    s.Title,
			Subtitle: s.ModTime.Format("2006-01-02 15:04") + "  ·  " + shortID(s.ID),
		})
	}
	p := picker.New("Claude sessions in this directory — ↑/↓ and Enter:", items)
	return &p
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
		case m.focus == focusClaude && m.activeClaudeTab().picker == nil:
			tab := m.activeClaudeTab()
			tab.term.ScrollToBottom()
			tab.term.SendPaste(msg.Content)
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
		default:
			// A claude-cli process exited (e.g. Ctrl+C to quit): that tab returns
			// to its session menu. Terminal-tab exits are left as-is.
			if tab := m.claudeTabByID(msg.ID); tab != nil {
				m.resetClaudeTabToMenu(tab)
			}
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
	alt := altHeld(k.Mod)
	switch {
	case k.String() == "ctrl+q":
		m.cleanup()
		return m, tea.Quit
	case k.String() == "ctrl+a" && m.focus != focusClaude:
		// Focus the CLAUDE panel. When already focused, fall through so Ctrl+A
		// reaches claude (it is a common "start of line" binding).
		m.focus = focusClaude
		return m, nil
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
	case alt && (k.Code == '+' || k.Code == '='):
		m.newTab() // '+' is Shift+'='; accept either
		return m, nil
	case alt && k.Code == '-':
		m.closeTab()
		return m, nil
	case alt && (k.Code == 'a' || k.Code == 'A'):
		m.reselectAgent() // open the agent selector in a new tab
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
		// Alt+<n> jumps to CLAUDE tab n; Alt+Shift+Left/Right cycle tabs (Shift
		// is required for the arrows because many terminals reserve a bare
		// Alt+Left/Right for word navigation).
		if idx, ok := altTabIndex(k); ok {
			if idx < len(m.claudeTabs) {
				m.activeClaude = idx
			}
			return m, nil
		}
		if altHeld(k.Mod) && k.Mod.Contains(tea.ModShift) {
			switch k.Code {
			case tea.KeyLeft:
				m.switchClaude(-1)
				return m, nil
			case tea.KeyRight:
				m.switchClaude(1)
				return m, nil
			}
		}
		tab := m.activeClaudeTab()
		// While a selector (agent or session) is active, keys drive it.
		if tab.picker != nil {
			updated, chosen := tab.picker.Update(k)
			*tab.picker = updated
			if chosen != nil {
				m.selectInTab(tab, *chosen)
			}
			return m, nil
		}
		// Insert a newline in claude's prompt (instead of submitting) on
		// Shift+Enter, Ctrl+J or Alt/Option+Enter. Plain Enter still submits.
		// With the kitty keyboard protocol (negotiated by Bubble Tea v2),
		// Shift+Enter is now distinguishable on any supporting terminal.
		switch k.String() {
		case "shift+enter", "ctrl+j", "alt+enter":
			tab.term.ScrollToBottom()
			tab.term.SendNewline()
			return m, nil
		}
		// PgUp/PgDn scroll the CLAUDE history (its scrollback buffer), unless an
		// alt-screen program is running, in which case it does its own paging.
		// Any other key snaps back to the live view before reaching the process.
		if !tab.term.AltScreen() {
			switch k.Code {
			case tea.KeyPgUp:
				tab.term.ScrollPage(+1)
				return m, nil
			case tea.KeyPgDown:
				tab.term.ScrollPage(-1)
				return m, nil
			}
		}
		tab.term.ScrollToBottom()
		tab.term.SendKey(k)
	case focusTerminal:
		// Alt+Shift+Left/Right cycle tabs, Alt+<n> jumps to tab n, PgUp/PgDn
		// scroll the history; every other key goes to the terminal (after
		// snapping back to the live view). Shift is required for the arrows
		// because many terminals reserve a bare Alt+Left/Right for word nav.
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

// altTabIndex maps an Alt+<digit> press (with or without Shift) to a 0-based
// terminal tab index. With the kitty keyboard protocol the digit is reported
// directly in Code; on terminals without it an Alt+Shift+<digit> arrives as Alt
// + the US-layout shifted symbol (!@#$…). ok is false for any other key.
func altTabIndex(k tea.KeyPressMsg) (int, bool) {
	if !altHeld(k.Mod) {
		return 0, false
	}
	switch k.Code {
	case '1', '2', '3', '4', '5', '6', '7', '8', '9':
		return int(k.Code - '1'), true
	case '0':
		return 9, true
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

// toggleSidebar cycles the explorer through three states: hidden -> shown and
// focused; shown-but-unfocused -> focused (without hiding, so a stray Ctrl+B
// recovers focus instead of closing it); focused -> hidden (focus back to CLAUDE).
func (m *Model) toggleSidebar() {
	switch {
	case !m.showSidebar:
		m.sidebarPref = true
		m.layout()
		if m.showSidebar {
			m.focus = focusSidebar
		}
	case m.focus != focusSidebar:
		m.focus = focusSidebar
	default:
		m.sidebarPref = false
		m.focus = focusClaude
		m.layout()
	}
}

func (m *Model) startTerminals() {
	for _, t := range m.terms {
		_ = t.Start(m.prog)
	}
	// Each agent tab starts its process unless it is waiting on a selector.
	for _, tab := range m.claudeTabs {
		if tab.mode == pickNone {
			_ = tab.term.Start(m.prog)
		}
	}
}

// newTab / closeTab act on the focused tabbed section: the CLAUDE tabs when it
// is focused, otherwise the TERMINAL tabs.
func (m *Model) newTab() {
	if m.focus == focusClaude {
		m.newClaudeTab()
	} else {
		m.newTerminal()
	}
}

func (m *Model) closeTab() {
	if m.focus == focusClaude {
		m.closeClaudeTab()
	} else {
		m.closeTerminal()
	}
}

// activeTermModel returns the terminal of the active tab.
func (m *Model) activeTermModel() *terminal.Model { return m.terms[m.activeTerm] }

// newTerminal opens a new terminal tab, makes it active and focuses it.
func (m *Model) newTerminal() {
	m.showTerminals = true // creating a terminal reveals the section
	t := terminal.New(m.allocID(), "TERMINAL", m.dir, []string{m.shell})
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

// switchTerm moves the active terminal tab by delta (wrapping around).
func (m *Model) switchTerm(delta int) {
	n := len(m.terms)
	m.activeTerm = (m.activeTerm + delta + n) % n
}

// selectInTab applies the user's choice in a tab's active selector: in agent
// mode it sets the tab's agent (and the global default); in session mode it
// launches Claude (resuming unless "New session" was chosen).
func (m *Model) selectInTab(tab *claudeTab, choice picker.Item) {
	switch tab.mode {
	case pickAgent:
		if agent, ok := m.findAgent(choice.ID); ok {
			m.setDefaultAgent(agent)
			m.configureTab(tab, agent)
			if tab.mode == pickNone {
				m.launchTab(tab, nil)
			}
		}
	case pickSession:
		if choice.IsNew {
			m.launchTab(tab, nil)
		} else {
			m.launchTab(tab, &choice.ID)
		}
	}
}

// launchTab starts the tab's agent process. For Claude a non-empty resumeID
// resumes that session; other agents ignore it.
func (m *Model) launchTab(tab *claudeTab, resumeID *string) {
	args := tab.agent.Args
	if tab.agent.Bin == "claude" && resumeID != nil && *resumeID != "" {
		args = append(append([]string{}, args...), "--resume", *resumeID)
	}
	tab.mode = pickNone
	tab.picker = nil
	if tab.term.Started() {
		return
	}
	tab.term.SetArgs(args)
	if m.started {
		_ = tab.term.Start(m.prog)
	}
}

// newClaudeTab opens a new agent tab using the default agent (or the agent
// selector on first use), and focuses it.
func (m *Model) newClaudeTab() {
	tab := m.makeClaudeTab()
	m.claudeTabs = append(m.claudeTabs, tab)
	m.activeClaude = len(m.claudeTabs) - 1
	m.focus = focusClaude
	m.layout() // size the new tab before starting it
	if m.started && tab.mode == pickNone {
		_ = tab.term.Start(m.prog)
	}
}

// reselectAgent opens a new agent tab on the agent selector, so the user can
// pick a different agent (which also becomes the new default).
func (m *Model) reselectAgent() {
	tab := &claudeTab{}
	m.toAgentPicker(tab)
	m.claudeTabs = append(m.claudeTabs, tab)
	m.activeClaude = len(m.claudeTabs) - 1
	m.focus = focusClaude
	m.layout()
}

// closeClaudeTab closes the active CLAUDE tab. Closing the last remaining tab
// does not remove it; instead it returns that tab to its session menu.
func (m *Model) closeClaudeTab() {
	if len(m.claudeTabs) <= 1 {
		m.resetClaudeTabToMenu(m.claudeTabs[0])
		m.activeClaude = 0
		return
	}
	m.claudeTabs[m.activeClaude].term.Close()
	m.claudeTabs = append(m.claudeTabs[:m.activeClaude], m.claudeTabs[m.activeClaude+1:]...)
	if m.activeClaude >= len(m.claudeTabs) {
		m.activeClaude = len(m.claudeTabs) - 1
	}
	m.layout()
}

// switchClaude moves the active CLAUDE tab by delta (wrapping around).
func (m *Model) switchClaude(delta int) {
	n := len(m.claudeTabs)
	m.activeClaude = (m.activeClaude + delta + n) % n
}

// claudeTabByID returns the CLAUDE tab whose terminal has the given id, or nil.
func (m *Model) claudeTabByID(id int) *claudeTab {
	for _, tab := range m.claudeTabs {
		if tab.term.ID() == id {
			return tab
		}
	}
	return nil
}

// resetClaudeTabToMenu replaces a tab's exited agent with a fresh terminal and
// returns it to a selector (used when the agent exits or the last tab is
// closed): Claude returns to its session menu, other agents to the agent picker.
func (m *Model) resetClaudeTabToMenu(tab *claudeTab) {
	tab.term.Close()
	if tab.agent.Bin == "claude" {
		tab.term = terminal.New(m.allocID(), strings.ToUpper(tab.agent.Name), m.dir, tab.agent.Args)
		tab.term.SetEnv(tab.agent.Env)
		tab.picker = newSessionPicker(sessions.List(m.dir))
		tab.mode = pickSession
	} else {
		m.toAgentPicker(tab)
	}
	m.layout()
}

// toggleTerminals cycles the TERMINAL section through three states like
// toggleSidebar: hidden -> shown and focused; shown-but-unfocused -> focused;
// focused -> hidden (focus back to CLAUDE). The shells keep running while hidden.
func (m *Model) toggleTerminals() {
	switch {
	case !m.showTerminals:
		m.showTerminals = true
		m.focus = focusTerminal
	case m.focus != focusTerminal:
		m.focus = focusTerminal
	default:
		m.showTerminals = false
		m.focus = focusClaude
	}
	m.layout()
}

func (m *Model) cleanup() {
	for _, tab := range m.claudeTabs {
		tab.term.Close()
	}
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
	m.editorID = m.allocID()
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

// toggleGit cycles the Git panel through three states like toggleSidebar:
// hidden -> shown (refreshed) and focused; shown-but-unfocused -> focused;
// focused -> hidden (focus back to CLAUDE).
func (m *Model) toggleGit() {
	switch {
	case !m.showGit:
		m.showGit = true
		m.gitPanel.Refresh()
		m.focus = focusGit
	case m.focus != focusGit:
		m.gitPanel.Refresh()
		m.focus = focusGit
	default:
		m.showGit = false
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
	for _, tab := range m.claudeTabs {
		tab.term.SetSize(max(rightW, 1), max(m.claudeInnerH, 1))
		if tab.picker != nil {
			tab.picker.SetSize(max(rightW, 1), max(m.claudeInnerH, 1))
		}
	}
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

	// Right column: CLAUDE on top, TERMINAL below, each with a tabbed header. If
	// the active CLAUDE tab's session selector is active, it takes over that area.
	ctab := m.activeClaudeTab()
	ctab.term.SetFocused(m.focus == focusClaude)
	claudeView := ctab.term.View()
	if ctab.picker != nil {
		claudeView = ctab.picker.View()
	}
	divider := hLine(m.rightInnerW)
	claudeHeader := tabHeader(m.agentLabel(), len(m.claudeTabs), m.activeClaude, m.focus == focusClaude, m.rightInnerW)
	claudeContent := blockRect(claudeView, m.rightInnerW, m.claudeInnerH)

	var right string
	var jr []int // divider rows of the middle column, where the seams branch
	if m.showTerminals {
		at := m.activeTermModel()
		at.SetFocused(m.focus == focusTerminal)
		termHeader := tabHeader("TERMINAL", len(m.terms), m.activeTerm, m.focus == focusTerminal, m.rightInnerW)
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
