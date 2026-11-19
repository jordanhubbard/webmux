# Observe the Make client-install timeout

Issue [#542](https://github.com/jordanhubbard/webmux/issues/542) retains a real
macOS helper failure at the existing 30-second Make install limit. Later passing
runs did not identify its cause. This workflow collects a controlled comparison;
it does not repair or waive the failure.

The manual `macOS client install diagnostics` workflow uses two separate,
disposable GitHub-hosted ARM64 Macs. Both start with a new private `GOCACHE`.
The warm case first builds the real client with a separate 180-second diagnostic
bound. Each case then runs the original `npm run test:helpers`, with its normal
file scheduling, real Make client build/install/uninstall, 30-second install
limit and original assertions. A failing suite or observer returns nonzero and
fails its job; artifacts are retained on failure. Neither case is a retry for a
green result.

The Go proxy is selected through a private PATH prefix: Make assigns `GO := go`,
so setting an environment `GO` would not instrument its build. The proxy forwards
the original arguments and exit result. It records only
closed action/workload labels, existing trimpath and CGO settings, process IDs,
timestamps and elapsed time; it adds no Go flags. The observer samples descendant
process names, parent IDs and CPU times, plus host load averages. For observed
compiler descendants only, a separate bounded query extracts package IDs. Raw
arguments, source/output paths and other processes' arguments are not recorded.
Compiler queries are capped at 128 PIDs, 1 MiB and three seconds; unavailable
samples remain labeled as such. Source identity includes the effective CGO
setting and worker CPU count. Process samples are capped at 8 MiB, with dropped
samples counted. It bounds its own full-suite observation at 900 seconds and
cleans up only its freshly created process group, including when its command
leader has already exited. Descendants get a bounded three-second SIGTERM grace
before any remaining group members receive SIGKILL; reaping the leader alone is
not treated as group cleanup. Existing individual test deadlines remain unchanged.

Before the diagnostic, offline real-process regression controls exercise a
SIGTERM-ignoring descendant after timeout and after successful leader exit, plus
preservation of a nonzero command result. These use private Python processes;
they do not run Go, Make install, a service or an authenticated coding session.
Additional controls verify that only observed compiler package IDs survive
parsing, queries stay bounded, and the real proxy forwards arguments and exit
status without recording private arguments or unrelated environment values.

The script refuses ordinary user hosts, self-hosted runners, dirty source and
output outside a new directory below `RUNNER_TEMP`. Headroom is checked before
dependency installation. Existing helper tests may exercise their own temporary
launchd fixture when a GUI domain exists; they do not change the installed
WebMux service. Missing-domain and other optional skips remain skips.

Uploaded artifacts contain source/tool identity, outcome summaries, process
samples, Go timing events and helper output. They exclude the cache and the warm
client executable. No native coding credentials are used or copied.

Interpret the source, cache condition, actual test result and any skips together.
The fresh cache conditions, changed Go lookup path, proxy startup and sampling overhead do not recreate
the original worker or prove its historical cache state or scheduler cause. A
passing diagnostic is not a repaired #542, authenticated recovery or release
acceptance. Keep the original failures and compare actual build progress with
install/control time before proposing a repair.

## Corrected setup comparison

Original [run 44597559558](https://github.com/jordanhubbard/webmux/actions/runs/44597559558)
at `6dff348` retained a cold client timeout and a warm client pass, alongside
three other failures caused by missing tmux and differing Node patch versions.
It remains a failed, confounded comparison.

After the disposable-runner prerequisite and Node pin were reviewed and merged,
[run 44609660909](https://github.com/jordanhubbard/webmux/actions/runs/44609660909)
at `96af7d2` used Node 24.22.2, Go 1.27.4 and Python 3.15.2 in both conditions:

| Condition | Full helpers | Client-install case | Final Go build |
| --- | --- | --- | --- |
| Cold | 265 pass, 1 fail, 0 skip | fails at 30,008.903 ms | SIGPIPE after 31.138 s |
| Warm | 266 pass, 0 fail, 0 skip | passes in 2,689.473 ms | succeeds in 2.331 s |

The final cold build has sampled compiler descendants; the warm build has a
sampled linker. Those originals do not identify compiler packages or the active
trimpath/CGO flags. The added package/flag diagnostics address that gap without
changing the workload. They have not yet produced a new native comparison.
The separate warm preparation took 16.503 seconds. Both original artifacts are
retained with GitHub digest and private readback verification. This one corrected
comparison supports a cold-only deadline reproduction; it does not establish the
historical worker's cache state, a particular bottleneck or a product repair.
