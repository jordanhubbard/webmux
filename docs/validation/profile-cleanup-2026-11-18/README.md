# Native private-profile cleanup diagnosis

[#473](https://github.com/jordanhubbard/webmux/issues/473) retains a Windows
failure from exact source `8e90c3b`, CI `44258246960`, job `134645042243`.
All 22 ordinary first loads succeeded. The `direct-home-websocket` case then
failed removal of its private home profile, after allocator cancellation/wait.
The old code discarded the removal error. Artifact `14408200737` SHA-256 is
`c88881a431fa85b386f5f00aa41a04cc3294468399d06ee4c05d1a5b5f66e3cc`.
The separate full browser workflow, including broadcast input, passed.

The diagnostic keeps the original one-shot removal and failed assertion. On
failure it records only a fixed operation/class, OS error number and fixed file
category. It never retains raw paths, arbitrary error strings or file contents.
Portable controls check bounded serialization and refusal to export private data.
A Windows-only control holds an owned file open without delete sharing, requires
the actual sharing refusal, closes that handle and requires successful removal.

Normal qualification adds no process query before shutdown. Optional manual
sampling supports fixed sets of 20 or 100 independent trials. Process observation
is separately opt-in, so unobserved samples preserve the original timing. It retains native
handles for CDP-reported processes matching the owned browser's image and creation
ordering, then checks those handles without waiting after normal shutdown. It
exports cardinalities and parent state, never IDs, image paths, commands or tokens.
Unavailable or incomplete observations remain explicit. Process liveness alone
does not identify the holder of a failed file operation.

The optional samples are independent fresh trials, each with a separate artifact
directory. Every failure is retained and makes the step fail. The existing full
navigation set, trace/cancellation controls, protected browser flags, three-minute
test deadlines and twelve-minute workflow budget remain unchanged. There is no
removal retry, sleep, extra shutdown action or navigation replay.

Local browser-package race tests, the privacy controls, a real macOS private-home
WebSocket case, YAML/gate invariants and Windows AMD64/ARM64 compilation pass.
Compilation does not establish native Windows execution. [Source/log bindings](source-manifest.json)
record the local evidence.

Native Windows run `44265066746` on diagnostic source `f09b5eb` passes the actual
held-file sharing/refusal control and all 70 records: 46 ordinary first loads,
four Stop controls and 20 additional observed cleanup samples. All 20 samples
observe the parent exited. Four still observe one or two validated children
running at that point; removal nevertheless succeeds in every sample. This
confirms the parent-only completion boundary does not establish child exit, but
does not identify the historical removal failure's resource or cause. Artifact
`14410715059` SHA-256 is `133d39a0c4a17ddf252e1268a3484ffa850b3f11f4a4b1de6d01c76c800ee6f0`.

Native Windows run `44266839675`, exact `16afd2326f2303188e3c16a07a1a4633c538c8a7`,
completed all 100 unobserved trials and failed as required: trials 41 and 58
both loaded successfully with one request/navigation, then reported `unlinkat`,
Windows error 32 (`sharing_violation`), category `profile_child`. All 150 records
were retained. Artifact `14411052386` SHA-256 is
`786418709d562c69e7962559c4a8ac3acb40d83832985c68d1e9668d2fc6f69a`.
The reproduced failure establishes an actual sharing refusal, not its holder.

Further diagnosis refines fixed file categories and, only after a sharing
failure, asks Windows Restart Manager about that single refused regular file.
Only counts and fixed states leave the fixture. Native process creation-time
validation rejects PID reuse. A matching browser image is labeled as such; it
does not prove ancestry or ownership. Names, PIDs, image paths, timestamps and
session keys remain private. The query never calls shutdown/restart APIs and
does not retry removal. Unsupported/unavailable/no-holder results stay explicit;
absence of a reported holder does not establish absence of a lock. The native
held-file control also checks the probe's identification of its own process.

Current holder-probe changes require native Windows qualification. The actual
failed resource/holder and shutdown repair remain open. No production runtime
or dependency change is made.

Observed Windows run `44270302359` on the same `f15e3fd` source completes all
100 trials and fails on 12 sharing refusals (11 cache, one profile child).
All 12 failures observe the parent exited and one or two validated children
still running. Successful trials include 66 with no running child and 22 with
one, two or five running children. Liveness alone is therefore insufficient to
identify a file holder. In trial 71, Restart Manager reports one holder matching
the fixture Chrome executable; the other 11 report none at query time. This
positive image match still lacks captured-process membership correlation.
Artifact `14413080612` SHA-256 is
`2686c331c06fd4b0d5e5a77d2e75e9216392064a28fbdfd9ea76f15e56b470ee`.

The next probe keeps the validated CDP process ID/creation-time set in an
unexported map. A failure-time holder is counted as observed-owned only when its
native creation time matches both Restart Manager and that captured identity.
Only the match count is serialized. The native held-file control requires a
correct identity match and rejects the same PID with a different creation time.
This correlation change still requires native qualification; it is not a
shutdown repair.
