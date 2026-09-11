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

// linkInto pipes one fixture through symlink() and returns the re-emitted files.
func linkInto(t *testing.T, globs []string, out string, opts vfs.SymlinkOptions) []*vinyl.File {
	t.Helper()
	files, err := vfs.Src(globs, vfs.SrcOptions{Cwd: testDir(t)}).
		Pipe(vfs.Symlink(out, opts)).
		Collect(context.Background())
	if err != nil {
		t.Fatalf("symlink(%v -> %s): %v", globs, out, err)
	}
	return files
}

func TestSymlinkReturnsATransform(t *testing.T) {
	if vfs.Symlink(t.TempDir(), vfs.SymlinkOptions{}) == nil {
		t.Error("Symlink returned nil")
	}
	if vfs.SymlinkWith(func(*vinyl.File) string { return "" }, vfs.SymlinkOptions{}) == nil {
		t.Error("SymlinkWith returned nil")
	}
}

func TestSymlinkWithChoosesTheDirectoryPerFile(t *testing.T) {
	out := t.TempDir()
	files, err := vfs.Src([]string{"./fixtures/*.coffee"}, vfs.SrcOptions{Cwd: testDir(t)}).
		Pipe(vfs.SymlinkWith(func(f *vinyl.File) string {
			return filepath.Join(out, filepath.Ext(f.Path())[1:])
		}, vfs.SymlinkOptions{})).
		Collect(context.Background())
	if err != nil {
		t.Fatalf("symlinkWith: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}

	link := filepath.Join(out, "coffee", "test.coffee")
	if files[0].Path() != link {
		t.Errorf("path = %q, want %q", files[0].Path(), link)
	}
	if _, err := os.Lstat(link); err != nil {
		t.Errorf("link was not created: %v", err)
	}
}

func TestSymlinkRejectsAnEmptyOutputFolder(t *testing.T) {
	_, err := vfs.Src([]string{"./fixtures/*.coffee"}, vfs.SrcOptions{Cwd: testDir(t)}).
		Pipe(vfs.SymlinkWith(func(*vinyl.File) string { return "" }, vfs.SymlinkOptions{})).
		Collect(context.Background())
	if !errors.Is(err, vfs.ErrInvalidOutputFolder) {
		t.Errorf("err = %v, want ErrInvalidOutputFolder", err)
	}
}

func TestSymlinkWritesRelativeTargetsOnRequest(t *testing.T) {
	out := t.TempDir()
	files := linkInto(t, []string{"./fixtures/*.coffee"}, out, vfs.SymlinkOptions{
		RelativeSymlinks: vfs.Value(true),
	})

	link := filepath.Join(out, "test.coffee")
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.IsAbs(target) {
		t.Errorf("target = %q, want a relative path", target)
	}
	if files[0].Symlink() != target {
		t.Errorf("file symlink = %q, want %q", files[0].Symlink(), target)
	}

	// A relative link is only useful if it actually resolves from the directory
	// it lives in.
	resolved := filepath.Join(filepath.Dir(link), target)
	want := filepath.Join(testDir(t), "fixtures", "test.coffee")
	if same, err := sameFile(resolved, want); err != nil || !same {
		t.Errorf("relative target resolves to %q, want %q (err %v)", resolved, want, err)
	}
}

func TestSymlinkReplacesAnExistingLinkByDefault(t *testing.T) {
	out := t.TempDir()
	link := filepath.Join(out, "test.coffee")
	if err := os.Symlink(filepath.Join(out, "stale"), link); err != nil {
		t.Fatal(err)
	}

	linkInto(t, []string{"./fixtures/*.coffee"}, out, vfs.SymlinkOptions{})

	target, err := os.Readlink(link)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(testDir(t), "fixtures", "test.coffee")
	if target != want {
		t.Errorf("target = %q, want the fixture %q", target, want)
	}
}

func TestSymlinkKeepsAnExistingLinkWhenOverwriteIsOff(t *testing.T) {
	out := t.TempDir()
	link := filepath.Join(out, "test.coffee")
	stale := filepath.Join(out, "stale")
	if err := os.Symlink(stale, link); err != nil {
		t.Fatal(err)
	}

	linkInto(t, []string{"./fixtures/*.coffee"}, out, vfs.SymlinkOptions{
		Overwrite: vfs.Value(false),
	})

	target, err := os.Readlink(link)
	if err != nil {
		t.Fatal(err)
	}
	if target != stale {
		t.Errorf("target = %q, want the pre-existing %q", target, stale)
	}
}

func TestSymlinkCreatesMissingDirectories(t *testing.T) {
	out := filepath.Join(t.TempDir(), "deeply", "nested")
	linkInto(t, []string{"./fixtures/*.coffee"}, out, vfs.SymlinkOptions{
		DirMode: vfs.Value(os.FileMode(0o755)),
	})

	info, err := os.Lstat(filepath.Join(out, "test.coffee"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("mode = %v, want a symlink", info.Mode())
	}
}

func TestSymlinkStatDescribesTheLinkNotTheTarget(t *testing.T) {
	out := t.TempDir()
	files := linkInto(t, []string{"./fixtures/*.coffee"}, out, vfs.SymlinkOptions{})

	if files[0].Stat == nil {
		t.Fatal("stat was not re-attached")
	}
	if !files[0].Stat.Symlink {
		t.Error("stat must describe the link itself")
	}
}

func TestSymlinkResolvesTheOutputAgainstCwd(t *testing.T) {
	out := t.TempDir()
	files := linkInto(t, []string{"./fixtures/*.coffee"}, "relative-out", vfs.SymlinkOptions{
		Cwd: out,
	})

	link := filepath.Join(out, "relative-out", "test.coffee")
	if files[0].Path() != link {
		t.Errorf("path = %q, want %q", files[0].Path(), link)
	}
	if _, err := os.Lstat(link); err != nil {
		t.Errorf("link was not created under cwd: %v", err)
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
