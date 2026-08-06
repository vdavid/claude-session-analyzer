package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/vdavid/claude-session-analyzer/internal/session"
)

// transcriptRoot is where to read from: what was asked for, or where Claude Code keeps its transcripts.
func transcriptRoot(flagValue string) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}
	root, err := session.DefaultRoot()
	if err != nil {
		return "", fmt.Errorf("Couldn't work out where the transcripts are: %w", err)
	}
	return root, nil
}

// missingRoot recognises the one problem worth its own message: there's nothing at the transcript root. It's what a
// fresh machine, a typo in `--root`, and a Claude Code that keeps its files elsewhere all look like.
func missingRoot(dir string, err error) error {
	if !errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return fmt.Errorf("There's no transcript directory at %s. Claude Code keeps its sessions under "+
		"`~/.claude/projects`, and `CLAUDE_CONFIG_DIR` moves that; point `--root` at yours if they live "+
		"somewhere else.", dir)
}

// resolve turns a session id or a prefix of one into a location, and turns the two ways that can go wrong into
// something worth reading. Both of them have an obvious next step, so both messages name it.
func resolve(root, id string) (session.Location, error) {
	loc, err := session.Find(root, id)
	if err == nil {
		return loc, nil
	}

	var ambiguous *session.AmbiguousIDError
	switch {
	case errors.Is(err, session.ErrNotFound):
		return session.Location{}, fmt.Errorf(
			"No session id starts with %q. Run `%s sessions` to see what's on disk.", id, binary)
	case errors.As(err, &ambiguous):
		var b strings.Builder
		fmt.Fprintf(&b, "%q matches %s sessions:\n", id, count(len(ambiguous.Matches)))
		for _, m := range ambiguous.Matches {
			fmt.Fprintf(&b, "  %s\n", m)
		}
		b.WriteString("Add a few more characters to pick one.")
		return session.Location{}, errors.New(b.String())
	default:
		if problem := missingRoot(root, err); problem != nil {
			return session.Location{}, problem
		}
		return session.Location{}, fmt.Errorf("Couldn't look for session %q: %w", id, err)
	}
}

// clip shortens s to width, ending in an ellipsis when it had to cut. It counts runes, so a title with an emoji in it
// doesn't come out a column short.
func clip(s string, width int) string {
	runes := []rune(s)
	if len(runes) <= width {
		return s
	}
	return strings.TrimRight(string(runes[:width-1]), " ") + "…"
}

// shortenHome writes a path the way a person does, so the project column stays readable.
func shortenHome(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" || path == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if rel := strings.TrimPrefix(path, home+string(filepath.Separator)); rel != path {
		return "~" + string(filepath.Separator) + rel
	}
	return path
}

// count renders a number with thousands separators, because a session with 15831 rows reads as a typo without them.
func count(n int) string {
	s := strconv.Itoa(n)
	sign := ""
	if strings.HasPrefix(s, "-") {
		sign, s = "-", s[1:]
	}

	var b strings.Builder
	for i, digit := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(digit)
	}
	return sign + b.String()
}

// humanBytes renders a size the way the operating system does, in powers of a thousand.
func humanBytes(b int64) string {
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	value, exp := float64(b)/unit, 0
	for value >= unit && exp < 3 {
		value /= unit
		exp++
	}
	return fmt.Sprintf("%.1f %s", value, [...]string{"KB", "MB", "GB", "TB"}[exp])
}
