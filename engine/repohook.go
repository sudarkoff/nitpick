package engine

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sudarkoff/nitpick/findings"
)

// prePushHook is the git pre-push script nitpick init installs. It fails open if
// nitpick is not on PATH, and the user can always bypass with --no-verify.
const prePushHook = `#!/bin/sh
# nitpick pre-push gate — blocks a push to origin (any branch) while open P0/P1
# reliability findings remain. Managed by 'nitpick init'. Bypass: 'git push --no-verify'.
command -v nitpick >/dev/null 2>&1 || exit 0
exec nitpick precheck "$@"
`

// InitRepo installs the git pre-push gate in the current repository.
func InitRepo() int {
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "init: %v\n", err)
		return 1
	}
	return initRepoAt(wd)
}

func initRepoAt(dir string) int {
	hooksDir, err := gitHooksDir(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "init: not a git repository (%v)\n", err)
		return 1
	}
	dest := filepath.Join(hooksDir, "pre-push")
	if existing, err := os.ReadFile(dest); err == nil {
		if strings.Contains(string(existing), "nitpick precheck") {
			fmt.Fprintf(os.Stderr, "nitpick pre-push gate already installed at %s\n", dest)
			return 0
		}
		if err := os.WriteFile(dest+".bak", existing, 0o755); err == nil {
			fmt.Fprintf(os.Stderr, "backed up existing pre-push hook to %s.bak\n", dest)
		}
	}
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "init: %v\n", err)
		return 1
	}
	if err := os.WriteFile(dest, []byte(prePushHook), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "init: cannot write %s: %v\n", dest, err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "installed nitpick pre-push gate -> %s\n", dest)
	fmt.Fprintln(os.Stderr, "pushes to origin from any client now check open P0/P1 findings (bypass: git push --no-verify)")
	return 0
}

// Precheck is the git pre-push callback (`nitpick precheck <remote> <url>`): it
// blocks (exit 1) a push to the origin remote, on any branch, while open P0/P1
// findings remain. It fails open (exit 0) on any internal error so a nitpick
// problem never wedges pushing.
func Precheck() int {
	_, _ = io.Copy(io.Discard, os.Stdin) // drain the refs git writes to the hook
	remote := ""
	if len(os.Args) >= 3 {
		remote = os.Args[2] // os.Args = [nitpick, precheck, <remote>, <url>]
	}
	wd, _ := os.Getwd()
	return precheckAt(wd, remote)
}

func precheckAt(dir, remote string) int {
	if remote != "" && remote != "origin" {
		return 0 // only gate pushes to origin
	}
	repo := RepoForDir(dir)
	if repo == "" {
		return 0
	}
	store, err := findings.Open(DefaultDBDir())
	if err != nil {
		return 0
	}
	open, err := store.List(repo, "open")
	if err != nil || len(open) == 0 {
		return 0
	}
	fmt.Fprintf(os.Stderr, "nitpick: push to origin blocked — %d open P0/P1 reliability finding(s):\n", len(open))
	for _, r := range open {
		fmt.Fprintf(os.Stderr, "  - %s [%s] %s — %s\n", r.FindingID, r.Severity, r.Component, r.Recommendation)
	}
	fmt.Fprintln(os.Stderr, "Fix (`nitpick resolve <ID> --evidence …`) or waive (`nitpick waive <ID> --reason …`), or bypass with `git push --no-verify`.")
	return 1
}

func gitHooksDir(dir string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--git-path", "hooks").Output()
	if err != nil {
		return "", err
	}
	p := strings.TrimSpace(string(out))
	if !filepath.IsAbs(p) {
		p = filepath.Join(dir, p)
	}
	return p, nil
}
