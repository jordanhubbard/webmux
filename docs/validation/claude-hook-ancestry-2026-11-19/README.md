# Native macOS Claude hook ancestry

Refs #583, #318. Base: `73a64145bb43196e2bc50fbdef661d05e3e25e32`.

A controlled private PATH helper delayed `ps` by 800 ms. The receiver accepted the token and observation and authenticated the peer PID, then rejected ancestry after the existing 400 ms process-list timeout. This demonstrates a failure mechanism; it does not establish the cause of the first historical failure.

On macOS, read each authenticated peer ancestor directly from `kern.proc.pid`, retaining the 400 ms admission budget and 64-hop bound. Linux retains its original function unchanged. Hook I/O and observation deadlines, token validation, unrelated-process refusal and owned endpoint lifetime are unchanged.

The new native test launches a real child through the private Unix socket while PATH contains a failing `ps` helper. Identical test bytes fail on the original implementation and pass with the kernel lookup. Existing private-channel token/process/lifetime controls pass. This is actual native process/socket qualification, not an authenticated Claude conversation or reboot acceptance.

The full native macOS ARM64 server race suite and vet pass; Linux AMD64 recovery cross-compiles (not native Linux execution). See receipt.json for counts and source hashes. The first full runner accidentally selected host Node26 and failed the unrelated native-create fixture; that log remains retained. Correcting only the runner PATH to pinned Node24 passes the original inventory. No test limits or assertions changed.

Raw native/fault/regression logs and bounded process observations remain in the private qualification archive. No hosted Actions run was requested for this fix.
