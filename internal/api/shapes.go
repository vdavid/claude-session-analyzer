package api

// The JSON shapes live in `internal/report`, shared with the CLI's `--json` so both surfaces answer with one
// vocabulary. What's left here is the two envelopes and the error body, which are HTTP's own.

import "github.com/vdavid/claude-session-analyzer/internal/report"

type oneSessionBody struct {
	Session report.Session `json:"session"`
}

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	// Code is what a caller branches on. Message is what a person reads.
	Code    string `json:"code"`
	Message string `json:"message"`
	// Matches names the candidates when an id matched more than one session.
	Matches []string `json:"matches,omitempty"`
}
