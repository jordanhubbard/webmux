# Native two-host discovery direction controls

Issue [#631](https://github.com/jordanhubbard/webmux/issues/631) remains open.
Production Go discovery finds the exact Linux advertisement on macOS, but Linux
fails to find the macOS Go advertisement. A separate native macOS Bonjour
registration is found by the same Linux production browser. No product discovery
code, installed service, firewall or network permission changed.

| Independent native endpoints | macOS endpoint exit | Linux endpoint exit | Result |
| --- | --- | --- | --- |
| Linux Go advertises; macOS Go browses | 0 | 0 | Exact peer found |
| macOS Go advertises; Linux Go browses | 0 | 1 | Linux times out |
| Both Go endpoints advertise and browse | 0 | 1 | Only macOS finds its peer |
| macOS system Bonjour advertises; Linux Go browses | -2, owned SIGINT cleanup | 0 | Exact Bonjour peer found |

**The complete qualification exits 1.** A successful advertiser exit only means
that it ran for the observation interval. It does not mean a browser found it.
The Bonjour control remains separate from production implementation. Its process
must remain alive until Linux finishes and exit on the fixture's SIGINT; an early
registration failure cannot pass. Candidate address records observed here are
IPv4. This does not establish IPv6 transport support.

The opt-in `TestNodeNativeEndpoint` calls actual `StartNodes` with a fresh node ID,
a private output directory and the same `os.Hostname()` input as the production
command. Each host checks the other's independently generated ID, exact instance,
port and TLS declaration. It preserves eight-second browse/both bounds and a
12-second advertiser lifetime. The simultaneous case stays alive after observing
a peer, permitting the other endpoint to finish. Missing peers are failures.
Without the role environment variable, this explicit network fixture skips.

These endpoints open no HTTP listener. Port 19311 and `tls=1` are declared fixture
metadata; they provide no TLS or enrollment evidence. Advertisements remain
untrusted. No provider, session, account policy or OS configuration is changed.

Run the published qualifier from an already authorized macOS ARM64 checkout with
Go, SCP, SSH and Python 3, and an already authorized Linux ARM64 host with Python
3. The remote parent directory must exist. Both leaf output directories must be
new. Replace the example paths and SSH alias with owned locations:

```sh
python3 scripts/qualify-discovery-two-host.py \
  --output "$PWD/_build/discovery-two-host-owned" \
  --ssh-host YOUR_AUTHORIZED_HOST \
  --remote-root /YOUR/EXISTING/PRIVATE/PARENT/discovery-owned \
  --bonjour-control
```

The script builds both binaries itself with `CGO_ENABLED=0`, verifies the
transferred Linux executable's hash before every phase, checks Linux's native
kernel and architecture, and binds results to unchanged discovery/dependency and
qualifier sources before and after the run. The Linux executable is cross-built
on macOS and actually executed on Linux; this is not a native Linux build claim.
Output directories and files are private. Logs and records contain LAN addresses
and must remain private. The [public receipt](receipt.json) contains hashes and
directional results, without private hostnames or addresses. Private evidence is
retained below `_build/discovery-darwin631/qualified-two-host`.

Earlier synthetic-host runs produced the same directional failures. Switching
only the Mac endpoint to its real production hostname also failed reverse and
simultaneous discovery. A bounded Pion debug run of that advertiser recorded zero
received packets and zero reply attempts. The macOS application firewall reported
disabled, with incoming connections permitted for that test binary. These narrow
the investigation; they do not establish the underlying OS/network cause.
The original failed [self-origin multicast controls](../discovery-transport-2026-11-22/README.md)
remain failed. Their results alone cannot establish inter-host reception: this
fresh forward phase succeeds despite those self-loop failures.

Discovery race tests and vet pass. Windows ARM64 test compilation passes, with no
native Windows execution claim. Python compilation and CLI help pass. Full
bidirectional discovery, native platform integration, authenticated enrollment,
address changes, expiry and remaining #631 acceptance still require work.

## Current-main integration, 2026-11-23

The combined #658/#663/#666 stack was qualified at `fbee9bf` on main
`7191892`. [The integration receipt](../discovery-ambiguity-2026-11-22/integration-2026-11-23.json)
binds fresh native macOS/Linux HTTPS and HTTP directional/instance/ambiguity
controls, standalone and race-build Bonjour registration lifecycle controls, the
full local server race suite and vet, the production API/session contract, and
standalone Darwin ARM64/AMD64, Linux ARM64 and Windows ARM64 builds. All pass.
The original observation bounds and production/qualifier sources are unchanged
from #666; original failed receipts retain their outcomes. Cross-compilation
does not establish native platform execution.

The retained owner instruction requires local resources and no new hosted
qualification jobs, with `[skip ci]` on new commit and merge subjects. It
supersedes the earlier fresh-hosted-check delivery wording. Existing hosted
runs remain attributed to their original source; no new hosted packaging or
whole-current-source platform result is claimed. Raw LAN records remain
private. Full #631 discovery/enrollment, #664 convergence, remaining native
platform and release acceptance remain open. No installed service, OS/network
policy or shared-storage state changed.
