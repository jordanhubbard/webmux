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
closed action labels, process IDs, timestamps and elapsed time; it adds no Go
flags. The observer samples descendant process names, parent IDs and CPU times,
plus host load averages. Process samples are capped at 8 MiB, with dropped
samples counted. It bounds its own full-suite observation at 900 seconds and
terminates only its freshly created process group on an observer failure or
timeout. Existing individual test deadlines remain unchanged.

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
