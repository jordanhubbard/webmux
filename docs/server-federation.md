# Connected WebMux servers

Run WebMux on each host. Each server owns its terminals, processes, session IDs,
project bindings and persistence. A browser connected to one server can select
**This server**, one configured server, or **All servers**. The default local
workspace keeps its existing terminal grid. The server selector appears only
when an administrator has configured at least one peer.

Connections are opened on demand. There is no distributed mesh, shared home
directory, automatic discovery, or requirement for Rig or Agent Deck. Those
coordinators remain optional for task assignment and provider lifecycle work.

## Configure a pair

Use the same WebMux release on both servers. Give each server a distinct
`WEBMUX_HOME`; never run two servers against the same data directory.

Generate a random credential, for example with `openssl rand -hex 32`. Transfer it
privately to both servers and set it in their service environments. Do not put
credentials in a URL, browser storage, shell history, Git, or the frontend app
configuration. Environment values must contain at least 32 characters.

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

Choose a server in the toolbar. **All servers** groups terminal tiles under
server labels. Each tile shows its server, durable project/session label,
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
