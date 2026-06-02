package terminal

import (
	"unicode"

	tea "charm.land/bubbletea/v2"
)

// encodeKey converts a Bubble Tea v2 key press into the byte sequence a terminal
// process expects to receive on stdin.
func encodeKey(k tea.KeyPressMsg) []byte {
	var prefix []byte
	if k.Mod.Contains(tea.ModAlt) {
		prefix = []byte{0x1b} // ESC as the Meta/Alt prefix
	}

	// Control combinations: Ctrl+A..Ctrl+Z map to 0x01..0x1a, plus the handful
	// of other C0 codes. These take precedence over the key's printable text.
	if k.Mod.Contains(tea.ModCtrl) {
		switch c := k.Code; {
		case c >= 'a' && c <= 'z':
			return append(prefix, byte(c-'a'+1))
		case c >= 'A' && c <= 'Z':
			return append(prefix, byte(c-'A'+1))
		case c == ' ' || c == '@':
			return append(prefix, 0x00)
		case c == '[':
			return append(prefix, 0x1b)
		case c == '\\':
			return append(prefix, 0x1c)
		case c == ']':
			return append(prefix, 0x1d)
		case c == '^':
			return append(prefix, 0x1e)
		case c == '_':
			return append(prefix, 0x1f)
		}
		// Ctrl with a non-letter special key falls through to the code switch.
	}

	switch k.Code {
	case tea.KeyEnter:
		return append(prefix, '\r')
	case tea.KeyTab:
		return append(prefix, '\t')
	case tea.KeyBackspace:
		return append(prefix, 0x7f)
	case tea.KeyEscape:
		return append(prefix, 0x1b)
	case tea.KeySpace:
		return append(prefix, ' ')
	case tea.KeyUp:
		return append(prefix, 0x1b, '[', 'A')
	case tea.KeyDown:
		return append(prefix, 0x1b, '[', 'B')
	case tea.KeyRight:
		return append(prefix, 0x1b, '[', 'C')
	case tea.KeyLeft:
		return append(prefix, 0x1b, '[', 'D')
	case tea.KeyHome:
		return append(prefix, 0x1b, '[', 'H')
	case tea.KeyEnd:
		return append(prefix, 0x1b, '[', 'F')
	case tea.KeyPgUp:
		return append(prefix, 0x1b, '[', '5', '~')
	case tea.KeyPgDown:
		return append(prefix, 0x1b, '[', '6', '~')
	case tea.KeyDelete:
		return append(prefix, 0x1b, '[', '3', '~')
	}

	// Printable characters: prefer the resolved text (handles shifted symbols
	// and non-Latin input), falling back to the raw code point.
	if k.Text != "" {
		return append(prefix, []byte(k.Text)...)
	}
	if k.Code != 0 && unicode.IsPrint(k.Code) {
		return append(prefix, []byte(string(k.Code))...)
	}
	return prefix
}
