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
2. In WebMux's Managed host dialog, copy the displayed client command. It uses
   the browser's current origin by default; edit **WebMux URL reachable from
   this host** when the client reaches the server through another address. On
   that machine, run the copied command. On first use, it prompts for a WebMux
   username and password in the terminal. It stores a private, revocable client
   credential and prints a one-time pairing code. Do not put a password,
   browser token or client credential on the command line. For HTTPS, the URL's
   hostname must be covered by the server certificate. If the server is on an
   explicitly trusted cleartext network, `--allow-insecure-http` permits
   `http://`; HTTP exposes login and client credentials to that network.
   A bare `WEBMUX_HOST` discovers the server's advertised port. On the local
   network, WebMux advertises its active HTTP and HTTPS listeners with multicast
   DNS. The client prefers HTTPS unless `--allow-insecure-http` is passed. For
   routed hosts, publish a DNS SRV record
   at `_webmuxs._tcp.WEBMUX_HOST` for HTTPS or `_webmux._tcp.WEBMUX_HOST` for
   HTTP. The SRV target must be `WEBMUX_HOST`; HTTPS still validates that
   hostname against a certificate trusted by the client. Without an
   advertisement, the client tries the older
   default HTTPS port 8443, or HTTP port 8080 when `--allow-insecure-http` is
   explicitly passed. An explicit URL or `WEBMUX_HOST:PORT` bypasses discovery.
   Linux clients resolve `.local` server names with multicast DNS even in the
   static native build. The client and server must be on a network where mDNS
   multicast is reachable; use a routable DNS name when crossing network
   boundaries. TLS verification continues to use the original server name.
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
is reporting host facts. Optional canonical Rig observation uses the same
channel, with its own versioned report. Installing this client does not
install a coordinator, start a daemon, enroll an arbitrary running agent, or
make independent projects appear centrally. Run `make install-agent-deck`
separately on macOS/Linux execution hosts that Agent Deck will manage. Native
Windows supports the WebMux client, while Agent Deck and OpenRig currently run
on Windows through WSL2.

The protocol is versioned. The server accepts one current connection for each
client ID, stores only a hash of its long-lived credential, and uses the
client-initiated WebSocket for bounded requests. The client reports its facts;
they are observations from that machine, not proof of provider readiness.


## Observe existing Rig seats

After enrolling and linking the client to its saved SSH host, explicitly opt in:

```sh
webmux-client --server https://WEBMUX_HOST:8443 --observe-rig
```

`--background --observe-rig` retains that choice in the background worker's
arguments. Ordinary clients continue to report only host facts. Rig observation
uses the host's installed CLI and existing local daemon; it never starts Rig,
launches a provider, adopts a session or changes host selection. Rig 0.6.14's
`ps --nodes -A --no-cleanup` contract is required. Missing, unsupported or failed
CLI observations report unavailable without blocking ordinary host facts.

Enable the Rig integration in Coordination to see the linked host's canonical
projects, pods and seats. Rig and logical-seat IDs are scoped to the enrolled
client identity, so identical names on two hosts remain separate. The host
binding, rather than a reported hostname, supplies the SSH destination. The
controller does not need a local Rig installation to display these reports.

Open session creates an SSH viewer of the reported existing tmux target. It
uses the saved host's username, port and optional SSH key, requires existing
host trust and noninteractive key/agent authentication, and never runs a
reported shell command or starts a missing tmux session. Password-only hosts
can still be opened as ordinary SSH terminals. Remote topology is read-only
here; create, stop, adopt and recover conversations through Rig on that host.

Reports use `webmux-rig-observation/v1`, with a maximum of 256 seats and 128 KiB
per observation, a three-second CLI deadline and no transcripts, prompts,
messages or credentials. The server timestamps successful observations. After
50 seconds without a new successful report, loss of the client, or a failed
Rig observation, existing seats remain visibly labeled stale, offline or
unavailable with their last observed lifecycle and timestamp. New attachments
are disabled until a current report supplies an attachment target. Revoking a
client or unlinking/deleting its saved host removes its inventory authority;
existing viewers are not provider-stop commands.


## Remote Rig qualification

The bounded observer has ordinary Go tests, authenticated socket lifecycle
controls, browser controls for read-only/offline inventory, and an observation
check inside the real published-Rig/tmux browser fixture. These are distinct
from actual SSH viewer qualification.

`scripts/qualify-remote-rig.mts` runs the complete path against an already owned,
disposable Linux QEMU guest. It requires the guest's `launch.json` (live PID,
VM identity, noninteractive loopback-forwarded SSH with a private known-hosts
file), a prepared shell-seat fixture, native server/client binaries built at
one known revision, Chrome, and a new output directory. It never uses the
controller's installed Rig or starts a Rig daemon. It creates an isolated,
authenticated controller, enrolls its own client, and transfers its disposable
client credential over the existing SSH connection. It uploads the client only
to the prepared fixture directory and verifies its digest before execution.

The prepared guest fixture is intentionally explicit: its directory is
`/home/<user>/qualification/rig-200/run-<hex>`, it has a private Rig instance and
`bin/rig` wrapper, and its existing `wm-remote200-<hex>` tmux seat is enrolled
through `rig attach --self` in a one-seat Rig. Its shell prints
`REMOTE-SEAT-READY`, then echoes each complete input line with
`printf 'REMOTE-SEAT:%s\n' "$line"`. This is a shell transport fixture, not a
coding agent or an authenticated coding-conversation claim. The input JSON has
`contract: "webmux-remote-rig-fixture/v1"`, `root`, `rig_id`, `tmux`, the absolute
`node` path, and `version` from the installed Rig CLI. Rig is started only by
explicit fixture preparation, separately from observation.

```sh
node scripts/qualify-remote-rig.mts \
  --vm-root /path/to/owned-vm \
  --guest-fixture /path/to/guest-fixture.json \
  --server-binary /path/to/native-controller \
  --client-binary /path/to/linux-arm64-client \
  --chromium /path/to/chrome \
  --expected-revision <full-commit> \
  --output /path/to/new-evidence-directory
```

The command checks browser-rendered SSH text and keyboard output, two browser
windows sharing one viewer, loss of the actual client process, preservation of
the canonical Rig lifecycle, reconnect, controller restart with stable identity
and coordinates, a fresh post-restart response, revocation without stopping
the provider, and ordinary SSH with Rig disabled. It records payload digests,
producer/build observations, browser/guest versions and each acceptance phase.
It terminates its controller, tunnels and client processes; the prepared Rig
seat stays running for independent inspection and explicit fixture teardown.
Keep the output directory private because it contains disposable controller
configuration. Never publish its authentication files as evidence.

The [2026-11-17 qualification receipt](validation/remote-rig-2026-11-17/receipt.json)
records the successful run at runtime revision `74d0f2d`: macOS ARM64 controller,
actual Chrome, an owned Linux ARM64 guest, published Rig 0.6.14 and tmux 3.2a.
The same remote pane PID survived every phase. The retained canonical lifecycle
was `attention_required`, and going offline preserved it. This qualifies the
read/attach path against that existing tmux seat; WebMux's separate local Rig
launcher still requires its documented tmux minimum. It does not qualify
coding-session resume, provider credentials, every SSH configuration or Windows
execution hosts. Those boundaries remain under #318 and the broader roadmap.
