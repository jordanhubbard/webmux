# Durable coordination send intent

Issue [#598](https://github.com/jordanhubbard/webmux/issues/598) adds a caller UUID and immutable, owner-scoped intent before Agent Deck dispatch. The original provider, target, message and queue choice are frozen. An exclusive, synced dispatch claim is consumed before the provider call. Same-ID duplicates inspect the retained result; changed input or another owner cannot consume another dispatch right. A possible dispatch with no trustworthy result remains uncertain. Dismissal of a not-yet-observed UUID consumes the same exclusive claim, preventing a delayed original request from sending.

## Source and verification

The [receipt](receipt.json) binds the native experiment to `f9d94b3ca6c43864fa0be81393c59a8805d60373`. The full Go race run executed at `cdd24416858ea7b22ad19bff018a6e090ae3e593`; its entire server tree equals the native source. Subsequent main integration changes documentation and the maintained browser creation-response observer; production and frontend unit-test sources remain equal. All eight original browser cases pass again on the integrated revision recorded in the receipt, with zero skips, failures or flaky outcomes and unchanged assertions/deadlines.

Local checks pass: 548 top-level Go race tests with 11 recorded skips, HTTP API vet, five strict TypeScript projects, 472 frontend tests, lint and frontend build. Windows AMD64 test cross-compilation passes; it is not Windows execution. Linux ARM64 controls run in an existing pinned, network-disabled, read-only container with a private writable temporary directory. They exercise immutable files, process races, reconstruction and HTTP boundaries; their fake provider is not native Linux coding acceptance.

The regression run overlays only the previous `openrig.go` handler from `567f0e9` into the current test tree. It fails with two owner send calls where one is required. This is a previous-handler negative control, not a run of an original released executable.

## Actual native queue and browser proof

A private official Agent Deck 1.16.39 binary, actual Go WebMux server and Chrome exercise one owned synthetic shell. Two distinct intended instructions produce exactly two actual producer calls and two native queue records:

- The first HTTP reply is dropped after actual native queue acceptance. Reload restores the original owner send ID; repeating the original UUID returns it. A changed body is refused without enqueueing again.
- The second native CLI reply is captured and dropped after real queue acceptance. WebMux retains uncertainty without promoting the captured native ID into a receipt. Killing only the private controller with SIGKILL, starting the same binary/home, reloading and repeating the original UUID produce no extra native queue record. The original payload survives the controller crash.
- A third request is held before upstream dispatch. Explicit dismissal writes a durable claim; delivering the original afterward returns cancellation without a third producer call. A fresh UUID with another owner/home scope is refused before any ledger write.

The ledger contains two intents, two dispatch claims, one dismissal claim and two results, all mode 0600. Explicit `-p default`, stdin message transport, the input-prompt guard and the queue flag are preserved despite an unrelated ambient profile. Native queued/typing state is not proof of delivered or executed work. The WebMux build identity is a clean-source producer self-report, not an attestation.

The first native attempt assumed the wrong generated group and never sent. The second qualifier asserted completion from an immediate in-flight banner and failed before its second intended producer call. Both original false results remain false. The successful fresh fixture uses the actual native group and waits for the authoritative HTTP outcome; scenario counts and deadlines are unchanged. The UI now distinguishes a request in progress from a verified undispatched intent. Bounded cleanup also observes late children of the uniquely named private executable.

Native owner Stop succeeds, the synthetic payload exits, and final process/open-file controls find no remaining private workers or controller. Retained tmux socket inodes have no running servers; no evidence files were deleted. The Linux container is removed. Source, drivers, raw successes and failures, selected ledger/queue records, logs and binaries have private checksum readback; credential/config stores and whole fixture homes are excluded.

## Scope

This closes the WebMux request-identity prerequisite. It does not implement [#246](https://github.com/jordanhubbard/webmux/issues/246), establish atomic native occupant/CID binding, run a coding model, qualify authenticated recovery or test a host reboot. The private application uses loopback auth-none with synthetic instructions. Existing lifecycle ownership, authorization and provider routing remain in force. Preserve the WebMux home, including its instruction ledger, across upgrades. The full roadmap and release remain incomplete. No Actions jobs were dispatched.
