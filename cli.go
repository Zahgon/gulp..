package gulp

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/gulpjs/gulp-go/internal/cli"
)

// Version is the version of gulp this port tracks.
const Version = cli.Version

// Main runs the command line interface against the default instance and exits
// the process with the resulting status.
//
// This is the last line of a gulpfile. Where JavaScript keeps the gulpfile
// declarative and lets a separately installed `gulp` binary require it, a Go
// gulpfile is an ordinary program, so it ends by handing control to the CLI:
//
//	func main() {
//		gulp.Task("build", build)
//		gulp.Main()
//	}
//
// Interrupts are translated into cancellation of the context tasks receive, so
// a well-behaved task shuts down cleanly on the first Ctrl-C. A second signal
// is left to the runtime's default handler, which terminates immediately.
func Main() {
	os.Exit(Execute(os.Args[1:]))
}

// Execute runs the command line interface against the default instance and
// returns the exit status instead of exiting, which is what makes the CLI
// testable and lets a caller run tasks as part of a larger program.
func Execute(argv []string) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return cli.Run(cli.Runtime{
		Undertaker: Default.Undertaker,
		Context:    ctx,
	}, argv)
}
