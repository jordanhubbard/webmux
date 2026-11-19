# Current Rig routing qualification and creation-response observation

Issues #464 and #469 remain open. A fresh local compatibility refresh found
that the modern synthetic managed-agent case no longer qualifies the current
tracked native-creation path. [#603](https://github.com/jordanhubbard/webmux/issues/603)
tracks completing that workload without weakening native qualification.

The [receipt](receipt.json) attributes each execution to its original source.
The baseline is `694c000f79e820bb8059bafc6befbc90392c9b77`; observer-only source
is `fcab74bfbdd3a3d8d01a41b8c7edd7154df12263`. Actual pinned Rig, Go, tmux and
Chrome run locally, with synthetic interactive provider children. No coding
login, credential copy or model conversation is part of these cases.

| Original 13-case inventory | Passed | Failed | Result |
| --- | ---: | ---: | --- |
| Rig 0.6.14 (`df1c1517`) | 13 | 0 | Legacy workflow passes. |
| Rig 0.7.18 (`3bf87c62`) | 12 | 1 | Managed native creation refuses the unqualified synthetic Codex CLI. |

Each file has a fresh private controller/daemon/tmux fixture. The initial file
inventory also passed a newer two-seat grid case on each build, but skipped
the original coexistence cases because their fixture flag was omitted. The
skipped results are retained. Separately executing the original cases with the
source-required `WEBMUX_REAL_AGENTS=1` passes on both builds. The table reconciles
original cases by exact title; it does not relabel an aggregate run or count an
extra case as a replacement for a skipped original case.

The native two-real-tmux-server restore/rebind test passes without a skip,
including wrong ambient server, input/mouse isolation, automatic refusal,
explicit same-viewer rebind, history and both providers surviving with one
start each. Its manager inventory is a protocol fixture. Four Go routing and
dashboard boundary tests, 33 helper controls and strict E2E TypeScript pass.
All 5,625 published modern CLI files match the existing complete installation.

The managed test previously waited only for `/api/coordination/agents` and hid
the modern response behind a timeout. It now waits for the exact tracked UUID
route on the modern build and the legacy route on 0.6.14. An allowlisted
attachment records only fixture role, route category and HTTP status. The
legacy managed lifecycle passes with two HTTP 200 responses. The modern case
still fails its original HTTP 200 assertion, now visibly at HTTP 502 with
`qualified_codex_unavailable`. This diagnostic success does not waive the
ordinary failed gate or qualify the modern managed lifecycle.

Source reversal verifies that only the response predicate and attachment
changed; all original workload assertions, deadlines and production code remain
byte-preserved. Earlier zero-execution setup/discovery errors and the incorrectly
named fixture flag are retained separately. The tarball-only dependency error
was corrected by selecting an already complete installation of the identical
CLI build, with no package installation or global upgrade.

A separate [genuine native policy probe](native-policy.json) uses clean main
`3920286ed8d70a5736610064d7c60cb13333a3c3`, the same pinned modern Rig and
Codex 0.169.0 on macOS ARM64. Two real seats are created through the browser's
tracked requests and each completes a canary-reading native tool. Five native
turn contexts and both launch plans retain gpt-6.2-astra and read-only/on-request
with network access disabled. The first model initially omits its canary tool's
seat argument and corrects it; both tool outputs remain retained. There is one
successful correlated canary result per seat, rather than one attempt each.

That probe's messaging gate **fails**. The first provider's actual `rig send`
tool exits 1 with `fetch failed (EPERM)` at the loopback daemon. Rig reports
**Delivery UNCONFIRMED**; the 30-second WebMux observation sees no challenge or
peer ACK. This establishes a policy boundary, not a newly proven routing defect
or definitive non-delivery. No escalation is approved and the policy is not
relaxed. Reload, reconnect, loss-recovery, history and sibling phases after the
message gate are not reached. Both private producer processes exit 0 while their
JSON reports say false; acceptance explicitly reads those failed reports and
the native tool result instead of treating process exit as success.

The private archive retains the drivers, binary, bound source, both selected
native journals and failed reports. All 18 sampled owned controller/tmux/provider
PIDs and the separately recorded daemon PID exit; daemon shutdown is clean and
complete, and private tmux stops. Source, intended project policy and all 42
private SDK files remain unchanged. The existing account stays in place, with
no authentication-store copy. This additional failed probe does not replace
any original matrix result or close #603, #464 or #469.
Later main `94ee2e9` adds durable coordination-send handling; this probe retains
its executed `3920286` source and does not qualify those newer public wrappers.

The remaining work is actual current modern native-creation qualification.
Native version/account spoofing, policy bypass, provider substitution or dropping
the managed case would not establish it. Older 26-case receipts retain their
historical source scope; separate real native coding receipts are not replaced
or invalidated by this synthetic fixture result. No hosted jobs, installed
service/protection changes or release publication occurred. Full roadmap and
release acceptance remain incomplete.
