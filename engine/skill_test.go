package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sudarkoff/nitpick/skills"
)

// The embedded skill must instruct the model to emit exactly what the parser
// accepts, and to ingest via `nitpick review`. This guards skill/parser drift.
func TestEmbeddedSkill_MatchesParserAndIngestContract(t *testing.T) {
	b, err := skills.FS.ReadFile("reliability-architect-review/SKILL.md")
	if err != nil {
		t.Fatalf("embedded skill missing: %v", err)
	}
	s := string(b)
	for _, want := range []string{
		"FINDING RAR", "Promise at risk", "Component", "Failure mode",
		"Detection gap", "Recommendation", "nitpick review",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("embedded skill is missing %q (parser/ingest contract)", want)
		}
	}
}

func TestInstallSkillFiles(t *testing.T) {
	root := t.TempDir()

	dests, err := installSkillFiles(root, false) // dry run
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if len(dests) == 0 {
		t.Fatal("expected at least one skill file")
	}
	if _, err := os.Stat(dests[0]); !os.IsNotExist(err) {
		t.Errorf("dry run must not write files, but %s exists", dests[0])
	}

	if _, err = installSkillFiles(root, true); err != nil { // write
		t.Fatalf("write: %v", err)
	}
	want := filepath.Join(root, "reliability-architect-review", "SKILL.md")
	got, err := os.ReadFile(want)
	if err != nil {
		t.Fatalf("written skill: %v", err)
	}
	emb, _ := skills.FS.ReadFile("reliability-architect-review/SKILL.md")
	if string(got) != string(emb) {
		t.Error("written skill content != embedded")
	}
}
