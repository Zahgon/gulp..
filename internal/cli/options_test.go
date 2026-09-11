package cli

import (
	"strings"
	"testing"
)

func parseOK(t *testing.T, argv ...string) *Options {
	t.Helper()
	opts, err := Parse(argv)
	if err != nil {
		t.Fatalf("Parse(%q) = %v", argv, err)
	}
	return opts
}

func parseErr(t *testing.T, argv ...string) string {
	t.Helper()
	opts, err := Parse(argv)
	if err == nil {
		t.Fatalf("Parse(%q) = %+v, want an error", argv, opts)
	}
	return err.Error()
}

func TestParseRejectsAnEmptyLongName(t *testing.T) {
	if msg := parseErr(t, "--=build"); !strings.Contains(msg, "unknown option") {
		t.Fatalf("error = %q", msg)
	}
}

func TestParseRejectsAnUnknownShortFlag(t *testing.T) {
	if msg := parseErr(t, "-Z"); !strings.Contains(msg, "unknown option: -Z") {
		t.Fatalf("error = %q", msg)
	}
}

func TestParseRejectsAShortFlagWithNoValue(t *testing.T) {
	if msg := parseErr(t, "-f"); !strings.Contains(msg, "--gulpfile requires a value") {
		t.Fatalf("error = %q", msg)
	}
}

func TestParseRejectsALongFlagWithNoValue(t *testing.T) {
	if msg := parseErr(t, "--cwd", "--silent"); !strings.Contains(msg, "--cwd requires a value") {
		t.Fatalf("error = %q", msg)
	}
}

func TestParseIgnoresANonNumericDepth(t *testing.T) {
	for _, argv := range [][]string{{"--tasks-depth", "deep"}, {"--depth=deep"}} {
		opts, err := Parse(argv)
		if err != nil {
			t.Fatalf("Parse(%v) = %v, want the yargs NaN fallback", argv, err)
		}
		if opts.Depth() != DefaultTasksDepth {
			t.Errorf("Parse(%v).Depth() = %d, want %d", argv, opts.Depth(), DefaultTasksDepth)
		}
	}
}

func TestParseRejectsANonBooleanFlagValue(t *testing.T) {
	if msg := parseErr(t, "--silent=maybe"); !strings.Contains(msg, "--silent requires a boolean") {
		t.Fatalf("error = %q", msg)
	}
}

func TestParseReadsAnExplicitBooleanValue(t *testing.T) {
	if opts := parseOK(t, "--silent=false"); opts.Silent {
		t.Fatal("--silent=false left Silent set")
	}
	if opts := parseOK(t, "--silent=true"); !opts.Silent {
		t.Fatal("--silent=true did not set Silent")
	}
	if opts := parseOK(t, "--continue=0"); opts.ContinueOnError {
		t.Fatal("--continue=0 left ContinueOnError set")
	}
}

func TestParseSetsTheListingBooleans(t *testing.T) {
	opts := parseOK(t, "--compact-tasks", "--sort-tasks", "--color")
	if !opts.CompactTasks || !opts.SortTasks || !opts.Color {
		t.Fatalf("opts = %+v", opts)
	}
}

func TestParseKeepsUnknownFlagsForTheGulpfile(t *testing.T) {
	// gulp forwards flags it does not define so a task can declare its own.
	opts := parseOK(t, "--env", "staging", "--watch", "--no-minify", "--tag=v1")
	want := map[string]string{"env": "staging", "watch": "true", "minify": "false", "tag": "v1"}
	for name, value := range want {
		if got := opts.Extra[name]; got != value {
			t.Errorf("Extra[%q] = %q, want %q", name, got, value)
		}
	}
	if len(opts.Args) != 0 {
		t.Fatalf("Args = %q, want none", opts.Args)
	}
}

func TestParseDoesNotSwallowAFlagAsAnUnknownFlagsValue(t *testing.T) {
	opts := parseOK(t, "--env", "--silent")
	if opts.Extra["env"] != "true" {
		t.Fatalf("Extra[env] = %q, want true", opts.Extra["env"])
	}
	if !opts.Silent {
		t.Fatal("--silent was consumed as a value")
	}
}

func TestParseClustersShortFlags(t *testing.T) {
	opts := parseOK(t, "-TS")
	if !opts.Tasks || !opts.Silent {
		t.Fatalf("opts = %+v", opts)
	}
}

func TestParseTakesAShortFlagValueFromTheRestOfTheCluster(t *testing.T) {
	if got := parseOK(t, "-fbuild.go").Gulpfile; got != "build.go" {
		t.Fatalf("Gulpfile = %q", got)
	}
	if got := parseOK(t, "-f", "build.go").Gulpfile; got != "build.go" {
		t.Fatalf("Gulpfile = %q", got)
	}
}

func TestParseCountsRepeatedLogLevelFlags(t *testing.T) {
	if got := parseOK(t, "-LL").LogLevel; got != Level(2) {
		t.Fatalf("LogLevel = %v, want 2", got)
	}
	if got := parseOK(t, "-LLLLLL").LogLevel; got != LevelDebug {
		t.Fatalf("clamped LogLevel = %v, want %v", got, LevelDebug)
	}
}

func TestParseTreatsEverythingAfterADashDashAsATaskName(t *testing.T) {
	opts := parseOK(t, "build", "--", "--silent", "-f")
	want := []string{"build", "--silent", "-f"}
	if strings.Join(opts.Args, ",") != strings.Join(want, ",") {
		t.Fatalf("Args = %q, want %q", opts.Args, want)
	}
	if opts.Silent {
		t.Fatal("a positional argument was parsed as a flag")
	}
}

func TestParseCollectsPreloadsUnderEitherName(t *testing.T) {
	opts := parseOK(t, "--preload", "one", "--require", "two")
	if strings.Join(opts.Preload, ",") != "one,two" {
		t.Fatalf("Preload = %q", opts.Preload)
	}
}

func TestParseTakesAPathForTasksJSONOnlyWhenOneFollows(t *testing.T) {
	opts := parseOK(t, "--tasks-json")
	if !opts.TasksJSON || opts.TasksJSONPath != "" {
		t.Fatalf("bare --tasks-json gave %+v", opts)
	}
	opts = parseOK(t, "--tasks-json", "out.json")
	if opts.TasksJSONPath != "out.json" {
		t.Fatalf("TasksJSONPath = %q", opts.TasksJSONPath)
	}
	opts = parseOK(t, "--tasks-json", "--silent")
	if opts.TasksJSONPath != "" || !opts.Silent {
		t.Fatalf("--tasks-json swallowed the next flag: %+v", opts)
	}
}

func TestDepthFallsBackToTheDefault(t *testing.T) {
	// A bare "-3" would be read as the next flag, so a negative depth has to
	// use the "--flag=value" spelling.
	for _, argv := range [][]string{nil, {"--tasks-depth", "0"}, {"--tasks-depth=-3"}} {
		if got := parseOK(t, argv...).Depth(); got != DefaultTasksDepth {
			t.Errorf("Parse(%q).Depth() = %d, want %d", argv, got, DefaultTasksDepth)
		}
	}
	if got := parseOK(t, "--tasks-depth", "2").Depth(); got != 2 {
		t.Fatalf("Depth() = %d, want 2", got)
	}
}

func TestListingAndQuiet(t *testing.T) {
	cases := []struct {
		argv    []string
		listing bool
		quiet   bool
	}{
		{nil, false, false},
		{[]string{"--silent"}, false, true},
		{[]string{"--help"}, true, true},
		{[]string{"--version"}, true, true},
		{[]string{"--tasks-simple"}, true, true},
		{[]string{"--tasks-json"}, true, true},
		{[]string{"--tasks"}, false, false},
	}
	for _, tc := range cases {
		opts := parseOK(t, tc.argv...)
		if opts.Listing() != tc.listing || opts.Quiet() != tc.quiet {
			t.Errorf("Parse(%q): listing=%v quiet=%v, want %v/%v",
				tc.argv, opts.Listing(), opts.Quiet(), tc.listing, tc.quiet)
		}
	}
}

func TestParseReadsASingleDashAsATaskName(t *testing.T) {
	if got := parseOK(t, "-").Args; len(got) != 1 || got[0] != "-" {
		t.Fatalf("Args = %q", got)
	}
}
