# Evidence Index: agentdesk-permissions

## Evidence Index

| Surface | Authority | Canonical Artifact | Status | Route |
|---------|-----------|--------------------|--------|-------|
| truth | requirement_truth | truth.md | frozen | truth |
| tests | test_truth | tests.md | frozen | qa |
| tasks | acceptance_truth | tasks.md | current | implement |
| review | qa_verdict | review-independent.md | current | qa |

## Verification Log

Executed on darwin/arm64, 2026-11-04. The selector column is the exact `-run`
pattern; every row was run after the final fix, not before it.

| AC | Test | Result |
|----|------|--------|
| AC-011 | TestModeDefaultAndSessionAllow | PASS |
| AC-012 | TestApprovalBlocksThenResumes | PASS |
| AC-013 | TestApprovalTimeoutAndCancel | PASS |
| AC-014 | TestBypassWarningVisible | PASS |
| AC-015 | TestReasoningEffortForwarded | PASS |

Whole-suite state: all packages pass, `go vet` is clean, `go test -race` over
perm, agent, desktop and tool is clean, and the tree cross-compiles for
windows/amd64. The beacon autotest run over this package recorded 5 of 5 test
cases passed.

## Live behaviour, not just unit tests

A gated tool was exercised end to end against the real OpenCode Go endpoint.
Without approval the model was told the run was refused and it explained that
in its answer; with `-yes` the same request ran and the model reported the
actual command output. That is AC-012's shape outside a test harness.

## Two review rounds

Round one returned five findings: shell bounds were defeatable (a grandchild
held the pipe past the deadline and `CombinedOutput` buffered everything before
truncating), `Cancel` dropped session grants, the denial text did not match the
frozen string exactly, the frontend kept a single approval slot, and dead code
had accumulated. All four behavioural findings were fixed.

Round two re-checked the fixes and caught two gaps: the process tree was still
left alive after the deadline, and the new deadline test was cosmetic because
`sh -c "sleep 20"` execs rather than forks. Both were fixed: the shell now runs
in its own process group and kills the group on cancel, and the test now uses a
real grandchild so removing the fix makes it fail.

Round two also noted that the allow memory grows for the process lifetime. With
no session-delete API that is bounded by the number of conversations a single
run creates, which is acceptable for v0.1 and recorded here rather than hidden.
