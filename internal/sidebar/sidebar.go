// Package sidebar implementa el explorador de archivos lateral: un árbol
// navegable con carpetas plegables.
package sidebar

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type node struct {
	name     string
	path     string
	isDir    bool
	expanded bool
	loaded   bool
	depth    int
	children []*node
}

// Model es el estado del explorador.
type Model struct {
	root          *node
	flat          []*node
	cursor        int
	offset        int
	width, height int
}

var (
	dirStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("75"))
	fileStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Background(lipgloss.Color("24"))
)

// New crea el explorador apuntando a dir.
func New(dir string) Model {
	root := &node{name: filepath.Base(dir), path: dir, isDir: true, expanded: true}
	m := Model{root: root, width: 28, height: 20}
	loadChildren(root)
	m.rebuild()
	return m
}

// loadChildren lee las entradas del directorio del nodo (carpetas primero).
func loadChildren(n *node) {
	n.loaded = true
	entries, err := os.ReadDir(n.path)
	if err != nil {
		return
	}
	sort.Slice(entries, func(i, j int) bool {
		di, dj := entries[i].IsDir(), entries[j].IsDir()
		if di != dj {
			return di
		}
		return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
	})
	n.children = n.children[:0]
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") && name != ".." {
			// Ocultar dotfiles por defecto (como VSCode con files.exclude).
			continue
		}
		n.children = append(n.children, &node{
			name:  name,
			path:  filepath.Join(n.path, name),
			isDir: e.IsDir(),
			depth: n.depth + 1,
		})
	}
}

// rebuild recompone la lista plana de nodos visibles.
func (m *Model) rebuild() {
	m.flat = m.flat[:0]
	var walk func(n *node)
	walk = func(n *node) {
		for _, c := range n.children {
			m.flat = append(m.flat, c)
			if c.isDir && c.expanded {
				walk(c)
			}
		}
	}
	walk(m.root)
	if m.cursor >= len(m.flat) {
		m.cursor = len(m.flat) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

// SetSize fija el tamaño interior disponible (en celdas).
func (m *Model) SetSize(w, h int) {
	m.width, m.height = w, h
}

// Update procesa la navegación cuando el explorador tiene el foco.
func (m Model) Update(k tea.KeyMsg) (Model, tea.Cmd) {
	switch k.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.flat)-1 {
			m.cursor++
		}
	case "enter", "right", "l":
		if n := m.current(); n != nil && n.isDir {
			if !n.expanded && !n.loaded {
				loadChildren(n)
			}
			n.expanded = !n.expanded
			m.rebuild()
		}
	case "left", "h":
		if n := m.current(); n != nil {
			if n.isDir && n.expanded {
				n.expanded = false
				m.rebuild()
			}
		}
	case "g", "home":
		m.cursor = 0
	case "G", "end":
		m.cursor = len(m.flat) - 1
	}
	m.ensureVisible()
	return m, nil
}

func (m *Model) current() *node {
	if m.cursor < 0 || m.cursor >= len(m.flat) {
		return nil
	}
	return m.flat[m.cursor]
}

func (m *Model) ensureVisible() {
	if m.height <= 0 {
		return
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+m.height {
		m.offset = m.cursor - m.height + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

// View renderiza el árbol dentro del área asignada.
func (m Model) View() string {
	if len(m.flat) == 0 {
		return fileStyle.Render("(vacío)")
	}
	var lines []string
	end := m.offset + m.height
	if end > len(m.flat) {
		end = len(m.flat)
	}
	for i := m.offset; i < end; i++ {
		n := m.flat[i]
		indent := strings.Repeat("  ", n.depth-1)
		var icon string
		switch {
		case n.isDir && n.expanded:
			icon = "▾ "
		case n.isDir:
			icon = "▸ "
		default:
			icon = "  "
		}
		label := indent + icon + n.name
		label = truncate(label, m.width)
		if i == m.cursor {
			label = padRight(label, m.width)
			lines = append(lines, selectedStyle.Render(label))
		} else if n.isDir {
			lines = append(lines, dirStyle.Render(label))
		} else {
			lines = append(lines, fileStyle.Render(label))
		}
	}
	return strings.Join(lines, "\n")
}

func truncate(s string, w int) string {
	r := []rune(s)
	if w <= 0 || len(r) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	return string(r[:w-1]) + "…"
}

func padRight(s string, w int) string {
	n := w - len([]rune(s))
	if n <= 0 {
		return s
	}
	return s + strings.Repeat(" ", n)
}
