# WebMux recovery handoff

Updated 2026-11-10 (Claude fix-forward session). This is repository context for the next Claude or Codex
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
  identity and repeated reconnects do not rerun the job.
- Guest-native Agent Deck 1.16.34/Claude 2.1.322 conversation continuity across
  an actual guest reboot passed on 2026-11-10 (binary `296931b`): same native
  conversation, project, roots and launch policy, a fresh remembered-marker
  answer, and once-only recovery across reconnects and controller
  reconstruction. Details in docs/session-recovery.md.

## Fix-forward acceptance still open

1. Finish native active-session/policy/profile and platform qualification.
   Cursor chat switches are observed from the agent's open chat store and
   passed on 2026-11-12 (`qualify-recovery-configuration.py --cli cursor
   --cursor-switch`, Cursor 2026.11.11). OpenCode 1.19.5 exposes no selected
   conversation (no event, log line or file); owner decision 2026-11-12: keep
   it an explicit choice, no last-prompt tracking; revisit when
   anomalyco/opencode#54329 ships.
   Explicit models and other ordinary options are resumed for Codex, Cursor
   and OpenCode (2026-11-12, `--model CLI=MODEL` harness mode); Codex `-c`,
   working-root and worktree options stay manual. OpenCode 1.19.5 ignores `-m`
   when resuming with `--session`.
   Remote managed-host failure fallback passed on 2026-11-10
   (`scripts/qualify-recovery-remote-managed.py`, Mac controller + guest):
   remote native resume is manual-only by owner decision. Claude Code Rig seats
   on SSH hosts passed on 2026-11-10 (`scripts/qualify-recovery-rig-ssh.py`;
   the guest has a `~/.local/bin/rig` wrapper for the qualified build; pass
   `--rig-cli`/`--rig-node` for the Mac's Rig release, never the installed
   `rig` wrapper, which pins the installed OpenRig instance). Codex seats on
   SSH hosts (relay-identity attach) passed on 2026-11-11 with `--runtime
   codex`; the guest has the Codex 0.167.0 release binary in
   `~/.local/bin/codex`, logged in to ChatGPT via device code (check
   `codex login status` before reuse). Upstream Agent
   Deck reports: #3107 (silent empty remote list), #3108 (forward ownership
   inspect). Rig across an actual reboot passed on 2026-11-10
   (`scripts/qualify-recovery-rig-vm.py`; the guest now has Rig 0.6.14
   9d0200f3 and Node 22.23.3 under `~/qualification`). Retain clear manual
   fallback for unqualified cases.
2. Codex default shared-daemon mode is **manual-only** (owner decision
   2026-11-10). Upstream request: openai/codex#60625. Revisit only when that or
   an equivalent ships; do not change launch mode or drop sandbox/approval
   settings to obtain evidence. A `--no-daemon` opt-in was deferred.
3. [#325](https://github.com/jordanhubbard/webmux/issues/325) is mitigated: the
   broker answers Agent Deck attach's startup termenv probe once
   (`internal/session/attach_probe.go`). Remove it when a release fixing
   asheshgoplani/agent-deck#3105 is the minimum supported version.
4. Audit every acceptance criterion in #318 before closing it. The merge itself
   is not a release.

Claude's scoped SessionStart observer covers native clear/resume transitions on
qualified Unix launches. Hook/launch-selector evidence remains manual because
observation can stop; automatic resume requires reliable process evidence.
Preserve original native roots and their absence, invocation policy, lifecycle
owner and execution host. A WebMux transcript is not a provider-native file.

## Disposable reboot guest

The guest's own Claude 2.1.322 at `/home/webmuxqa/.local/bin/claude` is logged
in to claude.ai (expires 2026-12-08), via `claude auth login --claudeai` and the
paste-code flow driven over the fixture SSH connection. No host credentials
were copied. Recheck `claude auth status --json` in the guest before reuse.

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

To rerun the native reboot qualification after starting the guest:

```bash
python3 scripts/qualify-recovery-managed-vm.py \
  --vm-root _build/recovery-vm/qualification-441375b00aa1 \
  --binary _build/recovery-qualification/webmux-native-managed-reboot-linux-296931b \
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
