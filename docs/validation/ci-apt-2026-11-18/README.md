# Ubuntu CI package-source isolation

CI 44281413355 at `2f9a13d` failed before native Linux desktop, checked Rig
Stop and relay qualification: Ubuntu indexes loaded, but the runner's unrelated
Microsoft package index returned HTTP 429. Strict APT update handling correctly
failed the step. This is #487, separate from npm security advisory #429.

The existing Ubuntu CI setup steps now explicitly use `--ubuntu-only`.
`configure-ci-apt.py` copies configured official Ubuntu entries into its own
source directory, retaining suites, components, architectures, signing keys and
disabled state. It supports legacy and Deb822 files, mixed URI fields and
continuations separated by comments. Both snapshot formats are rewritten on
each call, avoiding stale entries. Unsupported/ambiguous configuration fails.
Vendor configuration remains in its original location. The default mode
continues to retain third-party entries. Official archive, security and ARM
ports URLs are normalized to HTTPS; known mirror-file aliases remain supported.

Source-directory selection follows [APT configuration](https://manpages.ubuntu.com/manpages/jammy/man5/apt.conf.5.html).
The copied entries preserve the documented [source formats and signing-key fields](https://manpages.ubuntu.com/manpages/jammy/man5/sources.list.5.html).
Network retries/timeouts, lock waits, strict update-error handling, signature
verification and all native application tests/deadlines remain in place.

Six deterministic controls pass, including mixed formats, inline key blocks,
disabled state, mirror aliases, unknown URI rejection, idempotence and stale
format removal. Native Linux ARM64 qualification also passes on Ubuntu 24.04,
APT 2.8.3, using the installed Ubuntu archive keyring:

- An unscoped update contacts the controlled HTTP 429 feed six times and exits
  100, reproducing the original dependency on an unavailable unrelated source.
- The selected signed Ubuntu update succeeds and contacts that feed zero times.
  Both the separate vendor file and the commented Deb822 alternate URI are
  excluded from the selected source set; the original vendor file is retained.
- An empty untrusted keyring causes a native `NO_PUBKEY` refusal and exit 100.

The qualifier declares controlled release/security sources in both formats and
isolates configuration, list/cache state and logs beneath its output directory.
It does not write system source files, installed trust keyrings or installed
packages. Unit controls cover canonical mirror-file aliases; full CI separately
exercises its original configured Ubuntu entries and those aliases. Local
prerequisites were installed only inside an owned disposable container, which
was removed. Source, keyring, report and log hashes are in `results.json`.

Hosted native controls on Ubuntu 22.04/24.04 and the full Linux application
pipeline are pending. Local ARM64 success is not a claim that those AMD64
checks passed. No dependency-audit exemption, merge or release is claimed.
