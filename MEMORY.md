# WebMux recovery handoff

Updated 2026-11-10. This is repository context for the next Claude or Codex
session; follow [AGENTS.md](AGENTS.md) for the development workflow.

## Owner's current direction

Commit and push all work, merge the current recovery checkpoint through
[PR #322](https://github.com/jordanhubbard/webmux/pull/322), then fix forward in a
Claude session. Keep [#318](https://github.com/jordanhubbard/webmux/issues/318)
open: integration is authorized before full recovery acceptance. Do not wait for
guest authentication to merge this checkpoint. No release or restart of the
installed workstation service was requested.

The current priorities and evidence boundaries are in
[the roadmap](docs/roadmap.md). Detailed receipts, versions, hashes, successful
checks and failed experiments are in [session recovery](docs/session-recovery.md).
Use GitHub's current PR/main checks as authority for the latest commit, rather
than assuming an older checkpoint's green checks apply to new source.

## Implemented and demonstrated

- Durable owner/terminal activity and native-session references, retained
  transcripts, recovery previews, explicit replay/context consent and optional
  qualified automatic native resume. Surviving providers are checked first.
- Durable request identities prevent duplicate launches. Requested/uncertain
  outcomes are not retried after a lost response or controller reconstruction.
- Real native conversation continuation for supported Codex 0.167.0, Claude
  Code, Cursor agent 2026.11.05-86047df and OpenCode 1.19.5 invocations. Recent
  Claude/Rig evidence uses Claude 2.1.322. Exact versions and limits vary by
  receipt; launching a fresh CLI does not establish recovered state.
- Real ordinary local, SSH and native mosh controller restart/update/SIGKILL and
  guest reboot checks, including retained history and execution-count checks.
- Real local Agent Deck 1.16.34/Claude and Rig 0.6.14 (9d0200f3)/Claude managed
  native actions. Rig preserves original project, session, settings and
  executable identity and passed an in-flight lost-response test.
- Rig captures the canonical OS executable path and hash. Preflight verifies
  both before invoking its version command or asking Rig to launch, and hashes
  again afterward. Legacy pathless records remain readable with manual recovery.
- Real Agent Deck shell-job guest reboot: fallback retains manager/project/log
  identity and repeated reconnects do not rerun the job. This does **not** prove
  native coding-conversation continuity across reboot.

## Fix-forward acceptance still open

1. Run the guest-native managed conversation reboot test described below. Require
   a fresh post-reboot answer recalling the original marker, the same native
   conversation/project/configuration, and once-only recovery across reconnects
   and controller reconstruction.
2. Finish remote managed-host/failure, native active-session/policy/profile and
   platform qualification. Retain clear manual fallback for unqualified cases.
3. Resolve Codex's default shared-daemon active-conversation observation after
   interactive switches. Explicit selected IDs have passed manual continuation;
   they do not prove the current session after an unobserved switch. `-c` hook
   injection required embedded mode in the tested native CLI. Do not silently
   change launch mode or drop sandbox/approval settings to obtain evidence.
4. Audit every acceptance criterion in #318 before closing it. The merge itself
   is not native reboot acceptance or a release.

Claude's scoped SessionStart observer covers native clear/resume transitions on
qualified Unix launches. Hook/launch-selector evidence remains manual because
observation can stop; automatic resume requires reliable process evidence.
Preserve original native roots and their absence, invocation policy, lifecycle
owner and execution host. A WebMux transcript is not a provider-native file.

## Guest-native test prepared, not passed

The user said Claude is authenticated. `claude auth status --json` confirmed the
Mac's existing claude.ai login. The disposable Linux guest has its own Claude
2.1.322 at `/home/webmuxqa/.local/bin/claude`; its last verified status was
`loggedIn: false`. Its browser login did not complete. The private login process
and OAuth callback forwarding were stopped for this handoff; no host credentials
were copied. Recheck the guest's own authentication before running the test.

The retained QEMU guest is stopped, with its disk and evidence preserved and
4 GiB configured RAM. Its ignored fixture directory is:

```
_build/recovery-vm/qualification-441375b00aa1/
```

`launch.json` contains the QEMU argv; its PID is historical after shutdown.
`ssh-argv.json` contains the fixture-only SSH connection. Revalidate the process,
disk, QMP socket, fixture marker and ports when starting it. Never substitute the
workstation or a shared worker for the disposable reboot target. Authentication
status is recorded without credentials in `handoff-auth-status.json`.

The native mode is implemented and Python-compilation checked, but has no
passing native reboot receipt. After starting/authenticating the guest:

```bash
python3 scripts/qualify-recovery-managed-vm.py \
  --vm-root _build/recovery-vm/qualification-441375b00aa1 \
  --binary _build/recovery-qualification/webmux-native-managed-reboot-linux \
  --native-claude
```

Rebuild the binary when application code changes. The harness creates a new
synthetic guest-native conversation, resumes through the real Agent Deck
manager, reboots, invokes WebMux's approved managed action and requires fresh
remembered-marker responses. It checks native executable/project/state-file,
original roots/launch policy and duplicate receipts after reconstruction.
Unexpected native onboarding or provider behavior must produce negative
evidence until corrected; do not weaken the oracle to obtain a passing receipt.

## Local development and evidence

- Run npm commands in `webmux/`, Go commands in `webmux/server/`.
- Build standalone executables with `go run ./cmd/build -o /absolute/output`
  from `webmux/server`; add `-goos linux -goarch arm64` for this guest.
- Relevant implementation: `internal/recovery`, `internal/session/managed_*`,
  `internal/httpapi/managed_recovery.go`, `rig_native_recovery.go`,
  `internal/openrigcoord/rig-recovery.mts`, and frontend `RecoveryDialog`.
- Application checkpoint `3afb3bf` passed all hosted CI/Packaging. Prior local
  evidence includes 339 frontend tests, 50 helper tests, affected Go race/vet,
  strict TypeScript, builds and real native Rig HTTP-action continuation. Check
  #322/current main for the final handoff commit's own results.
- Native synthetic receipts live in
  `~/.local/share/webmux/recovery-qualification/`; VM receipts and binaries are
  under `_build/recovery-vm/` and `_build/recovery-qualification/`. They are
  intentionally ignored. Do not delete them during branch cleanup.
- [#320](https://github.com/jordanhubbard/webmux/issues/320) tracks intermittent
  hosted SSH/lease/reconnect failures. [#324](https://github.com/jordanhubbard/webmux/issues/324)
  tracks an intermittent existing fake-launchctl fixture timeout.
- Original retired branch refs are retained in
  `.git/branch-cleanup-20261109.bundle`. Detached `rt`, `si` and `wb` qualification
  worktrees were clean and their patches already incorporated; preserve their
  ignored evidence. The final branch cleanup should leave `main` and any actual
  release branches, without inventing a release branch for unfinished work.
