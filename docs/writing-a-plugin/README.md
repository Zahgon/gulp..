<!-- front-matter
id: writing-a-plugin
title: Writing a Plugin
hide_title: true
sidebar_label: Overview
-->

# Writing a Plugin

A plugin is a `pipeline.Transform`: something that reads vinyl files from a channel, does work, and writes vinyl files to another channel. That is the whole contract. There is no registry to publish to, no naming convention to follow, and no wrapper library to depend on — a plugin is an ordinary Go value that anyone can construct.

Most plugins never implement the interface directly. The `pipeline` package provides constructors that cover the common shapes:

| Constructor | Use it when |
| :---------- | :---------- |
| `pipeline.Map` | One file in, one file out (or `nil` to drop it) |
| `pipeline.Filter` | Keep or discard files without changing them |
| `pipeline.Tap` | Observe files without changing them |
| `pipeline.Flush` | The whole stream must be buffered first, as for concatenation |

## The smallest useful plugin

```go
package banner

import (
	"context"
	"fmt"

	"github.com/gulpjs/gulp-go/pipeline"
	"github.com/gulpjs/gulp-go/vinyl"
)

// Prepend adds a comment to the top of every buffered file.
func Prepend(text string) pipeline.Transform {
	return pipeline.Map(func(_ context.Context, f *vinyl.File) (*vinyl.File, error) {
		if !f.IsBuffer() {
			return f, nil
		}
		body, err := f.Bytes()
		if err != nil {
			return nil, fmt.Errorf("banner: %s: %w", f.Path(), err)
		}
		f.Contents = vinyl.Buffer(append([]byte(text), body...))
		return f, nil
	})
}
```

Use it like any other stage:

```go
gulp.Src([]string{"src/**/*.js"}).
	Pipe(banner.Prepend("// built by gulp\n")).
	Pipe(gulp.Dest("dist"))
```

## What a plugin must do

**Pass through what it does not handle.** A file can hold a buffer, a stream, or nothing at all, and it can represent a directory. A plugin that only understands buffers must return the others untouched rather than failing on them:

```go
if f.IsNull() || f.IsDirectory() {
	return f, nil
}
```

**Report errors with context.** Wrap the underlying error and name both the plugin and the file. The `plugins` package exports an error type for this:

```go
return nil, &plugins.Error{Plugin: "banner", Path: f.Path(), Err: err}
```

The pipeline attributes failures by stage position (`pipeline stage 2: ...`), but the stage number does not tell a user which of their thousand files was the problem.

**Respect the context.** Anything slow should select on `ctx.Done()`. `pipeline.Map` passes the context in for exactly this reason, and a subprocess should be built with `exec.CommandContext` so cancelling the build kills it.

**Leave the path alone unless renaming is the point.** `Dest()` writes to `directory + file.Relative()`, so changing `Path()` or `Base()` moves the output. If a plugin changes the extension, use `SetExtname` rather than rebuilding the path by hand, so the history stays intact.

## What a plugin must not do

**Never close the output channel.** The pipeline runner owns it. A transform that closes `out` will crash the next stage. This only comes up when implementing `pipeline.Transform` by hand; the constructors handle it.

**Never buffer the whole stream unless that is the feature.** `pipeline.Flush` exists for concatenation and manifests. Everything else should stream, so a build over ten thousand files does not hold them all in memory.

**Never do work at construction time.** A plugin is often built long before the pipeline runs, and may never run at all. Read files, spawn processes and allocate buffers inside the transform, not in the constructor.

**Do not reimplement a CLI tool.** If the work is already done well by `esbuild`, `sass`, `terser` or `imagemin`, wrap it with `plugins.Exec` instead of porting it. That is covered in [Using plugins][using-plugins].

## Pages in this section

1. [Using buffers][using-buffers] — the common case
2. [Dealing with streams][dealing-with-streams] — supporting `Buffer: gulp.Value(false)`
3. [Testing][testing] — how to test a transform
4. [Guidelines][guidelines] — the rules above, as a checklist
5. [Recommended modules][recommended-modules] — what the standard library already gives you

[using-buffers]: using-buffers.md
[dealing-with-streams]: dealing-with-streams.md
[testing]: testing.md
[guidelines]: guidelines.md
[recommended-modules]: recommended-modules.md
[using-plugins]: ../getting-started/7-using-plugins.md
