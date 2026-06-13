package engine

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/sudarkoff/nitpick/skills"
)

// skillsDir returns the Claude Code skills directory for the given scope
// (alongside settings.json: ~/.claude/skills, or .claude/skills for --project).
func skillsDir(scope string) (string, error) {
	sp, err := settingsPath(scope)
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(sp), "skills"), nil
}

// installSkillFiles writes every embedded skill file under root. When write is
// false it only returns the destinations it would write (a dry run). An existing
// file whose content differs is backed up to <file>.bak before being overwritten.
func installSkillFiles(root string, write bool) ([]string, error) {
	var dests []string
	err := fs.WalkDir(skills.FS, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		content, err := skills.FS.ReadFile(p)
		if err != nil {
			return err
		}
		dest := filepath.Join(root, filepath.FromSlash(p))
		dests = append(dests, dest)
		if write {
			if err := writeSkillFile(dest, content); err != nil {
				return err
			}
		}
		return nil
	})
	return dests, err
}

func writeSkillFile(dest string, content []byte) error {
	if existing, err := os.ReadFile(dest); err == nil && !bytes.Equal(existing, content) {
		_ = os.WriteFile(dest+".bak", existing, 0o644)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dest, content, 0o644)
}
