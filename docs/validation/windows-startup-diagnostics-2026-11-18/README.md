# Windows startup phases and connected-application gate

Hosted browser fixtures report an unconfirmed ordinary-local provider even in
the green baseline. #543 remains open: its hosted cause and repair are unproven.
This change adds safe phase evidence and a required application gate; it does
not treat a browser companion anchored to an error-state terminal as native
startup qualification. See the [receipt](receipt.json) for exact bindings.

Bootstrap and owner failures now report only fixed labels for configuration,
root, ownership, socket, image, screen, record, terminal, owner process/reply and
bootstrap process/reply. Numeric Windows codes are retained only when a typed
syscall error is available. Arbitrary child error text is ignored, preserving
the credential boundary. A missing executable reduced by Go to `exec.ErrNotFound`
has a terminal phase but no invented OS code. Existing ready/Error/Code fields
carry this evidence; successful replies and durable identity records are unchanged.

The controller still checks the original rendezvous after startup failure. It
does not interpret timeout as proof that an owner died or authorize another
job. Bootstrap, owner and handshake budgets, lifetime ownership and survivor
lookup remain unchanged. Existing security-block diagnostics remain available.

Actual native Windows AMD64/Go 1.27.4 validation:

- All three new top-level controls pass, with fifteen test/subtest pass events.
  They cover the twelve phase labels, typed OS codes, payload privacy, one safe
  Serve failure reply and actual Ensure propagation without starting a job.
- The full provider package passes all 23 top-level tests and 91 test/subtest
  events, without failures or skips. Existing external test bodies are
  byte-identical, including original-owner/survivor and ConPTY teardown cases.
- Restoring the parent server/lifecycle code while retaining the same focused
  new Serve test fails as expected: the old generic error/code0 loses the
  terminal phase and typed code2. This is a negative phase-reporting control.
- The first new integration expectation incorrectly required an OS code after
  Go's lookup had discarded it. Its native failure remains retained. The
  corrected test requires the phase and rejects invented codes; separate typed
  failure controls still require codes2/5. No existing test was weakened.
- No Go runtime overrides were present. All 43 recorded temporary provider
  roots are absent, and Restart Manager finds zero holders for the three
  executed test images. No forced owner cleanup was required.
- Windows AMD64/ARM64 cross-builds, Windows-target vet and five strict
  TypeScript projects pass. ARM64 compilation is not native ARM64 evidence.

The new Windows browser test creates an ordinary-local application terminal and
requires a real owner selection. It launches a bounded shell child, checks its
fresh nonce and PID receipt, then requires original-only Stop confirmation and
child exit before independent expiry. An identical Stop request must remain
idempotent. Cleanup follows the existing Stop/retain contract. An unconfirmed
launch fails this gate and retains its recovery record.

At cc4316c, [hosted run 44461807506](https://github.com/jordanhubbard/webmux/actions/runs/44461807506)
fails the new application gate: bootstrap starts, but independent owner creation
returns Windows error 5. The same job skips recovery component cases after its
breakaway permission probe is denied. Those skips do not prove recovery. The
precise application failure cause remains unproven. Raw logs and artifacts are
retained; the gate is still required and no unchanged failed run was retried.

The follow-up adds read-only job observations only on owner creation failure:
outside jobs, immediate job allows/disallows breakaway, in a job with unreadable
limits, or unknown membership. These are closed labels with no arbitrary text,
paths, tokens or job names. Immediate job flags cannot establish restrictions in
all parent jobs. Process flags, deadlines, failure exit status and ownership
remain unchanged. The Windows APIs are documented by Microsoft:
[IsProcessInJob](https://learn.microsoft.com/en-us/windows/win32/api/jobapi/nf-jobapi-isprocessinjob)
and [QueryInformationJobObject](https://learn.microsoft.com/en-us/windows/win32/api/jobapi2/nf-jobapi2-queryinformationjobobject).

This application gate does not authenticate a coding CLI or replace the separate
[native conversation/application evidence](../windows-local-stop-2026-11-18/README.md).
CI and the actual #543 diagnosis remain required. No new login, authentication
storage read/copy, CLI installation, installed service/account/protection/firewall
change or reboot occurred during this component qualification.

The job-observation follow-up passes three native AMD64 top-level controls / 23
pass events with zero failures/skips and no runtime overrides. The malformed
image control retains the original failure exit status and exactly one safe
reply. AMD64/ARM64 cross-builds and Windows-target vet pass. See the separate
[source-bound receipt](job-observation.json); the earlier full-package receipt
continues to describe its original image. No persistent owner is started by
these focused checks.
