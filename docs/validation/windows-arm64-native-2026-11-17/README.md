# Native Windows ARM64 qualification

## Component acceptance verified 2026-11-19

The component requirements in #456 are verified against original native source
`bb16ac823a40b3d15f4b55fc55c91318ec803b3e`,
[run 44608762013](https://github.com/jordanhubbard/webmux/actions/runs/44608762013)
and artifact `14574383516`. This is a readback and semantic audit of retained
native evidence; no new workflow or native run was started.

| Requirement | Verified evidence |
| --- | --- |
| Native OS, compiler and tools | Windows ARM64 OS; native ARM64 Go host/target and Node; actual capture executable independently PE-inspected as ARM64 and hash-bound to its receipt. |
| Server and retained models | 495 server, 156 cell-model and 317 terminal-engine top-level passes; no failures. Existing platform skips remain recorded, and no race-detector support is claimed. |
| Actual ConPTY streams | All 28 distinct source-defined capture cases execute and pass with no capture failures/skips, one job per case and 56 distinct snapshot tokens. |
| Renderer comparison | All 1,384 source-derived case titles match the completed job log. All 1,381 comparison receipts match at initial, continued and recaptured stages; zero replay queries. The 259 bare-reference cases retain unmodified renderer methods; 1,122 use the separately identified shared-hyperlink reference. |
| Refusal and existing gates | Wrong OS/Node/Go host/target and mismatched/malformed PE controls pass. The reviewed ARM candidate preserves CI/Packaging bytes versus updated parent `3b80d0d`; the independent native gate retains its separate non-race scope. |
| Source and artifact binding | Original exact-head workflow, artifact metadata, source-head record, native capture PE/hash and all 2,782 extracted member hashes verify. |

The renderer browser is **AMD64 under emulation**, explicitly PE-inspected and
selected by the workflow. This does not claim a native ARM64 browser.

The [acceptance receipt](acceptance-2026-11-19.json) also reconciles current main
`694ee835b3b46fd56e4c7a4aa75588a1ca00f367`: runtime/capture/renderer trees,
dependencies, architecture admission and the native workflow equal the qualified
head. Subsequent application-tree changes are Darwin/Linux-only hook files and
an expanded POSIX test whose Windows skip guard is unchanged. Actual native
execution remains attributed to `bb16`, not relabeled as a fresh `694` run.
Workflow/artifact source binding is not an independent signed VCS attestation.

This completes #456's component scope. Authenticated Windows ARM64 coding
conversation/reboot recovery remains under #318/#438; installed-service recovery,
physical/provider acceptance and the original #450 corruption are separate.
Current-candidate release checks and the full roadmap remain open.

## Historical implementation and first qualification attempts

[#456](https://github.com/jordanhubbard/webmux/issues/456) adds a separate native
Windows 11 ARM64 gate under #318/#355. The existing AMD64, macOS and Linux CI and
packaging workflows remain byte-identical to the updated parent `3b80d0d`. This candidate does
not replace their race suites with non-race testing.

The new workflow checks native OS/Node/Go host and target architecture, and
requires ARM64 PE machine headers for the actual Node, Go and capture images.
Image SHA-256 values, the exact checked-out source commit, worker headroom,
Go test JSON, raw ConPTY captures and renderer receipts are retained. It checks
out the exact PR head rather than relying on an implicit merge ref. Pull
requests against stacked branches also run this independent gate.

Server and retained UV/VT suites run natively without a race-detector claim.
The existing native screen collector must execute all 28 distinct ConPTY cases
with no failed or skipped capture cases. The frontend then compares those real
captures at initial/continued/second attachment, retaining the existing bare
versus shared-hyperlink reference distinctions. The browser PE machine is
recorded separately: an AMD64 browser under emulation is not native ARM64
browser execution. The renderer is pinned to that same inspected executable,
so a default headless-shell selection cannot invalidate its architecture receipt.

Local checks pass for the two architecture-admission tests, including wrong
OS/Node/Go target/host, AMD64/IA32 images, truncated headers, invalid offsets,
bad signatures and unknown machines. The positive PE fixture is never executed;
these portable controls are not native Windows acceptance. Strict project types,
YAML structure and unchanged existing-gate checks also pass. Source bindings
are in [source-manifest.json](source-manifest.json).

At this original preparation checkpoint, native ARM64 results were pending and
#429 blocked its merge/release gate. The historical source manifest retains
that status; the later acceptance review above records the completed native
component evidence. It does not establish authenticated coding-conversation or
installed-service recovery, or repair the separate unexplained #450 panic.


The combined candidate also retains the native-qualified VT aggregate-budget
correction (#458, source `52a8527`) and exact-payload USB fixture readiness
(#459, source `3b80d0d`). Their input artifacts are bound in the manifest. They
remain component evidence; this combined head still needs its own full CI and
native ARM64 results. The original ARM64 input reached actual native server,
UV/VT and 28-case ConPTY success, but its renderer comparison was not explicitly
bound to the PE-inspected executable. The new explicit binding closes that
provenance gap without claiming the old browser receipt proved actual selection.

The first native run at `2e0fdd3` reached its initial 30-minute job deadline
after passing 735 of 1,384 serial renderer cases. It was still completing
cases at approximately 1.6 seconds each, with no renderer assertion failure.
The renderer started about nine minutes after job setup; that observed rate
requires approximately 38 minutes for all renderer cases, before native tests,
dependency installation and cleanup. The job now has a 60-minute aggregate
budget. Per-test, browser, native-suite and production deadlines, the single
worker, zero retries and all cases remain unchanged. This is a measured job
budget correction, not a successful full ARM64 renderer qualification.
The canceled run's artifact `14401767083` (SHA256
`3177669a6a8ecc2efa1832c800768eac237cea3deda598c79d1795b1df2e9a23`)
retains raw native captures, architecture admission and partial comparisons.
