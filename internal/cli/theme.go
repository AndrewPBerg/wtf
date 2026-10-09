package cli

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/term"
	"github.com/muesli/termenv"
	"github.com/pelletier/go-toml/v2"
)

type palette struct{ Accent, Foreground, Muted, Good, Warning, Bad string }

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// Each invocation reads Omarchy's active palette, so theme changes need no restart.
func activePalette() palette {
	p := palette{"4", "7", "8", "2", "3", "1"}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	state := os.Getenv("XDG_STATE_HOME")
	if state == "" {
		state = filepath.Join(home, ".local", "state")
	}
	config := os.Getenv("XDG_CONFIG_HOME")
	if config == "" {
		config = filepath.Join(home, ".config")
	}
	for _, path := range []string{
		filepath.Join(state, "omarchy", "current", "theme", "colors.toml"),
		filepath.Join(config, "omarchy", "current", "theme", "colors.toml"),
	} {
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var colors map[string]string
		if err := toml.Unmarshal(b, &colors); err != nil {
			continue
		}
		for key, target := range map[string]*string{"accent": &p.Accent, "foreground": &p.Foreground, "color8": &p.Muted, "color2": &p.Good, "color3": &p.Warning, "color1": &p.Bad} {
			if value := colors[key]; hexColor.MatchString(value) {
				*target = value
			}
		}
		return p
	}
	return p
}

type presentation struct {
	Renderer                               *lipgloss.Renderer
	Title, Text, Muted, Good, Warning, Bad lipgloss.Style
	Width                                  int
}

func humanPresentation(out io.Writer) presentation {
	r := lipgloss.NewRenderer(out)
	width := 100
	color := false
	if file, ok := out.(interface{ Fd() uintptr }); ok && term.IsTerminal(file.Fd()) {
		color = true
		if w, _, err := term.GetSize(file.Fd()); err == nil && w > 0 {
			width = w
		}
	}
	if os.Getenv("FORCE_COLOR") != "" && os.Getenv("FORCE_COLOR") != "0" {
		color = true
	}
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		color = false
	}
	if color {
		r.SetColorProfile(termenv.TrueColor)
	} else {
		r.SetColorProfile(termenv.Ascii)
	}
	p := activePalette()
	style := func(value string) lipgloss.Style { return r.NewStyle().Foreground(lipgloss.Color(value)) }
	return presentation{r, style(p.Accent).Bold(true), style(p.Foreground), style(p.Muted), style(p.Good), style(p.Warning), style(p.Bad), width}
}

// Keep control characters in recorded data out of terminal presentation.
func plain(value string) string {
	return strings.Map(func(r rune) rune {
		if r < 32 || (r >= 127 && r <= 159) {
			return ' '
		}
		return r
	}, value)
}
