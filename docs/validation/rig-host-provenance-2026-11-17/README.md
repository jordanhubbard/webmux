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

The complete nine-case macOS Rig/browser run is **7 passed, 2 failed**. Both
older failures also reproduce on unchanged `8f5d322` with the same published
CLI, Node24 and tmux3.8: global mouse mode becomes on instead of remaining off,
and the diagnostic TUI selects its stopped-daemon page at port7433 instead of
the fixture's live endpoint. A first, longer baseline path additionally exceeded
the native Unix-socket limit; the shorter registered baseline reproduced the
two original failures. These are tracked in
[#431](https://github.com/jordanhubbard/webmux/issues/431); no assertions or
production deadlines were weakened.

Keep the repair draft until its exact-head hosted gates pass. The independent
[#429](https://github.com/jordanhubbard/webmux/issues/429) dependency audit still
blocks merge and release. This scoped result does not close all of #194/#318.
