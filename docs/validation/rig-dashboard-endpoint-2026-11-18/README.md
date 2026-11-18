# Bind the diagnostic dashboard to the native fixture daemon

Hosted run44372260604/job135041940170 retained the native dashboard at Explorer
instead of Topology. The new diagnostics recorded one resize frame and no input
before readiness. Its actual native screen targeted `http://127.0.0.1:7433` and
reported that daemon stopped; the fixture had allocated a different ephemeral
port. This rules out early input for this observed failure. It does not prove
why native recorded-home endpoint discovery fell back in that run.

The fixture now sets `OPENRIG_URL` to its daemon's explicit endpoint after native
startup, preserving subsequent Rig enrollment identity. Its CLI wrapper records
only the endpoint selected at TUI entry. The browser requires that endpoint to
match the original fixture daemon and retains both values in its diagnostics.
No readiness deadline, native input sequence or daemon lifecycle assertion was
relaxed. No replacement daemon is started to satisfy the dashboard check.

Actual Go controller, installed Chrome and pinned published Rig0.6.14 pass all
eight native browser cases together, including the preceding managed-agent
case, topology/system navigation, reload attachment, detach and daemon health.
The existing bounded real-wheel fix from #526 is included; it remains separate
from this routing correction. Failed hosted and successful native receipt/log
hashes and exact source bindings are in [receipt.json](receipt.json). Current-head
hosted checks and normal review remain required; this is not full roadmap or
release completion.
