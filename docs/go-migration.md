# Go server migration

Tracked in [#77](https://github.com/jordanhubbard/webmux/issues/77).

The target architecture is a native Go server and the existing React/TypeScript
browser application. Keep xterm.js, noVNC, Guacamole, DOM structure, styles, and
fonts. WASM is not required. First-party browser and Node tooling sources must be
checked TypeScript; generated JavaScript and upstream dependencies are not
first-party source.

The Node server remains the default until the complete replacement passes the
compatibility gates. The Go implementation lives in `webmux/server`; partial
implementation is not a production replacement. Do not run both implementations
against the same writable `WEBMUX_HOME`.

## Completion requirements

- Preserve HTTP methods, paths, response shapes, status codes, WebSocket framing,
  one-time tickets, ownership and viewer/focus semantics.
- Read existing YAML files, Argon2id password hashes and JWT signing secrets;
  preserve saved hosts, keys, layout, sessions and runtime settings.
- Preserve terminal SSH/mosh/exec transports, PTYs, agent/tmux integration,
  reconnects, resize, scrollback, broadcasting, transcripts and process cleanup.
- Preserve VNC and RDP proxying, network restrictions, credential lifetime and
  desktop session persistence.
- Preserve settings/font hosting, uploads, templates, AI integrations, audit
  logging and configuration reloads.
- Preserve macOS, Linux and Windows service, installation and packaging support.
- Convert maintained JavaScript tooling/configuration to checked TypeScript.
- Run differential HTTP/protocol tests against both servers; test concurrency,
  authorization, malformed input, restart and shutdown behavior on Go.
- Run frontend tests and browser workflows against Go. Establish visual baselines
  with fixed browser, viewport, fonts and scaling; compare unchanged UI states.
- Measure startup, memory and interactive throughput, and verify CI/build/release
  paths before removing the Node server and changing the default commands.

Tests establish parity for the covered cases, not universal equivalence. Any
intentional security improvement must be recorded separately from compatibility
claims. Work remaining stays on #77 until these requirements are verified.

## Implemented foundation

The Go server currently implements health, authentication/account, saved-host,
SSH-key catalog, runtime settings, layout and authenticated font routes,
Argon2id PHC password storage, HS256 JWTs, one-use WebSocket ticket storage,
per-client rate limiting, HTTP/TLS listeners and graceful HTTP shutdown. It uses
the existing `WEBMUX_HOME/config` and `data/events` layout. Authentication config
updates are serialized and atomically replaced, preserve unknown fields and
follow configuration symlinks. Catalog updates preserve legacy missing/null
fields and custom metadata, serialize concurrent changes, and redact key paths
and private metadata from API responses. Settings enforce administrator/trusted
access, preserve nested defaults and disabled agent definitions, and keep
environment overrides out of YAML. Font serving follows the real app.yaml
directory and confines file opens to that root, with range/conditional requests
and the existing cache headers. Terminal session HTTP routes now support
owner-scoped CRUD, patch precedence, reconnect and saved-session recovery. The
broker serializes grid allocation and persistence, rebuilds layout tiles while
preserving layout metadata, and rejects stale output/exit events from replaced
processes. Terminal WebSockets now support ticket/token authentication, atomic
scrollback-to-live delivery, input, resize, viewer presence/focus and deletion.
Transcript logging now follows the configured launch default, supports manual
pause/resume into the same file, and rotates files across launches. It preserves
headers/footers, audit event fields and owner-only POSIX permissions. Other
application routes and static UI serving remain pending.

The internal agent service now normalizes agent configuration independently,
checks the multi-user access policy, discovers tmux sessions, assigns duplicate
display names, resolves pane directories and builds attach argument vectors.
It reads existing base64url-named JSON status files and atomically updates them
while preserving unknown hook metadata. Agent HTTP routes now support config,
discovery, attach and scratch. The broker reuses owner-scoped panes, relaunches
changed attach commands and keeps live scratch shells when their selected cwd
changes. Agent sessions remain outside the terminal grid and startup reconnect.
Access checks cover HTTP, WebSocket joins and reconnect; a one-second policy
poll removes revoked agents and closes viewers with code 1008. Malformed app
configuration blocks new access without deleting recoverable saved records.
Activity writes debounce for 200 ms, suppress output during the first 1.5 seconds
of a launch and flush on shutdown. Status inference preserves waiting slop and
recent/stale boundaries. Tests use isolated homes and fixture processes; they
do not contact operator tmux sockets. Locale-dependent label ordering and
non-ISO legacy timestamps still require differential coverage;
the initial implementation uses English collation and RFC3339 timestamps.

VNC and RDP session HTTP APIs now support owner-scoped create/list/get/move/delete
and reconnect state changes. Separate desktop brokers preserve saved-host port
defaults, per-owner/per-protocol grids, compaction and the legacy desktop YAML
files. Passwords remain in memory, are inaccessible to other owners and disappear
on deletion, shutdown or restart. Recovery marks desktops disconnected without
opening network connections. Transport lifetime notifications are available for
the proxies. VNC now supports authenticated binary WebSocket/TCP forwarding,
session ownership, one-use tickets, destination validation and DNS address
pinning. Private network destinations remain supported; the existing explicit
local-target environment override is preserved. Connections stop on deletion,
shutdown or account revocation. Multiple viewers retain independent connection
state so a closing viewer cannot disconnect another live viewer's session state.
The RDP proxy and Guacamole handshake remain unimplemented. Full browser-based
VNC rendering parity is still unverified.

The internal terminal process layer now supports Unix PTYs and Windows ConPTY,
with independent process lifetime, serialized input, resize, exit status,
output draining and idempotent cancellation. SSH/mosh/exec command planning
preserves the existing keepalive, host-key, key-path, shell-template and password
environment behavior. Session routes now launch through this process layer. Its
tests launch a local fixture in a real terminal and check dimensions, Unicode,
large final output, exit codes and blocked-I/O cleanup. Windows runtime results
must be verified in CI; cross-compilation alone does not establish ConPTY parity.

Run from `webmux/server` with Go 1.26 or later:

```sh
go test -race ./...
go vet ./...
go run ./cmd/webmux --root .. --home /tmp/webmux-go-dev --listen 127.0.0.1:18080
```

From `webmux`, `npm run test:contract` builds both servers, runs the same
authentication, catalog and settings expectations against each, then restarts the
opposite server against persisted credentials, catalogs, settings and font files.
This verifies these formats across both migration directions. The settings-only
layout restart fixture has no sessions and therefore no tiles. A separate session
contract checks ownership, defaults, patch precedence, grid limits, reconnect
failure, layout reconciliation and cross-backend session recovery. These HTTP
fixtures deliberately use failed local exec launches; a real Go PTY test covers
initial-command injection, input, resize, reconnect, Unicode and process exit.
The terminal WebSocket contract also runs a checked TypeScript fixture inside a
real PTY through both servers and compares authentication, tickets, input,
dimensions, Unicode, late-viewer replay, focus, leave and deletion. It also
compares transcript defaults, pause/resume status, excluded paused output,
reconnect rotation, deletion footers and POSIX permissions. The broker tests
cover the replay/live boundary, bounded queues, disk failures, drain races and
shutdown flushing. Transcript filenames retain the legacy pattern; launch
generation numbers are per-runtime identifiers, not persisted sequence numbers.
An isolated Go tmux protocol fixture also runs through both servers to compare
agent discovery, errors, sizing, attach/replacement, interactive output, scratch
cwd/reuse, status metadata and multi-user revocation. It never opens a real tmux
socket. Desktop contracts compare both protocols' defaults, host resolution,
ownership, grid moves/compaction, credential exclusion and cross-backend restart
recovery. A local TCP fixture also verifies VNC authentication, ticket reuse,
binary data in both directions and upstream-close semantics against both servers.
The terminal size contract retains an exact dimension assertion and a
ten-second deadline; its TypeScript fixture opens a fresh terminal handle for
each size query because Node's cached stdout dimensions can remain stale on
Windows with cooked input.
Fixtures use temporary homes and never
connect to real terminal or desktop hosts. `npm run typecheck` checks the
TypeScript contract harness as well as the application.

Packaging scripts, browser test runners, the E2E fixture and agent status helper
now have checked TypeScript sources. ESLint and Jest configurations are
declarative JSON. `npm run build:helpers` produces the installable agent helper
and Windows launcher in `webmux/scripts/dist`; generated JavaScript is ignored
by Git. `npm run build` includes this step, and `npm run test:helpers` exercises
the actual compiled status hook. Inline Node snippets in shell/CI/service files
still need conversion or removal as the Go deployment tooling replaces them.

### Deliberate security differences

- Go rejects tokens for accounts that have been deleted, including refresh and
  ticket requests. Previously issued tickets must also be checked against current
  account/session authorization when WebSocket handlers are implemented.
- JWTs must use HS256 and have an expiry, matching tokens WebMux issues.
- Concurrent bootstrap attempts cannot overwrite the first account. Account
  changes recheck administrator rights in the storage transaction.
- Malformed authentication config fails closed; startup fails if it cannot load
  authentication or persist a new signing secret. There is no ephemeral-secret
  fallback or automatic test signing secret.
- Password verification supports existing Argon2id v19 defaults. Imported hashes
  outside the supported bounds (8–64 byte salt, 16–64 byte digest, 1–16 lanes,
  1–10 passes, memory between 8 KiB per lane and 256 MiB) fail closed. At most two
  password operations run concurrently.
- New state files and replacement configuration files use owner-only permissions
  on POSIX. Windows access control still follows the containing directory's ACL.
- Secure mode requires a usable TLS certificate/key; partial or unreadable TLS
  configuration fails startup instead of silently omitting HTTPS.
- Invalid JSON/types produce bounded JSON errors; oversized bodies receive 413.
- Settings updates validate the effective response before committing, so invalid
  environment limits cannot produce a failed response after saving a change.
- Font file opens use an OS-confined root in addition to canonical-path checks,
  preventing a replaced symlink from escaping the configured directory.
- Terminal launches reject invalid ports, unsupported transports and dimensions
  outside the WebSocket resize limits. Unreadable key/mosh configuration fails
  the launch instead of silently falling back to a different configuration.
- Mosh key paths use complete shell quoting, including spaces and apostrophes.
  Unix terminal descriptors remain pollable so cancellation can release blocked
  I/O; Windows launch failures release pipes, attribute lists and native handles.
- Corrupt or duplicate saved terminal-session records fail startup instead of
  silently becoming an empty session list. Failed session writes roll back
  in-memory CRUD changes and close any process created by the failed request.
- Grid allocation searches occupied cells instead of iterating through the
  configured grid dimensions. Reconnect launch failures are saved as errors so
  disk state agrees with the failed request.
- Terminal WebSockets enforce session ownership and recheck the ticket/token
  subject against current accounts. The old terminal handler only checked that
  credentials were valid. Existing sockets recheck authorization on messages
  and every five seconds, closing when the account or authentication mode changes.
- Browser WebSocket origins must match the request Host; clients without Origin
  remain supported. Reverse proxies must preserve the browser-facing Host. The
  Vite development proxy now does so, without changing UI rendering.
- WebSocket upgrades share the HTTP request rate limit. Messages are limited to
  1 MiB. Each viewer has a 128-event output queue, and each PTY has an input queue
  bounded by 256 messages and 2 MiB of queued data. Slow viewers or excess
  input receive close code 1013 instead of blocking unrelated sessions or growing
  queues without bound. Viewer disconnection does not terminate its PTY.
- Transcript writes run outside the session lock with a queue bounded by 256
  chunks and 4 MiB. A write failure or overflow disables recording, emits an audit
  error and notifies viewers while leaving the terminal running. A socket may
  have at most 16 pending transcript toggles. Pause/resume requests are serialized
  per session, and draining an old log cannot disable a replacement launch's log.
- Transcript opens are confined to the log directory. Resume rejects a substituted
  symlink or non-regular file instead of appending to its target. Disconnected
  processes cannot start a new transcript through a stale process handle.
- Agent discovery commands have a five-second timeout and 1 MiB limit per output
  stream. Status reads are limited to 1 MiB and confined to the agent directory;
  writes use atomic replacement and owner-only POSIX file permissions. The
  configured home and agent directories remain operator-controlled.
- Concurrent attach/scratch requests serialize pane selection. If persisting a
  policy-driven deletion fails, Go still closes viewers and the revoked process,
  retaining the record for a later cleanup retry. Agent status writes occur
  outside the session lock and shutdown waits for their completion.
- Desktop records reject invalid ports and non-finite/negative positions. Failed
  create/move/delete writes roll back in-memory state. Corrupt, duplicate or
  wrong-protocol saved desktop records fail startup rather than being silently
  discarded. Passwords have no fields in the persisted session types.
- VNC upgrades enforce browser Origin/Host agreement and recheck accounts during
  input and every five seconds. Inbound messages are limited to 1 MiB with four
  queued messages; writes have ten-second deadlines. Destination lookup is
  bounded to five seconds and TCP connection establishment to ten seconds.
  Canonical address checks cover the full IPv6 link-local range and mapped IPv4
  forms, extending the old textual address checks. Dialing uses only the checked
  IP address. Tests use local fixtures or stub DNS answers, not remote hosts.

These are intentional changes, not claims of byte-for-byte error compatibility.

### Platform verification

The Linux Go CI job passed at commit 2176490. The Windows run exposed rooted
font-path validation and redirected-standard-handle inheritance failures.
Font validation now rejects leading separators on every platform. ConPTY
startup explicitly supplies null standard handles, following the
[Microsoft terminal maintainers' guidance](https://github.com/microsoft/terminal/discussions/15814),
so child terminal I/O does not inherit redirected server streams. Linux and
Windows Go race tests and vet then passed at 150073e, including native ConPTY
tests. Linux's complete Go and differential job passed at 69d2319. Windows still
failed the Node fixture's cached-size assertion despite ConPTY reporting the new
dimensions. The fresh-handle query above addresses that fixture limitation;
a successful Windows differential run remains required.
The subsequent Windows job at 8b98c85 stopped earlier on a shutdown assertion
that treated a buffered status frame as a live socket. That test now drains
buffered frames and requires an actual WebSocket close within its deadline.
