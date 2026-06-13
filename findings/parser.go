package findings

import (
	"regexp"
	"strings"
)

// Finding is a single reliability-architect-review finding, as emitted by the
// skill in its RAR-NN output format.
type Finding struct {
	ID             string // e.g. RAR-01
	Severity       string // P0|P1|P2|P3
	Promise        string
	Component      string
	FailureMode    string
	DetectionGap   string
	Recommendation string
}

var headerRE = regexp.MustCompile(`^FINDING\s+(RAR-\d+)\s+\[(P[0-3])\]\s*$`)

// fieldLabels maps a lowercased field label to the address of the field it sets
// on a finding, so the parser can both set and append (multi-line) to it.
var fieldLabels = map[string]func(*Finding) *string{
	"promise at risk": func(f *Finding) *string { return &f.Promise },
	"component":       func(f *Finding) *string { return &f.Component },
	"failure mode":    func(f *Finding) *string { return &f.FailureMode },
	"detection gap":   func(f *Finding) *string { return &f.DetectionGap },
	"recommendation":  func(f *Finding) *string { return &f.Recommendation },
}

// ParseRAR parses zero or more RAR-NN findings out of free text. Lines outside a
// finding block are ignored, so it tolerates surrounding prose or markers. A
// wrapped (indented or unlabeled) line continues the previous field; a blank
// line ends the current field.
func ParseRAR(text string) ([]Finding, error) {
	var findings []Finding
	var cur *Finding
	var curField *string

	flush := func() {
		if cur != nil {
			findings = append(findings, *cur)
			cur = nil
		}
		curField = nil
	}

	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)

		if m := headerRE.FindStringSubmatch(trimmed); m != nil {
			flush()
			cur = &Finding{ID: m[1], Severity: m[2]}
			continue
		}
		if cur == nil {
			continue
		}
		if trimmed == "" {
			curField = nil
			continue
		}
		if label, value, ok := splitLabel(trimmed); ok {
			if field, known := fieldLabels[label]; known {
				curField = field(cur)
				*curField = value
				continue
			}
		}
		// Continuation of the current field, if any.
		if curField != nil {
			if *curField == "" {
				*curField = trimmed
			} else {
				*curField += " " + trimmed
			}
		}
	}
	flush()
	return findings, nil
}

// splitLabel splits "Label: value" into a lowercased label and trimmed value,
// using the first colon. ok is false when there is no colon.
func splitLabel(line string) (label, value string, ok bool) {
	i := strings.IndexByte(line, ':')
	if i < 0 {
		return "", "", false
	}
	label = strings.ToLower(strings.TrimSpace(line[:i]))
	value = strings.TrimSpace(line[i+1:])
	return label, value, true
}
