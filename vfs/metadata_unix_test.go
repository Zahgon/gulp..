//go:build unix

package vfs

import (
	"os"
	"path/filepath"
	"testing"

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
	defer handle.Close()

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
	defer handle.Close()

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
