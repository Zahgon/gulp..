package cli

import "strings"

// flagHelp carries the descriptions gulp-cli prints for --help, verbatim.
var flagHelp = map[string]string{
	"help":     "Show this help.",
	"version":  "Print the global and local gulp versions.",
	"preload":  "Will preload a module before running the gulpfile. This is useful for transpilers but also has other applications.",
	"gulpfile": "Manually set path of gulpfile. Useful if you have multiple gulpfiles. This will set the CWD to the gulpfile directory as well.",
	"cwd":      "Manually set the CWD. The search for the gulpfile, as well as the relativity of all requires will be from here.",
	"tasks":    "Print the task dependency tree for the loaded gulpfile.",

	"tasks-simple":  "Print a plaintext list of tasks for the loaded gulpfile.",
	"tasks-json":    "Print the task dependency tree, in JSON format, for the loaded gulpfile.",
	"tasks-depth":   "Specify the depth of the task dependency tree.",
	"compact-tasks": "Reduce the output of task dependency tree by printing only top tasks and their child tasks.",
	"sort-tasks":    "Will sort top tasks of task dependency tree.",
	"color":         "Will force gulp and gulp plugins to display colors, even when no color support is detected.",
	"no-color":      "Will force gulp and gulp plugins to not display colors, even when color support is detected.",
	"silent":        "Suppress all gulp logging.",
	"continue":      "Continue execution of tasks upon failure.",
	"series":        "Run tasks given on the CLI in series (the default is parallel).",
	"log-level":     "Set the loglevel. -L for least verbose and -LLLL for most verbose. -LLL is default.",
}

// helpOrder lists the flags in the order gulp-cli declares them. "require" is
// omitted because it is only an alias kept for compatibility with gulp's own
// CLI documentation, which still uses the pre-3.0 name for --preload.
var helpOrder = []string{
	"help", "version", "preload", "gulpfile", "cwd", "tasks", "tasks-simple",
	"tasks-json", "tasks-depth", "compact-tasks", "sort-tasks", "color",
	"no-color", "silent", "continue", "series", "log-level",
}

// Help renders the --help screen.
func Help(msg Messages) string {
	var b strings.Builder
	b.WriteString(msg.Usage())
	b.WriteString("\n\nOptions:\n")

	labels := make([]string, len(helpOrder))
	widest := 0
	for i, name := range helpOrder {
		labels[i] = flagLabel(name)
		if w := DisplayWidth(labels[i]); w > widest {
			widest = w
		}
	}
	for i, name := range helpOrder {
		padding := strings.Repeat(" ", widest-DisplayWidth(labels[i]))
		b.WriteString("  " + labels[i] + padding + "  " + msg.Palette.Gray(flagHelp[name]) + "\n")
	}
	return b.String()
}

func flagLabel(name string) string {
	spec, ok := lookupFlag(name)
	if ok && spec.short != "" {
		return "-" + spec.short + ", --" + name
	}
	return "    --" + name
}
