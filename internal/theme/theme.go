// Package theme resolves the accent color used to highlight focus and
// selection. When an Omarchy theme is present, its accent is used; otherwise a
// built-in blue scheme is kept.
package theme

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Accent is the highlight color (focus and selection). OnAccent is the text
// color drawn on top of Accent. They default to the built-in blue scheme and
// are overridden by Load when an Omarchy accent is found.
var (
	Accent   = lipgloss.Color("39")
	OnAccent = lipgloss.Color("231")
)

// Loaded reports whether an Omarchy accent was applied.
var Loaded bool

// Load reads the current Omarchy theme's accent color, if available, and sets
// Accent/OnAccent accordingly. It is meant to be called once at startup, before
// anything renders.
func Load() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	path := filepath.Join(home, ".config", "omarchy", "current", "theme", "colors.toml")
	colors := parseColors(path)

	accent := firstNonEmpty(colors["accent"], colors["active_border_color"], colors["selection_background"])
	if !isHexColor(accent) {
		return
	}
	Accent = lipgloss.Color(accent)
	if on := colors["selection_foreground"]; isHexColor(on) {
		OnAccent = lipgloss.Color(on)
	}
	Loaded = true
}

// parseColors reads a simple `key = "value"` TOML file into a map. It is not a
// full TOML parser: it only handles the flat key/value lines Omarchy emits.
func parseColors(path string) map[string]string {
	out := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue // blank line or comment
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		val = strings.Trim(val, `"'`)
		out[key] = val
	}
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// isHexColor reports whether s looks like #rgb or #rrggbb.
func isHexColor(s string) bool {
	if !strings.HasPrefix(s, "#") || (len(s) != 4 && len(s) != 7) {
		return false
	}
	for _, c := range s[1:] {
		isHex := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
		if !isHex {
			return false
		}
	}
	return true
}
