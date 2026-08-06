// Package session finds Claude Code sessions on disk and loads them into lanes.
//
// A session is a lead transcript plus one transcript per subagent it spawned. [Find] locates those files from a
// session id (a unique prefix works too), and [Load] reads them into a [Session] whose [Lane] values hold each agent's
// records in order. Everything downstream, the timeline and both surfaces over it, works from that.
//
// See docs/transcript-format.md for the layout on disk and how each claim about it was verified, including why a
// subagent's file name can't be parsed and why the project slug can't be inverted.
package session
