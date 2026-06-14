// Command nitpick records and queries reliability-architect-review findings in a
// Dolt database, gates pushes to main on unresolved must-fix findings (via the
// `run` hook dispatcher), and wires itself into Claude Code (`install`).
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/sudarkoff/nitpick/engine"
	"github.com/sudarkoff/nitpick/findings"
	"github.com/sudarkoff/nitpick/loop"
)

const defaultSkill = "reliability-architect-review"

const usage = `nitpick — reliability findings gate

usage:
  nitpick init                                   set up THIS REPO (install the git pre-push gate)
  nitpick doctor                                 report dependency availability
  nitpick install [binary] [--project] [--dry-run] set up this MACHINE (db + skill + Claude Code hooks)
  nitpick run                                    hook dispatcher (reads an event on stdin)
  nitpick review [--repo R] [--skill S] [--from FILE]
                                                 ingest RAR-NN findings (stdin if no --from)
  nitpick list   [--repo R] [--status S]         list findings (status: open|resolved|deferred)
  nitpick resolve ID [--repo R] --evidence E     mark a finding fixed (evidence is verified)
  nitpick waive   ID [--repo R] --reason TEXT    defer a finding with a written reason
  nitpick defer   ID [--repo R]                  carry a finding forward

The findings DB lives at $NITPICK_DB or ~/.local/share/nitpick/db.
--repo defaults to the current repo's git origin.`

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "init":
		return engine.InitRepo()
	case "precheck":
		return engine.Precheck()
	case "doctor":
		return engine.Doctor(stdout)
	case "install":
		return engine.Install(rest)
	case "hook", "run":
		return engine.Hook()
	case "review":
		return cmdReview(rest, stdout, stderr)
	case "list":
		return cmdList(rest, stdout, stderr)
	case "resolve":
		return cmdResolve(rest, stdout, stderr)
	case "waive":
		return cmdWaive(rest, stdout, stderr)
	case "defer":
		return cmdDefer(rest, stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprintln(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n%s\n", cmd, usage)
		return 2
	}
}

func cmdReview(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("review", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repo := fs.String("repo", "", "repository identifier (default: git origin)")
	skill := fs.String("skill", defaultSkill, "skill that produced the findings")
	from := fs.String("from", "", "read findings from FILE (default: stdin)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	r := engine.ResolveRepo(*repo)
	if r == "" {
		fmt.Fprintln(stderr, "review: --repo required (could not detect git origin)")
		return 2
	}
	var text []byte
	var err error
	if *from != "" {
		text, err = os.ReadFile(*from)
	} else {
		text, err = io.ReadAll(os.Stdin)
	}
	if err != nil {
		fmt.Fprintf(stderr, "review: %v\n", err)
		return 1
	}
	store, err := findings.Open(engine.DefaultDBDir())
	if err != nil {
		fmt.Fprintf(stderr, "review: %v\n", err)
		return 1
	}
	n, err := findings.Ingest(store, r, *skill, os.Getenv("CLAUDE_SESSION_ID"), string(text))
	if err != nil {
		fmt.Fprintf(stderr, "review: %v\n", err)
		return 1
	}
	open, _ := store.List(r, "open")
	deferred, _ := store.List(r, "deferred")
	fmt.Fprintf(stdout, "ingested %d findings for %s (%d open, %d deferred)\n", n, r, len(open), len(deferred))
	return 0
}

func cmdList(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repo := fs.String("repo", "", "repository identifier (default: git origin)")
	status := fs.String("status", "", "filter by status: open|resolved|deferred")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	r := engine.ResolveRepo(*repo)
	if r == "" {
		fmt.Fprintln(stderr, "list: --repo required (could not detect git origin)")
		return 2
	}
	store, err := findings.Open(engine.DefaultDBDir())
	if err != nil {
		fmt.Fprintf(stderr, "list: %v\n", err)
		return 1
	}
	recs, err := store.List(r, *status)
	if err != nil {
		fmt.Fprintf(stderr, "list: %v\n", err)
		return 1
	}
	if len(recs) == 0 {
		fmt.Fprintln(stdout, "no findings")
		return 0
	}
	for _, rec := range recs {
		fmt.Fprintf(stdout, "%-7s %-2s %-9s %-28s %s\n",
			rec.FindingID, rec.Severity, rec.Status, truncate(rec.Component, 28), truncate(rec.Recommendation, 60))
		if rec.Status == "deferred" && rec.WaiverReason != "" {
			fmt.Fprintf(stdout, "          waived: %s\n", rec.WaiverReason)
		}
	}
	return 0
}

func cmdResolve(args []string, stdout, stderr io.Writer) int {
	id, rest := popPositional(args)
	fs := flag.NewFlagSet("resolve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repo := fs.String("repo", "", "repository identifier (default: git origin)")
	evidence := fs.String("evidence", "", "evidence for the fix (sha:… / test:…)")
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	if id == "" || *evidence == "" {
		fmt.Fprintln(stderr, "resolve: usage: nitpick resolve ID --evidence E")
		return 2
	}
	r := engine.ResolveRepo(*repo)
	if r == "" {
		fmt.Fprintln(stderr, "resolve: --repo required (could not detect git origin)")
		return 2
	}
	store, err := findings.Open(engine.DefaultDBDir())
	if err != nil {
		fmt.Fprintf(stderr, "resolve: %v\n", err)
		return 1
	}
	rec, err := store.Get(r, id)
	if err != nil {
		fmt.Fprintf(stderr, "resolve: %v\n", err)
		return 1
	}
	if rec == nil {
		fmt.Fprintf(stderr, "resolve: no finding %s for %s\n", id, r)
		return 1
	}

	dir, _ := os.Getwd()

	// Gate 1 — deterministic: the cited evidence must be real.
	ev := loop.VerifyEvidence(dir, *evidence)
	if !ev.OK {
		fmt.Fprintf(stderr, "resolve %s rejected: %s\n", id, ev.Detail)
		return 1
	}
	fmt.Fprintf(stdout, "evidence verified — %s\n", ev.Detail)

	// Gate 2 — fenced oracle: does the evidence actually resolve the finding?
	rc := loop.Recheck(*rec, *evidence, loop.AnthropicRaw)
	if !rc.OK {
		fmt.Fprintf(stderr, "resolve %s rejected: %s\n", id, rc.Detail)
		return 1
	}
	fmt.Fprintf(stdout, "%s\n", rc.Detail)

	// Advisory — slimemold (non-blocking).
	if c := loop.SlimemoldConcerns(dir); c != "" {
		fmt.Fprintf(stdout, "%s\n", c)
	}

	if err := store.SetStatus(r, id, "resolved", *evidence, ""); err != nil {
		fmt.Fprintf(stderr, "resolve: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "%s -> resolved\n", id)
	return 0
}

func cmdWaive(args []string, stdout, stderr io.Writer) int {
	id, rest := popPositional(args)
	fs := flag.NewFlagSet("waive", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repo := fs.String("repo", "", "repository identifier (default: git origin)")
	reason := fs.String("reason", "", "why this finding is being deferred rather than fixed now")
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	if id == "" || *reason == "" {
		fmt.Fprintln(stderr, "waive: usage: nitpick waive ID --reason TEXT")
		return 2
	}
	return setStatus(*repo, id, "deferred", "", *reason, stdout, stderr)
}

func cmdDefer(args []string, stdout, stderr io.Writer) int {
	id, rest := popPositional(args)
	fs := flag.NewFlagSet("defer", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repo := fs.String("repo", "", "repository identifier (default: git origin)")
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	if id == "" {
		fmt.Fprintln(stderr, "defer: usage: nitpick defer ID")
		return 2
	}
	return setStatus(*repo, id, "deferred", "", "", stdout, stderr)
}

func setStatus(repo, id, status, evidence, reason string, stdout, stderr io.Writer) int {
	r := engine.ResolveRepo(repo)
	if r == "" {
		fmt.Fprintln(stderr, "--repo required (could not detect git origin)")
		return 2
	}
	store, err := findings.Open(engine.DefaultDBDir())
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}
	if err := store.SetStatus(r, id, status, evidence, reason); err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "%s -> %s\n", id, status)
	return 0
}

// popPositional removes and returns the first token as the finding ID; the rest
// are flags. A leading flag means no ID was given.
func popPositional(args []string) (string, []string) {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		return args[0], args[1:]
	}
	return "", args
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}
