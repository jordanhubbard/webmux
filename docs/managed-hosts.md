# Managed hosts with webmux-client

`webmux-client` is an optional, user-managed program for a machine you access
through WebMux. It connects **outward** to the WebMux server. The server never
starts it, contacts its machine directly, or opens an inbound port there. An SSH
terminal works normally without a client. Client enrollment requires WebMux's
local user authentication mode; the Host control appears on SSH tiles when that
mode is enabled.

## Connect a host

1. On a macOS or Linux source checkout, run `make install`. It builds only
   `webmux-client` and installs it to `~/.local/bin` (override with
   `CLIENT_BINDIR=/your/bin`). Add that directory to PATH if needed. Or copy
   `bin/webmux-client` (or `bin/webmux-client.exe`) from a client-only native
   bundle to a directory on PATH. The combined bundle also includes the client.
   None of these installation methods starts or enrolls it.
2. On that machine, run `webmux-client --server https://WEBMUX_HOST:8443
   --background`. On first use, it prompts for a WebMux username and password
   in the terminal. It stores a private, revocable client credential and prints
   a one-time pairing code. Do not put a password, browser token or client
   credential on the command line. If the server is on an explicitly trusted
   cleartext network, `--allow-insecure-http` permits `http://`; HTTP exposes
   login and client credentials to that network.
   A bare `WEBMUX_HOST:8443` also works and uses HTTPS.
3. In WebMux, open an SSH tile for that machine and click **Host**. If the
   connection is not yet saved, click **Save this host**. Enter the pairing code
   under **Link client**. The binding belongs to the authenticated WebMux user.
   If the browser uses `localhost`, replace the displayed WebMux URL with the
   server address reachable from the remote machine before copying the command.
   Other SSH tiles opened with that saved host ID show the same managed-host
   status. Source IP addresses and editable terminal titles are never used to
   decide identity.
4. The Host dialog shows the client's reported OS, architecture, CPU count,
   optional one-minute load and last check-in. **Refresh host facts** asks the
   connected client over its outbound channel. **Unlink host** preserves the
   client registration; **Revoke client** closes its connection and invalidates
   its stored credential.

The first pairing code expires after 15 minutes. On the remote host, run
`webmux-client --server https://WEBMUX_HOST:8443 --pair-code` to print a fresh
one from the saved credential. Use the same `--allow-insecure-http` option if
the initial connection used HTTP. The credential lives in the user's config
directory in a private `webmux-client/<server-hash>.json` file. Protect that
account and file; if it is copied elsewhere, revoke the client in WebMux.

The client reconnects after transport loss. `--background` detaches it from the
launching terminal and writes errors beside its identity file. For sustained
operation after reboot, the user may instead run it under their chosen launchd,
systemd or Windows service manager. Shell startup files can spawn duplicate
instances; use a user service if automatic startup is wanted.

## Relationship to Rig

The managed-host channel is general WebMux infrastructure. Its first capability
is reporting host facts. Rig enrollment and project/seat observation are separate
capabilities to add over the same channel. Installing this client does not
install Rig, start a Rig daemon, enroll an arbitrary running agent, or make
independent projects appear in the central Rig. Native Windows supports the
WebMux client, while OpenRig currently runs on Windows through WSL2.

The protocol is versioned. The server accepts one current connection for each
client ID, stores only a hash of its long-lived credential, and uses the
client-initiated WebSocket for bounded requests. The client reports its facts;
they are observations from that machine, not proof of provider readiness.
