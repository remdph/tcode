// Package gitpanel implements the right-hand Git panel with two tabs: CHANGES
// (the pending working-tree changes) and HISTORY (the commit log of the current
// branch, with author, date and tags).
package gitpanel

import (
	"os/exec"
	"strings"

	"code-tui/internal/theme"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
)

type tab int

const (
	tabChanges tab = iota
	tabHistory
)

type change struct {
	code string // two-letter porcelain status (e.g. " M", "??")
	path string
}

type commit struct {
	hash    string
	subject string
	author  string
	date    string // relative, e.g. "5 minutes ago"
	tags    []string
}

// Model is the Git panel state.
type Model struct {
	dir string

	tab    tab
	cursor int
	offset int

	isRepo  bool
	branch  string
	changes []change
	commits []commit
	loadErr string

	width, height int
}

var (
	headerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	pathStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	subjStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	hashStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("179"))
)

// New creates the Git panel for dir and loads its data.
func New(dir string) Model {
	m := Model{dir: dir, width: 36, height: 20}
	m.Refresh()
	return m
}

// SetSize sets the available inner size.
func (m *Model) SetSize(w, h int) { m.width, m.height = w, h }

// Refresh re-runs the git commands and repopulates the panel.
func (m *Model) Refresh() {
	m.loadErr = ""
	if strings.TrimSpace(m.git("rev-parse", "--is-inside-work-tree")) != "true" {
		m.isRepo = false
		m.loadErr = "Not a git repository"
		return
	}
	m.isRepo = true
	m.branch = strings.TrimSpace(m.git("rev-parse", "--abbrev-ref", "HEAD"))
	m.changes = parseChanges(m.git("status", "--porcelain=v1"))
	m.commits = parseCommits(m.git(
		"-c", "log.showSignature=false", "log", "--decorate=short",
		"--pretty=format:%h%x1f%D%x1f%an%x1f%ar%x1f%s", "-n", "300",
	))
	m.clampCursor()
}

// git runs a git command in the panel's directory and returns stdout (empty on
// error).
func (m *Model) git(args ...string) string {
	cmd := exec.Command("git", append([]string{"-C", m.dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(out)
}

func parseChanges(out string) []change {
	var cs []change
	for _, line := range strings.Split(out, "\n") {
		if len(line) < 4 {
			continue
		}
		cs = append(cs, change{code: line[:2], path: line[3:]})
	}
	return cs
}

func parseCommits(out string) []commit {
	var cs []commit
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		f := strings.Split(line, "\x1f")
		if len(f) < 5 {
			continue
		}
		c := commit{hash: f[0], author: f[2], date: f[3], subject: f[4]}
		for _, ref := range strings.Split(f[1], ",") {
			ref = strings.TrimSpace(ref)
			if t, ok := strings.CutPrefix(ref, "tag: "); ok {
				c.tags = append(c.tags, t)
			}
		}
		cs = append(cs, c)
	}
	return cs
}

// Update handles navigation when the panel is focused.
func (m Model) Update(k tea.KeyPressMsg) (Model, tea.Cmd) {
	switch k.String() {
	case "up", "k", "ctrl+p":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j", "ctrl+n":
		if m.cursor < m.itemCount()-1 {
			m.cursor++
		}
	case "tab", "right", "l", "shift+tab", "left", "h":
		if m.tab == tabChanges {
			m.tab = tabHistory
		} else {
			m.tab = tabChanges
		}
		m.cursor, m.offset = 0, 0
	case "g", "home":
		m.cursor = 0
	case "G", "end":
		m.cursor = m.itemCount() - 1
	case "r":
		m.Refresh()
	}
	m.clampCursor()
	m.ensureVisible()
	return m, nil
}

// Tab returns whether the active tab is CHANGES (true) or HISTORY (false).
func (m Model) onChanges() bool { return m.tab == tabChanges }

func (m Model) itemCount() int {
	if m.onChanges() {
		return len(m.changes)
	}
	return len(m.commits)
}

// rowsPerItem: history items take 2 rows, changes take 1.
func (m Model) rowsPerItem() int {
	if m.onChanges() {
		return 1
	}
	return 2
}

func (m *Model) clampCursor() {
	if m.cursor >= m.itemCount() {
		m.cursor = m.itemCount() - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m *Model) ensureVisible() {
	per := m.rowsPerItem()
	visible := m.height / per
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

// TabBar renders the CHANGES/HISTORY tab row.
func (m Model) TabBar(focused bool, w int) string {
	tabChip := func(label string, active bool) string {
		if active {
			return lipgloss.NewStyle().
				Foreground(theme.OnAccent).Background(theme.Accent).Bold(true).
				Render(" " + label + " ")
		}
		fg := lipgloss.Color("240")
		if focused {
			fg = lipgloss.Color("245")
		}
		return lipgloss.NewStyle().Foreground(fg).Render(" " + label + " ")
	}
	line := lipgloss.JoinHorizontal(lipgloss.Top,
		tabChip("CHANGES", m.tab == tabChanges),
		tabChip("HISTORY", m.tab == tabHistory),
	)
	return lipgloss.NewStyle().Inline(true).Width(w).MaxWidth(w).Render(line)
}

// View renders the content of the active tab.
func (m Model) View() string {
	if !m.isRepo {
		return dimStyle.Render(orDefault(m.loadErr, "Not a git repository"))
	}
	if m.onChanges() {
		return m.changesView()
	}
	return m.historyView()
}

func (m Model) changesView() string {
	if len(m.changes) == 0 {
		return dimStyle.Render("✓ Working tree clean") + "\n" +
			dimStyle.Render("on "+m.branch)
	}
	var lines []string
	end := min(m.offset+m.height, len(m.changes))
	for i := m.offset; i < end; i++ {
		c := m.changes[i]
		marker := statusStyle(c.code).Render(symbol(c.code))
		label := marker + " " + c.path
		lines = append(lines, m.row(label, i == m.cursor))
	}
	return strings.Join(lines, "\n")
}

func (m Model) historyView() string {
	if len(m.commits) == 0 {
		return dimStyle.Render("(no commits)")
	}
	var lines []string
	visible := m.height / 2
	if visible < 1 {
		visible = 1
	}
	end := min(m.offset+visible, len(m.commits))
	for i := m.offset; i < end; i++ {
		c := m.commits[i]
		selected := i == m.cursor

		// Tags go before the subject so they survive truncation.
		first := hashStyle.Render(c.hash)
		for _, t := range c.tags {
			first += " " + tagStyle().Render("⚑"+t)
		}
		first += " " + subjStyle.Render(c.subject)
		lines = append(lines, m.row(first, selected))

		meta := c.author + " · " + c.date
		lines = append(lines, m.metaRow(meta, selected))
	}
	return strings.Join(lines, "\n")
}

// row renders a single selectable line, highlighting it when selected.
func (m Model) row(content string, selected bool) string {
	content = clip(content, m.width)
	if selected {
		return lipgloss.NewStyle().
			Foreground(theme.OnAccent).Background(theme.Accent).
			Width(m.width).MaxWidth(m.width).Inline(true).Render(content)
	}
	return content
}

func (m Model) metaRow(text string, selected bool) string {
	text = "  " + clip(text, m.width-2)
	if selected {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("153")).Render(text)
	}
	return dimStyle.Render(text)
}

func tagStyle() lipgloss.Style { return lipgloss.NewStyle().Foreground(theme.Accent).Bold(true) }

// symbol/statusStyle map porcelain codes to a readable marker and color.
func symbol(code string) string { return code }

func statusStyle(code string) lipgloss.Style {
	c := lipgloss.Color("250")
	switch {
	case strings.Contains(code, "?"):
		c = lipgloss.Color("78") // untracked - green
	case strings.Contains(code, "A"):
		c = lipgloss.Color("78")
	case strings.Contains(code, "D"):
		c = lipgloss.Color("203") // deleted - red
	case strings.Contains(code, "R"):
		c = lipgloss.Color("75") // renamed - blue
	case strings.Contains(code, "M"):
		c = lipgloss.Color("179") // modified - yellow
	}
	return lipgloss.NewStyle().Foreground(c).Bold(true)
}

func clip(s string, w int) string {
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

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
