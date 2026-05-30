// Package config persists per-project UI settings (e.g. the explorer width).
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Project holds the persisted settings for a single working directory.
type Project struct {
	SidebarWidth int `json:"sidebarWidth,omitempty"`
	GitWidth     int `json:"gitWidth,omitempty"`
}

// encodeDir turns a directory path into a safe file name (non-alphanumeric
// characters become '-').
func encodeDir(dir string) string {
	var b strings.Builder
	for _, r := range dir {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return b.String()
}

// path returns the config file path for dir, or "" if the home dir is unknown.
func path(dir string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "code-tui", "projects", encodeDir(dir)+".json")
}

// Load reads the settings for dir. It returns a zero-value Project when there is
// no saved config.
func Load(dir string) Project {
	var p Project
	file := path(dir)
	if file == "" {
		return p
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return p
	}
	_ = json.Unmarshal(data, &p)
	return p
}

// Save writes the settings for dir, creating the config directory as needed.
func Save(dir string, p Project) error {
	file := path(dir)
	if file == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(file, data, 0o644)
}
