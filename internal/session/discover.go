package session

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ErrNotFound means no session on disk matched the id that was looked for.
var ErrNotFound = errors.New("no session matched")

// AmbiguousIDError means a prefix matched more than one session. It names them all so a person can pick.
type AmbiguousIDError struct {
	ID string
	// Matches are `<project-slug>/<session-id>`, sorted.
	Matches []string
}

func (e *AmbiguousIDError) Error() string {
	return fmt.Sprintf("%q matches %d sessions: %s", e.ID, len(e.Matches), strings.Join(e.Matches, ", "))
}

// Location is where a session's files are, without having read any of them.
type Location struct {
	ID          string
	ProjectSlug string
	// TranscriptPath is the lead transcript.
	TranscriptPath string
	// DirPath is the sibling directory holding subagent lanes, empty when the session spawned none.
	DirPath string
	// SubagentPaths are the subagent transcripts, sorted by file name.
	SubagentPaths []string
}

// DefaultRoot is where Claude Code keeps its transcripts, honouring CLAUDE_CONFIG_DIR.
func DefaultRoot() (string, error) {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, "projects"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find the home directory: %w", err)
	}
	return filepath.Join(home, ".claude", "projects"), nil
}

// Find locates a session under root by its id. A prefix works too, as long as it matches exactly one session; an exact
// id always wins over a prefix.
func Find(root, id string) (Location, error) {
	slugs, err := os.ReadDir(root)
	if err != nil {
		return Location{}, fmt.Errorf("read the transcript root %s: %w", root, err)
	}

	var exact, prefixed []Location
	for _, slug := range slugs {
		if !slug.IsDir() {
			continue
		}
		entries, err := os.ReadDir(filepath.Join(root, slug.Name()))
		if err != nil {
			return Location{}, fmt.Errorf("read %s: %w", filepath.Join(root, slug.Name()), err)
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
				continue
			}
			found := strings.TrimSuffix(entry.Name(), ".jsonl")
			switch {
			case found == id:
				exact = append(exact, Location{ID: found, ProjectSlug: slug.Name()})
			case id != "" && strings.HasPrefix(found, id):
				prefixed = append(prefixed, Location{ID: found, ProjectSlug: slug.Name()})
			}
		}
	}

	matches := exact
	if len(matches) == 0 {
		matches = prefixed
	}

	switch len(matches) {
	case 0:
		return Location{}, fmt.Errorf("%w for %q under %s", ErrNotFound, id, root)
	case 1:
		return fill(root, matches[0])
	default:
		names := make([]string, 0, len(matches))
		for _, m := range matches {
			names = append(names, filepath.Join(m.ProjectSlug, m.ID))
		}
		sort.Strings(names)
		return Location{}, &AmbiguousIDError{ID: id, Matches: names}
	}
}

// fill completes a match with its paths, including any subagent lanes.
func fill(root string, loc Location) (Location, error) {
	dir := filepath.Join(root, loc.ProjectSlug)
	loc.TranscriptPath = filepath.Join(dir, loc.ID+".jsonl")

	sessionDir := filepath.Join(dir, loc.ID)
	switch info, err := os.Stat(sessionDir); {
	case errors.Is(err, os.ErrNotExist):
		return loc, nil
	case err != nil:
		return Location{}, fmt.Errorf("stat %s: %w", sessionDir, err)
	case !info.IsDir():
		return loc, nil
	}
	loc.DirPath = sessionDir

	subagentDir := filepath.Join(sessionDir, "subagents")
	entries, err := os.ReadDir(subagentDir)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return loc, nil
	case err != nil:
		return Location{}, fmt.Errorf("read %s: %w", subagentDir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}
		loc.SubagentPaths = append(loc.SubagentPaths, filepath.Join(subagentDir, entry.Name()))
	}
	sort.Strings(loc.SubagentPaths)
	return loc, nil
}
