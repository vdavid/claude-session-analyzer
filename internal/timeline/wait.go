package timeline

import "github.com/vdavid/claude-session-analyzer/internal/transcript"

// waitInfo says what a lane was waiting for, read off the record that ended the wait.
func waitInfo(rec *transcript.Record) string { return "" }

// nameWaits annotates the lead's waiting rows with the teammates that were alive at the time.
func nameWaits(tl *Timeline) {}
