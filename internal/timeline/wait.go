package timeline

import (
	"container/heap"
	"sort"
	"strconv"
	"strings"

	"github.com/vdavid/claude-session-analyzer/internal/session"
	"github.com/vdavid/claude-session-analyzer/internal/transcript"
)

// The harness wraps input from another agent in an envelope naming the sender, and a background task's notification in
// one of its own. Both are structure the harness writes, not prose a model wrote, so reading them is parsing rather
// than guessing at wording. See `docs/transcript-format.md`.
const (
	teammateEnvelope = "<teammate-message "
	taskEnvelope     = "<task-notification>"
	// taskCallElement is the notification's link back to the call that started the task, which is what says whose task
	// it was. 9,884 of the corpus's 12,076 notifications carry one (verified 2026-08-07).
	taskCallElement = "tool-use-id"
	// envelopeSearchLimit is how far into a message the envelope is looked for. A lead's copy comes after one line of
	// preamble; anything further in is quoted text rather than the message's own wrapper.
	envelopeSearchLimit = 200
)

// waitedFor says what a lane was waiting for, read off whatever ended the wait: the record that arrived is the signal.
//
// The third result is the tool-use id of the task a notification reported on, empty for every other kind of wait. It's
// handed back rather than kept on the row because whose task it was can't be answered yet: it takes every lane's calls,
// and every lane's span. attributeTaskWaits settles it once the walk is done.
//
// The two envelopes never share a message. Across the corpus's 12,437 envelope-carrying records, not one carries both
// (verified 2026-08-06), so the order these are checked in doesn't decide anything.
func waitedFor(rec *transcript.Record) (Kind, string, string) {
	text := rec.Prompt
	if text == "" && rec.Queue != nil {
		text = rec.Queue.Content
	}
	if text == "" {
		for _, b := range rec.Blocks {
			if b.Type == transcript.BlockText {
				text = b.Text
				break
			}
		}
	}

	if from, ok := teammateSender(text); ok {
		return KindWaitingForTeammate, "waiting for teammate " + from, ""
	}
	if start := strings.Index(head(text), taskEnvelope); start >= 0 {
		return KindWaitingForTask, "waiting for a background task", element(text[start:], taskCallElement)
	}
	if rec.Type == transcript.TypeQueueOperation {
		return KindWaitingForPerson, "waiting for the next prompt, which arrived queued", ""
	}
	return KindWaitingForPerson, "waiting for the next prompt", ""
}

// attributeTaskWaits hands the waits a teammate's still-running task closed to the teammate.
//
// A notification says a task finished, not that the lane was waiting for the task. On session 532ac591 the lead sat
// idle 25m30s and was woken by a 0.14 s poll loop that the subagent `m1-honesty` had left in the background: the lead
// was waiting for `m1-honesty`, which was alive and working the whole time, and calling that a background task's wait
// claims the app took 25 minutes to launch.
//
// Two things have to hold, and dropping either gets a real case wrong. The task belongs to another lane, because a lane
// waiting on its own task is waiting on the task. And that lane was still running when the gap ended, because a teammate
// that starts a 40-minute build, reports back, and exits leaves the lead genuinely waiting on the build.
//
// An id no lane claims keeps what it had, which is what a notification carrying none and one whose owner's transcript
// was compacted away both look like. Unknown means unchanged, never a guess.
func attributeTaskWaits(tl *Timeline, owners map[string]string, waits map[int]string) {
	if len(waits) == 0 {
		return
	}
	spans := make(map[string]LaneSpan, len(tl.Lanes))
	for _, lane := range tl.Lanes {
		spans[lane.ID] = lane
	}

	for i, task := range waits {
		row := &tl.Rows[i]
		owner, ok := owners[task]
		if !ok || owner == row.LaneID {
			continue
		}
		span, ok := spans[owner]
		if !ok || span.First.After(row.Until) || row.Until.After(span.Last) {
			continue
		}
		row.Kind = KindWaitingForTeammate
		// Phrased like every other teammate wait, with how it was established on the end, so a reader can tell this one
		// came from a task rather than from a message.
		row.Info = "waiting for teammate " + span.Name + ", via its background task"
	}
}

// toolUseOwners maps every tool-use id in the session to the lane that issued it. It covers every lane rather than the
// one being walked, because the task that woke a lane was nearly always started by a different one.
func toolUseOwners(s *session.Session) map[string]string {
	owners := map[string]string{}
	for _, lane := range s.Lanes {
		for _, rec := range lane.Records {
			for _, b := range rec.Blocks {
				if b.Type == transcript.BlockToolUse && b.ToolUseID != "" {
					owners[b.ToolUseID] = lane.ID
				}
			}
		}
	}
	return owners
}

// element reads one `<name>value</name>` out of a tag's body, empty when the element isn't there.
func element(tag, name string) string {
	openTag, closeTag := "<"+name+">", "</"+name+">"
	start := strings.Index(tag, openTag)
	if start < 0 {
		return ""
	}
	rest := tag[start+len(openTag):]
	end := strings.Index(rest, closeTag)
	if end < 0 {
		return ""
	}
	return rest[:end]
}

// teammateSender reads the sender out of a teammate message's envelope.
func teammateSender(text string) (string, bool) {
	start := strings.Index(head(text), teammateEnvelope)
	if start < 0 {
		return "", false
	}
	return attribute(text[start:], "teammate_id")
}

// attribute reads one `key="value"` out of an opening tag.
func attribute(tag, key string) (string, bool) {
	i := strings.Index(tag, key+`="`)
	if i < 0 {
		return "", false
	}
	rest := tag[i+len(key)+2:]
	end := strings.IndexByte(rest, '"')
	if end < 0 {
		return "", false
	}
	return rest[:end], true
}

func head(text string) string {
	if len(text) > envelopeSearchLimit {
		return text[:envelopeSearchLimit]
	}
	return text
}

// spawnInfo names the teammate an `Agent` call created. The result carries the teammate's id, name, and model, which
// is a direct link from the call to the lane it started rather than a guess from matching names.
func spawnInfo(c *call) string {
	if c.class != ClassAgent || c.record == nil {
		return ""
	}
	name, ok := payloadString(c.record, "name")
	if !ok {
		return ""
	}
	info := "spawned teammate " + name
	if model, ok := payloadString(c.record, "model"); ok {
		info += " (" + model + ")"
	}
	return info
}

// nameWaits annotates the lead's waiting rows with the teammates that were alive at the time, which is how a reader
// tells "the lead was blocked on four agents" from "the lead was blocked on nobody".
//
// It's a sweep rather than a scan per row: the lead's rows come out in time order and the lanes are sorted by start,
// so a heap of the lanes still running carries the answer forward. A session with a thousand workflow lanes would
// otherwise cost a thousand comparisons per row.
func nameWaits(tl *Timeline, opts Options) {
	if len(tl.Lanes) == 0 || !tl.Lanes[0].IsLead {
		return
	}
	leadID := tl.Lanes[0].ID

	lanes := make([]LaneSpan, 0, len(tl.Lanes))
	for _, lane := range tl.Lanes[1:] {
		lanes = append(lanes, lane)
	}
	if len(lanes) == 0 {
		return
	}
	sort.Slice(lanes, func(i, j int) bool { return lanes[i].First.Before(lanes[j].First) })

	next := 0
	live := &laneHeap{}
	for i := range tl.Rows {
		row := &tl.Rows[i]
		if !row.Kind.IsWaiting() || row.LaneID != leadID {
			continue
		}

		for next < len(lanes) && lanes[next].First.Before(row.Until) {
			heap.Push(live, lanes[next])
			next++
		}
		for live.Len() > 0 && !(*live)[0].Last.After(row.From) {
			heap.Pop(live)
		}
		if live.Len() == 0 {
			continue
		}
		row.Info += "; " + teammateList(*live, opts.MaxNamedTeammates)
	}
}

// teammateList renders who was alive, naming the first few and counting the rest.
func teammateList(live []LaneSpan, max int) string {
	names := make([]string, 0, len(live))
	for _, lane := range live {
		names = append(names, lane.Name)
	}
	sort.Strings(names)

	count := strconv.Itoa(len(names)) + " teammate"
	if len(names) != 1 {
		count += "s"
	}
	if len(names) <= max {
		return count + " alive: " + strings.Join(names, ", ")
	}
	return count + " alive: " + strings.Join(names[:max], ", ") + ", and " + strconv.Itoa(len(names)-max) + " more"
}

// laneHeap keeps the lanes that are still running, the earliest to finish on top.
type laneHeap []LaneSpan

func (h laneHeap) Len() int           { return len(h) }
func (h laneHeap) Less(i, j int) bool { return h[i].Last.Before(h[j].Last) }
func (h laneHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *laneHeap) Push(x any)        { *h = append(*h, x.(LaneSpan)) }
func (h *laneHeap) Pop() any {
	old := *h
	n := len(old)
	*h = old[:n-1]
	return old[n-1]
}
