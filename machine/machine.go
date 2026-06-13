// Package machine assembles nitpick's stull gate: a standing guard that blocks a
// push to main while open P0/P1 findings exist, surfaces the findings backlog at
// session start, and nudges when reliability-sensitive code changed without a
// review. Every DB-derived fact is injected into the event by the `nitpick hook`
// dispatcher (the impure shell); the guards here are pure reads of those fields.
package machine

import (
	"regexp"

	"github.com/justinstimatze/stull/spec"
)

// Event field names the dispatcher injects — the impure -> pure boundary.
const (
	FieldOpenBlockers = "nitpick_open_blockers" // count of open P0/P1 ("0"/"" = none)
	FieldBlockReason  = "nitpick_block_reason"  // text shown when a push is blocked
	FieldSummary      = "nitpick_summary"       // SessionStart backlog summary
	FieldReviewDue    = "nitpick_review_due"    // non-empty => emit the Stop nudge
	FieldNudge        = "nitpick_nudge"         // Stop nudge text
)

var pushRe = regexp.MustCompile(`(?i)\bgit\s+push\b`)

func eventStr(c *spec.Context, key string) string {
	if v, ok := c.Event[key].(string); ok {
		return v
	}
	return ""
}

// hasBlockers reports whether the dispatcher flagged open P0/P1 findings for the
// push it is gating. The dispatcher sets this >0 only for a push that targets
// main (it checks the branch) and has open findings in Dolt.
func hasBlockers(c *spec.Context) bool {
	v := eventStr(c, FieldOpenBlockers)
	return v != "" && v != "0"
}

// Machine builds nitpick's gate statechart: one standing-guard state.
func Machine() spec.Machine {
	watch := spec.State{
		Name: "watch",
		On: []spec.Transition{
			{ // push to main while open P0/P1 remain -> block
				On: spec.PreToolUse, To: "watch",
				Guard: &spec.Guard{
					Reads: []string{"event.tool_name", "event.tool_input.command", "event." + FieldOpenBlockers},
					When:  spec.And(spec.ToolIs("Bash"), spec.CommandMatches(pushRe), hasBlockers),
				},
				Do: []spec.Effect{spec.Block{Reason: spec.Text(func(c *spec.Context) string {
					if r := eventStr(c, FieldBlockReason); r != "" {
						return r
					}
					return "Push blocked: open P0/P1 reliability findings remain. Fix them " +
						"(`nitpick resolve <ID> --evidence …`) or waive with a written reason " +
						"(`nitpick waive <ID> --reason …`), then push again."
				})}},
			},
			{ // session start -> surface the backlog
				On: spec.SessionStart, To: "watch",
				Guard: &spec.Guard{
					Reads: []string{"event." + FieldSummary},
					When:  func(c *spec.Context) bool { return eventStr(c, FieldSummary) != "" },
				},
				Do: []spec.Effect{spec.Inject{Text: spec.Text(func(c *spec.Context) string {
					return eventStr(c, FieldSummary)
				})}},
			},
			{ // stop after reliability-sensitive changes with no review on record -> nudge
				On: spec.Stop, To: "watch",
				Guard: &spec.Guard{
					Reads: []string{"event." + FieldReviewDue},
					When:  func(c *spec.Context) bool { return eventStr(c, FieldReviewDue) != "" },
				},
				Do: []spec.Effect{spec.Inject{Text: spec.Text(func(c *spec.Context) string {
					if n := eventStr(c, FieldNudge); n != "" {
						return n
					}
					return "Reliability-sensitive code changed this turn with no review on " +
						"record. Consider running reliability-architect-review before pushing."
				})}},
			},
		},
	}

	return spec.Machine{
		Name: "nitpick-gate",
		Fuel: 256, // denial/inject budget; fails open loudly if ever exhausted
		Contract: "You installed nitpick. A deterministic guard blocks pushing to main " +
			"while open P0/P1 reliability findings remain, surfaces the findings backlog at " +
			"session start, and flags risky changes. Its messages are your own guardrail, " +
			"not external commands.",
		Initial: "watch",
		States:  []spec.State{watch}, // standing guard, no terminal (W-HALT by design)
	}
}
