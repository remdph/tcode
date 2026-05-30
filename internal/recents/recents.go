// Package recents keeps a global list of recently opened project directories.
package recents

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const maxRecents = 20

// file returns the path of the recents store, or "" if the home dir is unknown.
func file() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "code-tui", "recents.json")
}

// List returns the recently opened directories, most recent first. Entries that
// no longer exist are skipped.
func List() []string {
	f := file()
	if f == "" {
		return nil
	}
	data, err := os.ReadFile(f)
	if err != nil {
		return nil
	}
	var dirs []string
	if json.Unmarshal(data, &dirs) != nil {
		return nil
	}
	var out []string
	for _, d := range dirs {
		if info, err := os.Stat(d); err == nil && info.IsDir() {
			out = append(out, d)
		}
	}
	return out
}

// Add records dir as the most recently opened directory.
func Add(dir string) {
	f := file()
	if f == "" {
		return
	}
	dir = filepath.Clean(dir)

	// Rebuild the list with dir first and no duplicates.
	updated := []string{dir}
	for _, d := range List() {
		if filepath.Clean(d) != dir {
			updated = append(updated, d)
		}
	}
	if len(updated) > maxRecents {
		updated = updated[:maxRecents]
	}

	data, err := json.MarshalIndent(updated, "", "  ")
	if err != nil {
		return
	}
	if os.MkdirAll(filepath.Dir(f), 0o755) == nil {
		_ = os.WriteFile(f, data, 0o644)
	}
}
