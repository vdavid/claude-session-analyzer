package timeline

import "time"

// DefaultCheapStall and DefaultHeavyStall are how long a tool may take before the row stops being a tool execution and
// becomes a stall. Getting this wrong in either direction hurts: too low and the tool calls an honest 40-minute build
// a stall, too high and it reports a six-hour suspension as a six-hour `rm`.
//
// So the threshold depends on what the call was doing, and both numbers sit far above anything the class does in
// practice:
//
//   - Cheap classes are the ones with no plausible long form: removing a file, reading one, listing a directory,
//     grepping a tree, a git command. An hour of that is not the tool working, even over a slow network mount. The
//     stall that prompted this tool was a 6h15m `rm` plus `du -sh`, which is 6 times over.
//   - Heavy classes earn their time: a build, a test suite, a checker script, a dev server the agent left running, a
//     wait loop that exists to block. Twelve hours is past anything honest and still catches an overnight suspension.
//
// A call that timed out is never a stall whatever it cost: the harness ended it on purpose, and the duration is the
// timeout the agent asked for.
const (
	DefaultCheapStall = time.Hour
	DefaultHeavyStall = 12 * time.Hour
)

// stallThreshold is how long this class may run before the row is called a stall.
func stallThreshold(class ToolClass, opts Options) time.Duration {
	switch class {
	case ClassBuild, ClassTest, ClassChecker, ClassDevServer, ClassWait, ClassAgent:
		return opts.HeavyStall
	default:
		return opts.CheapStall
	}
}
