<!-- front-matter
id: getting-started
title: Getting Started
hide_title: true
sidebar_label: Overview
-->

# Getting Started

New to gulp? Work through these in order. Each chapter builds on the one before
it, and together they cover everything you need to write a real build.

1. [Quick Start][quick-start] — install the toolchain and run your first task.
2. [Go and Gulpfiles][go-and-gulpfiles] — how a gulpfile is structured, and why
   it is a compiled program rather than a config file.
3. [Creating Tasks][creating-tasks] — public and private tasks, composition
   with `Series` and `Parallel`.
4. [Async Completion][async-completion] — the one task signature, and how to
   adapt callbacks, channels and commands to it.
5. [Working with Files][working-with-files] — `Src`, `Dest`, and the pipeline
   in between.
6. [Explaining Globs][explaining-globs] — how patterns are matched and how the
   base is computed.
7. [Using Plugins][using-plugins] — transforms, conditional pipelines, and
   wrapping external tools.
8. [Watching Files][watching-files] — rebuilding automatically on change.

If you are coming from the JavaScript gulp, read [MIGRATION.md][migration]
first. It maps every JavaScript package onto its Go counterpart and lists the
places where the two deliberately differ.

[quick-start]: 1-quick-start.md
[go-and-gulpfiles]: 2-go-and-gulpfiles.md
[creating-tasks]: 3-creating-tasks.md
[async-completion]: 4-async-completion.md
[working-with-files]: 5-working-with-files.md
[explaining-globs]: 6-explaining-globs.md
[using-plugins]: 7-using-plugins.md
[watching-files]: 8-watching-files.md
[migration]: ../../MIGRATION.md
