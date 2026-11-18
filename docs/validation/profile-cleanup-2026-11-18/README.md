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

Further unobserved sampling remains diagnostic, not a repair claim. The original
failure's resource/cause is still unknown, and a later passing run does not
explain it. No production runtime or dependency change is made.
