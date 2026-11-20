# Native original-conversation attachment recovery

The scoped two-loss native workload passes at `65dc25bc17bf3f8c16379c992b5df68c5c096ad9`, addressing [#608](https://github.com/jordanhubbard/webmux/issues/608) and the discovered transcript defect [#614](https://github.com/jordanhubbard/webmux/issues/614). Both initial authenticated native coding probes and every recovery use the existing Rig0.7.18/Codex0.169 installation and account in place. No SDK, policy, assertion, deadline or check count is relaxed. **This does not qualify the full roadmap or a release.** The [receipt](receipt.json) binds source, binary, drivers and retained evidence.

## Qualified behavior

The broker persists the approved original request before acknowledging pending. Its worker keeps the original response deadline, capped at45seconds, survives connection loss, and is cancelled/drained at broker shutdown. Ordinary duplicates remain read-only. Previews skip busy capture rather than queue behind recovery. Background completion retains the pending original; an explicit Check must reverify the current native identity before attachment.

An explicit Check can observe the existing worker for up to3seconds. The native helper separately observes an unjoined replacement using GETs for up to3seconds, within the existing response window. Those readiness reads open no account/environment channel and issue no native POST. An absent running row still permits crash-before-first-POST completion with the original key. Ready metadata only permits the same keyed conditional confirmation; it never authorizes another conversation or a new operation.

The full unchanged workload verifies:

- Two real native coding probes, original-conversation no-tool recall after reload/reconnect, offscreen reveal, occupied-cell minimize restore and detach/reopen continuity.
- Two attachment losses, each followed by confirmed original-conversation recovery and another no-tool canary recall. All requests within each loss retain their original UUID/body, apart from the explicit inspection flag.
- Three distinct native processes for the recovered seat, one unchanged sibling process, four owner session records and exactly two native recovery intents, consumed dispatch claims, confirmations and one-use environment acknowledgments.
- One retained transcript filename through both recovered attachment generations, unchanged byte prefixes and appended output. Native reconnect carries the owned current log forward; the replacement writer waits for the prior writer to drain. Controller reconstruction and removed-file recreation have separate deterministic tests. Paused recording remains paused, foreign filenames are refused, and ordinary reconnect behavior is preserved.
- Exact ordinary-terminal input `xy`, no duplicate viewers, unchanged read-only/on-request project policy and all42bound SDK files.

| Cycle | Initial pending response | Explicit Checks | Final result |
| --- | ---: | ---: | --- |
| First loss | 1.615s | 5 | consumed |
| Second loss | 1.706s | 4 | consumed |

Every response is below the unchanged10second observer; the maximum is4.474seconds. The existing six-Check allowance, native10second request timeout and45second server budget remain unchanged. Native journals independently prove original-user anchors, successful remembered canaries and no model tool calls after either recovery prompt. The private fixture uses WebMux loopback auth-none; the native coding account is authenticated. No WebMux SSO or independent policy-enforcement probe is claimed.

Local checks pass:270 helpers, including17native-resume controls; five strict TypeScript projects; lint; vet; and the full33-package server Go race suite. Focused transcript race controls pass10repetitions. Source controls fail with the previous synchronous scheduling/readiness behavior and before the log fix. The earlier unpinned Node26 invocation and corrected HTTP-test sequencing failure remain retained.

The native executable ran from a clean source tree. Integration of current main changes only documentation; all non-doc tracked inputs remain byte-identical to the executed candidate. All scoped private processes are inactive after exact socket/open-file controls. Selected journals, operation records, log bytes, sources, binary and checks have private SHA-256 readback; no whole home, authentication store or native database is archived.

## Additional failed candidates remain failures

`ebf236d` returned pending within the observer but exhausted the original six Checks before the owner became ready. `38b3bf0` completed both conditional native recoveries and no-tool recalls, but failed the second retained-log assertion: it created new generation filenames. The first old log grew only through its stop marker. Its overall result stays false and only one full cycle is credited. #614 records that defect; the final candidate corrects it without changing the workload.

Full #603/#464/#469, lifecycle #318/#438 and release acceptance remain incomplete. Native peer messaging remains separately unqualified; there is no permission escalation, unchanged send retry, host reboot, installed-service upgrade, new Linux/Windows qualification or hosted Actions job.

## Historical diagnostic and partial handoff candidate

The early diagnostic and partial candidate below were unqualified and could not support a merge. The original 10-second recovery gate remains failed; no timeout, assertion or native SDK is relaxed. Full #603/#464/#469, recovery #318/#438 and release acceptance remain incomplete.

## What the same-request diagnostic establishes

At clean `c11a6e7e565cded095efe71c315ad99eeb7991a9`, two real native Codex seats retain read-only/on-request and complete their original canary/no-tool viewer-continuity checks. A passive recorder then keeps the same recovery request alive after its failed 10-second assertion. It records only that request's HTTP response, selected immutable files, owned process ancestry, native journal metadata and GET-only owner inventory. It issues no native POST or replacement operation.

The extended diagnostic records:

| Observation | Time from original POST |
| --- | ---: |
| Replacement native process first observed | 8.069 s |
| HTTP 202 / `native_resume_response_unavailable` | 16.977 s |
| Replacement owner session first observed ready with expected original `codex_id` | 18.078 s |
| Final passive snapshot | 50.230 s |

The diagnostic polling window is 50 seconds, derived from the server's existing 45-second request budget plus observation grace; a bounded final snapshot follows. It is separate from the unchanged failed 10-second assertion. The sibling PID and original journal prefix remain stable. A prior shorter diagnostic recorded HTTP 202 after 17.863 seconds and stopped before the owner marked its replacement ready. HTTP uncertainty is not a terminal native outcome. An old journal alone never established the replacement's active conversation identity; the extended record separately correlates the replacement PID and its new running owner row with the expected token and ready status.

This does not prove a post-resume model turn, tool restrictions after resume, both required loss cycles or WebMux-confirmed completion. All producer reports remain false. The pinned owner's source checks joined process/original-conversation proof before retaining the new session's resume token; a live process or a historical record is insufficient. The source-bound receipt distinguishes this late owner observation from the failed public gate.

## Partial code candidate

Candidate `25a77dd4b39c780885bb8a086b7340a332b29774` changes WebMux to return typed uncertainty at a new one-use owner-environment acknowledgement, rather than continuing to wait for joined proof. Explicit inspection reads native readiness before confirming the same keyed request. The native conditional identity check still makes the final decision; the readiness hint never grants launch or confirmation authority. An absent running row remains distinct from an unjoined replacement, so explicit completion can still deliver the first original POST after a controller crash between its local claim and HTTP dispatch.

Its actual native rerun **still fails** the original 10-second response observer, before the local native dispatch marker or environment-handoff acknowledgment exists. Returning at handoff is therefore insufficient: pre-dispatch preparation can consume the response window too. This is not a completed latency repair, and the native rerun verifies zero of the planned two loss cycles.

The historical follow-up required the implementation to durably bank the selected original request before returning pending, keep authorized preparation under a bounded broker-owned lifetime, and keep preview/status or duplicate checks from blocking behind that work. GETs must not launch providers or account observers. Confirmation must reverify the exact live original; no cached result may authorize attachment to a changed provider. Broker shutdown must cancel/drain its owned work. Complete the unchanged two-loss workload, original no-tool recall, sibling and log-prefix checks before closing #608 or merging the candidate.

## Checks and retained scope

Local candidate checks pass: 269 helper tests, including 16 native-resume controls, all five strict TypeScript projects, lint, vet and 548 Go race tests with 11 recorded skips. The private old-module negative control fails the readiness regression as expected; it is not an original released executable. Nine passive-recorder controls verify failure retention, request correlation and collection bounds. The initial type error and a 267/268 helper invocation missing `lsof` from its PATH remain retained. A causal missing/complete PATH control establishes that setup error without changing the native executable observer or its assertions.

Raw diagnostic failures, the failed candidate run, selected owned journals/operation records, source snapshots, owner source, binary and checks have private SHA-256 readback. All three fixtures are inactive after bounded open-file and exact tmux checks; inactive homes/socket inodes remain retained. Intended policy and all 42 private SDK files stay unchanged. No native auth store, whole home, installed service, account permission, global CLI, host reboot, hosted job or release is changed.
