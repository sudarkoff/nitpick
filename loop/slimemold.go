package loop

import (
	"os/exec"
	"strings"
)

// slimemoldMarkers are the vulnerability classes (from slimemold's audit) that
// bear on a "fixed" claim — premature closure and weak-basis confidence.
var slimemoldMarkers = []string{
	"premature closure",
	"load-bearing vibes",
	"ability overstatement",
	"sycophancy",
}

// SlimemoldConcerns runs slimemold's audit over the project at dir and, if it is
// installed and flags any closure/confidence vulnerability, returns an advisory
// string. It is intentionally non-blocking: slimemold reports project-wide
// epistemic concerns, not a verdict on one finding, so it informs the human
// rather than gating the resolve. Absent slimemold, it is silent.
func SlimemoldConcerns(dir string) string {
	if _, err := exec.LookPath("slimemold"); err != nil {
		return ""
	}
	cmd := exec.Command("slimemold", "audit")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	s := strings.ToLower(string(out))
	var hits []string
	for _, m := range slimemoldMarkers {
		if strings.Contains(s, m) {
			hits = append(hits, m)
		}
	}
	if len(hits) == 0 {
		return ""
	}
	return "slimemold flags active epistemic concerns in this project (" +
		strings.Join(hits, ", ") + ") — sanity-check the 'fixed' claim before trusting it (advisory)."
}
