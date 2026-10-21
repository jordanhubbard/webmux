# Central cross-host qualification — 2026-10-21

The central daemon is OpenRig 0.6.14 on Puck (macOS ARM64), with native Codex
0.163.0 on Puck and native Codex on sparky (Linux ARM64). The remote process uses
a Rig client and a loopback SSH reverse forward to the Puck daemon. There is no
Rig daemon on sparky for this exchange.

Both agents share one Rig project with host-specific working directories:
`/Users/jkh/Src/webmux` and `/home/jkh/Src/webmux`. The central tmux relay carries
the remote interactive CLI, which lives in a persistent tmux session on sparky.

[Independent receipts](receipts.json) record a fresh nonce challenge sent only to
Puck, a fresh nonce/hash response from sparky, and Puck's independently checked
ACK. The controller recomputed the SHA256 digest and checked both agents'
receipts after the exchange. Agents coordinated through attributed `rig send`,
without reading peer files or impersonating each other's sender identity.

The SSH relay was then terminated and recreated using the same registered seat
and reverse-forward port. The remote tmux pane PID stayed unchanged. This tests
reconnection to the existing agent instead of launching another remote provider.

Frontend tests cover modal creation, attributed messages, ordinary-grid refresh,
and failure reporting. Go tests cover owner isolation, grid attachment reuse,
administrator authorization, and reattachment of persisted agent tiles after a
WebMux restart. Browser tests exercise the modal, real Rig diagnostic TUI, and
existing OpenSSH authentication/input/resize.

This record qualifies the Codex-to-Codex transport. Claude Code is selectable
and uses the same registration/SSH path, but Puck's revoked Claude OAuth token
prevents a successful live Claude cross-host run; no Claude success is claimed.
