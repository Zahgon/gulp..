<p align="center">
  <a href="https://gulpjs.com">
    <img height="257" width="114" src="https://raw.githubusercontent.com/gulpjs/artwork/master/gulp-2x.png">
  </a>
</p>

# Documentation

A Go port of gulp, the streaming build system. If you are new here, start with
[Getting Started][getting-started].

These pages describe the Go API. Where behaviour differs from the JavaScript
original, the difference is called out inline and catalogued in
[MIGRATION.md][migration].

## Getting started

1. [Quick Start][quick-start]
2. [Go and Gulpfiles][go-and-gulpfiles]
3. [Creating Tasks][creating-tasks]
4. [Async Completion][async-completion]
5. [Working with Files][working-with-files]
6. [Explaining Globs][explaining-globs]
7. [Using Plugins][using-plugins]
8. [Watching Files][watching-files]

## API

* [Concepts][concepts]
* [`Src()`][src], [`Dest()`][dest], [`Symlink()`][symlink]
* [`Task()`][task], [`Series()`][series], [`Parallel()`][parallel]
* [`Watch()`][watch], [`LastRun()`][last-run]
* [`Registry()`][registry], [`Tree()`][tree]
* [Vinyl][vinyl], [`vinyl.IsVinyl()`][is-vinyl], [`vinyl.IsCustomProp()`][is-custom-prop]

## Command line

* [CLI reference][cli]

## Advanced

* [Creating Custom Registries][custom-registries]
* [Why Go needs no `pump`][why-use-pump]

## Writing a plugin

* [Overview][writing-a-plugin]
* [Using Buffers][using-buffers]
* [Dealing with Streams][dealing-with-streams]
* [Testing][testing]
* [Guidelines][guidelines]
* [Recommended Packages][recommended-modules]

## Other

* [Recipes][recipes]
* [FAQ][faq]
* [Documentation Missing][documentation-missing]
* [For Enterprise][for-enterprise]

[getting-started]: getting-started/README.md
[quick-start]: getting-started/1-quick-start.md
[go-and-gulpfiles]: getting-started/2-go-and-gulpfiles.md
[creating-tasks]: getting-started/3-creating-tasks.md
[async-completion]: getting-started/4-async-completion.md
[working-with-files]: getting-started/5-working-with-files.md
[explaining-globs]: getting-started/6-explaining-globs.md
[using-plugins]: getting-started/7-using-plugins.md
[watching-files]: getting-started/8-watching-files.md
[concepts]: api/concepts.md
[src]: api/src.md
[dest]: api/dest.md
[symlink]: api/symlink.md
[task]: api/task.md
[series]: api/series.md
[parallel]: api/parallel.md
[watch]: api/watch.md
[last-run]: api/last-run.md
[registry]: api/registry.md
[tree]: api/tree.md
[vinyl]: api/vinyl.md
[is-vinyl]: api/vinyl-isvinyl.md
[is-custom-prop]: api/vinyl-iscustomprop.md
[cli]: CLI.md
[custom-registries]: advanced/creating-custom-registries.md
[why-use-pump]: why-use-pump/README.md
[writing-a-plugin]: writing-a-plugin/README.md
[using-buffers]: writing-a-plugin/using-buffers.md
[dealing-with-streams]: writing-a-plugin/dealing-with-streams.md
[testing]: writing-a-plugin/testing.md
[guidelines]: writing-a-plugin/guidelines.md
[recommended-modules]: writing-a-plugin/recommended-modules.md
[recipes]: recipes/README.md
[faq]: FAQ.md
[documentation-missing]: documentation-missing.md
[for-enterprise]: support/for-enterprise.md
[migration]: ../MIGRATION.md
