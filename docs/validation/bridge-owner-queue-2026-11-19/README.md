# Owner-queue contract before the Rig/Agent Deck bridge

Refs #246 and #598. The bridge remains unimplemented. This source-bound review identifies a concrete prerequisite; it does not qualify shared topology, coding execution, completion forwarding or native identity binding.

## Native observation

Two identical `session send --queue --require-input-prompt` calls against one owned synthetic shell worker produce two different durable send IDs under the published Agent Deck1.16.39 binary on macOS ARM64. Both immediate replies say queued and not submitted. A subsequent native queue listing contains both IDs; one is typing and the other queued. These are queue states, not proof of delivery. The shell's append-only input witness contains only its startup row and no received message. No model or authenticated coding conversation is used.

This does not contradict the owner's per-send at-most-once guarantee. A second CLI invocation is a second independent send. A WebMux retry after a lost acceptance reply cannot assume that the owner will deduplicate it.

The private binary matches the previously verified official release SHA-256. Native owner Stop succeeds, the fixture payload exits, and a final process inspection finds no remaining private queue executable. An explicit check of the generated tmux socket reports no server running; the stale socket and inactive homes remain retained. The first setup used a socket path exceeding the Unix socket limit and never started its payload; it is excluded. Raw `passed:false` remains unchanged in the observation driver. The separate review verifies only the limited queue-record observation, not end-to-end bridge acceptance. Raw attempts, driver, source and review have private archive checksum readback.

## Published source and current WebMux path

The [pinned ordinary CLI queue path](https://github.com/asheshgoplani/agent-deck/blob/d62c2898d623e5353392a06f9a77c19f97b33438/cmd/agent-deck/session_send_queue_cmd.go) calls `sendqueue.Enqueue`. The [queue library](https://github.com/asheshgoplani/agent-deck/blob/d62c2898d623e5353392a06f9a77c19f97b33438/internal/sendqueue/enqueue.go) separately offers `EnqueueOnce` for keyed internal callers. Its existence does not expose a client operation key through the ordinary CLI send contract. Source bytes were downloaded by Git blob SHA from the pinned release tree and checked against their Git object hashes; the [receipt](receipt.json) records SHA-256 bindings.

Current WebMux calls the owner's guarded send path and then returns its new send ID. Its HTTP handler has no durable caller request identity before dispatch. If queue acceptance succeeds but the response is unavailable, the handler returns an error and the browser retains its editable instruction. Repeating that operation can therefore allocate a second native queue record. The native experiment establishes the allocation distinction; it is not a WebMux HTTP fault-injection test or a duplicate coding-execution claim.

## Required next implementation

[#598](https://github.com/jordanhubbard/webmux/issues/598) tracks an owner-scoped frozen send intent and durable exclusive consumption before the provider call. The browser must retain the original request UUID and selection through errors, edits and reconstruction. A same-ID/same-body duplicate returns its saved outcome; changed bodies and other owners cannot reuse it. A crash or lost response after possible dispatch stays uncertain and must not trigger automatic redispatch. Native queued/typing/submitted/landed outcomes remain distinct from broker intent state. Existing profile/host routing, stdin message transport and the owner's input-prompt guard stay intact.

The larger bridge still needs explicit Rig-seat/Agent-Deck-owner/profile/host/native bindings, one lifecycle owner, authorization/provenance and correlated completion receipts with duplicate handling. This review does not establish an atomic expected-native-CID dispatch operation. A read/recheck around a separate owner command cannot supply that atomic contract. No direct tmux typing, native queue-file mutation, installed coordinator update or inferred ownership is proposed.

No new Actions jobs were dispatched. The full roadmap and release remain incomplete.
