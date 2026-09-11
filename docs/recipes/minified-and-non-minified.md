<!--
name: minified-and-non-minified
-->

# Minified and non-minified

Write both `app.js` and `app.min.js` from a single read of the source.

## Two destinations, one pipeline

`Dest()` re-emits every file it writes, so a second `Dest()` further down the
chain sees them again. Put the unminified write before the minifier:

```go
func scripts(ctx context.Context) error {
	return gulp.Src([]string{"src/**/*.js"}).
		Pipe(gulp.Dest("dist")).
		Pipe(plugins.Exec(plugins.ExecOptions{
			Name:    "terser",
			Command: "terser",
			Args:    func(*gulp.File) []string { return []string{"--compress", "--mangle"} },
		})).
		Pipe(plugins.Rename(func(p *plugins.Path) {
			p.Basename += ".min"
		})).
		Pipe(gulp.Dest("dist")).
		Run(ctx)
}
```

`src/app.js` produces `dist/app.js` and `dist/app.min.js`. The source is read
once and minified once.

> The order matters. If the minifier came first, both destinations would
> receive minified contents and only the names would differ.

## Renaming

`plugins.Rename` receives the path split into three parts, so appending to
`Basename` leaves the extension where it belongs:

| Field      | `src/js/app.js` with base `src` |
| ---------- | ------------------------------- |
| `Dirname`  | `js`                            |
| `Basename` | `app`                           |
| `Extname`  | `.js`                           |

Setting `p.Basename += ".min"` yields `js/app.min.js`. Setting
`p.Extname = ".min.js"` would give the same result here, but breaks on a file
named `app.test.js`, where `Extname` is only `.js`.

## Sourcemaps for the minified copy only

Initialise sourcemaps before the minifier and write them after the rename, so
the `.map` file lands beside `app.min.js`:

```go
return gulp.Src([]string{"src/**/*.js"}).
	Pipe(gulp.Dest("dist")).
	Pipe(plugins.SourcemapsInit()).
	Pipe(minify()).
	Pipe(plugins.Rename(func(p *plugins.Path) { p.Basename += ".min" })).
	Pipe(plugins.SourcemapsWrite(".")).
	Pipe(gulp.Dest("dist")).
	Run(ctx)
```

`SourcemapsWrite(".")` emits `app.min.js.map` as a second file immediately after
its owner, so the `Dest()` below writes both. Passing `""` instead appends an
inline `data:` comment and emits no extra file.

## Skipping files that are already minified

A file that arrives already minified should not be minified again. `plugins.If`
routes it around the minifier:

```go
alreadyMinified := func(f *gulp.File) bool {
	return strings.HasSuffix(f.Basename(), ".min.js")
}

return gulp.Src([]string{"src/**/*.js"}).
	Pipe(plugins.If(alreadyMinified, nil, minify())).
	Pipe(gulp.Dest("dist")).
	Run(ctx)
```

A `nil` branch passes files through untouched. Note that `If` runs its two
branches concurrently, so output order is not input order — which does not
matter here, because `Dest()` writes each file independently.

## Related

- [Bundling with esbuild][bundling] — one bundler invocation instead of a
  per-file minifier
- [Using plugins][plugins] — more on `plugins.Exec` and `plugins.Rename`

[bundling]: bundling-with-esbuild.md
[plugins]: ../getting-started/7-using-plugins.md
