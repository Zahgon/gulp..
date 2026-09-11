<!-- front-matter
name: Delete files and folders
-->

# Delete files and folders

There is no plugin for this, and there does not need to be. A task is an
ordinary Go function, so deleting a directory is one call to `os.RemoveAll`.

```go
func clean(ctx context.Context) error {
	return os.RemoveAll("dist")
}
```

`os.RemoveAll` returns `nil` when the path is already gone, so a clean task is
safe to run on a fresh checkout.

## Deleting a glob

To delete a matched set rather than a whole tree, run a pipeline that reads
nothing and removes each path it is given:

```go
func cleanMaps(ctx context.Context) error {
	return gulp.Src([]string{"dist/**/*.map"}, gulp.SrcOptions{
		Read:       gulp.Value(false),
		AllowEmpty: true,
	}).Pipe(pipeline.Tap(func(f *gulp.File) error {
		return os.Remove(f.Path())
	})).Run(ctx)
}
```

`Read: gulp.Value(false)` skips reading contents, which matters when the files
are large. `AllowEmpty: true` stops the pipeline erroring when the glob matches
nothing, which is the normal case on a clean tree.

## Do not delete outside the project

`gulp.Src` resolves globs against the working directory, and a stray `../`
in a glob will happily match files above it. If the paths come from anywhere
but a literal in your gulpfile, check them first:

```go
root, err := os.Getwd()
if err != nil {
	return err
}
if rel, err := filepath.Rel(root, f.Path()); err != nil || strings.HasPrefix(rel, "..") {
	return fmt.Errorf("refusing to delete outside the project: %s", f.Path())
}
```

## Ordering

Deletion almost always belongs first in a series, so that the rest of the build
writes into a clean tree:

```go
gulp.TaskRef("build", gulp.Series(gulp.Names("clean", "styles", "scripts")...))
```

[src]: ../api/src.md
[series]: ../api/series.md
