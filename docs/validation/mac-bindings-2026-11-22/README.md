# Versioned MAC project binding foundation

Source-bound local qualification for [#673](https://github.com/jordanhubbard/webmux/issues/673).
Read the [contract](../../mac-project-bindings.md) and [receipt](receipt.json).
This is a parser/resolver and read-authentication foundation. No deployed product
read adapter invokes it yet.

The account and exact verified repository select one explicit operator mapping.
Foreign accounts, changed repository/root witnesses, unsupported capabilities,
ambiguous or duplicate entries, duplicate/escaped JSON keys, unknown fields and
credential-bearing or unsafe origins are refused without echoing policy data.
Revision and copy controls protect later scoped observation caches.

Local package race tests, vet and a bounded two-worker fuzz exercise pass.
Separate deliberate Go overlays remove account selection or duplicate-key
refusal. The expected foreign/unknown-account and duplicate/escaped-key controls
fail in those mutants; the real source remains unchanged. These are mutation
controls, not historical defects, prior source revisions or live fleet evidence.

At the first source-only checkpoint, MAC main source was refreshed by pinned GitHub reads at
`fa08464470f6562ae53329c8e4385e87e9061856`. The inspected API and project-graph
contracts remain unchanged from the earlier source audit. No MAC checkout or NAS
was accessed, and no task, worker, credential or fleet policy was changed.

Remaining #673 mappings, remote authority qualification, human/agent read
consumers, shared admission/publication and three-host acceptance remain open.
The observed local checks do not qualify those requirements or current-head CI.


## Fixed read-route qualifier and exact-field correction

The later [read-access receipt](read-access.json) binds the current source
separately from the first parser-only checkpoint. A Go helper now checks absent,
invalid and supplied credentials through one fixed GET route, with TLS, redirect,
proxy, response-bound and cancellation controls. It exposes only observation
metadata and leaves principal identity and non-read operations unqualified. No
product loader, browser/MCP consumer or live fleet configuration invokes it yet.

The work also corrects Go JSON's case-insensitive struct-field matching. Operator
policy fields are now checked by exact name before decoding; uppercase or
case-alias fields cannot silently select another account. Upstream evidence uses
exact wire-key lookup as well. Deliberate missing/invalid-credential and case-alias
mutations fail the intended controls; the actual source remains unchanged.

At that earlier checkpoint, private profile metadata was inspected locally without reading credential files.
No suitable current read-only profile was found. Controlled HTTP/TLS tests do not
qualify the real fleet, actual least-privilege scopes, verified SSH publisher
ownership or live revocation. Those and the remaining phase-one wire mappings
stay open under #673. No MAC or NAS mutation was performed.

## Same-host native authentication qualification

The later [native series receipt](native-read.json) records actual calls to the
already-running local MAC service. It supersedes the earlier checkpoint's lack
of a dedicated read-only credential; earlier receipts remain historical.

The native CLI enrolled two dedicated clients scoped exactly to `read`. One
observer profile was installed without activation; the temporary probe was not
installed. The original active profile and original client metadata were
preserved. Only the temporary probe was revoked. All credentials, native profiles
and enrollment manifests remain private and local.

The opt-in Darwin test pins the listener's PID, OS account and process-start
fingerprint before and after calling the actual Go helper. The observer passed;
the probe passed before revocation and was refused afterward while its expiry
was still future. The observer passed again after the probe's revocation. Generic
writes to an intentionally nonexistent route were refused. The publisher's
fingerprint stayed identical across all four observations. No worker registration,
task dispatch, fleet policy, service restart or NAS access occurred.

Run only with a dedicated private enrollment manifest and an explicitly qualified
local publisher, from `webmux/server`:

```sh
WEBMUX_MAC_NATIVE_MANIFEST=/private/path/read-only-enrollment.json \
WEBMUX_MAC_NATIVE_ORIGIN=http://127.0.0.1:8789 \
WEBMUX_MAC_NATIVE_PUBLISHER_PID=<verified-pid> \
WEBMUX_MAC_NATIVE_RECEIPT=/private/path/new-observation.json \
go test -v ./internal/maccoord -run '^TestNativeLocalMACReadAccess$' -count=1 -timeout=30s
```

The test skips by default and never enrolls, renews or revokes a credential itself.
For a separately revoked, still-unexpired own probe, set
`WEBMUX_MAC_NATIVE_EXPECT_REFUSAL=1`. The recorded revocation test is historical;
repeating it after expiry would not prove revocation independently of expiry.

The full package race suite and vet passed. Windows AMD64 test cross-compilation
also passed; that is compilation evidence, not native Windows execution. The
installed MAC source revision is recorded separately from the running publisher,
whose loaded revision is unattested. A synthetic project key does not establish
project existence or the missing session/attempt/source mappings. Human/agent
consumers, participation, shared admission/publication and three-host acceptance
remain open.

An optional broad `GET /projects` request timed out after three seconds. Fresh
health and fixed graph calls passed afterward. No broader endpoint retry,
deadline increase or service restart was used; its exact cause remains unproved.

## Source revision qualification

The [source receipt](source-observation.json) covers the later execution-host Git
observation component. Real temporary repositories exercise commit refresh,
clean/modified/staged/untracked and detached states, a different root with the
same remote, account refusal, inherited Git routing/configuration, cancellation
and bounded output (including `io.Copy`'s optional fast paths). The full package
race suite and vet pass; Windows AMD64 cross-compilation passes.

An explicit opt-in test also called the helper on this feature worktree and
matched the supplied exact commit, canonical root and dirty state. Its test-only
authority/project mapping does not identify or enroll a live MAC project. The
private raw receipt retains the local worktree path; the public receipt omits it.
No MAC credential, worker/task state, service or NAS access was involved.

From `webmux/server`, use a fresh private receipt path and explicit expectations:

```sh
WEBMUX_MAC_SOURCE_RECEIPT=/private/path/new-source-observation.json \
WEBMUX_MAC_SOURCE_EXPECTED_ROOT=/canonical/feature/worktree \
WEBMUX_MAC_SOURCE_EXPECTED_COMMIT=<full-commit-object-id> \
WEBMUX_MAC_SOURCE_EXPECTED_DIRTY=1 \
go test -v ./internal/maccoord -run '^TestNativeLocalSourceObservation$' -count=1 -timeout=30s
```

The test skips by default. This is metadata sampling, not a frozen workspace,
roadmap-source import, session/task/lease mapping or publication qualification.
