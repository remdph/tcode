// Command code-tui is a VSCode-like environment for the terminal: a file
// explorer on the left, claude-cli on the right and a terminal below.
//
// Usage:
//
//	code-tui [directory]
//
// With no argument it opens the current working directory as the project.
// If a directory is given, that folder is opened instead.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"code-tui/internal/app"
	"code-tui/internal/launcher"
	"code-tui/internal/recents"
	"code-tui/internal/theme"

	tea "charm.land/bubbletea/v2"
)

// version is the current release of code-tui (installed as `tcode`).
const version = "0.1.0"

func main() {
	prog := filepath.Base(os.Args[0])
	args := os.Args[1:]

	if len(args) == 1 {
		switch args[0] {
		case "-v", "--version", "version":
			fmt.Printf("%s %s\n", prog, version)
			return
		case "-h", "--help", "help":
			fmt.Printf("Usage: %s [directory]\n\n"+
				"Opens directory (or the current directory) as a project: a file\n"+
				"explorer, claude-cli, terminals and a Git panel.\n", prog)
			return
		}
	}

	theme.Load()

	var dir string
	if len(args) >= 1 {
		// An explicit directory was given: open it directly.
		d, err := resolveDir(args)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", prog, err)
			os.Exit(1)
		}
		dir = d
	} else {
		// No argument: show the directory launcher (EXEC DIR + recents).
		execDir, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", prog, err)
			os.Exit(1)
		}
		d, ok := runLauncher(execDir)
		if !ok {
			return // cancelled
		}
		dir = d
	}

	recents.Add(dir)

	m := app.New(dir)
	p := tea.NewProgram(m)
	m.SetProgram(p)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", prog, err)
		os.Exit(1)
	}
}

// runLauncher shows the directory picker and returns the chosen directory, or
// ("", false) if the user cancelled.
func runLauncher(execDir string) (string, bool) {
	m := launcher.New(execDir, recents.List())
	res, err := tea.NewProgram(m).Run()
	if err != nil {
		return "", false
	}
	chosen := res.(launcher.Model).Chosen()
	return chosen, chosen != ""
}

// resolveDir determines the project directory to open. With no argument it uses
// the current working directory; with one argument it uses (and validates) that
// path. A leading ~ is expanded to the home directory.
func resolveDir(args []string) (string, error) {
	if len(args) == 0 {
		return os.Getwd()
	}
	if len(args) > 1 {
		return "", fmt.Errorf("expected at most one directory argument, got %d", len(args))
	}

	dir := expandHome(args[0])
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("cannot open %q: %w", args[0], err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%q is not a directory", args[0])
	}
	return abs, nil
}

// expandHome replaces a leading ~ with the user's home directory.
func expandHome(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(path[1:], "/"))
		}
	}
	return path
}
