package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// inRepo runs the body from a directory holding a `.env`, which is what the binary sees when it runs inside the repo.
func inRepo(t *testing.T, env string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(env), 0o644); err != nil {
		t.Fatalf("write the `.env`: %v", err)
	}
	t.Chdir(dir)
}

func TestServeTakesItsPortFromTheEnvFile(t *testing.T) {
	inRepo(t, "CSA_BACKEND_PORT=19427\nCSA_FRONTEND_PORT=19428\n")

	cfg, err := resolveServeConfig(sessionRoot(), 0)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if cfg.address() != "127.0.0.1:19427" {
		t.Errorf("address = %q, want the `.env` port on the loopback address", cfg.address())
	}
	if !slices.Contains(cfg.frontendOrigins(), "http://127.0.0.1:19428") {
		t.Errorf("origins = %v, want the frontend's own", cfg.frontendOrigins())
	}
	if !slices.Contains(cfg.frontendOrigins(), "http://localhost:19428") {
		t.Errorf("origins = %v, want localhost too: a browser uses both names", cfg.frontendOrigins())
	}
}

func TestServeLetsTheFlagWin(t *testing.T) {
	inRepo(t, "CSA_BACKEND_PORT=19427\n")

	cfg, err := resolveServeConfig(sessionRoot(), 20001)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if cfg.address() != "127.0.0.1:20001" {
		t.Errorf("address = %q, want the port the flag asked for", cfg.address())
	}
}

func TestServeFallsBackWithNoEnvFileAtAll(t *testing.T) {
	t.Chdir(t.TempDir())

	cfg, err := resolveServeConfig(sessionRoot(), 0)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if cfg.backendPort != defaultBackendPort || cfg.frontendPort != defaultFrontendPort {
		t.Errorf("ports = %d and %d, want the built-in %d and %d",
			cfg.backendPort, cfg.frontendPort, defaultBackendPort, defaultFrontendPort)
	}
}

func TestServeSaysWhichSettingIsWrong(t *testing.T) {
	inRepo(t, "CSA_BACKEND_PORT=eleven\n")

	_, err := resolveServeConfig(sessionRoot(), 0)
	if err == nil {
		t.Fatal("want an error for a port that isn't a number")
	}
	for _, want := range []string{"CSA_BACKEND_PORT", "eleven", "`.env`"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message should mention %q: %q", want, err)
		}
	}
}

func TestServeRefusesAPortOutOfRange(t *testing.T) {
	t.Chdir(t.TempDir())

	if _, err := resolveServeConfig(sessionRoot(), 70000); err == nil {
		t.Error("want an error for a port above 65535")
	}
}

// TestServeNeverBindsAnythingButLoopback is the rule that doesn't bend: this tool reads every transcript on the
// machine, so it has no business on the network.
func TestServeNeverBindsAnythingButLoopback(t *testing.T) {
	t.Chdir(t.TempDir())

	cfg, err := resolveServeConfig(sessionRoot(), 20002)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !strings.HasPrefix(cfg.address(), "127.0.0.1:") {
		t.Errorf("address = %q, want it on 127.0.0.1", cfg.address())
	}
}
