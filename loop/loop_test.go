package loop

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestVerifyEvidence_Sha(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	run(t, dir, "git", "init", "-q")
	run(t, dir, "git", "commit", "-q", "--allow-empty", "-m", "x")
	sha := output(t, dir, "git", "rev-parse", "HEAD")

	if v := VerifyEvidence(dir, "sha:"+sha); !v.OK {
		t.Errorf("real sha should verify: %s", v.Detail)
	}
	if v := VerifyEvidence(dir, "sha:deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"); v.OK {
		t.Errorf("fake sha should NOT verify: %s", v.Detail)
	}
}

func TestVerifyEvidence_Test(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH")
	}
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module tmptest\n\ngo 1.26\n")
	writeFile(t, dir, "x_test.go", "package tmptest\nimport \"testing\"\nfunc TestPass(t *testing.T){}\nfunc TestFail(t *testing.T){ t.Fatal(\"boom\") }\n")

	if v := VerifyEvidence(dir, "test:TestPass"); !v.OK {
		t.Errorf("passing test should verify: %s", v.Detail)
	}
	if v := VerifyEvidence(dir, "test:TestFail"); v.OK {
		t.Errorf("failing test should NOT verify: %s", v.Detail)
	}
	if v := VerifyEvidence(dir, "test:TestNope"); v.OK {
		t.Errorf("non-matching test should NOT verify (nothing ran): %s", v.Detail)
	}
}

func TestVerifyEvidence_NonVerifiableRejected(t *testing.T) {
	for _, ev := range []string{"note:trust me", "alert:added a metric", "manual:done", "garbage", ""} {
		if v := VerifyEvidence(t.TempDir(), ev); v.OK {
			t.Errorf("%q should be rejected (use waive), got OK: %s", ev, v.Detail)
		}
	}
}

func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	c := exec.Command(name, args...)
	c.Dir = dir
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v: %s", name, args, err, out)
	}
}

func output(t *testing.T, dir, name string, args ...string) string {
	t.Helper()
	c := exec.Command(name, args...)
	c.Dir = dir
	out, err := c.Output()
	if err != nil {
		t.Fatalf("%s %v: %v", name, args, err)
	}
	return string(trimNL(out))
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func trimNL(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r') {
		b = b[:len(b)-1]
	}
	return b
}
