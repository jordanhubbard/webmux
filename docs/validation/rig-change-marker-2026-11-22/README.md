# Rig cancellation fixture PID publication

Issues [#633](https://github.com/jordanhubbard/webmux/issues/633) and
[#642](https://github.com/jordanhubbard/webmux/issues/642) retain intermittent
`TestChangeWaitCancellationTerminatesHungCLIChild` startup failures. The latest
full Node 24 suite reproduced the unchanged failure at 0.41 seconds, despite
the original five-second marker wait. Its log remains privately retained in
`_build/coordination-ssh630/go-test-final.log`.

The fixture used `echo $$ > marker`. Shell redirection creates `marker` before
`echo` writes its contents. The Go observer breaks on any successful ReadFile,
including an empty file; an early empty read therefore produces the observed
failure path without using the five-second budget. A partial numeric PID could
also name the wrong process during cleanup.

The fixture now writes a sibling `.pending` file, closes it, then publishes it
with `/bin/mv -f`. The original observer, five-second startup deadline,
three-second cancellation deadline, one-second child-exit deadline and actual
process termination assertions remain unchanged. No production code changed.

The [causal control](control.py) uses a pipe barrier to stop the writer after
redirection opens its target but before the PID write. With direct publication,
the original observer can successfully read zero bytes and stop without a PID.
With sibling-file/rename publication, the destination remains absent until a
complete exact writer PID is available. [Both control results](causal-control.json)
are retained. Run `python3 control.py` on a POSIX host to reproduce them.

This proves a race in the original fixture and removes that race. Earlier failed
invocations did not retain marker contents, so their exact interleavings remain
unproven. Passing reruns do not reconstruct those historical observations.
