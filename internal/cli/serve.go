package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/vdavid/claude-session-analyzer/internal/api"
	"github.com/vdavid/claude-session-analyzer/internal/dotenv"
)

const (
	// backendPortKey and frontendPortKey are the `.env` keys. The file is committed, because ports aren't secret and a
	// fresh clone should run.
	backendPortKey  = "CSA_BACKEND_PORT"
	frontendPortKey = "CSA_FRONTEND_PORT"
	// The fallbacks, for a binary run from outside the repo. They're the same numbers the committed `.env` holds.
	defaultBackendPort  = 19427
	defaultFrontendPort = 19428
	// loopback is the only address this listens on. A tool that reads every transcript on the machine has no business
	// on the network, so the address isn't configurable.
	loopback = "127.0.0.1"
)

// serveConfig is everything `serve` needs, resolved before anything binds a socket. Keeping it separate is what makes
// the precedence testable without a listener.
type serveConfig struct {
	root string
	// backendPort is what this listens on. frontendPort is the dev server's, which only matters because the browser
	// won't let that origin read an answer unless it's named.
	backendPort  int
	frontendPort int
}

func (c serveConfig) address() string { return fmt.Sprintf("%s:%d", loopback, c.backendPort) }

// frontendOrigins are the origins allowed to read an answer: the frontend's port, under both names a browser uses for
// the loopback address.
func (c serveConfig) frontendOrigins() []string {
	return []string{
		fmt.Sprintf("http://%s:%d", loopback, c.frontendPort),
		fmt.Sprintf("http://localhost:%d", c.frontendPort),
	}
}

func runServe(a *app, args []string) error {
	fs := newFlagSet(a, "serve")
	root := fs.String("root", "", "read transcripts from this directory instead of ~/.claude/projects")
	port := fs.Int("port", 0, "listen on this port instead of the one in `.env`")
	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return usagef("`%s serve` doesn't take arguments, and it got %q.", binary, rest[0])
	}

	cfg, err := resolveServeConfig(*root, *port)
	if err != nil {
		return err
	}

	handler := api.New(api.Options{Root: cfg.root, FrontendOrigins: cfg.frontendOrigins()})
	server := &http.Server{
		Addr:    cfg.address(),
		Handler: handler,
		// A local tool still shouldn't hold a connection open forever on a header that never finishes arriving.
		ReadHeaderTimeout: 10 * time.Second,
	}

	fmt.Fprintf(a.err, "Reading transcripts from %s.\n", cfg.root)
	fmt.Fprintf(a.err, "API on http://%s/api/sessions. Press Ctrl+C to stop.\n", cfg.address())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	done := make(chan error, 1)
	go func() { done <- server.ListenAndServe() }()

	select {
	case err := <-done:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("Couldn't listen on %s: %w", cfg.address(), err)
		}
		return nil
	case <-ctx.Done():
		fmt.Fprintln(a.err, "\nStopping.")
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			return fmt.Errorf("Couldn't stop cleanly: %w", err)
		}
		return nil
	}
}

// resolveServeConfig works out where to read from and what to listen on. A flag wins over the environment, which wins
// over the `.env` next to the repo, which wins over the built-in default.
func resolveServeConfig(rootFlag string, portFlag int) (serveConfig, error) {
	dir, err := transcriptRoot(rootFlag)
	if err != nil {
		return serveConfig{}, err
	}

	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	env := dotenv.Load(cwd)

	backend, err := resolvePort(portFlag, env.Get(backendPortKey), defaultBackendPort, backendPortKey)
	if err != nil {
		return serveConfig{}, err
	}
	frontend, err := resolvePort(0, env.Get(frontendPortKey), defaultFrontendPort, frontendPortKey)
	if err != nil {
		return serveConfig{}, err
	}
	return serveConfig{root: dir, backendPort: backend, frontendPort: frontend}, nil
}

// resolvePort picks a port and says which source was wrong when one is.
func resolvePort(flagValue int, envValue string, fallback int, key string) (int, error) {
	if flagValue != 0 {
		if !validPort(flagValue) {
			return 0, usagef("Port %d is out of range. Pick one between 1 and 65535, ideally a high one nothing "+
				"else uses.", flagValue)
		}
		return flagValue, nil
	}
	if envValue == "" {
		return fallback, nil
	}

	port, err := strconv.Atoi(envValue)
	if err != nil || !validPort(port) {
		return 0, fmt.Errorf("%s is %q, which isn't a port. Set it to a number between 1 and 65535 in `.env`, or "+
			"pass `--port`.", key, envValue)
	}
	return port, nil
}

func validPort(port int) bool { return port > 0 && port < 65536 }
