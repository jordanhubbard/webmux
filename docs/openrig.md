# OpenRig dashboard prototype

WebMux 1.3.23 adds an optional **OpenRig** workspace beside Terminals, Desktops,
and configured agent workspaces. It shows an existing OpenRig shared dashboard
through WebMux's terminal and WebSocket connection. SSH connections, tmux
discovery, and scratch shells keep their existing behavior.

## Use it

1. On the Linux or macOS host running WebMux, set up OpenRig separately using its
   [upstream guide](https://github.com/mvschwarz/openrig). Run it as the same OS
   user as the WebMux service. Make `rig`, its supported Node.js runtime, and
   `tmux` available in the service's `PATH`. A service does not necessarily inherit
   your interactive shell's PATH or OpenRig environment.
2. Start your OpenRig instance and shared dashboard there. Verify that
   `rig tui --shared` attaches successfully from that user's environment.
3. In WebMux, select **OpenRig**, then **Connect dashboard**. In authenticated
   mode this host-level dashboard is available to administrators only.
4. Use **Pop out** for a separate browser window. The pop-out uses WebMux's
   normal login and the same dashboard attachment. If blocked, allow pop-ups for
   WebMux. You can also select OpenRig as the default workspace in Settings.

There is no OpenRig dependency in WebMux's package, no automatic setup, and no
automatic daemon or agent launch. Merely selecting the panel checks availability.
OpenRig setup can change tmux and provider configuration; review its upstream
setup plan before applying it. WebMux does not apply that plan.

## Lifetime and input

One attachment is reused for each WebMux user, including across browser windows.
Switching workspaces, reloading, or closing a pop-out leaves the attachment and
OpenRig running. **Detach dashboard** stops only WebMux's `rig tui --shared`
client and removes its WebMux session. It does not run `rig down`, stop a daemon,
or kill the underlying OpenRig tmux session. Other views of that attachment will
disconnect; reconnect them when needed.

After restarting WebMux, explicitly reconnect the dashboard. It is not launched
during server startup. The dashboard does not occupy a terminal-grid tile and
does not receive or send **Type to All** broadcasts. Multiple views share one
PTY, so their dimensions and focus use WebMux's normal shared-viewer behavior.

## Scope

The dashboard provides status and navigation. Operator and agent conversations
remain in their own terminals. OpenRig handles identities, messaging, and shared
work; placing independent CLI agents in WebMux does not register them with
OpenRig. Launch or attach agents using OpenRig's supported workflow before
expecting them to coordinate.

This prototype supports an existing instance on the WebMux host. To view agents
on another machine, run WebMux alongside OpenRig there and open that WebMux
instance. A remote-host picker, automatic enrollment of existing agent sessions,
and a native Windows OpenRig runtime are outside this prototype.

## Validation

Broker tests cover concurrent attachment reuse, ownership, grid separation,
detach, and explicit reconnect after server restart. API tests cover administrator
access and missing installations. Frontend tests cover prerequisite guidance,
attachment errors, pop-out behavior, and broadcast isolation.

The optional browser acceptance test uses the published OpenRig CLI with a real
PTY and private tmux server. A controlled daemon projection supplies the shared
terminal binding; no provider login or autonomous agent is started. It qualifies
the dashboard attachment, keyboard input, shared pop-out, and detach lifetime,
not agent-to-agent messaging or a complete live OpenRig deployment. To run it:

```sh
# Use a supported Node 24 runtime. Install the CLI into an isolated test prefix.
npm install --prefix /tmp/webmux-openrig-test @openrig/cli@0.6.14
cd webmux
WEBMUX_REAL_OPENRIG=1 \
WEBMUX_OPENRIG_CLI=/tmp/webmux-openrig-test/node_modules/@openrig/cli/dist/bin-wrapper.js \
  npm run test:e2e -- openrig.spec.ts
```
