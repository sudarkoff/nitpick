package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/sudarkoff/nitpick/findings"
)

// popPositional contract: the finding ID must be the first token; anything after
// is flags. A leading flag means no ID was given.
func TestPopPositional(t *testing.T) {
	id, rest := popPositional([]string{"RAR-01", "--evidence", "x"})
	if id != "RAR-01" || !reflect.DeepEqual(rest, []string{"--evidence", "x"}) {
		t.Errorf("id=%q rest=%v", id, rest)
	}
	id, rest = popPositional([]string{"RAR-02", "--repo", "r"})
	if id != "RAR-02" || !reflect.DeepEqual(rest, []string{"--repo", "r"}) {
		t.Errorf("id=%q rest=%v", id, rest)
	}
	id, _ = popPositional([]string{"--repo", "r"})
	if id != "" {
		t.Errorf("expected empty id (flag-first), got %q", id)
	}
}

// formatRecord must render every field in full — `show` exists precisely because
// `list` truncates. A long recommendation must survive verbatim, with no ellipsis.
func TestFormatRecord_Untruncated(t *testing.T) {
	long := strings.Repeat("propagate the source event to every destination calendar ", 5)
	rec := findings.Record{
		FindingID: "NP-07", Severity: "P1", Status: "open", Skill: "nitpick",
		Promise:        "events stay in sync",
		Component:      "engine/sync.go: the long-poll fallback path that nobody watches",
		FailureMode:    "the loop wedges and stops advancing the cursor",
		DetectionGap:   "no alert fires; the queue just stops draining",
		Recommendation: long,
	}
	out := formatRecord(rec)

	if strings.Contains(out, "…") {
		t.Errorf("output was truncated (found ellipsis):\n%s", out)
	}
	if !strings.Contains(out, long) {
		t.Errorf("full recommendation missing from output:\n%s", out)
	}
	for _, want := range []string{"NP-07", "P1", "open", rec.Promise, rec.Component, rec.FailureMode, rec.DetectionGap} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

// Lifecycle context must appear: evidence for a resolved finding, the waiver
// reason for a deferred one.
func TestFormatRecord_Lifecycle(t *testing.T) {
	resolved := findings.Record{
		FindingID: "NP-01", Severity: "P2", Status: "resolved",
		Recommendation: "add a timeout", Evidence: "sha:abc123",
	}
	if out := formatRecord(resolved); !strings.Contains(out, "sha:abc123") {
		t.Errorf("resolved finding missing evidence:\n%s", out)
	}

	deferred := findings.Record{
		FindingID: "NP-02", Severity: "P3", Status: "deferred",
		Recommendation: "tune the log volume", WaiverReason: "low impact, tracked in NP-99",
	}
	if out := formatRecord(deferred); !strings.Contains(out, "low impact, tracked in NP-99") {
		t.Errorf("deferred finding missing waiver reason:\n%s", out)
	}
}
