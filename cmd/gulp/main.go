// Command gulp runs a Go gulpfile.
//
// It is the counterpart of the `gulp` binary installed by gulp-cli. The
// JavaScript launcher finds a gulpfile, transpiles it if necessary and
// requires it into its own process; that is impossible in Go, where a gulpfile
// is compiled code rather than a script. So this launcher finds the gulpfile,
// builds it, runs the result, and forwards every argument. The gulpfile's own
// gulp.Main call then performs the real flag handling, which keeps a single
// implementation of the command line behaviour.
//
// The practical consequence is that the launcher is optional. `go run .` in a
// gulpfile directory does exactly the same thing.
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gulpjs/gulp-go/internal/cli"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(argv []string) int {
	opts, err := cli.Parse(argv)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return cli.ExitError
	}

	palette := cli.NewPalette(opts.Color, opts.NoColor)
	msg := cli.Messages{Palette: palette}

	// gulp-cli answers --help in handleArguments, before it checks whether a
	// gulpfile was resolved, so `gulp --help` works in any directory. Asking
	// for usage and being told "No gulpfile found" would be a poor trade.
	if opts.Help {
		fmt.Fprintln(os.Stdout, cli.Help(msg))
		return cli.ExitOK
	}

	dir, gulpfile, err := locate(opts)
	if err != nil {
		if opts.Version {
			return reportVersions(palette, "")
		}
		fmt.Fprintln(os.Stderr, msg.MissingGulpfile())
		return cli.ExitError
	}

	if opts.Version {
		return reportVersions(palette, dir)
	}

	return launch(dir, gulpfile, argv)
}

// locate resolves the working directory and gulpfile the same way the CLI does,
// so that --cwd and --gulpfile behave identically whether or not the launcher
// is used. --cwd wins over the directory implied by --gulpfile.
func locate(opts *cli.Options) (dir, gulpfile string, err error) {
	start, err := os.Getwd()
	if err != nil {
		return "", "", err
	}
	if opts.Cwd != "" {
		if start, err = filepath.Abs(opts.Cwd); err != nil {
			return "", "", err
		}
	}

	if opts.Gulpfile != "" {
		gulpfile, err = filepath.Abs(opts.Gulpfile)
		if err != nil {
			return "", "", err
		}
		if _, statErr := os.Stat(gulpfile); statErr != nil {
			return "", "", statErr
		}
		if opts.Cwd == "" {
			start = filepath.Dir(gulpfile)
		}
		return filepath.Dir(gulpfile), gulpfile, nil
	}

	gulpfile, ok := cli.FindGulpfile(start)
	if !ok {
		return "", "", errors.New("no gulpfile found")
	}
	return filepath.Dir(gulpfile), gulpfile, nil
}

// launch compiles and runs the gulpfile's package, forwarding argv untouched.
//
// The package directory rather than the single file is compiled, so that a
// gulpfile split across several files still builds.
//
// Building and then running is what preserves the gulpfile's exit status.
// `go run` reports a failing program as "exit status N" on stderr and then
// exits 1 itself, so the real code never reaches the caller, and a build tool
// that cannot report how it failed is of little use to the CI job running it.
func launch(dir, gulpfile string, argv []string) int {
	binary, err := compile(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return cli.ExitError
	}
	defer func() { _ = os.Remove(binary) }()

	cmd := exec.Command(binary, argv...)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "GULP_GULPFILE="+gulpfile)

	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return exit.ExitCode()
		}
		fmt.Fprintln(os.Stderr, err)
		return cli.ExitError
	}
	return cli.ExitOK
}

// compile builds the gulpfile's package into a temporary binary and returns
// its path. The caller owns the file and is responsible for removing it.
//
// Build diagnostics go to stderr rather than stdout so that a gulpfile which
// fails to compile does not corrupt output being piped to another program.
func compile(dir string) (string, error) {
	binary := filepath.Join(os.TempDir(), fmt.Sprintf("gulp-go-%d", os.Getpid()))
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}

	cmd := exec.Command("go", "build", "-o", binary, ".")
	cmd.Dir = dir
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("build gulpfile: %w", err)
	}
	return binary, nil
}

// reportVersions mirrors `gulp -v`, which prints the version of the globally
// installed CLI followed by the version of the gulp the project depends on.
// The local version is read from the project's module graph; when there is no
// project, or gulp-go is not a dependency of it, only the CLI line is printed.
func reportVersions(palette cli.Palette, dir string) int {
	fmt.Printf("CLI version: %s\n", palette.Magenta(cli.Version))
	if local := localVersion(dir); local != "" {
		fmt.Printf("Local version: %s\n", palette.Magenta(local))
	}
	return cli.ExitOK
}

func localVersion(dir string) string {
	if dir == "" {
		return ""
	}
	cmd := exec.Command("go", "list", "-m", "-f", "{{.Version}}", "github.com/gulpjs/gulp-go")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
