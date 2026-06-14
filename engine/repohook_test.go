package engine

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRefsPushToMain(t *testing.T) {
	cases := []struct {
		name, stdin string
		want        bool
	}{
		{"push to main", "abc 111 refs/heads/main 000", true},
		{"feature only", "abc 111 refs/heads/feature 222", false},
		{"main among several", "a 1 refs/heads/feat 2\nb 3 refs/heads/main 4", true},
		{"empty", "", false},
		{"main-lookalike branch", "a 1 refs/heads/main-thing 2", false},
	}
	for _, tc := range cases {
		if got := refsPushToMain(tc.stdin); got != tc.want {
			t.Errorf("%s: refsPushToMain=%v want %v", tc.name, got, tc.want)
		}
	}
}

func TestInitRepoAt(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	if rc := initRepoAt(dir); rc != 0 {
		t.Fatalf("initRepoAt rc=%d", rc)
	}
	hook := filepath.Join(dir, ".git", "hooks", "pre-push")
	info, err := os.Stat(hook)
	if err != nil {
		t.Fatalf("pre-push not written: %v", err)
	}
	if info.Mode()&0o100 == 0 {
		t.Errorf("pre-push hook is not executable (mode %v)", info.Mode())
	}
	b, _ := os.ReadFile(hook)
	if !strings.Contains(string(b), "nitpick precheck") {
		t.Errorf("hook does not call nitpick precheck:\n%s", b)
	}
	if rc := initRepoAt(dir); rc != 0 { // idempotent
		t.Fatalf("second initRepoAt rc=%d", rc)
	}
}
