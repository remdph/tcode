// Package quickopen implements a VSCode-style "quick open" fuzzy file finder:
// a floating prompt that filters the project's files as you type and opens the
// selected one.
package quickopen

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"code-tui/internal/theme"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
)

// maxFiles caps how many files are indexed, to stay responsive in huge trees.
const maxFiles = 50000

// skipDirs are directory names never descended into while indexing.
var skipDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	".hg":          true,
	".svn":         true,
}

// Model is the quick-open finder state.
type Model struct {
	root    string
	files   []string // file paths relative to root
	query   []rune
	matches []string // filtered file paths (relative), best first
	cursor  int
	offset  int

	width, height int
}

var (
	promptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Bold(true)
	nameStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	dirStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	emptyStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Italic(true)
	countStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

func accentStyle() lipgloss.Style { return lipgloss.NewStyle().Foreground(theme.Accent) }
func selNameStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Bold(true)
}
func cursorStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.OnAccent).Background(theme.Accent)
}

// New creates a finder rooted at dir, indexing its files immediately.
func New(dir string) Model {
	m := Model{root: dir, width: 60, height: 20}
	m.files = gatherFiles(dir)
	m.refilter()
	return m
}

// gatherFiles walks root and returns file paths relative to it (folders, and a
// few noise directories such as .git and node_modules, are skipped).
func gatherFiles(root string) []string {
	var files []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if path != root && skipDirs[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		if rel, err := filepath.Rel(root, path); err == nil {
			files = append(files, rel)
			if len(files) >= maxFiles {
				return fs.SkipAll
			}
		}
		return nil
	})
	return files
}

// SetSize sets the available inner area of the finder.
func (m *Model) SetSize(w, h int) { m.width, m.height = w, h }

// Update processes a key. It returns the updated model and, when the user
// finishes, an outcome: open is the absolute path to open (empty otherwise),
// and cancelled is true when the finder should close without opening anything.
func (m Model) Update(k tea.KeyPressMsg) (_ Model, open string, cancelled bool) {
	switch k.String() {
	case "esc", "ctrl+c":
		return m, "", true
	case "enter":
		if p, ok := m.currentPath(); ok {
			return m, p, false
		}
	case "up", "ctrl+p", "ctrl+k":
		m.moveCursor(-1)
	case "down", "ctrl+n", "ctrl+j":
		m.moveCursor(1)
	case "backspace":
		if n := len(m.query); n > 0 {
			m.query = m.query[:n-1]
			m.refilter()
		}
	case "ctrl+u":
		m.query = m.query[:0]
		m.refilter()
	case "space":
		m.query = append(m.query, ' ')
		m.refilter()
	default:
		// A printable character (no control/alt modifier) extends the query.
		if k.Text != "" && k.Mod&(tea.ModCtrl|tea.ModAlt|tea.ModSuper|tea.ModMeta) == 0 {
			m.query = append(m.query, []rune(k.Text)...)
			m.refilter()
		}
	}
	m.ensureVisible()
	return m, "", false
}

// currentPath returns the absolute path of the selected match, if any.
func (m *Model) currentPath() (string, bool) {
	if m.cursor < 0 || m.cursor >= len(m.matches) {
		return "", false
	}
	return filepath.Join(m.root, m.matches[m.cursor]), true
}

func (m *Model) moveCursor(delta int) {
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.matches) {
		m.cursor = len(m.matches) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

// refilter recomputes the match list for the current query, ranked best-first.
func (m *Model) refilter() {
	m.cursor, m.offset = 0, 0
	query := string(m.query)
	if query == "" {
		// No query: show every file, shortest paths first (closest to the root).
		m.matches = append(m.matches[:0], m.files...)
		sort.Slice(m.matches, func(i, j int) bool {
			if len(m.matches[i]) != len(m.matches[j]) {
				return len(m.matches[i]) < len(m.matches[j])
			}
			return m.matches[i] < m.matches[j]
		})
		return
	}

	type scored struct {
		path  string
		score int
	}
	var hits []scored
	for _, f := range m.files {
		if s, ok := score(f, query); ok {
			hits = append(hits, scored{f, s})
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		if len(hits[i].path) != len(hits[j].path) {
			return len(hits[i].path) < len(hits[j].path)
		}
		return hits[i].path < hits[j].path
	})
	m.matches = m.matches[:0]
	for _, h := range hits {
		m.matches = append(m.matches, h.path)
	}
}

// score returns a fuzzy-match score for query against path (case-insensitive),
// requiring every query character to appear as an in-order subsequence. A match
// within the file name is strongly preferred over a match that only spans the
// directory path. ok is false when the query does not match at all.
func score(path, query string) (int, bool) {
	lp := strings.ToLower(path)
	lq := strings.ToLower(query)
	base := lp[strings.LastIndexByte(lp, '/')+1:]

	// A name match dominates path-only matches (the +1000 floor).
	if s, ok := seqScore(base, lq); ok {
		return s + 1000 - len(path)/50, true
	}
	if s, ok := seqScore(lp, lq); ok {
		return s - len(path)/50, true
	}
	return 0, false
}

// seqScore scores query as an in-order subsequence of text, rewarding matches
// at word boundaries and in consecutive runs. ok is false if text does not
// contain the full subsequence.
func seqScore(text, query string) (int, bool) {
	total, qi, prev := 0, 0, -2
	for i := 0; i < len(text) && qi < len(query); i++ {
		if text[i] != query[qi] {
			continue
		}
		s := 1
		if i == prev+1 {
			s += 5 // consecutive run
		}
		if i == 0 || isBoundary(text[i-1]) {
			s += 10 // start of a word segment
		}
		total += s
		prev = i
		qi++
	}
	if qi < len(query) {
		return 0, false
	}
	return total, true
}

func isBoundary(b byte) bool {
	return b == '/' || b == '_' || b == '-' || b == '.' || b == ' '
}

func (m *Model) ensureVisible() {
	if m.height <= 1 {
		return
	}
	rows := m.listRows()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+rows {
		m.offset = m.cursor - rows + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

// listRows is the number of result rows that fit (the prompt takes one row plus
// a blank separator).
func (m *Model) listRows() int {
	r := m.height - 2
	if r < 1 {
		r = 1
	}
	return r
}

// View renders the finder content (prompt line plus the result list).
func (m Model) View() string {
	var b strings.Builder

	// Prompt line: "› query▏" with a block cursor.
	prompt := accentStyle().Render("› ") + promptStyle.Render(string(m.query)) +
		cursorStyle().Render(" ")
	count := countStyle.Render(plural(len(m.matches)))
	b.WriteString(fitLine(prompt, count, m.width))
	b.WriteString("\n\n")

	if len(m.matches) == 0 {
		b.WriteString(emptyStyle.Render("  (no matching files)"))
		return b.String()
	}

	rows := m.listRows()
	end := m.offset + rows
	if end > len(m.matches) {
		end = len(m.matches)
	}
	for i := m.offset; i < end; i++ {
		rel := m.matches[i]
		name := filepath.Base(rel)
		dir := filepath.Dir(rel)
		if dir == "." {
			dir = ""
		}
		selected := i == m.cursor

		bar := "  "
		if selected {
			bar = accentStyle().Render("▎ ")
		}

		ns := nameStyle
		if selected {
			ns = selNameStyle()
		}
		line := bar + ns.Render(name)
		if dir != "" {
			line += "  " + dirStyle.Render(dir)
		}
		b.WriteString(truncate(line, m.width))
		if i < end-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func plural(n int) string {
	if n == 1 {
		return "1 result"
	}
	return itoa(n) + " results"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// fitLine renders left and right segments on one line of width w, with the
// right segment pushed to the edge and dropped if there is no room.
func fitLine(left, right string, w int) string {
	lw, rw := lipgloss.Width(left), lipgloss.Width(right)
	if lw+rw+1 > w {
		return truncate(left, w)
	}
	gap := w - lw - rw
	return left + strings.Repeat(" ", gap) + right
}

// truncate trims styled text to a maximum visible width, adding an ellipsis.
func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	return lipgloss.NewStyle().MaxWidth(w).Render(s)
}
