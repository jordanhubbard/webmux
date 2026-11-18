# Native Linux observer signal delivery

[Receipt](receipt.json) binds clean 646c4ce source to actual AMD64 and ARM64
kernel/runtime qualification in [run 44424131750](https://github.com/jordanhubbard/webmux/actions/runs/44424131750).
Both architectures pass the native kernel controls and six real Codex 0.169.0
startup cases. All 24 subprocesses remain alive until intentional cleanup,
produce complete bounded diagnostics, and stop without panic or forced kill.
These use disposable unauthenticated homes and submit no model turn.

The diagnostic run on 6cea5d8 had passed policy assertions while every guarded
policy process panicked in its signal thread and later required SIGKILL. Those
original artifacts remain unchanged. The new strict audit rejects both of them;
passing policy snapshots alone are insufficient startup evidence.

The helper had allowed unnamed Unix socket pairs but denied the addressless
send/receive forms needed to use them. The repair permits those read/write-
equivalent forms with restricted flags and canonical NULL address/length words.
Endpoint creation, connections, addressed I/O, message/descriptor transfer,
child creation, later exec and filter replacement remain denied. Native kernel
tests exercise actual pair send/receive, nonblocking reads, refused address/flag
forms and message operations, plus all previous enforcement controls. The pinned
native binary is unchanged between the old and repaired cases for each
architecture. The signal-pair change removes the observed panic and restores
graceful cleanup while the exact policy snapshots remain unchanged.

Seven disposable subprocess tests cover noisy startup/cleanup, bounded tails,
exit versus cleanup status, opt-in capture, launch/initialization failures and
strict refusal of damaged processes. A panic flag survives tail truncation.
The original RPC and cleanup bounds remain unchanged. Late MCP stdio and HTTP
integrations still start in positive controls and remain blocked under the
guard; account observation starts no integration.

The earlier ARM64 stdout EOF had no stderr, so this receipt does not prove the
exact cause of that historical exit. It proves the recorded signal defect and
repair. Authenticated Linux coding/recovery, native Windows acceptance, review,
full CI and the broader release roadmap remain separate open requirements.
