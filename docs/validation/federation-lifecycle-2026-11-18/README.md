# Federation lifecycle visibility and diagnostics

The qualification follow-up [#478 is closed](https://github.com/jordanhubbard/webmux/issues/478#issuecomment-7024703723)
after the integrated acceptance audit below. The historical sections retain the
original failures and pending dispositions as recorded at those checkpoints.
The established correction is the independently reproduced offscreen DOM
measurement hazard; the geometry-free historical sample is not uniquely
attributed to it.

Issue #478 retains a native Windows failure in which the initial remote
provider marker was visible, but a later reconnect/refresh expected that same
marker and received a blank terminal. The original catch block replaced the
assertion stack. Standard failure context was captured after cleanup had
stopped the peer and removed local sessions, so it cannot establish provider
survival or transport state at failure.

This candidate preserves the original assertion and adds optional test-only
observations under `WEBMUX_FEDERATION_DIAGNOSTICS=1`. It records the current
lifecycle phase, bounded socket event order and frame metadata, rendered fixture
markers, read-only session state, and fixture process-start instances. Capture
runs before cleanup. The first 64 and last 256 events, at most 64 sockets and 32
process starts are retained. Raw frames, arbitrary error text, URL origins/query
strings, credentials and native account data are excluded. Two privacy controls
exercise protected-field projection and malformed/oversized inputs.

An intentional assertion before grant revocation validates the diagnostic
mechanism. On native macOS, its receipt shows both views connected and both
fixture providers present before cleanup. The original assertion location is
retained, and both providers are gone after normal fixture cleanup. The complete
unfaulted case also passes (8.7 seconds; 11.2 seconds including startup), with
one local provider start and two peer starts including its explicit restart.
Its actual generated service token is absent from the receipt. Source and
receipt hashes are in `results.json`.

The initial diagnostics snapshot passed all 123 helper controls, five strict
TypeScript projects, the frontend build, workflow YAML parsing and whitespace
checks. Instrumentation can affect timing; green samples alone do not establish
a cause or fix. The subsequent visibility correction is described below.

The separate Windows workflow checks the intentional failure and then runs
one or ten independent fresh samples. Every outcome is retained and any
unexpected failure or missing receipt fails the job. Existing federation
assertions, the five-second marker checks, the 90-second case deadline,
protected Chromium preparation and standard CI gates remain in place.
Native Windows diagnostic outcomes are recorded below. Profile cleanup #473
and dependency audit #429 remain separate blockers.

The first ten-sample Windows run (44273974857, initial source `28f32b5`)
retained nine passing samples and one failure during Pack. Before cleanup,
the failed sample had connected viewers/APIs and both original fixture
providers present; the peer received 49 replay bytes and then no additional
output for the five-second assertion window. This narrows that occurrence
without establishing its cause.

The follow-up records terminal clipping geometry. If an assertion fails with a
blank peer view, it then scrolls that existing viewer into view and captures the
result, counting any new frames/connections and preserving the original failed
outcome. This is an explicit post-failure diagnostic intervention, not a retry,
reconnect or replay. Installed xterm source pauses offscreen painting; a separate
macOS library control has a populated native xterm buffer and blank offscreen
DOM, then displays the marker after scrolling with no additional terminal
writes. The historical failing sample has no geometry; the controlled Windows
observation below establishes this measurement hazard independently.

The geometry follow-up at `3cceb89` completed all ten Windows samples in run
44276326962. Each retained receipt has `completed: true`; its separate expected
failure control was retained before cleanup. No spontaneous blank failure was
captured, so this cohort cannot settle the visibility hypothesis. The earlier
nine-pass/one-failure cohort remains evidence; no runtime repair is claimed.

A deterministic application control now delays only delivery of real native
WebSocket output callbacks until an IntersectionObserver has observed the
existing peer viewer offscreen. It forwards every original message unchanged.
On macOS the ordinary five-second DOM assertion then fails with both native
providers alive and both viewers connected. Scrolling exposes the original
provider marker with a complete trace and zero new received frames, output
frames or sockets. Event recording includes the read-only diagnostic queries
before the scroll. A normal native lifecycle also passes (14.1 seconds).
The same schedule passed on native Windows AMD64 in [run 44280531734](https://github.com/jordanhubbard/webmux/actions/runs/44280531734),
source `fea9cb8`: the actual five-second assertion failed on a blank view; both
original providers and both viewers stayed connected. Scrolling revealed the
original marker with a complete trace, zero new received/output frames and zero
new sockets. The ordinary full lifecycle also passed. Artifact 14417565130
and its reported digest are recorded in `results.json`. This establishes a
visibility hazard in the DOM oracle, without uniquely attributing all historical
failures.

The corrected lifecycle test exposes the stable terminal card before reading
xterm rows, including negative isolation checks. Refresh replaces the inner
xterm screen, so scrolling that disposable inner element races the refresh;
scrolling its stable card avoids that race. All original expected markers,
provider identity, session count, input, resize and failure checks remain.
Marker assertions retain five seconds and the case retains 90 seconds, with no
added sleep or replay/reconnect request. The full native macOS case passes
(13.4 seconds); the deliberate unscrolled control still fails its original
blank assertion and recovers the old marker solely on visibility. The control
receipt is now written only for that exact matcher failure with blank actual
text. Native Windows qualification of the corrected lifecycle is pending.

## Integrated acceptance — 2026-11-19

The [acceptance receipt](acceptance-2026-11-19.json) audits main
`629478cdd5984a21056bd3526490970361ec9a91` against five original native packets.
Original immutable ZIPs, live source/run bindings and relevant current source
equality were independently checked. No new hosted or native Windows jobs ran.

The spontaneous Pack failure remains failed. Its raw receipt shows both
connected APIs/views, the two original fixture providers present and 49 peer
replay bytes delivered, followed by 5,029 ms without further peer receive,
close or error frames while rendered rows were blank. Its missing geometry is
not reconstructed or inferred from later passing runs.

The native offscreen intervention proves the measurement hazard independently:
the original five-second blank-text assertion fails, then visibility alone
reveals the same PID marker with a complete trace and zero new frames or sockets.
The corrected `2f9a13d` Windows run `44281407842` passes ten complete ordinary
lifecycle samples while both deliberate negative controls remain failures
admitted by their explicit verifier. Later integrated `0056000` native run
`44541541564` also passes both controls and the full ordinary workflow; its full
CI passes. These exercise real Windows Go/Chrome with synthetic exec fixture
children, separate from native ConPTY renderer and authenticated coding evidence.

An AST comparison preserves all 75 original issue-source assertions in order.
All 81 pre-fix matchers and arguments are unchanged after normalizing only the
visibility lookup and one equivalent descendant selector. The 90-second case
limit, five-second matcher default, explicit timeouts and protected Chromium
settings remain. No new native input, replay or reconnect is part of the
visibility correction. The native test and its diagnostics/barrier/fixture,
workflow, proxy and terminal/workspace sources match the qualified composition.

This completes the qualification follow-up for the established invalid DOM
oracle after correction and full native lifecycle qualification. It does not
claim a unique cause for every historical blank occurrence or a production
transport repair. Native executions retain their original source attribution;
no fresh whole-current-main, authenticated recovery or full-roadmap/release
acceptance is claimed. The original ordinary failures remain failed, and #484's
capability propagation and #473's private-file cleanup remain separate evidence.
