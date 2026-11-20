# Stored SSH destination validation (draft)

Refs [#625](https://github.com/jordanhubbard/webmux/issues/625) and
[#91](https://github.com/jordanhubbard/webmux/issues/91).

A failed terminal launch can intentionally remain in session history. The browser
companion previously treated that retained metadata as already validated. A
controlled HTTP regression creates an invalid SSH username, receives the retained
error-state session (201), then starts its browser companion (200) and observes
execution of an explicit local SSH witness. This demonstrates a first-party
validation gap, not an OpenSSH exploit or successful remote connection.

The candidate shares the existing terminal address contract with browser workers,
auth callbacks, browser profile removal, file-transfer command construction,
capability probes and recovery command construction. Host/user length and character
limits and the port range are checked before constructing or executing SSH.
Port zero still selects 22 and an empty username still uses the native default.
Error-state history and existing file-route connected-state checks remain intact.
Local browser workers retain their existing path.

## Evidence and remaining qualification

The original implementation fails the new HTTP, browser/file command and capability
probe regressions. After the fix, the focused regressions and existing valid argv
controls pass. Server vet and both native macOS ARM64 development binary builds
pass. The [receipt](receipt.json) binds source and evidence hashes. Three subsequent
compatibility tests (default port/empty user and overlong recovery identities) have
been added but have not run; the earlier pass does not cover those additions.

The full server race run fails and remains failed. One native-create fixture ran
with ambient Node 26, while its supported contract and repository CI require
Node 24. A diagnostic run reports `native_create_state_unavailable` before its
request directory is created. Node 24 is already available locally; the next run
must select that executable without changing the fixture contract.

The other failure is a recovery observation timeout. A later diagnostic records
`ps -axo pid=,ppid=` taking 12 seconds against the existing two-second context;
`lsof` never starts because that context has already expired. This identifies a
later process-query timeout, not proof of the original failure's precise cause.
No recovery deadline, assertion or production observer has been relaxed.

Puck is severely overloaded. Lightweight native Mach samples at 04:30 UTC on
2026-11-20 show 79–87% CPU and load averages in the hundreds on 12 logical CPUs.
New builds, tests and native qualification are held pending worker recovery or an
already-authorized alternate Mac. The failed run is not accepted as green.

A native application fixture is prepared to compare the original and candidate
binaries over authenticated loopback HTTP, retained sessions and an actual server
restart, with a controlled SSH executable witness. It has **not run**. It requires
a fresh explicit healthy-worker admission receipt. It does not qualify native SSH
authentication, remote Chrome, vendor patch status, signing or packages.

Before merge: run all added compatibility tests, rerun the complete server race
suite on a healthy worker with Node 24, and execute the prepared native boundary
check. Preserve earlier failures alongside the new results. No frontend behavior
changed. Broader native-component, roadmap and release gates remain open.
