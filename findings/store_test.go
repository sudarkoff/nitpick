package findings

import (
	"os/exec"
	"testing"
)

func requireDolt(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("dolt"); err != nil {
		t.Skip("dolt not on PATH")
	}
}

func TestStore_UpsertAndList(t *testing.T) {
	requireDolt(t)
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	repo := "github.com/x/y"
	recs := []Record{
		{Repo: repo, FindingID: "RAR-01", Skill: "rar", Severity: "P1", Status: "open",
			Promise: "syncs propagate", Component: "sync", FailureMode: "hang",
			DetectionGap: "undetected", Recommendation: "add timeout", SessionID: "s1"},
		{Repo: repo, FindingID: "RAR-02", Skill: "rar", Severity: "P3", Status: "deferred",
			Promise: "ops", Component: "logs", FailureMode: "noisy",
			DetectionGap: "n/a", Recommendation: "tune", SessionID: "s1"},
	}
	for _, r := range recs {
		if err := s.Upsert(r); err != nil {
			t.Fatalf("Upsert %s: %v", r.FindingID, err)
		}
	}

	all, err := s.List(repo, "")
	if err != nil {
		t.Fatalf("List all: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("List all = %d findings, want 2", len(all))
	}

	open, err := s.List(repo, "open")
	if err != nil {
		t.Fatalf("List open: %v", err)
	}
	if len(open) != 1 || open[0].FindingID != "RAR-01" {
		t.Fatalf("List open = %+v, want [RAR-01]", open)
	}
	if open[0].Recommendation != "add timeout" || open[0].Severity != "P1" {
		t.Errorf("round-trip fields wrong: %+v", open[0])
	}
}

func TestStore_SetStatusAndReingestPreservesResolution(t *testing.T) {
	requireDolt(t)
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	repo := "github.com/x/y"
	base := Record{Repo: repo, FindingID: "RAR-01", Skill: "rar", Severity: "P0", Status: "open",
		Promise: "p", Component: "c", FailureMode: "fm", DetectionGap: "dg",
		Recommendation: "r", SessionID: "s1"}
	if err := s.Upsert(base); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if err := s.SetStatus(repo, "RAR-01", "resolved", "sha:abc123", ""); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}

	// Re-ingest the same finding (as a fresh review would): descriptive fields may
	// update, but a resolved finding must NOT be reopened or lose its evidence.
	reingest := base
	reingest.FailureMode = "fm (clarified)"
	reingest.Status = "open"
	if err := s.Upsert(reingest); err != nil {
		t.Fatalf("re-Upsert: %v", err)
	}

	got, err := s.List(repo, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Status != "resolved" {
		t.Errorf("status = %q, want resolved (re-ingest must not reopen)", got[0].Status)
	}
	if got[0].Evidence != "sha:abc123" {
		t.Errorf("evidence = %q, want sha:abc123", got[0].Evidence)
	}
	if got[0].FailureMode != "fm (clarified)" {
		t.Errorf("failure_mode = %q, want updated descriptive field", got[0].FailureMode)
	}
}

func TestStore_ReopenExistingDBIsIdempotent(t *testing.T) {
	requireDolt(t)
	dir := t.TempDir()
	if _, err := Open(dir); err != nil {
		t.Fatalf("first Open: %v", err)
	}
	// Reopening runs CREATE TABLE IF NOT EXISTS again (a no-op): committing an
	// empty changeset must not error.
	if _, err := Open(dir); err != nil {
		t.Fatalf("second Open: %v", err)
	}
}
