<!--
name: automate-releases
title: Automate releases
-->

# Automate releases

Upstream's recipe reaches for `gulp-conventional-changelog`, `gulp-bump` and
`gulp-git` to rewrite `package.json`, regenerate `CHANGELOG.md` and tag the
result. A Go module has no version field to bump — the version *is* the git
tag — so the recipe shrinks to generating a changelog, tagging, and pushing.

## Reading the current version

`git describe` is the whole lookup. There is no file to parse and therefore no
file that can disagree with the tag.

```go
func currentVersion(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "git", "describe", "--tags", "--abbrev=0").Output()
	if err != nil {
		// No tags yet. Everything before the first release is v0.0.0.
		return "v0.0.0", nil
	}
	return strings.TrimSpace(string(out)), nil
}
```

## Choosing the next version

Take the bump from the command line so the decision stays with the person
running the release, and let the task compute the number.

```go
func nextVersion(current, bump string) (string, error) {
	var major, minor, patch int
	if _, err := fmt.Sscanf(current, "v%d.%d.%d", &major, &minor, &patch); err != nil {
		return "", fmt.Errorf("parse %s: %w", current, err)
	}

	switch bump {
	case "major":
		major, minor, patch = major+1, 0, 0
	case "minor":
		minor, patch = minor+1, 0
	case "patch":
		patch++
	default:
		return "", fmt.Errorf("unknown bump %q, want major, minor or patch", bump)
	}

	return fmt.Sprintf("v%d.%d.%d", major, minor, patch), nil
}
```

`gulp release --bump=minor` reaches the task as an unrecognised flag, which the
CLI leaves alone. See [Pass arguments from the CLI][args] for the lookup helper.

## Generating the changelog

`git log` with a format string produces the same list `conventional-changelog`
builds, without the dependency.

```go
func changelog(ctx context.Context, from, to string) ([]byte, error) {
	span := from + "..HEAD"
	if from == "v0.0.0" {
		span = "HEAD" // First release: everything.
	}

	out, err := exec.CommandContext(ctx, "git", "log", span, "--no-merges", "--pretty=format:- %s (%h)").Output()
	if err != nil {
		return nil, fmt.Errorf("git log %s: %w", span, err)
	}

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "## %s - %s\n\n", to, time.Now().Format(time.DateOnly))
	buf.Write(out)
	buf.WriteString("\n\n")
	return buf.Bytes(), nil
}
```

Prepending it to the existing file is a read, a concatenation and a write —
`Src`/`Dest` would only get in the way for a single known path.

```go
func prependChangelog(entry []byte) error {
	existing, err := os.ReadFile("CHANGELOG.md")
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	header, body, found := bytes.Cut(existing, []byte("\n\n"))
	if !found {
		header, body = []byte("# Changelog"), existing
	}

	var buf bytes.Buffer
	buf.Write(header)
	buf.WriteString("\n\n")
	buf.Write(entry)
	buf.Write(body)

	return os.WriteFile("CHANGELOG.md", buf.Bytes(), 0o644)
}
```

## Refusing to release a dirty tree

Check this first. A release built from uncommitted work is a release nobody can
reproduce.

```go
func requireCleanTree(ctx context.Context) error {
	out, err := exec.CommandContext(ctx, "git", "status", "--porcelain").Output()
	if err != nil {
		return fmt.Errorf("git status: %w", err)
	}
	if len(bytes.TrimSpace(out)) > 0 {
		return errors.New("working tree has uncommitted changes")
	}
	return nil
}
```

## The task

```go
func release(ctx context.Context) error {
	if err := requireCleanTree(ctx); err != nil {
		return err
	}

	current, err := currentVersion(ctx)
	if err != nil {
		return err
	}

	next, err := nextVersion(current, flagValue("bump", "patch"))
	if err != nil {
		return err
	}

	entry, err := changelog(ctx, current, next)
	if err != nil {
		return err
	}
	if err := prependChangelog(entry); err != nil {
		return err
	}

	for _, args := range [][]string{
		{"add", "CHANGELOG.md"},
		{"commit", "-m", "release " + next},
		{"tag", "-a", next, "-m", next},
	} {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("git %s: %w", args[0], err)
		}
	}

	fmt.Println("tagged", next, "- push with: git push --follow-tags")
	return nil
}

func main() {
	t := gulp.Task("release", release)
	t.Description = "Tag a new release and update the changelog"
	t.Flags = map[string]string{"--bump": "major, minor or patch (default patch)"}

	gulp.Main()
}
```

The push is deliberately left to the operator. Everything before it is local
and reversible with `git tag -d` and `git reset`; the push is not.

> Run the tests before tagging by registering the release task as a series:
> `gulp.TaskRef("release", gulp.Series(gulp.Name("check"), gulp.Anonymous(release)))`.
> A `Series` stops at the first failure, so a failing test never reaches the
> tagging step.

## Publishing

There is nothing to publish. `go install example.com/tool@v1.2.0` resolves
straight from the tag once it is pushed, and the module proxy picks it up on
first request. If the release also ships binaries, add a task that runs
`goreleaser release --clean` after the tag exists.

[args]: pass-arguments-from-cli.md
[cli]: ../CLI.md
[task]: ../api/task.md
