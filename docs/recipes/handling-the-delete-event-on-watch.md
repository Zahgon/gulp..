<!--
name: handling-the-delete-event-on-watch
title: Handling the delete event on watch
-->

# Handling the delete event on watch

A rebuild copies files into the destination, but it never removes anything. Delete
`src/js/old.js` and `dist/js/old.js` stays behind until the next clean.

Register a listener for the unlink events and mirror the deletion.

## Deleting the mirrored file

```go
package main

import (
	"context"
	"os"
	"path/filepath"

	gulp "github.com/gulpjs/gulp-go"
	"github.com/gulpjs/gulp-go/watch"
)

func watchScripts(ctx context.Context) error {
	w, err := gulp.Watch([]string{"src/**/*.js"}, gulp.WatchOptions{}, gulp.Series(gulp.Name("scripts")).Fn)
	if err != nil {
		return err
	}
	defer w.Close()

	w.On(watch.EventUnlink, func(path string) {
		mirrored := filepath.Join("dist", mustRel("src", path))
		if err := os.Remove(mirrored); err != nil && !os.IsNotExist(err) {
			gulp.Default.Log(err)
		}
	})

	<-ctx.Done()
	return nil
}
```

The listener receives the path that changed, relative to the watcher's `Cwd`. The
mirrored path is that same path below the destination, which is exactly the mapping
`Dest()` used on the way in — see [maintain directory structure][structure] for why
the base is what decides it.

> `gulp.Default.Log` is not a real method. Use whatever your gulpfile already uses
> for output; the point is that a listener cannot return an error, so it has to
> handle its own.

## Removing empty directories

Deleting the file leaves the directory. If that matters:

```go
w.On(watch.EventUnlink, func(path string) {
	mirrored := filepath.Join("dist", mustRel("src", path))
	if err := os.Remove(mirrored); err != nil && !os.IsNotExist(err) {
		return
	}
	// Remove fails with ENOTEMPTY on a directory that still has entries,
	// which is exactly the condition we want to stop at.
	_ = os.Remove(filepath.Dir(mirrored))
})
```

`os.Remove` on a non-empty directory fails, so this prunes one level and stops. Walk
upward in a loop if you want it to prune further.

## Watching for directory removals too

By default a watcher reports `add`, `change` and `unlink` — files only. Ask for the
directory events explicitly:

```go
opts := gulp.WatchOptions{
	Events: []watch.EventKind{
		watch.EventAdd,
		watch.EventChange,
		watch.EventUnlink,
		watch.EventUnlinkDir,
	},
}
```

Then `w.On(watch.EventUnlinkDir, ...)` fires when a source directory disappears, and
`os.RemoveAll` on the mirrored path is the right response.

## Why the task itself cannot do this

The task runs `gulp.Src(...)`, which only ever sees files that exist. A deleted file
is absent from the glob results, so nothing in the pipeline can know it used to be
there. The same is true of `SrcOptions.Since`: it filters by modification time and a
deleted file has none. Deletion has to be handled by the watcher, not the build.

The alternative is to clean the destination before every rebuild, which is correct
but slow — see [delete files and folders][delete].

[structure]: maintain-directory-structure-while-globbing.md
[delete]: delete-files-folder.md
