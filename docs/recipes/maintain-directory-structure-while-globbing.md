<!--
name: maintain-directory-structure-while-globbing
title: Maintain Directory Structure while Globbing
-->

# Maintain Directory Structure while Globbing

`Dest()` writes each file to the output directory joined with `file.Relative()`,
and `Relative()` is the path below the file's **base**. So the shape of the
output tree is decided entirely by the base, and the base is decided by the
glob.

## The base is the part before the first magic character

```go
gulp.Src([]string{"src/js/**/*.js"})
```

`src/js/` contains no magic characters, so it becomes the base. A file at
`src/js/vendor/lib.js` has a relative path of `vendor/lib.js`, and

```go
gulp.Src([]string{"src/js/**/*.js"}).Pipe(gulp.Dest("dist"))
```

writes `dist/vendor/lib.js`. The `src/js/` prefix is gone.

Move the magic earlier and the tree changes with it:

| Glob                | Base     | `src/js/vendor/lib.js` lands at |
| ------------------- | -------- | ------------------------------- |
| `src/js/**/*.js`    | `src/js` | `dist/vendor/lib.js`            |
| `src/**/*.js`       | `src`    | `dist/js/vendor/lib.js`         |
| `**/*.js`           | `.`      | `dist/src/js/vendor/lib.js`     |

## Keeping the structure you meant

To preserve everything below `src/`, put the first wildcard directly under it:

```go
gulp.Src([]string{"src/**/*.js"}).Pipe(gulp.Dest("dist"))
```

To keep the structure while still selecting from several directories, set the
base explicitly instead of hunting for a glob that happens to produce it:

```go
gulp.Src(
	[]string{"src/js/**/*.js", "src/css/**/*.css"},
	gulp.SrcOptions{Base: "src"},
).Pipe(gulp.Dest("dist"))
```

Both globs now share the base `src`, so `src/js/app.js` becomes
`dist/js/app.js` and `src/css/site.css` becomes `dist/css/site.css`. Without
`Base`, each glob would compute its own base and both files would land at the
top of `dist`.

> `Base` is the reliable answer whenever one pipeline reads from more than one
> directory. Relying on the computed base means the output tree changes if
> somebody edits a glob.

## Flattening on purpose

The opposite problem — you want everything in one directory — is `Rename`:

```go
gulp.Src([]string{"src/**/*.png"}).
	Pipe(plugins.Rename(func(p *plugins.Path) {
		p.Dirname = "."
	})).
	Pipe(gulp.Dest("dist/images"))
```

Setting `Dirname` to `.` makes `Relative()` return just the file name.

## Using the cwd as the base

`CwdBase` sets the base to the working directory for every glob, which
reproduces the full source path under the destination:

```go
gulp.Src([]string{"src/**/*.js"}, gulp.SrcOptions{CwdBase: true}).
	Pipe(gulp.Dest("dist"))
```

`src/js/app.js` becomes `dist/src/js/app.js`.

## Checking what you will get

The base and the relative path are readable, so print them before you write
anything:

```go
gulp.Src([]string{"src/**/*.js"}).
	Pipe(pipeline.Tap(func(f *gulp.File) error {
		rel, err := f.Relative()
		if err != nil {
			return err
		}
		fmt.Printf("%s -> %s\n", f.Path(), rel)
		return nil
	})).
	Run(ctx)
```

[globs]: ../getting-started/6-explaining-globs.md
[src]: ../api/src.md
[dest]: ../api/dest.md
