package loop

import (
	"testing"

	"github.com/justinstimatze/stull/spec"
	"github.com/sudarkoff/nitpick/findings"
)

func scripted(term string) RawProvider { return func(spec.Cell) string { return term } }

func TestRecheck(t *testing.T) {
	f := findings.Record{FindingID: "RAR-01", Severity: "P1", Component: "sync",
		FailureMode: "hang", Recommendation: "add timeout"}

	cases := []struct {
		name     string
		provider RawProvider
		wantOK   bool
	}{
		{"model says resolved", scripted("resolved"), true},
		{"model says unresolved", scripted("unresolved"), false},
		{"out-of-language answer releases", scripted("maybe basically done"), true},
		{"empty completion (no key) skips", scripted(""), true},
		{"no provider configured skips", nil, true},
	}
	for _, tc := range cases {
		if v := Recheck(f, "sha:abc123", tc.provider); v.OK != tc.wantOK {
			t.Errorf("%s: OK=%v want %v (%s)", tc.name, v.OK, tc.wantOK, v.Detail)
		}
	}
}
