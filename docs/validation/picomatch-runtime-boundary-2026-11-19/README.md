# Picomatch runtime boundary review

The inspected native preview archives and byte-matched embedded browser do not
contain the affected matcher. Generic picomatch 4.0.7 still exhibits the repeated
extglob CPU failures. These are separate conclusions. [#429](https://github.com/jordanhubbard/webmux/issues/429)
remains open for review of the release classification and build-tool boundary;
this report does not establish an algorithm repair or authorize a waiver.

## Fresh primary status

On 2026-11-19, the [GitHub advisory](https://github.com/advisories/GHSA-fc5q-39xc-vjpf)
is withdrawn and still lists no patched version. The
[CVE record](https://cveawg.mitre.org/api/cve/CVE-2026-112366) remains PUBLISHED,
and the [npm latest metadata](https://registry.npmjs.org/picomatch/latest)
still reports 4.0.7. Advisory withdrawal and a passing unchanged npm audit do
not change matcher behavior.

Six fresh probes run in separate owned Node processes with a two-second bound.
The normal extension and configured-test patterns finish successfully. Three
star-containing repeated patterns exceed the bound and their children are
killed and waited. The repeated-word alternative finishes in about 37 ms.
Compiled regexes, input lengths, timings and source hashes remain in the
[receipt](receipt.json). No installed dependency, lockfile, override or audit
policy was changed.

## Observed artifact boundary

Evidence binds to clean source `4b0d4b0ec699bc6d34b48fa7a9f9911481879557` and
[Packaging run 44445866139](https://github.com/jordanhubbard/webmux/actions/runs/44445866139):

- The committed lockfile marks picomatch and each of its four direct importing
  packages as development dependencies: Vite, Vitest, tinyglobby and fdir's
  optional peer. Workspace propagation is not a count of distinct flaws.
- An actual production frontend build records 145 module identities, with no
  picomatch, micromatch, tinyglobby, fdir or Vitest module. It emits six final
  assets including HTML. The graph is captured without changing source or
  writing generated files into the repository.
- The actual macOS packaged Go server is started with an isolated writable home
  and private loopback listener. Its build reports this exact clean source.
  All six served asset bodies match the production graph's final output hashes.
  The server exits zero after the check; no account, model turn or service is used.
- All twelve server/all/client tar or ZIP archives have inspected member lists
  and component inventories, with no adjacent JavaScript, node_modules or Node
  runtime and no glob tooling in their browser dependency inventories. The two
  MSI files retain separate checksum/install-smoke scope; this audit does not
  claim an independent MSI unpack inspection.

The native packager uses an explicit Go executable/service/license allowlist,
and the browser's non-development dependency inventory excludes the matcher.
The frontend/vendor Git trees and package, lock, Vite, packager and Go-builder
inputs are byte-identical in the separate VT preparation composition. This is
a source/artifact boundary statement, not external build attestation.

The first installed-topology `npm ls` attempt followed shared fixture links and
reported an extraneous package. It is retained and rejected as dependency-graph
evidence. The receipt uses the exact committed lockfile edges instead; the
matching packaged browser bytes independently bind the build output.

## Proposed disposition and remaining review

This supports a **not present in the inspected native runtime** disposition for
this matcher. It does not support saying that picomatch itself is repaired or
that arbitrary uses of its regex-producing API are safe. The development-tool
call boundary, handling of user-selected build patterns, final release-source
checks and disposition of #429 still require review. No security gate is
suppressed or narrowed by this work.

Optional operator-installed Rig/other CLIs, Node packages, source-development
servers, external tools and their deployed inventories are separate products
and configurations. This is not global dependency, vendor, license or security
clearance under #91. Full roadmap and release acceptance remain open.

The subsequent [default build-input review](../picomatch-build-inputs-2026-11-19/README.md)
adds actual compile-pattern traces, a full 444-test pass, static origin checks and
six unchanged final asset hashes on sourcec195. It proposes the repository
classification for #429 review, with generic matcher and external-deployment
limits retained.

The later [candidate exposure disposition](../picomatch-disposition-2026-11-19/README.md)
records the technical review on b155, including fresh 450-test/146-module
execution and native-package evidence. It preserves this report's historical
source scope and the generic failures; documentation integration and exact-main
reconciliation remain required under #429.
