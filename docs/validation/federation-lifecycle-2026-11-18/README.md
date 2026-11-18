# Federation lifecycle diagnostics

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

All 123 helper controls, five strict TypeScript projects, the frontend build,
workflow YAML parsing and whitespace checks pass. This is qualification of the
observations, not repair of #478. Instrumentation can affect timing; a later
green sample alone does not establish a cause or fix.

The separate Windows workflow checks the intentional failure and then runs
one or ten independent fresh samples. Every outcome is retained and any
unexpected failure or missing receipt fails the job. Existing federation
assertions, the five-second marker checks, the 90-second case deadline,
protected Chromium preparation and standard CI gates remain in place.
Native Windows evidence is still required. Profile cleanup #473 and dependency
audit #429 remain separate blockers.
