// Command docgraph holds the docs to being one connected graph.
//
// Three rules, run over every markdown file in the repo:
//
//  1. Reachable. Every doc can be walked to from `AGENTS.md` by following references. A doc nothing points at is a
//     doc nobody finds, which is how a repo ends up with two answers to the same question.
//  2. No dead references. Every repo path a doc names exists on disk.
//  3. No link repeating its own target. `[`docs/api.md`](docs/api.md)` is noise; a bare backticked path is the
//     convention, and the graph follows both.
//
// A reference is a markdown link, a bare backticked repo path, or a `@path` import. Deciding which backticked spans
// are paths is the whole trick, and `pathReference` is where that judgement lives.
package main

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
)

// roots are where the walk starts. `AGENTS.md` is the hub every doc hangs off; the root `CLAUDE.md` is the harness's
// own entry point, which imports the hub rather than being pointed at by it.
var roots = []string{"AGENTS.md", "CLAUDE.md"}

// skipDirs never hold docs the graph is responsible for: build output, dependencies, and fixtures that are shaped like
// the transcripts they stand in for.
var skipDirs = map[string]bool{
	".git":         true,
	".svelte-kit":  true,
	"build":        true,
	"dist":         true,
	"node_modules": true,
	"testdata":     true,
}

var (
	linkPattern = regexp.MustCompile(`\[([^\]]*)\]\(([^)\s]+)\)`)
	codePattern = regexp.MustCompile("`([^`\n]+)`")
	// importPattern is the `@path` form the Claude Code harness inlines, which the root `CLAUDE.md` is made of.
	importPattern = regexp.MustCompile(`(?m)^@([A-Za-z0-9._+/-]+)\s*$`)
	// pathShape is deliberately narrow. Docs are full of backticked things that aren't paths, and every character
	// class left out here (`<`, `*`, `~`, `$`, `?`, `:`, a space) is one that keeps a placeholder such as
	// `subagents/agent-a<something>.jsonl` or a package name such as `@tanstack/table-core` out of the graph.
	pathShape = regexp.MustCompile(`^[A-Za-z0-9._+-]+(/[A-Za-z0-9._+-]+)*/?$`)
)

type problem struct {
	file string
	line int
	msg  string
}

func main() {
	dir := "."
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	problems, err := check(os.DirFS(dir))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	for _, p := range problems {
		if p.line > 0 {
			fmt.Printf("%s:%d: %s\n", p.file, p.line, p.msg)
			continue
		}
		fmt.Printf("%s: %s\n", p.file, p.msg)
	}
	if len(problems) > 0 {
		fmt.Printf("\n%d problem(s). Every doc hangs off AGENTS.md, and every path it names has to exist.\n",
			len(problems))
		os.Exit(1)
	}
}

func check(fsys fs.FS) ([]problem, error) {
	docs, err := markdownFiles(fsys)
	if err != nil {
		return nil, err
	}

	var problems []problem
	edges := map[string][]string{}
	for _, doc := range docs {
		body, err := fs.ReadFile(fsys, doc)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", doc, err)
		}
		targets, found := references(fsys, doc, string(body))
		edges[doc] = targets
		problems = append(problems, found...)
	}

	for _, doc := range unreachable(docs, edges) {
		problems = append(problems, problem{file: doc, msg: "nothing links here: reachable from neither " +
			strings.Join(roots, " nor ")})
	}
	return problems, nil
}

// references pulls every repo path a doc names, reporting the ones that don't resolve and the links that repeat their
// own target. It returns the markdown files among them, which are the graph's edges.
func references(fsys fs.FS, doc, body string) ([]string, []problem) {
	var edges []string
	var problems []problem
	dir := path.Dir(doc)

	keep := func(line int, ref string) {
		target, ok := resolve(fsys, dir, ref)
		if !ok {
			problems = append(problems, problem{doc, line, fmt.Sprintf("%q doesn't exist", ref)})
			return
		}
		if strings.HasSuffix(target, ".md") {
			edges = append(edges, target)
		}
	}

	inFence := false
	for i, line := range strings.Split(body, "\n") {
		number := i + 1
		if isFence(line) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}

		rest := line
		for _, m := range linkPattern.FindAllStringSubmatchIndex(line, -1) {
			text, ref := line[m[2]:m[3]], line[m[4]:m[5]]
			// Blank out the whole link so its text isn't read again as a bare path below.
			rest = strings.Replace(rest, line[m[0]:m[1]], "", 1)

			if bare := strings.Trim(text, "`"); pathReference(fsys, dir, bare) {
				problems = append(problems, problem{doc, number, fmt.Sprintf(
					"link text %q is a path: reference a doc as a bare backticked path, and link only for "+
						"descriptive text or an anchor", bare)})
			}
			if ref, ok := local(ref); ok {
				keep(number, ref)
			}
		}

		for _, m := range codePattern.FindAllStringSubmatch(rest, -1) {
			if pathReference(fsys, dir, m[1]) {
				keep(number, m[1])
			}
		}
	}

	for _, m := range importPattern.FindAllStringSubmatch(body, -1) {
		keep(0, m[1])
	}
	return edges, problems
}

// pathReference says whether a backticked span is naming something in this repo. A span that resolves is one; a span
// that doesn't is only reported as dead when its first segment names something real, so that prose shaped like a path
// (`usage/output_tokens`, `waiting/thinking`) is left alone rather than blamed.
func pathReference(fsys fs.FS, dir, ref string) bool {
	if !pathShape.MatchString(ref) {
		return false
	}
	if !strings.Contains(ref, "/") && !strings.HasSuffix(ref, ".md") {
		return false
	}
	if _, ok := resolve(fsys, dir, ref); ok {
		return true
	}
	head, _, _ := strings.Cut(strings.TrimPrefix(ref, "./"), "/")
	_, rooted := resolve(fsys, dir, head)
	return rooted
}

// resolve reads a reference the way the docs write them: from the repo root first, which is the convention, then
// relative to the doc that names it, which is how the files under `web/` point at each other.
func resolve(fsys fs.FS, dir, ref string) (string, bool) {
	cleaned := strings.TrimSuffix(ref, "/")
	for _, candidate := range []string{path.Clean(cleaned), path.Join(dir, cleaned)} {
		if candidate == "." || strings.HasPrefix(candidate, "..") {
			continue
		}
		if _, err := fs.Stat(fsys, candidate); err == nil {
			return candidate, true
		}
	}
	return "", false
}

// local drops the references a link can carry that aren't files: an external URL and an anchor within a page.
func local(ref string) (string, bool) {
	if strings.HasPrefix(ref, "#") || strings.Contains(ref, "://") || strings.HasPrefix(ref, "mailto:") {
		return "", false
	}
	trimmed, _, _ := strings.Cut(ref, "#")
	return trimmed, trimmed != ""
}

func unreachable(docs []string, edges map[string][]string) []string {
	seen := map[string]bool{}
	var walk func(string)
	walk = func(doc string) {
		if seen[doc] {
			return
		}
		seen[doc] = true
		for _, next := range edges[doc] {
			walk(next)
		}
	}
	for _, root := range roots {
		walk(root)
	}

	var orphans []string
	for _, doc := range docs {
		if !seen[doc] {
			orphans = append(orphans, doc)
		}
	}
	return orphans
}

func markdownFiles(fsys fs.FS) ([]string, error) {
	var docs []string
	err := fs.WalkDir(fsys, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if name != "." && (skipDirs[entry.Name()] || strings.HasPrefix(entry.Name(), ".")) {
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(name, ".md") {
			docs = append(docs, name)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(docs)
	return docs, nil
}

func isFence(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~")
}
