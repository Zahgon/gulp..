package vfs_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gulpjs/gulp-go/vfs"
	"github.com/gulpjs/gulp-go/vinyl"
)

// pipe runs a src() pipeline through dest() and returns the re-emitted files.
func pipe(t *testing.T, globs []string, srcOpts vfs.SrcOptions, out string, destOpts vfs.DestOptions) []*vinyl.File {
	t.Helper()
	if srcOpts.Cwd == "" {
		srcOpts.Cwd = testDir(t)
	}
	files, err := vfs.Src(globs, srcOpts).
		Pipe(vfs.Dest(out, destOpts)).
		Collect(context.Background())
	if err != nil {
		t.Fatalf("src(%v) -> dest(%s): %v", globs, out, err)
	}
	return files
}

func TestDestReturnsAStream(t *testing.T) {
	if vfs.Dest(t.TempDir(), vfs.DestOptions{}) == nil {
		t.Fatal("Dest returned nil")
	}
}

func TestDestEmptyFolderIsAnError(t *testing.T) {
	_, err := vfs.Src([]string{"./fixtures/*.coffee"}, vfs.SrcOptions{Cwd: testDir(t)}).
		Pipe(vfs.Dest("", vfs.DestOptions{})).
		Collect(context.Background())
	if !errors.Is(err, vfs.ErrInvalidDestFolder) {
		t.Errorf("error = %v, want %v", err, vfs.ErrInvalidDestFolder)
	}
}

func TestDestReturnsAnOutputStreamThatWritesFiles(t *testing.T) {
	out := t.TempDir()
	files := pipe(t, []string{"./fixtures/**/*.txt"}, vfs.SrcOptions{}, out, vfs.DestOptions{})

	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}

	want := filepath.Join(out, "copy", "example.txt")
	if files[0].Path() != want {
		t.Errorf("path = %q, want %q", files[0].Path(), want)
	}

	source := readOnDisk(t, filepath.Join(testDir(t), "fixtures", "copy", "example.txt"))

	got, err := files[0].Bytes()
	if err != nil {
		t.Fatalf("reading contents: %v", err)
	}
	if string(got) != string(source) {
		t.Errorf("contents = %q, want %q", got, source)
	}
	if written := readOnDisk(t, want); string(written) != string(source) {
		t.Errorf("on disk = %q, want %q", written, source)
	}
}

func TestDestReturnsAnOutputStreamThatDoesNotWriteNonReadFiles(t *testing.T) {
	out := t.TempDir()
	files := pipe(t, []string{"./fixtures/**/*.txt"},
		vfs.SrcOptions{Read: vfs.Value(false)}, out, vfs.DestOptions{})

	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}
	if !files[0].IsNull() {
		t.Error("contents should stay empty")
	}

	want := filepath.Join(out, "copy", "example.txt")
	if files[0].Path() != want {
		t.Errorf("path = %q, want %q", files[0].Path(), want)
	}
	if _, err := os.Stat(want); !os.IsNotExist(err) {
		t.Errorf("file should not have been written, stat error = %v", err)
	}
}

func TestDestReturnsAnOutputStreamThatWritesStreamingFiles(t *testing.T) {
	out := t.TempDir()
	files := pipe(t, []string{"./fixtures/**/*.txt"},
		vfs.SrcOptions{Buffer: vfs.Value(false)}, out, vfs.DestOptions{})

	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}

	want := filepath.Join(out, "copy", "example.txt")
	if files[0].Path() != want {
		t.Errorf("path = %q, want %q", files[0].Path(), want)
	}

	source := readOnDisk(t, filepath.Join(testDir(t), "fixtures", "copy", "example.txt"))
	if written := readOnDisk(t, want); string(written) != string(source) {
		t.Errorf("on disk = %q, want %q", written, source)
	}
}

// TestDestReEmitsAReadableStream covers the behaviour dest() documents for
// streaming contents: the stream is re-armed after writing so a later stage can
// still read the file.
func TestDestReEmitsAReadableStream(t *testing.T) {
	out := t.TempDir()
	files := pipe(t, []string{"./fixtures/**/*.txt"},
		vfs.SrcOptions{Buffer: vfs.Value(false)}, out, vfs.DestOptions{})

	if !files[0].IsStream() {
		t.Fatal("contents should still be a stream after dest()")
	}
	got, err := files[0].Bytes()
	if err != nil {
		t.Fatalf("re-reading contents: %v", err)
	}
	source := readOnDisk(t, filepath.Join(testDir(t), "fixtures", "copy", "example.txt"))
	if string(got) != string(source) {
		t.Errorf("contents = %q, want %q", got, source)
	}
}

// TestDestReturnsAnOutputStreamThatWritesStreamingFilesIntoNewDirectories is the port of gulp's testWriteDir cases, which run
// the same assertions across every combination of the read and buffer options
// to prove that directory entries are recreated regardless of how they were
// read.
func TestDestReturnsAnOutputStreamThatWritesStreamingFilesIntoNewDirectories(t *testing.T) {
	cases := map[string]vfs.SrcOptions{
		"defaults":                {},
		"buffer false":            {Buffer: vfs.Value(false)},
		"read false":              {Read: vfs.Value(false)},
		"read false buffer false": {Buffer: vfs.Value(false), Read: vfs.Value(false)},
	}

	for name, srcOpts := range cases {
		t.Run(name, func(t *testing.T) {
			out := t.TempDir()
			files := pipe(t, []string{"./fixtures/stuff"}, srcOpts, out, vfs.DestOptions{})

			if len(files) != 1 {
				t.Fatalf("got %d files, want 1", len(files))
			}

			want := filepath.Join(out, "stuff")
			if files[0].Path() != want {
				t.Errorf("path = %q, want %q", files[0].Path(), want)
			}

			info, err := os.Stat(want)
			if err != nil {
				t.Fatalf("stat %s: %v", want, err)
			}
			if !info.IsDir() {
				t.Errorf("%s is not a directory", want)
			}
		})
	}
}

func TestDestOverwriteFalseKeepsExistingFile(t *testing.T) {
	out := t.TempDir()
	existing := filepath.Join(out, "copy", "example.txt")
	if err := os.MkdirAll(filepath.Dir(existing), 0o755); err != nil {
		t.Fatalf("preparing destination: %v", err)
	}
	if err := os.WriteFile(existing, []byte("untouched"), 0o644); err != nil {
		t.Fatalf("preparing destination: %v", err)
	}

	pipe(t, []string{"./fixtures/**/*.txt"}, vfs.SrcOptions{}, out,
		vfs.DestOptions{Overwrite: vfs.Value(false)})

	if got := readOnDisk(t, existing); string(got) != "untouched" {
		t.Errorf("on disk = %q, want %q", got, "untouched")
	}
}

func TestDestAppendAddsToExistingFile(t *testing.T) {
	out := t.TempDir()
	existing := filepath.Join(out, "copy", "example.txt")
	if err := os.MkdirAll(filepath.Dir(existing), 0o755); err != nil {
		t.Fatalf("preparing destination: %v", err)
	}
	if err := os.WriteFile(existing, []byte("head:"), 0o644); err != nil {
		t.Fatalf("preparing destination: %v", err)
	}

	pipe(t, []string{"./fixtures/**/*.txt"}, vfs.SrcOptions{}, out,
		vfs.DestOptions{Append: vfs.Value(true)})

	source := readOnDisk(t, filepath.Join(testDir(t), "fixtures", "copy", "example.txt"))
	want := "head:" + string(source)
	if got := readOnDisk(t, existing); string(got) != want {
		t.Errorf("on disk = %q, want %q", got, want)
	}
}

func TestSymlinkCreatesLinkToOriginal(t *testing.T) {
	out := t.TempDir()
	source := filepath.Join(testDir(t), "fixtures", "test.coffee")

	files, err := vfs.Src([]string{"./fixtures/*.coffee"}, vfs.SrcOptions{Cwd: testDir(t)}).
		Pipe(vfs.Symlink(out, vfs.SymlinkOptions{})).
		Collect(context.Background())
	if err != nil {
		t.Fatalf("symlink: %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}

	link := filepath.Join(out, "test.coffee")
	if files[0].Path() != link {
		t.Errorf("path = %q, want %q", files[0].Path(), link)
	}
	if files[0].Symlink() != source {
		t.Errorf("symlink = %q, want %q", files[0].Symlink(), source)
	}
	if !files[0].IsNull() {
		t.Error("contents should be cleared")
	}

	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("readlink %s: %v", link, err)
	}
	if target != source {
		t.Errorf("link target = %q, want %q", target, source)
	}
	if got := readOnDisk(t, link); string(got) != string(readOnDisk(t, source)) {
		t.Error("link does not resolve to the original contents")
	}
}
