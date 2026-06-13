package engine

import "testing"

func TestIsPushToMain(t *testing.T) {
	type c struct {
		cmd, branch string
		want        bool
	}
	cases := []c{
		{"git push origin main", "feature", true},      // explicit main ref
		{"git push -u origin main", "feature", true},   // flags + explicit main
		{"git push origin HEAD:main", "feature", true}, // refspec to main
		{"git push", "main", true},                     // bare push on main branch
		{"git push origin", "main", true},              // remote only, on main
		{"git push origin feature", "feature", false},  // explicit non-main ref
		{"git push", "feature", false},                 // bare push off main
		{"git push origin develop", "main", false},     // explicit ref overrides branch
		{"git status", "main", false},                  // not a push at all
		{"echo git push origin main", "x", true},       // still a push token to main (coarse, by design)
	}
	for _, tc := range cases {
		if got := isPushToMain(tc.cmd, tc.branch); got != tc.want {
			t.Errorf("isPushToMain(%q, branch=%q) = %v, want %v", tc.cmd, tc.branch, got, tc.want)
		}
	}
}

func TestNormalizeRemote(t *testing.T) {
	cases := map[string]string{
		"https://github.com/sudarkoff/twocal.git":   "github.com/sudarkoff/twocal",
		"https://github.com/sudarkoff/twocal":       "github.com/sudarkoff/twocal",
		"git@github.com:sudarkoff/twocal.git":       "github.com/sudarkoff/twocal",
		"ssh://git@github.com/sudarkoff/twocal.git": "github.com/sudarkoff/twocal",
	}
	for in, want := range cases {
		if got := normalizeRemote(in); got != want {
			t.Errorf("normalizeRemote(%q) = %q, want %q", in, got, want)
		}
	}
}
