package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/vdavid/claude-session-analyzer/internal/session"
)

// fingerprintHex is how much of the hash a fingerprint keeps. Sixteen hex characters is 64 bits, which is far past
// what a corpus of a few thousand sessions needs, and short enough to read in a JSON file.
const fingerprintHex = 16

// Fingerprint identifies the state of every file a session is written across: the lead transcript plus each file of
// each subagent lane, which is several files under several project slugs when the session entered a worktree.
//
// Covering the lanes is the point. A subagent appends to its own transcript while the lead's mtime stays where it was,
// so a fingerprint over the lead alone reads a changed session as unchanged, and a stale digest is invisibly wrong.
//
// Each file contributes its path, its size, and its mtime. Size comes free with the mtime out of the same stat, and it
// catches what mtime alone misses: a coarse filesystem timestamp, and a restore or a copy that puts the old mtime back
// on different bytes.
//
// Paths go in relative to the transcript root, so a corpus that moved on disk is still the same corpus.
func Fingerprint(loc session.Location) (string, error) {
	paths := []string{loc.TranscriptPath}
	for _, lane := range loc.SubagentLanes {
		paths = append(paths, lane.Paths...)
	}

	root := transcriptRootOf(loc)
	lines := make([]string, 0, len(paths))
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return "", fmt.Errorf("stat %s: %w", path, err)
		}
		lines = append(lines, strings.Join([]string{
			relativeTo(root, path),
			strconv.FormatInt(info.Size(), 10),
			strconv.FormatInt(info.ModTime().UnixNano(), 10),
		}, "\x00"))
	}

	// Sorting means the order the lanes were discovered in can't move the fingerprint.
	sort.Strings(lines)

	h := sha256.New()
	for _, line := range lines {
		h.Write([]byte(line))
		h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))[:fingerprintHex], nil
}

// transcriptRootOf recovers the root a location was found under. A lead transcript is always `<root>/<slug>/<id>.jsonl`,
// and Location carries no root of its own.
func transcriptRootOf(loc session.Location) string {
	return filepath.Dir(filepath.Dir(loc.TranscriptPath))
}

// relativeTo is a file's path inside the transcript root, in slash form so the fingerprint reads the same on either
// kind of filesystem. A path that somehow sits outside the root keeps its full form rather than being silently
// shortened to something another file could collide with.
func relativeTo(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}
