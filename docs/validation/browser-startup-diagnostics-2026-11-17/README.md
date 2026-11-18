# Browser startup and navigation diagnostics — 2026-11-17

This is investigation evidence for #282, not a runtime fix or release qualification.

## What the Windows failure proves

PR #440 at `14827c7a580695d713cc37bc1a8b810c404b5147` failed its final temporary-
profile navigation in [Windows job 134396447356](https://github.com/jordanhubbard/webmux/actions/runs/44184265934/job/134396447356).
The `browser-parity-windows` artifact 14375185972 retained 72 trace events with
zero dropped events. The initial and replacement saved-profile phases passed.

The temporary worker requested one navigation. Its matching navigation/document
request began at 78/81 ms, then Chromium canceled that document with ERR_ABORTED
at 110 ms. The acknowledgment also reported aborted. No document reached the
loopback fixture during that phase; the same target stayed on about:blank through
the existing five-second assertion. No second requested navigation appears in
that trace. See the [bounded failure summary](windows-abort-summary.json).

This establishes an aborted Chromium/CDP document load, rather than screenshot
lag or the separate historical cookie-setter sequencing bug. It does not establish
what caused Chromium to abort. The current navigation API deliberately accepts
asynchronous ERR_ABORTED outcomes; this change adds no replay, recovery shortcut,
longer deadline or relaxed assertion. #282 remains open.

## Observation added

The opt-in observer now starts after the target/viewport initialize, before the
worker's explicit startup navigation to about:blank. Sequence zero records blank
startup and worker readiness. Later user requests keep their existing sequence
numbers. The same observer passes into the companion and registers once per
target; it also observes new tabs. Renderer navigation reason/disposition,
document initiator category, redirect presence, loading start/stop and frame
detachment supplement the existing target/frame/loader/request IDs.

Stage and sequence describe the state when an event arrives, not proof that the
most recent user request caused every event. URL equality is represented by the
existing hash comparison; raw URLs, referrers, stack traces, headers, cookies and
page bodies are omitted. Protocol enums use fixed allowlists and unknown strings
become other. The trace and observer history are capped at 128 entries/targets.
`WEBMUX_BROWSER_NAVIGATION_DIAGNOSTICS=1` remains required. There are no new CDP
commands, navigation waits or default browser logs.

## Native and deterministic acceptance

Five unchanged macOS baseline runs passed, so this work did not reproduce the
Windows failure locally. Three initial instrumented runs passed. The final native
case adds a controlled script redirect: it captures renderer reason
scriptInitiated and a script-originated document request, both distinct from the
user's requested URL, and verifies one observer and startup sequence zero across
all worker replacements. Cookie persistence and exclusive controller checks stay
in place. The final default-off control verifies no navigation diagnostics on the
wire. See the [native summary](native-summary.json).

The exact final test fails against the unchanged parent at the missing startup
marker, and passes against this implementation. The parent runtime was not
overlaid, and its test overlay was restored. An earlier negative harness attempt
lacked built frontend assets and is excluded; a separately retained qualified
run verifies the intended assertion failure.

Go tests cover startup stages, safe enum projection, correlation, nil diagnostics,
privacy and the existing event cap. Browser/HTTP race suites, strict TypeScript
source checks, frontend lint/build, Darwin and Windows AMD64 vet pass locally.
The [source manifest](source-manifest.json) binds code, test and retained receipts.

Actual Windows execution of these new observations remains a hosted-CI gate.
A passing run does not prove the intermittent abort fixed. The dependency audit
in #429 and the broader roadmap/release gates also remain required.


## Windows follow-up and component isolation

PR #442 head 618b36e reproduced the abort in the initial saved worker, before its
first /set document, in [Windows job 134419891067](https://github.com/jordanhubbard/webmux/actions/runs/44190954805/job/134419891067).
The [new bounded summary](windows-startup-abort-summary.json) has 27 outer trace
events and zero dropped events. Explicit blank startup completes at 2154 ms,
worker readiness at 2156 ms, and the user navigation starts at 2690 ms. Its
matching document request is canceled with ERR_ABORTED at 2976 ms. No fixture
document arrives, no intervening renderer navigation is recorded, and the target
remains about:blank. Thus this occurrence is not an overlap with the explicit
blank-page navigation, nor is it limited to temporary-profile replacement.
Other browser initialization or policy causes remain unproven.

The native component probe adds four separate first loads against one private
Node HTTP fixture: direct CDP, direct CDP with an initial screenshot, and the
production WorkerCommand/Worker subprocess with temporary or saved profiles.
They share the production allocator options, including normal sandbox and
safe-browsing behavior. Each trial creates a fresh private browser profile and
issues one fixture navigation. Original execution roots remain intact; no
credentials are copied. Version metadata is queried only after a direct trial.
The test executable supplies the private worker entry point; this does not
qualify an installed service image, user authentication, or reboot recovery.

[Local component records](native-component-summary.json) show all four loads
passing on macOS ARM64 with one fixture request apiece and no cleanup errors.
These ordered single samples do not establish statistical equivalence or a
Windows fix. Initial unqualified harness attempts changed HOME or had setup/build
errors; they are retained privately and excluded from acceptance. The qualified
harness preserves the original roots and uses an actual worker subprocess.

Windows CI runs this opt-in probe after the application workflows, including
when those workflows fail, provided Chromium installation succeeded and the job
was not canceled. Its bounded records join browser-parity-windows. Failures still
fail the job; there is no continue-on-error, navigation replay, protection
bypass or longer application navigation deadline. The application workflow is
left ahead of the controls so they do not warm its first browser beforehand.

Browser/HTTP race suites passed after the allocator-options extraction; strict
TypeScript, Darwin/Windows vet and workflow YAML checks passed. The full native
application profile/renderer workflow also passes locally. The new component
records and source bindings are in [component-manifest.json](component-manifest.json).
Windows component outcomes and the cause of the abort remain pending.


## Windows component outcomes and matched profile locations

At d3ab2aa, the [Windows component records](windows-component-summary.json)
show both direct CDP cases failing with zero fixture hits, canceled/aborted
requested documents and no dropped events. The plain direct load is aborted at
488 ms after blank readiness at 367 ms. The initial-screenshot case is aborted
at 709 ms after blank readiness at 436 ms. Both production worker subprocess
cases pass with one fixture request, HTTP 200 and a document commit. The
application workflows also pass on that exact head; the Windows component gate
remains failed. Packaging succeeds and audit remains red.

The same symptom therefore occurs without the WebMux browser controller or
companion. These ordered single samples do not identify a cause or prove a
worker fix. The direct cases use system-temporary profile directories, while
the production temporary/saved workers use the original account home. Their
initialization and process ancestry differ too.

Two new direct controls use exactly the production temporaryProfile() helper in
the original home, with and without the initial screenshot. The direct CDP
startup and navigation path otherwise stays the same. The four preceding modes
remain, and bounded records now label profile location through fixed categories;
no directory paths or profile keys are added. All six cases create independent
fresh profiles and make one navigation each. Private home profiles are removed
after browser shutdown and cleanup errors fail the case. The six [local
records](native-profile-location-summary.json) pass with one fixture request
apiece; actual Windows outcomes for these matched controls remain pending.

Only the optional qualification test changes in this follow-up. Browser runtime,
protection flags, original execution roots and application waits are unchanged.
The [profile-location manifest](profile-location-manifest.json) binds this source,
the retained preceding Windows artifact and the new local receipts. Earlier
manifests continue to describe their original sources.


## Profile-factor outcome and transport/startup control

At 540d7d2, the [matched Windows records](windows-profile-location-summary.json)
show both plain direct cases aborting: system-temp at 518 ms, home at 511 ms,
with zero fixture hits. Both screenshot cases and both worker subprocess cases
pass. The application initial saved-worker document also aborts at 689 ms after
blank readiness at 399 ms and a user navigation at 576 ms. Location alone is
not a sufficient explanation. Screenshot cases pass here but a screenshot case
failed at d3ab2aa, so no screenshot/readiness workaround is established.

The installed chromedp 0.23.0 exec allocator uses pipes by default. Its optional
remote.WebSocket allocator uses a debugging-port startup path instead. Two more
fresh direct controls compare that path at system-temp and home locations,
without adding the initial screenshot. Each retains the same protected allocator
flags and one fixture navigation. The experimental listener is explicitly
127.0.0.1 on an ephemeral port, and only fresh anonymous test profiles use it.
No port, WebSocket URL, credentials or extra raw protocol logs enter the records.
The production worker continues to use private pipes.

All eight [native local cases](native-transport-summary.json) pass. This is a
transport/startup-path comparison, not a pure timing-independent wire comparison:
the WebSocket allocator waits for Chrome's advertised debugging endpoint. A
passing Windows WebSocket case would not itself justify opening a listener in
the production authentication browser or identify the cause of pipe-path aborts.
Actual Windows results remain pending. The [transport manifest](transport-manifest.json)
binds the optional test, compact safe summaries and full private receipts.


## Transport outcome and matched target selection

At 351f653, [Windows job 134469638981](https://github.com/jordanhubbard/webmux/actions/runs/44205320390/job/134469638981)
retains all eight [transport controls](windows-transport-summary.json). All four
direct pipe cases fail with zero fixture requests and ERR_ABORTED, regardless
of profile location or initial screenshot. Both direct WebSocket controls and
both production worker subprocess controls pass with one fixture request each.
The application browser workflows pass in this occurrence. All controls issue
one navigation and report zero dropped events. This does not establish a pipe
defect: the successful workers also use pipes, and WebSocket allocation changes
the startup handshake. Screenshot priming is not a demonstrated fix.

The installed chromedp 0.23.0 implementation attaches the first context to a
browser-created page. A subsequent child context creates a new target/window in
the same browser context. Four additional optional pipe controls test target
selection at both profile locations. Each matched pair creates and initializes
one child target with the same viewport and blank navigation. One navigates the
child; the other navigates the original page after that identical setup. They
assert distinct target IDs, the same Browser and BrowserContextID, and record
only fixed selection categories and an extra-target count. Both browser pages
remain owned until test cleanup. No additional external fixture navigation,
retry, arbitrary delay or production behavior change is introduced.

All twelve [local controls](native-target-summary.json) pass with exactly one
fixture request each and no cleanup failures. The four new controls use private
pipes. Actual Windows outcomes remain pending; target ownership is a hypothesis,
not an established cause or production fix. The [target manifest](target-manifest.json)
binds these sources, the preceding Windows evidence and the local receipts.


## Target-control outcome and Windows network diagnostics

At 55f86e8, [Windows job 134497265523](https://github.com/jordanhubbard/webmux/actions/runs/44213446168/job/134497265523)
records all twelve [target-selection outcomes](windows-target-summary.json).
The system-temp child target passes, but the home child and both original-after-
child cases abort with zero fixture hits. Both direct WebSocket cases also abort
in this occurrence, despite passing at 351f653. Both initial-screenshot cases and
both worker subprocess cases pass. All twelve issue one navigation and report
zero dropped events. The application browser workflow also fails in this run.
Neither target selection nor the WebSocket path is a demonstrated reliable fix.

Two new anonymous first-load controls add bounded Chromium NetLog at each profile
location, keeping private pipes and ordinary sandbox/protection flags. They use
Default capture mode, a four-MiB producer limit and an eight-MiB input limit.
After the trial, graceful browser shutdown precedes projection and profile
removal. Only requests with the exact private fixture URL and their dependency
sources are selected. The projection retains numeric event/source types, fixed
allowlisted names, relative times and bounded numeric net errors. Native source
IDs become local aliases. It omits URLs, paths, hostnames, addresses, headers,
command lines, raw parameter dictionaries and unknown event names. Limits are
64 sources, 256 projected events and 32768 scanned events; truncation is explicit.
Raw files stay inside the disposable anonymous profile and are never uploaded.
An absent matching request is inconclusive: asynchronous logger initialization
can miss early activity. Logging may also change startup timing.

The [Chromium logging documentation](https://www.chromium.org/for-testers/providing-network-details/)
describes the startup flags and bounded capture. Local macOS trials hit the
network service's scratch-directory refusal with the normal sandbox. Their
empty-log failures are retained privately and excluded from acceptance. The two
capture controls therefore target Windows explicitly and are skipped on other
platforms. The twelve existing native macOS controls pass; those two skips are
not claimed as successful native capture. Unit controls independently cover
projection privacy, dependency selection, input/source/event bounds, corrupt
input and invalid timestamps. Actual Windows capture remains a hosted gate.

A separate `Browser startup diagnostics` workflow runs the same controls on
Windows without waiting for frontend/application suites. Its unique check name
is `Windows first-navigation diagnostics`; it does not reuse or replace any CI,
audit or packaging check. Path-scoped pull-request/main triggers and manual
dispatch support the isolated diagnostic job. All original CI steps, including
the later application-adjacent component probe, remain unchanged. Failures still
fail the job. Only bounded projected JSON records are uploaded. The new workflow
itself also awaits actual hosted acceptance.

See [network-diagnostics-manifest.json](network-diagnostics-manifest.json) for
source and receipt scopes. This remains investigation work under #282; no
production listener, sleep, navigation replay, protection bypass or runtime fix
is introduced.


## Internal navigation trace and a known-cancellation control

At923d404, the isolated Windows workflow completes in3m16s and retains all
fourteen [first-load outcomes](windows-network-summary.json): seven pass and
seven fail. The saved-worker subprocess also aborts in this occurrence. All
trials issue one navigation, with zero dropped CDP observations or cleanup
failures. Both bounded NetLog projections are valid but contain zero matching
fixture requests. Logger startup can lag, and no positive native matching-request
NetLog control is established; absence of a record does not locate the abort.

Two additional direct controls now collect Chromium's `navigation` tracing
category at system-temp and home profile locations. Tracing starts after the
browser connection opens, before initial target attachment and explicit blank
setup, and ends after the trial. The producer buffer is oneMiB, record-until-full;
raw in-memory collection is limited to twoMiB/8192events. Projection retains up
to256fixed-name events,64locally aliased navigation identities, relative times,
URL-equality booleans, bounded numeric state/error/throttle codes and fixed
cancellation-reason enums. URLs, dynamic event names containing URLs, raw
argument dictionaries, browser/process/frame metadata, page data and raw traces
are not uploaded. Unknown names are omitted; incomplete/truncated captures fail.
Raw buckets are copied under a lock and projected after tracingComplete, keeping
protocol callbacks free of commands and trace parsing.

A separate positive capture control starts one request to a private hanging
fixture. Only after the fixture confirms receipt does the test send Stop Loading.
It requires the requested navigation to appear in the completed trace with
net_error-3, unchanged one-request count and an explicit stop-issued flag. This
intentional cancellation is separate from first-load success; it neither retries
a failed load nor changes production navigation behavior. Both CI probe selectors
include this control, so an unestablished Windows control fails its job.

[Final local evidence](native-trace-summary.json) passes fourteen ordinary native
first loads plus that cancellation control. The two Windows-only NetLog cases
remain explicit skips. The reference capture includes RenderFrameHostImpl::Stop
and the requested, uncommitted navigation with net_error-3, with no trace loss or
dropped events. Privacy/enum/correlation/bounds unit controls, browser race and
Darwin/Windows AMD64 browser vet pass. Final source bytes are bound before/after
native execution. Exploratory schema probes and an intermediate test compile
error are retained privately and excluded from acceptance.

The [DevTools Tracing contract](https://chromedevtools.github.io/devtools-protocol/tot/Tracing/)
and actual local Chromium158 trace schema informed the projection. Numeric states
and error codes are observations, not a cancellation-initiator verdict. Tracing
itself changes startup timing; Windows execution and an actual unexplained-abort
trace remain pending. See [trace-manifest.json](trace-manifest.json).
No runtime workaround, protection change, extra application wait or replay is
introduced. The original CI delta in this follow-up only adds the positive
control to its existing native-probe selection; other CI/audit/package gates
remain intact.


## Windows trace acceptance and bounded independent samples

At2e309f4, the [isolated Windows evidence](windows-first-trace-summary.json)
confirms the positive cancellation control: its hanging fixture receives one
request, Stop Loading is issued, and the complete trace retains the requested
uncommitted net_error-3 navigation and RenderFrameHostImpl::Stop. Both ordinary
trace cases pass with committed requested documents. Twelve of the sixteen
ordinary first loads pass; four fail, so there is still no unexplained-abort
internal trace or runtime fix. One NetLog case also passes with three matching
URL-bearing source objects and exactly one fixture request. The diagnostic
counter is now named matchedURLSources; the old matchedRequests label did not
count HTTP requests. The original artifact values are retained with that
clarification. Known-control and ordinary traces report no capture loss/drops.

The diagnostic workflow now runs three additional independent rounds of just
the two trace cases and their known-cancellation control. Every case owns a
fresh browser and each round has its own artifact directory. This is bounded
sampling of an intermittent failure, with one fixture navigation per browser.
The full first-load matrix still runs first and its failure remains a job
failure. Sampling proceeds after a matrix failure only when browser installation
succeeded and the job was not canceled. Any sample failure also fails its step.
No continue-on-error, failed-load replay or replacement of another gate is added.
Only projected JSON below the owned artifact root is collected recursively.

The trace projection also adds locally aliased frame-tree-node identities,
main-frame booleans and fixed URL classes (fixture, about_blank, data,
browser_internal, other). These distinguish other startup navigation activity
from the requested frame without retaining a URL or raw frame/process dictionary.
Both navigation and frame aliases are capped at64. Chromium can encode optional
frame metadata as an object or an opaque pointer string; only object snapshots
supply aliases, and pointer strings remain unprojected/unknown. Unit controls
cover both forms, privacy and both identity limits. The first native correlation
attempt rejected the pointer variant; that failed attempt is excluded, and the
corrected final source is requalified.

[Final local sampling](native-sampling-summary.json) passes the full fourteen
ordinary first loads and separate control, then six more ordinary first loads
and three separate cancellation controls across the three sampling rounds.
The two Windows-only NetLog cases remain explicit skips. Browser race,
Darwin/Windows browser vet, workflow policy, source bindings, JSON, links and
whitespace pass. Actual Windows sampling and frame correlation remain pending;
tracing can perturb startup, so successful instrumented loads alone cannot
establish a fix. See [sampling-manifest.json](sampling-manifest.json).


## Paired observer placement

At8248058, [Windows sampling](windows-sampling-summary.json) retains all26
records: the full matrix has six passes and ten failures, all eight early-traced
first loads pass, and all four intentional cancellation controls pass. The
sampling step passes while the failed full matrix keeps the job failed. Traces
are complete without loss/drops. Frame aliases place observed browser-internal
startup commits on a separate frame from the fixture. This does not establish
what canceled the uninstrumented loads; early observation can perturb startup.

Two more first-load controls now start the same navigation tracer after target
initialization, viewport setup and the explicit blank navigation, immediately
before the one fixture navigation. Their matched early controls still start
before target attachment. Both positions use the same protected allocator,
profile locations, trace collector and caps. A fixed traceStartStage category
labels before_target or after_blank. No sleep, navigation retry, production
readiness barrier or application timeout change is added. The three bounded
sampling rounds include both positions at both locations plus the separate
known-cancellation control. The full matrix remains first and retains failures.

One intermediate local sample loaded the fixture but received no trace data
within the existing five-second drain deadline. It is retained as an incomplete
capture, not navigation failure or accepted trace evidence. The record now also
labels whether Tracing.end was acknowledged and projects only a fixed error
class. The diagnostic drain deadline and all navigation deadlines remain
unchanged. Final accepted captures require completion without loss or truncation.

[Final local source](native-position-summary.json) passes sixteen ordinary first
loads plus one separate intentional cancellation, then twelve ordinary first
loads and three separate cancellations across the three paired rounds. Two
Windows-only NetLog skips remain explicit. Browser race, privacy/bounds controls,
Darwin/Windows browser vet, workflow checks and source bindings pass. Actual
Windows late-start results remain pending; neither local success nor a missing
abort under instrumentation demonstrates a runtime fix. See
[position-manifest.json](position-manifest.json).


## Complete unexplained-abort traces

At9d065f7, the [paired Windows probe](windows-late-abort-summary.json) captures
both unexplained late-observer failures in its full matrix. Each makes one
fixture navigation, receives zero fixture document requests and retains four
matching navigation events. Both traces acknowledge Tracing.end, complete with
zero loss/drops, and report the requested uncommitted navigation ending with
net_error-3/state10. Neither retained trace contains RenderFrameHostImpl::Stop.
The known Stop control succeeds and ends the requested navigation at state3
with a Stop event. This is a difference in recorded execution paths, not proof
of the initiating cause. All three later sampling rounds pass both observer
positions, preserving the intermittent nature of the failure.

The [navigation state enum](https://chromium.googlesource.com/chromium/src/+/refs/tags/158.0.8127.18/content/browser/renderer_host/navigation_request.h)
for the reported Chromium158.0.8127.18 maps state3 to WILL_START_REQUEST and
state10 to WILL_FAIL_REQUEST. The
[request implementation](https://chromium.googlesource.com/chromium/src/+/refs/tags/158.0.8127.18/content/browser/renderer_host/navigation_request.cc)
sets WILL_FAIL_REQUEST in OnRequestFailedInternal. Its failure handling silently
resets an ERR_ABORTED navigation, consistent with the observed about:blank state;
that handling does not identify the source of the failure status. The retained
sequence includes start checks, URL-loader startup, then OnRequestFailedInternal.
There is no response/commit before failure in these captures.

Six additional fixed URL-loader event names are now retained from the existing
navigation category: interceptor creation, restart, throttling-loader creation,
URL-loader throttle creation, response-interceptor selection and request-failure
notification. The exact names come from the matching
[loader source](https://chromium.googlesource.com/chromium/src/+/refs/tags/158.0.8127.18/content/browser/loader/navigation_url_loader_impl.cc).
This only expands projection after the trial. It adds no observer commands,
category, buffer, pre-navigation delay, runtime behavior or deadline change.
Raw URLs, dictionaries and traces remain excluded; all existing bounds remain.
The aim is to distinguish the next loader stage, not to infer a cause from the
currently incomplete path.

Local acceptance for this projection passes the full sixteen ordinary native
first loads and intentional control, plus twelve ordinary loads and three
controls in the paired rounds. Two Windows-only NetLog cases remain explicit
skips. Browser race, privacy/bounds/correlation controls and host/Windows vet
pass. Actual Windows expanded-loader outcomes are retained below. Source and receipt
bindings for that projection are in [loader-manifest.json](loader-manifest.json).


## Loader failure boundary and separate loading-category trials

At `376a666`, Windows diagnostics run 44225190775 retained all 34 records:
full ordinary matrix 13 PASS / 5 FAIL; sampled ordinary loads 10 PASS / 2 FAIL; three
intentional Stop controls PASS. The full control aborted before the fixture was
hit and never issued Stop; it remains a failed control. Three ordinary failures
and that control have complete, end-acknowledged traces with zero loss/drops.
The sequence includes interceptor/restart/throttling-loader stages, then
MaybeCreateLoaderForResponse and NotifyRequestFailed before the requested
uncommitted navigation finishes with state 10 / net_error -3. Raw events without
explicit identities cannot by themselves prove object association. See
[windows-loader-summary.json](windows-loader-summary.json).

Two additional independent conditions, `direct-trace-loading-late` and its
home-profile counterpart, add only Chromium's `loading` category to the late
collector. Navigation-only conditions are retained. Fixed ThrottlingURLLoader
constructor/start/StartNow/response/destructor names come from the matching
[Chromium loader source](https://chromium.googlesource.com/chromium/src/+/refs/tags/158.0.8127.18/third_party/blink/common/loader/throttling_url_loader.cc).
StartNow records passing the initial throttle-start stage; its internal redirect
branch can still return before calling the loader factory. Bounded capture-local flow aliases preserve
recorded flow equality and explicit process-local scope. They are flow edges,
not universal object IDs. Raw IDs, process IDs, scope strings and request IDs
are omitted. Each alias namespace remains bounded to 64; existing event/byte
bounds and hard failures remain. Three fixed rounds now sample all six trace
conditions and the separate known-cancellation control (42 Windows records).

During local qualification, intermediate five-second capture deadlines expired
after Tracing.end acknowledgment despite successful fixture loads. The matching
[TracingHandler source](https://chromium.googlesource.com/chromium/src/+/refs/tags/158.0.8127.18/content/browser/devtools/protocol/tracing_handler.cc)
acknowledges End before asynchronously stopping Perfetto and reading the trace.
The post-navigation collection deadline is now 15 seconds, with `endElapsedMs`
retained. One final-source sample completed in 5022 ms. This changes collection
only; no navigation, fixture, startup, protection or retry behavior changes.
Incomplete, lost, truncated and over-limit captures still fail.

Final local acceptance: full 18 ordinary first loads plus one intentional control,
and three fixed rounds totaling 18 ordinary loads plus three intentional controls,
all PASS with complete captures. Two Windows-only NetLog cases are explicit skips.
Browser race (1.680s), flow privacy/scope/bounds controls and Darwin/Windows AMD64
browser vet pass. Earlier incomplete captures remain retained and excluded.
See [native-loading-summary.json](native-loading-summary.json) and
[loading-manifest.json](loading-manifest.json). Native Windows loading-category
acceptance is pending; no originating-cause or runtime-fix claim is made.

The preceding full CI failed before browser qualification in a separate native
Windows retained-VT test, tracked in [#450](https://github.com/jordanhubbard/webmux/issues/450).
Its USB job hit HTTP 429 fetching pinned sources. Packaging passed. These failures
and audit #429 remain required gates; none is waived by the diagnostic results.


## Focused loader and network-factory handoffs

Windows `35205bd` run 44226836358 retains all 42 expected records. The full
ordinary matrix has 12 loaded documents and 8 failures; the three sampling rounds
have 13 loaded documents and 5 failures. Two deliberate Stop controls pass; two
abort before reaching the fixture and never issue Stop, so remain failed controls.
All 28 trace collectors acknowledge End, complete without producer data loss,
and flush in 64 ms or less. Six loading projections exceed the 256-event bound;
these remain failed captures and are excluded from accepted path evidence.
[Windows records and scope](windows-loading-summary.json).

Two ordinary loading-category failures, one for each profile location, retain
complete projections with zero drops and four matching navigation events. Each
shows StartNow shortly after the navigation-loader startup, followed by
NotifyRequestFailed/OnRequestFailedInternal and the requested state 10 / -3,
uncommitted, before any fixture hit. Background requests also appear; timestamps
alone do not establish universal object identity or the initiating error source.

The next projection keeps only two loading handoff names, as complete X scopes:
ThrottlingURLLoader::StartNow and CorsURLLoaderFactory::CreateLoaderAndStart.
Loading constructors, responses, destructors and flow bookkeeping are omitted;
navigation-category events and their bounded flow aliases remain. The factory
scope is defined in the matching
[Chromium source](https://chromium.googlesource.com/chromium/src/+/refs/tags/158.0.8127.18/services/network/cors/cors_url_loader_factory.cc).
This reduces background projection pressure and tests whether the network
service factory stage is observed. It does not change collection categories,
placement, commands, buffers, deadlines or navigation. The256-event limit and
all truncation failures remain. A regression drives 80 background loader
lifecycles before the requested failed navigation and verifies that the failure
remains projected without truncation.

Native macOS full matrix passes 18 ordinary first loads and one deliberate Stop
control, with two explicit Windows-only NetLog skips. Both focused loading
conditions observe the network factory stage with 70–72 projected events and
zero drops. All three fixed rounds also pass (18 ordinary loads and three deliberate Stop
controls), with complete captures and positive factory observations in all eight
focused loading trials across the full matrix and rounds. Browser race,
privacy/scope/bounds/background-noise controls and Darwin/Windows browser vet
pass. See [native-handoff-summary.json](native-handoff-summary.json). Source and
receipt bindings are recorded in
[handoff-manifest.json](handoff-manifest.json). Actual Windows focused-projection
acceptance remains pending; no runtime fix or release is claimed.
