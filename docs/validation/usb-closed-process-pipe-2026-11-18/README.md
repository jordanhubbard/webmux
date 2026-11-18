# Closed process stdout during established USB relay teardown

Native Windows CI44297064754, job134776754451, reaches the end of its successful
trusted SSH byte relay, cancels it, then reports `read |0: file already closed`
from `finished` at ssh_test.go:286. Process termination can close stdout before
cancellation propagates to the relay's child context. The established relay
already accepts EOF, io.ErrClosedPipe and net.ErrClosed as transport closure;
add the equivalent os.ErrClosed sentinel returned by process stdout pipes.

The [receipt](receipt.json) binds the failing native source and exact repaired
source/tests. A deterministic transport reproduces both bare os.ErrClosed and
the wrapped process-pipe error without cancellation: both fail against the
parent implementation using Go's source overlay and pass with the repair.
Protocol, truncated payload and permission errors remain errors. The original
SSH test, host-key assertions, repetition counts and deadlines are unchanged.

Local darwin-arm64 Go1.27.3 targeted regressions pass twenty times under race;
the complete USB-forward race suite and vet pass. This does not substitute for
the pending native Windows SSH repetition. Audit429 and full CI/release gates
remain required; no native Windows repair or whole-roadmap completion claim.
