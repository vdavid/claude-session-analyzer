package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Rendering for the terminal: what a person reads, rather than what a machine parses. The API's JSON keeps the raw
// numbers.

// clip shortens s to width, ending in an ellipsis when it had to cut. It counts runes, so a title with an emoji in it
// doesn't come out a column short.
func clip(s string, width int) string {
	runes := []rune(s)
	if len(runes) <= width {
		return s
	}
	return strings.TrimRight(string(runes[:width-1]), " ") + "…"
}

// clipStart shortens a path from the front, which is where a path is least distinctive: two worktrees under the same
// project differ at the end.
func clipStart(s string, width int) string {
	runes := []rune(s)
	if len(runes) <= width {
		return s
	}
	return "…" + string(runes[len(runes)-width+1:])
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
