# Rig Stop relay fixture build timeout

On original source67019, CI44471142427 Linux job135380639885 passes the real local
checked-Stop browser case. Its following SSH relay fixture times out building
the disposable Docker image after the original180-second bound, before SSH or
the relay browser case starts. The original report,384-byte fixture log, full
job log and artifact are retained. The underlying build cause is unknown: the
old logger writes output only after subprocess.run returns, discarding the
TimeoutExpired object's partial streams. A later pass cannot establish that
historical cause or its repair.

The [receipt](receipt.json) records a real-child negative control executing the
original nested run function from its parent AST. The child emits both streams
before timing out; the exception contains them, but the original log is empty.
This is an observed logger failure, without rerunning Docker or native Stop.

Command evidence now flushes a start record before execution and keeps bounded
stdout/stderr on success, nonzero exit and timeout, with elapsed time and
observed/omitted byte counts. Each stream retains at most1MiB of its original
bytes (head/tail), plus its omission marker and JSON escaping. Completed command
return values remain untruncated, and original timeout/spawn errors propagate.
Subprocess capture memory behavior is unchanged; this is a bounded-log change.
Four actual-child controls and Python syntax compilation pass locally.

The report marks the failing phase and whether native browser execution was
attempted. A Docker client timeout explicitly leaves daemon build completion
unconfirmed. Label-scoped image/container cleanup and private-key cleanup remain;
there is no daemon reset, broad kill or cleanup-success claim for an unobserved
build. Private keys are excluded from the Docker context and removed before
upload as before. No operator/native authentication state is read or copied.

The180-second build deadline,120-second browser deadline, actual SSH endpoint,
remote persistent process, native Stop ownership/refusal/sibling assertions and
result/cleanup acceptance remain unchanged. The new controls run before the
existing Linux qualification. A changed-diagnostic native run is still required;
this preparation does not repair or qualify the failed Docker build. Host loss
before the timeout handler can still prevent output retention.

Windows' required application gate passes inside the original failed CI. That
scoped positive and the retained Linux failure do not make the overall CI green.
Refs #545, #477, #438, #452, #543.
