// Package terminal implements a panel backed by a pseudo-terminal (PTY) and a
// virtual terminal emulator. It is used to embed interactive processes
// (claude-cli, a shell) inside the TUI.
package terminal

import (
	"os"
	"os/exec"
	"sync"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
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

	// scrollOff is how many lines the view is scrolled up into the scrollback
	// buffer (0 = live, showing the current screen).
	scrollOff int

	// Cursor overlay state.
	emuMu         sync.Mutex // guards the cursor overlay against readLoop writes
	focused       bool       // draw a synthetic cursor when true
	cursorVisible bool       // the program's cursor visibility (DECTCEM)
}

// SetFocused controls whether this panel draws a synthetic cursor at the
// emulator's cursor position (the embedded screen has no real hardware cursor).
func (m *Model) SetFocused(f bool) { m.focused = f }

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
	m.cursorVisible = true
	m.emu.SetCallbacks(vt.Callbacks{
		CursorVisibility: func(visible bool) { m.cursorVisible = visible },
	})

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
			m.emuMu.Lock()
			m.emu.Write(buf[:n])
			m.emuMu.Unlock()
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

// AltScreen reports whether the running process is using the alternate screen
// (e.g. a full-screen editor or pager). Such programs do their own paging, so
// PgUp/PgDn should be forwarded to them rather than scrolling our scrollback.
func (m *Model) AltScreen() bool {
	return m.emu != nil && m.emu.IsAltScreen()
}

// maxScroll is the number of scrollback lines available above the screen.
func (m *Model) maxScroll() int {
	if m.emu == nil {
		return 0
	}
	return m.emu.ScrollbackLen()
}

// ScrollPage moves the view by one page through the scrollback buffer. dir > 0
// scrolls up (into the past), dir < 0 scrolls back down toward the live view.
func (m *Model) ScrollPage(dir int) {
	page := m.height - 1
	if page < 1 {
		page = 1
	}
	off := m.scrollOff + dir*page
	if hi := m.maxScroll(); off > hi {
		off = hi
	}
	if off < 0 {
		off = 0
	}
	m.scrollOff = off
}

// ScrollToBottom returns to the live view (the bottom of the history).
func (m *Model) ScrollToBottom() { m.scrollOff = 0 }

// Scrolled reports whether the panel is currently showing scrollback.
func (m *Model) Scrolled() bool { return m.scrollOff > 0 }

// ScrollOffset is how many lines the view is scrolled up from the live bottom.
func (m *Model) ScrollOffset() int { return m.scrollOff }

// SendKey translates a Bubble Tea key press to bytes and sends it to the PTY.
func (m *Model) SendKey(k tea.KeyPressMsg) {
	if m.ptmx == nil {
		return
	}
	if b := encodeKey(k); len(b) > 0 {
		_, _ = m.ptmx.Write(b)
	}
}

// SendNewline sends the Meta+Enter sequence (ESC + CR), which interactive
// prompts such as claude-cli treat as "insert a newline" rather than "submit".
func (m *Model) SendNewline() {
	if m.ptmx == nil {
		return
	}
	_, _ = m.ptmx.Write([]byte{0x1b, '\r'})
}

// SendPaste delivers pasted text to the process. The emulator wraps it in
// bracketed-paste markers if the program enabled that mode (DECSET 2004), so a
// multi-line paste is handled atomically rather than submitting on each newline.
func (m *Model) SendPaste(text string) {
	if m.emu == nil || m.dead {
		return // a dead process has no pipe reader; the write would block
	}
	m.emu.Paste(text)
}

// View renders the emulator screen as a string with ANSI styling. When the
// panel is focused and the program's cursor is visible, it draws a synthetic
// block cursor (reverse video) at the cursor position, since the embedded
// screen has no real hardware cursor.
func (m *Model) View() string {
	if m.emu == nil {
		return ""
	}

	m.emuMu.Lock()
	defer m.emuMu.Unlock()

	// When scrolled into the history, show that window instead of the live
	// screen (and no synthetic cursor, which lives at the bottom).
	if m.scrollOff > 0 {
		return m.scrolledView()
	}

	if !m.focused {
		return m.emu.Render()
	}

	if !m.cursorVisible {
		return m.emu.Render()
	}
	pos := m.emu.CursorPosition()
	cell := m.emu.CellAt(pos.X, pos.Y)
	if cell == nil {
		return m.emu.Render()
	}

	orig := cell.Clone()
	cur := cell.Clone()
	if cur.Content == "" {
		cur.Content = " "
		cur.Width = 1
	}
	cur.Style.Attrs |= uv.AttrReverse

	m.emu.SetCell(pos.X, pos.Y, cur)
	out := m.emu.Render()
	m.emu.SetCell(pos.X, pos.Y, orig)
	return out
}

// scrolledView renders a window into the scrollback buffer followed by the
// current screen, offset upward by m.scrollOff lines. The caller must hold
// m.emuMu. The history is the scrollback lines (oldest first) followed by the
// h on-screen rows; the visible window is bottom-aligned and shifted up by the
// scroll offset.
func (m *Model) scrolledView() string {
	w, h := m.emu.Width(), m.emu.Height()
	sbLen := m.emu.ScrollbackLen()
	total := sbLen + h

	top := total - h - m.scrollOff
	if top < 0 {
		top = 0
	}

	lines := make(uv.Lines, 0, h)
	for row := 0; row < h; row++ {
		idx := top + row
		if idx < 0 || idx >= total {
			lines = append(lines, uv.Line{})
			continue
		}
		line := make(uv.Line, w)
		for x := 0; x < w; x++ {
			var cell *uv.Cell
			if idx < sbLen {
				cell = m.emu.ScrollbackCellAt(x, idx)
			} else {
				cell = m.emu.CellAt(x, idx-sbLen)
			}
			if cell != nil {
				line[x] = *cell
			} else {
				line[x] = uv.EmptyCell
			}
		}
		lines = append(lines, line)
	}
	return lines.Render()
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
