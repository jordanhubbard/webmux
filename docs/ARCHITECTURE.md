# WebMux Architecture

## Overview

WebMux is a web-native terminal multiplexer built as a two-tier application: a React frontend and a Node.js backend, communicating over REST and WebSocket.

## Component Diagram

```
Browser                          Jump Box (WebMux Server)
┌──────────────────────┐        ┌─────────────────────────────────────┐
│  React + xterm.js    │  HTTP  │  Express                            │
│  ┌────────────────┐  │◄──────►│  ├── REST API (sessions, hosts,     │
│  │ Workspace      │  │        │  │   config, auth)                   │
│  │ ├── Tile       │  │  WS    │  ├── WebSocket handler              │
│  │ │   └ Terminal  │  │◄──────►│  │   └── PresenceService            │
│  │ ├── Tile       │  │        │  ├── SessionBroker                   │
│  │ └── ...        │  │        │  │   └── TransportLauncher           │
│  ├────────────────┤  │        │  │       ├── SSH (via node-pty)      │
│  │ TopBar         │  │        │  │       └── Mosh (via node-pty)     │
│  │ ConnectionDlg  │  │        │  ├── PersistenceManager (YAML)       │
│  │ LoginPage      │  │        │  └── CredentialHandler (in-memory)   │
│  └────────────────┘  │        └─────────────────────────────────────┘
└──────────────────────┘                    │
                                           ▼
                                    Remote Hosts (SSH/Mosh)
```

## Backend Services

| Service | Responsibility |
|---------|---------------|
| **SessionBroker** | Session lifecycle (create, reconnect, delete, resize), layout positioning |
| **TransportLauncher** | Spawns SSH/mosh processes via node-pty, manages PTY handles |
| **PresenceService** | Multi-viewer tracking, focus management, WebSocket broadcast |
| **CredentialHandler** | In-memory password storage with 5-minute TTL, auto-zeroing |
| **PersistenceManager** | YAML config I/O, atomic writes, JSONL audit logging, file watchers |

## Frontend Components

| Component | Responsibility |
|-----------|---------------|
| **App** | Auth routing, config loading, top-level state |
| **TopBar** | Navigation, font size controls, broadcast toggle, auth badge |
| **Workspace** | 2D tile grid, session CRUD, split positioning |
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
6. Session state persisted to YAML after each change

## Configuration

All config is YAML in `webmux/config/`. See the main README for format details.
