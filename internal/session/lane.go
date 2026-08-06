package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/vdavid/claude-session-analyzer/internal/transcript"
)

// Lane is one agent's transcript: the lead, or one subagent. A session's time is spent inside lanes, and they run
// concurrently, so the timeline is built per lane and merged afterwards.
type Lane struct {
	// ID is the session id for the lead, and the agent id for a subagent. An agent id is the transcript file's base
	// name minus its `agent-` prefix, which is also what the records inside carry as `agentId`.
	ID string
	// Name is what to label the lane: the metadata's name, its agent type, or the agent id, whichever comes first.
	Name   string
	IsLead bool
	// Paths are the files the lane was read from, more than one when it was written under several project slugs.
	Paths []string
	Meta  AgentMeta
	// WorkflowID names the workflow that spawned this lane, empty when the session spawned it directly.
	WorkflowID string

	// Records are every decoded record in the transcript, in file order.
	Records []*transcript.Record
	// Stats accounts for every line the reader saw, including the ones it skipped.
	Stats transcript.Stats
}

// AgentMeta is a subagent's `.meta.json`. Every field is optional: older files carry only AgentType and Description,
// and the file is missing entirely for plenty of lanes.
type AgentMeta struct {
	// Present says a `.meta.json` was there to read.
	Present     bool
	AgentType   string `json:"agentType"`
	Description string `json:"description"`
	Name        string `json:"name"`
	Model       string `json:"model"`
	Color       string `json:"color"`
	SpawnDepth  int    `json:"spawnDepth"`
	TaskKind    string `json:"taskKind"`
	TeamName    string `json:"teamName"`
}

// agentIDFromPath turns `subagents/agent-am1-engine-aeeff1f0.jsonl` into `am1-engine-aeeff1f0`. The remainder can't be
// split into a name and a hash, because names contain dashes too, so don't try: the label comes from the metadata.
func agentIDFromPath(rel string) string {
	return strings.TrimPrefix(strings.TrimSuffix(filepath.Base(rel), ".jsonl"), "agent-")
}

// workflowIDFromPath returns the workflow a lane belongs to, read from `subagents/workflows/<workflow-id>/agent-*`,
// and empty for a lane the session spawned directly.
func workflowIDFromPath(rel string) string {
	dir, parent := filepath.Split(filepath.Dir(rel))
	if filepath.Base(filepath.Clean(dir)) != "workflows" {
		return ""
	}
	return parent
}

// readAgentMeta reads the `.meta.json` sitting next to a subagent transcript, taking the first of the lane's files that
// has one: a lane split across project slugs carries its metadata beside exactly one of its fragments, and which one
// isn't predictable. A missing, unreadable, or undecodable one isn't an error: plenty of lanes have none, and a lane
// without metadata still has all its records. It costs a label, nothing more.
func readAgentMeta(transcriptPaths []string) AgentMeta {
	for _, transcriptPath := range transcriptPaths {
		raw, err := os.ReadFile(strings.TrimSuffix(transcriptPath, ".jsonl") + ".meta.json")
		if err != nil {
			continue
		}
		var meta AgentMeta
		if err := json.Unmarshal(raw, &meta); err != nil {
			continue
		}
		meta.Present = true
		return meta
	}
	return AgentMeta{}
}

// laneName picks the best label available.
func laneName(meta AgentMeta, agentID string) string {
	switch {
	case meta.Name != "":
		return meta.Name
	case meta.AgentType != "":
		return meta.AgentType
	default:
		return agentID
	}
}
