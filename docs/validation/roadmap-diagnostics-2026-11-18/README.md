# Roadmap diagnostic and provisioning composition

Compose pinned #489 with #491 Mosh cleanup probing, #492 Windows VT runtime
context/diagnostic workflow, and #493 signed Ubuntu source selection. All four
input commits are ancestors. Go application, retained VT, frontend and lockfile
source remain byte-identical to #489 at `29e1f02`.

The new Mosh workflow now supplies the explicit ephemeral CI context required by
the provisioning entrypoint. Canonicalize both operands of its system APT path
guard: macOS resolves `/etc` to `/private/etc`; the existing guard control fails
before this correction and passes afterward. No operator APT configuration is
changed locally.

Local qualification: all seven APT controls pass on Linux in an isolated
Ubuntu 24.04 container. On macOS six pass and the actual APT command control
skips. All nine tmux lifecycle/probe controls, five strict TypeScript projects,
application build and actual macOS Mosh/private-SSH/browser sign-in pass. The
Mosh receipt confirms all ten functional/privacy/cleanup flags. The container
exits and is removed. Workflow structural comparison preserves every existing CI
job, test, assertion and deadline, allowing only explicit provisioning context,
read-only VT context and its retained artifact. ARM workflow and lockfile match
the integration base exactly.

Upstream #491 has forty independently inspected passing native Linux samples;
#492 has a complete stock/guard native Windows pass, which does not reproduce or
repair the original VT crash. Those are input evidence, not qualification of
this new combined head. Exact combined hosted CI/packaging/diagnostics remain
pending. Keep #429, #450, #473 and broader roadmap acceptance open; no release,
main mutation, test suppression, unsupported runtime installation or audit waiver.
