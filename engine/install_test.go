package engine

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// install writes by default; --dry-run only simulates.
func TestInstall_DefaultWrites_DryRunDoesNot(t *testing.T) {
	if _, err := exec.LookPath("dolt"); err != nil {
		t.Skip("dolt not on PATH")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("NITPICK_DB", filepath.Join(t.TempDir(), "db"))
	skill := filepath.Join(home, ".claude", "skills", "nitpick", "SKILL.md")

	if rc := Install([]string{"--dry-run"}); rc != 0 {
		t.Fatalf("--dry-run rc=%d", rc)
	}
	if _, err := os.Stat(skill); !os.IsNotExist(err) {
		t.Errorf("--dry-run must not write the skill, but %s exists", skill)
	}

	if rc := Install(nil); rc != 0 {
		t.Fatalf("default install rc=%d", rc)
	}
	if _, err := os.Stat(skill); err != nil {
		t.Errorf("default install (no flags) should write the skill: %v", err)
	}
}
