# Evidence Index: agentdesk

## Evidence Index

| Surface | Authority | Canonical Artifact | Status | Route |
|---------|-----------|--------------------|--------|-------|
| truth | requirement_truth | truth.md | frozen | truth |
| tests | test_truth | tests.md | frozen | qa |
| tasks | acceptance_truth | tasks.md | current | implement |
| review | qa_verdict | review-independent.md | current | qa |

## Verification Log

Every row was executed on darwin/arm64 on 2026-11-04 (Go 1.25.6, toolchain
auto-upgraded to go1.26.10 by the wails v2 module). The selector column is the
exact `-run` pattern; nothing here is a hand-written hash.

| AC | Test | Result |
|----|------|--------|
| AC-001 | TestRegistryListsProviders | PASS |
| AC-002 | TestStreamSplitsReasoningAndContent | PASS |
| AC-003 | TestLoopExecutesToolAndConverges | PASS |
| AC-004 | TestStreamEventsWired | PASS |
| AC-005 | TestRunCommandStreams | PASS |
| AC-006 | TestSessionPersistsAcrossReopen | PASS |
| AC-007 | TestSecretsNeverCommitted | PASS |
| AC-008 | TestUnregisteredToolRejected | PASS |
| AC-009 | TestLiveProviders | PASS |
| AC-010 | TestOpenSourceFilesPresent | PASS |

Whole-suite state: all packages pass, go vet is clean, gofmt reports nothing.
The beacon autotest run over this package recorded 10 of 10 test cases passed.

## Real provider closure

Both configured providers answered a live streaming request with the literal
token PONG: the OpenCode Go endpoint on opencode.ai and the Command Code
endpoint on api.commandcode.ai, each in under five seconds and each reporting
a non-zero usage total. A real tool round trip was also executed end to end
against OpenCode Go: the model requested the get_time tool, the loop executed
it, fed the timestamp back, and the model produced the final answer. The
desktop build produced one Mach-O arm64 executable of about 13 MB, and the app
window rendered with the persisted session list.

## Known external flakiness

The Command Code endpoint intermittently dropped connections before the
response headers from this network during verification (measured 4 of 6 and
then 1 of 5 in two bursts) while OpenCode Go stayed at 100 percent. The same
autotest run failed one case twice and then passed ten of ten twice with no
code change. The transport retries a request up to three times on
transport-level errors before streaming starts, which is safe because nothing
has been consumed at that point; streaming itself is never replayed. This
behaviour is covered by TestStreamRetriesOnConnectionDrop.

## Independent review

A separate reviewer that did not write the code returned PASS on all ten
checks. It raised five findings; four were fixed in the same release (tool
approval default-deny in the desktop, nil-store guards, an honest availability
flag in the registry, and a duplicated error emit in the loop). The fifth
(defensive skipping of malformed stream chunks, and a hand-rolled dotenv
parser) was accepted and is documented in the review file.
