<!-- front-matter
id: cron-task
title: Running a task on a schedule
hide_title: true
sidebar_label: Running a task on a schedule
-->

# Running a task on a schedule

Sometimes a task should run every few minutes rather than once — regenerating a
search index, polling an API, refreshing a mirror.

## A ticker

`time.Ticker` is all you need. The loop runs until the context is cancelled,
which happens when you press Ctrl-C.

```go
package main

import (
	"context"
	"log"
	"time"

	gulp "github.com/gulpjs/gulp-go"
)

func refresh(ctx context.Context) error {
	log.Println("refreshing the index")
	return nil
}

func schedule(ctx context.Context) error {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		if err := refresh(ctx); err != nil {
			log.Printf("refresh failed: %v", err)
		}

		select {
		case <-ticker.C:
		case <-ctx.Done():
			return nil
		}
	}
}

func main() {
	gulp.Task("refresh", refresh)
	gulp.Task("schedule", schedule)
	gulp.Main()
}
```

Two details are worth calling out.

The work happens *before* the first `<-ticker.C`, so `gulp schedule` does
something immediately instead of sitting idle for five minutes.

A failing run is logged and the loop continues. Returning the error instead
would end the schedule on the first transient failure, which is rarely what a
long-running job wants. If you do want it to stop, return the error — the CLI
will report it and exit non-zero, exactly as for any other task.

## Not overlapping

A ticker fires on a fixed interval regardless of how long the work took. If
`refresh` occasionally takes six minutes, a five-minute ticker will queue up
behind it.

The loop above cannot overlap, because it only waits on the ticker after the
work returns — a tick that arrives during the work is delivered immediately
afterwards, and any further ticks are dropped, since `time.Ticker` buffers
exactly one. That is usually the behaviour you want: catch up once, never pile
up.

If you would rather measure the gap *between* runs, use a timer and reset it
after each pass:

```go
func schedule(ctx context.Context) error {
	const gap = 5 * time.Minute

	for {
		start := time.Now()
		if err := refresh(ctx); err != nil {
			log.Printf("refresh failed: %v", err)
		}
		log.Printf("took %s", time.Since(start))

		timer := time.NewTimer(gap)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return nil
		}
	}
}
```

## Cancellation

`gulp.Main` installs a signal handler, so Ctrl-C cancels the context every task
receives. A scheduled task must select on `ctx.Done()` — otherwise the first
Ctrl-C is ignored and the process only dies on the second, harsher signal.

The same applies inside `refresh`: pass `ctx` down to `exec.CommandContext`,
`http.NewRequestWithContext` and any pipeline you run, so a long HTTP call is
interrupted rather than waited out.

## Combining with a watcher

A watcher is already a long-running task, so a schedule and a watch can share
one process with `gulp.Parallel`:

```go
func main() {
	gulp.Task("refresh", refresh)
	gulp.Task("schedule", schedule)
	gulp.Task("watch", watchFiles)

	gulp.TaskRef("dev", gulp.Parallel(gulp.Names("schedule", "watch")...))
	gulp.Main()
}
```

Both run until the context is cancelled. Remember that `Parallel` cancels its
siblings when one returns an error, so a fatal error in the schedule also stops
the watcher — usually the right outcome for a development process, and
avoidable with `gulp.SettleParallel` if it is not.

## Use cron for real scheduling

A ticker keeps a process alive. If the machine reboots, the schedule stops.

For anything that must survive a reboot, register a single-shot task and let the
operating system schedule it:

```
*/5 * * * * cd /srv/site && /usr/local/bin/gulp refresh
```

or, with systemd, a `.timer` unit invoking `gulp refresh`. The gulpfile stays
simple, the scheduling is owned by the thing that is good at scheduling, and
failures are reported wherever the rest of your system reports them.

[parallel]: ../api/parallel.md
[watching-files]: ../getting-started/8-watching-files.md
[async-completion]: ../getting-started/4-async-completion.md
