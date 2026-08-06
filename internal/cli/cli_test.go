package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	alphaID  = "11111111-1111-1111-1111-111111111111"
	soloID   = "22222222-2222-2222-2222-222222222222"
	goldenID = "11111111-2222-3333-4444-555555555555"
)

// sessionRoot holds four fixture sessions; goldenRoot holds the one the derivation's golden file was built from.
func sessionRoot() string { return filepath.Join("..", "session", "testdata", "projects") }
func goldenRoot() string  { return filepath.Join("..", "timeline", "testdata", "projects") }
func goldenCSV() string   { return filepath.Join("..", "timeline", "testdata", "golden", "timeline.csv") }

func run(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errs bytes.Buffer
	code = Run(args, &out, &errs)
	return code, out.String(), errs.String()
}

func TestSessionsListsWhatIsOnDisk(t *testing.T) {
	code, stdout, stderr := run(t, "sessions", "--root", sessionRoot())

	if code != 0 {
		t.Fatalf("exit code = %d, stderr: %s", code, stderr)
	}
	for _, want := range []string{"Session", "Started", "Subagents", alphaID, "Widgets, counted", "/tmp/alpha"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("listing should mention %q:\n%s", want, stdout)
		}
	}
	if !strings.Contains(stdout, "4 sessions") {
		t.Errorf("listing should total what it found:\n%s", stdout)
	}
}

func TestSessionsPutsTheNewestFirst(t *testing.T) {
	_, stdout, _ := run(t, "sessions", "--root", sessionRoot())

	newest := strings.Index(stdout, "33333333-bbbb")
	oldest := strings.Index(stdout, alphaID)
	if newest < 0 || oldest < 0 {
		t.Fatalf("both sessions should be listed:\n%s", stdout)
	}
	if newest > oldest {
		t.Errorf("the newest session should come first:\n%s", stdout)
	}
}

func TestSessionsHonoursTheLimitAndSaysItCapped(t *testing.T) {
	code, stdout, stderr := run(t, "sessions", "--root", sessionRoot(), "--limit", "2")

	if code != 0 {
		t.Fatalf("exit code = %d, stderr: %s", code, stderr)
	}
	if strings.Contains(stdout, alphaID) {
		t.Errorf("a limit of two should leave the oldest session out:\n%s", stdout)
	}
	if !strings.Contains(stdout, "2 of 4 sessions") {
		t.Errorf("a capped listing should say what it left out:\n%s", stdout)
	}
}

func TestSessionsShowsEverythingWithoutALimit(t *testing.T) {
	_, stdout, _ := run(t, "sessions", "--root", sessionRoot(), "--limit", "0")

	for _, id := range []string{alphaID, soloID, "33333333-aaaa", "33333333-bbbb"} {
		if !strings.Contains(stdout, id) {
			t.Errorf("`--limit 0` should list every session, missing %q:\n%s", id, stdout)
		}
	}
}

// TestTimelineWritesTheEnginesCSV holds the CLI to the same bytes the derivation's golden file holds, so the two can't
// drift apart: the command is a surface over the engine, not a second opinion.
func TestTimelineWritesTheEnginesCSV(t *testing.T) {
	code, stdout, stderr := run(t, "timeline", goldenID, "--root", goldenRoot())

	if code != 0 {
		t.Fatalf("exit code = %d, stderr: %s", code, stderr)
	}
	want, err := os.ReadFile(goldenCSV())
	if err != nil {
		t.Fatalf("read the golden CSV: %v", err)
	}
	if stdout != string(want) {
		t.Errorf("the CSV on stdout isn't the golden one:\ngot:\n%s\nwant:\n%s", stdout, want)
	}
}

func TestTimelineTakesAUniquePrefix(t *testing.T) {
	code, stdout, stderr := run(t, "timeline", "11111111-2222", "--root", goldenRoot())

	if code != 0 {
		t.Fatalf("exit code = %d, stderr: %s", code, stderr)
	}
	if !strings.HasPrefix(stdout, "From,Until,Agent,Activity,Extra info,Duration (s)") {
		t.Errorf("stdout should start with the CSV header:\n%s", stdout)
	}
}

func TestTimelineWritesToAFileAndSaysSo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "timeline.csv")
	code, stdout, stderr := run(t, "timeline", goldenID, "--root", goldenRoot(), "--out", path)

	if code != 0 {
		t.Fatalf("exit code = %d, stderr: %s", code, stderr)
	}
	if stdout != "" {
		t.Errorf("stdout should stay empty when the CSV goes to a file:\n%s", stdout)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read what was written: %v", err)
	}
	want, err := os.ReadFile(goldenCSV())
	if err != nil {
		t.Fatalf("read the golden CSV: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("the file isn't the golden CSV:\n%s", got)
	}
	for _, phrase := range []string{"rows", path} {
		if !strings.Contains(stderr, phrase) {
			t.Errorf("the note on stderr should mention %q, got %q", phrase, stderr)
		}
	}
}

func TestTimelineSaysWhatToDoNextForAnUnknownID(t *testing.T) {
	code, _, stderr := run(t, "timeline", "no-such-session", "--root", sessionRoot())

	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "no-such-session") {
		t.Errorf("the message should repeat the id that was looked for: %q", stderr)
	}
	if !strings.Contains(stderr, "sessions") {
		t.Errorf("the message should point at the command that lists what's on disk: %q", stderr)
	}
}

func TestTimelineNamesTheCandidatesForAnAmbiguousID(t *testing.T) {
	code, _, stderr := run(t, "timeline", "33333333", "--root", sessionRoot())

	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	for _, want := range []string{"33333333-aaaa", "33333333-bbbb", "characters"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("the message should name the candidates and say how to pick, missing %q: %q", want, stderr)
		}
	}
}

func TestTimelineWantsASessionID(t *testing.T) {
	code, _, stderr := run(t, "timeline")

	if code != 2 {
		t.Errorf("exit code = %d, want 2 for a usage problem", code)
	}
	if !strings.Contains(stderr, "session id") {
		t.Errorf("the message should say what's missing: %q", stderr)
	}
}

// TestAMissingRootSaysWhereTranscriptsLive covers the fresh machine and the mistyped `--root`, which look the same
// from here and have the same answer.
func TestAMissingRootSaysWhereTranscriptsLive(t *testing.T) {
	absent := filepath.Join(t.TempDir(), "absent")

	// `serve` is in here because it has to notice before it binds a socket: a server that starts and then answers 500
	// to everything makes a person read a log to find out what a message could have told them.
	for _, args := range [][]string{
		{"sessions", "--root", absent},
		{"timeline", alphaID, "--root", absent},
		{"serve", "--root", absent},
	} {
		code, _, stderr := run(t, args...)
		if code != 1 {
			t.Errorf("%v: exit code = %d, want 1", args, code)
		}
		for _, want := range []string{absent, "~/.claude/projects", "--root"} {
			if !strings.Contains(stderr, want) {
				t.Errorf("%v: the message should mention %q: %q", args, want, stderr)
			}
		}
	}
}

func TestNoCommandPrintsTheUsage(t *testing.T) {
	code, _, stderr := run(t)

	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	for _, want := range []string{"timeline", "sessions", "serve"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("the usage should list %q: %q", want, stderr)
		}
	}
}

func TestAnUnknownCommandRepeatsIt(t *testing.T) {
	code, _, stderr := run(t, "tokens")

	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "tokens") {
		t.Errorf("the message should repeat the command that wasn't understood: %q", stderr)
	}
}

func TestHelpGoesToStdout(t *testing.T) {
	code, stdout, _ := run(t, "help")

	if code != 0 {
		t.Errorf("exit code = %d, want 0: asking for help isn't a mistake", code)
	}
	if !strings.Contains(stdout, "timeline") {
		t.Errorf("help should list the commands:\n%s", stdout)
	}
}
