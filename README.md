# code-tui (`tcode`)

**A VS Code–style workspace that lives entirely in your terminal.**

`tcode` recreates the comfort of an editor like VS Code as a pure console app
(TUI): a file explorer on one side, **Claude Code** running in one pane, and one
or more **virtual terminals** in another — all arranged in a tidy, keyboard-driven
layout, with a Git panel and a project launcher. It is written in Go with
[Bubble Tea v2](https://github.com/charmbracelet/bubbletea) and rendered through a
real terminal emulator, so the panes are genuine PTYs, not fake text boxes.

Think of it as a lightweight "IDE shell" whose primary tenant is **Claude Code**:
you browse and open files on the left, talk to Claude on the right, and drop into
a shell whenever you need one — without ever leaving the terminal or juggling tmux
splits by hand.

Once installed it is invoked as **`tcode`**. Current version: **v0.1.0**.

## What you get

- **Claude Code, front and center** — `claude` runs in its own virtual terminal,
  with a built-in picker to start a new session or **resume a past one** for the
  current project.
- **Virtual terminals in a comfortable layout** — real shells in PTYs, with
  multiple tabs, laid out below Claude so you never lose your place.
- **A navigable file explorer** — a collapsible tree of the project, with a
  floating `nano`/`vi` editor for quick edits.
- **A Git panel** — pending changes and branch history at a glance.
- **A project launcher** — pick from recently opened directories (or the current
  path) on startup, under a `T-CODE` banner.
- **Adapts to your theme** — picks up your [Omarchy](https://omarchy.org) accent
  color automatically.

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

When claude-cli exits (e.g. you quit it with `Ctrl+C`), the CLAUDE panel returns
to this session menu so you can start or resume another session.

## Install

The installed binary is named **`tcode`**. To build and install it to
`~/.local/bin` (make sure that is on your `PATH`):

```bash
go build -trimpath -ldflags="-s -w" -o ~/.local/bin/tcode .
```

Then invoke it from anywhere:

```bash
tcode            # shows the project launcher (CURRENT PATH + recents)
tcode /some/dir  # opens that directory directly
tcode ~/repos/x  # ~ is expanded
tcode --version  # prints the version (0.1.0)
```

### Project launcher

Running `tcode` with **no argument** first shows a launcher (the same selector
used for Claude sessions) to choose which project to open:

- The first option, **CURRENT PATH**, opens the directory you ran `tcode` from.
- Below it are the **recently opened** directories — each shown by its folder
  name with the full path on a second line.

Every directory you open is recorded in `~/.config/code-tui/recents.json`.
Passing a directory argument skips the launcher and opens it directly (but still
records it).

During development you can also run it without installing:

```bash
go run .            # opens the current directory
go run . /some/dir  # opens another directory
```

> The CLAUDE panel requires `claude` (the Claude Code CLI) on your `PATH`.

## Shortcuts

| Key             | Action                                          |
|-----------------|-------------------------------------------------|
| `Ctrl+P`        | Open the fuzzy file finder (quick open)         |
| `Ctrl+B`        | Show / hide the side explorer (and focus it)    |
| `Ctrl+G`        | Show / hide the Git panel (and focus it)        |
| `Ctrl+T`        | Show / hide the TERMINAL section (keeps shells) |
| `Alt+1`         | Focus the CLAUDE panel                          |
| `Alt+2`         | Focus the TERMINAL panel                        |
| `Alt+3`         | Focus the explorer (when visible)               |
| `Alt++`         | Open a new terminal tab (and focus it)          |
| `Alt+-`         | Close the current terminal tab (keeps one)      |
| `Alt+←` / `Alt+→` | Cycle terminal tabs (when TERMINAL focused)    |
| `Alt+Shift+1…0` | Jump to terminal tab 1…10 (when TERMINAL focused) |
| `Shift+Enter`   | Insert a newline in the CLAUDE prompt (multiline) |
| `Ctrl+J`        | Insert a newline in the CLAUDE prompt (multiline) |
| `PgUp` / `PgDn` | Scroll the CLAUDE / terminal history (scrollback) |
| `.`             | Toggle hidden dotfiles (when the explorer is focused) |
| `Ctrl+Q`        | Quit                                            |

Revealing the explorer (`Ctrl+B`), the Git panel (`Ctrl+G`) or the TERMINAL
section (`Ctrl+T`) moves focus **to** that section; hiding it returns focus to
the CLAUDE panel, which is never hidden.

The TERMINAL panel supports multiple tabs, shown next to its label (the active
one is highlighted with the accent color). Each tab is an independent shell.
Cycle through them with `Alt+←/→`, or jump straight to one with `Alt+Shift+<n>`
(`Alt+Shift+1` for the first tab, and so on). `Ctrl+T` hides or shows the whole
TERMINAL section (giving CLAUDE the full height); the shells keep running while
hidden.

### Multiline input to Claude

Plain `Enter` submits your message to Claude. To insert a **newline** instead
(for a multi-line prompt), press `Shift+Enter`, `Ctrl+J`, or `Alt`/`Option`+`Enter`.

`Shift+Enter` works terminal-agnostically: tcode is built on **Bubble Tea v2**,
which negotiates the [kitty keyboard protocol](https://sw.kovidgoyal.net/kitty/keyboard-protocol/)
with the host terminal on startup, so a modified `Enter` becomes distinguishable
from a plain one. On terminals that don't support the protocol, `Shift+Enter`
falls back to behaving like `Enter`; use `Ctrl+J` there.

The **Git panel** (`Ctrl+G`) opens on the right and has two tabs, switched with
`Tab` (or `←/→`):

- **CHANGES** (default) — the pending working-tree changes (`git status`), with a
  colored status marker per file.
- **HISTORY** — the commits on the current branch, each showing its short hash,
  any tags (`⚑`), the subject, and the author and relative date.

Navigate with `↑/↓`, `r` refreshes, and `+/-` resize the panel (its width is
remembered per project, like the explorer).

In the explorer (when focused): `↑/↓` or `j/k` to move, `→` to expand a folder,
`←` to collapse, `.` to toggle hidden dotfiles, and `+` / `-` to resize the
explorer (its width is remembered per project under `~/.config/code-tui/`).
**Hidden files (names starting with a dot) are shown by default**; press `.` to
hide or reveal them. Press `Enter` on a folder to toggle it, or on a **file** to
open it in a floating editor (almost full-screen) running `nano` — or `vi` if
nano is not available. Close the editor with its own command (`Ctrl+X` in nano,
`:q` in vi) to return to the UI.

### Quick open (fuzzy file finder)

Press `Ctrl+P` (or `Super+P` if your terminal forwards it) to open a **floating
file finder**, just like VS Code's *Quick Open*. Start typing and the project's
files are fuzzy-matched as you go, ranked best-first (matches in the file name
beat matches that only span the directory path). Move the selection with `↑/↓`
(or `Ctrl+N`/`Ctrl+P`), press `Enter` to open the highlighted file in the same
floating `nano`/`vi` editor the explorer uses, or `Esc` to dismiss. The index is
built when you open the finder and skips noise directories (`.git`,
`node_modules`, …).

### Scrolling the history

The CLAUDE and TERMINAL panels keep a scrollback buffer, just like a normal
terminal. With either panel focused, press `PgUp` / `PgDn` to scroll back through
its history (Claude streams its conversation into this buffer, so this is how you
review earlier messages). The status bar shows a `SCROLLBACK` indicator while you
are scrolled up; pressing any other key — or `PgDn` to the bottom — snaps back to
the live view. If a full-screen program is running in the panel (a pager or
editor using the alternate screen), `PgUp` / `PgDn` are sent to it instead so it
can do its own paging.

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

## Source layout

```
main.go             Entry point
internal/app/       Root model: layout, focus and shortcuts
internal/sidebar/   File explorer (tree)
internal/terminal/  PTY-backed terminal panel + vt emulator
internal/sessions/  Discovery of past Claude Code sessions
internal/picker/    Selector UI (sessions and the launcher)
internal/quickopen/ Fuzzy file finder (Ctrl+P quick open)
internal/launcher/  Startup project launcher (CURRENT PATH + recents)
internal/recents/   Recently opened directories
internal/gitpanel/  Git panel (CHANGES / HISTORY)
internal/theme/     Accent color (Omarchy integration)
internal/config/    Per-project settings (panel widths)
```

## Status

**v0.1.0.** Working: the project launcher (CURRENT PATH + recents), file
explorer (with dotfiles and a `.` toggle), a `Ctrl+P` fuzzy file finder, claude-cli
with a session picker, scrollable panel history (`PgUp`/`PgDn`), multiple terminal
tabs, a floating nano/vi editor, a Git panel (CHANGES / HISTORY), Omarchy accent
theming, visible cursors and per-project persisted panel widths.

Possible next steps: mouse-wheel scrolling and click-to-focus, a command
palette, resizing the CLAUDE/TERMINAL split, and Git actions (stage/commit).
