<p align="center">
  <h1 align="center">gulp-go</h1>
  <p align="center">The streaming build system, ported to Go.</p>
</p>

A faithful Go port of [gulp](https://github.com/gulpjs/gulp) 5.0.1. Same
concepts, same task semantics, same CLI output — with a compiled gulpfile, real
concurrency, and no `node_modules`.

```go
package main

import (
	"context"

	gulp "github.com/gulpjs/gulp-go"
	"github.com/gulpjs/gulp-go/plugins"
)

func styles(ctx context.Context) error {
	return gulp.Src([]string{"src/**/*.css"}).
		Pipe(plugins.Concat("app.css")).
		Pipe(gulp.Dest("dist")).
		Run(ctx)
}

func main() {
	gulp.Task("styles", styles)
	gulp.TaskRef("default", gulp.Series(gulp.Name("styles")))
	gulp.Main()
}
```

```
$ go run .
[13:05:29] Using gulpfile ~/project/gulpfile.go
[13:05:29] Starting 'default'...
[13:05:29] Starting 'styles'...
[13:05:29] Finished 'styles' after 5.69 ms
[13:05:29] Finished 'default' after 5.8 ms
```

## Install

```
go get github.com/gulpjs/gulp-go
```

Optionally install the launcher, which finds `gulpfile.go` and runs it:

```
go install github.com/gulpjs/gulp-go/cmd/gulp@latest
```

You do not need it. `go run .` does the same thing, because your gulpfile *is*
the program.

## Concepts

If you know gulp, you know this library. Files flow through a pipeline as
[vinyl](docs/api/concepts.md) values; `Src` reads them, transforms change them,
`Dest` writes them.

```go
gulp.Src([]string{"src/**/*.js", "!src/vendor/**"}).
	Pipe(plugins.Replace("__VERSION__", version)).
	Pipe(plugins.Concat("bundle.js")).
	Pipe(gulp.Dest("dist")).
	Run(ctx)
```

Nothing touches the filesystem until `Run`, `Collect` or `Each` is called.

### Tasks

A task is a function. Every task receives a context, and cancelling it — with
Ctrl-C, or because a sibling failed — cancels the build.

```go
gulp.Task("clean", func(ctx context.Context) error {
	return os.RemoveAll("dist")
})

gulp.TaskRef("build", gulp.Series(
	gulp.Name("clean"),
	gulp.Parallel(gulp.Names("styles", "scripts")...),
))
```

`Series` and `Parallel` compose; names resolve when the task runs, so order of
registration does not matter.

### Watching

```go
w, err := gulp.Watch([]string{"src/**/*.css"}, gulp.WatchOptions{}, styles)
if err != nil {
	return err
}
defer w.Close()
```

Defaults match gulp: initial events ignored, 200ms debounce, at most one queued
run, `add`/`change`/`unlink`.

### Incremental builds

```go
since, ok, _ := gulp.LastRun(gulp.Name("styles"), 0)
opts := gulp.SrcOptions{}
if ok {
	opts.Since = gulp.Value(since)
}
gulp.Src([]string{"src/**/*.css"}, opts)
```

## Plugins

There is no plugin registry. A plugin is a `pipeline.Transform`, which usually
means a `pipeline.Map`:

```go
func banner(text string) pipeline.Transform {
	return pipeline.Map(func(ctx context.Context, f *vinyl.File) (*vinyl.File, error) {
		body, err := f.Bytes()
		if err != nil {
			return nil, err
		}
		f.Contents = vinyl.Buffer(append([]byte(text), body...))
		return f, nil
	})
}
```

For real tooling, `plugins.Exec` wraps any command-line program:

```go
plugins.Exec(plugins.ExecOptions{
	Name:    "esbuild",
	Command: "esbuild",
	Args:    func(f *vinyl.File) []string { return []string{"--minify", "--loader=js"} },
	Rename:  func(p *plugins.Path) { p.Basename += ".min" },
})
```

Bundled: `Concat`, `Rename`, `Replace`, `Filter`, `If`, `SourcemapsInit`,
`SourcemapsWrite`, `Exec`.

## CLI

`gulp.Main()` gives your gulpfile gulp's command line.

```
gulp [options] tasks

  -h, --help           Show this help.
  -v, --version        Print the global and local gulp versions.
      --preload        Will preload a module before running the gulpfile.
  -f, --gulpfile       Manually set path of gulpfile.
      --cwd            Manually set the CWD.
  -T, --tasks          Print the task dependency tree for the loaded gulpfile.
      --tasks-simple   Print a plaintext list of tasks.
      --tasks-json     Print the task dependency tree, in JSON format.
      --tasks-depth    Specify the depth of the task dependency tree.
      --compact-tasks  Reduce the output of task dependency tree.
      --sort-tasks     Will sort top tasks of task dependency tree.
      --color          Force colors.
      --no-color       Force no colors.
  -S, --silent         Suppress all gulp logging.
      --continue       Continue execution of tasks upon failure.
      --series         Run tasks given on the CLI in series.
  -L, --log-level      Set the loglevel. -L least verbose, -LLLL most.
```

```
$ go run . --tasks
Tasks for ~/project/gulpfile.go
├── clean    Remove the build directory
│   --dry    …Report what would be removed
├── styles   Compile stylesheets
├─┬ build    Build everything
│ └─┬ <series>
│   ├── clean
│   └─┬ <parallel>
│     ├── styles
│     └── scripts
└─┬ default
  └─┬ build
```

Give a task a description and flags so they appear here:

```go
t := gulp.Task("clean", clean)
t.Description = "Remove the build directory"
t.Flags = map[string]string{"--dry": "Report what would be removed"}
```

## Differences from gulp

The full catalogue is in [MIGRATION.md](MIGRATION.md). The four that matter:

- **npm plugins cannot be used.** Use `plugins.Exec` to call the underlying
  tool, or write a transform. This is the real cost of the migration.
- **A task is `func(context.Context) error`**, not one of Node's six async
  conventions. Adapters live in `internal/asyncdone`.
- **`Parallel` cancels siblings on the first error.** JavaScript cannot.
- **`useJunctions` is inert**, because Go cannot select a Windows link type.

## Documentation

- [MIGRATION.md](MIGRATION.md) — what changed and why
- [docs/api](docs/api) — the gulp API reference, applicable as written
- [docs/CLI.md](docs/CLI.md) — the gulp CLI reference

## Tests

```
go test ./...
go test -race ./...
go test -short ./...   # skips subprocess and filesystem-timing tests
```

Every gulp test file has a counterpart, and `test/fixtures/` is copied from the
JavaScript repository unchanged.

## License

MIT, same as gulp. See [LICENSE](LICENSE).
