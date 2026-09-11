package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/gulpjs/gulp-go/undertaker"
)

// Runtime is the environment a CLI invocation runs against.
type Runtime struct {
	Undertaker *undertaker.Undertaker
	Gulpfile   string
	Version    string
	Stdout     io.Writer
	Stderr     io.Writer
	Context    context.Context
}

// ExitCode reports success or failure to the shell, matching gulp's use of
// exit(1) for any failed build.
const (
	ExitOK    = 0
	ExitError = 1
)

// Run parses argv, which excludes the program name, and carries out the
// requested action against rt. It returns the process exit code.
func Run(rt Runtime, argv []string) int {
	rt = rt.withDefaults()

	opts, err := Parse(argv)
	if err != nil {
		fmt.Fprintln(rt.Stderr, err)
		return ExitError
	}

	palette := NewPalette(opts.Color, opts.NoColor)
	msg := Messages{Palette: palette}
	level := opts.LogLevel
	if opts.Quiet() {
		level = LevelSilent
	}
	log := NewLogger(rt.Stdout, rt.Stderr, palette, level)

	switch {
	case opts.Help:
		fmt.Fprint(rt.Stdout, Help(msg))
		return ExitOK
	case opts.Version:
		fmt.Fprintf(rt.Stdout, "CLI version: %s\n", rt.Version)
		fmt.Fprintf(rt.Stdout, "Local version: %s\n", rt.Version)
		return ExitOK
	}

	if err := rt.locate(opts, log, msg); err != nil {
		if errors.Is(err, errNoGulpfile) {
			log.Error("%s", msg.MissingGulpfile())
			return ExitError
		}
		log.Error("%s", msg.TaskError(err))
		return ExitError
	}

	if opts.TasksSimple || opts.Tasks || opts.TasksJSON {
		return listTasks(rt, opts, msg)
	}

	log.Info("%s", msg.Gulpfile(rt.Gulpfile))
	reported := attachEvents(rt.Undertaker, log, msg)

	if err := runTasks(rt, opts); err != nil {
		reportFailure(rt, log, msg, err, reported)
		return ExitError
	}
	return ExitOK
}

func (rt Runtime) withDefaults() Runtime {
	if rt.Stdout == nil {
		rt.Stdout = os.Stdout
	}
	if rt.Stderr == nil {
		rt.Stderr = os.Stderr
	}
	if rt.Context == nil {
		rt.Context = context.Background()
	}
	if rt.Version == "" {
		rt.Version = Version
	}
	if rt.Gulpfile == "" {
		rt.Gulpfile = detectGulpfile()
	}
	return rt
}

// locate applies --cwd and --gulpfile.
//
// INIT_CWD is recorded before anything moves, so a task can still find the
// directory the user invoked gulp from; gulp exports the same variable.
// Passing --gulpfile also changes directory to the gulpfile's folder, which is
// what makes relative globs in a gulpfile mean the same thing however gulp was
// invoked. An explicit --cwd wins over that.
func (rt *Runtime) locate(opts *Options, log *Logger, msg Messages) error {
	if initial, err := os.Getwd(); err == nil {
		if _, set := os.LookupEnv("INIT_CWD"); !set {
			if err := os.Setenv("INIT_CWD", initial); err != nil {
				return err
			}
		}
	}

	target := ""
	if opts.Gulpfile != "" {
		abs, err := filepath.Abs(opts.Gulpfile)
		if err != nil {
			return err
		}
		if info, err := os.Stat(abs); err != nil || info.IsDir() {
			return errNoGulpfile
		}
		rt.Gulpfile = abs
		target = filepath.Dir(abs)
	}
	if opts.Cwd != "" {
		abs, err := filepath.Abs(opts.Cwd)
		if err != nil {
			return err
		}
		target = abs
	}
	if target == "" {
		return nil
	}
	current, err := os.Getwd()
	if err != nil {
		return err
	}
	if err := os.Chdir(target); err != nil {
		return err
	}
	if sameDir(current, target) {
		return nil
	}
	log.Info("%s", msg.CwdChanged(target))
	return nil
}

// errNoGulpfile marks an explicit --gulpfile that names nothing on disk. gulp
// reports that as "No gulpfile found" and exits 1 rather than carrying on with
// a path that cannot be read.
var errNoGulpfile = errors.New("no gulpfile found")

// sameDir compares two absolute directories after resolving symlinks, so that
// a --cwd spelled through /tmp is not reported as a change when it resolves to
// the directory gulp is already in.
func sameDir(a, b string) bool {
	if a == b {
		return true
	}
	ra, err := filepath.EvalSymlinks(a)
	if err != nil {
		return false
	}
	rb, err := filepath.EvalSymlinks(b)
	if err != nil {
		return false
	}
	return ra == rb
}

func listTasks(rt Runtime, opts *Options, msg Messages) int {
	u := rt.Undertaker
	switch {
	case opts.TasksSimple:
		fmt.Fprintln(rt.Stdout, SimpleList(u.Tree(false)))
		return ExitOK

	case opts.TasksJSON:
		tree := u.Tree(true)
		tree.Label = msg.Description(rt.Gulpfile)
		data, err := TreeJSON(tree, opts.Depth(), opts.CompactTasks)
		if err != nil {
			fmt.Fprintln(rt.Stderr, err)
			return ExitError
		}
		if opts.TasksJSONPath != "" {
			if err := os.WriteFile(opts.TasksJSONPath, data, 0o644); err != nil {
				fmt.Fprintln(rt.Stderr, err)
				return ExitError
			}
			return ExitOK
		}
		fmt.Fprintln(rt.Stdout, string(data))
		return ExitOK

	default:
		tree := u.Tree(true)
		tree.Label = msg.Description(rt.Gulpfile)
		renderer := TreeRenderer{
			Messages:     msg,
			Lookup:       u.Get,
			MaxDepth:     opts.Depth(),
			CompactTasks: opts.CompactTasks,
			SortTasks:    opts.SortTasks,
		}
		for _, line := range renderer.Render(tree) {
			fmt.Fprintln(rt.Stdout, line)
		}
		return ExitOK
	}
}

// runTasks executes the requested tasks. With no arguments gulp runs "default",
// and multiple arguments run concurrently unless --series is given.
func runTasks(rt Runtime, opts *Options) error {
	names := opts.Args
	if len(names) == 0 {
		names = []string{"default"}
	}
	refs := undertaker.Names(names...)

	u := rt.Undertaker
	if opts.ContinueOnError {
		u.SetSettle(true)
	}
	var root *undertaker.Task
	if opts.Series {
		root = u.Series(refs...)
	} else {
		root = u.Parallel(refs...)
	}
	return root.Fn(rt.Context)
}

// attachEvents mirrors gulp-cli's event log.
//
// Branch events — the synthetic <series> and <parallel> wrappers — are logged
// at debug rather than info, which is why a normal build shows only the tasks
// the user actually wrote. Errors are de-duplicated by identity because a
// single failure propagates up through every enclosing composition and would
// otherwise be printed once per level. Settle mode joins failures, so the
// check walks the error tree rather than comparing identity alone.
func attachEvents(u *undertaker.Undertaker, log *Logger, msg Messages) func(error) bool {
	var mu sync.Mutex
	reported := map[error]bool{}

	u.On(func(evt undertaker.Event) {
		switch evt.Kind {
		case undertaker.EventStart:
			if evt.Branch {
				log.Debug("%s", msg.TaskStart(evt.Name))
			} else {
				log.Info("%s", msg.TaskStart(evt.Name))
			}

		case undertaker.EventStop:
			if evt.Branch {
				log.Debug("%s", msg.TaskStop(evt.Name, evt.Duration))
			} else {
				log.Info("%s", msg.TaskStop(evt.Name, evt.Duration))
			}

		case undertaker.EventError:
			if evt.Branch {
				log.Debug("%s", msg.TaskFailure(evt.Name, evt.Duration))
			} else {
				log.Error("%s", msg.TaskFailure(evt.Name, evt.Duration))
			}
			mu.Lock()
			seen := wasReported(evt.Err, reported)
			reported[evt.Err] = true
			mu.Unlock()
			if !seen && evt.Err != nil {
				log.Error("%s", msg.TaskError(evt.Err))
			}
		}
	})

	return func(err error) bool {
		mu.Lock()
		defer mu.Unlock()
		return wasReported(err, reported)
	}
}

// wasReported walks the error tree that runTasks returned looking for an error
// the event stream already printed. Settle mode joins every failure into one
// error, so the terminal error is usually a wrapper around the ones already
// seen rather than one of them.
func wasReported(err error, reported map[error]bool) bool {
	for err != nil {
		if reported[err] {
			return true
		}
		switch unwrapper := err.(type) {
		case interface{ Unwrap() error }:
			err = unwrapper.Unwrap()
		case interface{ Unwrap() []error }:
			for _, inner := range unwrapper.Unwrap() {
				if wasReported(inner, reported) {
					return true
				}
			}
			return false
		default:
			return false
		}
	}
	return false
}

// reportFailure prints the terminal error. An unknown task name gets the
// "did you mean?" treatment rather than a bare error, because a typo is the
// overwhelmingly common cause.
func reportFailure(rt Runtime, log *Logger, msg Messages, err error, reported func(error) bool) {
	var missing *undertaker.UndefinedTaskError
	if errors.As(err, &missing) {
		log.Error("%s", msg.TaskMissing(missing.Name, SimilarTasks(missing.Name, taskNames(rt.Undertaker))))
		return
	}
	if errors.Is(err, context.Canceled) {
		return
	}
	if reported != nil && reported(err) {
		return
	}
	log.Error("%s", msg.TaskError(err))
}

func taskNames(u *undertaker.Undertaker) []string {
	tasks := u.Tasks()
	names := make([]string, 0, len(tasks))
	for name := range tasks {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
