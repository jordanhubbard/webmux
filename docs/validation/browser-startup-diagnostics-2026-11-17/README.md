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
