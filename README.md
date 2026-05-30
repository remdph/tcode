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

| Key             | Action                                          |
|-----------------|-------------------------------------------------|
| `Ctrl+B`        | Show / hide the side explorer (and focus it)    |
| `Alt+1`         | Focus the CLAUDE panel                          |
| `Alt+2`         | Focus the TERMINAL panel                        |
| `Alt+3`         | Focus the explorer (when visible)               |
| `Alt++`         | Open a new terminal tab (and focus it)          |
| `Alt+-`         | Close the current terminal tab (keeps one)      |
| `Alt+←` / `Alt+→` | Switch terminal tab (only when TERMINAL focused) |
| `Ctrl+Q`        | Quit                                            |

Showing the explorer with `Ctrl+B` moves focus to it; hiding it returns focus to
CLAUDE.

The TERMINAL panel supports multiple tabs, shown next to its label (the active
one is highlighted with the accent color). Each tab is an independent shell.

In the explorer (when focused): `↑/↓` or `j/k` to move, `→` to expand a folder,
`←` to collapse. Press `Enter` on a folder to toggle it, or on a **file** to
open it in a floating editor (almost full-screen) running `nano` — or `vi` if
nano is not available. Close the editor with its own command (`Ctrl+X` in nano,
`:q` in vi) to return to the UI.

When a terminal panel is focused, every key press is sent to its process
(including `Ctrl+C` to interrupt).

The explorer hides itself automatically when the window is narrower than 80
columns, and comes back when it is widened again (unless you hid it manually).

> **About `Super+B`:** most terminal emulators do not forward the *Super*
> (Win/Cmd) key to applications, so the reliable shortcut for the explorer is
> **`Ctrl+B`**. `Super+B` is recognized if your terminal happens to send it (the
> Kitty keyboard protocol), but it is not guaranteed.

## Theming

If an [Omarchy](https://omarchy.org) theme is active, code-tui picks up its
accent color and uses it for every focus/selection highlight (focused panel
headers, the status bar, the session picker and the selected file in the
explorer). It reads `accent` (and `selection_foreground` for contrast) from
`~/.config/omarchy/current/theme/colors.toml`. Without Omarchy it falls back to
a built-in blue.

## Layout

```
main.go             Entry point
internal/app/       Root model: layout, focus and shortcuts
internal/sidebar/   File explorer (tree)
internal/terminal/  PTY-backed terminal panel + vt emulator
internal/sessions/  Discovery of past Claude Code sessions
internal/picker/    Session selector UI
internal/theme/     Accent color (Omarchy integration)
```

## Status

This is an early milestone: layout + explorer + two virtual terminals + the
Claude session picker. There is **no** text editor yet (that comes later).
Possible next steps: a visible cursor in the terminal panels, mouse-wheel
scrolling, click to focus, a command palette and, later on, the editor.
