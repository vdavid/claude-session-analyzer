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

// LaneFiles are the files holding one subagent lane.
type LaneFiles struct {
	// Rel is the lane's path inside a session directory: `subagents/agent-<id>.jsonl`, or
	// `subagents/workflows/wf_<id>/agent-<id>.jsonl` for a lane a workflow spawned. It's the lane's identity, because
	// it's the same under every directory the session wrote to.
	Rel string
	// Paths are the files themselves, sorted.
	Paths []string
}

// Location is where a session's files are, without having read any of them.
type Location struct {
	ID string
	// ProjectSlug is the directory holding the lead transcript.
	ProjectSlug string
	// TranscriptPath is the lead transcript.
	TranscriptPath string
	// DirPaths are the directories named after the session, in slug order, and empty when the session spawned no
	// subagents. There's more than one when the session entered a git worktree: the lead transcript stays where it
	// started while the lanes are written under the slug of wherever the session is at the time.
	DirPaths []string
	// SubagentLanes are the subagent lanes: the ones the session spawned directly, sorted by file name, then the ones
	// its workflows spawned, sorted by workflow then file name.
	SubagentLanes []LaneFiles
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
	dirs := map[string][]string{}
	for _, slug := range slugs {
		if !slug.IsDir() {
			continue
		}
		dir := filepath.Join(root, slug.Name())
		entries, err := os.ReadDir(dir)
		if err != nil {
			return Location{}, fmt.Errorf("read %s: %w", dir, err)
		}
		for _, entry := range entries {
			if entry.IsDir() {
				dirs[entry.Name()] = append(dirs[entry.Name()], filepath.Join(dir, entry.Name()))
				continue
			}
			if filepath.Ext(entry.Name()) != ".jsonl" {
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
		return fill(root, matches[0], dirs[matches[0].ID])
	default:
		names := make([]string, 0, len(matches))
		for _, m := range matches {
			names = append(names, filepath.Join(m.ProjectSlug, m.ID))
		}
		sort.Strings(names)
		return Location{}, &AmbiguousIDError{ID: id, Matches: names}
	}
}

// Locations is every session under root, sorted by project slug then id, each filled the way Find fills one.
//
// Find re-reads the whole root per lookup, so locating all 725 sessions on this machine that way is 725 scans. This is
// one, which is what a corpus walk wants.
//
// A project directory that can't be read is skipped rather than sinking the walk, the same way List treats one:
// transcripts get written while the tool runs, and the two have to agree on which sessions exist.
func Locations(root string) ([]Location, error) {
	slugs, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read the transcript root %s: %w", root, err)
	}

	var found []Location
	dirs := map[string][]string{}
	for _, slug := range slugs {
		if !slug.IsDir() {
			continue
		}
		dir := filepath.Join(root, slug.Name())
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() {
				dirs[name] = append(dirs[name], filepath.Join(dir, name))
				continue
			}
			if filepath.Ext(name) != ".jsonl" {
				continue
			}
			found = append(found, Location{ID: strings.TrimSuffix(name, ".jsonl"), ProjectSlug: slug.Name()})
		}
	}

	sort.Slice(found, func(i, j int) bool {
		if found[i].ProjectSlug != found[j].ProjectSlug {
			return found[i].ProjectSlug < found[j].ProjectSlug
		}
		return found[i].ID < found[j].ID
	})

	out := make([]Location, 0, len(found))
	for _, loc := range found {
		filled, err := fill(root, loc, dirs[loc.ID])
		if err != nil {
			return nil, err
		}
		out = append(out, filled)
	}
	return out, nil
}

// fill completes a match with its paths. dirs are every directory named after the session, wherever they sit.
func fill(root string, loc Location, dirs []string) (Location, error) {
	loc.TranscriptPath = filepath.Join(root, loc.ProjectSlug, loc.ID+".jsonl")
	if len(dirs) == 0 {
		return loc, nil
	}
	loc.DirPaths = dirs

	// A lane is its path inside a session directory, so the fragments of one lane written under different slugs group
	// back together rather than arriving as two lanes carrying one id.
	byRel := map[string][]string{}
	for _, dir := range dirs {
		paths, err := lanePathsIn(dir)
		if err != nil {
			return Location{}, err
		}
		for _, path := range paths {
			key := rel(dir, path)
			byRel[key] = append(byRel[key], path)
		}
	}

	// Sorting the lane paths puts the direct lanes first in file-name order, then the workflows' in workflow order,
	// because `agent-` sorts before `workflows/`.
	rels := make([]string, 0, len(byRel))
	for key := range byRel {
		rels = append(rels, key)
	}
	sort.Strings(rels)

	loc.SubagentLanes = make([]LaneFiles, 0, len(rels))
	for _, key := range rels {
		paths := byRel[key]
		sort.Strings(paths)
		loc.SubagentLanes = append(loc.SubagentLanes, LaneFiles{Rel: key, Paths: paths})
	}
	return loc, nil
}

// lanePathsIn lists every agent transcript in one session directory: the ones the session spawned directly, plus the
// ones its workflows spawned a level deeper, under `subagents/workflows/wf_<id>/`. Those are real lanes doing real
// work: one session on the machine this was built against holds 977 of them.
func lanePathsIn(sessionDir string) ([]string, error) {
	subagentDir := filepath.Join(sessionDir, "subagents")
	paths, err := laneFilesIn(subagentDir)
	if err != nil {
		return nil, err
	}

	workflows := filepath.Join(subagentDir, "workflows")
	entries, err := os.ReadDir(workflows)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read %s: %w", workflows, err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		lanes, err := laneFilesIn(filepath.Join(workflows, entry.Name()))
		if err != nil {
			return nil, err
		}
		paths = append(paths, lanes...)
	}
	return paths, nil
}

// rel is a lane file's path inside its session directory, which identifies the lane across directories.
func rel(sessionDir, path string) string {
	return strings.TrimPrefix(path, sessionDir+string(filepath.Separator))
}

// isLaneFile says a file name is an agent transcript. Taking only `agent-*.jsonl` leaves out a workflow's
// `journal.jsonl`: that's the workflow's own log of what it started and what came back, not a lane.
func isLaneFile(name string) bool {
	return strings.HasPrefix(name, "agent-") && filepath.Ext(name) == ".jsonl"
}

// laneFilesIn lists the agent transcripts directly inside dir, sorted.
func laneFilesIn(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}

	var paths []string
	for _, entry := range entries {
		if entry.IsDir() || !isLaneFile(entry.Name()) {
			continue
		}
		paths = append(paths, filepath.Join(dir, entry.Name()))
	}
	sort.Strings(paths)
	return paths, nil
}
