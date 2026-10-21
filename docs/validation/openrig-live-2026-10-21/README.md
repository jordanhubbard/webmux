# Live OpenRig coordination acceptance — 2026-10-21

**Passed before installing or relaunching WebMux.** Run
`webmux-live-0b2862dfbeecec74` used the published OpenRig 0.6.14 daemon, Claude
Code 2.1.302 and Codex CLI 0.162.0. Two distinct native CLI processes ran in
separate working directories on this Linux ARM64 host. There was no mocked
daemon, scripted agent substitute, direct tmux injection, or controller-supplied
response. The controller sent its challenge only to Claude.

The live daemon's `outbox_entries` records show:

| Time (UTC) | Sender | Recipient | Message |
| --- | --- | --- | --- |
| 07:40:31 | Claude / ALPHA | Codex / BETA | Fresh controller challenge nonce |
| 07:40:37 | Codex / BETA | Claude / ALPHA | Independently generated second nonce and SHA-256 response |
| 07:40:51 | Claude / ALPHA | Codex / BETA | Verified acknowledgement |

Each message has transport-derived seat identity (`transport:v1`). Both agents
independently recomputed the digest and wrote matching results in their own
directories. ALPHA could not claim BETA's completion from reading its files;
the controller collected both files after the exchange.

Expected digest:

```text
34700fde1d0f1bf9af56d1d8ab83ae3a374d66fe0e1fee9f34f83e62fb73cb55
```

## Inspect and verify

From the repository root:

```sh
node scripts/verify-openrig-proof.mts
node --test scripts/verify-openrig-proof.test.mts
```

The verifier independently hashes `webmux-openrig-v1:run_id:nonce_a:nonce_b`,
checks both agents' results, distinct native process identities, message order,
sender/recipient envelopes, transport attribution, response and ACK payloads,
and the live daemon's recorded health/version. Negative tests reject fixture
daemons, stub runtimes, missing replies, forged attribution, wrong digests and
weakened filesystem isolation. The helper test suite runs these checks in CI.

- [proof.json](proof.json): daemon health, native process/version observations,
  both agents' result files, and the three matching daemon outbox rows.
- [Claude transcript](alpha-transcript.txt) and [Codex transcript](beta-transcript.txt):
  captured native terminal views, with the temporary root replaced by `$POC_ROOT`.
- [Rig spec](rig.yaml), [agent manifest](agent/agent.yaml),
  [bounded role](agent/role.md), and [controller task](controller-task.txt):
  the actual inputs, rather than a reconstructed demonstration.

OpenRig's outbox rows remain marked `pending`; that bookkeeping field alone
does not establish consumption. Here, consumption is established by BETA's
fresh nonce, ALPHA's matching ACK, both independent digest checks, and the native
transcripts. The proof records retain the original outbox state.

## Requirement found by the test

The first attempt correctly failed: the isolated Codex test profile permitted
workspace writes but blocked network access, so its reply to the loopback daemon
failed with `EPERM`. Neither agent wrote a passing result. The corrected isolated
profile kept `sandbox_mode = "workspace-write"` and explicitly set:

```toml
[sandbox_workspace_write]
network_access = true
```

This is the documented [Codex sandbox network setting](https://learn.chatgpt.com/docs/config-file/config-reference).
It permits outbound networking in that sandbox; it is not a loopback-only allow
list. WebMux does not change provider policy. Configure an appropriate policy
for your agent environment and check that it permits access to the local daemon.

Only the private test rig was relaunched after that correction. A fresh run ID
and nonce were used for the successful exchange. The real WebMux service and
the user's provider settings were unchanged.

## Repeat the live test

Use an isolated OS home, `CODEX_HOME`, `CLAUDE_CONFIG_DIR`, `OPENRIG_HOME`, and
private `TMUX_TMPDIR`, with a supported Node 24 runtime and the published CLI.
Authenticate the two providers there without putting credentials in evidence.
Ensure the sandbox can reach the daemon. Do not run `rig setup` against the
user's normal home for this acceptance test.

1. Copy the rig spec and agent directory to a scratch workspace; create separate
   `alpha` and `beta` Git working directories beneath it.
2. Start `rig daemon start --no-kernel --host 127.0.0.1 --port <unused-port>`.
3. Launch `rig up <scratch>/rig.yaml --yes --json`; verify both real runtimes
   reach their normal prompts and report READY.
4. Generate a new run ID and nonce with Python `secrets`, update the controller
   task, and send it **only** to `proof-alpha@webmux-proof` using `rig send`.
   Let both agents perform all three peer messages through `rig send`.
5. Collect each agent's `*-proof.json`, the matching daemon outbox rows, health
   and process observations in the format of `proof.json`. Run the verifier on
   that new evidence file. Do not substitute a prewritten BETA response.
6. Stop the private rig and daemon, remove temporary credential copies, and
   confirm that no test processes remain.

This run was cleanly shut down with `rig down` and `rig daemon stop`; temporary
credential copies were removed. The result qualifies same-host bidirectional
communication between Claude Code and Codex through OpenRig. It does not qualify
cross-host routing, unattended long-running team management, or automatic
enrollment of arbitrary existing tmux sessions.
