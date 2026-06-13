package findings

import "testing"

func TestStatusForSeverity(t *testing.T) {
	cases := map[string]string{"P0": "open", "P1": "open", "P2": "deferred", "P3": "deferred"}
	for sev, want := range cases {
		if got := StatusForSeverity(sev); got != want {
			t.Errorf("StatusForSeverity(%q) = %q, want %q", sev, got, want)
		}
	}
}

func TestIngest_AppliesSeverityPolicy(t *testing.T) {
	requireDolt(t)
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	text := `FINDING RAR-01 [P0]
Promise at risk: events never lost
Component: worker
Failure mode: stuck job
Detection gap: undetected
Recommendation: watchdog

FINDING RAR-02 [P3]
Promise at risk: tidy logs
Component: logs
Failure mode: noisy
Detection gap: n/a
Recommendation: tune`

	n, err := Ingest(s, "github.com/x/y", "rar", "sess-1", text)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if n != 2 {
		t.Fatalf("Ingest count = %d, want 2", n)
	}
	open, _ := s.List("github.com/x/y", "open")
	if len(open) != 1 || open[0].FindingID != "RAR-01" {
		t.Errorf("open = %+v, want [RAR-01]", open)
	}
	deferred, _ := s.List("github.com/x/y", "deferred")
	if len(deferred) != 1 || deferred[0].FindingID != "RAR-02" {
		t.Errorf("deferred = %+v, want [RAR-02]", deferred)
	}
}
