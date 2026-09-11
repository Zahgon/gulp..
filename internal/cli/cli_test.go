package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gulpjs/gulp-go/undertaker"
)

func plain() Messages { return Messages{Palette: Palette{}} }

func TestFormatDuration(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{0, ""},
		{7 * time.Nanosecond, "7 ns"},
		{999 * time.Nanosecond, "999 ns"},
		{time.Microsecond, "1 μs"},
		{37 * time.Microsecond, "37 μs"},
		{1500 * time.Nanosecond, "1.5 μs"},
		{1320 * time.Nanosecond, "1.32 μs"},
		{time.Millisecond, "1 ms"},
		{5690 * time.Microsecond, "5.69 ms"},
		{21 * time.Millisecond, "21 ms"},
		{999 * time.Millisecond, "999 ms"},
		{time.Second, "1 s"},
		{1500 * time.Millisecond, "1.5 s"},
		{1320 * time.Millisecond, "1.32 s"},
		{9999 * time.Millisecond, "10 s"},
		{time.Minute, "1 min"},
		{90 * time.Second, "1.5 min"},
		{10 * time.Minute, "10 min"},
		{time.Hour, "1 h"},
		{90 * time.Minute, "1.5 h"},
	}
	for _, c := range cases {
		if got := FormatDuration(c.in); got != c.want {
			t.Errorf("FormatDuration(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseDefaults(t *testing.T) {
	opts, err := Parse(nil)
	if err != nil {
		t.Fatal(err)
	}
	if opts.LogLevel != DefaultLevel {
		t.Errorf("LogLevel = %v, want %v", opts.LogLevel, DefaultLevel)
	}
	if opts.Depth() != DefaultTasksDepth {
		t.Errorf("Depth() = %d, want %d", opts.Depth(), DefaultTasksDepth)
	}
	if len(opts.Args) != 0 {
		t.Errorf("Args = %v, want empty", opts.Args)
	}
	if opts.Quiet() {
		t.Error("Quiet() = true, want false")
	}
}

func TestParseFlags(t *testing.T) {
	cases := []struct {
		name  string
		argv  []string
		check func(*testing.T, *Options)
	}{
		{"long bool", []string{"--tasks"}, func(t *testing.T, o *Options) {
			if !o.Tasks {
				t.Error("Tasks not set")
			}
		}},
		{"short bool", []string{"-T"}, func(t *testing.T, o *Options) {
			if !o.Tasks {
				t.Error("Tasks not set")
			}
		}},
		{"short cluster", []string{"-TS"}, func(t *testing.T, o *Options) {
			if !o.Tasks || !o.Silent {
				t.Errorf("Tasks=%v Silent=%v, want both true", o.Tasks, o.Silent)
			}
		}},
		{"equals value", []string{"--gulpfile=build/gulpfile.go"}, func(t *testing.T, o *Options) {
			if o.Gulpfile != "build/gulpfile.go" {
				t.Errorf("Gulpfile = %q", o.Gulpfile)
			}
		}},
		{"separate value", []string{"--gulpfile", "build/gulpfile.go"}, func(t *testing.T, o *Options) {
			if o.Gulpfile != "build/gulpfile.go" {
				t.Errorf("Gulpfile = %q", o.Gulpfile)
			}
		}},
		{"short attached value", []string{"-fbuild.go"}, func(t *testing.T, o *Options) {
			if o.Gulpfile != "build.go" {
				t.Errorf("Gulpfile = %q", o.Gulpfile)
			}
		}},
		{"repeated preload", []string{"--preload", "a", "--preload", "b"}, func(t *testing.T, o *Options) {
			if strings.Join(o.Preload, ",") != "a,b" {
				t.Errorf("Preload = %v", o.Preload)
			}
		}},
		{"require aliases preload", []string{"--require", "a"}, func(t *testing.T, o *Options) {
			if strings.Join(o.Preload, ",") != "a" {
				t.Errorf("Preload = %v", o.Preload)
			}
		}},
		{"log level once", []string{"-L"}, func(t *testing.T, o *Options) {
			if o.LogLevel != LevelError {
				t.Errorf("LogLevel = %v, want %v", o.LogLevel, LevelError)
			}
		}},
		{"log level thrice", []string{"-LLL"}, func(t *testing.T, o *Options) {
			if o.LogLevel != LevelInfo {
				t.Errorf("LogLevel = %v, want %v", o.LogLevel, LevelInfo)
			}
		}},
		{"log level four", []string{"-LLLL"}, func(t *testing.T, o *Options) {
			if o.LogLevel != LevelDebug {
				t.Errorf("LogLevel = %v, want %v", o.LogLevel, LevelDebug)
			}
		}},
		{"log level clamps", []string{"-LLLLLLL"}, func(t *testing.T, o *Options) {
			if o.LogLevel != LevelDebug {
				t.Errorf("LogLevel = %v, want %v", o.LogLevel, LevelDebug)
			}
		}},
		{"tasks depth", []string{"--tasks-depth", "2"}, func(t *testing.T, o *Options) {
			if o.Depth() != 2 {
				t.Errorf("Depth() = %d", o.Depth())
			}
		}},
		{"depth alias", []string{"--depth", "3"}, func(t *testing.T, o *Options) {
			if o.Depth() != 3 {
				t.Errorf("Depth() = %d", o.Depth())
			}
		}},
		{"tasks json bare", []string{"--tasks-json"}, func(t *testing.T, o *Options) {
			if !o.TasksJSON || o.TasksJSONPath != "" {
				t.Errorf("TasksJSON=%v path=%q", o.TasksJSON, o.TasksJSONPath)
			}
		}},
		{"tasks json path", []string{"--tasks-json", "out.json"}, func(t *testing.T, o *Options) {
			if !o.TasksJSON || o.TasksJSONPath != "out.json" {
				t.Errorf("TasksJSON=%v path=%q", o.TasksJSON, o.TasksJSONPath)
			}
		}},
		{"no-color", []string{"--no-color"}, func(t *testing.T, o *Options) {
			if !o.NoColor {
				t.Error("NoColor not set")
			}
		}},
		{"terminator", []string{"--", "--tasks"}, func(t *testing.T, o *Options) {
			if o.Tasks {
				t.Error("Tasks set after terminator")
			}
			if strings.Join(o.Args, ",") != "--tasks" {
				t.Errorf("Args = %v", o.Args)
			}
		}},
		{"unknown long", []string{"--dry-run"}, func(t *testing.T, o *Options) {
			if o.Extra["dry-run"] != "true" {
				t.Errorf("Extra = %v", o.Extra)
			}
		}},
		{"unknown negated", []string{"--no-minify"}, func(t *testing.T, o *Options) {
			if o.Extra["minify"] != "false" {
				t.Errorf("Extra = %v", o.Extra)
			}
		}},
		{"unknown with value", []string{"--env=prod"}, func(t *testing.T, o *Options) {
			if o.Extra["env"] != "prod" {
				t.Errorf("Extra = %v", o.Extra)
			}
		}},
		{"task names", []string{"clean", "build"}, func(t *testing.T, o *Options) {
			if strings.Join(o.Args, ",") != "clean,build" {
				t.Errorf("Args = %v", o.Args)
			}
		}},
		{"flags around names", []string{"-T", "build", "--silent"}, func(t *testing.T, o *Options) {
			if !o.Tasks || !o.Silent || strings.Join(o.Args, ",") != "build" {
				t.Errorf("Tasks=%v Silent=%v Args=%v", o.Tasks, o.Silent, o.Args)
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			opts, err := Parse(c.argv)
			if err != nil {
				t.Fatalf("Parse(%v): %v", c.argv, err)
			}
			c.check(t, opts)
		})
	}
}

func TestParseQuiet(t *testing.T) {
	for _, argv := range [][]string{{"--silent"}, {"--tasks-simple"}, {"--tasks-json"}, {"--help"}, {"--version"}} {
		opts, err := Parse(argv)
		if err != nil {
			t.Fatal(err)
		}
		if !opts.Quiet() {
			t.Errorf("Parse(%v).Quiet() = false, want true", argv)
		}
	}
	opts, err := Parse([]string{"--tasks"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.Quiet() {
		t.Error("--tasks should not silence logging")
	}
}

func TestParseErrors(t *testing.T) {
	for _, argv := range [][]string{{"--gulpfile"}, {"--cwd"}} {
		if _, err := Parse(argv); err == nil {
			t.Errorf("Parse(%v) = nil error, want failure", argv)
		}
	}
}

func buildTree(t *testing.T) (*undertaker.Undertaker, *undertaker.Node) {
	t.Helper()
	u := undertaker.New()
	noop := func(context.Context) error { return nil }

	clean := u.Set("clean", noop)
	clean.Description = "Remove the build directory"
	clean.Flags = map[string]string{"--dry": "Report what would be removed"}
	u.Set("styles", noop)
	u.Set("scripts", noop)
	if _, err := u.SetTask("build", u.Series(
		undertaker.Name("clean"),
		u.Parallel(undertaker.Names("styles", "scripts")...),
	)); err != nil {
		t.Fatal(err)
	}
	return u, u.Tree(true)
}

func renderer(u *undertaker.Undertaker) TreeRenderer {
	return TreeRenderer{Messages: plain(), Lookup: u.Get}
}

func TestTreeRender(t *testing.T) {
	u, tree := buildTree(t)
	tree.Label = "Tasks for gulpfile.go"
	got := strings.Join(renderer(u).Render(tree), "\n")

	want := strings.Join([]string{
		"Tasks for gulpfile.go",
		"├── clean    Remove the build directory",
		"│   --dry    …Report what would be removed",
		"├── styles",
		"├── scripts",
		"└─┬ build",
		"  └─┬ <series>",
		"    ├── clean",
		"    └─┬ <parallel>",
		"      ├── styles",
		"      └── scripts",
	}, "\n")

	if got != want {
		t.Errorf("Render mismatch\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestTreeRenderRespectsDepth(t *testing.T) {
	u, tree := buildTree(t)
	lines := TreeRenderer{Messages: plain(), Lookup: u.Get, MaxDepth: 1}.Render(tree)
	for _, line := range lines[1:] {
		if strings.Contains(line, "<series>") {
			t.Fatalf("depth 1 should not descend into compositions: %q", line)
		}
	}
	if len(lines) != 6 {
		t.Errorf("got %d lines, want 6 (root, four tasks, one flag): %v", len(lines), lines)
	}
}

func TestTreeRenderSortsTopLevel(t *testing.T) {
	u, tree := buildTree(t)
	lines := TreeRenderer{Messages: plain(), Lookup: u.Get, SortTasks: true, MaxDepth: 1}.Render(tree)
	var names []string
	for _, line := range lines[1:] {
		label := strings.Fields(line)[1]
		if strings.HasPrefix(label, "--") {
			continue
		}
		names = append(names, label)
	}
	if strings.Join(names, ",") != "build,clean,scripts,styles" {
		t.Errorf("sorted labels = %v", names)
	}
}

func TestSimpleListUsesRegistrationOrder(t *testing.T) {
	u, _ := buildTree(t)
	if got := SimpleList(u.Tree(false)); got != "clean\nstyles\nscripts\nbuild" {
		t.Errorf("SimpleList() = %q", got)
	}
}

func TestTreeJSONDoesNotEscapeHTML(t *testing.T) {
	_, tree := buildTree(t)
	data, err := TreeJSON(tree, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "<series>") {
		t.Errorf("JSON escaped the composition label: %s", data)
	}
	var decoded undertaker.Node
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if decoded.Label != "Tasks" || len(decoded.Nodes) != 4 {
		t.Errorf("decoded = %+v", decoded)
	}
}

func TestTreeJSONDepth(t *testing.T) {
	_, tree := buildTree(t)
	data, err := TreeJSON(tree, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "<series>") {
		t.Errorf("depth 1 should prune compositions: %s", data)
	}
}

func TestTreeJSONLeavesCarryEmptyNodes(t *testing.T) {
	_, tree := buildTree(t)
	data, err := TreeJSON(tree, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`{"label":"clean","type":"task","nodes":[]}`,
		`{"label":"styles","type":"task","nodes":[]}`,
	} {
		if !strings.Contains(string(data), want) {
			t.Errorf("missing %s in %s", want, data)
		}
	}
}

func TestTreeJSONShallowNodesAreStrings(t *testing.T) {
	u, _ := buildTree(t)
	data, err := undertaker.MarshalTree(u.Tree(false))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"label":"Tasks","nodes":["clean","styles","scripts","build"]}`
	if string(data) != want {
		t.Errorf("MarshalTree(shallow) = %s, want %s", data, want)
	}
}

func TestMarshalTreeKeepsCompositionLabels(t *testing.T) {
	u, _ := buildTree(t)
	data, err := undertaker.MarshalTree(u.Tree(true))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"<series>"`) {
		t.Errorf("MarshalTree escaped the composition label: %s", data)
	}
}

func TestMessagesWording(t *testing.T) {
	m := plain()
	cases := [][2]string{
		{m.Gulpfile("/a/gulpfile.go"), "Using gulpfile /a/gulpfile.go"},
		{m.Description("/a/gulpfile.go"), "Tasks for /a/gulpfile.go"},
		{m.TaskStart("build"), "Starting 'build'..."},
		{m.TaskStop("build", 21*time.Millisecond), "Finished 'build' after 21 ms"},
		{m.TaskFailure("build", time.Second), "'build' errored after 1 s"},
		{m.MissingGulpfile(), "No gulpfile found"},
		{m.CwdChanged("/a"), "Working directory changed to /a"},
		{m.TaskMissing("buld", nil), "Task never defined: buld\nTo list available tasks, try running: gulp --tasks"},
		{
			m.TaskMissing("buld", []string{"build"}),
			"Task never defined: buld - did you mean? build\nTo list available tasks, try running: gulp --tasks",
		},
	}
	for _, c := range cases {
		if c[0] != c[1] {
			t.Errorf("got %q, want %q", c[0], c[1])
		}
	}
}

func TestSimilarTasks(t *testing.T) {
	candidates := []string{"build", "clean", "scripts", "styles"}
	if got := SimilarTasks("buld", candidates); strings.Join(got, ",") != "build" {
		t.Errorf("SimilarTasks(buld) = %v", got)
	}
	if got := SimilarTasks("zzzzzzzz", candidates); len(got) != 0 {
		t.Errorf("SimilarTasks(zzzzzzzz) = %v, want none", got)
	}
	if got := SimilarTasks("style", candidates); strings.Join(got, ",") != "styles" {
		t.Errorf("SimilarTasks(style) = %v", got)
	}
}

func TestPaletteDisabledIsIdentity(t *testing.T) {
	p := NewPalette(false, true)
	if p.Enabled() {
		t.Fatal("palette should be disabled")
	}
	if got := p.Cyan("build"); got != "build" {
		t.Errorf("Cyan = %q, want plain", got)
	}
}

func TestPaletteForcedEmitsANSI(t *testing.T) {
	p := NewPalette(true, true)
	got := p.Cyan("build")
	if got != "\x1b[36mbuild\x1b[39m" {
		t.Errorf("Cyan = %q", got)
	}
	if StripANSI(got) != "build" {
		t.Errorf("StripANSI(%q) = %q", got, StripANSI(got))
	}
	if DisplayWidth(got) != 5 {
		t.Errorf("DisplayWidth(%q) = %d, want 5", got, DisplayWidth(got))
	}
}

func TestDisplayWidth(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"build", 5},
		{"フォルダ", 8},
		{"a\u0301", 1},
		{"├── clean", 9},
	}
	for _, c := range cases {
		if got := DisplayWidth(c.in); got != c.want {
			t.Errorf("DisplayWidth(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestLoggerLevels(t *testing.T) {
	var out, errOut strings.Builder
	log := NewLogger(&out, &errOut, Palette{}, LevelInfo)
	log.Debug("invisible")
	log.Info("visible")
	log.Error("bad")

	if strings.Contains(out.String(), "invisible") {
		t.Errorf("debug leaked at info level: %q", out.String())
	}
	if !strings.Contains(out.String(), "visible") {
		t.Errorf("info missing: %q", out.String())
	}
	if !strings.Contains(errOut.String(), "bad") {
		t.Errorf("error should go to stderr: %q", errOut.String())
	}
	if strings.Contains(out.String(), "bad") {
		t.Errorf("error leaked to stdout: %q", out.String())
	}
}

func TestLoggerSilent(t *testing.T) {
	var out, errOut strings.Builder
	log := NewLogger(&out, &errOut, Palette{}, LevelSilent)
	log.Info("hi")
	log.Error("bad")
	if out.String() != "" || errOut.String() != "" {
		t.Errorf("silent logger wrote out=%q err=%q", out.String(), errOut.String())
	}
}

func TestLoggerTimestampFormat(t *testing.T) {
	var out strings.Builder
	log := NewLogger(&out, &out, Palette{}, LevelInfo)
	log.Info("hello")
	line := out.String()
	if len(line) < 11 || line[0] != '[' || line[9] != ']' || line[10] != ' ' {
		t.Fatalf("unexpected timestamp shape: %q", line)
	}
	if _, err := time.Parse("15:04:05", line[1:9]); err != nil {
		t.Errorf("timestamp %q is not HH:MM:SS: %v", line[1:9], err)
	}
	if !strings.HasSuffix(line, "hello\n") {
		t.Errorf("message missing: %q", line)
	}
}

func TestLoggerPrintHasNoTimestamp(t *testing.T) {
	var out strings.Builder
	NewLogger(&out, &out, Palette{}, LevelSilent).Print("├── clean")
	if out.String() != "├── clean\n" {
		t.Errorf("Print() = %q", out.String())
	}
}

func TestRunVersion(t *testing.T) {
	var out, errOut strings.Builder
	code := Run(Runtime{
		Undertaker: undertaker.New(),
		Stdout:     &out,
		Stderr:     &errOut,
	}, []string{"--version", "--no-color"})

	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "CLI version: "+Version) {
		t.Errorf("stdout = %q", out.String())
	}
}

func TestRunHelp(t *testing.T) {
	var out, errOut strings.Builder
	code := Run(Runtime{Undertaker: undertaker.New(), Stdout: &out, Stderr: &errOut}, []string{"--help", "--no-color"})
	if code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	for _, want := range []string{"Usage: gulp [options] tasks", "--tasks-simple", "--log-level"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("help output missing %q:\n%s", want, out.String())
		}
	}
}

func TestRunTask(t *testing.T) {
	u := undertaker.New()
	ran := false
	u.Set("default", func(context.Context) error { ran = true; return nil })

	var out, errOut strings.Builder
	code := Run(Runtime{Undertaker: u, Stdout: &out, Stderr: &errOut, Gulpfile: "gulpfile.go"}, []string{"--no-color"})

	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, errOut.String())
	}
	if !ran {
		t.Error("default task did not run")
	}
	for _, want := range []string{"Using gulpfile ", "gulpfile.go", "Starting 'default'...", "Finished 'default' after "} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("stdout missing %q:\n%s", want, out.String())
		}
	}
}

func TestRunMissingTask(t *testing.T) {
	u := undertaker.New()
	u.Set("build", func(context.Context) error { return nil })

	var out, errOut strings.Builder
	code := Run(Runtime{Undertaker: u, Stdout: &out, Stderr: &errOut}, []string{"--no-color", "buld"})

	if code != ExitError {
		t.Fatalf("exit = %d, want %d", code, ExitError)
	}
	if !strings.Contains(errOut.String(), "Task never defined: buld - did you mean? build") {
		t.Errorf("stderr = %q", errOut.String())
	}
}

func TestRunTaskFailure(t *testing.T) {
	u := undertaker.New()
	u.Set("default", func(context.Context) error { return errors.New("boom") })

	var out, errOut strings.Builder
	code := Run(Runtime{Undertaker: u, Stdout: &out, Stderr: &errOut}, []string{"--no-color"})

	if code != ExitError {
		t.Fatalf("exit = %d, want %d", code, ExitError)
	}
	if !strings.Contains(errOut.String(), "'default' errored after ") {
		t.Errorf("stderr missing failure line: %q", errOut.String())
	}
	if !strings.Contains(errOut.String(), "boom") {
		t.Errorf("stderr missing error text: %q", errOut.String())
	}
}

func TestRunBranchEventsHiddenByDefault(t *testing.T) {
	u := undertaker.New()
	u.Set("one", func(context.Context) error { return nil })
	if _, err := u.SetTask("default", u.Series(undertaker.Name("one"))); err != nil {
		t.Fatal(err)
	}

	var quiet, verbose strings.Builder
	if code := Run(Runtime{Undertaker: u, Stdout: &quiet, Stderr: &quiet}, []string{"--no-color"}); code != ExitOK {
		t.Fatalf("exit = %d: %s", code, quiet.String())
	}
	if strings.Contains(quiet.String(), "<series>") {
		t.Errorf("branch task visible at default level:\n%s", quiet.String())
	}

	if code := Run(Runtime{Undertaker: u, Stdout: &verbose, Stderr: &verbose}, []string{"--no-color", "-LLLL"}); code != ExitOK {
		t.Fatalf("exit = %d: %s", code, verbose.String())
	}
	if !strings.Contains(verbose.String(), "Starting '<series>'...") {
		t.Errorf("branch task hidden at debug level:\n%s", verbose.String())
	}
}

func TestRunTasksSimple(t *testing.T) {
	u, _ := buildTree(t)
	var out, errOut strings.Builder
	code := Run(Runtime{Undertaker: u, Stdout: &out, Stderr: &errOut}, []string{"--tasks-simple", "--no-color"})
	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, errOut.String())
	}
	if out.String() != "clean\nstyles\nscripts\nbuild\n" {
		t.Errorf("stdout = %q", out.String())
	}
}

func TestRunTasksSimpleIsUnlogged(t *testing.T) {
	u, _ := buildTree(t)
	var out strings.Builder
	Run(Runtime{Undertaker: u, Stdout: &out, Stderr: &out, Gulpfile: "gulpfile.go"}, []string{"--tasks-simple", "--no-color"})
	if strings.Contains(out.String(), "Using gulpfile") {
		t.Errorf("listing modes must suppress logging:\n%s", out.String())
	}
}

func TestRunSeriesFlag(t *testing.T) {
	u := undertaker.New()
	var order []string
	u.Set("a", func(context.Context) error {
		time.Sleep(30 * time.Millisecond)
		order = append(order, "a")
		return nil
	})
	u.Set("b", func(context.Context) error {
		order = append(order, "b")
		return nil
	})

	var out strings.Builder
	code := Run(Runtime{Undertaker: u, Stdout: &out, Stderr: &out}, []string{"--no-color", "--series", "a", "b"})
	if code != ExitOK {
		t.Fatalf("exit = %d: %s", code, out.String())
	}
	if strings.Join(order, ",") != "a,b" {
		t.Errorf("order = %v, want a,b", order)
	}
}

func TestRunContinueOnError(t *testing.T) {
	u := undertaker.New()
	ranSecond := false
	u.Set("a", func(context.Context) error { return errors.New("boom") })
	u.Set("b", func(context.Context) error { ranSecond = true; return nil })

	var out strings.Builder
	code := Run(Runtime{Undertaker: u, Stdout: &out, Stderr: &out}, []string{"--no-color", "--series", "--continue", "a", "b"})
	if code != ExitError {
		t.Fatalf("exit = %d, want %d", code, ExitError)
	}
	if !ranSecond {
		t.Error("--continue should run the remaining tasks")
	}
}

func TestTildify(t *testing.T) {
	if got := Tildify("/definitely/not/home"); got != "/definitely/not/home" {
		t.Errorf("Tildify() = %q", got)
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home directory")
	}
	if got := Tildify(home + "/project"); got != "~/project" {
		t.Errorf("Tildify(%q) = %q", home+"/project", got)
	}
}
