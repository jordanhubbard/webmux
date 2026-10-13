# Host session manager

The **Host sessions** workspace connects to a Session Manager v1 service. It is independent of any mobile application and does not replace terminal, desktop or existing agent/tmux views. A provider owns session execution; WebMux supplies the browser client and an authenticated backend adapter.

Configure the backend environment variable `SESSION_MANAGER_SOCKET` with an absolute owner-only Unix socket path. This feature requires WebMux secure mode and exactly one authenticated administrator. Unauthenticated/trusted-network mode and multi-user configurations cannot access host sessions. This initial transport is supported on Unix hosts; Windows reports it unavailable.

The socket and its immediate parent must belong to the WebMux process user with no group/other permissions. Symlink paths are rejected. The configured service must be trusted to act as that local user. The browser cannot select a socket path, redirect the adapter, access arbitrary local URLs or obtain provider credentials. Every request is rechecked for authentication, single-user authorization and same origin.

The view lists live sessions and optional saved history, current state, bounded recent progress, pending approvals/questions and registered client attachments. Search title, folder or ID. Connect opens the existing conversation; Disconnect leaves its work running. Drafts survive pane switches. A lost response preserves the draft and blocks further changes until a successful relevant refresh and explicit user acknowledgment; requests are never automatically replayed.

**Inspect permissions** shows configured access, approval policy and named profile. Selecting a replacement preset deliberately replaces custom settings. Changes apply to subsequent turns, not a command already waiting for approval. Existing approval requests remain separate and show their details and offered decisions. Creation accepts explicit permissions, and shared creation defaults can be read/saved without altering existing sessions.

## Provider contract

The backend forwards only `POST /v1/session-manager` over the configured Unix socket. Browser requests use the existing authenticated `POST /api/session-manager` endpoint. The backend delegates a derived user identity and the label WebMux through `X-Session-Manager-Principal` and `X-Session-Manager-Label`; it never forwards browser Authorization credentials.

Requests have `{version:1,operation,params,clientId,requestId}`. Responses have `{version:1,hostId,providerId,generation,result}`. Errors use non-2xx HTTP status and `{error:{message}}` on the service contract (translated to WebMux's ordinary error shape). Bodies are capped at 128 KiB and responses at 8 MiB, with a 65-second deadline and no redirects or automatic retry.

Operations: `list`, `read`, `attach`, `detach`, `permissions.read`, `permissions.update`, `defaults.read`, `defaults.update`, `create`, `send`, `stop`, `respond`. Session identity is host/provider/session, not folder or terminal title. Permission updates carry a revision from a fresh read; send/stop carry the observed turn ID. Providers must check current ownership, generation and authorization before writes. `detach` must never interrupt, archive or delete work.

The initial Codex provider covers the current user's shared daemon. Other user accounts, isolated runtimes and remote hosts require additional providers. A manager attachment is distinct from a physical terminal/tmux pane association; this integration does not guess those associations from process titles. Provider coverage is displayed in the UI.

Tests cover local socket validation, secure single-user authorization, CSRF rejection, generic forwarding, approval details, permission replacement and client uncertainty/draft lifecycle. This feature does not install or start a manager, change Codex policies globally, or restart running sessions.
