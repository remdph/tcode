// Package sessions discovers the Claude Code sessions saved for a given working
// directory.
package sessions

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Session describes a past Claude Code session.
type Session struct {
	ID      string    // identifier (the .jsonl file name without extension)
	Title   string    // first user message, for display
	ModTime time.Time // last modification of the transcript
}

// encodeDir reproduces Claude Code's encoding of the project path: every
// non-alphanumeric character is replaced with '-'.
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

// projectDir returns Claude's transcript folder for dir.
func projectDir(dir string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", "projects", encodeDir(dir))
}

// List returns the sessions for dir, ordered from most to least recent. It
// returns nil when there are none.
func List(dir string) []Session {
	proj := projectDir(dir)
	if proj == "" {
		return nil
	}
	entries, err := os.ReadDir(proj)
	if err != nil {
		return nil
	}

	var out []Session
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		s := Session{
			ID:      strings.TrimSuffix(e.Name(), ".jsonl"),
			ModTime: info.ModTime(),
			Title:   firstUserMessage(filepath.Join(proj, e.Name())),
		}
		if s.Title == "" {
			s.Title = "(untitled session)"
		}
		out = append(out, s)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].ModTime.After(out[j].ModTime)
	})
	return out
}

// record is the minimal shape of a transcript line that we care about.
type record struct {
	Type    string `json:"type"`
	Message struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

// firstUserMessage extracts the first real user message from the transcript,
// skipping metadata or system entries.
func firstUserMessage(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	// Transcript lines can be large (tool results), so grow the buffer.
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	for sc.Scan() {
		var rec record
		if json.Unmarshal(sc.Bytes(), &rec) != nil || rec.Type != "user" {
			continue
		}
		text := clean(extractText(rec.Message.Content))
		if text == "" || strings.HasPrefix(text, "<") || strings.HasPrefix(text, "Caveat:") {
			continue
		}
		return text
	}
	return ""
}

// extractText pulls the text out of the content field, which may be a string or
// an array of { "type": "text", "text": ... } blocks.
func extractText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) == nil {
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				return b.Text
			}
		}
	}
	return ""
}

// clean normalizes the title to a single readable line.
func clean(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(strings.Join(strings.Fields(s), " "))
}
