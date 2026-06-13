package loop

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/justinstimatze/stull/runtime"
	"github.com/justinstimatze/stull/spec"

	"github.com/sudarkoff/nitpick/findings"
)

// RawProvider runs a fenced cell and returns its raw completion ("" if the model
// is unavailable). Production code uses AnthropicRaw; tests inject a scripted one.
type RawProvider func(spec.Cell) string

// AnthropicRaw is the production RawProvider: stull's Messages-API model, which
// returns "" when ANTHROPIC_API_KEY is unset or the call fails.
func AnthropicRaw(c spec.Cell) string { return runtime.AnthropicModel(c, &spec.Context{}) }

var recheckWord = regexp.MustCompile(`[a-z]+`)

// recheckCell is a fenced oracle over the language {resolved, unresolved}: the
// model is asked to judge whether the cited evidence resolves the finding, and
// any answer outside that language is rejected by the grammar (fail-safe).
func recheckCell(f findings.Record, evidence string) spec.Cell {
	instr := fmt.Sprintf("A reliability finding was reported and a fix is now claimed.\n\n"+
		"Finding %s [%s]\nComponent: %s\nFailure mode: %s\nRecommendation: %s\n\n"+
		"Cited evidence: %s\n\n"+
		"Judge ONLY whether the cited evidence genuinely resolves the finding. "+
		"Output exactly one word: resolved or unresolved.",
		f.FindingID, f.Severity, f.Component, f.FailureMode, f.Recommendation, evidence)
	return spec.NewCell("recheck", "claude-sonnet-4-6", instr,
		func(raw string) (string, bool) {
			w := recheckWord.FindString(strings.ToLower(raw))
			return w, w == "resolved" || w == "unresolved"
		},
		func(string) bool { return true })
}

// Recheck asks a fenced oracle whether the cited evidence genuinely resolves the
// finding — an additional gate after deterministic evidence verification. It
// degrades safely: with no model it is skipped (evidence already passed), and an
// out-of-language answer releases rather than blocks. Only a clean "unresolved"
// rejects.
func Recheck(f findings.Record, evidence string, raw RawProvider) Verdict {
	if raw == nil {
		return Verdict{true, "re-check skipped (no model configured)"}
	}
	cell := recheckCell(f, evidence)
	completion := raw(cell)
	if strings.TrimSpace(completion) == "" {
		return Verdict{true, "re-check skipped (model unavailable — set ANTHROPIC_API_KEY)"}
	}
	res := cell.Check(completion)
	if !res.WellFormed {
		return Verdict{true, "re-check inconclusive — releasing"}
	}
	if res.Term == "unresolved" {
		return Verdict{false, "re-check judged the finding NOT resolved by this evidence"}
	}
	return Verdict{true, "re-check confirms resolved"}
}
