# Independent review — agentdesk-permissions

Reviewer: a separate agent that did not write this code. Two rounds.

## Round 1: FAIL on one security check, all five ACs pass

| # | Check | Verdict |
|---|-------|---------|
| 1 | build + full test suite | PASS |
| 2 | default `ask`, allow scoped to (session, tool) | PASS |
| 3 | approval really blocks the loop | PASS |
| 4 | timeout and cancel are bounded | PASS |
| 5 | bypass skips the ask and reports a warning | PASS |
| 6 | `reasoning_effort` omitted when unset | PASS |
| 7 | `run_shell` bounds | FAIL |
| 8 | no silent auto-approve in the desktop | PASS |
| 9 | dead code and duplication | FAIL (minor) |
| 10 | goroutine or pending-map leak | PASS |

Findings: `CombinedOutput` buffered everything before truncating (OOM vector);
`exec.CommandContext` killed only `sh`, so a grandchild holding the pipe kept
the wait alive; `Cancel` dropped session grants, contradicting AC-011; the
denial string did not equal the frozen text; the frontend held a single
approval slot so a second concurrent request was lost.

## Round 2: re-verify after fixes

| Fix | Verdict |
|-----|---------|
| bounded shell output and wait | PASS (half-fixed: process tree still alive) |
| cancel keeps session grants | PASS |
| exact denial text | PASS |
| approval queue in the frontend | PASS |

New findings: the process group was never killed, and the deadline test was
cosmetic because `sh -c "sleep 20"` execs rather than forks.

## Round 3: final state

The shell now sets its own process group and kills the group on cancel, with a
non-unix stub so the tree still cross-compiles. The deadline test now uses a
real grandchild, so removing the process-group kill makes it fail. The dead
grant-clearing helper was removed.

Remaining accepted limitation: the per-session allow map grows for the process
lifetime. There is no session-delete API yet, so it is bounded by the number of
conversations created in one run.
