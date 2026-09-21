# WebMux Architecture

## Overview

WebMux is a browser-based remote workspace built as a React frontend and Go backend. REST manages configuration and session lifecycle; WebSockets carry interactive terminal, VNC, and RDP traffic.

## Component Diagram

```
Browser                                WebMux Host
┌──────────────────────────┐           ┌──────────────────────────────────┐
│ React application        │   HTTP    │ Go HTTP REST API                 │
│ ├ Terminal workspace     │◄─────────►│ ├ auth, users, config, hosts    │
│ │ └ xterm.js tiles       │           │ └ terminal/VNC/RDP/agent state │
│ ├ Desktop workspace      │ WebSocket │                                  │
│ │ ├ VNC client           │◄─────────►│ Session brokers and proxies      │
│ │ └ Guacamole RDP client │           │ ├ PTY/ConPTY: SSH, mosh, exec     │
│ ├ Agent workspace        │           │ ├ VNC WebSocket proxy           │
│ └ Navigation and dialogs │           │ └ guacd RDP proxy               │
└──────────────────────────┘           │                                  │
                                       │ YAML/JSONL persistence           │
                                       └───────────────┬──────────────────┘
                                                       ▼
                                         Remote hosts and tmux sessions
```

## Backend Services

The server lives in `webmux/server/`, with its entry point in `cmd/webmux`.

| Go package | Responsibility |
|------------|----------------|
| `internal/httpapi` | REST routing, authentication middleware, terminal and desktop WebSockets |
| `internal/session` | Session lifecycle, layout, viewer focus, scrollback and transcripts |
| `internal/desktop` | VNC/RDP session lifecycle, ownership and persisted state |
| `internal/terminal` | SSH/mosh/exec launch planning and Unix PTY/Windows ConPTY processes |
| `internal/auth` | Accounts, password verification, tokens and WebSocket tickets |
| `internal/agent` | tmux discovery, access policy and agent status |
| `internal/config`, `internal/storage` | Runtime configuration, atomic state writes and audit logging |
| `internal/guacamole`, `internal/netguard` | RDP protocol handling and validated desktop destinations |

## Frontend Components

| Component | Responsibility |
|-----------|---------------|
| **App** | Auth lifecycle, workspace routing, config loading, top-level state |
| **TopBar** | Workspace navigation, terminal controls, settings gear, auth countdown, account administration |
| **SettingsDialog** | Structured editing for allowlisted runtime configuration |
| **Workspace** | Terminal grid, session CRUD, movement, minimization, themes, lock, and auto-scroll |
| **GraphicsWorkspace** | Shared grid and fullscreen flows for VNC and RDP sessions |
| **AgentWorkspace** | Lists agent tmux sessions and hosts attach and scratch terminals |
| **Tile** | Terminal chrome (title, status, controls), border/focus styling |
| **Terminal** | xterm.js instance, WebSocket connection, resize handling |
| **ConnectionDialog** | Host selection, auth method, transport choice |
| **LoginPage** | Bootstrap and login flows |
| **InputBroadcastContext** | "Type to All" broadcast mode state and routing |

## Data Flow

1. User creates session via ConnectionDialog -> POST `/api/sessions`
2. The session broker resolves the host and starts a PTY through the terminal launcher
3. Frontend opens WebSocket to `/api/term/:id`
4. The session package tracks the viewer and assigns focus
5. Terminal data flows: PTY stdout -> WebSocket -> xterm.js (and reverse for input)
6. When `app.session_logging.enabled` is true, the same PTY output is streamed to a protected, per-launch transcript under `WEBMUX_HOME/logs/sessions/`
7. Session metadata and layout are persisted under `WEBMUX_HOME`; audit events are appended as JSONL

Desktop sessions follow the same REST-managed lifecycle, but their interactive streams use protocol-specific WebSocket handlers. VNC traffic is proxied directly; RDP traffic is translated through `guacd`. Agent sessions are internal terminal sessions created only after the requested tmux target passes access and existence checks.

## Configuration

Runtime configuration is YAML under `WEBMUX_HOME/config/`, which defaults to `~/.config/webmux/config/`. Files in `webmux/config.defaults/` are templates copied on first run, not live configuration. Operators can edit allowlisted runtime-safe fields through `SettingsDialog` and `PUT /api/config`; the backend requires an administrator in secure mode and permits the operator in trusted mode. Startup-sensitive and security-sensitive fields remain file-managed. See the main README for formats and deployment details.
