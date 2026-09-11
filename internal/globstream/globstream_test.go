package globstream

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gulpjs/gulp-go/pipeline"
	"github.com/gulpjs/gulp-go/vinyl"
)

// testCwd returns the repo's test/ directory. gulp's own src suite runs with
// `cwd: __dirname` pointing there, so the ported expectations use the same
// anchor and the same fixture tree.
func testCwd(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("..", "..", "test"))
	if err != nil {
		t.Fatalf("resolving test dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(abs, "fixtures")); err != nil {
		t.Fatalf("fixtures missing at %s: %v", abs, err)
	}
	return abs
}

func collect(t *testing.T, globs []string, opts Options) ([]*vinyl.File, error) {
	t.Helper()
	if opts.Cwd == "" {
		opts.Cwd = testCwd(t)
	}
	if !opts.ResolveSymlinks {
		opts.ResolveSymlinks = true
	}
	return pipeline.New(New(globs, opts)).Collect(context.Background())
}

func paths(files []*vinyl.File) []string {
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = f.Path()
	}
	return out
}

// TestFlatGlob ports gulp test/src.js "should return a input stream for a flat
// glob".
func TestFlatGlob(t *testing.T) {
	cwd := testCwd(t)
	files, err := collect(t, []string{"./fixtures/*.coffee"}, Options{})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("got %d files (%v), want 1", len(files), paths(files))
	}
	want := filepath.Join(cwd, "fixtures", "test.coffee")
	if files[0].Path() != want {
		t.Errorf("path = %q, want %q", files[0].Path(), want)
	}
	if wantBase := filepath.Join(cwd, "fixtures"); files[0].Base() != wantBase {
		t.Errorf("base = %q, want %q", files[0].Base(), wantBase)
	}
	if files[0].Contents != nil {
		t.Error("globstream must not read contents")
	}
}

// TestDeepGlob ports "should return a input stream for a deep glob".
func TestDeepGlob(t *testing.T) {
	cwd := testCwd(t)
	files, err := collect(t, []string{"./fixtures/**/*.jade"}, Options{})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("got %d files (%v), want 1", len(files), paths(files))
	}
	if want := filepath.Join(cwd, "fixtures", "test", "run.jade"); files[0].Path() != want {
		t.Errorf("path = %q, want %q", files[0].Path(), want)
	}
	// The base is the glob parent, which is what makes dest() reproduce the
	// `test/` subdirectory in the output.
	if want := filepath.Join(cwd, "fixtures"); files[0].Base() != want {
		t.Errorf("base = %q, want %q", files[0].Base(), want)
	}
}

// TestDeeperGlob ports "should return a input stream for a deeper glob".
func TestDeeperGlob(t *testing.T) {
	files, err := collect(t, []string{"./fixtures/**/*.dmc"}, Options{})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("got %d files (%v), want 2", len(files), paths(files))
	}
}

// TestMultipleGlobsPreservesOrder ports "should return a input stream for
// multiple globs": files come back grouped by glob, in argument order.
func TestMultipleGlobsPreservesOrder(t *testing.T) {
	cwd := testCwd(t)
	files, err := collect(t, []string{
		"./fixtures/stuff/run.dmc",
		"./fixtures/stuff/test.dmc",
	}, Options{})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("got %d files (%v), want 2", len(files), paths(files))
	}
	if want := filepath.Join(cwd, "fixtures", "stuff", "run.dmc"); files[0].Path() != want {
		t.Errorf("files[0] = %q, want %q", files[0].Path(), want)
	}
	if want := filepath.Join(cwd, "fixtures", "stuff", "test.dmc"); files[1].Path() != want {
		t.Errorf("files[1] = %q, want %q", files[1].Path(), want)
	}
}

// TestNegatedGlob ports "should return a input stream for multiple globs, with
// negation". The negative pattern is written relative while the positive uses
// a ./ prefix, exactly as in gulp's suite.
func TestNegatedGlob(t *testing.T) {
	cwd := testCwd(t)
	files, err := collect(t, []string{
		"./fixtures/stuff/*.dmc",
		"!fixtures/stuff/test.dmc",
	}, Options{})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("got %d files (%v), want 1", len(files), paths(files))
	}
	if want := filepath.Join(cwd, "fixtures", "stuff", "run.dmc"); files[0].Path() != want {
		t.Errorf("path = %q, want %q", files[0].Path(), want)
	}
}

// TestAbsolutePathNoMagic ports "should return a input stream with no contents
// when read is false" style absolute-path handling: a literal path resolves via
// stat rather than a walk.
func TestAbsolutePathNoMagic(t *testing.T) {
	cwd := testCwd(t)
	abs := filepath.Join(cwd, "fixtures", "test.coffee")
	files, err := collect(t, []string{abs}, Options{})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}
	if files[0].Path() != abs {
		t.Errorf("path = %q, want %q", files[0].Path(), abs)
	}
}

// TestDirectoryGlobEmitsDirectory covers the case gulp's dest suite relies on:
// src('./fixtures/stuff') emits a single vinyl describing the directory.
func TestDirectoryGlobEmitsDirectory(t *testing.T) {
	cwd := testCwd(t)
	files, err := collect(t, []string{"./fixtures/stuff"}, Options{})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("got %d files (%v), want 1", len(files), paths(files))
	}
	f := files[0]
	if !f.IsDirectory() {
		t.Errorf("expected a directory vinyl, got stat %+v contents %v", f.Stat, f.Contents)
	}
	if want := filepath.Join(cwd, "fixtures", "stuff"); f.Path() != want {
		t.Errorf("path = %q, want %q", f.Path(), want)
	}
	if want := filepath.Join(cwd, "fixtures"); f.Base() != want {
		t.Errorf("base = %q, want %q", f.Base(), want)
	}
}

// TestSingularGlobNotFound checks the documented error and its allowEmpty
// escape hatch.
func TestSingularGlobNotFound(t *testing.T) {
	_, err := collect(t, []string{"./fixtures/does-not-exist.txt"}, Options{})
	if !errors.Is(err, ErrSingularGlobNotFound) {
		t.Fatalf("err = %v, want ErrSingularGlobNotFound", err)
	}
	if got := err.Error(); !strings.Contains(got, "File not found with singular glob") ||
		!strings.Contains(got, "`allowEmpty` option") {
		t.Errorf("message %q lacks the documented wording", got)
	}

	files, err := collect(t, []string{"./fixtures/does-not-exist.txt"}, Options{AllowEmpty: true})
	if err != nil {
		t.Fatalf("allowEmpty should suppress the error, got %v", err)
	}
	if len(files) != 0 {
		t.Errorf("got %d files, want 0", len(files))
	}
}

// TestMagicGlobMayMatchNothing confirms a magic glob with no matches is not an
// error, unlike a singular glob.
func TestMagicGlobMayMatchNothing(t *testing.T) {
	files, err := collect(t, []string{"./fixtures/**/*.nope"}, Options{})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("got %d files, want 0", len(files))
	}
}

func TestInvalidGlobArgument(t *testing.T) {
	for _, globs := range [][]string{
		{},
		{""},
		{"!fixtures/stuff/test.dmc"}, // negatives only: no positive to expand
	} {
		if _, err := collect(t, globs, Options{}); !errors.Is(err, ErrInvalidGlob) {
			t.Errorf("collect(%v) err = %v, want ErrInvalidGlob", globs, err)
		}
	}
}

// TestUniqueByPath checks that overlapping globs emit each path once.
func TestUniqueByPath(t *testing.T) {
	files, err := collect(t, []string{
		"./fixtures/stuff/*.dmc",
		"./fixtures/**/*.dmc",
	}, Options{UniqueBy: "path"})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("got %d files (%v), want 2 deduped", len(files), paths(files))
	}
}

func TestUniqueDisabled(t *testing.T) {
	files, err := collect(t, []string{
		"./fixtures/stuff/*.dmc",
		"./fixtures/**/*.dmc",
	}, Options{UniqueBy: "", UniqueByFunc: nil})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(files) != 4 {
		t.Fatalf("got %d files (%v), want 4 with dedupe off", len(files), paths(files))
	}
}

func TestBaseOverrides(t *testing.T) {
	cwd := testCwd(t)

	files, err := collect(t, []string{"./fixtures/**/*.jade"}, Options{CwdBase: true})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if files[0].Base() != cwd {
		t.Errorf("cwdbase: base = %q, want %q", files[0].Base(), cwd)
	}

	explicit := filepath.Join(cwd, "fixtures", "test")
	files, err = collect(t, []string{"./fixtures/**/*.jade"}, Options{Base: explicit})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if files[0].Base() != explicit {
		t.Errorf("explicit base = %q, want %q", files[0].Base(), explicit)
	}
}

func TestIgnoreOption(t *testing.T) {
	files, err := collect(t, []string{"./fixtures/**/*.dmc"}, Options{
		Ignore: []string{"**/test.dmc"},
	})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("got %d files (%v), want 1", len(files), paths(files))
	}
	if filepath.Base(files[0].Path()) != "run.dmc" {
		t.Errorf("kept %q, want run.dmc", files[0].Path())
	}
}

// TestSince filters by modification time, backing gulp's incremental-build
// recipe (src(globs, {since: lastRun(task)})).
func TestSince(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "old.txt")
	fresh := filepath.Join(dir, "fresh.txt")
	for _, p := range []string{old, fresh} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cutoff := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(old, cutoff.Add(-time.Hour), cutoff.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}

	files, err := collect(t, []string{"*.txt"}, Options{Cwd: dir, Since: cutoff})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(files) != 1 || filepath.Base(files[0].Path()) != "fresh.txt" {
		t.Fatalf("got %v, want only fresh.txt", paths(files))
	}
}

// TestDotfiles confirms node-glob's default of skipping dot-prefixed entries,
// and that the dot option re-enables them.
func TestDotfiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"visible.txt", ".hidden.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	files, err := collect(t, []string{"*.txt"}, Options{Cwd: dir})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(files) != 1 || filepath.Base(files[0].Path()) != "visible.txt" {
		t.Fatalf("default: got %v, want only visible.txt", paths(files))
	}

	files, err = collect(t, []string{"*.txt"}, Options{Cwd: dir, Dot: true})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("dot:true: got %v, want 2 files", paths(files))
	}
}

// TestResolveSymlinks covers both the follow and no-follow modes plus the
// dangling-link fallback.
func TestResolveSymlinks(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(target, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	dangling := filepath.Join(dir, "dangling.txt")
	if err := os.Symlink(filepath.Join(dir, "missing"), dangling); err != nil {
		t.Fatal(err)
	}

	files, err := collect(t, []string{"link.txt"}, Options{Cwd: dir, ResolveSymlinks: true})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if files[0].Stat.IsSymbolic() {
		t.Error("resolveSymlinks:true should report the target's stat")
	}
	if files[0].Stat.Size() != 5 {
		t.Errorf("size = %d, want 5 (the target's)", files[0].Stat.Size())
	}

	opts := Options{Cwd: dir}
	opts.ResolveSymlinks = false
	files, err = pipeline.New(New([]string{"link.txt"}, opts)).Collect(context.Background())
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if !files[0].Stat.IsSymbolic() {
		t.Error("resolveSymlinks:false should keep the link's own stat")
	}

	files, err = collect(t, []string{"dangling.txt"}, Options{Cwd: dir, ResolveSymlinks: true})
	if err != nil {
		t.Fatalf("a dangling link must not fail the stream: %v", err)
	}
	if len(files) != 1 || !files[0].Stat.IsSymbolic() {
		t.Error("dangling link should fall back to the link's stat")
	}
}

// TestLaziness guards gulp 5.0.1's "Avoid globbing before read stream is
// opened" fix: constructing the transform must not touch the file system.
func TestLaziness(t *testing.T) {
	transform := New([]string{"./fixtures/does-not-exist.txt"}, Options{Cwd: testCwd(t)})
	if transform == nil {
		t.Fatal("New returned nil")
	}
	// No error yet, because nothing has run. Running it now surfaces one.
	if err := pipeline.New(transform).Run(context.Background()); err == nil {
		t.Fatal("expected the singular-glob error once the pipeline runs")
	}
}

func TestContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	opts := Options{Cwd: testCwd(t), ResolveSymlinks: true}
	_, err := pipeline.New(New([]string{"./fixtures/**/*"}, opts)).Collect(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}
