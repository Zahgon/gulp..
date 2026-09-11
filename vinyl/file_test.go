package vinyl_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gulpjs/gulp-go/vinyl"
)

func fileAt(t *testing.T, path, base string) *vinyl.File {
	t.Helper()
	f, err := vinyl.New(vinyl.Options{Cwd: "/tmp", Base: base, Path: path})
	if err != nil {
		t.Fatalf("New(%q) = %v", path, err)
	}
	return f
}

func TestNewCopiesCustomMetadata(t *testing.T) {
	custom := map[string]any{"sourceMap": "inline"}
	f, err := vinyl.New(vinyl.Options{Cwd: "/tmp", Path: "/tmp/a.js", Custom: custom})
	if err != nil {
		t.Fatalf("New = %v", err)
	}
	if got := f.Custom()["sourceMap"]; got != "inline" {
		t.Fatalf("Custom()[sourceMap] = %v", got)
	}
	custom["sourceMap"] = "mutated"
	if got := f.Custom()["sourceMap"]; got != "inline" {
		t.Fatalf("the file shares its caller's map: got %v", got)
	}
}

func TestNormalizeStripsTrailingSeparators(t *testing.T) {
	sep := string(filepath.Separator)
	f := fileAt(t, filepath.Join(sep, "tmp", "pkg", "index.js")+sep, "")
	if strings.HasSuffix(f.Path(), sep) {
		t.Fatalf("Path = %q, want no trailing separator", f.Path())
	}
	root := fileAt(t, sep, "")
	if root.Path() != sep {
		t.Fatalf("root Path = %q, want %q", root.Path(), sep)
	}
}

func TestSetCwdRejectsAnEmptyValue(t *testing.T) {
	f := fileAt(t, "/tmp/a.js", "")
	if err := f.SetCwd(""); err == nil {
		t.Fatal("SetCwd(\"\") = nil, want an error")
	}
	if err := f.SetCwd("/tmp/other/"); err != nil {
		t.Fatalf("SetCwd = %v", err)
	}
	if want := filepath.Clean("/tmp/other"); f.Cwd() != want {
		t.Fatalf("Cwd = %q, want %q", f.Cwd(), want)
	}
}

func TestSetBaseResetsToCwdWhenCleared(t *testing.T) {
	f := fileAt(t, "/tmp/pkg/a.js", "/tmp/pkg")
	if err := f.SetBase("/tmp/"); err != nil {
		t.Fatalf("SetBase = %v", err)
	}
	if want := filepath.Clean("/tmp"); f.Base() != want {
		t.Fatalf("Base = %q, want %q", f.Base(), want)
	}
	if err := f.SetBase(""); err != nil {
		t.Fatalf("SetBase(\"\") = %v", err)
	}
	if f.Base() != f.Cwd() {
		t.Fatalf("Base = %q, want Cwd %q", f.Base(), f.Cwd())
	}
}

func TestRelativeReportsAMissingPath(t *testing.T) {
	f, err := vinyl.New(vinyl.Options{Cwd: "/tmp"})
	if err != nil {
		t.Fatalf("New = %v", err)
	}
	if _, err := f.Relative(); !errors.Is(err, vinyl.ErrNoPath) {
		t.Fatalf("Relative() error = %v, want ErrNoPath", err)
	}
}

func TestRelativeIsTakenFromTheBase(t *testing.T) {
	f := fileAt(t, filepath.Join("/tmp", "pkg", "src", "a.js"), filepath.Join("/tmp", "pkg"))
	rel, err := f.Relative()
	if err != nil {
		t.Fatalf("Relative() = %v", err)
	}
	if want := filepath.Join("src", "a.js"); rel != want {
		t.Fatalf("Relative() = %q, want %q", rel, want)
	}
}

func TestCloneSharesContentsWhenCopyingIsOff(t *testing.T) {
	f := fileAt(t, "/tmp/a.js", "/tmp")
	f.Contents = vinyl.Buffer("hello")
	share := false
	shallow := f.Clone(vinyl.CloneOptions{Contents: &share})
	shallow.Contents.(vinyl.Buffer)[0] = 'H'
	if string(f.Contents.(vinyl.Buffer)) != "Hello" {
		t.Fatalf("contents were copied: %q", f.Contents)
	}
	deep := f.Clone()
	deep.Contents.(vinyl.Buffer)[0] = 'y'
	if string(f.Contents.(vinyl.Buffer)) != "Hello" {
		t.Fatalf("a deep clone shared its buffer: %q", f.Contents)
	}
}

func TestCloneOfANullFileHasNoContents(t *testing.T) {
	f := fileAt(t, "/tmp/a.js", "/tmp")
	if c := f.Clone(); c.Contents != nil {
		t.Fatalf("Contents = %v, want nil", c.Contents)
	}
}

func TestStringDescribesTheFile(t *testing.T) {
	f := fileAt(t, filepath.Join("/tmp", "pkg", "a.js"), filepath.Join("/tmp", "pkg"))
	if got := f.String(); !strings.Contains(got, "a.js") {
		t.Fatalf("String() = %q", got)
	}
	f.Contents = vinyl.Buffer("x")
	if got := f.String(); !strings.Contains(got, "<Buffer>") {
		t.Fatalf("String() = %q, want a Buffer marker", got)
	}
	f.Contents = vinyl.NewStreamFromBytes([]byte("x"))
	if got := f.String(); !strings.Contains(got, "<Stream>") {
		t.Fatalf("String() = %q, want a Stream marker", got)
	}
}
