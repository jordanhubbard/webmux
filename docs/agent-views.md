# Agent Views

Agent views let WebMux list and attach to agent runtime sessions that already
exist in named tmux servers. The feature is disabled by default. When enabled,
WebMux adds an Agents workspace that can attach to a selected tmux session and
open a scratch shell beside it.

The agent attach sessions are internal WebMux sessions. They do not appear in
the normal terminal grid, do not occupy terminal layout cells, and are launched
with `exec_argv` instead of shell-interpolated command strings.

## Runtime Requirements

- `tmux` installed on the WebMux host.
- Agent sessions running inside configured tmux servers.
- WebMux access to the same user account or tmux socket path.

## Configuration

Agent views are configured in `WEBMUX_HOME/config/app.yaml`:

```yaml
app:
  ui:
    default_pane: terminals
    host_switcher:
      enabled: false
      suffixes: []
      hosts: []
  agents:
    enabled: true
    combined_pane: true
    disable_in_multi_user_mode: true
    definitions:
      - id: codex
        label: Codex
        plural_label: Codex Sessions
        badge: CODEX
        tmux_socket: codex
```

Definition fields:

- `id`: stable lowercase ID used in URLs and status metadata.
- `label`: human label for errors and UI text.
- `plural_label`: pane or list label.
- `badge`: short badge shown beside sessions.
- `tmux_socket`: tmux `-L` socket name or absolute `-S` socket path.
- `workspace`: optional pane key. If omitted, WebMux derives `agent-<id>`.
- `enabled`: optional per-definition toggle. Defaults to `true`.

`combined_pane: true` shows one Agents pane for all enabled definitions.
Set it to `false` to show one top-bar button per definition.

Keep `ui.default_pane: terminals` for the upstream default. If you set it to
`agents`, make sure agents are enabled and at least one definition exists.

## Starting Agent Sessions

Create a named tmux server and session before opening WebMux:

```bash
tmux -L codex new-session -d -s codex-review
tmux -L codex attach-session -t codex-review
```

Start the agent runtime inside that tmux session. WebMux lists live sessions
from:

```bash
tmux -L codex list-sessions
```

When a user attaches from WebMux, the backend first validates the requested
session name against live `tmux list-sessions` output, then launches:

```bash
tmux -L codex attach-session -t codex-review
```

as an argv array.

## Scratch Shells

The Agents workspace can open a scratch shell next to the selected agent
session. When possible, WebMux asks tmux for the selected pane's
`#{pane_current_path}` and starts the scratch shell in that directory.

## Optional Status Hooks

Agent views work without hooks. Without hooks, WebMux falls back to tmux
session activity timestamps for recency and status.

For runtimes with Codex-style hooks, install the optional status writer:

```bash
install -m 0755 webmux/scripts/webmux-agent-status.js ~/.local/bin/webmux-agent-status
```

Example hook commands:

```bash
webmux-agent-status --agent codex --tmux-socket codex --status working
webmux-agent-status --agent codex --tmux-socket codex --status waiting
```

The script writes JSON metadata under:

```text
WEBMUX_HOME/data/agent-status/<agent-id>/<encoded-session-name>.json
```

If the agent runtime sends JSON on stdin with `session_id` or `turn_id`, the
script stores those values as optional hook metadata.

## Host Switcher

The host switcher is also disabled by default. Enable it only with public or
site-local names you intend to show in the UI:

```yaml
app:
  ui:
    host_switcher:
      enabled: true
      suffixes:
        - example.net
      hosts:
        - id: lab-a
          label: Lab A
          hostname: lab-a-webmux.example.net
        - id: lab-b
          label: Lab B
          hostname: lab-b-webmux.example.net
        - id: gpu-box
          label: GPU Box
          hostname: gpu-box-webmux.example.net
        - id: workstation
          label: Workstation
          hostname: workstation-webmux.example.net
```

This is only a link switcher. It does not configure reverse proxies or tunnels.

## Security Notes

Agent attach exposes local tmux sessions and a local scratch shell to the WebMux
user. For that reason:

- `app.agents.enabled` defaults to `false`.
- `app.agents.disable_in_multi_user_mode` defaults to `true`.
- Attach requests are validated against live tmux sessions before launch.
- Attach commands use `exec_argv`, not shell string interpolation.
- Use local auth and HTTPS when exposing WebMux beyond a trusted host or LAN.
- Only configure tmux sockets that WebMux users are allowed to access.

To allow agent routes in multi-user mode, set:

```yaml
app:
  agents:
    disable_in_multi_user_mode: false
```

Do this only when the WebMux account model and tmux socket permissions match
your deployment policy.

## Setup For Agents

1. Install tmux:

```bash
sudo apt install tmux
```

2. Create a named tmux session:

```bash
tmux -L codex new-session -d -s codex-review
```

3. Configure `WEBMUX_HOME/config/app.yaml`:

```yaml
app:
  agents:
    enabled: true
    combined_pane: true
    disable_in_multi_user_mode: true
    definitions:
      - id: codex
        label: Codex
        plural_label: Codex Sessions
        badge: CODEX
        tmux_socket: codex
```

4. Optionally install the hook script:

```bash
install -m 0755 webmux/scripts/webmux-agent-status.js ~/.local/bin/webmux-agent-status
```

5. Restart WebMux:

```bash
make restart
```

6. Verify the API:

```bash
curl http://localhost:8080/api/agents/sessions
```

7. Open WebMux and select the Agents pane.

## Example Reverse Proxy Snippet

This example uses fake public hostnames and is not a production recommendation:

```nginx
server {
  listen 443 ssl;
  server_name lab-a-webmux.example.net;

  location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
  }
}
```

Use your normal TLS, authentication, and network controls for any real
deployment.
