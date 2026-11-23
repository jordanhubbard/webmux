# Native agent coordination across two hosts

This record and its [source-bound receipt](receipt.json) distinguish actual native
CLI tool events, guarded job evidence, failed attempts and the remaining #630/#631
acceptance requirements. A host process exiting is not workflow success; the
separate audit requires native scoped views, real counterpart identities, claims,
overlap, busy/shared grants, original-fence releases and completed handoff receipts.

The opt-in host fixture starts separate isolated WebMux controllers on macOS ARM64
and Linux ARM64, each with independent real Git repositories/worktrees and an
ordinary production browser-shell PTY. The operator enrolls each node/account over
routed pinned HTTPS using a temporary grant and isolated certificate trust, then
types one normal native task command per session. The aliases attach MCP and
private session access automatically. Agents never receive federation/admin
credentials or instructions to extract them. The controller does not type
coordination messages into their terminal prompts. No installed service, CLI,
account configuration or pre-existing agent is replaced or restarted.

The reviewer claims `task.py`, reserves the verifier host's heavy worker, and asks
its counterpart to demonstrate the overlap/busy result. After release and an
acknowledged handoff, the verifier fixes `score(x)` and invokes the unchanged
guarded verifier with its original lease record/fence/epoch. The verifier program
checks the session identity, holder and fence, renews the original grant before
and after bounded real computation, checks 201 cases, and records evidence. The
native verifier then releases the worker and completes the handoff. The audit
consumes actual native MCP calls/results and the guarded artifact; model prose is
not proof of a call, grant, job or completion.

Native permission policies remain authoritative. Linux Claude 2.1.319 initially
discovered the provisioned MCP view but refused it without tool permission. That
failed run is retained. An explicit task-selected allowlist of only its two
session-specific coordination tools subsequently passed native view, claim,
current lease and original-fence release. No account policy or unrelated tool was
automatically allowed. The adapter adds this exact version to its bounded support
set while preserving explicit MCP restrictions and original CLI arguments.

The first pair used Codex 0.171.0 under explicit read-only/never and Linux Claude
with scoped tool approval. Both discovered the real counterpart, but Codex refused
task registration because mutation required approval and its policy was `never`.
The verifier also could not pause with shell commands under its narrow policy.
The audit failed; no claim/handoff/job success is attributed to this attempt.
Codex's approval policy was not changed to recover that run.

The subsequent pair uses native Claude 2.1.327/macOS and 2.1.319/Linux with explicit
session-tool allowlists. The verifier additionally allows Read, Edit and only the
bounded `python3 verify.py ...` command for its fixture task. Native deny rules and
account configuration still apply. This is a qualified task-selected permission
profile, not approval-free mutation under every native policy.

The successful pair's selected native conversation transcripts contain 23 tool
calls per agent. Their actual results prove verified project/real-counterpart
discovery, both file claims, the counterpart conflict and busy denial, reviewer
fence 1 release before handoff, verifier fence 2, guarded computation and original
release, plus delivered/acknowledged/completed handoff receipts. The job checked
201 inputs and performed bounded SHA-256 computation between verified renewals.
The verifier edited only the fixture's function; its driver remained unchanged.

The first terminal capture kept only its 64 KiB rolling tail. Its audit correctly
failed because early events were missing. Acceptance therefore uses each exact
selected native conversation JSONL from the original account, with retained hashes
and session binding, while keeping that incomplete stdout audit separate. The
harness now joins overlapping scrollback windows and refuses gaps. That passive
capture correction has deterministic coverage; the successful native run predates
it and its original harness hash is retained. No native task was rerun or inferred
successful merely to hide the capture limit. Removing the actual busy result or
handoff completion, or changing the guarded job fence, makes the audit fail its
specific corresponding requirement. Original native transcripts remain private;
public evidence contains source/input/output hashes and the audit summary.

MCP `webmux_coordination_view` now supports `wait_seconds` (0–30) with an inbox
cursor, avoiding shell sleeps while waiting for another participant. It reloads
private access and reauthenticates the existing scoped read API on each bounded
poll. Reading/waiting never records delivery or acknowledgment. Arrival,
cancellation, input limits and authority refusal have deterministic/race controls.
Older calls keep immediate-view behavior. Policy refusals remain visible errors.

The first full Node 24 suite encountered unchanged
[#642](https://github.com/jordanhubbard/webmux/issues/642), a Rig change-wait child
startup failure. Five focused repetitions passed without changing the source or
deadlines. The initial outcome remains retained separately from subsequent gates.

Scope remains explicit: configured/enrolled routed nodes and isolated fixture TLS,
not mDNS/LAN enrollment, deployment certificates or installed upgrades. Managed/SSH
sessions, already-running/absolute-path bypass onboarding, additional policies,
platform custody, explicit mappings, scale/retention/backoff and the complete
restart/reconnect/discovered workflow remain separate. No roadmap checkbox is
completed by a narrower native workflow result.

## Reproduce the scoped native run

Build the production helper and `go test -c ./internal/httpapi` for each actual
host architecture. Create a private evidence directory and the same temporary
32-byte-or-longer credential file named `credential` on both hosts. Do not print
it or use an existing service credential. Run only the explicit opt-in host test:

```text
WEBMUX_NATIVE_PAIR_ROLE=reviewer|verifier
WEBMUX_NATIVE_PAIR_DIR=<absolute private evidence directory>
WEBMUX_NATIVE_PAIR_BIND=<explicit reachable non-loopback address>
WEBMUX_NATIVE_PAIR_HELPER=<absolute production helper>
<httpapi test binary> -test.run=^TestCoordinationNativePairHost$ -test.v -test.timeout=12m
```

Exchange each role's generated `manifest.json` as the other's `peer.json`. These
contain fixture node identity, URL and public certificate, never a capability or
bearer. The fixture enrolls the connected peer before independently starting its
ordinary native task. Requires authenticated native Claude versions/policies
recorded above and Python 3 on the verifier. After both finish, collect each
private `started.json`, `finished.json`, complete native events and guarded
`job.json`; audit using:

```bash
python3 scripts/audit-coordination-native-pair.py <reviewer-directory> <verifier-directory> --output <audit.json>
```

For a historical rolling-tail capture, retrieve only the exact test conversation
JSONL by its recorded native session ID, keep it as `native-transcript.jsonl`, and
add `--native-transcripts`. Do not scan/read unrelated conversation contents.
Retain failed audits and artifact hashes. Stop only the isolated fixture hosts,
remove their temporary credential files, and verify process exit. No existing
service should be reconfigured to run this qualification.
