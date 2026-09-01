# WebMux Architecture

## Overview

WebMux is a browser-based remote workspace built as a React frontend and Node.js backend. REST manages configuration and session lifecycle; WebSockets carry interactive terminal, VNC, and RDP traffic.

## Component Diagram

```
Browser                                WebMux Host
┌──────────────────────────┐           ┌──────────────────────────────────┐
│ React application        │   HTTP    │ Express REST API                 │
│ ├ Terminal workspace     │◄─────────►│ ├ auth, users, config, hosts    │
│ │ └ xterm.js tiles       │           │ └ terminal/VNC/RDP/agent state │
│ ├ Desktop workspace      │ WebSocket │                                  │
│ │ ├ VNC client           │◄─────────►│ Session brokers and proxies      │
│ │ └ Guacamole RDP client │           │ ├ node-pty: SSH, mosh, exec     │
│ ├ Agent workspace        │           │ ├ VNC WebSocket proxy           │
│ └ Navigation and dialogs │           │ └ guacd RDP proxy               │
└──────────────────────────┘           │                                  │
                                       │ YAML/JSONL persistence           │
                                       └───────────────┬──────────────────┘
                                                       ▼
                                         Remote hosts and tmux sessions
```

## Backend Services

| Service | Responsibility |
|---------|---------------|
| **SessionBroker** | Session lifecycle (create, reconnect, delete, resize), layout positioning |
| **VncBroker / RdpBroker** | Desktop session lifecycle, ownership, layout, and reconnect state |
| **TransportLauncher** | Spawns SSH/mosh processes via node-pty, manages PTY handles |
| **PresenceService** | Multi-viewer tracking, focus management, WebSocket broadcast |
| **CredentialHandler** | In-memory password storage with 5-minute TTL, auto-zeroing |
| **AgentService** | Discovers configured tmux sessions and creates validated attach or scratch sessions |
| **PersistenceManager** | Runtime config/state I/O, atomic writes, JSONL audit logging, file watchers |

## Frontend Components

| Component | Responsibility |
|-----------|---------------|
| **App** | Auth lifecycle, workspace routing, config loading, top-level state |
| **TopBar** | Workspace navigation, terminal controls, auth countdown, account administration |
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
2. SessionBroker resolves host, selects transport, spawns PTY via TransportLauncher
3. Frontend opens WebSocket to `/api/term/:id`
4. PresenceService tracks viewer, assigns focus
5. Terminal data flows: PTY stdout -> WebSocket -> xterm.js (and reverse for input)
6. Session metadata and layout are persisted under `WEBMUX_HOME`; audit events are appended as JSONL

Desktop sessions follow the same REST-managed lifecycle, but their interactive streams use protocol-specific WebSocket handlers. VNC traffic is proxied directly; RDP traffic is translated through `guacd`. Agent sessions are internal terminal sessions created only after the requested tmux target passes access and existence checks.

## Configuration

Runtime configuration is YAML under `WEBMUX_HOME/config/`, which defaults to `~/.config/webmux/config/`. Files in `webmux/config.defaults/` are templates copied on first run, not live configuration. See the main README for formats and deployment details.
