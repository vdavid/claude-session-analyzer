package session

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vdavid/claude-session-analyzer/internal/transcript"
)

func loadAlpha(t *testing.T) *Session {
	t.Helper()
	loc, err := Find(testRoot(), alphaID)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	s, err := Load(loc, transcript.Options{})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return s
}

func TestLoadPutsTheLeadFirstThenOneLanePerSubagent(t *testing.T) {
	s := loadAlpha(t)

	if len(s.Lanes) != 4 {
		t.Fatalf("lanes = %d, want the lead plus three subagents", len(s.Lanes))
	}
	lead := s.Lanes[0]
	if !lead.IsLead {
		t.Error("the first lane should be the lead")
	}
	if lead.ID != alphaID {
		t.Errorf("lead id = %q, want the session id", lead.ID)
	}
	if lead.Name != "lead" {
		t.Errorf("lead name = %q", lead.Name)
	}
	if s.Lead() != lead {
		t.Error("Lead() should return the same lane")
	}
	for _, lane := range s.Lanes[1:] {
		if lane.IsLead {
			t.Errorf("lane %q should not be marked lead", lane.ID)
		}
	}
}

func TestLoadNamesLanesFromMetadataWithFallbacks(t *testing.T) {
	s := loadAlpha(t)

	tests := []struct {
		name         string
		laneIndex    int
		wantID       string
		wantName     string
		wantColor    string
		wantMetaFile bool
	}{
		{
			name:         "a full meta file gives name and color",
			laneIndex:    1,
			wantID:       "abuilder-aaaa1111",
			wantName:     "builder",
			wantColor:    "blue",
			wantMetaFile: true,
		},
		{
			name:         "no meta file at all falls back to the agent id",
			laneIndex:    2,
			wantID:       "acccc3333",
			wantName:     "acccc3333",
			wantColor:    "",
			wantMetaFile: false,
		},
		{
			name:         "an older meta file with only agentType falls back to it",
			laneIndex:    3,
			wantID:       "alegacy-bbbb2222",
			wantName:     "general-purpose",
			wantColor:    "",
			wantMetaFile: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lane := s.Lanes[tt.laneIndex]
			if lane.ID != tt.wantID {
				t.Errorf("id = %q, want %q", lane.ID, tt.wantID)
			}
			if lane.Name != tt.wantName {
				t.Errorf("name = %q, want %q", lane.Name, tt.wantName)
			}
			if lane.Meta.Color != tt.wantColor {
				t.Errorf("color = %q, want %q", lane.Meta.Color, tt.wantColor)
			}
			if lane.Meta.Present != tt.wantMetaFile {
				t.Errorf("meta present = %v, want %v", lane.Meta.Present, tt.wantMetaFile)
			}
		})
	}
}

func TestLoadKeepsEachLanesRecordsInOrder(t *testing.T) {
	s := loadAlpha(t)

	lead := s.Lanes[0]
	if len(lead.Records) != 5 {
		t.Fatalf("lead records = %d, want every decoded record in file order", len(lead.Records))
	}
	if lead.Records[0].UUID != "u1" || lead.Records[3].UUID != "a1" {
		t.Errorf("lead records = %q, %q", lead.Records[0].UUID, lead.Records[3].UUID)
	}
	if lead.Stats.Skipped != 1 {
		t.Errorf("lead skipped = %d, want the one `mode` record", lead.Stats.Skipped)
	}

	builder := s.Lanes[1]
	if len(builder.Records) != 2 || builder.Records[0].UUID != "su1" {
		t.Fatalf("builder records = %+v", builder.Records)
	}
	if !builder.Records[0].IsSidechain {
		t.Error("subagent records should be marked as sidechain")
	}
}

func TestLoadPrefersACustomTitleOverAGeneratedOne(t *testing.T) {
	s := loadAlpha(t)

	if s.Title != "Widgets, counted" {
		t.Errorf("title = %q, want the custom title to win over the generated one", s.Title)
	}
}

func TestLoadFallsBackThroughTheTitleKinds(t *testing.T) {
	dir := t.TempDir()
	slug := filepath.Join(dir, "projects", "-tmp-gamma")
	if err := os.MkdirAll(slug, 0o750); err != nil {
		t.Fatalf("make slug: %v", err)
	}
	id := "44444444-4444-4444-4444-444444444444"
	body := `{"type":"agent-name","agentName":"Only an agent name","sessionId":"` + id + `"}` + "\n" +
		`{"type":"ai-title","aiTitle":"A generated title","sessionId":"` + id + `"}` + "\n"
	if err := os.WriteFile(filepath.Join(slug, id+".jsonl"), []byte(body), 0o600); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	loc, err := Find(filepath.Join(dir, "projects"), id)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	s, err := Load(loc, transcript.Options{})
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if s.Title != "A generated title" {
		t.Errorf("title = %q, want the generated title when there's no custom one", s.Title)
	}
}

func TestLoadReadsTheProjectPathOffTheFirstRecordNotTheSlug(t *testing.T) {
	s := loadAlpha(t)

	// The slug is lossy and the session later moved into a worktree, so only the first record's cwd is trustworthy.
	if s.ProjectPath != "/tmp/alpha" {
		t.Errorf("project path = %q, want /tmp/alpha", s.ProjectPath)
	}
}

func TestLoadHandlesASessionWithNoSubagents(t *testing.T) {
	loc, err := Find(testRoot(), soloID)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	s, err := Load(loc, transcript.Options{})
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if len(s.Lanes) != 1 || !s.Lanes[0].IsLead {
		t.Fatalf("lanes = %d, want just the lead", len(s.Lanes))
	}
	if s.Title != "" {
		t.Errorf("title = %q, want empty when the transcript carries none", s.Title)
	}
}

func TestLoadFailsLoudlyOnAnUnreadableTranscript(t *testing.T) {
	loc := Location{
		ID:             "gone",
		TranscriptPath: filepath.Join(t.TempDir(), "missing.jsonl"),
	}

	if _, err := Load(loc, transcript.Options{}); err == nil {
		t.Fatal("want an error when the transcript isn't readable")
	}
}
