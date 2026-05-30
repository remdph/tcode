// Command code-tui is a VSCode-like environment for the terminal: a file
// explorer on the left, claude-cli on the right and a terminal below.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"code-tui/internal/app"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}

	m := app.New(dir)
	p := tea.NewProgram(m, tea.WithAltScreen())
	m.SetProgram(p)

	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
