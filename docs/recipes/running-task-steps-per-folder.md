<!--
name: running-task-steps-per-folder
-->

# Running task steps per folder

Given a source tree with one directory per component:

```
src/
├── admin/
│   ├── index.css
│   └── table.css
├── public/
│   ├── index.css
│   └── hero.css
```

produce `dist/admin.css` and `dist/public.css` — one bundle per folder.

## Discover the folders

```go
func folders(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	return names, nil
}
```

`os.ReadDir` returns entries sorted by filename, so the task list is stable
between runs.

## One pipeline per folder

```go
func styles(ctx context.Context) error {
	names, err := folders("src")
	if err != nil {
		return err
	}

	for _, name := range names {
		err := gulp.Src([]string{filepath.Join("src", name, "*.css")}).
			Pipe(plugins.Concat(name + ".css")).
			Pipe(gulp.Dest("dist")).
			Run(ctx)
		if err != nil {
			return fmt.Errorf("styles %s: %w", name, err)
		}
	}
	return nil
}
```

Wrapping the error with the folder name matters: without it, a failure reports
only `pipeline stage 2: ...` and you have to guess which component broke.

## Concurrently

Each folder is independent, so they can run at once. Use
`golang.org/x/sync/errgroup` so the first failure cancels the rest, which is the
same behaviour `gulp.Parallel` has:

```go
func styles(ctx context.Context) error {
	names, err := folders("src")
	if err != nil {
		return err
	}

	group, ctx := errgroup.WithContext(ctx)
	for _, name := range names {
		group.Go(func() error {
			err := gulp.Src([]string{filepath.Join("src", name, "*.css")}).
				Pipe(plugins.Concat(name + ".css")).
				Pipe(gulp.Dest("dist")).
				Run(ctx)
			if err != nil {
				return fmt.Errorf("styles %s: %w", name, err)
			}
			return nil
		})
	}
	return group.Wait()
}
```

> `group.Go` captures `name` correctly because Go 1.22 gives each loop iteration
> its own variable. On older versions this silently built every bundle from the
> last folder.

## As separate registered tasks

If each folder should appear in `gulp --tasks` and be runnable on its own,
register them and compose:

```go
func registerStyles() error {
	names, err := folders("src")
	if err != nil {
		return err
	}

	refs := make([]gulp.Ref, 0, len(names))
	for _, name := range names {
		task := gulp.Task("styles:"+name, buildFolder(name))
		task.Description = "Bundle src/" + name
		refs = append(refs, task)
	}

	_, err = gulp.TaskRef("styles", gulp.Parallel(refs...))
	return err
}

func buildFolder(name string) gulp.TaskFunc {
	return func(ctx context.Context) error {
		return gulp.Src([]string{filepath.Join("src", name, "*.css")}).
			Pipe(plugins.Concat(name + ".css")).
			Pipe(gulp.Dest("dist")).
			Run(ctx)
	}
}
```

Note that registration reads the directory at startup, so a folder added later
needs a restart. That is the trade for having the tasks listed.

## Related

- [Creating tasks][tasks] — registering tasks from a loop
- [Using multiple sources in one task][multiple] — the opposite problem

[tasks]: ../getting-started/3-creating-tasks.md
[multiple]: using-multiple-sources-in-one-task.md
