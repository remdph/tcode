// Package terminal implements a panel backed by a pseudo-terminal (PTY) and a
// virtual terminal emulator. It is used to embed interactive processes
// (claude-cli, a shell) inside the TUI.
package terminal

import (
	"os"
	"os/exec"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/vt"
	"github.com/creack/pty"
)

// RefreshMsg is emitted when the PTY produced new output and the panel must be
// repainted.
type RefreshMsg struct{ ID int }

// ExitMsg is emitted when the PTY process exits.
type ExitMsg struct {
	ID  int
	Err error
}

// Model is an embedded terminal panel.
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

// New creates a terminal panel that will run args[0] with args[1:] in dir.
// The process is not launched until Start is called.
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

// Name returns the panel label.
func (m *Model) Name() string { return m.name }

// SetArgs sets the command to run. It only has effect if called before Start.
func (m *Model) SetArgs(args []string) { m.args = args }

// Started reports whether the process has already been launched.
func (m *Model) Started() bool { return m.started }

// Dead reports whether the underlying process has exited.
func (m *Model) Dead() bool { return m.dead }

// Start launches the process in a PTY at the current size and starts the read
// loop. prog is used to notify the Bubble Tea program.
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

// readLoop copies PTY output into the emulator and notifies the program.
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

// responseLoop drains the replies the emulator generates for the process's
// terminal queries (cursor position, device attributes, etc.) and writes them
// back to the PTY. This is essential: the emulator writes those replies to a
// synchronous io.Pipe, so without a concurrent reader its Write blocks and
// freezes the whole terminal.
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

// SetSize adjusts the panel size (in cells), resizing the emulator and PTY
// live.
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

// SendKey translates a Bubble Tea key press to bytes and sends it to the PTY.
func (m *Model) SendKey(k tea.KeyMsg) {
	if m.ptmx == nil {
		return
	}
	if b := encodeKey(k); len(b) > 0 {
		_, _ = m.ptmx.Write(b)
	}
}

// View renders the emulator screen as a string with ANSI styling.
func (m *Model) View() string {
	if m.emu == nil {
		return ""
	}
	return m.emu.Render()
}

// Close terminates the process and closes the PTY.
func (m *Model) Close() {
	if m.ptmx != nil {
		_ = m.ptmx.Close()
	}
	if m.cmd != nil && m.cmd.Process != nil {
		_ = m.cmd.Process.Kill()
	}
}
