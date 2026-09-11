<!--
name: go-test-runner
title: Running Go tests from a task
-->

# Running Go tests from a task

The upstream recipe wires the Mocha runner into a stream. Go has a test runner
built into the toolchain, so the task is a subprocess call.

## The simplest version

```go
func test(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "go", "test", "./...")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func main() {
	gulp.Task("test", test)
	gulp.Main()
}
```

Wiring the child's output straight to `os.Stdout` is what makes the pass/fail
lines appear as they happen instead of arriving in one block at the end. The
exit status becomes the error, so a failing suite fails the task, and
`gulp.Main` returns 1.

`exec.CommandContext` kills the child when the context is cancelled, so Ctrl-C
stops the tests rather than orphaning them.

## Race detector and coverage

```go
func test(ctx context.Context) error {
	return run(ctx, "go", "test", "-race", "-covermode=atomic",
		"-coverprofile=coverage.out", "./...")
}

func coverage(ctx context.Context) error {
	return run(ctx, "go", "tool", "cover", "-html=coverage.out", "-o", "coverage.html")
}

func run(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func main() {
	gulp.Task("test", test)
	gulp.Task("coverage", coverage)
	gulp.TaskRef("cover", gulp.Series(gulp.Names("test", "coverage")...))
	gulp.Main()
}
```

`coverage` needs the profile `test` writes, so the two belong in a `Series`.
Registering them separately as well means `gulp test` still works on its own.

## Watching

```go
func watchTests(ctx context.Context) error {
	w, err := gulp.Watch([]string{"**/*.go"}, gulp.WatchOptions{
		Ignored: []string{"**/testdata/**"},
	}, test)
	if err != nil {
		return err
	}
	defer w.Close()

	<-ctx.Done()
	return nil
}
```

The watcher's own defaults do most of the work here. `Delay` collapses the
burst of writes a formatter-on-save produces into one run, and `Queue` means a
save during a long test run schedules exactly one more run rather than one per
save.

Failing tests are expected while watching, so `test` should not stop the
watcher. It does not: a task error is reported and the watcher keeps running.

## Only the packages that changed

`go test ./...` is usually fast enough. When it is not, the watcher can tell you
which package to run:

```go
w, err := gulp.Watch([]string{"**/*.go"}, gulp.WatchOptions{}, nil)
if err != nil {
	return err
}
defer w.Close()

w.On(watch.EventChange, func(rel string) {
	pkg := "./" + filepath.Dir(rel)
	if err := run(ctx, "go", "test", pkg); err != nil {
		log.Printf("test %s: %v", pkg, err)
	}
})
```

Passing `nil` as the task and doing the work in a listener trades the delay and
queue behaviour for precision. That is a real trade: a formatter-on-save can
fire this several times per save. Prefer the task form unless the full suite is
genuinely too slow.

> A listener cannot return an error, so it has to log its own failures.

## Other checks

The same shape covers everything else the toolchain provides:

```go
gulp.Task("vet", func(ctx context.Context) error {
	return run(ctx, "go", "vet", "./...")
})
gulp.Task("lint", func(ctx context.Context) error {
	return run(ctx, "golangci-lint", "run")
})
gulp.TaskRef("check", gulp.Series(gulp.Names("vet", "lint", "test")...))
```

`Series` is right for `check`: running `test` when `vet` has already failed
wastes time. Use `gulp.Parallel` — or `gulp check --continue` — when you would
rather see every failure at once.

[watching]: ../getting-started/8-watching-files.md
[exec]: https://pkg.go.dev/github.com/gulpjs/gulp-go/plugins#Exec
