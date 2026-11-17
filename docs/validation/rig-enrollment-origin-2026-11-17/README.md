# Rig enrollment origin — 2026-11-17

This evidence addresses the provenance distinction in #438 under #194. It does
not complete conditional Stop, adoption, lifecycle ownership or the release.

## Verified distinction

The public `managed` boolean was previously rendered as “Rig-managed”, but it
only represented a private WebMux launcher record. Native Rig starts can lack
that record, and WebMux launchers currently register adopted/claimed sessions.
The UI now names the record separately and labels manager-reported enrollment
origin as Launched by Rig, Adopted by Rig or unknown. The static badge is a Rig
viewer link, not an ownership claim.

The native receipt for builtin terminals has `managed: false` and
`enrollmentOrigin: launched`. The external synthetic Node terminal has
`managed: false` and `enrollmentOrigin: claimed`. The existing WebMux launcher
browser workflow also asserts `managed: true` plus `enrollmentOrigin: claimed`.
Thus the behavior is not inferred from a WebMux record or from a tmux process.

- [Native Rig launch, lifecycle and stable-viewer receipt](native-launched.json)
- [External adopted terminal and viewer recovery receipt](native-adopted.json)
- [Native tile screenshot](rig-tile-status.png)
- [Source and artifact manifest](source-manifest.json)

All real-manager cases use a private daemon and tmux server, published Rig 0.6.14
(df1c1517), tmux 3.8, Node 24.21.0 and Chrome 157 on macOS ARM64. The app inherits a
hostile unrelated host selection; direct fixture commands select their owned
local coordinator. No authenticated coding-provider or runtime-hook readiness
qualification is claimed. Other UI cases in the four-file browser regression use
mocks, as documented by their fixtures.

## Contract and limits

The helper reads only GET node/session inventory routes on loopback, using the
port returned by the configured CLI's daemon status. It projects just enrollment
origin, source, ID and observation time. It validates rig/node/logical/native
identity and selects the greatest session ULID, matching this qualified build's
inventory implementation. Historical rows cannot override a successor, and a
reused name or contradictory binding cannot establish lineage.

The projection is qualified only for df1c1517; another build reports unknown.
This does not broaden the separate native recovery version qualification.
Redirects are refused; response, sweep, row, seat, rig and deadline limits are
explicit in the integration documentation. Failures degrade to unknown without
copying raw event bodies, resume tokens or prompt evidence. Manager-reported
origin is not an atomic lifecycle authorization or proof of remote ownership.

Display discovery requests enrollment separately from basic discovery. A guard
regression test verifies that the basic attach/recovery inventory does not even
run the enrollment version/daemon probe. A stalled or unsupported metadata source
therefore adds no enrollment dependency to those paths.

## Reproduction

The committed native assertions are in `tests/e2e/openrig.spec.ts`,
`openrig-host.spec.ts` and `openrig-activity.spec.ts`. Run each browser file through
`npm run test:e2e -- <file>` in its own real Rig fixture lifecycle/request budget.
Do not use the user's daemon or default tmux socket. Local qualification used
PR #433's qualified mouse-off fixture overlay, restored in a finally block before
commit. Runtime source was not overlaid. The manifest records both fixture hashes.

Deterministic helper tests cover claimed/launched origin, private-field omission,
latest-enrollment selection, stale history, malformed and duplicate identities,
status/binding mismatch, unavailable versions/ports, oversized/bad responses,
per-rig failure isolation and deadline cancellation. Go tests validate the public
projection and basic-discovery guard; frontend tests keep launch records distinct
from enrollment and unavailable origin unknown.

#438 remains open: native manager-owned launch/adopt, conditional reviewed-
occupant Stop, explicit remote lifecycle effects and their live acceptance are
still required. This change adds no Stop action and changes no dependency lock.


## Local checks

390 frontend tests across 41 files, 85 helper tests (including 21 enrollment
contract cases), strict maintained-source TypeScript checks, frontend lint/build,
HTTP API race tests (55.314s), coordinator race tests (2.746s), Darwin and Windows
AMD64 root Go vet passed. Eleven browser cases across the four Rig files passed
on the final source. Each browser file had its own fixture/server lifecycle.
Source/artifact hashes, documentation links, lock identity and whitespace checks
were verified before commit. The qualified fixture overlay was restored, owned
daemon state and private tmux sockets were removed, and retained evidence remains
in the worktree and qualification outputs.

The dependency locks are unchanged. The known picomatch advisory in #429 still
blocks required audit gates; this evidence is not merge or release qualification.
