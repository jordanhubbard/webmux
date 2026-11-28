# Linux native owner configuration boundary — 2026-11-27

Issues [#523](https://github.com/jordanhubbard/webmux/issues/523),
[#438](https://github.com/jordanhubbard/webmux/issues/438) and
[#787](https://github.com/jordanhubbard/webmux/issues/787) remain open.
This record advances the authenticated Linux baseline and reproduces a distinct
manager configuration effect. It does not qualify native WebMux creation or
conversation recovery.

## Authenticated native baseline

On Linux x86_64, native Codex 0.169.0 completed both model turns from the unchanged
`qualify-recovery-native.py` at main `f55db344`. The create and resume native
responses identify the same original conversation, and the second response
returns the exact synthetic remembered marker. The existing account stays in
place. The harness uses an explicit per-invocation read-only policy and ignores
user configuration; this baseline does not establish preservation of the owner's
normal coding profile or any WebMux transport/lifecycle behavior.

The [receipt](receipt.json) binds the original native outputs and result by
SHA256 without publishing the native conversation ID or account data.

## Real private manager result

Published Rig `0.7.18 (3bf87c62)` was installed privately with installation scripts
disabled and the [frozen dependency lock](rig-package-lock.json) retained. The
package matches npm's SHA512 integrity, its clean build stamp matches the manager
health identity, and its native SQLite open/query/close smoke passes with Node
24.22.3. No global package, daemon, account or service was changed.

The maintained `scripts/qualify-rig-owner-config.py` creates a private manager
with no kernel, its own tmux server, an empty profile, a disposable Codex home
and a synthetic custom launch wrapper. The initial configuration has a trusted
parent directory, `read-only` sandbox and `never` approval. No native account or
credentials enter this fixture.

| Control | Original policy retained | Owner config retained | Observed effect |
| --- | --- | --- | --- |
| Hooks disabled, explicit defect expectation | Yes | No | Adds a project trust entry |
| Hooks enabled, explicit defect expectation | Yes | No | Adds project trust and managed hook configuration |
| Default preservation requirement, hooks disabled | Yes | No; exits 1 | Refuses to call this preservation acceptance |

Both real manager cases start the custom wrapper. Their member HTTP replies
remain uncertain at the original production eight-second response boundary;
the probe does not repost creation or equate a timeout with confirmation.
Independent wrapper and configuration observations establish the side effect.
Every final control confirms successful private daemon shutdown, closed manager
port and removal of its private tmux server.

An initial response-only observer failed at its 15-second request timeout. That
failed receipt is retained privately and hashed here. The corrected observer
uses the production eight-second boundary and records uncertainty before
inspecting effects; no product deadline, assertion or readiness rule changed.

## Cause and required integration work

WebMux's `nativeRequest` supplies the original owner's `CODEX_HOME` to the native
member and selects an empty profile with a custom launch wrapper. Rig's startup
orchestrator nevertheless calls `deliverStartup` for empty file lists. The Codex
adapter calls `ensureManagedBootstrap` / `provisionWorkspaceTrust` against that
home; hook configuration is a separate startup effect. The custom wrapper does
not prevent these earlier writes. Disabling hooks alone therefore does not
establish owner configuration preservation.

A supported owner-configuration boundary is required before authenticated Linux
manager-owned creation is accepted. It must preserve native account, project
trust, policy/roots/model/approval and observation/lifecycle identity. Copying
credentials into an empty manager home, weakening drift checks, or silently
using adopted ownership does not satisfy that requirement. #787 tracks the
necessary integration/dependency repair. No attempted real-owner creation or
automatic native trust approval was performed for this investigation.

## Reproduce

Install the retained package inputs into a private directory with qualified
Node 24. Name the copied files `package.json` and `package-lock.json`, then run
`npm ci --ignore-scripts --no-audit --no-fund` there. Use the qualified native
Codex 0.169.0 executable; this control does not need its authentication.

```sh
python3 scripts/qualify-rig-owner-config.py \
  --node /absolute/qualified-node/bin/node \
  --cli /absolute/private-rig/node_modules/@openrig/cli/dist/bin-wrapper.js \
  --codex /absolute/qualified/codex \
  --output /absolute/private-evidence \
  --hooks disabled --expect changed
```

Repeat with `--hooks enabled`. Omit `--expect changed` to require actual
configuration preservation: the qualified uncorrected manager fails that
requirement. A successful explicit defect reproduction is recorded separately
from `owner_preservation_passed`, which remains false in all current cases.
