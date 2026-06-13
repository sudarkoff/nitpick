package findings

import "testing"

func TestParseRAR_SingleFinding(t *testing.T) {
	input := `FINDING RAR-01 [P1]
Promise at risk: syncs propagate in seconds
Component: apps/api/internal/sync
Failure mode: external API hangs with no timeout, goroutine leaks
Detection gap: undetected
Recommendation: add context deadline to all outbound HTTP calls`

	got, err := ParseRAR(input)
	if err != nil {
		t.Fatalf("ParseRAR returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(got))
	}
	f := got[0]
	if f.ID != "RAR-01" {
		t.Errorf("ID = %q, want RAR-01", f.ID)
	}
	if f.Severity != "P1" {
		t.Errorf("Severity = %q, want P1", f.Severity)
	}
	if f.Promise != "syncs propagate in seconds" {
		t.Errorf("Promise = %q", f.Promise)
	}
	if f.Component != "apps/api/internal/sync" {
		t.Errorf("Component = %q", f.Component)
	}
	if f.FailureMode != "external API hangs with no timeout, goroutine leaks" {
		t.Errorf("FailureMode = %q", f.FailureMode)
	}
	if f.DetectionGap != "undetected" {
		t.Errorf("DetectionGap = %q", f.DetectionGap)
	}
	if f.Recommendation != "add context deadline to all outbound HTTP calls" {
		t.Errorf("Recommendation = %q", f.Recommendation)
	}
}

func TestParseRAR_MultilineFieldsAndMultipleFindings(t *testing.T) {
	input := `Here is the review output:

FINDING RAR-02 [P0]
Promise at risk: events never get lost or duplicated
Component: apps/worker
Failure mode: job stuck in processing forever
  because no watchdog kills stale jobs
Detection gap: undetected for >1h
Recommendation: add a watchdog that requeues
  jobs older than 2x the expected interval

FINDING RAR-03 [P2]
Promise at risk: we alert you if sync breaks
Component: apps/api/internal/webhooks
Failure mode: subscription expiry only caught on safety-net poll
Detection gap: up to 4h
Recommendation: track last inbound webhook per subscription

That concludes the review.`

	got, err := ParseRAR(input)
	if err != nil {
		t.Fatalf("ParseRAR error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(got))
	}
	if got[0].ID != "RAR-02" || got[0].Severity != "P0" {
		t.Errorf("finding 0 header = %q %q", got[0].ID, got[0].Severity)
	}
	if got[0].FailureMode != "job stuck in processing forever because no watchdog kills stale jobs" {
		t.Errorf("multiline FailureMode = %q", got[0].FailureMode)
	}
	if got[0].Recommendation != "add a watchdog that requeues jobs older than 2x the expected interval" {
		t.Errorf("multiline Recommendation = %q", got[0].Recommendation)
	}
	if got[1].ID != "RAR-03" || got[1].Severity != "P2" {
		t.Errorf("finding 1 header = %q %q", got[1].ID, got[1].Severity)
	}
	if got[1].Component != "apps/api/internal/webhooks" {
		t.Errorf("finding 1 Component = %q", got[1].Component)
	}
}
