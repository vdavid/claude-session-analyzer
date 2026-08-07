package cli

import (
	"flag"
	"strings"
	"time"

	"github.com/vdavid/claude-session-analyzer/internal/session"
)

// scope is which sessions a command works on. Every command that can read more than one session takes the same three
// flags, so `sessions` and `stats` narrow the corpus the same way.
type scope struct {
	since   string
	until   string
	project string
}

func (s *scope) register(fs *flag.FlagSet) {
	fs.StringVar(&s.since, "since", "", "only sessions starting on or after this date (`2026-07-01`, or an RFC 3339 instant)")
	fs.StringVar(&s.until, "until", "", "only sessions starting on or before this date, the whole day included")
	fs.StringVar(&s.project, "project", "", "only sessions whose project path, name, or slug contains this")
}

// filter turns the flags into a predicate, or says which one didn't make sense.
func (s *scope) filter() (func(session.Summary) bool, error) {
	since, err := parseDate(s.since, false)
	if err != nil {
		return nil, err
	}
	until, err := parseDate(s.until, true)
	if err != nil {
		return nil, err
	}
	project := strings.ToLower(strings.TrimSpace(s.project))

	return func(sum session.Summary) bool {
		when := startedAt(sum)
		switch {
		case !since.IsZero() && when.Before(since):
			return false
		case !until.IsZero() && when.After(until):
			return false
		case project != "" && !matchesProject(sum, project):
			return false
		}
		return true
	}, nil
}

// any says the scope lets everything through, which is what makes a corpus-wide command worth warning about.
func (s *scope) any() bool { return s.since == "" && s.until == "" && s.project == "" }

// startedAt is when a session began, falling back to the transcript's mtime for one whose records carry no timestamp.
// That's the same fallback the listing sorts by, so a filter and a sort agree about where a session sits in time.
func startedAt(sum session.Summary) time.Time {
	if sum.Start.IsZero() {
		return sum.Modified
	}
	return sum.Start
}

func matchesProject(sum session.Summary, want string) bool {
	for _, field := range []string{sum.ProjectPath, sum.ProjectSlug} {
		if strings.Contains(strings.ToLower(field), want) {
			return true
		}
	}
	return false
}

// parseDate reads a date or an instant, in local time, because a person asking about July means their July. A date-only
// value with endOfDay set covers the whole day, so `--since 2026-07-01 --until 2026-07-31` is the month a person means
// rather than 30 days of it.
func parseDate(raw string, endOfDay bool) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}
	if t, err := time.ParseInLocation(time.RFC3339, raw, time.Local); err == nil {
		return t, nil
	}
	t, err := time.ParseInLocation(time.DateOnly, raw, time.Local)
	if err != nil {
		return time.Time{}, usagef("%q isn't a date. Use `2026-07-01`, or a full RFC 3339 instant.", raw)
	}
	if endOfDay {
		return t.AddDate(0, 0, 1).Add(-time.Nanosecond), nil
	}
	return t, nil
}
