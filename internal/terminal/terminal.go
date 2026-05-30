// Package terminal implementa un panel respaldado por un pseudo-terminal (PTY)
// y un emulador de terminal virtual. Se usa para incrustar procesos
// interactivos (claude-cli, una shell) dentro de la TUI.
package terminal

import (
	"os"
	"os/exec"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/vt"
	"github.com/creack/pty"
)

// RefreshMsg se emite cuando el PTY produjo nueva salida y el panel debe
// repintarse.
type RefreshMsg struct{ ID int }

// ExitMsg se emite cuando el proceso del PTY terminó.
type ExitMsg struct {
	ID  int
	Err error
}

// Model es un panel de terminal incrustado.
type Model struct {
	id   int
	name string
	dir  string
	args []string

	emu  *vt.SafeEmulator
	ptmx *os.File
	cmd  *exec.Cmd
	prog *tea.Program

	mu            sync.Mutex
	width, height int
	started       bool
	dead          bool
}

// New crea un panel de terminal que ejecutará args[0] con args[1:] en dir.
// El proceso no se lanza hasta llamar a Start.
func New(id int, name, dir string, args []string) *Model {
	return &Model{
		id:     id,
		name:   name,
		dir:    dir,
		args:   args,
		width:  80,
		height: 24,
	}
}

// Name devuelve el rótulo del panel.
func (m *Model) Name() string { return m.name }

// Dead indica si el proceso subyacente terminó.
func (m *Model) Dead() bool { return m.dead }

// Start lanza el proceso en un PTY con el tamaño actual y arranca el bucle de
// lectura. prog se usa para notificar al programa de Bubble Tea.
func (m *Model) Start(prog *tea.Program) error {
	m.prog = prog
	m.mu.Lock()
	w, h := m.width, m.height
	m.mu.Unlock()

	m.emu = vt.NewSafeEmulator(w, h)

	c := exec.Command(m.args[0], m.args[1:]...)
	c.Dir = m.dir
	c.Env = append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor")

	ptmx, err := pty.StartWithSize(c, &pty.Winsize{Rows: uint16(h), Cols: uint16(w)})
	if err != nil {
		return err
	}
	m.cmd = c
	m.ptmx = ptmx
	m.started = true
	go m.readLoop()
	go m.responseLoop()
	return nil
}

// readLoop copia la salida del PTY al emulador y avisa al programa.
func (m *Model) readLoop() {
	buf := make([]byte, 32*1024)
	for {
		n, err := m.ptmx.Read(buf)
		if n > 0 {
			m.emu.Write(buf[:n])
			if m.prog != nil {
				m.prog.Send(RefreshMsg{ID: m.id})
			}
		}
		if err != nil {
			m.dead = true
			if m.prog != nil {
				m.prog.Send(ExitMsg{ID: m.id, Err: err})
			}
			return
		}
	}
}

// responseLoop drena las respuestas que el emulador genera ante las consultas
// del proceso (posición del cursor, atributos del dispositivo, etc.) y las
// devuelve al PTY. Es imprescindible: el emulador escribe esas respuestas en un
// io.Pipe síncrono, así que sin un lector concurrente su Write se bloquea y
// congela toda la terminal.
func (m *Model) responseLoop() {
	buf := make([]byte, 4096)
	for {
		n, err := m.emu.Read(buf)
		if n > 0 && m.ptmx != nil {
			_, _ = m.ptmx.Write(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

// SetSize ajusta el tamaño del panel (en celdas), redimensionando emulador y
// PTY en vivo.
func (m *Model) SetSize(w, h int) {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	m.mu.Lock()
	if w == m.width && h == m.height {
		m.mu.Unlock()
		return
	}
	m.width, m.height = w, h
	m.mu.Unlock()

	if m.emu != nil {
		m.emu.Resize(w, h)
	}
	if m.ptmx != nil {
		_ = pty.Setsize(m.ptmx, &pty.Winsize{Rows: uint16(h), Cols: uint16(w)})
	}
}

// SendKey traduce una pulsación de Bubble Tea a bytes y la envía al PTY.
func (m *Model) SendKey(k tea.KeyMsg) {
	if m.ptmx == nil {
		return
	}
	if b := encodeKey(k); len(b) > 0 {
		_, _ = m.ptmx.Write(b)
	}
}

// View renderiza la pantalla del emulador como string con estilos ANSI.
func (m *Model) View() string {
	if m.emu == nil {
		return ""
	}
	return m.emu.Render()
}

// Close termina el proceso y cierra el PTY.
func (m *Model) Close() {
	if m.ptmx != nil {
		_ = m.ptmx.Close()
	}
	if m.cmd != nil && m.cmd.Process != nil {
		_ = m.cmd.Process.Kill()
	}
}
