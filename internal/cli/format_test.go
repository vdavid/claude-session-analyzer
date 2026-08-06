package cli

import "testing"

func TestCountSeparatesThousands(t *testing.T) {
	for _, c := range []struct {
		in   int
		want string
	}{
		{0, "0"},
		{7, "7"},
		{999, "999"},
		{1000, "1,000"},
		{15944, "15,944"},
		{1221828, "1,221,828"},
		{-4438, "-4,438"},
	} {
		if got := count(c.in); got != c.want {
			t.Errorf("count(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestHumanBytesReadsLikeTheOperatingSystem(t *testing.T) {
	for _, c := range []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{999, "999 B"},
		{1000, "1.0 KB"},
		{304981, "305.0 KB"},
		{66984138, "67.0 MB"},
		{3468649374, "3.5 GB"},
	} {
		if got := humanBytes(c.in); got != c.want {
			t.Errorf("humanBytes(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestClipKeepsColumnsColumns(t *testing.T) {
	if got := clip("Widgets, counted", 44); got != "Widgets, counted" {
		t.Errorf("a title that fits should come back whole, got %q", got)
	}
	if got := clip("Fix the transfer dialog padding", 10); got != "Fix the t…" {
		t.Errorf("clip = %q", got)
	}
	// Runes, not bytes: an emoji title shouldn't come out a column short or cut in half.
	if got := clip("🦀🦀🦀🦀", 3); got != "🦀🦀…" {
		t.Errorf("clip = %q", got)
	}
}

func TestClipStartKeepsTheDistinctiveEndOfAPath(t *testing.T) {
	got := clipStart("~/projects-git/vdavid/cmdr/.claude/worktrees/david-i18n", 20)
	if want := "…orktrees/david-i18n"; got != want {
		t.Errorf("clipStart = %q, want %q", got, want)
	}
	if n := len([]rune(got)); n != 20 {
		t.Errorf("clipStart came back %d runes wide, want 20", n)
	}
	if got := clipStart("~/projects-git/vdavid/cmdr", 40); got != "~/projects-git/vdavid/cmdr" {
		t.Errorf("a path that fits should come back whole, got %q", got)
	}
}
