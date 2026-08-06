package dotenv

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadReadsTheShapesAnEnvFileComesIn(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, ".env"), `
# Ports for local development. Nothing here is secret.
CSA_BACKEND_PORT=19427

  CSA_FRONTEND_PORT = 19428   # the Vite dev server
export CSA_QUOTED="a value # with a hash"
CSA_SINGLE='single quoted'
CSA_EMPTY=
NOT A PAIR
`)

	env := Load(dir)

	for _, c := range []struct{ key, want string }{
		{"CSA_BACKEND_PORT", "19427"},
		{"CSA_FRONTEND_PORT", "19428"},
		{"CSA_QUOTED", "a value # with a hash"},
		{"CSA_SINGLE", "single quoted"},
		{"CSA_EMPTY", ""},
	} {
		if got := env.Get(c.key); got != c.want {
			t.Errorf("%s = %q, want %q", c.key, got, c.want)
		}
	}
	if _, ok := env["NOT A PAIR"]; ok {
		t.Error("a line with no `=` isn't a pair and shouldn't become one")
	}
}

func TestLoadFindsTheFileFurtherUp(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".env"), "CSA_BACKEND_PORT=19427\n")
	deep := filepath.Join(root, "apps", "website", "src")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatalf("make the directories: %v", err)
	}

	if got := Load(deep).Get("CSA_BACKEND_PORT"); got != "19427" {
		t.Errorf("port = %q, want the one from the `.env` three levels up", got)
	}
}

func TestLoadIsFineWithNoFileAtAll(t *testing.T) {
	if got := Load(t.TempDir()).Get("CSA_BACKEND_PORT"); got != "" {
		t.Errorf("port = %q, want empty", got)
	}
}

func TestTheProcessEnvironmentWins(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, ".env"), "CSA_BACKEND_PORT=19427\n")
	t.Setenv("CSA_BACKEND_PORT", "20000")

	if got := Load(dir).Get("CSA_BACKEND_PORT"); got != "20000" {
		t.Errorf("port = %q, want the one set on the process: a `.env` is a default, not an override", got)
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
