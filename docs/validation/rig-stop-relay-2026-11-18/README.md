# Checked Stop across an actual SSH relay

This #438 qualification exercises the published local Stop effect across real
OpenSSH, with a native macOS ARM64 WebMux/Rig controller and a separately hosted
Linux ARM64 SSH/tmux endpoint. The native Rig terminal's owner types `exec ssh`
through the WebMux browser. The remote tmux owner already runs its persistent
sleep process. This fixture does not convert the WebMux agent creator to native
ownership, authenticate a coding provider or establish physical-host failure
recovery.

The actual browser review states that no remote provider Stop is performed and
requires its separate confirmation checkbox. After confirmation, the selected
local tmux seat disappears and the observed local SSH PID exits. The sibling
keeps its PID; independent authenticated SSH probes confirm that the remote
tmux pane keeps its original PID and `/usr/bin/sleep` executable. A duplicate
request returns the consumed Stop receipt. The original WebMux viewer, grid
position and transcript remain, including the marker from the remote pane.

The final native case passes in 6.1 seconds (10.8 seconds including startup),
with no skip or retry. `results.json` records runtime versions, the controller
binary hash, harness/product source hashes and the private browser-proof hash.
Application sources match the `d871ac6` candidate; qualification source bytes
are bound separately. Strict TypeScript coverage, the frontend build, Python
compilation and workflow YAML parsing pass. Hosted exact-commit acceptance is
pending; this evidence does not mark #438 or the full roadmap complete.

The driver pins an Ubuntu OCI index supporting AMD64 and ARM64 and records the
actual fixture image and SSH/tmux package versions. It generates a fixture-only
client key and an ephemeral container host key, verifies host identity, exposes
only a loopback SSH port and uses no operator credentials or source mounts.
Cleanup checks the unique ownership label before removing its container/image,
removes the generated client private key, and retains diagnostics and receipts.
The driver requires the browser receipt and every acceptance/cleanup check;
skipping the browser case cannot pass qualification.

With the frontend built, Node 24, qualified local tmux, Docker and the published
Rig CLI available:

```sh
python3 scripts/qualify-rig-stop-relay.py \
  --cli "$WEBMUX_OPENRIG_CLI" \
  --output "$PWD/_build/rig-stop-relay-unique"
```

Use a new output directory for each run. `--chromium /absolute/browser/path`
can select an installed browser. Linux CI runs this driver after the existing
native managed/adopted/replacement Stop case, preserving its assertions and the
legacy compatibility lane. Existing claimed WebMux SSH relays remain ineligible
for Stop; native launch conversion and original policy/host semantics remain
required under #438.
