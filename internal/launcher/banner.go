package launcher

import (
	"strings"

	"code-tui/internal/theme"

	"github.com/charmbracelet/lipgloss"
)

// Block-style glyphs for the T-CODE logo. Each is a list of rows; widths need
// not match across rows because JoinHorizontal pads each block to its own width.
var (
	glyphT = []string{
		"████████╗",
		"╚══██╔══╝",
		"   ██║   ",
		"   ██║   ",
		"   ██║   ",
		"   ╚═╝   ",
	}
	glyphDash = []string{
		"      ",
		"      ",
		"█████╗",
		"╚════╝",
		"      ",
		"      ",
	}
	glyphC = []string{
		" ██████╗",
		"██╔════╝",
		"██║     ",
		"██║     ",
		"╚██████╗",
		" ╚═════╝",
	}
	glyphO = []string{
		" ██████╗ ",
		"██╔═══██╗",
		"██║   ██║",
		"██║   ██║",
		"╚██████╔╝",
		" ╚═════╝ ",
	}
	glyphD = []string{
		"██████╗ ",
		"██╔══██╗",
		"██║  ██║",
		"██║  ██║",
		"██████╔╝",
		"╚═════╝ ",
	}
	glyphE = []string{
		"███████╗",
		"██╔════╝",
		"█████╗  ",
		"██╔══╝  ",
		"███████╗",
		"╚══════╝",
	}
)

// banner renders the "T-CODE" logo in the accent color, with a small tagline.
func banner() string {
	glyphs := [][]string{glyphT, glyphDash, glyphC, glyphO, glyphD, glyphE}
	sep := strings.TrimSuffix(strings.Repeat(" \n", len(glyphT)), "\n") // 1-col gap

	blocks := make([]string, 0, len(glyphs)*2)
	for i, g := range glyphs {
		if i > 0 {
			blocks = append(blocks, sep)
		}
		blocks = append(blocks, strings.Join(g, "\n"))
	}
	art := lipgloss.JoinHorizontal(lipgloss.Top, blocks...)

	logo := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true).Render(art)
	tagline := lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render("a VSCode-like terminal workspace")
	return lipgloss.JoinVertical(lipgloss.Center, logo, "", tagline)
}

// bannerWidth is the rendered width of the logo, used to decide whether it fits.
func bannerWidth() int { return lipgloss.Width(banner()) }
