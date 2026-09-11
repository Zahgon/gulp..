<!-- front-matter
id: quick-start
title: Quick Start
hide_title: true
sidebar_label: Quick Start
-->

# Quick Start

## Check your Go version

```sh
go version
```

Go 1.23 or newer is required. If the command is not found, install Go from
[go.dev/dl](https://go.dev/dl/).

## Create a project

```sh
mkdir my-project && cd my-project
go mod init example.com/my-project
go get github.com/gulpjs/gulp-go
```

## Create a gulpfile

Create `gulpfile.go` in the project root:

```go
package main

import (
	"context"
	"fmt"

	gulp "github.com/gulpjs/gulp-go"
)

func main() {
	gulp.Task("default", func(ctx context.Context) error {
		fmt.Println("hello from gulp")
		return nil
	})

	gulp.Main()
}
```

The file is `package main` and ends in `gulp.Main()`. That is what makes it
runnable — a gulpfile in Go is an ordinary program, not a manifest that some
other tool loads.

## Run it

```sh
go run . default
```

```
[13:05:29] Using gulpfile ~/my-project/gulpfile.go
[13:05:29] Starting 'default'...
hello from gulp
[13:05:29] Finished 'default' after 37 μs
```

Because `default` is the task gulp runs when you name none, `go run .` on its
own does the same thing.

## Install the launcher (optional)

If you would rather type `gulp` than `go run .`:

```sh
go install github.com/gulpjs/gulp-go/cmd/gulp@latest
```

```sh
gulp            # same as: go run .
gulp default    # same as: go run . default
```

The launcher searches upward from the current directory for `gulpfile.go`,
`Gulpfile.go` or `gulpfile/main.go`, then runs `go run .` in that package's
directory with your arguments forwarded. It is a convenience, nothing more —
every example in these docs works with plain `go run .`.

## Next

Continue to [Go and Gulpfiles][go-and-gulpfiles] to see how a real build is
laid out.

[go-and-gulpfiles]: 2-go-and-gulpfiles.md
