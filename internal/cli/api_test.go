package cli

import (
	"os"
	"strings"
	"testing"
)

func TestPaletteYellow(t *testing.T) {
	forced := NewPalette(true, false)
	if got := forced.Yellow("careful"); got != "\x1b[33mcareful\x1b[39m" {
		t.Errorf("Yellow = %q", got)
	}

	off := NewPalette(false, true)
	if got := off.Yellow("careful"); got != "careful" {
		t.Errorf("disabled Yellow = %q, want the bare string", got)
	}
}

// TestColorSupportedHonoursTheEnvironment pins the precedence chalk uses:
// FORCE_COLOR wins outright, then NO_COLOR, then a dumb terminal. Everything
// below that depends on whether stdout is a terminal, which it is not under
// `go test`, so the remaining cases are covered by the TTY check separately.
func TestColorSupportedHonoursTheEnvironment(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{"force wins", map[string]string{"FORCE_COLOR": "1"}, true},
		{"force wins over no-color", map[string]string{"FORCE_COLOR": "1", "NO_COLOR": "1"}, true},
		{"no-color disables", map[string]string{"NO_COLOR": "1"}, false},
		{"dumb terminal disables", map[string]string{"TERM": "dumb"}, false},
		{"not a terminal under go test", map[string]string{"TERM": "xterm-256color"}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, key := range []string{"FORCE_COLOR", "NO_COLOR", "TERM"} {
				t.Setenv(key, "")
				os.Unsetenv(key)
			}
			for key, value := range tc.env {
				t.Setenv(key, value)
			}
			if got := colorSupported(); got != tc.want {
				t.Errorf("colorSupported() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIsTerminal(t *testing.T) {
	// A regular file is never a terminal, and neither is a pipe. Both are
	// what a build actually writes to, so both must report false or every
	// log line would carry escape codes into the file.
	file, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	defer file.Close()

	if isTerminal(file) {
		t.Error("isTerminal(regular file) = true")
	}

	read, write, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer read.Close()
	defer write.Close()

	if isTerminal(write) {
		t.Error("isTerminal(pipe) = true")
	}
}

func TestLoggerAccessors(t *testing.T) {
	palette := NewPalette(true, false)
	log := NewLogger(&strings.Builder{}, &strings.Builder{}, palette, LevelWarn)

	if !log.Palette().Enabled() {
		t.Error("Palette() lost the colour setting")
	}
	if log.Level() != LevelWarn {
		t.Errorf("Level() = %v, want %v", log.Level(), LevelWarn)
	}
}

func TestLoggerWarn(t *testing.T) {
	var out, errOut strings.Builder
	log := NewLogger(&out, &errOut, Palette{}, LevelWarn)

	log.Warn("disk is %d%% full", 91)

	if !strings.Contains(out.String(), "disk is 91% full") {
		t.Errorf("stdout = %q, want the warning", out.String())
	}
	if errOut.String() != "" {
		t.Errorf("stderr = %q, want warnings on stdout", errOut.String())
	}
}

func TestLoggerWarnIsSilencedBelowItsLevel(t *testing.T) {
	var out, errOut strings.Builder
	log := NewLogger(&out, &errOut, Palette{}, LevelError)

	log.Warn("ignored")

	if out.String() != "" || errOut.String() != "" {
		t.Errorf("warn leaked at LevelError: stdout=%q stderr=%q", out.String(), errOut.String())
	}
}
