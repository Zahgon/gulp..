package cli

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Box-drawing characters used by the task tree.
const (
	boxUpAndRight        = "└"
	boxVerticalAndRight  = "├"
	boxHorizontal        = "─"
	boxDownAndHorizontal = "┬"
	boxVertical          = "│"
)

// Messages renders the CLI's user-facing strings.
//
// The wording is copied verbatim from gulp-cli so that scripts and editor
// integrations which scrape gulp's output keep working against this port; only
// the colouring is indirected, through the palette.
type Messages struct {
	Palette Palette
}

// Description heads the task listing.
func (m Messages) Description(path string) string {
	return "Tasks for " + m.Palette.Magenta(Tildify(path))
}

// Gulpfile announces which gulpfile is being run.
func (m Messages) Gulpfile(path string) string {
	return "Using gulpfile " + m.Palette.Magenta(Tildify(path))
}

// TaskStart announces that a task has begun.
func (m Messages) TaskStart(task string) string {
	return "Starting '" + m.Palette.Cyan(task) + "'..."
}

// TaskStop announces that a task finished successfully.
func (m Messages) TaskStop(task string, d time.Duration) string {
	return "Finished '" + m.Palette.Cyan(task) + "' after " + m.Palette.Magenta(FormatDuration(d))
}

// TaskFailure announces that a task errored.
func (m Messages) TaskFailure(task string, d time.Duration) string {
	return "'" + m.Palette.Cyan(task) + "' " + m.Palette.Red("errored after") + " " +
		m.Palette.Magenta(FormatDuration(d))
}

// TaskError reports the error a task produced.
func (m Messages) TaskError(err error) string {
	if err == nil {
		return ""
	}
	return m.Palette.Red(err.Error())
}

// TaskMissing reports an unknown task name, suggesting close matches.
func (m Messages) TaskMissing(task string, similar []string) string {
	msg := "Task never defined: " + task
	if len(similar) > 0 {
		msg += " - did you mean? " + strings.Join(similar, ", ")
	}
	return m.Palette.Red(msg) + "\nTo list available tasks, try running: gulp --tasks"
}

// MissingGulpfile reports that no gulpfile could be located.
func (m Messages) MissingGulpfile() string {
	return m.Palette.Red("No gulpfile found")
}

// CwdChanged reports a --cwd or --gulpfile induced directory change.
func (m Messages) CwdChanged(cwd string) string {
	return "Working directory changed to " + m.Palette.Magenta(Tildify(cwd))
}

// Usage is the first line of --help.
func (m Messages) Usage() string {
	return "\n" + m.Palette.Bold("Usage:") + " gulp " + m.Palette.Blue("[options]") + " tasks"
}

// TaskName renders a task label inside the tree.
func (m Messages) TaskName(name string) string { return m.Palette.Cyan(name) }

// TaskDescription renders a task's description.
func (m Messages) TaskDescription(desc string) string { return m.Palette.White(desc) }

// TaskFlag renders a task flag inside the tree.
func (m Messages) TaskFlag(flag string) string { return m.Palette.Magenta(flag) }

// TaskFlagDescription renders a flag's description.
func (m Messages) TaskFlagDescription(desc string) string { return m.Palette.White("…" + desc) }

// Branch renders one connector of the task tree. last selects the corner
// rather than the tee, and leaf selects a flat line rather than a fork.
func (m Messages) Branch(last, leaf bool) string {
	corner := boxVerticalAndRight
	if last {
		corner = boxUpAndRight
	}
	fork := boxDownAndHorizontal
	if leaf {
		fork = boxHorizontal
	}
	return m.Palette.White(corner) + m.Palette.White(boxHorizontal) + m.Palette.White(fork) + " "
}

// Bars renders the vertical continuation drawn beneath a node's siblings.
func (m Messages) Bars(last bool) string {
	if last {
		return "  "
	}
	return m.Palette.White(boxVertical) + " "
}

// Tildify shortens a path by replacing the user's home directory with "~",
// keeping log lines readable without hiding which file was used.
func Tildify(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" || path == "" {
		return path
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	if abs == home {
		return "~"
	}
	prefix := home + string(filepath.Separator)
	if strings.HasPrefix(abs, prefix) {
		return "~" + string(filepath.Separator) + abs[len(prefix):]
	}
	return abs
}
