# Run the positive application fixture outside the hosted runner job

The hosted job's breakaway restriction is now observed directly; the unchanged
application gate passes on the private native target. The existing native screen
collector already runs its synthetic ConPTY producers in a short-lived SYSTEM
task on disposable hosted VMs. The application fixture now uses the same task
boundary in the manual Windows terminal workflow. Hosted execution of this new
launcher remains pending; this preparation is not a passing native receipt.

The launcher refuses non-Actions and self-hosted environments, requires the
actual checkout and a fresh output directory below RUNNER_TEMP, and builds the
application before registering the task. It binds the image and unchanged test
body by SHA-256. The worker verifies its actual SYSTEM identity, uses private
cache/configuration state and a loopback-only app, and runs the original native
gate with its original assertions and deadlines. No coding CLI, sign-in state,
model turn, service, account creation or protection change is involved.

The prebuilt-image path is available only to this hosted Windows fixture and
requires a regular absolute file with the matching digest. Other browser runs
keep their original build path. Five-minute task lifetime and bounded parent
observation contain the worker; the task is stopped/unregistered in `finally`.
The successful gate still requires authenticated original provider Stop and
child exit before its independent expiry. Uncertain outcomes fail and retain
the fixture's recovery state. No broad provider kill is added.

Only the public evidence subdirectory is uploaded: source/image/test bindings,
the actual account-scope boolean, reporter output, traces and the fixture image.
Private caches, launcher state and session/recovery-key storage are excluded.
This is synthetic SYSTEM application coverage, distinct from the existing
authenticated user-account coding/recovery qualification on the private host.

Local admission passes both prebuilt-image controls, all five strict TypeScript
projects and all 255 helper tests at default concurrency. Original helper
arguments retain their order. The first full run failed because its supplied
PATH omitted macOS `lsof`; the observer correctly refused identity verification.
The diagnostic records ENOENT, and the focused observer and corrected full run
pass with the normal system PATH. Failed logs remain retained. Windows PowerShell
parses the parent and generated worker without errors; no task is executed on
the private host by that static check.

The launcher guard uses GitHub's documented
[RUNNER_ENVIRONMENT](https://docs.github.com/en/actions/reference/workflows-and-actions/variables)
distinction between hosted and self-hosted runners. Exact hosted execution,
review and integration remain required before adopting this fixture in full CI.
