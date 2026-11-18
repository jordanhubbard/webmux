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

Normal qualification adds no process query before shutdown. An optional manual
20-sample run enables a read-only observation after each trial. It retains native
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
record the local evidence. Native held-file/process observation and the original
failure's actual resource/cause still require Windows evidence. This change is
diagnostic, not a repair claim; a later passing run does not explain the retained
failure. No production runtime or dependency change is made.
