# Native launchd fixture queries

The shared Node fake manager still starts an interpreter for every timed query.
CI [44458135080](https://github.com/jordanhubbard/webmux/actions/runs/44458135080/job/135338092460)
reproduced #324 after preparation had completed: the installer crash control
expected `did not remain running` but received a one-second manager-query timeout.
That does not prove a specific scheduler cause. The failed log remains retained.

The fixture now compiles a native Go fake manager before preparing each case.
Queries have no Node interpreter dependency. Cases retain independent state,
environment, command receipts and logs; the fake never calls system launchctl.
Readiness verifies the executing binary's SHA-256 before production checks begin.
Compilation is outside both readiness measurement and production query budgets.
The old Node fake remains solely for the explicit negative control.

The production checker is byte-identical. Its one-second state query, stable-PID
requirement, registered-crash refusal and registration retry behavior remain
unchanged. Existing controls still show that an unprepared 1.5-second fixture
fails the real one-second query; prepared queries pass. Permission errors and
actual two-second hung queries still fail closed.

A new control blocks PATH `node` after preparation. The old fake then fails
the same real checker; the native fake passes. The checker itself continues to
use its original absolute Node executable. This distinguishes removing fixture
interpreter startup from relaxing production behavior.

All seventeen affected Make/launchd controls and the full 253-helper suite pass
at default file concurrency, with no failures or skips on the Mac qualifier.
All five strict TypeScript projects pass. The existing private native launchd
replacement control also passes; no installed service is changed. The
[receipt](receipt.json) binds modified sources, the unchanged checker and failed
and passing logs. Hosted CI on the final source remains required before closing
the reopened #324.
