package vfs_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/gulpjs/gulp-go/vfs"
	"github.com/gulpjs/gulp-go/vinyl"
)

// testDir is the directory gulp's own test suite runs from, so that the
// relative globs in the ported cases resolve exactly as they do in JavaScript.
func testDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "test"))
	if err != nil {
		t.Fatalf("resolving test dir: %v", err)
	}
	return dir
}

// collect runs a src() pipeline to completion and returns the files it emitted.
func collect(t *testing.T, globs []string, opts vfs.SrcOptions) []*vinyl.File {
	t.Helper()
	if opts.Cwd == "" {
		opts.Cwd = testDir(t)
	}
	files, err := vfs.Src(globs, opts).Collect(context.Background())
	if err != nil {
		t.Fatalf("src(%v): %v", globs, err)
	}
	return files
}

// readOnDisk returns the bytes a fixture actually holds, which the assertions
// below compare against. gulp's JavaScript suite compares with a trailing
// newline stripped; the fixtures on disk do have that newline, so this port
// asserts against the real bytes instead.
func readOnDisk(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return b
}

func TestSrcReturnsAStream(t *testing.T) {
	if vfs.Src([]string{"./fixtures/*.coffee"}, vfs.SrcOptions{}) == nil {
		t.Fatal("Src returned nil")
	}
}

func TestSrcReturnsAnInputStreamFromAFlatGlob(t *testing.T) {
	dir := testDir(t)
	files := collect(t, []string{"./fixtures/*.coffee"}, vfs.SrcOptions{})

	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}
	want := filepath.Join(dir, "fixtures", "test.coffee")
	if files[0].Path() != want {
		t.Errorf("path = %q, want %q", files[0].Path(), want)
	}
	got, err := files[0].Bytes()
	if err != nil {
		t.Fatalf("reading contents: %v", err)
	}
	if string(got) != string(readOnDisk(t, want)) {
		t.Errorf("contents = %q, want %q", got, readOnDisk(t, want))
	}
}

func TestSrcReturnsAnInputStreamForMultipleGlobs(t *testing.T) {
	dir := testDir(t)
	files := collect(t, []string{
		"./fixtures/stuff/run.dmc",
		"./fixtures/stuff/test.dmc",
	}, vfs.SrcOptions{})

	if len(files) != 2 {
		t.Fatalf("got %d files, want 2", len(files))
	}
	if want := filepath.Join(dir, "fixtures", "stuff", "run.dmc"); files[0].Path() != want {
		t.Errorf("files[0] = %q, want %q", files[0].Path(), want)
	}
	if want := filepath.Join(dir, "fixtures", "stuff", "test.dmc"); files[1].Path() != want {
		t.Errorf("files[1] = %q, want %q", files[1].Path(), want)
	}
}

func TestSrcReturnsAnInputStreamForMultipleGlobsWithNegation(t *testing.T) {
	dir := testDir(t)
	files := collect(t, []string{
		"./fixtures/stuff/*.dmc",
		"!fixtures/stuff/test.dmc",
	}, vfs.SrcOptions{})

	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}
	if want := filepath.Join(dir, "fixtures", "stuff", "run.dmc"); files[0].Path() != want {
		t.Errorf("path = %q, want %q", files[0].Path(), want)
	}
}

func TestSrcReturnsAnInputStreamWithNoContentsWhenReadIsFalse(t *testing.T) {
	dir := testDir(t)
	files := collect(t, []string{"./fixtures/*.coffee"}, vfs.SrcOptions{
		Read: vfs.Value(false),
	})

	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}
	if !files[0].IsNull() {
		t.Error("contents should be empty when read is disabled")
	}
	if want := filepath.Join(dir, "fixtures", "test.coffee"); files[0].Path() != want {
		t.Errorf("path = %q, want %q", files[0].Path(), want)
	}
}

func TestSrcReturnsAnInputStreamWithContentsAsStreamWhenBufferIsFalse(t *testing.T) {
	dir := testDir(t)
	files := collect(t, []string{"./fixtures/*.coffee"}, vfs.SrcOptions{
		Buffer: vfs.Value(false),
	})

	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}
	if !files[0].IsStream() {
		t.Fatal("contents should be a stream when buffer is disabled")
	}

	stream, ok := files[0].Contents.(*vinyl.Stream)
	if !ok {
		t.Fatalf("contents type = %T, want *vinyl.Stream", files[0].Contents)
	}
	defer stream.Close()

	got, err := io.ReadAll(stream)
	if err != nil {
		t.Fatalf("reading stream: %v", err)
	}
	want := readOnDisk(t, filepath.Join(dir, "fixtures", "test.coffee"))
	if string(got) != string(want) {
		t.Errorf("contents = %q, want %q", got, want)
	}
}

func TestSrcReturnsAnInputStreamFromADeepGlob(t *testing.T) {
	dir := testDir(t)
	files := collect(t, []string{"./fixtures/**/*.jade"}, vfs.SrcOptions{})

	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}
	want := filepath.Join(dir, "fixtures", "test", "run.jade")
	if files[0].Path() != want {
		t.Errorf("path = %q, want %q", files[0].Path(), want)
	}
	got, err := files[0].Bytes()
	if err != nil {
		t.Fatalf("reading contents: %v", err)
	}
	if string(got) != string(readOnDisk(t, want)) {
		t.Errorf("contents = %q, want %q", got, readOnDisk(t, want))
	}
}

func TestSrcReturnsAnInputStreamFromADeeperGlob(t *testing.T) {
	files := collect(t, []string{"./fixtures/**/*.dmc"}, vfs.SrcOptions{})
	if len(files) != 2 {
		t.Fatalf("got %d files, want 2", len(files))
	}
}

func TestSrcReturnsAFileStreamFromAFlatPath(t *testing.T) {
	dir := testDir(t)
	absolute := filepath.Join(dir, "fixtures", "test.coffee")
	files := collect(t, []string{absolute}, vfs.SrcOptions{})

	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}
	if files[0].Path() != absolute {
		t.Errorf("path = %q, want %q", files[0].Path(), absolute)
	}
}
