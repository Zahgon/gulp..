<!-- front-matter
name: Sharing pipelines with factories
-->

# Sharing pipelines with factories

A `*Pipeline` runs once. Starting one twice is a bug, and there is nothing in
the type that stops you, so the safe habit is to build a new one every time
you need it. A function that returns a pipeline — a factory — is how you share
build logic between tasks without sharing the running object.

## The problem

This looks like it works and does not:

```go
// Wrong. One pipeline, reused.
var styles = gulp.Src([]string{"src/**/*.scss"}).
	Pipe(sass()).
	Pipe(gulp.Dest("dist"))

func build(ctx context.Context) error { return styles.Run(ctx) }
func watchBuild(ctx context.Context) error { return styles.Run(ctx) }
```

The first `Run` drains the channels and closes them. The second sees a
finished pipeline and produces nothing. Worse, if the two run concurrently
they race on the same stages.

## The fix

Return a fresh pipeline from a function:

```go
func styles() *gulp.Pipeline {
	return gulp.Src([]string{"src/**/*.scss"}).
		Pipe(sass()).
		Pipe(gulp.Dest("dist"))
}

func build(ctx context.Context) error { return styles().Run(ctx) }
```

Every call builds a new one. Because construction does no work — nothing is
globbed or opened until a terminal call — a factory is cheap enough to call in
a loop.

## Parameterising a factory

Once the pipeline is behind a function, options become arguments:

```go
func bundle(entry, out string, minify bool) *gulp.Pipeline {
	p := gulp.Src([]string{entry}).Pipe(esbuild())
	if minify {
		p = p.Pipe(terser()).Pipe(plugins.Rename(func(n *plugins.Path) {
			n.Basename += ".min"
		}))
	}
	return p.Pipe(gulp.Dest(out))
}

func init() {
	gulp.Task("app", func(ctx context.Context) error {
		return bundle("src/app.js", "dist", false).Run(ctx)
	})
	gulp.Task("app-min", func(ctx context.Context) error {
		return bundle("src/app.js", "dist", true).Run(ctx)
	})
}
```

`Pipe` returns the pipeline so the conditional stage reads naturally.

## Sharing stages instead of pipelines

A `Transform` is not a pipeline and usually *can* be shared, as long as it
holds no per-file state:

```go
// Safe: builds a new transform on every call.
func sass() gulp.Transform {
	return plugins.Exec(plugins.ExecOptions{
		Name:     "sass",
		Command:  "sass",
		TempFile: true,
		Args:     func(f *gulp.File) []string { return []string{plugins.Placeholder} },
	})
}
```

Prefer a constructor over a package-level variable even here. A transform that
looks stateless today may grow a cache tomorrow, and a shared value would then
be read by several pipelines at once. The
[plugin guidelines][guidelines] make the same point from the other side: keep
per-file state in the transform's own goroutine, never on the value.

## Reusing the *result*

If two tasks genuinely need the same files rather than the same logic, run the
pipeline once and replay the result:

```go
files, err := styles().Collect(ctx)
if err != nil {
	return err
}

if err := pipeline.New(pipeline.From(files...)).
	Pipe(gulp.Dest("dist")).Run(ctx); err != nil {
	return err
}

return pipeline.New(pipeline.From(files...)).
	Pipe(gulp.Dest("preview")).Run(ctx)
```

`pipeline.From` is replayable because it emits values you already hold. Note
that both runs see the *same* `*File` values, so a stage that mutates contents
affects the other run — `Clone()` first if that matters.

[guidelines]: ../writing-a-plugin/guidelines.md
