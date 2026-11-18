# Native controls for the canonical Ubuntu provisioner

This follow-up retains the real outage/signature controls from PR #495 on the
canonical guarded #493 provisioner already composed by #494/#496. It adds no
second provisioning implementation and changes no application source, unit
tests, system-path guard, CI source policy, signature checks or test deadlines.
The #496 base is `d77c29a`; source and receipt hashes are in `results.json`.

The isolated qualifier first configures its private source selection. For its
negative baseline only, APT command-line source options select the original
fixture entries, which include a live loopback HTTP 429 repository. The next
update uses the canonical selected entries. A third update supplies an empty
untrusted keyring. System APT configuration, public keyrings, installed packages
and original vendor files are not modified. Release/security entries exercise
both legacy and Deb822 formats, including a mixed URI continuation after a
comment. All list/cache/log state stays beneath the fixture output directory.

On native Linux ARM64, Ubuntu 24.04 with APT 2.8.3:

- All seven existing canonical source controls pass without skips.
- The unscoped baseline contacts the HTTP 429 feed six times and exits 100.
- The selected signed update succeeds with zero vendor requests and unchanged
  original vendor configuration.
- The untrusted key produces native `NO_PUBKEY` and exit 100.

macOS passes six unit controls and explicitly skips the APT-dependent control;
it is not counted as native Linux evidence. Python compilation, workflow YAML
parsing and whitespace checks pass. The disposable local container was removed.
Hosted Ubuntu 22.04/24.04 native controls are pending. The full #496 application
CI and its native desktop/Rig/relay/USB qualifications remain separate gates;
these component checks do not replace them or resolve security audit #429.
