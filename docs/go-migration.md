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

The Go server currently implements health and authentication/account routes,
Argon2id PHC password storage, HS256 JWTs, one-use WebSocket ticket storage,
per-client rate limiting, HTTP/TLS listeners and graceful HTTP shutdown. It uses
the existing `WEBMUX_HOME/config` and `data/events` layout. Authentication config
updates are serialized and atomically replaced, preserve unknown fields and
follow configuration symlinks. Other application routes, static UI serving and
WebSocket/session transports are not implemented yet.

Run from `webmux/server` with Go 1.26 or later:

```sh
go test -race ./...
go vet ./...
go run ./cmd/webmux --root .. --home /tmp/webmux-go-dev --listen 127.0.0.1:18080
```

From `webmux`, `npm run test:contract:auth` builds both servers, runs the same
authentication expectations against each, then restarts the opposite server
against the fixture's persisted credentials. This verifies password hashes and
tokens across both migration directions. Fixtures use temporary homes and never
connect to real terminal or desktop hosts. `npm run typecheck` checks the
TypeScript contract harness as well as the application.

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

These are intentional changes, not claims of byte-for-byte error compatibility.
