# Temporary macOS release runners

CI and Packaging use the stable `macos-arm64` platform/artifact identifier. Their
hosted default is `macos-26`. Actual macOS version, architecture, and runner name
are recorded in the job log and summary.

For trusted push, release, or manually dispatched runs, a maintainer can set the
repository Actions variable `WEBMUX_MACOS_RUNNER` to a temporary runner's unique
label. Pull-request workflows ignore this override and keep the hosted default.
Do not label a self-hosted Mac as a hosted OS image it does not run.

Register one just-in-time, single-job ARM64 runner at a time on the authorized
worker, with that label. Keep its checkout and tool cache in a private temporary
runner workspace. No launchd installation is needed. CI has a manual dispatch
trigger so a workflow branch can be qualified on its exact commit before merge.
Use the full CI and Packaging workflows; do not replace their tests with a status
assertion or reuse artifacts from another source commit.

Homebrew checks refuse to proceed when a WebMux installation or qualification
tap already exists. Temporary test kegs are installed without linking into the
host's PATH and removed with their private taps afterward. Existing dependency
packages remain installed. Hosted-runner OpenSSL and Homebrew setup adjustments
are skipped on self-hosted machines. Service smoke tests use private service
labels and configuration directories.

After qualification/publication, verify that every temporary runner registration
has disappeared and delete `WEBMUX_MACOS_RUNNER`. Subsequent trusted runs then
return to hosted macOS without another workflow change. Retain logs and exact
commit/artifact checksums as release evidence.
