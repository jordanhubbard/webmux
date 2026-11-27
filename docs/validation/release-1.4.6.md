# WebMux 1.4.6 local release-candidate qualification

The source and release metadata tested locally were committed unchanged as
`686a695e83493367a24db9b5c137043cd2f72a49`. The package artifacts below were
subsequently built from that clean commit. The complete pre-commit test run is
bound to the same source bytes; exact-revision hosted checks remain required.

Native Linux x86_64 qualification passes:

- Full `make test`, exit 0: Go race, Go and bundled-library stages; five strict
  TypeScript projects; 500 frontend tests; 299 helper passes and three platform
  skips; 1,418 E2E passes, zero failures and 22 existing opt-in/platform skips.
- `make -o deps lint`, exit 0, reusing the exact installed dependencies.
- Ten release metadata/readiness/real-Make workflow controls, with no failures,
  cancellations or skips. Git/GitHub side effects remain isolated fixture
  boundaries in those controls.
- Combined, server-only and client-only native archives, complete SHA-256 upload
  sets, and relocated standalone runtime smoke covering HTTP/UI, persisted
  login, PTY/WebSocket, deletion, restart and configuration preservation.
- Actual public configure/start/stop flows using the packaged server: saved
  ports, explicit override and OS allocation all pass without rewriting saved
  configuration. The operator service was preserved.

The first full run retained a type-declaration failure in the recovered port
resolver; optional overrides now have an optional type and all strict projects
pass. The next complete run retained 13 browser E2E failures on a worker with a
locked Secret Service login collection. Chrome's anonymous network trace stalled
before connecting to the fixture. Direct and screenshot navigation controls hit
the unchanged 15-second bound. In a private D-Bus session with a disposable
**encrypted, unlocked** keyring, unchanged browser controls passed in 0.28 and
0.25 seconds, and the full original E2E assertions subsequently passed. The real
login collection remained locked. Trial focus/tab changes were rejected and are
absent from the product source. No browser protections, assertions, retry policy
or test deadlines changed to obtain qualification.

Use `python3 scripts/with-test-keyring.py make test` for this headless fixture;
see the Linux guide for its prerequisites and production Secret Service boundary.
The native controller/iframe controls also passed independently.

The [machine-readable receipt](release-1.4.6.json) records the local artifact
hashes. These are candidate outputs, distinct from later hosted release assets.
Publication still requires reviewed candidate checks, latest exact-main CI,
Packaging and Windows ARM64 native qualification, then published asset/checksum
verification. Planned trackpad and automatic multi-host coordination acceptance
under #661/#631/#664 remains open; this patch release does not complete it.
