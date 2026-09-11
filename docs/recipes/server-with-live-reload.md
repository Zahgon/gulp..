<!--
name: server-with-live-reload
title: Development server with live reload
hide_title: true
sidebar_label: Development server with live reload
-->

# Development server with live reload

Upstream gulp has two recipes for this, one for BrowserSync and one for
gulp-livereload. Both exist because Node had no comfortable way to serve a
directory and push a reload signal from the same process. Go's standard library
has both, so the recipe is a task rather than a dependency.

The pieces are `http.FileServer` for the static files, an
[SSE][sse] endpoint for the signal, and `gulp.Watch` to fire it.

## A server task

```go
package main

import (
	"context"
	"log"
	"net/http"

	gulp "github.com/gulpjs/gulp-go"
)

func serve(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir("dist")))

	srv := &http.Server{Addr: ":3000", Handler: mux}

	go func() {
		<-ctx.Done()
		srv.Close()
	}()

	log.Println("listening on http://localhost:3000")

	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

func main() {
	gulp.Task("serve", serve)
	gulp.Main()
}
```

`ListenAndServe` blocks until the server is closed, which is what keeps
`gulp serve` running. The goroutine watching `ctx.Done()` is what makes Ctrl-C
work: `gulp.Main` cancels the context on `SIGINT`, `srv.Close()` unblocks
`ListenAndServe`, and it returns `http.ErrServerClosed`, which is a normal
shutdown rather than a failure.

## Pushing reloads

A reload signal needs one endpoint the browser subscribes to and one function
the build calls. Server-sent events are the smallest thing that works — no
websocket library, no client-side dependency.

```go
type reloader struct {
	mu      sync.Mutex
	clients map[chan struct{}]struct{}
}

func newReloader() *reloader {
	return &reloader{clients: make(map[chan struct{}]struct{})}
}

// Notify wakes every connected browser. The channels are buffered, so a client
// that is between requests is skipped rather than blocking the build.
func (r *reloader) Notify() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for ch := range r.clients {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (r *reloader) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	ch := make(chan struct{}, 1)

	r.mu.Lock()
	r.clients[ch] = struct{}{}
	r.mu.Unlock()

	defer func() {
		r.mu.Lock()
		delete(r.clients, ch)
		r.mu.Unlock()
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher.Flush()

	for {
		select {
		case <-req.Context().Done():
			return
		case <-ch:
			fmt.Fprint(w, "data: reload\n\n")
			flusher.Flush()
		}
	}
}
```

The `default` branch in `Notify` is the important line. Without it a browser tab
that has gone away but not yet been cleaned up would block the build until its
request context expired.

Register it alongside the file server:

```go
mux.Handle("/livereload", reload)
```

and subscribe from the page:

```html
<script>
  new EventSource('/livereload').onmessage = () => location.reload()
</script>
```

## Wiring the watcher

```go
func dev(ctx context.Context) error {
	reload := newReloader()

	w, err := gulp.Watch([]string{"src/**/*"}, gulp.WatchOptions{}, func(ctx context.Context) error {
		if err := build(ctx); err != nil {
			return err
		}
		reload.Notify()
		return nil
	})
	if err != nil {
		return err
	}
	defer w.Close()

	return serve(ctx, reload)
}
```

`Notify` runs only after `build` succeeds, so a compile error leaves the last
working page on screen instead of reloading into a broken one. The watcher's
`Delay` and `Queue` defaults mean a burst of saves produces one rebuild and one
reload rather than one of each per file.

> A failing build logs in red and the watcher keeps going. That is deliberate —
> a development server that exits on the first syntax error is worse than one
> that tells you about it.

## Injecting CSS without a reload

Reloading the page loses scroll position and form state, which is why
BrowserSync special-cases stylesheets. The same trick works here: send the
changed path instead of a bare signal, and let the client decide.

```go
w.On(watch.EventChange, func(path string) {
	if filepath.Ext(path) == ".css" {
		reload.NotifyPath(path)
		return
	}
	reload.Notify()
})
```

```js
new EventSource('/livereload').onmessage = (e) => {
  if (!e.data.endsWith('.css')) return location.reload()

  for (const link of document.querySelectorAll('link[rel=stylesheet]')) {
    const url = new URL(link.href)
    url.searchParams.set('v', Date.now())
    link.href = url.toString()
  }
}
```

Note that this uses `w.On` rather than the watcher's task, because the task is
given no path — it only knows that something changed. Use the listener for
routing decisions and the task for the build itself.

## Running both

```go
gulp.Task("build", build)
gulp.Task("serve", serve)
gulp.Task("watch", watchFiles)

gulp.TaskRef("dev", gulp.Series(
	gulp.Name("build"),
	gulp.Parallel(gulp.Names("serve", "watch")...),
))
```

`serve` and `watch` both run until cancelled, so `gulp.Parallel` is what holds
the process open. Remember that a failure in either cancels the other; use
`gulp.SettleParallel` if you would rather the server survive a watcher error.

[sse]: https://developer.mozilla.org/en-US/docs/Web/API/Server-sent_events
