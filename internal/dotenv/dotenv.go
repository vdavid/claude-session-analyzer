// Package dotenv reads the committed `.env` that holds this repo's dev ports.
//
// It's a dozen lines rather than a dependency, because what it has to understand is a dozen lines: `KEY=value`, with
// comments, blank lines, an optional `export`, and optional quotes. Anything fancier belongs in a config file.
package dotenv

import (
	"os"
	"path/filepath"
	"strings"
)

// FileName is what the file is called, at the repo root.
const FileName = ".env"

// Env is one parsed `.env`. A missing file gives an empty one, which every lookup then falls through.
type Env map[string]string

// Load reads the nearest `.env` at or above dir. Walking up is what lets the binary run from anywhere inside the repo
// and still find the ports.
func Load(dir string) Env {
	env := Env{}

	dir, err := filepath.Abs(dir)
	if err != nil {
		return env
	}
	for {
		raw, err := os.ReadFile(filepath.Join(dir, FileName))
		if err == nil {
			parse(string(raw), env)
			return env
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return env
		}
		dir = parent
	}
}

// Get returns the value for key: the process environment first, then the file. A `.env` is a default a person can
// override for one run, not something that overrides them.
func (e Env) Get(key string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return e[key]
}

func parse(content string, into Env) {
	for line := range strings.SplitSeq(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		into[strings.TrimSpace(key)] = unquote(strings.TrimSpace(value))
	}
}

// unquote takes the quotes off a value, and strips a trailing comment off one that has none. A quoted value is taken
// literally, hashes included, which is the only way to write a value containing one.
func unquote(value string) string {
	for _, quote := range []string{`"`, `'`} {
		if len(value) >= 2 && strings.HasPrefix(value, quote) && strings.HasSuffix(value, quote) {
			return value[1 : len(value)-1]
		}
	}
	if i := strings.Index(value, " #"); i >= 0 {
		value = value[:i]
	}
	return strings.TrimSpace(value)
}
