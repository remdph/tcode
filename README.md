# code-tui

A **VSCode-like environment for the terminal** (TUI), written in Go with
[Bubble Tea](https://github.com/charmbracelet/bubbletea).

The interface skeleton:

```
┌───────────────────────────────────────────────────┐
│  CLAUDE                                           │
│  (claude-cli in a virtual terminal / PTY)         │
├───────────────────────────────────────────────────┤
│  TERMINAL                                         │
│  (your shell in a virtual terminal / PTY)         │
└───────────────────────────────────────────────────┘
 CLAUDE   Ctrl+B explorer · Alt+1 Claude · Alt+2 terminal · Ctrl+Q quit
```

The file explorer is hidden by default; press `Ctrl+B` to reveal it on the left:

```
┌──────────────┬────────────────────────────────────┐
│              │  CLAUDE                            │
│  EXPLORER    │  (claude-cli)                      │
│  (file tree) ├────────────────────────────────────┤
│              │  TERMINAL  (your shell)            │
└──────────────┴────────────────────────────────────┘
```

- **Left panel** — a navigable file explorer of the current directory.
- **Right panel (top)** — `claude-cli` running in a virtual terminal.
- **Right panel (bottom)** — an interactive shell in another virtual terminal.

The two right-hand panels are **real virtual terminals**: each runs its process
in a PTY and is rendered through a terminal emulator
([`charmbracelet/x/vt`](https://github.com/charmbracelet/x)). Both start in the
same working directory.

There are no outer borders: content reaches the window edges and only the
internal seams are drawn (a vertical `│` between the explorer and the right
column, and the `TERMINAL` header between the two right panels).

### Claude sessions

claude-cli always launches with `claude --dangerously-skip-permissions`. When
the CLAUDE panel opens, code-tui looks for **past Claude Code sessions** for that
directory (under `~/.claude/projects/<path>`):

- If there are **none**, it starts a new session directly.
- If there **are**, it shows a selector in the CLAUDE panel. The **first option
  is always to create a new session**; below it the existing sessions are listed
  (first message + date), from most to least recent. Resuming a session uses
  `claude --dangerously-skip-permissions --resume <id>`.

  Navigate with `↑/↓` (or `j/k`) and confirm with `Enter`.

## Usage

```bash
go run .            # opens the current directory
go run . /some/dir  # opens another directory
```

Or build it:

```bash
go build -o code-tui .
./code-tui
```

> The CLAUDE panel requires `claude` (the Claude Code CLI) on your `PATH`.

## Shortcuts

| Key       | Action                                  |
|-----------|-----------------------------------------|
| `Ctrl+B`  | Show / hide the side explorer           |
| `Alt+1`   | Focus the CLAUDE panel                  |
| `Alt+2`   | Focus the TERMINAL panel                |
| `Alt+3`   | Focus the explorer (when visible)       |
| `Ctrl+Q`  | Quit                                    |

In the explorer (when focused): `↑/↓` or `j/k` to move, `Enter`/`→` to expand or
collapse folders, `←` to collapse.

When a terminal panel is focused, every key press is sent to its process
(including `Ctrl+C` to interrupt).

The explorer hides itself automatically when the window is narrower than 80
columns, and comes back when it is widened again (unless you hid it manually).

> **About `Super+B`:** most terminal emulators do not forward the *Super*
> (Win/Cmd) key to applications, so the reliable shortcut for the explorer is
> **`Ctrl+B`**. `Super+B` is recognized if your terminal happens to send it (the
> Kitty keyboard protocol), but it is not guaranteed.

## Layout

```
main.go             Entry point
internal/app/       Root model: layout, focus and shortcuts
internal/sidebar/   File explorer (tree)
internal/terminal/  PTY-backed terminal panel + vt emulator
internal/sessions/  Discovery of past Claude Code sessions
internal/picker/    Session selector UI
```

## Status

This is an early milestone: layout + explorer + two virtual terminals + the
Claude session picker. There is **no** text editor yet (that comes later).
Possible next steps: a visible cursor in the terminal panels, mouse-wheel
scrolling, click to focus, a command palette and, later on, the editor.
