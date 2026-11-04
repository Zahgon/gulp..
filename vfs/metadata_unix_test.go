//go:build unix

package vfs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gulpjs/gulp-go/pipeline"
	"github.com/gulpjs/gulp-go/vinyl"
)

func TestChmodTargetUsesTheDescriptor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.js")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	handle, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() {
		if err := handle.Close(); err != nil {
			t.Errorf("closing file: %v", err)
		}
	}()

	if err := chmodTarget(handle, path, 0o640); err != nil {
		t.Fatalf("chmodTarget: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Errorf("perm = %o, want 640", got)
	}
}

func TestChmodTargetFallsBackToThePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.js")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	// A nil handle is the directory case: Dest() adjusts a directory by path
	// because it never opens one for writing.
	if err := chmodTarget(nil, path, 0o750); err != nil {
		t.Fatalf("chmodTarget: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o750 {
		t.Errorf("perm = %o, want 750", got)
	}
}

func TestChownTargetToTheCurrentOwner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.js")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	handle, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() {
		if err := handle.Close(); err != nil {
			t.Errorf("closing file: %v", err)
		}
	}()

	// Chowning a file to the ids it already has is the only change a
	// non-root test can make and still expect to succeed.
	if err := chownTarget(handle, path, os.Geteuid(), os.Getegid()); err != nil {
		t.Fatalf("chownTarget: %v", err)
	}
}

func TestIsOwnerAcceptsAFileWeJustCreated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.js")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("lstat: %v", err)
	}

	if !isOwner(vinyl.StatFromFileInfo(info)) {
		t.Error("isOwner = false for a file this process just created")
	}
}

// vinyl-fs passes a failed fchown to dest's callback, so dest reports it rather
// than writing the file and carrying on.
func TestDestReportsAFailedChown(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root may chown to any id")
	}
	base := t.TempDir()
	f, err := vinyl.New(vinyl.Options{
		Cwd:      base,
		Base:     base,
		Path:     filepath.Join(base, "app.js"),
		Contents: vinyl.Buffer([]byte("x")),
	})
	if err != nil {
		t.Fatalf("vinyl.New: %v", err)
	}
	f.Stat = &vinyl.Stat{FileMode: 0o644, UID: os.Geteuid() + 1, GID: vinyl.InvalidID}

	_, err = pipeline.New(pipeline.From(f)).Pipe(Dest(t.TempDir(), DestOptions{})).Collect(context.Background())
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("dest error = %v, want a permission error from chown", err)
	}
}

// Without a stat of its own a file gets one with unknown ids, as vinyl-fs's
// empty fs.Stats has, so dest never tries to chown it.
func TestDestLeavesTheOwnerOfAStatlessFile(t *testing.T) {
	base := t.TempDir()
	f, err := vinyl.New(vinyl.Options{
		Cwd:      base,
		Base:     base,
		Path:     filepath.Join(base, "app.js"),
		Contents: vinyl.Buffer([]byte("x")),
	})
	if err != nil {
		t.Fatalf("vinyl.New: %v", err)
	}
	out := t.TempDir()
	files, err := pipeline.New(pipeline.From(f)).Pipe(Dest(out, DestOptions{})).Collect(context.Background())
	if err != nil {
		t.Fatalf("dest: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}
}
