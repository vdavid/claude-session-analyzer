package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vdavid/claude-session-analyzer/internal/session"
)

const (
	digestExt = ".digest.json"
	detailExt = ".detail.json"
	// rootHex is how much of the transcript root's hash names its cache directory. Six bytes is plenty to keep two
	// roots on one machine apart, and short enough that the path stays readable.
	rootHex = 12
)

// Store is where derived digests live between runs: one directory per transcript root, one file per session per tier.
//
// One file per session, never one per project. Several agents query the corpus at once, and a file holding a project's
// sessions would mean read, modify, write, which means a lock. A whole file per session is written by rename, so the
// worst a race can do is have the last writer win with a digest that's just as valid.
type Store struct{ dir string }

// Info is what `cache info` reports.
type Info struct {
	Sessions int
	Bytes    int64
	// Oldest and Newest are when the digests on disk were built, and are zero when nothing is cached.
	Oldest, Newest time.Time
}

// Open picks the cache directory for one transcript root: `~/.cache/claude-session-analyzer/<hash of the root>`,
// honouring XDG_CACHE_HOME.
//
// The path is hashed rather than embedded, because a transcript root is an absolute path and a directory named after
// one is unreadable. Two roots on one machine still get their own directory, so a second corpus can't answer for the
// first.
//
// Nothing is created here. A read of an empty cache is a miss, and asking what's cached shouldn't leave a directory
// behind.
func Open(transcriptRoot string) (*Store, error) {
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("find the home directory: %w", err)
		}
		base = filepath.Join(home, ".cache")
	}

	root, err := filepath.Abs(transcriptRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve the transcript root %s: %w", transcriptRoot, err)
	}
	sum := sha256.Sum256([]byte(root))
	dir := filepath.Join(base, "claude-session-analyzer", hex.EncodeToString(sum[:])[:rootHex])

	// Claude Code's transcripts are the one thing here nobody can regenerate, so a cache that would land among them is
	// a refusal rather than a surprise. XDG_CACHE_HOME pointing inside the root is the way it happens.
	if abs, err := filepath.Abs(dir); err == nil && within(root, abs) {
		return nil, fmt.Errorf("the cache directory %s sits inside the transcript root %s", abs, root)
	}
	return &Store{dir: dir}, nil
}

// OpenAt uses a directory as given. Tests want a cache they can throw away.
func OpenAt(dir string) *Store { return &Store{dir: dir} }

// Dir is where this store keeps its files.
func (s *Store) Dir() string { return s.dir }

// LoadDigest reads a session's tier one, if what's on disk answers the question being asked.
//
// Anything doubtful is a miss rather than an error: an absent file, an unreadable one, a half written one, a digest
// from another version of the derivation, one built from different bytes, or one whose days were cut in another zone.
// A wrong digest is invisibly wrong, and re-deriving one session costs about a second.
func (s *Store) LoadDigest(loc session.Location, fingerprint string, zone *time.Location) (Digest, bool) {
	var d Digest
	if !readJSON(s.path(loc, digestExt), &d) {
		return Digest{}, false
	}
	if !valid(d.Version, d.Fingerprint, d.Zone, fingerprint, zone) {
		return Digest{}, false
	}
	return d, true
}

// LoadDetail reads a session's tier two under the same rules as LoadDigest.
func (s *Store) LoadDetail(loc session.Location, fingerprint string, zone *time.Location) (Detail, bool) {
	var d Detail
	if !readJSON(s.path(loc, detailExt), &d) {
		return Detail{}, false
	}
	if !valid(d.Version, d.Fingerprint, d.Zone, fingerprint, zone) {
		return Detail{}, false
	}
	return d, true
}

// Save writes both tiers for one session. Each file goes down as a temp file and a rename inside the same directory,
// so a reader either sees the previous whole file or the new whole file, and never half of either.
func (s *Store) Save(loc session.Location, d Digest, det Detail) error {
	dir := filepath.Join(s.dir, safeName(loc.ProjectSlug))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("make the cache directory %s: %w", dir, err)
	}
	name := safeName(loc.ID)
	if err := writeJSON(filepath.Join(dir, name+digestExt), d); err != nil {
		return err
	}
	return writeJSON(filepath.Join(dir, name+detailExt), det)
}

// Clear removes everything this store has cached. What's here is derived, so losing it costs one re-parse.
func (s *Store) Clear() error {
	if s.dir == "" || s.dir == string(filepath.Separator) {
		return fmt.Errorf("refusing to clear %q", s.dir)
	}
	if err := os.RemoveAll(s.dir); err != nil {
		return fmt.Errorf("clear the cache at %s: %w", s.dir, err)
	}
	return nil
}

// Info reports what's cached: how many sessions, what they cost on disk, and when the digests were built. An empty or
// absent cache reports zeroes rather than an error.
func (s *Store) Info() (Info, error) {
	var info Info
	err := s.walk(func(path string, entry fs.DirEntry) error {
		stat, err := entry.Info()
		if err != nil {
			return err
		}
		info.Bytes += stat.Size()
		if !strings.HasSuffix(path, digestExt) {
			return nil
		}
		info.Sessions++

		// Only the timestamp is wanted, but a digest is a few KB, so reading the file whole is cheaper than being
		// clever about it. A digest that won't parse still counts as a session; it's on disk either way.
		var built struct {
			Built time.Time `json:"built"`
		}
		if !readJSON(path, &built) || built.Built.IsZero() {
			return nil
		}
		if info.Oldest.IsZero() || built.Built.Before(info.Oldest) {
			info.Oldest = built.Built
		}
		if built.Built.After(info.Newest) {
			info.Newest = built.Built
		}
		return nil
	})
	if err != nil {
		return Info{}, err
	}
	return info, nil
}

// Prune drops what's cached for sessions that aren't on disk any more, and reports how many sessions it dropped. live
// holds the session ids that still exist.
//
// Nothing else in the cache expires. A digest is keyed on the bytes it was built from, so an old one is either still
// right or already a miss; the only waste is a session a person deleted.
func (s *Store) Prune(live map[string]bool) (int, error) {
	dropped := map[string]bool{}
	err := s.walk(func(path string, entry fs.DirEntry) error {
		id, ok := sessionIDOf(entry.Name())
		if !ok || live[id] {
			return nil
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("remove %s: %w", path, err)
		}
		dropped[id] = true
		return nil
	})
	if err != nil {
		return 0, err
	}
	s.removeEmptyProjects()
	return len(dropped), nil
}

// walk visits every file in the cache. A cache that was never written to is an empty one, not a failure.
func (s *Store) walk(visit func(path string, entry fs.DirEntry) error) error {
	err := filepath.WalkDir(s.dir, func(path string, entry fs.DirEntry, err error) error {
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return filepath.SkipAll
		case err != nil:
			return err
		case entry.IsDir():
			return nil
		}
		return visit(path, entry)
	})
	if err != nil {
		return fmt.Errorf("read the cache at %s: %w", s.dir, err)
	}
	return nil
}

// removeEmptyProjects tidies away a project directory a prune emptied. A directory that still holds something stays,
// and one that won't go is left alone: this is housekeeping, not the job.
func (s *Store) removeEmptyProjects() {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			_ = os.Remove(filepath.Join(s.dir, entry.Name()))
		}
	}
}

func (s *Store) path(loc session.Location, ext string) string {
	return filepath.Join(s.dir, safeName(loc.ProjectSlug), safeName(loc.ID)+ext)
}

// sessionIDOf reads the session id off a cache file name, and says no for anything that isn't one of ours.
func sessionIDOf(name string) (string, bool) {
	for _, ext := range []string{digestExt, detailExt} {
		if id, ok := strings.CutSuffix(name, ext); ok && id != "" {
			return id, true
		}
	}
	return "", false
}

// valid says whether a stored tier answers the question being asked.
func valid(storedVersion int, storedFingerprint, storedZone, fingerprint string, zone *time.Location) bool {
	return storedVersion == Version && storedFingerprint == fingerprint && storedZone == zoneName(zone)
}

// zoneName names the zone a digest's days were cut in. A nil zone means UTC, matching Build.
func zoneName(zone *time.Location) string {
	if zone == nil {
		return time.UTC.String()
	}
	return zone.String()
}

// readJSON fills v from a file, reporting only whether it worked. Every caller here treats a failure as a miss.
func readJSON(path string, v any) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return json.Unmarshal(data, v) == nil
}

// writeJSON puts v in a file whole: a temp file in the same directory, then a rename, which is atomic within a
// filesystem. A reader mid-rename sees the old file, and a writer that dies leaves a temp file rather than a truncated
// digest.
func writeJSON(path string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return fmt.Errorf("make a temporary file beside %s: %w", path, err)
	}
	defer os.Remove(tmp.Name()) //nolint:errcheck // gone already on the happy path

	if _, err := tmp.Write(data); err != nil {
		tmp.Close() //nolint:errcheck // the write already failed
		return fmt.Errorf("write %s: %w", tmp.Name(), err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmp.Name(), err)
	}
	// CreateTemp makes the file 0600, and there's nothing secret here.
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return fmt.Errorf("set the mode of %s: %w", tmp.Name(), err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("put %s in place: %w", path, err)
	}
	return nil
}

// safeName keeps a slug or an id to one path element. Both come from file names on disk, so this is a guardrail rather
// than a transformation anything real hits.
func safeName(name string) string {
	cleaned := strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == 0 {
			return '_'
		}
		return r
	}, name)
	switch cleaned {
	case "", ".", "..":
		return "_"
	}
	return cleaned
}

// within says whether path sits inside dir.
func within(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
