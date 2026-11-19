# Packaged private service upgrade and original conversation recovery

Two clean native macOS ARM64 packaged server executables, canonical `6156a5a`
and policy guard `4b0d4b0`, were installed in sequence through the actual Make
installer and a private launchd service. Both identify as product 1.4.5. The
[receipt](receipt.json) binds the packaging runs, archive and executable hashes,
35 source files and all 18 emitted helper sources for each input.

Independent native-journal inspection passes 19 lifecycle controls. The original
Codex 0.169.0 conversation and Rig 0.7.18 node survive the executable upgrade.
Public survivor reattachment sends no Resume. After killing only the owned coding
process, a public original-conversation Resume is interrupted by an actual service
SIGKILL before native dispatch. Launchd automatically starts a replacement service;
ordinary duplicates send no new POST, while deliberate completion keeps the same
frozen request UUID and body. Recall returns the earlier nonce without tools.

Two separate actual native tool results, before upgrade and after recovery,
each preserve workspace-write/never, two allowed writes, four denied writes,
denied networking and the original owner environment. The native conversation
file keeps its prefix and appends. The same terminal log paths retain their
prefixes and append. Checked Stop preserves the viewer and sibling provider.
The private service and tmux processes are stopped; the external user service
state is unchanged.

The first attempt is rejected despite its browser's apparent success: the native
journal has only the baseline tool result, and its terminal transcript retains
the later prompt as an unsubmitted draft. Its cause is unproven. The corrected
harness keeps the original deadlines, zero-delay typing and single Enter, but
measures the original 750 ms pause after the complete text is visible at the native
recipient. It records input control frames and requires a correlated actual tool
result and authoritative completed assistant turn. All three submissions emit one
CR each; there is no automatic input retry. Both raw attempts and driver versions
are retained separately with NAS checksum readback. This does not establish the
cause of the first attempt.

The installer uses verified development ad hoc signatures. Payload comparison
removes signatures and normalizes only Mach-O __LINKEDIT virtual size on private
comparison copies; original inputs and installed images remain intact. This is
candidate packaged executable upgrade evidence, not a notarized released-version
migration. Actual coding host reboot, other platforms and policies, broader
roadmap acceptance and release remain open. No live user service, credential,
version, tag or publication is changed.

Refs #318, #438, #452, #539.

A second independent case on the same immutable packaged inputs passes the same
19 controls under **read-only/on-request**. Both actual tool outputs deny all six
writes and networking; original conversation recall without tools, owner
identity/environment, service replacement/SIGKILL/automatic restart, history/log
prefixes, duplicates, siblings and cleanup remain verified. The
[read-only receipt](readonly-receipt.json) records that case and all three raw
archive hashes/NAS readbacks, including the rejected original attempt. Each of
its three prompts has one observed CR after recipient readiness. This extends
policy coverage on source4b; it is not fresh native qualification of the later
Windows lifecycle composition.
