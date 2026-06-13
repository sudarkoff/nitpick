package machine

import (
	"testing"

	"github.com/justinstimatze/stull/check"
	"github.com/justinstimatze/stull/sim"
)

func TestMachine_IsSound(t *testing.T) {
	if err := check.Validate(Machine()); err != nil {
		t.Fatalf("machine does not pass the static checker: %v", err)
	}
}

func bash(cmd, blockers string) map[string]any {
	return map[string]any{
		"hook_event_name": "PreToolUse", "session_id": "sim",
		"tool_name": "Bash", "tool_input": map[string]any{"command": cmd},
		FieldOpenBlockers: blockers,
	}
}

func fired(s sim.Step) bool { return s.FuelAfter < s.FuelBefore }

func TestMachine_PushGate(t *testing.T) {
	steps, _ := sim.Run(Machine(), sim.Scenario{Events: []map[string]any{
		bash("git push origin main", "2"), // blocked: open P0/P1
		bash("git status", "2"),           // not a push: passes free
		bash("git push origin main", "0"), // push but no blockers: passes
	}})
	if len(steps) != 3 {
		t.Fatalf("want 3 steps, got %d", len(steps))
	}
	if !fired(steps[0]) {
		t.Errorf("push with open blockers should block (fire a transition): %+v", steps[0])
	}
	if fired(steps[1]) {
		t.Errorf("a non-push command must not fire: %+v", steps[1])
	}
	if fired(steps[2]) {
		t.Errorf("push with zero blockers must pass (not fire): %+v", steps[2])
	}
}

func TestMachine_SessionStartSurface(t *testing.T) {
	withSummary := map[string]any{
		"hook_event_name": "SessionStart", "session_id": "sim",
		FieldSummary: "3 open P0/P1, 5 deferred for github.com/x/y",
	}
	noSummary := map[string]any{"hook_event_name": "SessionStart", "session_id": "sim"}

	steps, _ := sim.Run(Machine(), sim.Scenario{Events: []map[string]any{withSummary, noSummary}})
	if !fired(steps[0]) {
		t.Errorf("SessionStart with a summary should inject (fire): %+v", steps[0])
	}
	if fired(steps[1]) {
		t.Errorf("SessionStart with no summary must not fire: %+v", steps[1])
	}
}
