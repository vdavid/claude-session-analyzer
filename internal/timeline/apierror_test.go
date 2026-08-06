package timeline

import (
	"strings"
	"testing"
	"time"
)

// TestAPIErrorGetsItsOwnKind covers the record the harness writes when a request doesn't come back. It arrives as an
// assistant record carrying a text block, so without the typed fields it reads as the agent writing that sentence.
func TestAPIErrorGetsItsOwnKind(t *testing.T) {
	lane := newLane("lead", true).
		add(0, promptRec("go")).
		add(5, assistantRec(textBlock("looking into it"))).
		add(65, apiErrorRec("rate_limit", 429,
			"API Error: Server is temporarily limiting requests (not your usage limit) · Rate limited")).
		done()

	rows := Derive(sessionOf(lane), Options{}).Rows

	requireKinds(t, rows, KindWriting, KindAPIError)
	checkTiling(t, rows, at(0), at(65))
	if got := rows[1].Duration(); got != time.Minute {
		t.Errorf("the outage lasted %s, want the minute the harness spent retrying", got)
	}
	if !strings.HasPrefix(rows[1].Info, "rate limit (429): API Error: Server is temporarily limiting") {
		t.Errorf("the row says %q, want the typed error, the status, and what the harness showed", rows[1].Info)
	}
}

// TestAPIErrorIsNotIdleTime is the reason the kind exists. An outage the harness retried through runs past
// MaxResponseSpan, and the backstop files anything that long as idle, which quietly moves a session's lost time into
// the number that's supposed to mean "nobody was working".
func TestAPIErrorIsNotIdleTime(t *testing.T) {
	lane := newLane("lead", true).
		add(0, promptRec("go")).
		add(5, assistantRec(textBlock("on it"))).
		add(2405, apiErrorRec("server_error", 529, "API Error: 529 Overloaded.")).
		add(2410, assistantRec(textBlock("that failed, trying again"))).
		done()

	rows := Derive(sessionOf(lane), Options{}).Rows

	requireKinds(t, rows, KindWriting, KindAPIError, KindWaitingUnknown, KindWriting)
	checkTiling(t, rows, at(0), at(2410))
	if got := rows[1].Duration(); got != 40*time.Minute {
		t.Errorf("the outage lasted %s, want the whole 40 minutes rather than an idle row", got)
	}
}

// TestLaneIsIdleAfterAFailedRequest covers what the lane is doing once the API has refused: nothing. The agent can't
// carry on by itself, so the stretch after the error belongs to whatever restarted it, and the row says which of the
// two idle rules put it there.
func TestLaneIsIdleAfterAFailedRequest(t *testing.T) {
	lane := newLane("lead", true).
		add(0, promptRec("go")).
		add(5, assistantRec(textBlock("on it"))).
		add(20, apiErrorRec("authentication_failed", 401, "Please run /login")).
		add(3620, promptRec("logged back in, carry on")).
		add(3625, assistantRec(textBlock("carrying on"))).
		done()

	rows := Derive(sessionOf(lane), Options{}).Rows

	requireKinds(t, rows, KindWriting, KindAPIError, KindWaitingForPerson, KindWriting)
	if got := rows[2].Duration(); got != time.Hour {
		t.Errorf("the wait lasted %s, want the hour until the person came back", got)
	}
}

// TestAPIErrorIsBoundedByItsRetryWindow covers the case there's no evidence for and every reason to expect: a session
// resumed long after it was left, whose first request fails on a login that expired while it sat there. The corpus's
// longest stretch before an API error is 1h19m, so weeks of it are a resumed session, not an outage.
func TestAPIErrorIsBoundedByItsRetryWindow(t *testing.T) {
	lane := newLane("lead", true).
		add(0, promptRec("go")).
		add(5, assistantRec(textBlock("goodbye"))).
		add(2200000, apiErrorRec("authentication_failed", 401, "Please run /login")).
		done()

	rows := Derive(sessionOf(lane), Options{}).Rows

	requireKinds(t, rows, KindWriting, KindWaitingUnknown, KindAPIError)
	checkTiling(t, rows, at(0), at(2200000))
	if got := rows[2].Duration(); got != DefaultMaxAPIErrorSpan {
		t.Errorf("the outage lasted %s, want it capped at %s", got, DefaultMaxAPIErrorSpan)
	}
	if got := rows[1].Duration(); got != 2199995*time.Second-DefaultMaxAPIErrorSpan {
		t.Errorf("the wait lasted %s, want everything the outage can't account for", got)
	}
}

// TestAPIErrorAfterATurnEndedClaimsNoTime covers a lane with nothing in flight. The turn was over, so the stretch
// before the error was the lane sitting idle, and the error marks the moment without claiming any of it.
func TestAPIErrorAfterATurnEndedClaimsNoTime(t *testing.T) {
	lane := newLane("lead", true).
		add(0, promptRec("go")).
		add(5, assistantRec(textBlock("done"))).
		add(6, systemRec("turn_duration")).
		add(3600, apiErrorRec("invalid_request", 413, "Prompt is too long")).
		done()

	rows := Derive(sessionOf(lane), Options{}).Rows

	requireKinds(t, rows, KindWriting, KindWaitingUnknown, KindAPIError)
	checkTiling(t, rows, at(0), at(3600))
	if rows[2].Duration() != 0 {
		t.Errorf("with the turn already over, the error can't claim the idle stretch: %s", rowSummary(rows[2]))
	}
	if !strings.Contains(rows[1].Info, "idle after the turn ended") {
		t.Errorf("the wait says %q, want it to say the turn had ended", rows[1].Info)
	}
}

// TestAPIErrorDegradesWithoutItsFields covers a later version writing something this doesn't know. A status of zero and
// a kind nothing recognises still make a row, because the flag alone says the request failed.
func TestAPIErrorDegradesWithoutItsFields(t *testing.T) {
	lane := newLane("lead", true).
		add(0, promptRec("go")).
		add(5, assistantRec(textBlock("on it"))).
		add(35, apiErrorRec("", 0, "")).
		done()

	rows := Derive(sessionOf(lane), Options{}).Rows

	requireKinds(t, rows, KindWriting, KindAPIError)
	if rows[1].Info != "the API didn't answer" {
		t.Errorf("the row says %q, want it to say what little is on record", rows[1].Info)
	}
}
