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
nitpick init               # create the findings database
nitpick install --write    # wire the gate into Claude Code hooks
```

Requires Go 1.26+ and the `dolt` CLI on PATH.

## Usage

```bash
nitpick review --repo github.com/you/proj --from findings.txt  # ingest RAR-NN findings
nitpick list --repo github.com/you/proj --status open          # show open findings
nitpick resolve RAR-03 --evidence sha:abc123                    # mark fixed (evidence verified + re-checked)
nitpick waive RAR-04 --reason "accepted risk until Q3 ..."      # defer with a reason
nitpick defer RAR-05                                            # carry forward
```
