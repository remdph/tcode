// Package sessions descubre las sesiones de Claude Code guardadas para un
// directorio de trabajo concreto.
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

// Session resume una sesión pasada de Claude Code.
type Session struct {
	ID      string    // identificador (nombre del .jsonl sin extensión)
	Title   string    // primer mensaje del usuario, para mostrar
	ModTime time.Time // última modificación del transcript
}

// encodeDir reproduce la codificación de Claude Code para la ruta del proyecto:
// cada carácter no alfanumérico se sustituye por '-'.
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

// projectDir devuelve la carpeta de transcripts de Claude para dir.
func projectDir(dir string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", "projects", encodeDir(dir))
}

// List devuelve las sesiones de dir ordenadas de la más reciente a la más
// antigua. Devuelve nil si no hay ninguna.
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
			s.Title = "(sesión sin título)"
		}
		out = append(out, s)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].ModTime.After(out[j].ModTime)
	})
	return out
}

// record es la forma mínima de una línea del transcript que nos interesa.
type record struct {
	Type    string `json:"type"`
	Message struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

// firstUserMessage extrae el primer mensaje real del usuario del transcript,
// saltando entradas de metadatos o de sistema.
func firstUserMessage(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	// Las líneas del transcript pueden ser grandes (resultados de herramientas).
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

// extractText obtiene el texto del campo content, que puede ser un string o un
// array de bloques { "type": "text", "text": ... }.
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

// clean normaliza el título a una sola línea legible.
func clean(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(strings.Join(strings.Fields(s), " "))
}
