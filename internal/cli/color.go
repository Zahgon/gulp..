package cli

import (
	"os"
	"strings"
)

// Palette reproduces the subset of chalk that gulp-cli uses.
//
// A disabled Palette returns its input untouched, which is how --no-color and
// piped output are handled: the message catalogue is written once and the
// decision about escape codes lives here.
type Palette struct {
	enabled bool
}

// NewPalette builds a Palette, auto-detecting colour support unless force or
// disable overrides it. force wins, matching gulp-cli, where --color is
// documented as forcing colour "even when no color support is detected".
func NewPalette(force, disable bool) Palette {
	switch {
	case force:
		return Palette{enabled: true}
	case disable:
		return Palette{enabled: false}
	default:
		return Palette{enabled: colorSupported()}
	}
}

// Enabled reports whether the palette emits escape codes.
func (p Palette) Enabled() bool { return p.enabled }

// wrap closes with the style's own reset code rather than a blanket "\x1b[0m",
// because that is what chalk emits: 39 restores the default foreground and 22
// cancels bold, so a nested style survives the inner one closing.
func (p Palette) wrap(code, closeCode, s string) string {
	if !p.enabled || s == "" {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[" + closeCode + "m"
}

func (p Palette) color(code, s string) string { return p.wrap(code, "39", s) }

// Bold renders s in bold.
func (p Palette) Bold(s string) string { return p.wrap("1", "22", s) }

// Red renders s in red.
func (p Palette) Red(s string) string { return p.color("31", s) }

// Blue renders s in blue.
func (p Palette) Blue(s string) string { return p.color("34", s) }

// Magenta renders s in magenta.
func (p Palette) Magenta(s string) string { return p.color("35", s) }

// Cyan renders s in cyan.
func (p Palette) Cyan(s string) string { return p.color("36", s) }

// White renders s in white.
func (p Palette) White(s string) string { return p.color("37", s) }

// Yellow renders s in yellow.
func (p Palette) Yellow(s string) string { return p.color("33", s) }

// Gray renders s in bright black, chalk's "gray".
func (p Palette) Gray(s string) string { return p.color("90", s) }

// colorSupported mirrors the checks chalk performs before enabling colour.
//
// NO_COLOR is honoured per the no-color.org convention, FORCE_COLOR overrides
// everything, a dumb terminal is treated as incapable, and otherwise colour is
// used only when stdout is a terminal rather than a pipe or a file.
func colorSupported() bool {
	if v, ok := os.LookupEnv("FORCE_COLOR"); ok {
		return v != "0" && !strings.EqualFold(v, "false")
	}
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return false
	}
	if term := os.Getenv("TERM"); term == "dumb" {
		return false
	}
	return isTerminal(os.Stdout)
}

func isTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
