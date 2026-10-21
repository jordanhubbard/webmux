# Puck owner-visible OpenRig qualification, 2026-10-21

The separate candidate WebMux process on macOS/ARM64 rendered the published
OpenRig 0.6.14 standalone text UI through its real PTY/WebSocket path. It used a
real local OpenRig daemon, not the earlier controlled dashboard projection. The
[dashboard capture](dashboard.png) shows its topology containing the named test
team. Both native agent sessions were created by OpenRig and were accessible in
WebMux's Claude/Codex views. Native Codex 0.163.0 successfully executed `rig whoami`
and reported READY. Puck's installed WebMux launch agent was not restarted.

The fresh exchange on Puck did **not** pass: Claude Code reported
`OAuth token revoked · Please run /login`. Its login status metadata alone did
not establish valid provider authentication. The browser's Claude view exposes
that error directly, and the owner was asked to complete `/login` there. No
pre-recorded Linux result is substituted for a Puck PASS. The candidate is on
Puck loopback port 18083 while that authentication prerequisite is resolved.

OpenRig has a separate supported, self-contained Node 22.23.3 runtime and private
provider/config home. Puck's default Node 26 runtime was verified working after
restoring a Homebrew `simdutf` link changed during the initial installation. The
final OpenRig wrapper uses the standalone native runtime, whose dependencies are
system libraries, so it needs no alternate Homebrew library path. Normal provider
configuration and the existing WebMux service configuration were preserved.

The owner-facing scenario and test controls are documented in
[the OpenRig guide](../../openrig.md#owner-visible-acceptance-on-puck). Complete
that live exchange and an owner-chosen follow-up message in the actual CLIs
before claiming macOS messaging qualification or relaunching the installed app.
