package session

import (
	"fmt"
	"os"

	"github.com/vdavid/claude-session-analyzer/internal/transcript"
)

// Session is one parsed session: the lead lane plus one lane per subagent.
type Session struct {
	Location

	// Title is what Claude Code shows for the session. A title a person set wins over a generated one.
	Title string
	// ProjectPath is the directory the session started in, read off the first record that carries a `cwd`. The project
	// slug can't be inverted, and a session that entered a worktree reports a different `cwd` further in.
	ProjectPath string
	// Lanes hold the lead first, then the subagents in file-name order.
	Lanes []*Lane
}

// Lead returns the lead lane.
func (s *Session) Lead() *Lane {
	if len(s.Lanes) == 0 {
		return nil
	}
	return s.Lanes[0]
}

// Load reads every lane of a located session into memory. Parsing is cheap enough that nothing is cached; opts caps
// how much of each payload is kept.
func Load(loc Location, opts transcript.Options) (*Session, error) {
	s := &Session{Location: loc}

	lead, err := loadLane(loc.TranscriptPath, opts)
	if err != nil {
		return nil, err
	}
	lead.ID = loc.ID
	lead.Name = "lead"
	lead.IsLead = true
	s.Lanes = append(s.Lanes, lead)

	for _, path := range loc.SubagentPaths {
		lane, err := loadLane(path, opts)
		if err != nil {
			return nil, err
		}
		lane.ID = agentIDFromPath(path)
		lane.Meta = readAgentMeta(path)
		lane.Name = laneName(lane.Meta, lane.ID)
		s.Lanes = append(s.Lanes, lane)
	}

	s.Title = titleOf(lead)
	s.ProjectPath = projectPathOf(lead)
	return s, nil
}

func loadLane(path string, opts transcript.Options) (*Lane, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open transcript: %w", err)
	}
	defer f.Close() //nolint:errcheck // read-only

	lane := &Lane{Path: path}
	r := transcript.NewReader(f, opts)
	for r.Next() {
		lane.Records = append(lane.Records, r.Record())
	}
	lane.Stats = r.Stats()
	if err := r.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return lane, nil
}

// titleOf picks the session's title. All three kinds get rewritten as the session goes, so the last of each wins, and
// a title a person set beats a generated one.
func titleOf(lead *Lane) string {
	var custom, ai, agent string
	for _, rec := range lead.Records {
		switch rec.Type {
		case transcript.TypeCustomTitle:
			custom = rec.Title
		case transcript.TypeAITitle:
			ai = rec.Title
		case transcript.TypeAgentName:
			agent = rec.Title
		}
	}
	for _, title := range []string{custom, ai, agent} {
		if title != "" {
			return title
		}
	}
	return ""
}

func projectPathOf(lead *Lane) string {
	for _, rec := range lead.Records {
		if rec.CWD != "" {
			return rec.CWD
		}
	}
	return ""
}
