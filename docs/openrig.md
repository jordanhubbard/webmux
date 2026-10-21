# OpenRig workspace

WebMux 1.3.23 provides the actual OpenRig text UI and interactive terminals for
OpenRig agents in a selectable workspace. Every view can be popped out into its
own browser window. Normal SSH connections, tmux discovery, scratch shells and
existing agent workspaces retain their behavior.

## Host setup

Configure OpenRig on the Linux or macOS host running WebMux, as the same OS user.
Follow the [upstream guide](https://github.com/mvschwarz/openrig). The service's
PATH must include `rig`, a Node runtime supported by OpenRig, and `tmux`. Its
OpenRig instance and tmux environment must match those used to launch the agents.
On macOS, a service does not necessarily inherit an interactive shell's PATH.

Start the OpenRig daemon separately and verify `rig tui` works in that environment.
A kernel team is optional: the WebMux dashboard runs standalone `rig tui`, without
requiring the separate `rig tui --shared` kernel terminal. WebMux shares its own
PTY between browser views.

WebMux does not install OpenRig or apply its setup plan. Selecting the workspace,
checking status, connecting the dashboard and listing/attaching existing agents
do not start teams. Existing independent CLI sessions need OpenRig registration
and provider integration before they can participate in its messaging.

## Use the real dashboard and agent terminals

1. Select **OpenRig**, then **Connect dashboard**. Authenticated deployments
   require a WebMux administrator; trusted mode follows host Settings access.
2. Use the dashboard's keyboard navigation to inspect topology, agents and feed.
   This is OpenRig's own UI, rendered through WebMux's terminal/WebSocket path.
3. Select a daemon-listed agent tab, then **Connect** to attach its interactive
   tmux CLI. **Check again** refreshes the list. Arbitrary commands and targets
   are not accepted: the server verifies agent targets against OpenRig's list.
4. **Pop out** the dashboard and each desired CLI. The selected view is preserved
   in the URL and uses the normal WebMux login. Allow pop-ups for the site.

One viewer PTY is reused per WebMux owner and selected view, across windows.
Closing a window or switching panels leaves the viewer and agent running.
**Detach** closes only that viewer client. Detaching a CLI does not kill its
underlying agent tmux session. After restarting WebMux, explicitly reconnect each
view. These terminals do not occupy normal grid tiles and are isolated from
**Type to All**. Shared viewers use normal WebMux sizing and focus behavior.

The dashboard's own commands can perform OpenRig operations. Provider desktop
window integrations remain upstream functionality; the WebMux agent tabs give
you browser terminals directly, without requiring herdr or another desktop UI.

## Owner-visible acceptance on Puck

The session owner can test this from WebMux after relaunch:

1. Connect **Dashboard** and pop it out. Keep the actual OpenRig UI visible.
2. Choose **Run live test**. This explicitly starts a named Claude Code / Codex
   pair using the host's configured provider accounts. It uses provider quota.
   The team is namespaced by WebMux owner. A preconfigured daemon and working
   provider authentication are required; no login or sandbox policy is changed.
3. Connect the **Claude** and **Codex** views and pop them out beside the dashboard.
   Confirm both are real interactive CLIs and that the pair appears in OpenRig.
4. Watch the fresh challenge arrive in Claude, Claude send it through `rig send`
   to Codex, Codex generate a new nonce and send its response, and Claude send
   the matching ACK. Codex verifies that ACK before writing its result.
5. **PASS** appears only when both result files match the current run, the fresh
   challenge and independently computed SHA-256 digest. Expand the result to
   inspect the run ID, both nonces and digest. The console and native CLI
   transcripts remain available; this is not a canned transcript replay.
6. **Run fresh challenge** repeats the protocol with a new run and nonce. The
   previous PASS is cleared before launching the new test.
7. After PASS, the pair remains running. In either CLI, ask it to send a new,
   owner-chosen message to its peer through OpenRig and have that peer reply.
   Confirm receipt in both terminals; inspect the team and activity in the TUI.
   This checks interactive coordination beyond the bounded test.
8. Close one pop-out and reopen it; the agent should remain available. Switch to
   **Terminals** and confirm an existing SSH/tmux session is still usable.
9. Choose **Stop test agents** when finished. This targets only the named test
   team. Other OpenRig teams, the daemon, dashboard and ordinary sessions remain.

Authentication, provider permission and daemon networking failures are visible in
those same CLI windows and the test console. They never produce PASS. For Codex,
workspace-write mode must permit the local daemon connection; WebMux does not
silently relax the provider's permissions.

Test status, current results, captured CLI transcripts and per-run results live
under `WEBMUX_HOME/data/openrig-proof/<owner-hash>/`. This hash scopes storage and
team names; OpenRig is still a host-level administrator feature, not isolation
between OS accounts. The live console is an explicit acceptance aid, while the
dashboard and existing-agent terminals are useful independently of it.

This prototype supports OpenRig on the WebMux host. For another machine, run
WebMux alongside OpenRig there. Remote host selection and native Windows OpenRig
are outside this prototype.

## Validation

Broker/API tests cover view reuse, owner and grid separation, administrator
revocation, exact daemon-listed tmux targets, detach and explicit reconnect.
Frontend tests cover independent CLI views, pop-outs, explicit test start/stop,
and invalidation of a previous PASS. Runner unit fixtures check fresh challenges,
rejection of tampered/stale results and shutdown scope; they are not live-agent
qualification.

The optional browser test starts the published OpenRig daemon without a kernel
or provider credentials. It renders the actual text UI, navigates it by keyboard,
shares its PTY with a pop-out and verifies that detaching preserves the daemon:

```sh
npm install --prefix /tmp/webmux-openrig-test @openrig/cli@0.6.14
cd webmux
# Run with a supported Node 24 runtime.
WEBMUX_REAL_OPENRIG=1 \
WEBMUX_OPENRIG_CLI=/tmp/webmux-openrig-test/node_modules/@openrig/cli/dist/bin-wrapper.js \
  npm run test:e2e -- openrig.spec.ts
```

The separate [live coordination record](validation/openrig-live-2026-10-21/README.md)
proves actual Claude Code ↔ Codex messaging through the published daemon on Linux,
with native transcripts, transport-attributed daemon outbox rows and an independent
verifier with negative tests. Puck's provider authentication and live exchange
must be verified separately; Linux evidence does not qualify a macOS deployment.
