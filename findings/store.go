package findings

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const commitAuthor = "nitpick <nitpick@localhost>"

const schema = `CREATE TABLE IF NOT EXISTS findings (
  repo VARCHAR(255) NOT NULL,
  finding_id VARCHAR(32) NOT NULL,
  skill VARCHAR(64) NOT NULL,
  severity VARCHAR(4) NOT NULL,
  status VARCHAR(16) NOT NULL,
  promise TEXT, component TEXT, failure_mode TEXT,
  detection_gap TEXT, recommendation TEXT,
  evidence TEXT, waiver_reason TEXT,
  first_seen_at DATETIME, resolved_at DATETIME, deferred_at DATETIME,
  session_id VARCHAR(64),
  PRIMARY KEY (repo, finding_id)
);`

// Record is a persisted finding: a Finding plus its lifecycle metadata.
type Record struct {
	Repo           string
	FindingID      string
	Skill          string
	Severity       string
	Status         string // open|resolved|deferred
	Promise        string
	Component      string
	FailureMode    string
	DetectionGap   string
	Recommendation string
	Evidence       string
	WaiverReason   string
	SessionID      string
}

// Store is a Dolt-backed findings database. It drives the `dolt` CLI, so each
// mutation is captured as a Dolt commit (a per-row audit trail).
type Store struct {
	dir string
}

// Open ensures a Dolt repository with the findings schema exists at dir.
func Open(dir string) (*Store, error) {
	s := &Store{dir: dir}
	if _, err := os.Stat(filepath.Join(dir, ".dolt")); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
		if _, err := s.dolt("init"); err != nil {
			return nil, err
		}
	}
	if _, err := s.dolt("sql", "-q", schema); err != nil {
		return nil, err
	}
	if err := s.commit("nitpick: ensure schema"); err != nil {
		return nil, err
	}
	return s, nil
}

// Upsert inserts a finding, or updates only its descriptive fields if it already
// exists. A re-ingest never reopens or clears a finding's lifecycle state
// (status, evidence, waiver, first_seen) — that is what makes deferred and
// resolved work durable across reviews.
func (s *Store) Upsert(r Record) error {
	stmt := fmt.Sprintf(`INSERT INTO findings
  (repo, finding_id, skill, severity, status, promise, component, failure_mode,
   detection_gap, recommendation, session_id, first_seen_at)
  VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, NOW())
  ON DUPLICATE KEY UPDATE
   skill=VALUES(skill), severity=VALUES(severity), promise=VALUES(promise),
   component=VALUES(component), failure_mode=VALUES(failure_mode),
   detection_gap=VALUES(detection_gap), recommendation=VALUES(recommendation);`,
		q(r.Repo), q(r.FindingID), q(r.Skill), q(r.Severity), q(r.Status),
		q(r.Promise), q(r.Component), q(r.FailureMode), q(r.DetectionGap),
		q(r.Recommendation), q(r.SessionID))
	if _, err := s.dolt("sql", "-q", stmt); err != nil {
		return err
	}
	return s.commit(fmt.Sprintf("nitpick: upsert %s %s", r.Repo, r.FindingID))
}

// SetStatus transitions a finding and records evidence (on resolve) or a waiver
// reason (on defer/waive). Empty evidence/reason are left untouched.
func (s *Store) SetStatus(repo, findingID, status, evidence, waiverReason string) error {
	sets := []string{"status=" + q(status)}
	if evidence != "" {
		sets = append(sets, "evidence="+q(evidence))
	}
	if waiverReason != "" {
		sets = append(sets, "waiver_reason="+q(waiverReason))
	}
	switch status {
	case "resolved":
		sets = append(sets, "resolved_at=NOW()")
	case "deferred":
		sets = append(sets, "deferred_at=NOW()")
	}
	upd := fmt.Sprintf("UPDATE findings SET %s WHERE repo=%s AND finding_id=%s;",
		strings.Join(sets, ", "), q(repo), q(findingID))
	if _, err := s.dolt("sql", "-q", upd); err != nil {
		return err
	}
	return s.commit(fmt.Sprintf("nitpick: %s %s %s", status, repo, findingID))
}

// List returns findings for a repo, optionally filtered by status ("" = all).
func (s *Store) List(repo, status string) ([]Record, error) {
	where := "repo=" + q(repo)
	if status != "" {
		where += " AND status=" + q(status)
	}
	sel := fmt.Sprintf(`SELECT repo, finding_id, skill, severity, status,
   COALESCE(promise,'') AS promise, COALESCE(component,'') AS component,
   COALESCE(failure_mode,'') AS failure_mode, COALESCE(detection_gap,'') AS detection_gap,
   COALESCE(recommendation,'') AS recommendation, COALESCE(evidence,'') AS evidence,
   COALESCE(waiver_reason,'') AS waiver_reason, COALESCE(session_id,'') AS session_id
   FROM findings WHERE %s ORDER BY finding_id;`, where)
	out, err := s.dolt("sql", "-q", sel, "-r", "json")
	if err != nil {
		return nil, err
	}
	rows, err := parseRows(out)
	if err != nil {
		return nil, err
	}
	recs := make([]Record, 0, len(rows))
	for _, row := range rows {
		recs = append(recs, Record{
			Repo: str(row, "repo"), FindingID: str(row, "finding_id"),
			Skill: str(row, "skill"), Severity: str(row, "severity"),
			Status: str(row, "status"), Promise: str(row, "promise"),
			Component: str(row, "component"), FailureMode: str(row, "failure_mode"),
			DetectionGap: str(row, "detection_gap"), Recommendation: str(row, "recommendation"),
			Evidence: str(row, "evidence"), WaiverReason: str(row, "waiver_reason"),
			SessionID: str(row, "session_id"),
		})
	}
	return recs, nil
}

func (s *Store) dolt(args ...string) (string, error) {
	cmd := exec.Command("dolt", args...)
	cmd.Dir = s.dir
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("dolt %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}

// commit stages and commits the working set, skipping when there is nothing to
// commit (e.g. a no-op CREATE TABLE IF NOT EXISTS on reopen).
func (s *Store) commit(msg string) error {
	changed, err := s.hasChanges()
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	if _, err := s.dolt("add", "-A"); err != nil {
		return err
	}
	if _, err := s.dolt("commit", "--author", commitAuthor, "-m", msg); err != nil {
		return err
	}
	return nil
}

// hasChanges reports whether the working set has uncommitted changes.
func (s *Store) hasChanges() (bool, error) {
	out, err := s.dolt("sql", "-q", "SELECT COUNT(*) AS n FROM dolt_status", "-r", "json")
	if err != nil {
		return false, err
	}
	rows, err := parseRows(out)
	if err != nil {
		return false, err
	}
	if len(rows) == 0 {
		return false, nil
	}
	return str(rows[0], "n") != "0", nil
}

// parseRows unmarshals dolt's `-r json` output ({"rows":[...]}) into row maps.
func parseRows(out string) ([]map[string]any, error) {
	if strings.TrimSpace(out) == "" {
		return nil, nil
	}
	var parsed struct {
		Rows []map[string]any `json:"rows"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		return nil, fmt.Errorf("parse dolt json: %w: %s", err, out)
	}
	return parsed.Rows, nil
}

// q renders a Go string as a single-quoted, escaped SQL string literal.
func q(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func str(row map[string]any, key string) string {
	v, ok := row[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}
