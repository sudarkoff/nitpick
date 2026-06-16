# nitpick

A Claude Code gate that runs the bundled `nitpick` reliability-review skill at the right moments,
files every finding in a [Dolt](https://www.dolthub.com/) database, blocks a push to
`origin` (any branch) until the must-fix (P0/P1) findings are genuinely fixed — or waived with a written
reason — and hands the rest forward with enough context to fix later.

Built on [stull](https://github.com/justinstimatze/stull) (guarded hook state machines),
with optional [defn](https://github.com/justinstimatze/defn) (Go structural evidence) and
[slimemold](https://github.com/justinstimatze/slimemold) (false-completion detection).

## Status

Early. See [`docs/superpowers/specs/2026-06-13-nitpick-design.md`](docs/superpowers/specs/2026-06-13-nitpick-design.md)
for the design. Phases 1–3 complete (the optional defn auto-verify is deferred).

## Install

```bash
go install github.com/sudarkoff/nitpick/cmd/nitpick@latest
nitpick doctor             # check dependencies (dolt required; slimemold/defn/API key optional)
nitpick install            # machine setup: findings DB + skill + Claude Code hooks  (preview: --dry-run)

# then, in each repository you want gated from the terminal too:
nitpick init               # repo setup: git pre-push gate (blocks ANY client)
```

Requires Go 1.26+ and the `dolt` CLI on PATH.

`nitpick install` is machine-wide setup: it ensures the findings DB, installs the
bundled `nitpick` skill (the renamed reliability review) into `~/.claude/skills`, and registers
the Claude Code gate hooks (which gate *Claude's* pushes). `nitpick init` is per-repo:
it installs a git `pre-push` hook so a push to `origin` (any branch) from *any* client — your
terminal or the agent — is checked (bypass with `git push --no-verify`).

The installed skill ends by running `nitpick review`, which records findings, so
`nitpick list` reflects exactly what has been ingested (run a review to populate it).

## The `nitpick` skill

`nitpick install` bundles a Claude Code skill (also named `nitpick`) into
`~/.claude/skills`. It is what turns a plain reliability review into tracked,
gate-enforced findings. Say **"nitpick please"** (or "nitpick this", "run a
nitpick") and the skill activates; ask **"how many P1 nitpicks are open?"** and it
answers from the database instead of re-reviewing.

The skill has two modes and picks one per request:

- **Query mode** — answers questions *about* existing findings ("what's
  deferred?", "is NP-03 still open?", "anything blocking a push?") by reading
  `nitpick list`. It does not start a new review.
- **Review mode** — runs the bundled reliability framework, adopting the mindset
  of a principal engineer who has been paged at 3am. It works through five phases
  in order:
  1. **Promise audit** — catalog every reliability promise (uptime, latency, data
     integrity, recovery) and find the code that actually keeps it.
  2. **Failure-mode analysis** — for each component, ask whether failures are
     detected in <5min, auto-recover, and have a runbook; watches for the
     "liveness trap" (healthy process doing no useful work).
  3. **Resource-exhaustion scenarios** — audit every bounded resource (connection
     pools, goroutines, queue depth, FDs/memory) for cliffs and missing timeouts.
  4. **Scale-tier analysis** — re-run the review at 10×, 100×, and 1000× load to
     find which component saturates first and what the minimum fix is.
  5. **Operational readiness** — verify detection, alerting, and automatic/manual
     recovery for each failure mode.

Each finding comes out in a fixed format with a stable ID and severity:

```
FINDING NP-01 [P0]
Promise at risk: <claim made to users>
Component: <file, service, or layer>
Failure mode: <what breaks and how>
Detection gap: <how long before we know, or "undetected">
Recommendation: <specific, implementable fix>
```

Severity drives the gate: **P0/P1** (silent failure, data at risk, or no
auto-recovery) are recorded as `open` and block a push to `origin` until fixed or
waived; **P2/P3** are filed as deferred so they carry forward without losing
context. The skill finishes by writing the findings to a file and running
`nitpick review --from <file>`, so everything it surfaces lands in the database
and shows up in `nitpick list`.

## Usage

```bash
nitpick review --repo github.com/you/proj --from findings.txt  # ingest NP-NN findings
nitpick list --repo github.com/you/proj --status open          # show open findings
nitpick resolve NP-03 --evidence sha:abc123                    # mark fixed (evidence verified + re-checked)
nitpick waive NP-04 --reason "accepted risk until Q3 ..."      # defer with a reason
nitpick defer NP-05                                            # carry forward
```
