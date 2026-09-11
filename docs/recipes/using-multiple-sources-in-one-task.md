<!--
name: using-multiple-sources-in-one-task
-->

# Using multiple sources in one task

## One glob list

Most of the time the answer is a longer glob list. `Src` accepts any number of
patterns and emits them in argument order:

```go
func vendor(ctx context.Context) error {
	return gulp.Src([]string{
		"node_modules/htmx.org/dist/htmx.min.js",
		"node_modules/alpinejs/dist/cdn.min.js",
		"src/js/**/*.js",
	}).
		Pipe(gulp.Dest("dist/js")).
		Run(ctx)
}
```

Order is guaranteed: every match of the first pattern is emitted before any
match of the second, and matches within one pattern are sorted. That is what
makes `plugins.Concat` usable — the bundle is deterministic.

> The base is computed from each pattern separately, so the three sources above
> land at different depths under `dist/js`. Set `Base` explicitly, or flatten
> with `plugins.Rename`, when that matters.

## Concatenating several sources

```go
func bundle(ctx context.Context) error {
	return gulp.Src([]string{"src/js/vendor/*.js", "src/js/app.js"}).
		Pipe(plugins.Concat("bundle.js")).
		Pipe(gulp.Dest("dist")).
		Run(ctx)
}
```

`Concat` buffers the whole stream, joins the contents with a newline and emits a
single file. It inherits `cwd` and `base` from the first file it sees, and emits
nothing at all for an empty stream.

## Merging two differently-configured pipelines

Sometimes the sources need different options — one read as a buffer, another
with `Read: false`. Build both, collect them and feed the results into a third
pipeline with `pipeline.From`:

```go
func merged(ctx context.Context) error {
	styles, err := gulp.Src([]string{"src/**/*.css"}).Collect(ctx)
	if err != nil {
		return err
	}

	icons, err := gulp.Src([]string{"assets/**/*.svg"},
		gulp.SrcOptions{Base: "assets"}).Collect(ctx)
	if err != nil {
		return err
	}

	return pipeline.New(pipeline.From(append(styles, icons...)...)).
		Pipe(gulp.Dest("dist")).
		Run(ctx)
}
```

This is the Go equivalent of `merge-stream`. It buffers both file lists in
memory, which is fine for metadata and small files, and wasteful for a large
tree read as buffers.

## Running two pipelines concurrently instead

If the two sources need no common downstream stage, do not merge them. Register
two tasks and compose them:

```go
gulp.Task("styles", styles)
gulp.Task("icons", icons)
gulp.TaskRef("assets", gulp.Parallel(gulp.Names("styles", "icons")...))
```

This is cheaper than merging, reports each half separately in the log, and lets
`--tasks` show the structure. Prefer it unless the sources genuinely have to
meet in one stream.

## Related

- [Explaining globs][globs] — how each pattern picks its base
- [Creating tasks][tasks] — `Series` and `Parallel`

[globs]: ../getting-started/6-explaining-globs.md
[tasks]: ../getting-started/3-creating-tasks.md
