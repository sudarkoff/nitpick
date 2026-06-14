// Package engine is nitpick's impure shell: the hook dispatcher and installer.
// It computes DB- and git-derived facts and injects them into the hook event,
// then runs the (pure) stull gate machine. It is the only place that touches the
// database, git, and the filesystem on the hot path.
package engine

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/justinstimatze/stull/compile"
	"github.com/justinstimatze/stull/runtime"
	"github.com/justinstimatze/stull/spec"

	"github.com/sudarkoff/nitpick/findings"
	"github.com/sudarkoff/nitpick/machine"
)

var pushRe = regexp.MustCompile(`(?i)\bgit\s+push\b`)

// sensitivePaths flags files whose change warrants a reliability review.
var sensitivePaths = regexp.MustCompile(`(?i)(sync|webhook|queue|jobs?|worker|pool|health|/db/|client|poll)`)

// isPushToMain reports whether a shell command pushes to the main branch, given
// the current branch. An explicit refspec wins over the branch; a bare push
// falls back to the current branch.
func isPushToMain(command, branch string) bool {
	if !pushRe.MatchString(command) {
		return false
	}
	fields := strings.Fields(command)
	idx := -1
	for i, f := range fields {
		if f == "push" {
			idx = i
			break
		}
	}
	if idx < 0 {
		return false
	}
	var nonFlags []string
	for _, f := range fields[idx+1:] {
		if !strings.HasPrefix(f, "-") {
			nonFlags = append(nonFlags, f)
		}
	}
	var refspecs []string
	if len(nonFlags) > 1 { // first non-flag is the remote
		refspecs = nonFlags[1:]
	}
	for _, r := range refspecs {
		if r == "main" || strings.HasSuffix(r, ":main") {
			return true
		}
	}
	if len(refspecs) > 0 {
		return false // an explicit non-main ref
	}
	return branch == "main" // bare push -> current branch
}

// Hook is the live dispatcher: read one hook event on stdin, enrich it with
// DB/git facts, run the gate machine, emit the hook protocol. Fail-open
// throughout — a broken nitpick must never wedge a session.
func Hook() int {
	var event map[string]any
	if err := json.NewDecoder(os.Stdin).Decode(&event); err != nil {
		return 0
	}
	enrichEvent(event)
	m := machine.Machine()
	ctx := runtime.LoadContext(m, event)
	out := runtime.SafeDispatch(m, event, ctx, runtime.AnthropicModel)
	_ = runtime.SaveContext(m, ctx)
	return runtime.Emit(spec.Trigger(asString(event["hook_event_name"])), out)
}

// enrichEvent adds the nitpick_* event fields the machine's guards read. Any
// failure leaves the event unenriched, so the gate simply does not fire.
func enrichEvent(event map[string]any) {
	defer func() { _ = recover() }()

	dir := asString(event["cwd"])
	if dir == "" {
		dir, _ = os.Getwd()
	}
	repo := RepoForDir(dir)
	if repo == "" {
		return
	}
	store, err := findings.Open(DefaultDBDir())
	if err != nil {
		return
	}

	switch asString(event["hook_event_name"]) {
	case "PreToolUse":
		cmd := commandOf(event)
		if cmd == "" || !isPushToMain(cmd, gitBranch(dir)) {
			return
		}
		open, _ := store.List(repo, "open")
		if len(open) == 0 {
			return
		}
		event[machine.FieldOpenBlockers] = strconv.Itoa(len(open))
		event[machine.FieldBlockReason] = blockReason(open)
	case "SessionStart":
		open, _ := store.List(repo, "open")
		deferred, _ := store.List(repo, "deferred")
		if len(open)+len(deferred) == 0 {
			return
		}
		event[machine.FieldSummary] = summaryText(repo, len(open), len(deferred))
	case "Stop":
		if paths := sensitiveChanged(dir); len(paths) > 0 {
			event[machine.FieldReviewDue] = "1"
			event[machine.FieldNudge] = nudgeText(paths)
		}
	}
}

func blockReason(open []findings.Record) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Push blocked: %d open P0/P1 reliability finding(s) must be fixed or waived first:\n", len(open))
	for _, r := range open {
		fmt.Fprintf(&b, "  - %s [%s] %s — %s\n", r.FindingID, r.Severity, r.Component, r.Recommendation)
	}
	b.WriteString("Fix each (`nitpick resolve <ID> --evidence …`) or waive with a reason " +
		"(`nitpick waive <ID> --reason …`), then push again.")
	return b.String()
}

func summaryText(repo string, open, deferred int) string {
	return fmt.Sprintf("nitpick: %d open P0/P1 and %d deferred reliability finding(s) for %s. "+
		"`nitpick list --status open` to see what blocks a push to main.", open, deferred, repo)
}

func nudgeText(paths []string) string {
	sample := paths
	if len(sample) > 3 {
		sample = sample[:3]
	}
	return fmt.Sprintf("nitpick: reliability-sensitive paths changed with no review on record (e.g. %s). "+
		"Consider running reliability-architect-review before pushing to main.", strings.Join(sample, ", "))
}

// sensitiveChanged returns changed tracked files (vs HEAD) matching sensitivePaths.
func sensitiveChanged(dir string) []string {
	out, err := git(dir, "diff", "--name-only", "HEAD")
	if err != nil {
		return nil
	}
	var hits []string
	for _, line := range strings.Split(out, "\n") {
		if line != "" && sensitivePaths.MatchString(line) {
			hits = append(hits, line)
		}
	}
	return hits
}

// Install sets up nitpick on this machine: it ensures the findings database
// exists, writes the embedded reliability-architect-review skill into the skills
// directory, and merges nitpick's hook fragment into settings.json (idempotently,
// with a backup). Per-repository setup (the git pre-push gate) is `nitpick init`.
//
//	nitpick install [binary] [--project] [--dry-run]
func Install(args []string) int {
	dryRun := false
	scope := "global"
	binary := "nitpick"
	for _, a := range args {
		switch a {
		case "--dry-run":
			dryRun = true
		case "--write":
			// writing is the default now; --write is accepted for compatibility
		case "--project":
			scope = "project"
		case "--global":
			scope = "global"
		default:
			binary = a
		}
	}

	target, err := settingsPath(scope)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	skillRoot, err := skillsDir(scope)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	m := machine.Machine()
	existing, err := readSettings(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "install: cannot read %s: %v\n", target, err)
		return 1
	}
	merged, added, err := compile.MergeHooks(existing, m, binary)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	mergedJSON, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	if dryRun {
		fmt.Fprintf(os.Stderr, "would ensure findings database at %s\n", DefaultDBDir())
		dests, _ := installSkillFiles(skillRoot, false)
		for _, d := range dests {
			fmt.Fprintf(os.Stderr, "would install skill -> %s\n", d)
		}
		if added == 0 {
			fmt.Fprintf(os.Stderr, "hooks: already present in %s\n", target)
		} else {
			fmt.Fprintf(os.Stderr, "hooks: would add %d trigger entr(y/ies) to %s\n", added, target)
		}
		fmt.Fprintln(os.Stderr, "(dry run — omit --dry-run to apply)")
		fmt.Println(string(mergedJSON))
		return 0
	}

	if _, err := findings.Open(DefaultDBDir()); err != nil {
		fmt.Fprintf(os.Stderr, "install: findings database: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "findings database ready at %s\n", DefaultDBDir())

	dests, err := installSkillFiles(skillRoot, true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "install: skill: %v\n", err)
		return 1
	}
	for _, d := range dests {
		fmt.Fprintf(os.Stderr, "installed skill -> %s\n", d)
	}

	if added == 0 {
		fmt.Fprintf(os.Stderr, "hooks: already present in %s\n", target)
		return 0
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := os.WriteFile(target, mergedJSON, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "install: cannot write %s: %v\n", target, err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "installed %d hook entr(y/ies) -> %s\n", added, target)
	return 0
}

func settingsPath(scope string) (string, error) {
	if scope == "project" {
		return ".claude/settings.json", nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "settings.json"), nil
}

func readSettings(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(string(b)) == "" {
		return map[string]any{}, nil
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// --- shared repo/DB helpers (single source of truth; the CLI uses these too) ---

// DefaultDBDir is the findings database location ($NITPICK_DB or ~/.local/share/nitpick/db).
func DefaultDBDir() string {
	if d := os.Getenv("NITPICK_DB"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "nitpick", "db")
}

// ResolveRepo returns the explicit repo if given, else the git origin of the cwd.
func ResolveRepo(explicit string) string {
	if explicit != "" {
		return explicit
	}
	return RepoForDir("")
}

// RepoForDir returns the normalized git-origin identifier for a directory
// ("" runs git in the current directory).
func RepoForDir(dir string) string {
	out, err := git(dir, "remote", "get-url", "origin")
	if err != nil {
		return ""
	}
	return normalizeRemote(strings.TrimSpace(out))
}

func gitBranch(dir string) string {
	out, err := git(dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func git(dir string, args ...string) (string, error) {
	full := args
	if dir != "" {
		full = append([]string{"-C", dir}, args...)
	}
	out, err := exec.Command("git", full...).Output()
	return string(out), err
}

func normalizeRemote(url string) string {
	url = strings.TrimSuffix(url, ".git")
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "http://")
	url = strings.TrimPrefix(url, "ssh://")
	if i := strings.Index(url, "@"); i >= 0 {
		url = url[i+1:]
	}
	return strings.Replace(url, ":", "/", 1)
}

func commandOf(event map[string]any) string {
	ti, ok := event["tool_input"].(map[string]any)
	if !ok {
		return ""
	}
	return asString(ti["command"])
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// Doctor reports the availability of nitpick's dependencies and what degrades
// when one is missing.
func Doctor(stdout io.Writer) int {
	report := func(name string, ok bool, note string) {
		mark := "MISSING"
		if ok {
			mark = "ok"
		}
		fmt.Fprintf(stdout, "  %-18s %-8s %s\n", name, mark, note)
	}
	has := func(bin string) bool { _, err := exec.LookPath(bin); return err == nil }
	fmt.Fprintln(stdout, "nitpick doctor:")
	report("dolt", has("dolt"), "required — findings database")
	report("git", has("git"), "required — push detection + sha: evidence")
	report("slimemold", has("slimemold"), "optional — false-completion advisory at resolve")
	report("defn", has("defn"), "optional — defn: evidence (auto-verify not implemented yet)")
	report("ANTHROPIC_API_KEY", os.Getenv("ANTHROPIC_API_KEY") != "", "optional — LLM re-check at resolve")
	fmt.Fprintf(stdout, "  db dir: %s\n", DefaultDBDir())
	return 0
}
