# Native Rig/Codex installed-service executable upgrade

A later [restricted-policy public recovery qualification](../codex-restricted-recovery-2026-11-18/README.md)
covers read-only/on-request and workspace-write/on-request controller
restart/crash cases. This installed-service receipt retains its default-policy
scope; it does not qualify restricted-policy service upgrades.

The actual Go application and Chrome pass a functional server-executable upgrade
through the shipped macOS Make installer and real launchd. The private service,
app bundle, state, loopback listener, Rig daemon and tmux server are isolated
from the user's running WebMux service. Its PID is unchanged after cleanup.
The native Codex account stays in its original home; no authentication files
are copied. Source bindings, binary digests and checks are in
[receipt.json](receipt.json).

The baseline is the retained capture-capable qualification binary from the
[earlier capture proof](../codex-native-capture-2026-11-18/README.md).
It was built from then-uncommitted source and self-reports that older build
identity. Nine emitted native helper sources independently match its recorded
source revision. The replacement is a clean build of the receipt's current
source revision and adds public original-conversation recovery. Both report
product version 1.4.5; this is a functional executable upgrade within the
candidate, not a released-version migration.

The browser creates one native-owned Codex session, handles a discarded native
creation reply through original-operation inspection, and completes actual
coding. The baseline has no public native recovery contract. The shipped
installer then atomically replaces the private app and replaces its launchd
job. Both signatures verify, executable payloads match their respective inputs,
and the registered and running paths identify the private app. The two signed
executable hashes and server PIDs differ. Signing metadata is normalized only
on private comparison copies as specified in the receipt.

The native provider PID, pane and viewer survive installation. The new server
captures that same original conversation and its public survivor recovery
reattaches without a native Resume POST. The qualifier then kills only its
owned Codex process and explicitly reviews and confirms Stop for the resulting
idle seat; its sibling survives. This explicit idle-seat Stop remains part of
the qualified flow.

After the challenge file is removed, public original-only recovery reaches a
gate just before native dispatch. The qualifier SIGKILLs only the private
installed server. Launchd starts its replacement; the driver does not manually
restart it or reinstall. An ordinary duplicate remains uncertain and sends no
native POST. The browser reconstructs and explicitly completes the same native
request UUID/body/revision, then verifies consumed duplicates have no effect.

The original conversation recalls the removed challenge without tools.
Fresh correlated tool outputs verify the original native identity, model,
default full-access/never policy and owner environment, including four allowed
writes and absence of manager-only values. Native history and the same terminal
log paths retain their original byte prefixes and append. Final checked Stop
retains the viewer and sibling. The private launchd service is removed, the Rig
daemon reports clean completed shutdown with its original PID absent, and the
private tmux server stops.

Earlier failed probe attempts remain retained and excluded. These include a
signing-metadata comparison error, installer tool-path precedence selecting an
older Rig, and native-file scanner errors. They do not establish upgrade or
recovery acceptance. The final probe explicitly selects the private installer
tool directory; supported runtime and native ownership checks remain unchanged.

This covers the stated macOS development-signed service path. It does not
qualify a native Rig/Codex host reboot, restricted policies, selected profiles
or permission graphs, other platforms/service accounts, or notarized release
package migration. The separate Windows standalone and installed-service
qualification keeps its own scope. #318, #438, required review/current-head CI
and the full release roadmap remain open. Private driver sources, raw records
and excluded attempts are retained by hash; account details and model turns
are excluded from these public records.
