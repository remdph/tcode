# code-tui

Un entorno tipo **VSCode para la terminal** (TUI), escrito en Go con
[Bubble Tea](https://github.com/charmbracelet/bubbletea).

La primera versión monta el esqueleto de la interfaz:

```
┌──────────────┬───────────────────────────────────┐
│              │  CLAUDE                           │
│  EXPLORADOR  │  (claude-cli en una terminal      │
│  (árbol de   │   virtual / PTY)                  │
│   archivos)  ├───────────────────────────────────┤
│              │  TERMINAL                         │
│              │  (tu shell en una terminal        │
│              │   virtual / PTY)                  │
└──────────────┴───────────────────────────────────┘
 CLAUDE   Ctrl+B explorador · Alt+1/2/3 foco · Ctrl+Q salir
```

- **Panel izquierdo** — explorador de archivos navegable del directorio actual.
- **Panel derecho (arriba)** — `claude-cli` corriendo en una terminal virtual.
- **Panel derecho (abajo)** — una shell interactiva en otra terminal virtual.

Los dos paneles de la derecha son **terminales virtuales reales**: cada uno
ejecuta su proceso en un PTY y se renderiza mediante un emulador de terminal
([`charmbracelet/x/vt`](https://github.com/charmbracelet/x)). Ambos arrancan en
el mismo directorio de trabajo.

### Sesiones de Claude

claude-cli se lanza siempre con `claude --dangerously-skip-permissions`. Al
abrir el panel CLAUDE, code-tui busca **sesiones pasadas** de Claude Code en ese
directorio (en `~/.claude/projects/<ruta>`):

- Si **no hay** ninguna, arranca directamente una sesión nueva.
- Si **las hay**, muestra un selector en el panel CLAUDE. La **primera opción es
  siempre crear una sesión nueva**; debajo se listan las sesiones existentes
  (primer mensaje + fecha), de la más reciente a la más antigua. Reanudar una
  sesión usa `claude --dangerously-skip-permissions --resume <id>`.

  Navega con `↑/↓` (o `j/k`) y confirma con `Enter`.

## Uso

```bash
go run .            # abre el directorio actual
go run . /ruta/dir  # abre otro directorio
```

O compilando:

```bash
go build -o code-tui .
./code-tui
```

> Requiere tener `claude` (Claude Code CLI) en el `PATH` para el panel CLAUDE.

## Atajos

| Tecla            | Acción                                            |
|------------------|---------------------------------------------------|
| `Ctrl+B`         | Mostrar / ocultar el explorador lateral           |
| `Alt+1`          | Enfocar el explorador                             |
| `Alt+2`          | Enfocar el panel CLAUDE                           |
| `Alt+3`          | Enfocar el panel TERMINAL                         |
| `Ctrl+Q`         | Salir                                             |

En el explorador (cuando tiene el foco): `↑/↓` o `j/k` para moverse,
`Enter`/`→` para expandir o plegar carpetas, `←` para plegar.

Cuando un panel de terminal tiene el foco, todas las pulsaciones se envían a su
proceso (incluido `Ctrl+C` para interrumpir).

> **Nota sobre `Super+B`:** la mayoría de emuladores de terminal no reenvían la
> tecla *Super* (Win/Cmd) a las aplicaciones, así que el atajo fiable para el
> explorador es **`Ctrl+B`**. `Super+B` se reconoce si tu terminal llega a
> enviarlo (protocolo de teclado Kitty), pero no está garantizado.

## Estructura

```
main.go                      Punto de entrada
internal/app/                Modelo raíz: layout, foco y atajos
internal/sidebar/            Explorador de archivos (árbol)
internal/terminal/           Panel de terminal respaldado por PTY + emulador vt
```

## Estado

Esto es el primer hito: layout + explorador + dos terminales virtuales. Todavía
**no** hay editor de texto (vendrá después). Próximos pasos posibles: cursor
visible en los paneles de terminal, scroll con rueda del ratón, click para
enfocar, paleta de comandos y, más adelante, el editor.
