<!--
name: for-enterprise
title: gulp for enterprise
-->

# gulp for enterprise

## Available as part of the Tidelift Subscription

The maintainers of gulp and thousands of other packages are working with Tidelift to deliver commercial support and maintenance for the open source dependencies you use to build your applications. Save time, reduce risk, and improve code health, while paying the maintainers of the exact dependencies you use.

[Learn more.](https://tidelift.com/subscription/pkg/npm-gulp?utm_source=npm-gulp&utm_medium=referral&utm_campaign=enterprise)

> [!NOTE]
> That subscription covers the **JavaScript** gulp packages on npm. This Go port is an independent reimplementation and is not part of it. If you are evaluating the port for production use, the relevant questions are answered below.

## Is this port supported?

It is maintained on a best-effort basis and carries no commercial support agreement. Treat it the way you would treat any other unpaid open source dependency: read the code, run the tests, and pin a version.

## How is correctness established?

Three layers, all of which run in CI:

1. **Unit tests** per package, run with `-race`.
2. **Ported gulp tests** — every case from gulp's own `test/index.test.js`, `test/src.js`, `test/dest.js` and `test/watch.js` has a Go counterpart, listed in [MIGRATION.md][migration].
3. **Differential tests** — a suite that runs the real JavaScript gulp 5.0.1 in a Node subprocess and compares its output against the Go port byte for byte: `src` contents and emission order across eight glob shapes, the full `dest` output tree, the task tree JSON, and `--tasks-simple`.

To reproduce the third layer yourself:

```sh
npm install gulp@5.0.1 --prefix /tmp/gulp-js
GULP_JS_REPO=/tmp/gulp-js/node_modules/gulp go test -run Differential -v .
```

## What is the dependency footprint?

Four direct dependencies, all widely used and permissively licensed:

| Module | Purpose |
|:------|:--------|
| `github.com/bmatcuk/doublestar/v4` | glob matching |
| `github.com/fsnotify/fsnotify` | filesystem notifications |
| `golang.org/x/sys` | `futimes` for metadata sync |
| `golang.org/x/text` | encoding and display width |

There is no plugin ecosystem to audit, because there are no plugins to install. Build tools are invoked as subprocesses through [`plugins.Exec`][exec], so `esbuild` or `sass` remains whatever you already vendored, under whatever policy you already apply to it.

## What is the license?

MIT, the same as gulp. See [LICENSE][license].

## How do I report a security issue?

See [SECURITY.md][security].

[migration]: ../../MIGRATION.md
[license]: ../../LICENSE
[security]: ../../.github/SECURITY.md
[exec]: https://pkg.go.dev/github.com/gulpjs/gulp-go/plugins#Exec
