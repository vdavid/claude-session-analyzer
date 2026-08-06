// Package timeline turns a parsed session into activity rows: which agent was doing what, from when until when.
//
// A row covers a stretch of one lane's wall clock. Rows in a lane tile the lane's span exactly, so the durations add
// up to the time the lane was alive with nothing unaccounted for and nothing counted twice. The one exception is a
// batch of parallel tool calls, which genuinely overlap and say so.
//
// The rules live in three places: kind.go names the activity kinds, tool.go classifies what a call was doing, and
// derive.go walks a lane's records. Every judgement call the derivation makes carries its reasoning next to the
// constant, because a wrong rule here is invisible in the output.
package timeline
