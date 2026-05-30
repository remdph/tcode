package terminal

import (
	tea "github.com/charmbracelet/bubbletea"
)

// encodeKey convierte una pulsación de Bubble Tea en la secuencia de bytes que
// un proceso de terminal espera recibir por stdin.
func encodeKey(k tea.KeyMsg) []byte {
	var prefix []byte
	if k.Alt {
		prefix = []byte{0x1b} // ESC como prefijo de Meta/Alt
	}

	switch k.Type {
	case tea.KeyRunes:
		return append(prefix, []byte(string(k.Runes))...)
	case tea.KeySpace:
		return append(prefix, ' ')
	case tea.KeyEnter:
		return append(prefix, '\r')
	case tea.KeyTab:
		return append(prefix, '\t')
	case tea.KeyBackspace:
		return append(prefix, 0x7f)
	case tea.KeyEsc:
		return append(prefix, 0x1b)
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
	default:
		// Las teclas de control (Ctrl+A..Ctrl+Z, etc.) tienen un KeyType cuyo
		// valor coincide con el código ASCII de control correspondiente.
		if k.Type >= 0 && int(k.Type) <= 31 {
			return append(prefix, byte(k.Type))
		}
	}
	return prefix
}
