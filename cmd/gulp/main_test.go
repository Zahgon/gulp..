package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// capture swaps os.Stdout and os.Stderr for pipes, because run writes to them
// directly the way a command line program should.
func capture(t *testing.T, fn func() int) (code int, stdout, stderr string) {
	t.Helper()

	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}

	origOut, origErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outW, errW

	done := make(chan struct{})
	var outBuf, errBuf []byte
	go func() {
		defer close(done)
		outBuf, _ = io.ReadAll(outR)
	}()
	errDone := make(chan struct{})
	go func() {
		defer close(errDone)
		errBuf, _ = io.ReadAll(errR)
	}()

	code = fn()

	os.Stdout, os.Stderr = origOut, origErr
	outW.Close()
	errW.Close()
	<-done
	<-errDone

	return code, string(outBuf), string(errBuf)
}

// emptyDir moves the process into a directory with no gulpfile, which is the
// situation every test here cares about.
func emptyDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
}

// TestHelpWithoutGulpfile pins the ordering gulp-cli uses: --help is answered
// before the gulpfile is resolved, so asking for usage never fails.
func TestHelpWithoutGulpfile(t *testing.T) {
	emptyDir(t)

	code, stdout, stderr := capture(t, func() int { return run([]string{"--help"}) })

	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, stderr)
	}
	if !strings.Contains(stdout, "Usage:") {
		t.Errorf("stdout does not contain the usage block:\n%s", stdout)
	}
	if !strings.Contains(stdout, "--tasks") {
		t.Errorf("stdout does not list the flags:\n%s", stdout)
	}
	if strings.Contains(stdout, "No gulpfile found") || strings.Contains(stderr, "No gulpfile found") {
		t.Errorf("reported a missing gulpfile while printing help")
	}
}

func TestHelpShortFlagWithoutGulpfile(t *testing.T) {
	emptyDir(t)

	code, stdout, _ := capture(t, func() int { return run([]string{"-h"}) })

	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "Usage:") {
		t.Errorf("stdout does not contain the usage block:\n%s", stdout)
	}
}

// TestVersionWithoutGulpfile pins the same rule for --version, which reports
// the CLI version even when there is no project to report a local version for.
func TestVersionWithoutGulpfile(t *testing.T) {
	emptyDir(t)

	code, stdout, stderr := capture(t, func() int { return run([]string{"--version"}) })

	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, stderr)
	}
	if !strings.Contains(stdout, "CLI version:") {
		t.Errorf("stdout does not report the CLI version:\n%s", stdout)
	}
}

// TestMissingGulpfileIsAnError pins the other half of the contract: running a
// task with no gulpfile fails, exactly as gulp does.
func TestMissingGulpfileIsAnError(t *testing.T) {
	emptyDir(t)

	code, _, stderr := capture(t, func() int { return run(nil) })

	if code == 0 {
		t.Fatalf("exit code = 0, want non-zero")
	}
	if !strings.Contains(stderr, "No gulpfile found") {
		t.Errorf("stderr does not explain the failure:\n%s", stderr)
	}
}

func TestUnparseableArgumentsAreReported(t *testing.T) {
	emptyDir(t)

	code, _, stderr := capture(t, func() int { return run([]string{"--gulpfile"}) })

	if code == 0 {
		t.Fatalf("exit code = 0, want non-zero")
	}
	if stderr == "" {
		t.Error("stderr is empty; the parse error was not reported")
	}
}

// probeModule writes a throwaway Go module whose program exits with code, so
// launch can be exercised without a real gulpfile.
func probeModule(t *testing.T, code int) string {
	t.Helper()
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	write("go.mod", "module launchprobe\n\ngo 1.23\n")
	write("main.go", fmt.Sprintf("package main\n\nimport \"os\"\n\nfunc main() { os.Exit(%d) }\n", code))
	return dir
}

func requireGoToolchain(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}
}

func TestLaunchRunsTheGulpfileModule(t *testing.T) {
	requireGoToolchain(t)
	dir := probeModule(t, 0)

	if code := launch(dir, filepath.Join(dir, "main.go"), nil); code != 0 {
		t.Errorf("launch() = %d, want 0", code)
	}
}

func TestLaunchPropagatesTheExitCode(t *testing.T) {
	requireGoToolchain(t)
	dir := probeModule(t, 3)

	// A failing gulpfile must surface its own status, not a generic 1, or
	// scripts that branch on the exit code cannot tell why the build stopped.
	if code := launch(dir, filepath.Join(dir, "main.go"), nil); code != 3 {
		t.Errorf("launch() = %d, want 3", code)
	}
}
