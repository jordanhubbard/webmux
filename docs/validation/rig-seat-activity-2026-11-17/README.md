# Rig seat activity and lifecycle observations — 2026-11-17

This evidence supports #439 under #194. It does not complete the roadmap or
qualify authenticated coding-provider readiness, automatic adoption, change
notifications, or conditional Stop.

## Product behavior

Rig tiles show logical lifecycle, bootstrap status and activity separately from
the viewer connection. Only a fresh runtime hook with resolved occupant provenance
can claim agent running, idle or needing input. Output motion is labelled terminal
output, and other sources are estimates. Unknown, stale and unresolved observations
remain unknown. Positive activity expires without requiring another server reply.

Canonical seat identity selects the logical lifecycle. Bootstrap/activity also
require matching saved manager and native attachment targets. Thus a stopped seat
can become detached after losing its attachment, while a rebind does not silently
attribute a successor's activity to an old viewer. An atomic expected-occupant
lifecycle precondition remains absent from the published manager contract.

The empty terminal workspace performs no coordinator inventory fetch. Grid reads
are authorized, bounded, non-overlapping and Rig-only; they exclude outbox bodies
and Agent Deck discovery. Observation commands pass `--no-cleanup` so discovery
does not remove stale daemon state. Both the runner's seat/legacy-agent projections and the
public API omit raw activity evidence, runtime event text and credentials.

## Native acceptance

The retained native browser receipt and screenshot below exercise the Go server,
Chrome, actual tmux 3.8 and published Rig 0.6.14 (df1c1517) on macOS ARM64 with Node
24.21.0. A hostile unrelated host selection stays inherited by the app. Direct
fixture commands explicitly select the isolated local coordinator.

Two native `builtin:terminal` seats start through Rig. The first opens at cell
(1,1), accepts terminal input, shows bootstrap/output separately from provider
readiness, and is then stopped using the owning manager's native CLI. The tile
updates to detached, the logical viewer/cell remain, and the sibling's PID does
not change. No coding runtime hook or authenticated coding provider is involved.

- [Native browser receipt](native-results.json)
- [Native tile screenshot](rig-tile-status.png)
- [Lifecycle contract probe](lifecycle-probe.json)
- [Qualification source manifest](source-manifest.json)

The separate lifecycle probe also enrolls one owned synthetic Node terminal as
adopted/claimed. Rig refuses `seat stop` with `claimed_session`; it and the native
sibling stay alive. Owned daemon state and tmux socket cleanup pass. Its source
hash identifies the retained qualification probe; the committed native browser
case independently covers managed single-seat stop and sibling survival.

The existing API's `managed` boolean in the receipt means a WebMux private launch
record. It is false for these externally created, natively Rig-launched seats.
That boolean is not native lifecycle ownership; #438 tracks the distinction.

## Reproduction and limits

The committed browser case is `tests/e2e/openrig-activity.spec.ts`; Linux CI runs
it in its own server lifecycle/request budget after the three existing Rig files.
Run it through `npm run test:e2e -- openrig-activity.spec.ts` with the isolated real
Rig fixture enabled. Never substitute the user's daemon or tmux socket. Native
runs use PR #433's qualified fixture overlay (global tmux mouse off) while that PR
awaits integration; the runtime source is unmodified, and the fixture is restored
in a finally block. The source manifest records both fixture hashes.

Deterministic tests cover hook input attention, source distinctions, unresolved
or absent generation provenance, invalid/future/stale timestamps, autonomous
expiry, duplicate IDs, rebinds, offline observations, failed and hanging discovery,
unmount cancellation, privacy projection and unrelated-manager exclusion. They do
not substitute for a live authenticated provider qualification.

A failed first native run exposed the need to keep logical detached state after
an attachment disappears; the corrected run passed. A privacy regression test
also found raw evidence in the legacy agent enrichment path; projection now occurs
before enrichment, and the final race suite covers it. Earlier failed logs remain
retained separately from final evidence.


## Local checks

- 389 frontend tests across 41 files passed; strict maintained-source TypeScript
  coverage, frontend lint/build and 64 helper tests passed.
- All eleven browser cases across the four Rig files passed. The native activity
  case was repeated on the final observation-only `--no-cleanup` command change.
- HTTP API and coordinator Go race suites passed on the final source. Darwin and
  Windows AMD64 root Go vet passed. Source hashes are in the manifest.
- The dependency audit still fails with twelve propagated high-severity entries
  for the single picomatch repeated-extglob advisory GHSA-fc5q-39xc-vjpf (#429).
  Dependency locks are unchanged. No audit suppression, downgrade or unreviewed
  upstream patch is included. This branch is not qualified for merge or release.
