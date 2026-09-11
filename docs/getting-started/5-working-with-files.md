<!-- front-matter
id: working-with-files
title: Working with Files
hide_title: true
sidebar_label: Working with Files
-->

# Working with Files

`Src()` and `Dest()` are the two ends of every file pipeline. `Src()` reads
files off disk and emits them; `Dest()` writes them back out. Everything in
between is a transform.

```go
func copyStyles(ctx context.Context) error {
	return gulp.Src([]string{"src/**/*.css"}).
		Pipe(gulp.Dest("dist")).
		Run(ctx)
}
```

## Vinyl files

A file travelling through a pipeline is a `*gulp.File` — a vinyl file. It is a
metadata object with contents attached, not a path and not an `os.File`:

```go
f.Path()      // /home/me/project/src/css/site.css
f.Base()      // /home/me/project/src
f.Relative()  // css/site.css
f.Cwd()       // /home/me/project
f.Stat        // *vinyl.Stat, an fs.FileInfo
f.Contents    // vinyl.Buffer, *vinyl.Stream, or nil
```

`Relative()` is the important one. It is `Path()` minus `Base()`, and it is
what `Dest()` joins onto the output directory. Preserving it is what keeps your
directory structure intact.

## Base, and why globs set it

The **base** is the part of the glob before the first magic character:

| Glob                  | Base       |
| --------------------- | ---------- |
| `src/css/**/*.css`    | `src/css/` |
| `src/**/*.css`        | `src/`     |
| `src/css/site.css`    | `src/css/` |

So `gulp.Src([]string{"src/**/*.css"}).Pipe(gulp.Dest("dist"))` turns
`src/css/site.css` into `dist/css/site.css`, while
`gulp.Src([]string{"src/css/**/*.css"})` turns the same input into
`dist/site.css`. If you want a different answer, set the base yourself:

```go
gulp.Src([]string{"src/css/**/*.css"}, gulp.SrcOptions{Base: "src"})
```

## Contents

A file's contents come in three shapes, and the predicates tell them apart:

- `f.IsBuffer()` — the whole file is in memory as a `vinyl.Buffer`.
- `f.IsStream()` — a `*vinyl.Stream`, read on demand.
- `f.IsNull()` — no contents at all, which is also how directories travel.

Buffers are the default because almost every transform needs the whole file.
Switch to streams for files too large to hold in memory:

```go
gulp.Src([]string{"assets/**/*.mp4"}, gulp.SrcOptions{
	Buffer: gulp.Value(false),
})
```

Or skip reading entirely when you only care about paths — moving, deleting or
symlinking:

```go
gulp.Src([]string{"dist/**/*"}, gulp.SrcOptions{
	Read: gulp.Value(false),
})
```

A file with null contents is **not written to disk** by `Dest()`, though it is
still re-emitted so later stages can see it. Directories are created.

> `gulp.Value(x)` pins an option to a constant. `gulp.Func(fn)` computes it per
> file, which is the equivalent of passing a function where the JavaScript API
> accepts a value.

## Reading

```go
gulp.Src([]string{"src/**/*.js", "!src/vendor/**"}, gulp.SrcOptions{
	Cwd:       "/path/to/project",
	AllowEmpty: true,
	Since:     gulp.Value(lastBuild),
})
```

Globs are matched in the order you give them, and a leading `!` negates. Some
options worth knowing:

- `AllowEmpty` — by default a glob with no magic characters that matches
  nothing is an error (`File not found with singular glob`), which catches
  typos. Set this to accept it.
- `Since` — skip files not modified since a timestamp. Pair it with
  `LastRun()` for incremental builds.
- `Encoding` — decode with something other than UTF-8, or set it to `""` to
  disable decoding entirely.
- `RemoveBOM` — strips a UTF-8 byte order mark by default.
- `ResolveSymlinks` — follow symlinks by default; dangling links are tolerated.

## Writing

```go
gulp.Dest("dist", gulp.DestOptions{
	Mode:      gulp.Value(os.FileMode(0o644)),
	Overwrite: gulp.Value(true),
})
```

`Dest()` re-emits every file after writing it, so you can write to more than
one place:

```go
return gulp.Src([]string{"src/**/*.css"}).
	Pipe(gulp.Dest("dist")).
	Pipe(plugins.Exec(minifyCSS)).
	Pipe(gulp.Dest("dist/min")).
	Run(ctx)
```

Writing updates the file's `Cwd`, `Base`, `Path` and `Stat` to describe the
copy that now exists, and re-arms a streaming `Contents` so the next stage can
read it again.

Permissions and timestamps are copied from the vinyl's `Stat` when they differ
from what landed on disk, using the open file descriptor. Ownership is copied
too, but only when the process actually owns the file or is running as root —
the same guard vinyl-fs applies. On platforms without Unix ids the whole step
is skipped.

The destination may also be computed per file:

```go
gulp.DestWith(func(f *gulp.File) string {
	if strings.HasSuffix(f.Path(), ".css") {
		return "dist/css"
	}
	return "dist"
})
```

## Symlinks

`Symlink()` takes the same arguments as `Dest()` but links instead of copying:

```go
return gulp.Src([]string{"src/**/*"}, gulp.SrcOptions{Read: gulp.Value(false)}).
	Pipe(gulp.Symlink("dist")).
	Run(ctx)
```

`Read: false` is the usual companion — there is no reason to read contents you
are not going to write. Set `RelativeSymlinks` to store a relative target
rather than an absolute one.

> `UseJunctions` is accepted for API compatibility but has no effect: Go's
> `os.Symlink` does not let a caller choose the Windows link type.

## Next

[Explaining Globs][globs]

[globs]: 6-explaining-globs.md
