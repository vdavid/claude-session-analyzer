// Package transcript decodes Claude Code session transcripts: the JSONL files under ~/.claude/projects.
//
// The format is reverse-engineered and drifts between Claude Code versions, so the reader is deliberately forgiving.
// It skips record types it doesn't know rather than failing on them, steps over lines that aren't valid JSON, and
// counts both in [Stats] so a caller can account for every line it fed in.
//
// See docs/transcript-format.md for what the format looks like and how each claim about it was verified. Read that
// before changing anything here.
package transcript
