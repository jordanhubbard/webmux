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
and the existing cache headers. Other application routes, static UI serving and
WebSocket/session transports are not implemented yet.

The internal terminal process layer now supports Unix PTYs and Windows ConPTY,
with independent process lifetime, serialized input, resize, exit status,
output draining and idempotent cancellation. SSH/mosh/exec command planning
preserves the existing keepalive, host-key, key-path, shell-template and password
environment behavior. This layer is not connected to session routes yet. Its
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
layout restart fixture has no sessions and therefore no tiles; session-driven
layout reconciliation and recovery remain part of the pending transport work.
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

These are intentional changes, not claims of byte-for-byte error compatibility.
