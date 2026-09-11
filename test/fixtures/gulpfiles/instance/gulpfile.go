// Command gulpfile is the instance-based counterpart of the gulpfile fixture
// next door. Where that one calls the package-level API, this one builds its
// task graph on a gulp.New() value and installs that registry on the default
// instance before handing control to the CLI, which is how a gulpfile shares a
// registry with a plugin or a second build graph.
package main

import (
	"context"

	gulp "github.com/gulpjs/gulp-go"
)

func main() {
	g := gulp.New()
	g.Task("clean", func(context.Context) error { return nil })
	g.Task("build", func(context.Context) error { return nil })
	if _, err := g.TaskRef("default", g.Series(gulp.Name("clean"), gulp.Name("build"))); err != nil {
		panic(err)
	}
	if err := gulp.SetRegistry(g.Registry()); err != nil {
		panic(err)
	}
	gulp.Main()
}
