<!-- front-matter
name: recommended-modules
-->

# Recommended modules

The JavaScript version of this page lists the npm packages a plugin author reaches for. Most of them exist to fill gaps that the Go standard library already covers, so this page is mostly a list of things you do not need to install.

## From this module

| Package | Use |
|:--------|:----|
| [`pipeline`][pipeline] | `Map`, `Filter`, `Tap`, `Flush`, `Send`, `Recv`, `From` — everything needed to build a transform |
| [`vinyl`][vinyl] | The file type, `Buffer`, `NewStream`, `Stat` |
| [`plugins`][plugins] | `Error` for typed failures, `Exec` for wrapping a CLI tool, `Path` for renaming |

## From the standard library

| Instead of | Use |
|:-----------|:----|
| `through2`, `readable-stream` | `pipeline.Map` — or `pipeline.TransformFunc` when the file count changes |
| `plugin-error` | `plugins.Error`, or your own type with `Unwrap` |
| `vinyl-file` | `vfs.Src` with a single non-magic path |
| `replace-ext` | `f.SetExtname(".css")` |
| `fancy-log` | `log/slog`, or `gulp.On` to hook the task event stream |
| `minimist`, `yargs` | `flag`, or the unrecognised flags the CLI leaves in place |
| `lodash.*` | `slices`, `maps`, `strings`, `cmp` |
| `async`, `bluebird` | goroutines, `context`, `errgroup` |
| `rimraf` | `os.RemoveAll` |
| `mkdirp` | `os.MkdirAll` |
| `glob`, `micromatch` | `gulp.Src` — or [`doublestar`][doublestar] directly for a bare match |
| `chokidar` | `gulp.Watch` |
| `chalk` | ANSI escape codes, or the CLI's own palette which already honours `NO_COLOR` |
| `iconv-lite` | [`golang.org/x/text/encoding`][xtext] |

## Third-party packages worth knowing

| Package | Use |
|:--------|:----|
| [`golang.org/x/sync/errgroup`][errgroup] | Concurrency inside a single task, with context cancellation |
| [`golang.org/x/text`][xtext] | Character encodings, Unicode normalisation, display width |
| [`github.com/bmatcuk/doublestar/v4`][doublestar] | Glob matching, if you need it outside a pipeline |
| [`github.com/fsnotify/fsnotify`][fsnotify] | Filesystem notifications, if you need them outside `gulp.Watch` |

Both of the last two are already dependencies of this module, so using them adds nothing to your build.

## Build tools

The most useful "modules" for a plugin author are not Go packages at all. `plugins.Exec` turns any of these into a transform in a few lines, and wrapping one is nearly always better than reimplementing it:

| Tool | Use |
|:-----|:----|
| [`esbuild`][esbuild] | Bundling, minification, TypeScript and JSX |
| [`sass`][sass] | Stylesheets |
| [`terser`][terser] | JavaScript minification |
| [`swc`][swc] | Transpilation |
| [`oxipng`][oxipng], [`sharp`][sharp] | Images |

esbuild also ships [a Go API][esbuild-go], which is worth preferring over the binary when it fits — one fewer process per file.

---

Back to [Writing a plugin][writing-a-plugin].

[pipeline]: https://pkg.go.dev/github.com/gulpjs/gulp-go/pipeline
[vinyl]: https://pkg.go.dev/github.com/gulpjs/gulp-go/vinyl
[plugins]: https://pkg.go.dev/github.com/gulpjs/gulp-go/plugins
[doublestar]: https://github.com/bmatcuk/doublestar
[fsnotify]: https://github.com/fsnotify/fsnotify
[errgroup]: https://pkg.go.dev/golang.org/x/sync/errgroup
[xtext]: https://pkg.go.dev/golang.org/x/text
[esbuild]: https://esbuild.github.io/
[esbuild-go]: https://pkg.go.dev/github.com/evanw/esbuild/pkg/api
[sass]: https://sass-lang.com/
[terser]: https://terser.org/
[swc]: https://swc.rs/
[oxipng]: https://github.com/shssoichiro/oxipng
[sharp]: https://sharp.pixelplumbing.com/
[writing-a-plugin]: README.md
