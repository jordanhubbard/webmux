# Canonical MAC registration reader — 2026-11-27

Foundation for [#673](https://github.com/jordanhubbard/webmux/issues/673), not
completion of #673/#674 or the three-host coordination workflow. The MAC API
prerequisite is already published; this change adds its WebMux-side reader.
No MAC project, worker, account, task or policy is created or changed.

## Boundary

`Bindings.ObserveProjectRegistration` first resolves the receiving account and
server-verified repository against the immutable operator binding. It then reads
only `GET /projects/{exact_project_key}/registration`, upstream schema
`mac.project_registration.v1`. It never uses graph `found`, a display-name match,
a task listing or a successful generic project request as registration evidence.

The helper repeats missing-credential, fresh-invalid-credential and intended-
credential observations on that fixed route. It uses the existing authenticated
transport with a tighter 16 KiB body bound, the same five-second total context,
normal TLS/operator CA handling, no ambient proxy, no redirects and no retries.
Other readers retain their existing 1 MiB body bounds. Cancellation/deadline
errors remain detectable without leaking URLs, credentials or response bodies.

Only the exact eight-field upstream shape is accepted. Unknown/duplicate keys,
case aliases, unsupported schema, foreign project names, invalid canonical IDs,
malformed timestamps/nullable fields and oversized bodies yield no observation.
A 404 also yields no observation: it cannot alone distinguish an unregistered
selection from an authority missing this supported endpoint.

The `webmux.mac.project-registration.v1` projection retains the canonical record
ID, exact selected key, reported registration status, timestamps, operator binding
revision and read-authentication observations. Descriptions and raw repository-
registration values are withheld, including credential-bearing URLs. A reported
repository-registration value does not prove agreement with the receiving
repository: `repository_link_state` stays `unverified`. Principal identity,
freshness and other operation capabilities also remain explicitly unqualified.
This record is not authority to dispatch, acquire a lease or publish work.

## Checks and native scope

The [receipt](receipt.json) binds runtime/test source hashes, retained test logs
and the native test executable. With Go 1.27.4 on Linux x86_64:

- The complete `internal/maccoord` package passes race tests and vet.
- Full unchanged `make test` and `make lint` pass, including 1,418 E2E
  passes and 22 existing platform/opt-in skips. The installed native Chrome
  override retains its sandbox; the keyring and tmux state are private.
- Registration controls cover exact/escaped selection, malformed-path rejection
  before transport, foreign accounts/repositories/projects, missing/invalid
  authentication, open-authority refusal, credential revocation without cached
  reuse, strict wire validation, withheld sensitive values, redirects, body
  bounds and cancellation both before headers and during streaming bodies.
- Darwin ARM64 and Windows AMD64 package test cross-builds pass. Cross-builds
  are not native authority-authentication acceptance on those targets.

A short actual Darwin ARM64 run on Puck passes the opt-in
`TestNativeMACRegistrationRead` against its already-running MAC publisher. The
existing native operator credential stays in place and is supplied only to that
local test process. The publisher's OS UID, listening PID and process-start
fingerprint are checked before and after the read. The real canonical `mac`
project record is projected, missing/invalid credentials are refused, and a
foreign receiving account is refused by the local binding.

That native test deliberately uses an explicit test-only mapping between the
verified WebMux repository and the existing `mac` project. It does not enroll a
canonical WebMux project or prove that those repositories belong together.
Operator-backed success is not a dedicated read-scope or actual revocation
qualification. The historical read-only enrollment expired on 2026-11-24 and was
not renewed. A separate uniquely named five-minute read-only test client then
passed the existing native read/scope control and the new registration read.
The generic write probe targets an intentionally nonexistent route and is refused
before business dispatch. The same credential is actually revoked and both
readers refuse it immediately, while its original expiry remains in the future.
All four controls retain the same publisher fingerprint. The test client is
revoked; its temporary manifest is removed, and no credential left Puck. No
persistent profile was installed or switched. These are read/generic-write/
revocation observations, not a complete least-privilege or principal-identity
audit. No project/worker registration, production service restart, deployment
or shared execution-authority activation was performed. Compilation ran on Bullwinkle; Puck ran only the short
read probe and its own temporary test executable was removed afterward.

An initial added unsafe-key test failed because it expected malformed slash,
backslash and query keys to reach the new reader; the existing binding parser
already rejects them. The final test asserts that earlier rejection separately
from the new reader's encoded/dot-segment refusal. Production guards were not
weakened. The failed log remains retained and is hashed in the receipt.

The first full-suite invocation also stopped in the unchanged private tmux
cleanup control: the external qualification shim forced a named socket even
when the maintained fixture selected its own `TMUX_TMPDIR/default` socket. The
recorded private process is gone. The corrected external launcher honors the
fixture's private location and guards only otherwise-unscoped calls. All nine
original cleanup controls then pass, followed by the complete original test and
lint targets. No product source, assertion or deadline was changed to resolve
this harness failure. Both failed and corrected logs are hashed and retained.

## Remaining work

No product loader, browser/MCP consumer or execution backend invokes this new
reader. A complete least-privilege/principal-identity audit, canonical WebMux
enrollment, repository/source/session joins, shared human/agent views and
ordered participation/admission/publication/three-host acceptance remain under
#673 through #678. Draft #682 retains its separate review and integration work;
none of that branch was merged to supply this helper.
