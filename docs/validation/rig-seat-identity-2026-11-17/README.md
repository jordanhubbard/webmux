# Canonical Rig seat identity — 2026-11-17

This addresses #436 under #194 and builds on draft #432. Local inventory IDs
and viewer keys now derive from canonical Rig ID + logical node ID rather than
a mutable attachment address. Manager and tmux targets remain separate metadata.
Fresh inventory resolves Open/reattachment; missing/ambiguous identity or a
native address shared by distinct seats is refused. Owner/provider boundaries
and authenticated remote-host namespaces remain separate.

Explicit Open after a provider-owned rebind reuses the viewer, history and
layout while detaching only its previous viewer client. Hidden viewers retain
their saved cell when free, choose another when occupied, and refuse a full
grid without detaching the running viewer. Legacy exact-address viewers can
receive the canonical key on explicit Open. Cached native process/conversation
references are cleared when their attachment destination changes. Startup and
transport reconnect refuse a changed binding; only explicit Open follows it.

## Native before and after

The same final browser control and unmodified fixture run against parent
`279db1e` and this candidate using published Rig0.6.14 (`df1c1517`), tmux3.8,
Node24.21.0 and Chrome157 on macOS ARM64. The parent fails exactly at the stable
seat assertion: first tmux address expected, second address received. Only the
qualification test was added to that parent checkout and removed afterward;
its tracked tree is clean. The candidate passes (20.3 seconds, no skips).

The external owner enrolls a synthetic Node terminal, then explicitly unclaims
its enrolled session and attaches a second terminal to the same logical node.
Canonical Rig/logical/node IDs remain unchanged while the enrollment ID and
native address change. WebMux sends no provider launch or recovery request.
The browser proves the same opaque seat ID and viewer UUID, saved cell1,1 after
hide/reopen, old provider still alive but detached, one start per externally
created terminal, input delivered only to the current provider, retained old
history, refused transport reconnect across the changed binding, and reload.
Owned provider/daemon/tmux cleanup was verified after the run.

## Regression scope

All ten browser cases pass in three isolated invocations: the existing8-case
Rig/browser file, external execution-host case, and canonical rebind case. These
regression runs temporarily use #433's qualified tmux mouse fixture; it changes
no application source and was restored afterward. The paired before/after
identity control above uses the same original fixture on both sides. Native
provider claims apply to managed enrollment/messaging, diagnostic TUI, external
host provenance and explicit rebind; other inventory UI cases retain their
mock fixtures.

Full HTTP/session Go race suites pass (55.234s/14.779s), as do374 frontend tests,
64 helper tests, strict TypeScript/source coverage, lint/build, workflow YAML
and whitespace checks. [native-results.json](native-results.json) records native
receipts, source hashes, exact negative control and private-log hashes.

This does not qualify authenticated coding conversations, automatic provider
adoption or general lifecycle change notifications. #194/#318 remain open.
Exact-head hosted gates remain required, and the independent #429 dependency
audit still blocks merge and release. Merge dependencies #432/#433 first;
retain the independent Agents-pane coexistence invocation from #435 when
resolving CI workflow context.
