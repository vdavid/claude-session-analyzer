// Package api is the HTTP surface over the engine: a session list, one session's metadata, and one session's timeline
// with the pie and the swimlane already aggregated.
//
// Nothing is cached. A session is parsed on request, which is under a second for all but the largest, and a transcript
// that's still being written would make a cache another thing to invalidate. `docs/api.md` is the contract.
package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"slices"
	"strconv"

	"github.com/vdavid/claude-session-analyzer/internal/session"
	"github.com/vdavid/claude-session-analyzer/internal/timeline"
	"github.com/vdavid/claude-session-analyzer/internal/transcript"
)

// Options configures the handler.
type Options struct {
	// Root is where the transcripts are, usually ~/.claude/projects.
	Root string
	// FrontendOrigins are the browser origins allowed to read an answer. The API binds to 127.0.0.1, but that alone
	// doesn't stop a page on another origin from reading it, so the list is the frontend's own dev origins and nothing
	// else.
	FrontendOrigins []string
}

type server struct {
	opts Options
}

// New builds the handler. It serves `/api/...` only, so a caller can mount the built frontend alongside it.
func New(opts Options) http.Handler {
	s := &server{opts: opts}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/sessions", s.listSessions)
	mux.HandleFunc("GET /api/sessions/{id}", s.oneSession)
	mux.HandleFunc("GET /api/sessions/{id}/timeline", s.timeline)

	// The same paths without a method, so anything but GET answers in JSON like everything else rather than in the
	// mux's plain text. A pattern naming a method wins over one that doesn't, so the GET routes above still match.
	for _, pattern := range []string{"/api/sessions", "/api/sessions/{id}", "/api/sessions/{id}/timeline"} {
		mux.HandleFunc(pattern, methodNotAllowed)
	}

	mux.HandleFunc("/", notFound)
	return s.withCORS(mux)
}

// withCORS lets the frontend's own origin read the answers, and nobody else. Without it the dev server on another port
// couldn't call the API at all; with a wildcard, any page in the browser could read the transcripts.
func (s *server) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		allowed := false
		if origin := r.Header.Get("Origin"); origin != "" && slices.Contains(s.opts.FrontendOrigins, origin) {
			allowed = true
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Add("Vary", "Origin")
		}

		// A plain GET needs no preflight, but a fetch carrying a header outside the safelist does, and answering it
		// costs less than the afternoon spent working out why the browser gave up before it asked.
		if r.Method == http.MethodOptions && allowed {
			w.Header().Set("Access-Control-Allow-Methods", http.MethodGet)
			w.Header().Set("Access-Control-Allow-Headers", r.Header.Get("Access-Control-Request-Headers"))
			w.Header().Set("Access-Control-Max-Age", "600")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *server) listSessions(w http.ResponseWriter, r *http.Request) {
	limit, err := intParam(r, "limit")
	if err != nil {
		writeError(w, http.StatusBadRequest, errorDetail{Code: "bad_request", Message: err.Error()})
		return
	}

	sums, err := session.List(s.opts.Root)
	if err != nil {
		s.writeRootProblem(w, err)
		return
	}

	body := sessionListBody{Root: s.opts.Root, Sessions: []sessionBody{}}
	for _, sum := range sums {
		body.Totals.Sessions++
		body.Totals.Subagents += sum.Subagents
		body.Totals.Bytes += sum.Bytes
	}

	shown := sums
	if limit > 0 && limit < len(shown) {
		shown = shown[:limit]
	}
	for _, sum := range shown {
		body.Sessions = append(body.Sessions, toSession(sum))
	}
	writeJSON(w, http.StatusOK, body)
}

func (s *server) oneSession(w http.ResponseWriter, r *http.Request) {
	loc, ok := s.resolve(w, r)
	if !ok {
		return
	}
	sum, err := session.Summarize(loc)
	if err != nil {
		writeError(w, http.StatusInternalServerError, errorDetail{
			Code:    "internal",
			Message: fmt.Sprintf("Couldn't read session %s: %v", loc.ID, err),
		})
		return
	}
	writeJSON(w, http.StatusOK, oneSessionBody{Session: toSession(sum)})
}

func (s *server) timeline(w http.ResponseWriter, r *http.Request) {
	loc, ok := s.resolve(w, r)
	if !ok {
		return
	}

	sum, err := session.Summarize(loc)
	if err != nil {
		writeError(w, http.StatusInternalServerError, errorDetail{
			Code:    "internal",
			Message: fmt.Sprintf("Couldn't read session %s: %v", loc.ID, err),
		})
		return
	}

	parsed, err := session.Load(loc, transcript.Options{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, errorDetail{
			Code:    "internal",
			Message: fmt.Sprintf("Couldn't read session %s: %v", loc.ID, err),
		})
		return
	}

	tl := timeline.Derive(parsed, timeline.Options{})
	writeJSON(w, http.StatusOK, buildTimeline(sum, tl, wantsRows(r)))
}

// resolve turns the `{id}` in the path into a location, answering for itself when it can't. An id that matches nothing
// and an id that matches several are both the caller's problem, and both get told what to do about it.
func (s *server) resolve(w http.ResponseWriter, r *http.Request) (session.Location, bool) {
	id := r.PathValue("id")
	loc, err := session.Find(s.opts.Root, id)
	if err == nil {
		return loc, true
	}

	var ambiguous *session.AmbiguousIDError
	switch {
	case errors.Is(err, session.ErrNotFound):
		writeError(w, http.StatusNotFound, errorDetail{
			Code:    "not_found",
			Message: fmt.Sprintf("No session id starts with %q. `/api/sessions` lists what's on disk.", id),
		})
	case errors.As(err, &ambiguous):
		writeError(w, http.StatusBadRequest, errorDetail{
			Code:    "ambiguous_id",
			Message: fmt.Sprintf("%q matches %d sessions. Add a few more characters to pick one.", id, len(ambiguous.Matches)),
			Matches: ambiguous.Matches,
		})
	default:
		s.writeRootProblem(w, err)
	}
	return session.Location{}, false
}

// writeRootProblem answers for anything that went wrong reading the transcript root, which is nearly always that it
// isn't there.
func (s *server) writeRootProblem(w http.ResponseWriter, err error) {
	if errors.Is(err, fs.ErrNotExist) {
		writeError(w, http.StatusInternalServerError, errorDetail{
			Code: "no_transcripts",
			Message: fmt.Sprintf("There's no transcript directory at %s. Claude Code keeps its sessions under "+
				"`~/.claude/projects`, and `CLAUDE_CONFIG_DIR` moves that.", s.opts.Root),
		})
		return
	}
	writeError(w, http.StatusInternalServerError, errorDetail{
		Code:    "internal",
		Message: fmt.Sprintf("Couldn't read the transcripts in %s: %v", s.opts.Root, err),
	})
}

// wantsRows says whether to ship the rows. A session with 983 lanes is worth drawing without its 22,000-row sheet, so
// `?rows=false` returns the aggregates alone.
func wantsRows(r *http.Request) bool {
	switch r.URL.Query().Get("rows") {
	case "false", "0", "no", "none":
		return false
	default:
		return true
	}
}

// intParam reads a numeric query parameter, and says which one was wrong rather than quietly using zero.
func intParam(r *http.Request, name string) (int, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("`%s` takes a number, and it got %q.", name, raw)
	}
	if n < 0 {
		return 0, fmt.Errorf("`%s` can't be negative. Leave it out, or pass 0, to get everything.", name)
	}
	return n, nil
}

func notFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound, errorDetail{
		Code:    "not_found",
		Message: fmt.Sprintf("There's nothing at %s. The API is `/api/sessions`, `/api/sessions/{id}`, and `/api/sessions/{id}/timeline`.", r.URL.Path),
	})
}

func methodNotAllowed(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Allow", http.MethodGet)
	writeError(w, http.StatusMethodNotAllowed, errorDetail{
		Code:    "method_not_allowed",
		Message: fmt.Sprintf("%s only answers GET, and this was %s.", r.URL.Path, r.Method),
	})
}

func writeError(w http.ResponseWriter, status int, detail errorDetail) {
	writeJSON(w, status, errorBody{Error: detail})
}

// writeJSON renders a body, then sends it. Rendering first means an encoding problem becomes a 500 rather than half a
// document under a 200, and it gives the response a Content-Length, which a browser fetching six megabytes appreciates.
func writeJSON(w http.ResponseWriter, status int, body any) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"error":{"code":"internal","message":%q}}`, "Couldn't render the answer: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}
