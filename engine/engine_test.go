package engine

import "testing"

func TestIsPushToOrigin(t *testing.T) {
	cases := map[string]bool{
		"git push":                   true, // defaults to origin
		"git push origin":            true,
		"git push origin main":       true,
		"git push origin feature":    true, // any branch
		"git push -u origin feature": true,
		"git push upstream main":     false, // different remote
		"git push fork":              false,
		"git status":                 false, // not a push
	}
	for cmd, want := range cases {
		if got := isPushToOrigin(cmd); got != want {
			t.Errorf("isPushToOrigin(%q) = %v, want %v", cmd, got, want)
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
