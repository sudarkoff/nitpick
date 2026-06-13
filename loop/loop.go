// Package loop is nitpick's verification layer: before a P0/P1 finding can be
// marked resolved, the evidence cited for the fix must be REAL. Only
// machine-checkable evidence (a commit that exists, a test that passes) counts;
// anything that cannot be auto-verified must go through `nitpick waive` with a
// written reason instead. This is the deterministic guard the design puts before
// any LLM re-check or slimemold gate.
package loop

import (
	"os/exec"
	"strings"
)

// Verdict is the outcome of verifying a piece of evidence.
type Verdict struct {
	OK     bool
	Detail string
}

// VerifyEvidence checks that the cited evidence is real, relative to repo dir.
// Supported, auto-verified forms:
//
//	sha:<commit>    the commit exists in this repo
//	test:<pattern>  `go test -run <pattern>` matches at least one test and passes
//
// Anything else is rejected: non-verifiable claims (alert:/note:/manual:/…) must
// be waived with a reason, not passed off as a verified fix.
func VerifyEvidence(dir, evidence string) Verdict {
	kind, val, ok := splitKind(evidence)
	if !ok {
		return Verdict{false, "evidence must be <kind>:<value> (e.g. sha:… or test:…)"}
	}
	switch kind {
	case "sha":
		return verifySha(dir, val)
	case "test":
		return verifyTest(dir, val)
	case "defn":
		// defn structural confirmation is a planned auto-verifier; until it lands,
		// be honest and reject rather than rubber-stamp.
		return Verdict{false, "defn auto-verification is not implemented yet — cite sha:/test:, or `nitpick waive` with a reason"}
	default:
		return Verdict{false, kind + ": evidence is not auto-verifiable — fix and cite sha:/test:, or `nitpick waive <ID> --reason …` instead"}
	}
}

func verifySha(dir, sha string) Verdict {
	if strings.TrimSpace(sha) == "" {
		return Verdict{false, "empty sha"}
	}
	cmd := exec.Command("git", "-C", dir, "cat-file", "-e", sha+"^{commit}")
	if err := cmd.Run(); err != nil {
		return Verdict{false, "commit not found in this repo: " + sha}
	}
	return Verdict{true, "commit exists: " + sha}
}

func verifyTest(dir, pattern string) Verdict {
	if strings.TrimSpace(pattern) == "" {
		return Verdict{false, "empty test pattern"}
	}
	cmd := exec.Command("go", "test", "-run", pattern, "-count=1", "-v", "./...")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	s := string(out)
	if strings.Contains(s, "--- FAIL:") || err != nil {
		return Verdict{false, "test failed or errored: " + pattern}
	}
	if !strings.Contains(s, "--- PASS:") {
		return Verdict{false, "no test matched pattern (nothing ran): " + pattern}
	}
	return Verdict{true, "test passed: " + pattern}
}

func splitKind(evidence string) (kind, value string, ok bool) {
	i := strings.IndexByte(evidence, ':')
	if i <= 0 {
		return "", "", false
	}
	return strings.ToLower(strings.TrimSpace(evidence[:i])), strings.TrimSpace(evidence[i+1:]), true
}
