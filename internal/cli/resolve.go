package cli

import (
	"errors"
	"fmt"
	"io/fs"
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
