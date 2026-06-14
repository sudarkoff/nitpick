# nitpick

A Claude Code gate that runs the `reliability-architect-review` skill at the right moments,
files every finding in a [Dolt](https://www.dolthub.com/) database, blocks a push to
`main` until the must-fix (P0/P1) findings are genuinely fixed — or waived with a written
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
nitpick install --write    # machine setup: findings DB + skill + Claude Code hooks

# then, in each repository you want gated from the terminal too:
nitpick init               # repo setup: git pre-push gate (blocks ANY client)
```

Requires Go 1.26+ and the `dolt` CLI on PATH.

`nitpick install` is machine-wide setup: it ensures the findings DB, installs the
bundled `reliability-architect-review` skill into `~/.claude/skills`, and registers
the Claude Code gate hooks (which gate *Claude's* pushes). `nitpick init` is per-repo:
it installs a git `pre-push` hook so a push to `main` from *any* client — your
terminal or the agent — is checked (bypass with `git push --no-verify`).

The installed skill ends by running `nitpick review`, which records findings, so
`nitpick list` reflects exactly what has been ingested (run a review to populate it).

## Usage

```bash
nitpick review --repo github.com/you/proj --from findings.txt  # ingest RAR-NN findings
nitpick list --repo github.com/you/proj --status open          # show open findings
nitpick resolve RAR-03 --evidence sha:abc123                    # mark fixed (evidence verified + re-checked)
nitpick waive RAR-04 --reason "accepted risk until Q3 ..."      # defer with a reason
nitpick defer RAR-05                                            # carry forward
```
