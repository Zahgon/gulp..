<!-- front-matter
id: make-stream-from-buffer
title: Make a vinyl file from memory
hide_title: true
sidebar_label: Make a file from memory
-->

# Make a vinyl file from memory

Not every file in a pipeline comes from disk. A version stamp, a generated manifest, a rendered index page — these exist only in memory, and `gulp.Src` cannot read them.

Build the vinyl file yourself and start the pipeline with `pipeline.From`.

## A single generated file

```go
package main

import (
	"context"
	"encoding/json"
	"path/filepath"

	gulp "github.com/gulpjs/gulp-go"
	"github.com/gulpjs/gulp-go/pipeline"
	"github.com/gulpjs/gulp-go/vinyl"
)

func manifest(ctx context.Context) error {
	body, err := json.MarshalIndent(map[string]string{
		"version": version,
		"commit":  commit,
	}, "", "  ")
	if err != nil {
		return err
	}

	cwd, err := filepath.Abs(".")
	if err != nil {
		return err
	}

	file, err := vinyl.New(vinyl.Options{
		Cwd:      cwd,
		Base:     cwd,
		Path:     filepath.Join(cwd, "manifest.json"),
		Contents: vinyl.Buffer(body),
	})
	if err != nil {
		return err
	}

	return pipeline.New(pipeline.From(file)).
		Pipe(gulp.Dest("dist")).
		Run(ctx)
}
```

`Base` matters as much as `Path`. `Dest()` writes to the destination joined with `Relative()`, which is `Path` measured from `Base`. Here they differ by one segment, so the file lands at `dist/manifest.json`. Set `Base` to the parent of a nested path and the directories are created for you:

```go
Base: cwd,
Path: filepath.Join(cwd, "meta", "manifest.json"),   // -> dist/meta/manifest.json
```

> Forgetting `Base` is the usual mistake. `Base()` falls back to `Cwd()`, which is often right, but if `Path` is not below the cwd then `Relative()` returns a `../..` path and `Dest()` writes outside the destination.

## Injecting a file into an existing pipeline

To add a generated file to files read from disk, collect the disk files and prepend:

```go
func build(ctx context.Context) error {
	files, err := gulp.Src([]string{"src/**/*.js"}).Collect(ctx)
	if err != nil {
		return err
	}

	banner, err := bannerFile(files)
	if err != nil {
		return err
	}

	return pipeline.New(pipeline.From(append([]*gulp.File{banner}, files...)...)).
		Pipe(gulp.Dest("dist")).
		Run(ctx)
}
```

`Collect` buffers, so use this when the generated file genuinely depends on the others — a manifest listing every bundle, for instance. If it does not, register a second task and let `gulp.Parallel` run both.

## Streaming contents

A file assembled from memory is normally a buffer. If the content is large or produced incrementally, hand `vinyl.NewStream` an opener instead:

```go
file.Contents = vinyl.NewStream(func() (io.ReadCloser, error) {
	pr, pw := io.Pipe()
	go func() {
		pw.CloseWithError(render(pw))
	}()
	return pr, nil
})
```

The opener is not called until a later stage reads, so nothing is generated for a pipeline that is built and never run. `vinyl.NewStreamFromBytes(b)` is the shorthand for the common case of wrapping bytes you already have, and unlike a plain `bytes.Reader` it can be re-read after `Reset()`.

## Directories and empty files

A vinyl file with `nil` contents is a *null* file: it is re-emitted by every stage but never written to disk. A file whose `Stat.Dir` is true is a directory, and `Dest()` creates it:

```go
dir, err := vinyl.New(vinyl.Options{
	Cwd:  cwd,
	Base: cwd,
	Path: filepath.Join(cwd, "empty-dir"),
	Stat: &vinyl.Stat{Dir: true},
})
```

To write a genuinely empty file, give it an empty buffer — `vinyl.Buffer(nil)` is null, `vinyl.Buffer([]byte{})` is empty.

## Related

- [Working with files][files] — the vinyl accessors in full.
- [Using multiple sources in one task][multiple] — merging pipelines.
- [Writing a plugin][plugin] — generating files from inside a transform.

[files]: ../getting-started/5-working-with-files.md
[multiple]: using-multiple-sources-in-one-task.md
[plugin]: ../writing-a-plugin/README.md
