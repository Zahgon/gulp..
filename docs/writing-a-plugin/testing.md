<!-- front-matter
name: testing
-->

# Testing

A transform is an ordinary Go value, so it is tested with `go test` and nothing else. There is no harness to install and no mock stream to build — `pipeline.From` gives you a source and `Collect` gives you the result.

## The shape of a test

Build a file, run it through the transform, inspect what comes out.

```go
package banner_test

import (
	"context"
	"testing"

	"github.com/gulpjs/gulp-go/pipeline"
	"github.com/gulpjs/gulp-go/vinyl"

	"example.com/build/banner"
)

func TestPrependAddsTheBanner(t *testing.T) {
	src := vinyl.MustNew(vinyl.Options{
		Base:     "/project/src",
		Path:     "/project/src/app.js",
		Contents: vinyl.Buffer("console.log(1)\n"),
	})

	out, err := pipeline.New(pipeline.From(src)).
		Pipe(banner.Prepend("/* (c) 2024 */")).
		Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if len(out) != 1 {
		t.Fatalf("got %d files, want 1", len(out))
	}
	got, err := out[0].Bytes()
	if err != nil {
		t.Fatal(err)
	}
	want := "/* (c) 2024 */\nconsole.log(1)\n"
	if string(got) != want {
		t.Errorf("contents = %q, want %q", got, want)
	}
}
```

`vinyl.MustNew` is the quickest way to build a fixture. Give it a `Base` as well as a `Path`, because `Relative()` — and therefore anything a later `Dest` would do — needs one.

## Use a real directory when the transform touches the filesystem

`t.TempDir()` is cleaned up automatically, and it gives the file a `Cwd` that actually exists. That matters more than it looks: `plugins.Exec` sets the subprocess working directory to `f.Cwd()`, so a fixture with an invented path like `/project/src` fails with `chdir /project: no such file or directory`.

```go
func file(t *testing.T, rel, body string) *vinyl.File {
	t.Helper()
	base := t.TempDir()
	path := filepath.Join(base, rel)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return vinyl.MustNew(vinyl.Options{
		Cwd:      base,
		Base:     base,
		Path:     path,
		Contents: vinyl.Buffer(body),
	})
}
```

## Cover the shapes you promised to pass through

The three contents shapes are the usual source of plugin bugs, so assert each one explicitly.

```go
func TestPrependIgnoresNullFiles(t *testing.T) {
	src := vinyl.MustNew(vinyl.Options{Base: "/p", Path: "/p/app.js"})

	out, err := pipeline.New(pipeline.From(src)).
		Pipe(banner.Prepend("/* x */")).
		Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !out[0].IsNull() {
		t.Error("null file should pass through untouched")
	}
}
```

Do the same for a directory (`vinyl.Options{Stat: &vinyl.Stat{Dir: true}}`) and, if the plugin claims to support them, for a streaming file built with `vinyl.NewStreamFromBytes`.

## Assert on errors, not on strings

Return `&plugins.Error{...}` from the transform and match it with `errors.As`. The pipeline wraps stage failures as `pipeline stage 2: ...`, so a string comparison breaks the moment the plugin moves to a different position.

```go
var perr *plugins.Error
if !errors.As(err, &perr) {
	t.Fatalf("error = %v, want *plugins.Error", err)
}
if perr.Plugin != "banner" {
	t.Errorf("plugin = %q, want %q", perr.Plugin, "banner")
}
```

## Skip cleanly when an external tool is missing

A transform built on `plugins.Exec` cannot be tested where the tool is not installed, and a hard failure there is noise rather than signal.

```go
func requireCommand(t *testing.T, name string) {
	t.Helper()
	if _, err := exec.LookPath(name); err != nil {
		t.Skipf("%s not installed", name)
	}
}
```

## Run with the race detector

Every stage runs in its own goroutine, so a plugin that shares state across files — a counter, a map, a reused buffer — will be caught by `-race` and by nothing else.

```sh
go test -race ./...
```

Add `-count=1` when a test involves the filesystem or a watcher; caching a pass is exactly what you do not want from those.

---

Next: [Guidelines][guidelines].

[guidelines]: guidelines.md
