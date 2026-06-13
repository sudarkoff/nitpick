package findings

// StatusForSeverity maps a finding's severity to its initial lifecycle status:
// must-fix findings (P0/P1) start open and gate; the rest (P2/P3) are deferred
// on ingest and carried forward with context.
func StatusForSeverity(sev string) string {
	switch sev {
	case "P0", "P1":
		return "open"
	default:
		return "deferred"
	}
}

// Ingest parses RAR-NN findings from text and upserts each into the store with
// its policy-derived initial status. Returns the number of findings ingested.
func Ingest(s *Store, repo, skill, sessionID, text string) (int, error) {
	parsed, err := ParseRAR(text)
	if err != nil {
		return 0, err
	}
	for _, f := range parsed {
		if err := s.Upsert(Record{
			Repo:           repo,
			FindingID:      f.ID,
			Skill:          skill,
			Severity:       f.Severity,
			Status:         StatusForSeverity(f.Severity),
			Promise:        f.Promise,
			Component:      f.Component,
			FailureMode:    f.FailureMode,
			DetectionGap:   f.DetectionGap,
			Recommendation: f.Recommendation,
			SessionID:      sessionID,
		}); err != nil {
			return 0, err
		}
	}
	return len(parsed), nil
}
