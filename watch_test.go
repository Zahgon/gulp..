package gulp_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	gulp "github.com/gulpjs/gulp-go"
	"github.com/gulpjs/gulp-go/undertaker"
)

// The three tests below port the cases in test/watch.js that have no literal
// Go spelling. JavaScript destructures the module and passes strings where a
// function is expected; Go takes the exports as values and refuses a string at
// compile time, so the surviving runtime failure is an unresolvable task name.

func TestWatchWorksWithDestructuring(t *testing.T) {
	watch, parallel, fn := gulp.Watch, gulp.Parallel, gulp.Fn

	dir := t.TempDir()
	sourceFile(t, dir, "a.txt", "one")

	ran := make(chan string, 2)
	task := parallel(
		fn("one", func(context.Context) error { ran <- "one"; return nil }),
		fn("two", func(context.Context) error { ran <- "two"; return nil }),
	)

	w, err := watch([]string{filepath.Join(dir, "*.txt")}, gulp.WatchOptions{}, task.Fn)
	if err != nil {
		t.Fatalf("watch: %v", err)
	}
	defer w.Close()

	sourceFile(t, dir, "a.txt", "two")

	seen := map[string]bool{}
	for len(seen) < 2 {
		select {
		case name := <-ran:
			seen[name] = true
		case <-time.After(10 * time.Second):
			t.Fatalf("only %v ran", seen)
		}
	}
}

func TestWatchThrowsAnErrorWhenThePassedStringParameterIsNotAFunction(t *testing.T) {
	assertWatchTaskIsUndefined(t, gulp.Series(gulp.Name("task1")), "task1")
}

func TestWatchThrowsAnErrorWhenThePassedArrayParameterIsNotAFunction(t *testing.T) {
	assertWatchTaskIsUndefined(t, gulp.Series(gulp.Names("task1", "task2")...), "task1")
}

func assertWatchTaskIsUndefined(t *testing.T, task *undertaker.Task, want string) {
	t.Helper()
	dir := t.TempDir()
	sourceFile(t, dir, "a.txt", "one")

	w, err := gulp.Watch([]string{filepath.Join(dir, "*.txt")}, gulp.WatchOptions{}, task.Fn)
	if err != nil {
		t.Fatalf("watch: %v", err)
	}
	defer w.Close()

	err = task.Fn(context.Background())
	var undefined *undertaker.UndefinedTaskError
	if !errors.As(err, &undefined) {
		t.Fatalf("error = %v, want an UndefinedTaskError", err)
	}
	if undefined.Name != want {
		t.Errorf("name = %q, want %q", undefined.Name, want)
	}
}
