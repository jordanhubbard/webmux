# Connected WebMux servers

Run WebMux on each host. Each server owns its terminals, processes, session IDs,
project bindings and persistence. A browser connected to one server can select
**This server**, one configured server, or **All servers**. The default local
workspace keeps its existing terminal grid. Administrators can open **Manage
servers** from the toolbar even before any peers exist. The server selector appears after the first peer is configured.

Terminal connections are opened on demand. Authorized coordination summaries refresh in the server without a browser. [Network discovery](network-discovery.md)
shows unverified candidate nodes in Manage servers; authenticated connections
still require explicit configuration. There is no distributed mesh or shared
home directory. Rig and Agent Deck remain optional for task assignment and
provider lifecycle work.

## Manage connections in the UI

Use the **Cluster** controls in the main toolbar: **Add server** opens a new connection form, **Manage servers** lists connections for editing or removal, and **Refresh servers** reloads the directory and selected viewers. These controls are available before the first peer is configured.

Open **Add server** on the controller. Enter a
stable ID, display name, and the other WebMux server's origin URL. Use HTTPS;
allow HTTP only on an already trusted private network or tunnel. Enter a shared
credential of at least 32 printable ASCII characters without spaces, or use
**Generate credential** and transfer it privately.

On the receiving server, open **Manage servers → Add incoming grant**. Enter an
ID, an existing local account as the session owner (`anonymous` in trusted mode),
and the same credential. A grant gives that trusted server access to this owner's
terminals, which run as the WebMux service user. Choose **Test connection** on the
controller to check the saved configuration. To connect in both directions,
configure another outgoing entry and incoming grant with a separate credential.

**Edit** changes an entry or repairs an unavailable credential. A blank replacement
keeps the existing credential. The environment-variable option preserves existing
service-based deployments; entering a replacement credential switches that entry
to server-managed storage. UI-supplied credentials are stored in the server's private
`$WEBMUX_HOME/config/servers.yaml` (mode 0600 on Unix), never returned through read
APIs, logged, or saved in browser storage. Secure the server's data and backups.
Existing `token_env` configurations remain supported. Saving UI credentials does
not require a service restart. Copy a generated credential before saving: it cannot
be retrieved through the UI afterward.

Use **Refresh servers** to reread the directory, fetch current session lists, and
reattach the selected servers' viewers. **Refresh <server name>** retries one
server. These actions keep the remote terminal processes running. Failed servers
keep their last observed session list with a stale warning. If a credential is
unavailable, open **Manage servers → Edit** and supply its replacement; refreshing
alone cannot repair missing credentials. If the receiving server rejects it,
check the matching incoming grant and its account owner.

**Remove** disconnects a peer's viewers while keeping its providers running.
Removing an incoming grant revokes that peer's access. A receiving server's
policy rejection closes only that server's viewer; it does not expire the
controller login or disconnect local terminals. Restore the grant or repair the
peer credential, then reconnect the viewer to its surviving provider. Both
removal actions require confirmation. These controls are administrator-only in authenticated mode and
available to operators in trusted `auth.mode: none`, like other settings.

## Verify a node identity

After configuring an HTTPS peer and its matching incoming owner grant, select
**Verify identity** in Manage servers. WebMux pins the remote node, receiving
account and TLS key. Pinned HTTP and WebSocket connections prove that original
identity on the same TLS connection before sending a credential. A changed key,
node or account is refused; editing the name/address preserves the pin. Remove
and explicitly enroll again for a deliberate identity change.

When browsing is enabled, a lost enrolled endpoint can migrate to a discovered
address only after the original identity is verified. This keeps the configured
peer/session namespace and its account scope. It never creates another provider.
Manual HTTP peers remain available without identity enrollment or automatic
migration. End-to-end HTTPS and normal OS certificate trust are required; TLS
termination between the peers is not supported for pinned connections. See
[network discovery](network-discovery.md) for the protocol, bounds and current
native qualification limits.

## Share project coordination context

Project summaries have a separate node-level permission. Existing incoming grants
retain their terminal access and do not acquire coordination sharing implicitly.
All eligible sessions for a mapped local account participate automatically; there
is no per-session switch.

1. Configure the HTTPS peer and matching incoming grant, then use **Verify
   identity** to enroll its receiving node/account.
2. On the receiving server, edit the incoming grant and select **Allow project
   coordination summaries** for its existing local owner.
3. On the controller, edit the verified peer and enter an existing local account
   in **Coordination account on this server**. Leave it empty to disable sharing
   toward that account. The local account and receiving account can have different
   names; WebMux retains both identities.

The mapped account's coordination view includes read-only `peers` entries for its
verified projects. They contain that receiving account's matching session/task,
branch/worktree, claim and worker-availability metadata. Private inbox/outbox
messages, admission receipts, capabilities and server credentials are excluded.
Other local accounts cannot borrow the peer credential. Each direction requires
its own mapping and receiving permission; connections do not transitively expose
another node's summaries.

Background refresh uses the pinned, same-connection HTTPS identity proof before
sending the existing server credential. HTTP-only/manual unverified peers remain
available for their terminal contract. The coordination controls require an
enrolled HTTPS identity. Removing a local mapping removes its peer context from
subsequent views immediately. A receiving refusal clears retained data; a network
outage can retain a clearly stale observation until its 30-second expiry. Pending,
invalid, disabled or unavailable context does not mean that remote work is absent.

These permissions authorize project context only. Cross-node claims and
reservations still require separate authority. A remote availability
summary does not acquire that worker. See [session coordination](session-coordination.md)
and the [two-host protocol receipt](validation/coordination-peers-2026-11-21/README.md).

## Allow coordination messages

After enabling summaries, edit the outgoing verified peer and select **Allow
coordination messages to this peer**. On the receiving server edit its incoming
grant, select **Allow incoming coordination messages**, and enter the sending
server's node ID and mapped local account. The management dialog displays this
server's node ID below **Incoming access**. Preserve the distinct sending and
receiving accounts even when their names match. Each direction requires its own
configuration; existing summary and terminal permissions do not enable messages.

The corresponding file fields are `coordination_messages: true` on the outgoing
entry and incoming grant, plus `source_node_id` and `source_owner` on the grant.
The grant still requires `coordination: true`. Messages append only to a live
same-project recipient's coordination inbox. Recipients explicitly record
delivery, acknowledgment and completion. A queued or uncertain transport is not
proof of delivery. Removing permission stops future attempts, while an existing
in-flight admission may already have committed. See the
[durable protocol and recovery rules](session-coordination.md#durable-messages-across-nodes).

## Allow shared worker reservations

For an enrolled HTTPS peer with a local coordination account, select **Allow
shared worker reservations on this peer**. On the receiving incoming grant,
enable summaries and **Allow incoming shared worker reservations**, then bind the
sending node ID/account. This permission is independent of inbox messaging and
defaults off. The file field is `coordination_reservations: true` in both entries;
the incoming grant also needs `coordination`, `source_node_id` and `source_owner`.

Local and remote sessions contend on one authority's heavy-worker lease. A grant
does not execute a job. Workers must verify the current lease and original fence,
honor the conservative expiry and stop before release. Older lease-unaware
clients are explicitly refused. See the
[authority, retry and compatibility contract](session-coordination.md#shared-heavy-worker-reservations).

## Configure a pair through files

Use the same WebMux release on both servers. Give each server a distinct
`WEBMUX_HOME`; never run two servers against the same data directory.

Generate a random credential, for example with `openssl rand -hex 32`. Transfer it
privately to both servers and set it in their service environments. Do not put
credentials in a URL, browser storage, shell history, Git, or the frontend app
configuration. The management dialog accepts credentials transiently and saves
them only in private server configuration. Environment values must contain at least 32 characters.

On the controller, edit `$WEBMUX_HOME/config/servers.yaml`:

```yaml
version: 1
servers:
  - id: build-linux
    name: Linux builder
    url: https://builder.example.net:8443
    token_env: WEBMUX_LINUX_PEER_TOKEN
grants: []
```

On the Linux server, configure the corresponding incoming grant:

```yaml
version: 1
servers: []
grants:
  - id: desktop-controller
    owner: jordan
    token_env: WEBMUX_DESKTOP_GRANT_TOKEN
```

The two environment variables have the **same credential value**. Their names
can differ. `owner` must name an existing account on the receiving server. Only
that owner's sessions are accessible through this grant. This is API ownership,
not an OS sandbox: terminals run as the WebMux service user, just like local
terminals. Grant credentials only to trusted servers and operators. In trusted
`auth.mode: none`, use `owner: anonymous`; the incoming peer still needs its
credential. Incoming credentials do not authenticate to account, configuration,
SSH key, desktop, or coordinator APIs. Keep the credential private: within its
owner scope it permits session creation, terminal interaction and provider restart.

An inbound grant does not create an outbound connection. To browse and send in
the other direction, add a separate server entry and grant with a fresh
credential. Each controller's server directory is its own allowlist.

HTTPS verifies the peer's certificate with the operating system trust store.
Only origin URLs are allowed; userinfo, paths, query strings and fragments are
rejected. For loopback development or an already secured private tunnel only,
an explicit `allow_http: true` enables an HTTP origin. There is no option to
skip TLS verification. Directory and grant IDs must be unique; `local` and `all`
are reserved server IDs. At most 32 peers and 32 grants are accepted.

Configuration files are reread for requests; adding/removing a server does not
require restarting WebMux. Browsers refresh their directory every 15 seconds.
Changing a process environment variable requires restarting that server through
its service manager. Removing a grant or changing its owner revokes new requests
immediately and existing viewers within five seconds. Removing/changing an
outbound entry also closes its existing viewers within five seconds. Existing
providers keep running when viewers close.

## Use the views

The selector offers **Show my windows only**, **Show SERVER windows only**, and
**Show all connected servers’ windows**. The combined view places all terminal
tiles in one responsive 2D matrix. Every tile carries its owning server's name
and directory ID, plus a stable color badge and top border. Session IDs remain
scoped to their owning server, including when two servers use the same ID.

Each tile shows its durable project/session label,
provider state and browser viewer connection separately. Missing project labels
can be set in the owning server's local workspace. Project names are saved
session metadata or coordinator identities, never inferred from mutable cwd.

The selected server view survives a page refresh in that browser tab. Opening a
viewer, refreshing, switching views or using **Reconnect viewer** attaches to the
same session and does not launch another provider. **Hide** removes only that
viewer; **Show hidden viewers** opens it again. Hidden viewers are remembered
until leaving the connected-server workspace. **Restart provider** is a separate,
confirmed action for a stopped/error session. **New terminal** creates a normal
local terminal on the selected server, using its existing local transport
requirements (including tmux where required). Full SSH creation, file transfer,
authentication handoff and desktop/coordinator controls remain in the owning
server's local workspace.

An unavailable server gets its own warning. Previously observed sessions remain
visible with a stale-status warning while the viewer reconnects. Other servers
remain usable. Peer terminal heartbeats close an unresponsive connection within
approximately 15 seconds so the browser can reconnect. Input broadcasting is not offered in the connected-server view;
keyboard input goes to the focused terminal only. Fixed grid-size and lock
controls stay in the local workspace; connected-server terminals fit their tiles.

## Cross-host agent messages

Use **Message** on the intended agent's tile. The recipient is explicit: server
and session. Send one line while the agent is ready to accept a prompt. The
server adds its authenticated source label, submits bracketed-paste terminal
input and Enter, and reports **queued**, never that an agent has processed or
completed the request. This sends terminal input; do not target a shell or an
agent whose current prompt would interpret it as an unintended command or answer.
Messages accept at most 16 KiB and reject control characters and newlines.

An agent can send directly to an allowed receiving server:

```text
POST /api/federation/sessions/SESSION_ID/messages
Authorization: Bearer <credential from the private server environment>
Content-Type: application/json

{"message":"Please review the current change and report your findings."}
```

Discover that grant's sessions with `GET /api/federation/sessions`. A controller
administrator can use its normal browser/API token with
`GET /api/servers`, `GET /api/servers/SERVER_ID/sessions`, and
`POST /api/servers/SERVER_ID/sessions/SESSION_ID/messages`. IDs are scoped to the
owning server. Browser terminal URLs go through the controller at
`/api/servers/SERVER_ID/term/SESSION_ID`; peer credentials never enter the browser.
The version-1 contract consists of these endpoints and the matching
session-create, provider-reconnect and terminal WebSocket paths. Federation does
not proxy arbitrary paths or follow redirects.

A `202` response contains `queued: true` and `processed: false`. A stopped session,
full input queue or revoked credential fails explicitly. Do not automatically
retry a send after an ambiguous network failure: delivery is not idempotent, and
there is no processing receipt, durable inbox or orchestration scheduler. Audit
records retain source, owner and target session, without retaining message text.

## Qualification

The Go federation tests run two HTTP servers with independent configuration and
native PTYs. They cover input/output, resize, viewer reattach without provider
duplication, messages, owner boundaries, bad/missing credentials, grant revocation,
redirect refusal, secret filtering and unavailable peers. The browser test starts
a second WebMux process and exercises real terminals through the combined UI.

```bash
cd webmux/server
go test -race ./internal/httpapi -run Federation -count=1
cd ..
npm run test:e2e -- server-federation.spec.ts
```

These tests qualify the local two-server path. TLS deployment and network access
still depend on the configured hosts and their certificates.

For opt-in native qualification against a separate Linux host accessible by
noninteractive SSH, run from `webmux`:

```bash
npm run qualify:cluster -- SSH_HOST
```

The runner builds this checkout for both hosts, starts private installations on
loopback, and connects them through SSH forwards in both directions. It tests
all three views, the shared matrix, owner isolation, terminal input, targeted
messages, remote terminal creation, resize, viewer reattachment, live grant
revocation, and recovery of the same tmux provider
after a peer server restart. It retains a JSON report, screenshots and logs under
`_build/server-cluster-*`, stops its test processes and private tmux servers,
and leaves the installed services running. The peer needs Python 3 and tmux;
the controller needs the normal build/browser dependencies. This verifies the
secured-tunnel path; it does not qualify a deployment's HTTPS certificates.
