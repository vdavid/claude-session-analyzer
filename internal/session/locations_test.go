package session

import (
	"path/filepath"
	"reflect"
	"testing"
)

// TestLocationsFindsEverySessionOneScanCanSee is the property that matters: a listing of the whole root has to agree
// with Find, session for session, or a corpus walk and a single lookup disagree about what a session is made of.
func TestLocationsFindsEverySessionOneScanCanSee(t *testing.T) {
	for _, root := range []string{testRoot(), filepath.Join("testdata", "worktree", "projects")} {
		locs, err := Locations(root)
		if err != nil {
			t.Fatalf("locations under %s: %v", root, err)
		}
		if len(locs) == 0 {
			t.Fatalf("no sessions under %s", root)
		}

		for _, loc := range locs {
			found, err := Find(root, loc.ID)
			if err != nil {
				t.Fatalf("find %s: %v", loc.ID, err)
			}
			if !reflect.DeepEqual(loc, found) {
				t.Errorf("locations gave\n%+v\nfind gave\n%+v", loc, found)
			}
		}
	}
}

func TestLocationsListsEverySessionInTheRoot(t *testing.T) {
	locs, err := Locations(testRoot())
	if err != nil {
		t.Fatalf("locations: %v", err)
	}

	want := []string{
		"-tmp-alpha/" + alphaID,
		"-tmp-alpha/" + soloID,
		"-tmp-beta/" + betaOneD,
		"-tmp-beta/33333333-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
	}
	got := make([]string, 0, len(locs))
	for _, loc := range locs {
		got = append(got, loc.ProjectSlug+"/"+loc.ID)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("sessions = %v, want %v (sorted by slug, then id)", got, want)
	}
}

// TestLocationsGroupsALaneWrittenUnderTwoSlugs holds the worktree case, where one lane arrives as fragments under two
// project slugs and has to come back as one lane.
func TestLocationsGroupsALaneWrittenUnderTwoSlugs(t *testing.T) {
	locs, err := Locations(filepath.Join("testdata", "worktree", "projects"))
	if err != nil {
		t.Fatalf("locations: %v", err)
	}
	if len(locs) != 1 {
		t.Fatalf("sessions = %d, want the one the worktree fixture holds", len(locs))
	}

	loc := locs[0]
	if len(loc.DirPaths) != 2 {
		t.Errorf("dir paths = %v, want one per slug", loc.DirPaths)
	}
	var split bool
	for _, lane := range loc.SubagentLanes {
		if len(lane.Paths) > 1 {
			split = true
		}
	}
	if !split {
		t.Errorf("lanes = %+v, want the home lane's two fragments grouped into one lane", loc.SubagentLanes)
	}
}
