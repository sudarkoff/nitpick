package loop

import (
	"os"
	"path/filepath"
	"testing"
)

func writeExec(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestSlimemoldConcerns_FlagsPrematureClosure(t *testing.T) {
	bin := t.TempDir()
	writeExec(t, bin, "slimemold", "#!/bin/sh\necho 'Vulnerabilities: Premature closure on claim X; load-bearing vibes'\n")
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	if c := SlimemoldConcerns(t.TempDir()); c == "" {
		t.Error("expected concerns from a flagging audit, got empty")
	}
}

func TestSlimemoldConcerns_CleanAudit(t *testing.T) {
	bin := t.TempDir()
	writeExec(t, bin, "slimemold", "#!/bin/sh\necho 'No vulnerabilities found.'\n")
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	if c := SlimemoldConcerns(t.TempDir()); c != "" {
		t.Errorf("clean audit should yield nothing, got %q", c)
	}
}

func TestSlimemoldConcerns_NotInstalled(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // no slimemold here
	if c := SlimemoldConcerns(t.TempDir()); c != "" {
		t.Errorf("missing slimemold should be silent, got %q", c)
	}
}
