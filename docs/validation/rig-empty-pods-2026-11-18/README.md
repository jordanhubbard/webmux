# Canonical empty Rig pod selection

Runtime `771d310` fixes #528 on top of the retained `a252b1e` roadmap candidate.
WebMux previously populated pod choices from seat rows and required a seat in the
chosen pod before launch. An actual parent Go/Chrome/Rig 0.7.18 control reports
two canonical empty pods and zero members, while the browser disables existing
project selection.

The browser now reads only the explicitly selected project's current pod
definitions. Native discovery joins the active Rig ID/name and its supported
spec API behind the verified configured manager, then rechecks manager process
identity. It projects only project identity and pod labels. Discovery does not
observe an account or start a provider. It remains bounded and runs on selection
or explicit Refresh pods, not for every project's polling round. Legacy discovery
preserves its existing observed-seat contract.

Launch revalidates the current native namespace instead of requiring a seat row.
The UI admits stopped/empty projects for verification, blocks launch while lookup
is pending or unavailable, ignores late replies for another project and exposes
Refresh pods in the cell-launch dialog. Native launch-name admission remains
unchanged; canonical labels are not themselves launch authority. No default pod,
replacement Rig or replacement provider is inferred from missing evidence.

## Actual native acceptance

The final isolated Go/Chrome/Rig 0.7.18 case starts with empty research/workers
pods and an unrelated canonical Review_Team label. All three remain discoverable.
After the browser chooses workers, the native API removes that owned empty pod.
Launch is refused with zero members created. Refresh pods rereads the namespace;
the browser chooses research and creates its first actual native Codex member
in that exact original Rig and pod at the clicked cell (row 2, column 2).

The observed native arm64 executable matches the pinned launch plan digest.
A fresh native coding turn executes Rig identity and a private workspace probe.
Correlated native records join the original node/pod, read an unpredictable
challenge, write it in the chosen workspace and preserve the owner's model and
default full-access/never policy. No delegation call or native trust grant occurs.
Only one durable native launch record exists and no replacement topology is
materialized. Browser-reviewed checked Stop retains the viewer; owned controller,
native process, private daemon and tmux cleanup are independently verified.

Two earlier live driver attempts stopped before creation: an exact label locator
did not match the project selector, then the optional name field had not been
expanded. Both remain excluded and retained. Subsequent successful cases include
the visible Refresh control and the final canonical-label compatibility change.
The final raw receipt, selected native conversation and owned fixture files are
retained privately by hash. No authentication file is copied.

## Validation and limits

434 frontend tests, 234 helpers with zero skips, five strict TypeScript projects,
frontend lint/build, 272 installed lock identities and full server Go race/vet
pass. All eight actual legacy published-Rig browser cases and the maintained
one-failure/replacement-worker/original-daemon/final-cleanup control also pass.
Protocol and UI refusal controls are listed in [receipt.json](receipt.json).

The first hosted CI run on `d21722a` exposed a test fixture dependency: the pod
HTTP test supplied fake Rig and Node executables but inherited tmux from the
host. Its macOS race-test job refused discovery with HTTP 409. Running the
compiled race-test binary with only `/usr/bin:/bin` reproduces that failure.
The fixture now supplies tmux explicitly and uses only its own executable
directory as PATH; the same test passes without host tmux. This changes test
isolation only. The production availability check and the native runtime source
above are unchanged. The failed hosted run remains part of the evidence.

The browser uses private loopback auth-none and the native provider account stays
in place on macOS. This is first-member discovery/creation and a fresh coding
check, not selected profiles, named graphs, recovery/reboot or broader platform
acceptance. Those parent requirements remain open. The feature needs its own
hosted checks and review; no release or full-roadmap completion is claimed.
