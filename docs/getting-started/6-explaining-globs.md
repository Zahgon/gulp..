<!-- front-matter
id: explaining-globs
title: Explaining Globs
hide_title: true
sidebar_label: Explaining Globs
-->

# Explaining Globs

A glob is a string of literal and wildcard characters used to match filepaths. Globbing is the act of locating files on a filesystem using one or more globs.

`Src()` takes a `[]string`, and every entry is a glob. Even a single path is a glob — one that happens to contain no wildcards.

```go
gulp.Src([]string{"src/**/*.js"})
```

Most of what follows is identical to gulp in JavaScript, because the matching rules come from the same place: the `**`, `*`, `[]`, `{}` and `!(...)` syntax that `micromatch` implements. Where the Go port differs, the difference is called out.

## Segments and separators

A **segment** is everything between two separators. In a glob, always use `/` as the separator, even on Windows.

```go
// Correct, on every platform.
gulp.Src([]string{"src/js/app.js"})

// Wrong. Backslash is the escape character in a glob, not a separator.
gulp.Src([]string{`src\js\app.js`})
```

Filepaths that come back out of the stream use the platform separator, because they are real paths. Only the glob itself is fixed to `/`.

## Special characters

| Character | Matches |
|:---------:|:--------|
| `*` | Zero or more characters within **one** segment |
| `**` | Zero or more characters across **any number of** segments |
| `[abc]` | One character from the set |
| `[a-z]` | One character from the range |
| `{a,b}` | Either alternative |
| `!` | Negates the entire glob (only as the first character) |
| `!(a\|b)` | Anything except the alternatives |
| `?(a\|b)` `*(a\|b)` `+(a\|b)` `@(a\|b)` | Extglob quantifiers |
| `\` | Escapes the next character |

### One segment versus many

`*` stops at a separator; `**` does not.

```go
// scripts/app.js       matches
// scripts/lib/util.js  does NOT match
gulp.Src([]string{"scripts/*.js"})

// scripts/app.js       matches
// scripts/lib/util.js  matches
gulp.Src([]string{"scripts/**/*.js"})
```

Always give `**` its own segment. `scripts/**.js` is legal but means something narrower than most people expect, and it changes the base — see below.

### `?` is not a single-character wildcard

This one surprises people, and it is worth stating plainly because it is easy to assume otherwise:

```go
// Matches a file literally named "?foo.js". It is NOT "any character
// followed by foo.js".
gulp.Src([]string{"src/?foo.js"})
```

`?` is only special as an **extglob quantifier**, where it follows `]`, `.`, `+` or `)`:

```go
// Zero or one "a" or "b".
gulp.Src([]string{"src/?(a|b).js"})
```

This is inherited from `is-glob`, the same module gulp uses, and the port reproduces its rules character for character. Getting this wrong is not cosmetic: it changes which part of the glob is treated as the base, and therefore where `Dest()` writes. See [Explaining the base](#the-base) below.

## Negation

A glob beginning with `!` removes matches that earlier globs added.

```go
gulp.Src([]string{"scripts/**/*.js", "!scripts/vendor/**"})
```

Two rules matter:

* Negation applies to the whole set, not just the glob before it. Ordering is irrelevant — a file excluded by any negative glob is excluded, however many positive globs matched it.
* A set of only negative globs matches nothing, and is reported as `Invalid glob argument`. Start with something to subtract from.

> [!TIP]
> Prefer the `Ignore` option when the exclusion is a property of the search rather than of one glob. `SrcOptions{Ignore: []string{"**/node_modules/**"}}` reads better than threading `!` globs through every call.

To match a file whose name genuinely starts with `!`, escape it: `\!important.css`.

## Dotfiles

By default a glob will not match a file or directory whose name begins with `.`, unless the glob names it explicitly.

```go
// Does not match .eslintrc.
gulp.Src([]string{"config/*"})

// Matches it.
gulp.Src([]string{"config/.*"})

// Matches everything, dotted or not.
gulp.Src([]string{"config/*"}, gulp.SrcOptions{Dot: true})
```

<a name="the-base"></a>
## The base

Every file that comes out of `Src()` carries a **base**: the leading portion of the glob up to the first segment containing a wildcard.

| Glob | Base |
|:-----|:-----|
| `src/js/app.js` | `src/js/` |
| `src/js/*.js` | `src/js/` |
| `src/**/*.js` | `src/` |
| `src/js/**.js` | `src/js/` |
| `src/?foo/*.js` | `src/?foo/` |

The base is what `Dest()` strips before joining onto the output directory, so it is the single thing that decides the shape of your output tree:

```go
// src/js/app.js -> dist/app.js
gulp.Src([]string{"src/js/*.js"}).Pipe(gulp.Dest("dist"))

// src/js/app.js -> dist/js/app.js
gulp.Src([]string{"src/**/*.js"}).Pipe(gulp.Dest("dist"))
```

Set it explicitly when the glob does not imply the structure you want:

```go
gulp.Src(
	[]string{"src/js/**/*.js", "src/css/**/*.css"},
	gulp.SrcOptions{Base: "src"},
)
```

`CwdBase: true` is shorthand for setting the base to the working directory.

## Order

Files are emitted in the order the globs are given, and sorted within each glob. That makes concatenation predictable:

```go
gulp.Src([]string{
	"src/js/banner.js",
	"src/js/**/*.js",
}).Pipe(plugins.Concat("bundle.js"))
```

Duplicates are removed. A file matched by two globs is emitted once, at its first position. Change the key with `UniqueBy`, or set `UniqueBy: ""` to disable deduplication entirely.

## Single globs that match nothing

A glob with no wildcards names exactly one file, so failing to find it is almost always a mistake. `Src()` reports it:

```
File not found with singular glob: /project/src/missing.js (if this was purposeful, use `allowEmpty` option)
```

```go
gulp.Src([]string{"src/optional.js"}, gulp.SrcOptions{AllowEmpty: true})
```

A glob that *does* contain wildcards and matches nothing is not an error — an empty stream is a perfectly reasonable answer to "every `.ts` file".

## Matching options

`SrcOptions` exposes the matcher's switches directly: `Dot`, `NoBrace`, `NoGlobstar`, `NoExt`, `NoCase`, `MatchBase`. They behave as in gulp, and are documented in full in the [`Src()` reference][src].

[src]: ../api/src.md

## Next

[Using plugins](7-using-plugins.md)
