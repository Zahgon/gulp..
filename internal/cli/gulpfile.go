package cli

import (
	"os"
	"path/filepath"
)

// Version is the version this port reports for both the CLI and the library.
//
// In JavaScript these differ because the `gulp` command is installed globally
// while `gulp` itself is a project dependency. A Go gulpfile compiles the
// library in, so there is only ever one version in play.
const Version = "5.0.1"

// GulpfileNames are the files the launcher looks for, in order.
var GulpfileNames = []string{
	"gulpfile.go",
	"Gulpfile.go",
	filepath.Join("gulpfile", "main.go"),
}

// FindGulpfile searches dir and then each ancestor for a gulpfile, returning
// its absolute path.
//
// Walking upwards is what lets gulp be run from a subdirectory of a project,
// and it is the behaviour liftoff gives the JavaScript CLI.
func FindGulpfile(dir string) (string, bool) {
	current, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}
	for {
		for _, name := range GulpfileNames {
			candidate := filepath.Join(current, name)
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate, true
			}
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", false
		}
		current = parent
	}
}

// detectGulpfile reports the gulpfile for the current process, falling back to
// the working directory so that log output still names something meaningful
// when Main is called from a program that is not laid out as a gulpfile.
func detectGulpfile() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	if path, ok := FindGulpfile(cwd); ok {
		return path
	}
	return cwd
}
