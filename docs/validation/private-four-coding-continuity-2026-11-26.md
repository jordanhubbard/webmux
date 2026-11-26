# Private four-session coding continuity

On 2026-11-26, four fresh private SSH coding sessions passed the full continuity
qualifier at `3ad474614347d350a17de30ba5b546bce1a4335c`. Each submitted one real
model request using the candidate catalog's visible Code Mode model,
`gpt-6-astra`. No coding request was replayed and the native daemon was not
restarted.

This is a scoped private-runtime result. It does not qualify migration of the
four original legacy clients, establish their selected conversations, complete
native platform CI, or authorize installed activation. Model-catalog parity is
an open gate in [#761](https://github.com/jordanhubbard/webmux/issues/761).

## Images and package

- Native source: `5fdc79dd4fcf89db3006e06a712c31543cc87bd1`.
- Native CLI SHA-256:
  `cf5c228c51d23c3804c393906a61190205090d78471e9533fb388d38745ad50b`.
- Code Mode host SHA-256:
  `ce39555d59ffe4c4f06c6cf419de7eb4d53bddf05349c1182703ec55c16703c1`.
- Exec server SHA-256:
  `f7d260e05566c132ce18e197920992683f6094d59a645918592295680da07294`.
- Initial signed WebMux image SHA-256:
  `42d70a1ebfb7d37248a781dfaaeff7622f4fa17b917dc0d91507c859e80e1103`.
- Replacement signed WebMux image SHA-256:
  `2b002f34c89a32eec8721708da13884a52a5d94a313a51e3fc454c8ae841d07a`.

Both WebMux images used embedded assets. The native runtime used the complete
private resource layout with all three qualified native executables replaced;
this was not a standalone CLI-only installation test.

## Observed result

All four actors passed baseline, orderly old-to-new controller restart,
controller SIGKILL, and provider reconnect checks. At every phase, authenticated
native command-output counters increased while thread, turn, command item,
worker process/start identity, script hash, provider identity, and native daemon
identity remained unchanged. Management reads and selected-thread reports
matched the active thread. All four viewer replacements passed the management
handoff checks and worktree-read parity checks.

The original four private turns completed after the qualifier's STOP signal.
Each generated prime predicate passed an independent 2,084-case audit over
integers from -32 through 2051. That audit extracted only the predicate function
from the syntax tree; it did not rerun module-level coding commands or heartbeat
loops. Worktree creation was not tested.

Private cleanup passed with no remaining owned processes. A separate kernel
identity read confirmed unchanged process start times for the four original
TUIs, installed WebMux, and installed native daemon. Process identity alone
does not establish uninterrupted coding work in those original clients.

## Retained evidence and limitations

The sealed private archive contains 835 independently hash-verified files:

- Bytes: `30955022`.
- SHA-256: `f781ede9728ff6c79b775a5acb7348028947a125b18c5eb427a887f5e66d9590`.

Authentication files were omitted regardless of file type. Symlinks were
inventoried without following them. Raw private runtime state and credentials
are not published in this report.

Earlier failures remain retained: four unsupported-model errors in the first
private attempt, followed by a separate model-catalog preflight refusal with
zero coding requests. The passing candidate advertises version `0.0.0` and its
model catalog differs from the installed client's catalog. The cause is not yet
established. This passing `gpt-6-astra` run does not resolve that difference.

Required native platform failures and missing gates remain open. No installed
service, original coding request, or global configuration was changed by this
qualification.
