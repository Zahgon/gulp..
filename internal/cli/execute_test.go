package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gulpjs/gulp-go/undertaker"
)

// locate() calls os.Chdir, which is process-global, so every test that reaches
// it must restore the working directory and must not run in parallel.
func keepCwd(t *testing.T) {
	t.Helper()
	before, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(before); err != nil {
			t.Fatal(err)
		}
	})
}

func TestRunTasksRendersTheTree(t *testing.T) {
	u, _ := buildTree(t)
	var out, errOut strings.Builder
	code := Run(Runtime{Undertaker: u, Stdout: &out, Stderr: &errOut, Gulpfile: "gulpfile.go"}, []string{"--tasks", "--no-color"})

	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, errOut.String())
	}
	got := out.String()
	for _, want := range []string{
		"Tasks for ",
		"gulpfile.go",
		"clean",
		"Remove the build directory",
		"--dry",
		"styles",
		"scripts",
		"build",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("stdout missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Using gulpfile") {
		t.Errorf("listing modes must suppress logging:\n%s", got)
	}
}

func TestRunTasksHonoursDepth(t *testing.T) {
	u, _ := buildTree(t)
	var out, errOut strings.Builder
	code := Run(Runtime{Undertaker: u, Stdout: &out, Stderr: &errOut}, []string{"--tasks", "--tasks-depth", "1", "--no-color"})

	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "build") {
		t.Errorf("stdout missing top-level task:\n%s", out.String())
	}
	if strings.Contains(out.String(), "<series>") {
		t.Errorf("depth 1 must not descend into composed branches:\n%s", out.String())
	}
}

func TestRunTasksJSONGoesToStdout(t *testing.T) {
	u, _ := buildTree(t)
	var out, errOut strings.Builder
	code := Run(Runtime{Undertaker: u, Stdout: &out, Stderr: &errOut, Gulpfile: "gulpfile.go"}, []string{"--tasks-json", "--no-color"})

	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, errOut.String())
	}

	var tree struct {
		Label string `json:"label"`
		Nodes []struct {
			Label string `json:"label"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal([]byte(out.String()), &tree); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, out.String())
	}
	if !strings.Contains(tree.Label, "gulpfile.go") {
		t.Errorf("label = %q, want it to name the gulpfile", tree.Label)
	}

	seen := make(map[string]bool, len(tree.Nodes))
	for _, n := range tree.Nodes {
		seen[n.Label] = true
	}
	for _, want := range []string{"clean", "styles", "scripts", "build"} {
		if !seen[want] {
			t.Errorf("JSON tree missing task %q:\n%s", want, out.String())
		}
	}
}

func TestRunTasksJSONWritesTheRequestedFile(t *testing.T) {
	u, _ := buildTree(t)
	path := filepath.Join(t.TempDir(), "tasks.json")

	var out, errOut strings.Builder
	code := Run(Runtime{Undertaker: u, Stdout: &out, Stderr: &errOut}, []string{"--tasks-json=" + path, "--no-color"})

	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, errOut.String())
	}
	if out.String() != "" {
		t.Errorf("writing to a file must leave stdout empty, got %q", out.String())
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(raw) {
		t.Errorf("file is not valid JSON:\n%s", raw)
	}
	if !strings.Contains(string(raw), `"build"`) {
		t.Errorf("file missing task list:\n%s", raw)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Errorf("mode = %o, want 644", got)
	}
}

func TestRunTasksJSONReportsAnUnwritablePath(t *testing.T) {
	u, _ := buildTree(t)
	path := filepath.Join(t.TempDir(), "no-such-dir", "tasks.json")

	var out, errOut strings.Builder
	code := Run(Runtime{Undertaker: u, Stdout: &out, Stderr: &errOut}, []string{"--tasks-json=" + path, "--no-color"})

	if code != ExitError {
		t.Fatalf("exit = %d, want %d", code, ExitError)
	}
	if errOut.String() == "" {
		t.Error("stderr must explain why the write failed")
	}
}

func TestRunChangesTheWorkingDirectory(t *testing.T) {
	keepCwd(t)
	dir := t.TempDir()
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}

	u := undertaker.New()
	ran := false
	u.Set("default", func(context.Context) error {
		ran = true
		return nil
	})

	var out, errOut strings.Builder
	code := Run(Runtime{Undertaker: u, Stdout: &out, Stderr: &errOut}, []string{"--cwd", dir, "--no-color"})

	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, errOut.String())
	}
	if !ran {
		t.Error("default task did not run")
	}
	if !strings.Contains(out.String(), "Working directory changed to ") {
		t.Errorf("stdout missing the cwd notice:\n%s", out.String())
	}

	now, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if got, err := filepath.EvalSymlinks(now); err != nil || got != resolved {
		t.Errorf("cwd = %q, want %q (err %v)", now, resolved, err)
	}
}

func TestRunReportsAnUnreachableCwd(t *testing.T) {
	keepCwd(t)
	missing := filepath.Join(t.TempDir(), "not-here")

	var out, errOut strings.Builder
	code := Run(Runtime{Undertaker: undertaker.New(), Stdout: &out, Stderr: &errOut}, []string{"--cwd", missing, "--no-color"})

	if code != ExitError {
		t.Fatalf("exit = %d, want %d", code, ExitError)
	}
	if errOut.String() == "" {
		t.Error("stderr must explain that the directory could not be entered")
	}
}

func TestRunTakesItsDirectoryFromTheGulpfile(t *testing.T) {
	keepCwd(t)
	dir := t.TempDir()
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	gulpfile := filepath.Join(dir, "gulpfile.go")
	if err := os.WriteFile(gulpfile, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	u := undertaker.New()
	u.Set("default", func(context.Context) error { return nil })

	var out, errOut strings.Builder
	code := Run(Runtime{Undertaker: u, Stdout: &out, Stderr: &errOut}, []string{"--gulpfile", gulpfile, "--no-color"})

	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "gulpfile.go") {
		t.Errorf("stdout missing the gulpfile name:\n%s", out.String())
	}

	now, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if got, err := filepath.EvalSymlinks(now); err != nil || got != resolved {
		t.Errorf("cwd = %q, want the gulpfile's directory %q (err %v)", now, resolved, err)
	}
}

func TestRunLetsCwdOverrideTheGulpfileDirectory(t *testing.T) {
	keepCwd(t)
	home := t.TempDir()
	elsewhere := t.TempDir()
	resolved, err := filepath.EvalSymlinks(elsewhere)
	if err != nil {
		t.Fatal(err)
	}
	gulpfile := filepath.Join(home, "gulpfile.go")
	if err := os.WriteFile(gulpfile, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	u := undertaker.New()
	u.Set("default", func(context.Context) error { return nil })

	var out, errOut strings.Builder
	code := Run(Runtime{Undertaker: u, Stdout: &out, Stderr: &errOut},
		[]string{"--gulpfile", gulpfile, "--cwd", elsewhere, "--no-color"})

	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, errOut.String())
	}

	now, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if got, err := filepath.EvalSymlinks(now); err != nil || got != resolved {
		t.Errorf("cwd = %q, want --cwd to win with %q (err %v)", now, resolved, err)
	}
}

func TestRunSeedsInitCwdOnlyWhenUnset(t *testing.T) {
	keepCwd(t)
	dir := t.TempDir()

	u := undertaker.New()
	u.Set("default", func(context.Context) error { return nil })

	t.Run("seeded from the launch directory", func(t *testing.T) {
		keepCwd(t)
		launch, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		t.Setenv("INIT_CWD", "")
		if err := os.Unsetenv("INIT_CWD"); err != nil {
			t.Fatal(err)
		}

		var out, errOut strings.Builder
		if code := Run(Runtime{Undertaker: u, Stdout: &out, Stderr: &errOut}, []string{"--cwd", dir, "--no-color"}); code != ExitOK {
			t.Fatalf("exit = %d, stderr = %q", code, errOut.String())
		}
		if got := os.Getenv("INIT_CWD"); got != launch {
			t.Errorf("INIT_CWD = %q, want the launch directory %q", got, launch)
		}
	})

	t.Run("an existing value is preserved", func(t *testing.T) {
		keepCwd(t)
		t.Setenv("INIT_CWD", "/set/by/the/caller")

		var out, errOut strings.Builder
		if code := Run(Runtime{Undertaker: u, Stdout: &out, Stderr: &errOut}, []string{"--cwd", dir, "--no-color"}); code != ExitOK {
			t.Fatalf("exit = %d, stderr = %q", code, errOut.String())
		}
		if got := os.Getenv("INIT_CWD"); got != "/set/by/the/caller" {
			t.Errorf("INIT_CWD = %q, want it left alone", got)
		}
	})
}

func TestFindGulpfileWalksUpwards(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "gulpfile.go")
	if err := os.WriteFile(want, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, ok := FindGulpfile(nested)
	if !ok {
		t.Fatal("FindGulpfile did not find the ancestor gulpfile")
	}
	if filepath.Base(got) != "gulpfile.go" {
		t.Errorf("FindGulpfile = %q, want a gulpfile.go", got)
	}
	if same, err := sameFile(got, want); err != nil || !same {
		t.Errorf("FindGulpfile = %q, want %q (err %v)", got, want, err)
	}
}

func TestFindGulpfileReportsAMissingFile(t *testing.T) {
	if got, ok := FindGulpfile(t.TempDir()); ok {
		t.Errorf("FindGulpfile = %q, want no match above an empty directory", got)
	}
}

func sameFile(a, b string) (bool, error) {
	fa, err := os.Stat(a)
	if err != nil {
		return false, err
	}
	fb, err := os.Stat(b)
	if err != nil {
		return false, err
	}
	return os.SameFile(fa, fb), nil
}

func TestRunReportsAMissingGulpfile(t *testing.T) {
	keepCwd(t)
	missing := filepath.Join(t.TempDir(), "gulpfile.go")

	var out, errOut strings.Builder
	code := Run(Runtime{Undertaker: undertaker.New(), Stdout: &out, Stderr: &errOut}, []string{"--gulpfile", missing, "--tasks", "--no-color"})

	if code != ExitError {
		t.Fatalf("exit = %d, want %d (stdout %q)", code, ExitError, out.String())
	}
	if !strings.Contains(errOut.String(), "No gulpfile found") {
		t.Errorf("stderr = %q, want the missing-gulpfile notice", errOut.String())
	}
	if out.String() != "" {
		t.Errorf("a missing gulpfile must not print a task listing:\n%s", out.String())
	}
}

func TestRunStaysQuietWhenTheDirectoryDoesNotChange(t *testing.T) {
	keepCwd(t)
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	gulpfile := filepath.Join(dir, "gulpfile.go")
	if err := os.WriteFile(gulpfile, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	u := undertaker.New()
	u.Set("default", func(context.Context) error { return nil })

	var out, errOut strings.Builder
	code := Run(Runtime{Undertaker: u, Stdout: &out, Stderr: &errOut}, []string{"--gulpfile", gulpfile, "--no-color"})

	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, errOut.String())
	}
	if strings.Contains(out.String(), "Working directory changed to") {
		t.Errorf("gulp only announces a cwd change when the directory really moves:\n%s", out.String())
	}
}

func TestRunPrintsATaskErrorOnce(t *testing.T) {
	u := undertaker.New()
	u.Set("boom", func(context.Context) error { return errors.New("boom exploded") })

	var out, errOut strings.Builder
	code := Run(Runtime{Undertaker: u, Stdout: &out, Stderr: &errOut}, []string{"boom", "--no-color"})

	if code != ExitError {
		t.Fatalf("exit = %d, want %d", code, ExitError)
	}
	if got := strings.Count(errOut.String(), "boom exploded"); got != 1 {
		t.Errorf("the failure was printed %d times, want 1:\n%s", got, errOut.String())
	}
}

func TestRunContinuesThroughANestedSeries(t *testing.T) {
	u := undertaker.New()
	u.Set("ok", func(context.Context) error { return nil })
	u.Set("boom", func(context.Context) error { return errors.New("boom exploded") })
	after := false
	u.Set("after", func(context.Context) error {
		after = true
		return nil
	})
	if _, err := u.SetTask("chain", u.Series(undertaker.Name("ok"), undertaker.Name("boom"), undertaker.Name("after"))); err != nil {
		t.Fatal(err)
	}

	var out, errOut strings.Builder
	code := Run(Runtime{Undertaker: u, Stdout: &out, Stderr: &errOut}, []string{"--continue", "chain", "--no-color"})

	if code != ExitError {
		t.Fatalf("exit = %d, want %d", code, ExitError)
	}
	if !after {
		t.Error("--continue must settle the series composed inside the gulpfile, so 'after' still runs")
	}
}
