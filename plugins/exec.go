package plugins

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gulpjs/gulp-go/pipeline"
	"github.com/gulpjs/gulp-go/vinyl"
)

// ExecOptions configures Exec.
type ExecOptions struct {
	// Name labels errors. It defaults to Command.
	Name string

	// Command is the executable to run. It is looked up on PATH.
	Command string

	// Args builds the argument list for each file. It may be nil.
	//
	// Prefer having the tool read stdin and write stdout. When a tool insists
	// on a real path, use TempFile and reference Placeholder.
	Args func(*vinyl.File) []string

	// Env adds "KEY=VALUE" entries to the child's environment.
	Env []string

	// Dir is the child's working directory. It defaults to the file's cwd.
	Dir string

	// Rename adjusts the output path, for tools that change a file's type.
	Rename func(*Path)

	// TempFile writes the contents to a temporary file and substitutes its
	// path for Placeholder in Args, instead of piping on stdin.
	TempFile bool

	// TempSuffix names the temporary file, which some tools inspect to decide
	// how to parse their input. It defaults to the file's own extension.
	TempSuffix string

	// IgnoreStderr keeps a chatty tool from being mistaken for a failing one.
	// Otherwise stderr is included in the error when the command fails.
	IgnoreStderr bool
}

// Placeholder is replaced in ExecOptions.Args with the temporary file's path
// when ExecOptions.TempFile is set.
const Placeholder = "{}"

// Exec pipes each file through an external command, replacing its contents
// with the command's standard output.
//
// This is the general answer to gulp's npm plugins. Nearly all of them wrap a
// tool that also ships a command-line interface, and running that interface is
// both simpler and less likely to drift from upstream than reimplementing it:
//
//	plugins.Exec(plugins.ExecOptions{
//		Command: "esbuild",
//		Args:    func(*gulp.File) []string { return []string{"--minify", "--loader=js"} },
//	})
//
// The command runs once per file, which is what makes it composable with the
// rest of a pipeline, and files are processed in order. A non-zero exit is
// reported as an error carrying the command's stderr.
func Exec(opts ExecOptions) pipeline.Transform {
	name := opts.Name
	if name == "" {
		name = opts.Command
	}
	if opts.Command == "" {
		return failed(pluginErrf(name, nil, "no command specified"))
	}
	return pipeline.Map(func(ctx context.Context, f *vinyl.File) (*vinyl.File, error) {
		if passthrough(f) {
			return f, nil
		}
		body, err := f.Bytes()
		if err != nil {
			return nil, pluginErr(name, f, err)
		}
		out, err := runCommand(ctx, opts, f, body)
		if err != nil {
			return nil, pluginErr(name, f, err)
		}
		f.Contents = vinyl.Buffer(out)
		if opts.Rename != nil {
			return renameOne(f, opts.Rename, name)
		}
		return f, nil
	})
}

func runCommand(ctx context.Context, opts ExecOptions, f *vinyl.File, body []byte) ([]byte, error) {
	var args []string
	if opts.Args != nil {
		args = opts.Args(f)
	}

	var stdin *bytes.Reader
	if opts.TempFile {
		path, cleanup, err := writeTemp(f, body, opts.TempSuffix)
		if err != nil {
			return nil, err
		}
		defer cleanup()
		args = substitute(args, path)
	} else {
		stdin = bytes.NewReader(body)
	}

	dir := opts.Dir
	if dir == "" {
		dir = f.Cwd()
	}

	cmd := exec.CommandContext(ctx, opts.Command, args...)
	cmd.Dir = dir
	if len(opts.Env) > 0 {
		cmd.Env = append(os.Environ(), opts.Env...)
	}
	if stdin != nil {
		cmd.Stdin = stdin
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, commandError(err, stderr.Bytes(), opts.IgnoreStderr)
	}
	return stdout.Bytes(), nil
}

// commandError folds the command's stderr into the exit status, because an
// exec.ExitError on its own only says "exit status 1" and the tool has already
// explained what went wrong.
func commandError(err error, stderr []byte, ignore bool) error {
	msg := strings.TrimSpace(string(stderr))
	if ignore || msg == "" {
		return err
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return errors.New(err.Error() + ": " + msg)
	}
	return err
}

func writeTemp(f *vinyl.File, body []byte, suffix string) (string, func(), error) {
	if suffix == "" {
		suffix = filepath.Ext(f.Path())
	}
	tmp, err := os.CreateTemp("", "gulp-*"+suffix)
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { _ = os.Remove(tmp.Name()) }
	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		cleanup()
		return "", func() {}, err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return tmp.Name(), cleanup, nil
}

func substitute(args []string, path string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = strings.ReplaceAll(a, Placeholder, path)
	}
	return out
}

func renameOne(f *vinyl.File, fn func(*Path), plugin string) (*vinyl.File, error) {
	rel, err := f.Relative()
	if err != nil {
		return nil, pluginErr(plugin, f, err)
	}
	ext := filepath.Ext(rel)
	p := &Path{
		Dirname:  filepath.Dir(rel),
		Basename: filepath.Base(rel[:len(rel)-len(ext)]),
		Extname:  ext,
	}
	fn(p)
	if err := f.SetPath(filepath.Join(f.Base(), p.Dirname, p.Basename+p.Extname)); err != nil {
		return nil, pluginErr(plugin, f, err)
	}
	return f, nil
}
