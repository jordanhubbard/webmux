# External Rig execution-host provenance — 2026-11-17

This candidate addresses #430 under #194/#318. A local tmux attachment is not
evidence that the provider executes on localhost. Canonical seats without a
WebMux launch record now retain an empty execution host, displayed as **Unknown
execution host**. The CLI's opaque `hostSelfId` is preserved separately and is
never used as a connection destination. Own local/SSH launch records retain
their existing host values.

Inventory, persisted terminal metadata, tile/navigation labels and recovery
previews preserve the distinction. An unknown destination cannot reach native
recovery observation or preparation. Opening an older viewer from canonical
inventory corrects its inferred localhost label without creating another viewer
or stopping its provider; subsequent persistence retains the unknown value.
This is not automatic adoption, a verified remote launch destination or a native
coding-conversation recovery mechanism.

## Native application control

An isolated macOS ARM64 workflow enrolls a synthetic Node terminal directly
through the published Rig CLI, outside WebMux's private launch registry. It
then discovers and opens that seat through the real application and Chrome.
The control verifies actual keyboard input, page reload, hidden-viewer reopening,
unchanged pane/native PID, exactly one terminal start, reused viewer identity,
empty persisted/recovery host and refusal to offer native resume. The native
Rig host identity is retained as a separate opaque field.

The same new control against production source `8f5d322` fails at the host
assertion: expected empty, received `localhost`. Only the qualification test was
added to that detached checkout; production source was unchanged, and the test
file was restored after collection. This negative control establishes the
original metadata error.

The corrected native control passes with Node24.21.0, tmux3.8 and published
`@openrig/cli@0.6.14` (`df1c1517`). No authenticated coding CLI or remote
execution is claimed. Daemons, tmux sockets, wrappers, homes and programs are
private to the test; installed services and user configuration are preserved.

## Regression checks and remaining failures

Affected HTTP/session Go race suites, Darwin and Windows AMD64 root vet, all374
frontend tests on Node24,64 helper tests, strict TypeScript/source inventory,
frontend build, lint and whitespace checks pass. The final native receipt and
source hashes are retained in `native-results.json`.

The original nine-case macOS run was **7 passed, 2 failed**; both older
failures reproduced on unchanged `8f5d322`. Investigation in
[#433](https://github.com/jordanhubbard/webmux/pull/433) established that tmux3.8
starts with global mouse on before Rig/WebMux runs. Its fixture repair sets
an explicit private baseline with mouse off, retaining all outcome assertions.
The apparent dashboard endpoint failure followed the failed worker's `afterAll`
stopping the shared daemon; with that fixture repaired all eight older cases
pass, including live dashboard reuse and detach.

Combining that repair with the new external-seat case passed the older eight,
then hit the production shared-address request budget at the ninth case. The
external-seat control now lives in `openrig-host.spec.ts`, giving it its own
server/home/socket and request budget through the existing per-file runner.
Hosted native Rig CI invokes both files separately with distinct evidence
folders. No rate limit, outcome assertion or timeout was changed. The original
before/after receipts remain checkpointed in `native-results.json`. The final
isolated-file run passes (1 test, 17.1 seconds), including actual input and
viewer reuse with the same native PID and exactly one start. Strict TypeScript
checks pass; runtime source hashes remain identical to `4b89fe6`.

The first hosted Linux job passed the older eight cases, then direct fixture
enrollment inherited CI's deliberately unrelated host selection and failed
looking for that remote host registry. Direct fixture commands now explicitly
select their own local daemon; the app server retains the hostile selection.
The final native run reproduces that environment and passes (1 test, 27.1s),
including controller routing, input and unchanged native identity. Strict
TypeScript checks and cleanup pass. The earlier result and its log hash remain
in the receipt, alongside the final test-source hash.

Keep the repair draft until its exact-head hosted gates pass. The independent
[#429](https://github.com/jordanhubbard/webmux/issues/429) dependency audit still
blocks merge and release. This scoped result does not close all of #194/#318.
