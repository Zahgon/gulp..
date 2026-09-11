<!--
name: recipes
title: Recipes
hide_title: true
sidebar_label: Recipes
-->

# Recipes

Short, self-contained answers to problems that come up once a build grows past
copying files around. Each page is a working gulpfile fragment, not a sketch.

## Recipes

* [Deleting files and folders][delete]
* [Passing arguments from the command line][args]
* [Incremental builds with concatenation][incremental]
* [Maintaining directory structure while globbing][structure]
* [Minified and non-minified output in one pass][minified]
* [Using multiple sources in one task][multiple-sources]
* [Running the same steps per folder][per-folder]
* [Sharing pipelines with factories][factories]
* [Making a file from bytes you already have][from-bytes]
* [Handling deletions while watching][watch-delete]
* [Running a build on a schedule][cron]
* [Running the Go test suite as a task][go-test]
* [Bundling with esbuild][esbuild]
* [A development server with live reload][server]
* [Templating with html/template and front matter][templating]
* [Automating releases][releases]

## Recipes that did not survive the port

Gulp's JavaScript documentation ships twenty-four recipes. Most of them are
instructions for wiring one specific npm package into a stream, and they have no
meaning here: there is no Browserify, no Watchify, no Rollup stream adapter, no
Grunt to delegate to, no Swig. Rewriting them as Go would have meant inventing
tools that do not exist.

The table below records what happened to each one, so that a reader arriving
from the JavaScript docs can find the equivalent rather than assume it was
forgotten.

| Upstream recipe | Here |
| :-------------- | :--- |
| `browserify-with-globs` | [Bundling with esbuild][esbuild] |
| `browserify-multiple-destination` | [Bundling with esbuild][esbuild] |
| `browserify-transforms` | [Bundling with esbuild][esbuild] |
| `browserify-uglify-sourcemap` | [Bundling with esbuild][esbuild] |
| `fast-browserify-builds-with-watchify` | [Bundling with esbuild][esbuild] |
| `rollup-with-rollup-stream` | [Bundling with esbuild][esbuild] |
| `run-grunt-tasks-from-gulp` | Dropped. Run any command with [`plugins.Exec`][exec] or `exec.CommandContext`. |
| `templating-with-swig-and-yaml-front-matter` | [Templating with html/template][templating] |
| `mocha-test-runner-with-gulp` | [Running the Go test suite as a task][go-test] |
| `minimal-browsersync-setup-with-gulp4` | [A development server with live reload][server] |
| `server-with-livereload-and-css-injection` | [A development server with live reload][server] |
| `combining-streams-to-handle-errors` | Dropped. A pipeline already propagates and attributes errors — see [why pump is unnecessary][pump]. |

Everything else in the upstream list has a page here under the same name.

## Something missing?

Recipes are the least complete part of this documentation. If you have solved a
problem that is not covered, please open an issue — see
[documentation missing][missing].

[delete]: delete-files-folder.md
[args]: pass-arguments-from-cli.md
[incremental]: incremental-builds-with-concatenate.md
[structure]: maintain-directory-structure-while-globbing.md
[minified]: minified-and-non-minified.md
[multiple-sources]: using-multiple-sources-in-one-task.md
[per-folder]: running-task-steps-per-folder.md
[factories]: sharing-streams-with-stream-factories.md
[from-bytes]: make-stream-from-buffer.md
[watch-delete]: handling-the-delete-event-on-watch.md
[cron]: cron-task.md
[go-test]: go-test-runner.md
[esbuild]: bundling-with-esbuild.md
[server]: server-with-live-reload.md
[templating]: templating-with-front-matter.md
[releases]: automate-releases.md
[exec]: https://pkg.go.dev/github.com/gulpjs/gulp-go/plugins#Exec
[pump]: ../why-use-pump/README.md
[missing]: ../documentation-missing.md
