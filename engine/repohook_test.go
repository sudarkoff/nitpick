package engine

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestPrecheckAt_NonOriginRemoteAllowed(t *testing.T) {
	// A push to a non-origin remote is not gated, regardless of findings.
	if rc := precheckAt(t.TempDir(), "upstream"); rc != 0 {
		t.Errorf("push to non-origin remote should be allowed, got rc=%d", rc)
	}
}
