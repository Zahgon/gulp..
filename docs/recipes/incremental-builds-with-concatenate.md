<!-- front-matter
name: Incremental builds with concatenation
-->

# Incremental builds with concatenation

Reading only the files that changed is easy. The catch is that concatenation
needs *every* file, not just the changed ones — so a naive incremental build
produces a bundle containing one file.

## The problem

```go
// Wrong: after the first run this concatenates only the changed files.
func scripts(ctx context.Context) error {
	since, ok, _ := gulp.LastRun(gulp.Name("scripts"), 0)
	opts := gulp.SrcOptions{}
	if ok {
		opts.Since = gulp.Value(since)
	}
	return gulp.Src([]string{"src/**/*.js"}, opts).
		Pipe(plugins.Concat("bundle.js")).
		Pipe(gulp.Dest("dist")).
		Run(ctx)
}
```

## Cache the transformed files

Do the expensive per-file work incrementally, then concatenate a cache that
holds every file. The cache survives between runs because the gulpfile process
stays alive while watching.

```go
type cache struct {
	mu    sync.Mutex
	files map[string]*gulp.File
}

func (c *cache) put(f *gulp.File) error {
	rel, err := f.Relative()
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.files[rel] = f.Clone()
	return nil
}

func (c *cache) all() []*gulp.File {
	c.mu.Lock()
	defer c.mu.Unlock()

	keys := slices.Sorted(maps.Keys(c.files))
	out := make([]*gulp.File, 0, len(keys))
	for _, k := range keys {
		out = append(out, c.files[k].Clone())
	}
	return out
}
```

Sorting the keys matters: without it the bundle's contents would depend on Go's
randomised map iteration order, and every run would produce a different file.

```go
var scriptCache = &cache{files: map[string]*gulp.File{}}

func scripts(ctx context.Context) error {
	since, ok, _ := gulp.LastRun(gulp.Name("scripts"), 0)
	opts := gulp.SrcOptions{}
	if ok {
		opts.Since = gulp.Value(since)
	}

	// Transform only what changed, and record the result.
	err := gulp.Src([]string{"src/**/*.js"}, opts).
		Pipe(transpile()).
		Pipe(pipeline.Tap(scriptCache.put)).
		Run(ctx)
	if err != nil {
		return err
	}

	// Concatenate everything seen so far.
	return pipeline.New(pipeline.From(scriptCache.all()...)).
		Pipe(plugins.Concat("bundle.js")).
		Pipe(gulp.Dest("dist")).
		Run(ctx)
}
```

Note the `Clone()` calls on the way in and out of the cache. Without them, a
later stage that replaces `Contents` would mutate the cached file, and the next
bundle would pick up the change.

## Removing deleted files

`Since` never reports a deletion, so watch for `unlink` and drop the entry:

```go
w, err := gulp.Watch([]string{"src/**/*.js"}, gulp.WatchOptions{}, gulp.Name("scripts").Fn)
if err != nil {
	return err
}
defer w.Close()

w.On(watch.EventUnlink, func(path string) {
	scriptCache.mu.Lock()
	delete(scriptCache.files, path)
	scriptCache.mu.Unlock()
})
```

## Simpler: do not bother

Concatenation is fast. Reading a few hundred files and joining them costs a few
milliseconds, and the cache above costs correctness risk. Use `Since` for the
expensive stage — transpiling, minifying, image processing — and read
everything for the cheap one.

[last-run]: ../api/last-run.md
[src]: ../api/src.md
