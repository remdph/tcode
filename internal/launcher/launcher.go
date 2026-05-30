// Package launcher shows a directory picker at startup: the execution directory
// plus the recently opened ones. It reuses the session picker UI.
package launcher

import (
	"os"
	"path/filepath"
	"strings"

	"code-tui/internal/picker"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Model is the launcher Bubble Tea model.
type Model struct {
	picker        picker.Model
	chosen        string
	width, height int
}

// New builds the launcher with EXEC DIR first, then the recent directories.
func New(execDir string, recents []string) Model {
	items := []picker.Item{{
		Title:    "CURRENT PATH",
		Subtitle: homify(execDir),
		ID:       execDir,
		IsNew:    true,
	}}
	execClean := filepath.Clean(execDir)
	for _, d := range recents {
		if filepath.Clean(d) == execClean {
			continue // already covered by EXEC DIR
		}
		items = append(items, picker.Item{
			Title:    filepath.Base(d), // directory name
			Subtitle: homify(d),        // full path on the second line
			ID:       d,
		})
	}
	return Model{picker: picker.New("Open a project — ↑/↓ and Enter:", items)}
}

func (m Model) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.picker.SetSize(min(msg.Width-4, 72), msg.Height-2)
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc", "q":
			return m, tea.Quit
		}
		p, choice := m.picker.Update(msg)
		m.picker = p
		if choice != nil {
			m.chosen = choice.ID
			return m, tea.Quit
		}
		return m, nil
	}
	return m, nil
}

// View implements tea.Model.
func (m Model) View() string {
	if m.width == 0 {
		return ""
	}
	content := m.picker.View()
	// Show the T-CODE logo above the selector when there is room for it.
	if m.width >= bannerWidth()+4 && m.height >= 18 {
		content = lipgloss.JoinVertical(lipgloss.Center, banner(), "", m.picker.View())
	}
	box := lipgloss.NewStyle().Padding(1, 2).Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

// Chosen returns the selected directory, or "" if the user cancelled.
func (m Model) Chosen() string { return m.chosen }

// homify replaces the home-directory prefix with ~.
func homify(path string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if path == home {
			return "~"
		}
		if strings.HasPrefix(path, home+"/") {
			return "~" + strings.TrimPrefix(path, home)
		}
	}
	return path
}
