package cli

import (
	"fmt"
	"time"

	"github.com/vdavid/claude-session-analyzer/internal/cache"
	"github.com/vdavid/claude-session-analyzer/internal/session"
	"github.com/vdavid/claude-session-analyzer/internal/timeline"
)

func runCache(a *app, args []string) error {
	fs := newFlagSet(a, "cache")
	root := fs.String("root", "", "read transcripts from this directory instead of ~/.claude/projects")
	var only scope
	only.register(fs)

	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if len(rest) == 0 {
		return usagef("`%s cache` takes `warm`, `info`, or `clear`.", binary)
	}
	if len(rest) > 1 {
		return usagef("One thing at a time, and this got %q and %q.", rest[0], rest[1])
	}

	dir, err := transcriptRoot(*root)
	if err != nil {
		return err
	}
	store, err := cache.Open(dir)
	if err != nil {
		return err
	}

	switch rest[0] {
	case "warm":
		return warmCache(a, store, dir, &only)
	case "info":
		return cacheInfo(a, store, dir)
	case "clear":
		return clearCache(a, store)
	default:
		return usagef("There's no `%s` to do to the cache. It takes `warm`, `info`, or `clear`.", rest[0])
	}
}

// warmCache parses whatever isn't cached yet. The first run over a whole corpus is the 30 seconds this cache exists to
// spend once, so it says what it's doing while it does it.
func warmCache(a *app, store *cache.Store, dir string, only *scope) error {
	keep, err := only.filter()
	if err != nil {
		return err
	}

	started := time.Now()
	walk, err := store.Corpus(dir, cache.CorpusOptions{
		Include:  keep,
		Zone:     time.Local,
		Detail:   true,
		Progress: progressTo(a, true),
	})
	if err != nil {
		if problem := missingRoot(dir, err); problem != nil {
			return problem
		}
		return fmt.Errorf("Couldn't read the transcripts in %s: %w", dir, err)
	}

	info, err := store.Info()
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out, "Warmed %s sessions in %s: %s were already cached, %s parsed. %s on disk in %s.\n",
		count(len(walk.Digests)), timeline.FormatDuration(time.Since(started)),
		count(walk.Hits), count(walk.Parsed), humanBytes(info.Bytes), shortenHome(store.Dir()))

	// A transcript that can't be read is worth naming: it's usually the format having moved, which is the one thing
	// this tool has to notice rather than route around.
	if walk.Failed > 0 {
		fmt.Fprintf(a.err, "\n%s sessions couldn't be read:\n", count(walk.Failed))
		for i, err := range walk.Errors {
			if i == 5 {
				fmt.Fprintf(a.err, "  and %s more.\n", count(len(walk.Errors)-i))
				break
			}
			fmt.Fprintf(a.err, "  %v\n", err)
		}
	}
	return nil
}

func cacheInfo(a *app, store *cache.Store, dir string) error {
	info, err := store.Info()
	if err != nil {
		return err
	}
	if info.Sessions == 0 {
		fmt.Fprintf(a.out, "Nothing cached yet. `%s cache warm` fills it, which takes about half a minute the first "+
			"time.\n", binary)
		return nil
	}

	sums, err := session.List(dir)
	if err != nil {
		return fmt.Errorf("Couldn't read the transcripts in %s: %w", dir, err)
	}

	fmt.Fprintf(a.out, "%s of %s sessions cached, %s in %s.\n",
		count(info.Sessions), count(len(sums)), humanBytes(info.Bytes), shortenHome(store.Dir()))
	fmt.Fprintf(a.out, "Built between %s and %s, under derivation version %d.\n",
		localTime(info.Oldest), localTime(info.Newest), cache.Version)
	if info.Sessions < len(sums) {
		fmt.Fprintf(a.out, "`%s cache warm` fills in the rest.\n", binary)
	}
	return nil
}

func clearCache(a *app, store *cache.Store) error {
	info, err := store.Info()
	if err != nil {
		return err
	}
	if err := store.Clear(); err != nil {
		return err
	}
	fmt.Fprintf(a.out, "Cleared %s cached sessions (%s) from %s. Nothing was read from the transcripts.\n",
		count(info.Sessions), humanBytes(info.Bytes), shortenHome(store.Dir()))
	return nil
}
