// The differential probes need a gulp checkout and Node, which are not
// present everywhere this suite runs. Rather than skip the comparison there,
// gulp's side of it is recorded once into testdata/gulp_oracle.json and
// replayed. The port is still computed live on every run and compared against
// that recording, so the comparison never silently stops happening.
//
// When GULP_JS_REPO does point at a gulp checkout the probes also run for
// real, and the recording is checked against them, so it cannot drift away
// from the implementation it claims to describe. Re-record after changing a
// probe:
//
//	npm install gulp@5.0.1 --prefix /tmp/gulp-js
//	GULP_ORACLE_RECORD=1 GULP_JS_REPO=/tmp/gulp-js/node_modules/gulp \
//	  go test -run Differential -count=1 .
package gulp_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

const (
	oraclePath    = "testdata/gulp_oracle.json"
	recordCommand = "GULP_ORACLE_RECORD=1 GULP_JS_REPO=<gulp checkout> go test -run Differential -count=1 ."
)

// gulpOracle is what gulp produced for each probe, split by shape so the
// recording stays reviewable: src is the file list gulp.src emitted, dest is
// the tree gulp.dest wrote, and text is raw probe stdout.
type gulpOracle struct {
	Version string                       `json:"gulp_version"`
	Src     map[string][]jsFile          `json:"src"`
	Dest    map[string]map[string]string `json:"dest"`
	Text    map[string]string            `json:"text"`

	repo   string
	record bool
}

// loadOracle reads the recording and works out whether the probes can also be
// run for real this time.
func loadOracle(t *testing.T) *gulpOracle {
	t.Helper()

	o := &gulpOracle{
		Src:  map[string][]jsFile{},
		Dest: map[string]map[string]string{},
		Text: map[string]string{},
	}

	raw, err := os.ReadFile(oraclePath)
	switch {
	case err == nil:
		if err := json.Unmarshal(raw, o); err != nil {
			t.Fatalf("decode %s: %v", oraclePath, err)
		}
	case os.IsNotExist(err):
		// Only a recording run may start without one.
	default:
		t.Fatalf("read %s: %v", oraclePath, err)
	}

	o.repo = liveRepo(t)
	o.record = os.Getenv("GULP_ORACLE_RECORD") != ""
	if o.record && o.repo == "" {
		t.Fatalf("GULP_ORACLE_RECORD needs GULP_JS_REPO and node to record from")
	}
	return o
}

// liveRepo returns a gulp checkout to run the probes against, or "" when it or
// Node is missing. Unlike a skip, an empty result still leaves the recorded
// comparison running.
func liveRepo(t *testing.T) string {
	t.Helper()

	repo := os.Getenv("GULP_JS_REPO")
	if repo == "" {
		return ""
	}
	abs, err := filepath.Abs(repo)
	if err != nil {
		t.Fatalf("resolve GULP_JS_REPO: %v", err)
	}
	if _, err := os.Stat(filepath.Join(abs, "index.js")); err != nil {
		t.Fatalf("GULP_JS_REPO=%s does not look like gulp: %v", abs, err)
	}
	if _, err := exec.LookPath("node"); err != nil {
		return ""
	}
	return abs
}

// replay returns gulp's recorded answer for key. With a live checkout it runs
// the probe instead and checks the recording still matches, so a stale
// recording fails loudly rather than pinning behaviour gulp has since changed.
func replay[T any](t *testing.T, o *gulpOracle, section map[string]T, key string, live func(repo string) T) T {
	t.Helper()

	recorded, ok := section[key]
	if o.repo == "" {
		if !ok {
			t.Fatalf("no recording for %q in %s; re-record with %s", key, oraclePath, recordCommand)
		}
		return recorded
	}

	got := live(o.repo)
	if o.record {
		section[key] = got
		o.save(t)
		return got
	}
	if !ok {
		t.Fatalf("no recording for %q in %s; re-record with %s", key, oraclePath, recordCommand)
	}
	if gotJSON, wantJSON := mustJSON(t, got), mustJSON(t, recorded); gotJSON != wantJSON {
		t.Fatalf("the recording for %q no longer matches gulp\n  gulp:     %s\n  recorded: %s\nre-record with %s",
			key, gotJSON, wantJSON, recordCommand)
	}
	return got
}

// save rewrites the recording, merging into whatever is already there so a
// partial run does not drop the probes it did not exercise.
func (o *gulpOracle) save(t *testing.T) {
	t.Helper()

	if o.Version == "" {
		o.Version = gulpVersion(t, o.repo)
	}
	encoded, err := json.MarshalIndent(o, "", "  ")
	if err != nil {
		t.Fatalf("encode oracle: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(oraclePath), 0o755); err != nil {
		t.Fatalf("create testdata: %v", err)
	}
	if err := os.WriteFile(oraclePath, append(encoded, '\n'), 0o644); err != nil {
		t.Fatalf("write %s: %v", oraclePath, err)
	}
}

// gulpVersion reads the version out of the checkout being recorded so the
// recording says which gulp it came from.
func gulpVersion(t *testing.T, repo string) string {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(repo, "package.json"))
	if err != nil {
		t.Fatalf("read gulp package.json: %v", err)
	}
	var pkg struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(raw, &pkg); err != nil {
		t.Fatalf("decode gulp package.json: %v", err)
	}
	return pkg.Version
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()

	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode for comparison: %v", err)
	}
	return string(encoded)
}
