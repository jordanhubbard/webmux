# Retain timed-out Rig relay fixture evidence

Current scoped acceptance is reconciled in the [relay provisioning audit](../relay-provisioning-acceptance-2026-11-19/README.md). The original observations and pending-at-the-time statements below remain historical evidence.

The original [integration CI run](https://github.com/jordanhubbard/webmux/actions/runs/44471142427)
at `67019a3` passed its checked local Rig Stop browser case, then hit the
180-second Docker build limit while preparing the isolated SSH/tmux endpoint.
The relay browser/native checks never started. The original artifact records
the timeout but contains no Docker build output: the command wrapper logged
captured output only after a normal subprocess return. The actual build cause
remains unknown. See the [source and artifact receipt](receipt.json).

The wrapper now flushes the command and configured deadline before execution.
On timeout it records the partial stdout/stderr and timeout metadata, then
rethrows the original `TimeoutExpired` so the existing failure and cleanup path
continues. Partial UTF-8 at the cutoff is decoded with replacement. Ordinary
success, nonzero failure and nonzero inspection behavior remain covered.

Four real Python subprocess controls pass. The same controls against the
original inner wrapper fail specifically because timeout output is missing;
the three ordinary result controls still pass. The original body is extracted
unchanged by AST, with only its function name and explicit log parameter adapted
for invocation. These controls run before relay qualification in Ubuntu CI.
The changed workflow parses with Ruby's standard YAML parser.

The Docker build deadline remains 180 seconds, and every existing operation
deadline, fixture assertion and cleanup rule is retained. No Docker, native
Rig, SSH or coding fixture runs as part of these subprocess controls. The failed
original run is retained. Full relay qualification and build-cause diagnosis
remain open; this diagnostic repair does not qualify #318/#438 or the release.
