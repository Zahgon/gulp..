<!-- front-matter
id: async-completion
title: Async Completion
hide_title: true
sidebar_label: Async Completion
-->

# Async Completion

gulp has to know when a task is finished, so it can log a duration, decide when
the next task in a series may begin, and record a successful run for
[`LastRun()`][last-run-api].

In JavaScript that question is genuinely hard: a task may return a stream, a
promise, an event emitter, a child process or an observable, or it may take an
error-first callback, and gulp has to sniff which one it got. Go collapses all
six conventions into one signature.

## Signature

```go
func(ctx context.Context) error
```

Returning `nil` means success. Returning an error fails the task, and any
series it belongs to.

```go
func clean(ctx context.Context) error {
	return os.RemoveAll("dist")
}
```

Because the signature is fixed, there is no such thing as forgetting to signal
completion. gulp's most common JavaScript error — *"Did you forget to signal
async completion?"* — cannot happen here.

## Returning a pipeline

A file pipeline finishes when its last stage finishes, so return `Run`:

```go
func styles(ctx context.Context) error {
	return gulp.Src([]string{"src/**/*.css"}).
		Pipe(gulp.Dest("dist")).
		Run(ctx)
}
```

`Run` drains the pipeline, waits for every stage, and returns the first error
any of them produced. If you would rather register a pipeline directly, `AsTask`
gives you the same function:

```go
gulp.Task("styles", gulp.Src([]string{"src/**/*.css"}).
	Pipe(gulp.Dest("dist")).
	AsTask())
```

> Building a pipeline performs no work. Nothing is globbed, opened or written
> until a terminal call such as `Run`, `Each` or `Collect`. That laziness is
> deliberate: it means a pipeline built at registration time still sees the
> files that exist when the task actually runs.

## Handling cancellation

The context is not decoration. gulp cancels it when you press `Ctrl-C`, and
`Parallel` cancels it for the remaining siblings as soon as one of them fails.
A long-running task should watch it:

```go
func compile(ctx context.Context) error {
	for _, unit := range units {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if err := build(unit); err != nil {
			return err
		}
	}
	return nil
}
```

Anything that already accepts a context — `exec.CommandContext`,
`http.NewRequestWithContext`, most database drivers — handles this for free.

## Wrapping other shapes

The `asyncdone` adapters exist for code that does not already have the right
signature. They are in `internal/`, so the exported equivalents live on the
types you already use, but the patterns are worth knowing.

### An external command

```go
func bundle(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "esbuild", "src/app.js", "--outfile=dist/app.js")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
```

`CommandContext` kills the process when the context is cancelled, which is what
makes `Ctrl-C` responsive.

### A callback-based library

```go
func upload(ctx context.Context) error {
	done := make(chan error, 1)
	client.Upload("dist", func(err error) { done <- err })

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
```

The buffered channel matters. If the library calls back after the context is
cancelled, an unbuffered send would leak the goroutine forever.

> A JavaScript task that calls `done()` twice crashes the process. This port
> ignores the second signal rather than panicking.

### Work that has no natural error

```go
func banner(ctx context.Context) error {
	fmt.Println("building")
	return nil
}
```

Synchronous tasks are perfectly legal in Go. JavaScript gulp cannot support
them, because it has no way to tell a synchronous function from an async one
that forgot to signal.

## Errors and the build

A failing task stops its series, is logged in red with the elapsed time, and
makes the process exit non-zero:

```
[13:05:29] 'compile' errored after 812 ms
[13:05:29] exit status 1
```

The task is also **not** recorded as a successful run, so
[`LastRun()`][last-run-api] keeps returning the previous success and the next
incremental build will pick the failed files up again.

To let the remaining tasks run anyway, use `--continue` on the command line, or
`SettleSeries`/`SettleParallel` in code. Both collect every error instead of
stopping at the first, and neither cancels its siblings.

## Next

[Working with Files][files]

[last-run-api]: ../api/last-run.md
[files]: 5-working-with-files.md
