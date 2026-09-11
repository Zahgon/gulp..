<!-- front-matter
id: why-use-pump
title: Why Use Pump?
hide_title: true
sidebar_label: Why Use Pump?
-->

# Why Use Pump?

In JavaScript gulp this page explains why you should wrap `.pipe()` chains in
the [`pump`][pump] module. **You do not need pump in Go.** This page explains
what pump solved, and why the `pipeline` package already solves it.

## The problem pump solved

Node's `.pipe()` does not forward errors. Given this chain:

```js
gulp.src('*.js')
  .pipe(uglify())
  .pipe(gulp.dest('build'));
```

if `uglify()` emits an error, `.pipe()` does not pass it to `gulp.dest()`, and
it does not tear down the stream it was reading from. The result is the failure
mode every gulp user has met at least once: the build prints an unhandled
`'error'` event, or worse, prints nothing and hangs, because the source stream
is still open and waiting for a consumer that has already given up.

`pump` fixed this by wiring every stream's error and close handlers together, so
one failure destroys the whole chain and reports a single error.

## Why Go does not have the problem

A `Pipeline` is not a chain of independent objects that happen to be connected.
It is a single unit that the runner starts, supervises and tears down together.
Three properties fall out of that.

**Errors propagate.** Each stage runs in its own goroutine. When one returns an
error, the runner cancels a context shared by every other stage and delivers
that error to whoever called the terminal method. There is no path by which an
error is emitted but not observed.

```go
files, err := gulp.Src([]string{"src/**/*.js"}).
    Pipe(minify()).
    Pipe(gulp.Dest("build")).
    Collect(ctx)
```

If `minify()` fails, `err` is non-nil. You cannot forget to handle it — Go's
compiler and vet both complain about a discarded error, whereas an unhandled
`'error'` event is invisible until runtime.

**Errors are attributed.** The runner wraps a stage failure with its position,
so the message reads `pipeline stage 2: ...` rather than leaving you to guess
which link in a ten-stage chain broke.

**Nothing leaks.** Every stage closes its output channel on the way out, and
drains its input afterwards, so a stage that stops early cannot wedge the one
feeding it. Cancelling the context — which the runner does automatically on the
first error, and which you can do yourself — unblocks every stage. A failed
build exits; it does not hang.

A panic is treated the same way as an error. The runner recovers it, converts it
to an error carrying the panic value, and cancels the rest of the pipeline. A
panicking transform takes down the build with a usable message rather than the
whole process.

## What this means in practice

Write the chain directly and check the error:

```go
func scripts(ctx context.Context) error {
	return gulp.Src([]string{"src/**/*.js"}).
		Pipe(plugins.Concat("bundle.js")).
		Pipe(gulp.Dest("build")).
		Run(ctx)
}
```

Returning the error is enough. The CLI logs it in red, marks the task failed,
skips recording it for [`LastRun`][last-run], and exits non-zero.

There is no `pump` equivalent to import, no `.on('error', ...)` to remember, and
no combined-stream helper to reach for. The behaviour pump gave you is the
default.

## Related

- [`pipeline` package reference][pipeline] — the `Transform` contract, and the
  rule that a stage must never close its output channel.
- [Async completion][async] — how task errors reach the CLI.
- [Writing a plugin][plugin] — building transforms that fail cleanly.

[pump]: https://github.com/mafintosh/pump
[pipeline]: https://pkg.go.dev/github.com/gulpjs/gulp-go/pipeline
[async]: ../getting-started/4-async-completion.md
[plugin]: ../writing-a-plugin/README.md
[last-run]: ../api/last-run.md
