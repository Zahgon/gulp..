package cli

import (
	"fmt"
	"io"
	"sync"
	"time"
)

// Level selects how much the CLI reports, matching gulp-cli's -L counter where
// -L is the least verbose and -LLLL the most.
type Level int

// The four verbosity levels. LevelInfo is the default, and is what -LLL
// selects explicitly.
const (
	LevelSilent Level = iota
	LevelError
	LevelWarn
	LevelInfo
	LevelDebug
)

// DefaultLevel is the verbosity used when -L is not given.
const DefaultLevel = LevelInfo

// Logger writes timestamped CLI output.
//
// Errors go to stderr and everything else to stdout, so that piping a build's
// output does not swallow its failures. A Logger is safe for concurrent use,
// which matters because tasks run in parallel and each emits start and stop
// events from its own goroutine.
type Logger struct {
	mu      sync.Mutex
	out     io.Writer
	err     io.Writer
	palette Palette
	level   Level
	now     func() time.Time
}

// NewLogger builds a Logger writing to out and errOut.
func NewLogger(out, errOut io.Writer, palette Palette, level Level) *Logger {
	return &Logger{out: out, err: errOut, palette: palette, level: level, now: time.Now}
}

// Palette returns the colour palette in use, so callers can build coloured
// messages that respect the same --color decision.
func (l *Logger) Palette() Palette { return l.palette }

// Level returns the configured verbosity.
func (l *Logger) Level() Level { return l.level }

// Enabled reports whether a message at level would be written.
func (l *Logger) Enabled(level Level) bool { return l.level >= level }

// Error writes a message to stderr.
func (l *Logger) Error(format string, args ...any) {
	l.write(LevelError, l.err, format, args...)
}

// Warn writes a warning to stdout.
func (l *Logger) Warn(format string, args ...any) {
	l.write(LevelWarn, l.out, format, args...)
}

// Info writes a normal progress message to stdout.
func (l *Logger) Info(format string, args ...any) {
	l.write(LevelInfo, l.out, format, args...)
}

// Debug writes a verbose message to stdout.
func (l *Logger) Debug(format string, args ...any) {
	l.write(LevelDebug, l.out, format, args...)
}

// Print writes to stdout without a timestamp or a level check.
//
// The task tree uses this: gulp-cli deliberately bypasses its logger there
// because a timestamp on every branch of the tree would be noise.
func (l *Logger) Print(s string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	fmt.Fprintln(l.out, s)
}

func (l *Logger) write(level Level, w io.Writer, format string, args ...any) {
	if !l.Enabled(level) {
		return
	}
	msg := format
	if len(args) > 0 {
		msg = fmt.Sprintf(format, args...)
	}
	if msg == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	fmt.Fprintln(w, l.timestamp()+" "+msg)
}

// timestamp renders the bracketed clock gulp prefixes to every log line. The
// 24-hour form matches toLocaleTimeString('en', {hour12: false}).
func (l *Logger) timestamp() string {
	return "[" + l.palette.Magenta(l.now().Format("15:04:05")) + "]"
}
