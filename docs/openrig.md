# Agent Deck and OpenRig

WebMux no longer launches Agent Deck or OpenRig. Coordination starts with
built-in WebMux terminals only (`coordination.enabled_providers: []`,
`default_launch_provider: webmux`). Saved configuration that still names
`agent-deck` or `rig` is loaded and those providers are ignored.

Existing tmux sessions were left running. Attach one from a WebMux terminal
with `tmux attach`. WebMux does not stop them, and it does not start or stop
the `agent-deck` or `rig` programs. A saved viewer whose command is a local
`tmux attach-session` is reattached. Viewers that launched through Agent Deck
or an SSH Rig wrapper stay disconnected; the tmux process is left untouched.

`make install-agent-deck` and `INSTALL_AGENT_DECK` are gone. CI no longer
installs `@openrig/cli` or compiles tmux 3.4 for Rig.

Historical qualification receipts under `docs/validation/` keep their original
scope. They describe integrations that this release removed.
