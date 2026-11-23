# Coordination availability during terminal startup

Implementation evidence for [#669](https://github.com/jordanhubbard/webmux/issues/669).
Native Windows acceptance remains a separate required gate.

The Windows job for #667 passed the coordination package after the fixture-clock
repair, then failed `TestCoordinationPeerClaimImmediateConflictAndDurableNotices`.
Its independently provisioned child exited after the existing five-second scoped
client timeout with `local coordination service is unavailable`. The retained
[failed native job](https://github.com/jordanhubbard/webmux/actions/runs/45047188611/job/137284246790)
is not a successful Windows result.

## Demonstrated cause and change

The broker held its global mutex across synchronous terminal/provider startup.
A coordination request needs that mutex to obtain and authorize a current session
snapshot. A blocked provider could therefore prevent an unrelated live session
from reading coordination state. A deterministic controlled launcher reproduces
that contention before the fix; it does not require slow hardware or change the
production client deadline. This establishes a real blocking path consistent
with the Windows failure; the original native test on the delivered revision
must establish whether it resolves that observed failure.

Provider startup now reserves a generation and runs outside the global mutex.
The result is published only if the broker and original entry/generation remain
current. A late result after deletion or shutdown is closed without resurrecting
the session. Shutdown tracks pending launches; duplicate pending launches are
refused. A resize during startup is applied before publishing the current run.
Managed agent access is rechecked after startup. Concurrent dashboard opens
retain their existing one-attachment serialization.

A pending process does not acquire live coordination authority. Its own child
request waits for that provider's publication under the original request context;
other pending providers do not block it. Capability identity and revocation are checked before that wait; a cached
inactive bit does not replace the current broker state. Full active authority
and incarnation are still checked when admission takes the broker snapshot. Deleted-session capabilities preserve
the existing 401 refusal. No request/fixture deadline was extended, fixture
assertion weakened, provider permission broadened or installed service restarted.

## Controls and qualification limits

The controlled regression verifies that an existing live session can obtain a
snapshot while another provider is blocked, and that the pending launch is not
reported live. Additional controls cover cancellation of the caller's startup
wait, duplicate reconnect refusal, deletion during startup, shutdown waiting for
the late result's cleanup, pending geometry reaching the published provider, and a live broker run
recovering from a stale inactive membership while direct inactive-ledger
authority remains refused.
The complete session race suite includes managed attachment sharing and existing
lifecycle/recovery tests. Targeted API race controls retain the original
peer-claim workflow, deleted-capability refusal and restored-state boundaries.

The first API run caught an incorrect 503 for a deleted capability; the first
session race run caught concurrent dashboard attachment conflicts. Both failures
are retained locally. Their corrections preserve the original assertions.
The final [receipt](receipt.json) records local checks and source hashes.

These component and macOS results do not constitute native Windows acceptance,
full automatic managed-agent onboarding, MAC convergence, three-host project
coordination or #318 recovery/release completion. Required hosted CI/review and
the original native Windows path remain delivery gates. #665's fixture-clock
repair is included in the branch; its original failure and distinct #669 failure
retain separate evidence and issue histories.

Final source-checked local qualification passes: complete Go suite with Node 24,
session and coordination race suites, targeted API race controls, vet and the
production API/session contract. The monitored server sources and contract script
remain unchanged across that run. Hosted checks remain pending.
