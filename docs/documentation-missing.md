<!--
name: documentation-missing
title: Documentation Missing
hide_title: true
sidebar_label: Documentation Missing
-->

# Documentation Missing

You followed a link and landed here, which means the page you wanted has not
been written yet.

## What to do

1. Check the [table of contents][docs] — the page may have moved rather than
   disappeared.
2. Search the [API reference][api]. Every exported function is documented
   there, and the godoc comments in the source are kept in sync with it.
3. Read [MIGRATION.md][migration] if you are looking for something that exists
   in JavaScript gulp. Anything deliberately left out is listed there with the
   reason.
4. Open an issue describing what you were looking for and where you expected
   to find it.

## Pages that are deliberately absent

Some of gulp's own documentation describes machinery that has no counterpart
in a compiled language, and porting it verbatim would be misleading rather
than helpful. Those pages are not missing by accident:

| gulp page | Why it is absent |
| --- | --- |
| Recipes for `browserify`, `watchify`, `rollup`, `swig`, `browser-sync`, `grunt` | These wrap specific npm packages. The Go equivalent is to call the tool with [`plugins.Exec`][plugins], which is covered in [Using plugins][using-plugins]. |
| `.babelrc` / transpiler setup | A gulpfile is compiled Go. There is nothing to transpile. |
| `pump` | Explained in [Why use pump][pump] — the short version is that the pipeline runner already does what `pump` was invented to do. |

If you think a page belongs here and does not fall into the table above,
please say so in an issue.

[docs]: README.md
[api]: api/README.md
[migration]: ../MIGRATION.md
[plugins]: https://pkg.go.dev/github.com/gulpjs/gulp-go/plugins
[using-plugins]: getting-started/7-using-plugins.md
[pump]: why-use-pump/README.md
