package main

import (
	"strings"
	"testing"
	"testing/fstest"
)

// file is a shorthand for a fixture entry, since every one of these is a small markdown body.
func file(body string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(body)} }

func TestAGraphWhereEveryDocIsReachedAndEveryPathExists(t *testing.T) {
	fsys := fstest.MapFS{
		"AGENTS.md":      file("The hub. Depth: `docs/rules.md`. Editing: `internal/thing/CLAUDE.md`.\n"),
		"CLAUDE.md":      file("@AGENTS.md\n"),
		"docs/rules.md":  file("Read `docs/deeper.md`, and see [the reasoning](deeper.md#why).\n"),
		"docs/deeper.md": file("A `usage/output_tokens` field, `agent-*.jsonl` files, and `@tanstack/table-core`.\n"),
		"internal/thing/CLAUDE.md": file(
			"Sits next to `main.go`.\n\n```\nA fenced `docs/gone.md` is not a reference.\n```\n"),
		"internal/thing/main.go": file("package thing\n"),
	}

	problems, err := check(fsys)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(problems) != 0 {
		t.Fatalf("want a clean graph, got %s", render(problems))
	}
}

func TestTheThreeWaysADocGraphRots(t *testing.T) {
	cases := []struct {
		name  string
		fsys  fstest.MapFS
		want  string
		where string
	}{
		{
			name: "a doc nothing points at",
			fsys: fstest.MapFS{
				"AGENTS.md":      file("The hub.\n"),
				"CLAUDE.md":      file("@AGENTS.md\n"),
				"docs/orphan.md": file("Nobody sent you here.\n"),
			},
			want:  "nothing links here",
			where: "docs/orphan.md",
		},
		{
			name: "a path that isn't there any more",
			fsys: fstest.MapFS{
				"AGENTS.md":     file("The hub. Depth: `docs/rules.md` and `docs/moved.md`.\n"),
				"CLAUDE.md":     file("@AGENTS.md\n"),
				"docs/rules.md": file("The depth.\n"),
			},
			want:  `"docs/moved.md" doesn't exist`,
			where: "AGENTS.md",
		},
		{
			name: "a link whose text repeats its target",
			fsys: fstest.MapFS{
				"AGENTS.md":     file("The hub: [`docs/rules.md`](docs/rules.md).\n"),
				"CLAUDE.md":     file("@AGENTS.md\n"),
				"docs/rules.md": file("The depth.\n"),
			},
			want:  "is a path",
			where: "AGENTS.md",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			problems, err := check(c.fsys)
			if err != nil {
				t.Fatalf("check: %v", err)
			}
			if len(problems) != 1 {
				t.Fatalf("want one problem, got %s", render(problems))
			}
			if problems[0].file != c.where || !strings.Contains(problems[0].msg, c.want) {
				t.Errorf("got %s, want %s saying %q", render(problems), c.where, c.want)
			}
		})
	}
}

// TestADeadLinkIsCaughtWhereProseThatLooksLikeOneIsNot holds the line the whole check rests on: docs name plenty of
// slash-separated things that aren't files, and blaming those would make the check unusable.
func TestADeadLinkIsCaughtWhereProseThatLooksLikeOneIsNot(t *testing.T) {
	fsys := fstest.MapFS{
		"AGENTS.md":     file("The hub: `docs/rules.md`.\n"),
		"CLAUDE.md":     file("@AGENTS.md\n"),
		"docs/rules.md": file("`docs/gone.md` went away, but `working/waiting` and `2.1.220` never were paths.\n"),
	}

	problems, err := check(fsys)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(problems) != 1 || !strings.Contains(problems[0].msg, "docs/gone.md") {
		t.Fatalf("want the missing doc alone, got %s", render(problems))
	}
}

func render(problems []problem) string {
	if len(problems) == 0 {
		return "no problems"
	}
	var out []string
	for _, p := range problems {
		out = append(out, p.file+": "+p.msg)
	}
	return strings.Join(out, "; ")
}
