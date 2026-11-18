# Combined roadmap qualification candidate

Compose #497 (password Return, USB integration and scoped Windows recovery
evidence), #498 (native signed APT outage controls) and #499 (native VT child
status reporting). The source-bound [receipt](receipt.json) records all three
immutable inputs, which remain ancestors without conflict resolutions.

The complete application tree, browser acceptance tests, canonical APT
provisioner and required CI workflow match #497 byte for byte. This composition
adds the two separate qualification workflows and their controls; it does not
change production policy or substitute diagnostic results for required gates.

Local verification passes all 418 frontend tests across 42 files, five strict
TypeScript projects, frontend lint, helper compilation and the complete native
application build. APT controls pass six cases with the actual-APT case explicitly
skipped on macOS; native-exit controls pass three with the Win32 high-bit case
explicitly skipped. Both workflow YAML files parse and whitespace checks pass.
A first local YAML probe used an unsupported default import from js-yaml; the
corrected named import passes. No project source changed for that probe.

Prior #498 Ubuntu 22.04/24.04 and ARM64 reports qualify their original inputs.
Prior #499 native Windows reporting controls qualify that input, not a repair
of #450. The abrupt Intel/XSTATE failure on #496 remains unresolved. The imported
Windows service receipt qualifies its original artifact, not authenticated
recovery of this candidate. Exact combined-head CI, native and packaging outcomes
remain required. Audit #429, #318, #355, #438, the other open roadmap acceptance
items and signing/release prerequisites remain open. No main merge or release.
