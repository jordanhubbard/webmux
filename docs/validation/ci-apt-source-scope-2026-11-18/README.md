# Ubuntu CI source selection

Issue #487 records an unrelated Microsoft repository returning HTTP429 before
Linux native tests could run. Provisioning now selects enabled official Ubuntu
repositories into a private APT source directory, preserving original legacy and
deb822 files, vendor feeds, suites, architecture constraints and signing keys.
Mixed URI stanzas select only their Ubuntu URI. Only exact official archive,
security and ports URLs and the hosted runner's known mirror-file references
are accepted; mirrors use HTTPS. Sources that disable verification are refused.

The CI-only entrypoint requires explicit GitHub CI context before configuring
/etc/apt. Workflows pass this through sudo explicitly. Signature verification,
strict index-update failure handling, network/lock bounds, all native tests and
their deadlines remain. No vendor configuration is deleted.

[Source-bound receipt](receipt.json): seven deterministic tests pass, including
real APT source selection with both source formats, signature settings, mixed
vendor URIs, disabled/host-lookalike sources, idempotence, symlink refusal and
the system-entrypoint guard. Python compilation and undefined-name checks pass.

A real APT update used empty isolated lists/cache, the Ubuntu archive Signed-By
keyring and a live third-party HTTP503 fixture. It fetched the official signed
jammy InRelease, returned zero, made zero requests to the vendor fixture and
left its configuration byte-identical. An independent gpgv check returned zero
with the Ubuntu Archive Automatic Signing Key (2018). No package indexes were
requested in this bounded network control. A host post-update hook printed a
local socket error; the signed index and APT exit were verified separately.

The receipt binds source and retained logs by SHA256. Exact-head hosted Linux
provisioning and native tests still require completion; this local control is
not a passing hosted gate, dependency-audit #429 remediation or release proof.

The combined integration #494 found a CLI guard portability issue: macOS
resolves /etc through a filesystem alias. The standalone follow-up now
canonicalizes both sides of that comparison too. All seven tests pass on Linux;
puck.local passes six with the effective-APT test explicitly skipped because
APT is unavailable. The existing guard test checks the actual macOS path. The
receipt preserves the original signed-index source binding separately from
this CLI-only follow-up. New-head hosted checks remain required.

Before this follow-up, hosted source cc5d3d6 passed signed Ubuntu24 provisioning
and the complete native USB kernel suite in run44286524931/job134740499929.
That result does not replace the remaining full Ubuntu22 desktop job or the
new-head gates.
