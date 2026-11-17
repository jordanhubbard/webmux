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
