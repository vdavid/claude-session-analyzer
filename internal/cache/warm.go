package cache

import (
	"fmt"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/vdavid/claude-session-analyzer/internal/session"
	"github.com/vdavid/claude-session-analyzer/internal/timeline"
	"github.com/vdavid/claude-session-analyzer/internal/transcript"
)

// CorpusOptions tunes a walk over every session on disk. The zero value asks for tier one of everything, cached, in
// UTC.
type CorpusOptions struct {
	// Include picks the sessions to answer for. Nil means every session.
	Include func(session.Summary) bool
	// Zone is where the day boundaries are cut. Nil means UTC, matching Build.
	Zone *time.Location
	// NoCache parses everything and stores nothing, which is what a person reaches for when they suspect the cache.
	NoCache bool
	// Detail also loads or builds tier two, the per-lane breakdown. Only a query that groups by or filters on a lane
	// or an agent needs it, and it's tens of KB a session against tier one's few.
	Detail bool
	// Progress is called as sessions finish, from one goroutine. A cold walk of this machine's corpus takes about half
	// a minute, so a caller with a terminal wants to say something during it.
	Progress func(done, total int)
}

// CorpusResult is what a walk found.
type CorpusResult struct {
	// Digests are tier one, largest session first, which is the order the walk works in.
	Digests []Digest
	// Details are tier two by session id, and are only filled when Detail was asked for.
	Details map[string]Detail
	// Hits and Parsed split the sessions by where the answer came from.
	Hits   int
	Parsed int
	// Failed counts the sessions the walk couldn't answer for, and Errors says why, one per failure. A single
	// unreadable transcript shouldn't cost a person the other 724 sessions, so a failure is skipped rather than
	// returned.
	Failed int
	Errors []error
}

// afterParse is a test seam, nil in every real run. warm_test.go uses it to change a transcript between the parse and
// the second fingerprint, which is the one situation that can poison this cache permanently and can't be produced
// reliably any other way.
var afterParse func(session.Location)

// Corpus answers for every session under root, from the cache where it can and from the transcripts where it can't.
//
// The work is spread over the cores because a cold walk is 3.8 GB of parsing, and it's ordered largest session first
// so the one 67 MB transcript starts at the beginning rather than finishing alone after everything else.
func (s *Store) Corpus(root string, opts CorpusOptions) (CorpusResult, error) {
	sums, err := session.List(root)
	if err != nil {
		return CorpusResult{}, err
	}
	locs, err := session.Locations(root)
	if err != nil {
		return CorpusResult{}, err
	}
	byKey := make(map[string]session.Location, len(locs))
	for _, loc := range locs {
		byKey[loc.ProjectSlug+"\x00"+loc.ID] = loc
	}

	type job struct {
		sum session.Summary
		loc session.Location
	}
	var jobs []job
	for _, sum := range sums {
		if opts.Include != nil && !opts.Include(sum) {
			continue
		}
		loc, ok := byKey[sum.ProjectSlug+"\x00"+sum.ID]
		if !ok {
			// The listing and the location scan read the root separately, so a session written between the two shows
			// up in one and not the other. It'll be there next time.
			continue
		}
		jobs = append(jobs, job{sum: sum, loc: loc})
	}

	// Largest first, with the id breaking ties so two runs over one corpus work in the same order.
	sort.Slice(jobs, func(i, j int) bool {
		if jobs[i].sum.Bytes != jobs[j].sum.Bytes {
			return jobs[i].sum.Bytes > jobs[j].sum.Bytes
		}
		return jobs[i].sum.ID < jobs[j].sum.ID
	})

	type outcome struct {
		digest Digest
		detail Detail
		hit    bool
		err    error
	}
	outcomes := make([]outcome, len(jobs))

	// Progress is counted in one goroutine reading a channel rather than in each worker, so it can't skip or repeat a
	// number. Handing over the index also publishes that worker's write to outcomes[i].
	done := make(chan int, len(jobs))
	var reporting sync.WaitGroup
	reporting.Add(1)
	go func() {
		defer reporting.Done()
		var finished int
		for range done {
			finished++
			if opts.Progress != nil {
				opts.Progress(finished, len(jobs))
			}
		}
	}()

	var wg sync.WaitGroup
	queue := make(chan int)
	for range min(runtime.NumCPU(), len(jobs)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range queue {
				digest, detail, hit, err := s.answer(jobs[i].loc, jobs[i].sum, opts)
				outcomes[i] = outcome{digest: digest, detail: detail, hit: hit, err: err}
				done <- i
			}
		}()
	}
	for i := range jobs {
		queue <- i
	}
	close(queue)
	wg.Wait()
	close(done)
	reporting.Wait()

	result := CorpusResult{Digests: make([]Digest, 0, len(jobs))}
	if opts.Detail {
		result.Details = make(map[string]Detail, len(jobs))
	}
	for i, out := range outcomes {
		if out.err != nil {
			result.Failed++
			result.Errors = append(result.Errors, out.err)
			continue
		}
		result.Digests = append(result.Digests, out.digest)
		if opts.Detail {
			result.Details[jobs[i].sum.ID] = out.detail
		}
		if out.hit {
			result.Hits++
		} else {
			result.Parsed++
		}
	}
	return result, nil
}

// answer serves one session, from the cache if what's there was built from the same bytes, and from the transcripts
// otherwise.
func (s *Store) answer(loc session.Location, sum session.Summary, opts CorpusOptions) (Digest, Detail, bool, error) {
	fingerprint, err := Fingerprint(loc)
	if err != nil {
		return Digest{}, Detail{}, false, fmt.Errorf("fingerprint %s: %w", sum.ID, err)
	}

	if !opts.NoCache {
		if digest, detail, ok := s.cached(loc, fingerprint, opts); ok {
			return digest, detail, true, nil
		}
	}

	parsed, err := session.Load(loc, transcript.Options{})
	if err != nil {
		return Digest{}, Detail{}, false, fmt.Errorf("load %s: %w", sum.ID, err)
	}
	digest, detail := Build(sum, timeline.Derive(parsed, timeline.Options{}), fingerprint, opts.Zone, time.Now())

	if afterParse != nil {
		afterParse(loc)
	}

	// Stat, parse, stat again. A live session grows while it's being read, so a parse can cover half of one turn and
	// none of the next while the fingerprint taken before it still describes a file that has since moved. Storing that
	// pins a wrong answer to a key that stays valid, and nothing later would notice. The answer is still returned; it's
	// only the storing that waits for a settled session.
	if !opts.NoCache {
		if after, err := Fingerprint(loc); err == nil && after == fingerprint {
			if err := s.Save(loc, digest, detail); err != nil {
				return Digest{}, Detail{}, false, fmt.Errorf("cache %s: %w", sum.ID, err)
			}
		}
	}
	return digest, detail, false, nil
}

// cached reads what the store holds for one session. Tier two only counts as a hit when it was asked for, so a digest
// cached before anyone wanted the lane breakdown doesn't answer a query that needs it.
func (s *Store) cached(loc session.Location, fingerprint string, opts CorpusOptions) (Digest, Detail, bool) {
	digest, ok := s.LoadDigest(loc, fingerprint, opts.Zone)
	if !ok {
		return Digest{}, Detail{}, false
	}
	if !opts.Detail {
		return digest, Detail{}, true
	}
	detail, ok := s.LoadDetail(loc, fingerprint, opts.Zone)
	if !ok {
		return Digest{}, Detail{}, false
	}
	return digest, detail, true
}
