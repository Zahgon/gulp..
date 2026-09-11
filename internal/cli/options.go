package cli

import (
	"fmt"
	"strconv"
	"strings"
)

// DefaultTasksDepth is the depth --tasks prints when --tasks-depth is absent.
const DefaultTasksDepth = 50

// Options is the parsed command line.
type Options struct {
	Help            bool
	Version         bool
	Preload         []string
	Gulpfile        string
	Cwd             string
	Tasks           bool
	TasksSimple     bool
	TasksJSON       bool
	TasksJSONPath   string
	TasksDepth      int
	CompactTasks    bool
	SortTasks       bool
	Color           bool
	NoColor         bool
	Silent          bool
	ContinueOnError bool
	Series          bool
	LogLevel        Level

	// Extra holds flags the CLI does not recognise. gulp forwards these to
	// the gulpfile, because a task may declare its own flags and document
	// them in the --tasks output; rejecting them would break that feature.
	Extra map[string]string

	// Args are the task names to run.
	Args []string
}

// Listing reports whether the invocation only prints information about tasks
// rather than running them. gulp silences its logger in that case so the
// output stays machine-readable.
func (o *Options) Listing() bool {
	return o.TasksSimple || o.TasksJSON || o.Help || o.Version
}

// Quiet reports whether logging should be suppressed entirely.
func (o *Options) Quiet() bool {
	return o.Silent || o.Listing()
}

// Depth returns the effective task-tree depth, clamped the way gulp-cli
// clamps it: unset means 50 levels, and anything below one level is
// meaningless so it is raised to one.
func (o *Options) Depth() int {
	if o.TasksDepth <= 0 {
		return DefaultTasksDepth
	}
	return o.TasksDepth
}

type flagKind int

const (
	flagBool flagKind = iota
	flagString
	flagList
	flagNumber
	flagOptionalString
	flagCount
)

type flagSpec struct {
	name  string
	kind  flagKind
	short string
}

// flagSpecs mirrors gulp-cli's cli-options.js. "require" is kept as an alias
// for "preload" because gulp's own CLI.md still documents the older name.
var flagSpecs = []flagSpec{
	{name: "help", kind: flagBool, short: "h"},
	{name: "version", kind: flagBool, short: "v"},
	{name: "preload", kind: flagList},
	{name: "require", kind: flagList},
	{name: "gulpfile", kind: flagString, short: "f"},
	{name: "cwd", kind: flagString},
	{name: "tasks", kind: flagBool, short: "T"},
	{name: "tasks-simple", kind: flagBool},
	{name: "tasks-json", kind: flagOptionalString},
	{name: "tasks-depth", kind: flagNumber},
	{name: "depth", kind: flagNumber},
	{name: "compact-tasks", kind: flagBool},
	{name: "sort-tasks", kind: flagBool},
	{name: "color", kind: flagBool},
	{name: "no-color", kind: flagBool},
	{name: "silent", kind: flagBool, short: "S"},
	{name: "continue", kind: flagBool},
	{name: "series", kind: flagBool},
	{name: "log-level", kind: flagCount, short: "L"},
}

func lookupFlag(name string) (flagSpec, bool) {
	for _, spec := range flagSpecs {
		if spec.name == name {
			return spec, true
		}
	}
	return flagSpec{}, false
}

func lookupShort(c byte) (flagSpec, bool) {
	for _, spec := range flagSpecs {
		if spec.short != "" && spec.short[0] == c {
			return spec, true
		}
	}
	return flagSpec{}, false
}

// Parse reads argv, which must not include the program name.
//
// The syntax follows yargs, which is what gulp-cli uses: long flags accept
// either "--name value" or "--name=value", short flags cluster so that -TS is
// two flags, an unknown "--no-x" negates "x", and everything after "--" is a
// positional argument.
func Parse(argv []string) (*Options, error) {
	opts := &Options{LogLevel: DefaultLevel, Extra: map[string]string{}}
	p := parser{argv: argv, opts: opts}
	if err := p.run(); err != nil {
		return nil, err
	}
	return opts, nil
}

type parser struct {
	argv []string
	pos  int
	opts *Options
}

func (p *parser) run() error {
	for p.pos < len(p.argv) {
		arg := p.argv[p.pos]
		p.pos++
		switch {
		case arg == "--":
			p.opts.Args = append(p.opts.Args, p.argv[p.pos:]...)
			p.pos = len(p.argv)
		case strings.HasPrefix(arg, "--"):
			if err := p.long(arg[2:]); err != nil {
				return err
			}
		case len(arg) > 1 && arg[0] == '-':
			if err := p.short(arg[1:]); err != nil {
				return err
			}
		default:
			p.opts.Args = append(p.opts.Args, arg)
		}
	}
	return nil
}

func (p *parser) long(body string) error {
	name, value, hasValue := strings.Cut(body, "=")
	if name == "" {
		return fmt.Errorf("unknown option: --%s", body)
	}
	spec, known := lookupFlag(name)
	if !known {
		return p.unknown(name, value, hasValue)
	}
	return p.apply(spec, value, hasValue, 1)
}

func (p *parser) short(body string) error {
	for i := 0; i < len(body); i++ {
		spec, known := lookupShort(body[i])
		if !known {
			return fmt.Errorf("unknown option: -%c", body[i])
		}
		if spec.kind == flagCount {
			count := 1
			for i+1 < len(body) && body[i+1] == body[i] {
				count++
				i++
			}
			if err := p.apply(spec, "", false, count); err != nil {
				return err
			}
			continue
		}
		// A value-taking short flag consumes the rest of the cluster, so
		// -fbuild.go works as well as -f build.go.
		if spec.kind != flagBool {
			rest := body[i+1:]
			if err := p.apply(spec, rest, rest != "", 1); err != nil {
				return err
			}
			return nil
		}
		if err := p.apply(spec, "", false, 1); err != nil {
			return err
		}
	}
	return nil
}

// unknown records a flag the CLI does not define so the gulpfile can read it,
// honouring yargs' boolean negation for "--no-something".
func (p *parser) unknown(name, value string, hasValue bool) error {
	if negated, ok := strings.CutPrefix(name, "no-"); ok && !hasValue {
		p.opts.Extra[negated] = "false"
		return nil
	}
	if !hasValue {
		if next, ok := p.peekValue(); ok {
			value = next
			p.pos++
		} else {
			value = "true"
		}
	}
	p.opts.Extra[name] = value
	return nil
}

func (p *parser) apply(spec flagSpec, value string, hasValue bool, count int) error {
	switch spec.kind {
	case flagBool:
		return p.setBool(spec.name, value, hasValue)
	case flagCount:
		p.opts.LogLevel = clampLevel(count)
		return nil
	case flagOptionalString:
		p.opts.TasksJSON = true
		// yargs leaves --tasks-json untyped, so a following non-flag token
		// becomes the output path and a bare flag prints to stdout. Consuming
		// the token is safe because listing modes never run tasks.
		if !hasValue {
			next, ok := p.peekValue()
			if !ok {
				return nil
			}
			value = next
			p.pos++
		}
		p.opts.TasksJSONPath = value
		return nil
	}
	if !hasValue {
		next, ok := p.peekValue()
		if !ok {
			return fmt.Errorf("option --%s requires a value", spec.name)
		}
		value = next
		p.pos++
	}
	switch spec.kind {
	case flagString:
		return p.setString(spec.name, value)
	case flagList:
		p.opts.Preload = append(p.opts.Preload, value)
		return nil
	case flagNumber:
		// yargs coerces an unparseable number to NaN and every gulp-cli
		// comparison against NaN is false, so the depth limit stops applying.
		if n, err := strconv.Atoi(value); err == nil {
			p.opts.TasksDepth = n
		}
		return nil
	}
	return fmt.Errorf("unknown option: --%s", spec.name)
}

func (p *parser) setBool(name, value string, hasValue bool) error {
	on := true
	if hasValue {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("option --%s requires a boolean: %w", name, err)
		}
		on = parsed
	}
	switch name {
	case "help":
		p.opts.Help = on
	case "version":
		p.opts.Version = on
	case "tasks":
		p.opts.Tasks = on
	case "tasks-simple":
		p.opts.TasksSimple = on
	case "compact-tasks":
		p.opts.CompactTasks = on
	case "sort-tasks":
		p.opts.SortTasks = on
	case "color":
		p.opts.Color = on
	case "no-color":
		p.opts.NoColor = on
	case "silent":
		p.opts.Silent = on
	case "continue":
		p.opts.ContinueOnError = on
	case "series":
		p.opts.Series = on
	default:
		return fmt.Errorf("unknown option: --%s", name)
	}
	return nil
}

func (p *parser) setString(name, value string) error {
	switch name {
	case "gulpfile":
		p.opts.Gulpfile = value
	case "cwd":
		p.opts.Cwd = value
	default:
		return fmt.Errorf("unknown option: --%s", name)
	}
	return nil
}

// peekValue reports the next argument when it can serve as a flag value. An
// argument beginning with "-" is treated as the next flag rather than a value,
// so a missing value is reported instead of silently swallowing a flag.
func (p *parser) peekValue() (string, bool) {
	if p.pos >= len(p.argv) {
		return "", false
	}
	next := p.argv[p.pos]
	if strings.HasPrefix(next, "-") && next != "-" {
		return "", false
	}
	return next, true
}

func clampLevel(count int) Level {
	switch {
	case count < int(LevelSilent):
		return LevelSilent
	case count > int(LevelDebug):
		return LevelDebug
	default:
		return Level(count)
	}
}
