# Make target and roadmap audit, 2026-11-26

This is an implementation and qualification checkpoint for #762 and #763. It
does not complete the remaining roadmap, qualify every platform, or authorize a
release. The starting revision was `7420061436fa914f463a3e5573fdd0af7d212413`.

## Changes

- `configure` initializes missing application/authentication configuration from
  defaults, parses YAML before changing files, validates supplied scalar values,
  and preserves unrelated settings. Existing credentials are never printed.
- The application-directory Makefile forwards every public root target,
  including `build-client`, `configure`, packaging, and split test targets.
  Delegation also works when the checkout path contains spaces.
- The bootstrap selects qualified Node 24.x even when the ambient Node is newer.
  The former minimum-only check accepted Node 26, while native coordination
  explicitly refused it, causing the full test suite to fail before its intended
  native-create assertions. Go's minimum-version policy is unchanged.
- Playwright browser downloads use a private checkout cache by default, honoring
  explicit overrides. Browser readiness checks rendering and context reopening
  for managed, fallback and explicitly selected executables.
- Managed-service status explicitly reports an absent job. A launchd start's
  bootstrap fallback uses the configured service domain.
- Browser profiles and transfers use a private WebMux subtree rather than
  creating `webmux-*` entries directly in the account home. See
  [browser storage and migration](../browser-companion.md) for locations and
  the offline saved-profile migration requirement.

## Verification

Private logs are retained in `_build/make-audit-2026-11-26/` and
`_build/release-targets-767/`; they are qualification evidence. Make prerequisites
are shared, so exercising a parent target also
executes its actual prerequisite recipes. Fixture checks are distinguished from
native commands below.

| Targets or behavior | Evidence and boundary |
| --- | --- |
| `build`, `build-server`, `build-client`, `deps`, toolchain targets | Native macOS ARM64 build passed. The nested `build-client` command also passed. |
| `package`, `package-server`, `package-client`, `package-assets` | All three native archives built successfully with matching SHA-256 checksums. The combined archive passed a relocated standalone runtime smoke test. This does not qualify signing, notarization, other architectures, or publication. |
| `configure` | Actual fresh-home command passed. Regression controls cover changed ports, IPv6, preserved settings, invalid values, malformed YAML, and a home path containing spaces. |
| `start`, `restart`, `status`, `stop` | Public commands passed against a private native manual server: healthy API, running native PID, a different PID after restart, retained configuration, and clean shutdown. The fixture uses the separately built image and explicitly selects manual control. |
| `install`, `install-client`, `uninstall`, `uninstall-client` | Native client install/uninstall regression passed with a private bindir and server tooling disabled. |
| `install-all` | Native macOS private install-all/uninstall-all smoke passed with a client bindir containing spaces, real launchd registration, active PTY shutdown, transcript drain, restart and cleanup. Parallel-order and failed-build preservation controls also passed. Build prerequisites were qualified separately. |
| `install-server`, `uninstall-server`, `uninstall-all` | Native private service smoke passed in gui/501 after that domain became available. No operator service was restarted or removed. Initial unavailable-domain failures remain retained below. |
| `install-agent-deck`, `install-optional-agent-deck` | Qualified-version/idempotence fixture controls and the actual default optional-install skip passed. No existing integration was replaced. |
| `clean` | Actual recipe removed all five seeded artifact/dependency trees in a private application directory. No primary build or installed service was removed. |
| `help`, `check-tmux`, `check-guacd`, `changelog-init` | Actual commands passed; existing changelog retained. |
| `lint`, strict TypeScript checks | Passed on the implementation checkpoint. |
| `test`, `test-unit`, `test-e2e` | The complete unmodified macOS GUI `make test` at `2cc3a26d` exits 0: Go/race/bundled-library and strict TypeScript stages pass, all 500 frontend and 297 helper tests pass, and E2E has 1,418 passes, zero failures and 22 existing opt-in/platform skips. The private GUI job uses the normal app resource role (`ProcessType=Interactive`); see #770 below. This qualifies this native macOS run, with the skipped/native-platform requirements still open. |
| `release`, `release-minor`, `release-major`, `release-publish` | The actual Make recipes and release scripts pass isolated local-Git transition controls with a private bare origin and substituted application tests/GitHub service. No real publication was invoked. See the release-control follow-up below. |

The delegate regression enumerates all 39 public targets, including names with
digits such as `test-e2e`, and runs each against an isolated parent fixture. Eight
toolchain controls, 15 Make controls and seven Playwright/browser-planner controls
passed. Native browser unit tests and real Chrome temporary-profile cleanup
passed. Windows ARM64 browser tests also
cross-compiled; this is not native Windows qualification.

The first E2E attempt explicitly selected system Google Chrome. An independent
minimal Playwright probe reproduced its exit after the first isolated context
closed. The preflight now rejects this browser. Managed Chromium initially could
not install because the default cache was a broken link to an unavailable volume;
the checkout cache avoids that operator-specific path without changing it.
Managed Chromium then installed and passed rendering/context-reuse readiness.
The initial managed retry was blocked by the interrupted run's orphaned private
server on port 18080; its executable, parent and fixture home were verified
before its wrapper was terminated gracefully and the port checked free. A later
full run hit the USB command-copy frontend test's five-second deadline during
concurrent cross-compilation (498 passed, one failed); its isolated 15-test repeat
passed. The subsequent run without overlapping compiler work passed all 499
frontend tests. #765 retains the original timing failure and its limits.
The interrupted system-browser run is preserved as failed evidence, not counted
as final qualification.

## Native platform limitations

Initial native Make service installation selected the user domain because no
GUI domain was accessible. Both it and a minimal private `/bin/sleep` job failed
with `Bootstrap failed: 5: Input/output error`. Those failures remain retained.
The service smoke helper now emits the fixture's native installation log before
cleanup, preserving the actual failure instead of only an assertion message.

A later probe found `gui/501` accessible. Both `--make` and `--make-all` native
service smoke runs then passed real installation, startup/UI, guided restart,
active PTY shutdown, transcript drain, restart and uninstall cleanup. The latter
also installed and removed the actual client binary in a private bindir
containing spaces. These use the separately qualified built images and private
service labels, not the installed operator service. Logs are
`service-gui-recheck.log` and `service-install-all.log` in the audit directory.

The earlier completed remote `make test` exited 2 with 1,417 E2E passes, one
saved-cookie failure and 22 opt-in/platform skips. Its failure and the later
GUI-role failure are retained as separate runs.

Earlier saved-cookie restart qualification failed on both the changed code and
unchanged baseline browser source. A minimal real-Chrome probe reproduced cookie
loss after graceful close with Keychain OSStatus `-25308`
(`errSecInteractionNotAllowed`) and `Encryption is not available`.

The caller's security session matters: status-only Security.framework probes
report the remote caller as remote/non-graphical with a locked default Keychain,
while an owned one-shot GUI job reports graphical/non-remote with an unlocked
default Keychain. Repeating the remote probe still reports locked. The same
minimal cookie probe passes inside the GUI job. The earlier GUI-domain-available
recheck ran from the remote session; domain availability did not change its
security context. No Keychain items or policies were read or changed by these
status probes.

A clean GUI `make test` at `73baf25f` passed 499 frontend and 297 helper tests,
then failed in browser startup/navigation and accessible controls with the
managed full Chrome backend. It was interrupted after reproducible failures;
the owned runner performed cleanup and port 18080 was verified free. This is an
interrupted failing batch, not another completed full-suite run.

An image comparison in the same GUI domain and with the same native test binary
then passed both frame controls and saved-cookie restart with installed Chrome
`158.0.8127.37`, but failed both with managed Chrome for Testing `158.0.8127.18`
(navigation/action timeouts). A focused E2E run at `73baf25f`, explicitly selecting
installed Chrome for the companion and the managed headless shell for Playwright,
passed all nine accessible-control, authentication callback and saved-profile
tests. The fixture now prefers the installed macOS companion browser, preserving
`WEBMUX_BROWSER_EXECUTABLE` overrides and the managed fallback when absent.
The precise cause of the managed image's GUI failures remains unproven. These
focused passes do not qualify the complete Make suite. Receipts and logs are
under `_build/release-targets-767/`, including
`native-gui-browser-comparison.json`, `gui-installed-e2e.json` and
`gui-make-test.json`. Browser encryption, sandboxing and Keychain policy were
not weakened. The changed Snap data location has separate native acceptance below at its recorded source.

## Screenshot resize input regression: #769

A default GUI `make test` at `8c7fbe22` passed Go, strict TypeScript, all 499
frontend and 297 helper tests, browser callbacks, docking and saved-profile
restart. The browser workspace test then failed waiting for the drag signal.
Its trace showed viewport heights changing 373 → 240 → 373 near the gesture.
The batch was interrupted after a deterministic component regression reproduced
the mismatch; the original failed test and partial batch remain retained.

The component regression keeps a decoded 240-pixel screenshot while the newest
reply describes a 373-pixel viewport. A visible drag press at y=40 was dispatched
at y=62.17, outside the fixture handle. Pointer and wheel conversion now use the
decoded screenshot's natural dimensions, with response dimensions as a fallback
before an image is decoded. The test checks press, release and wheel coordinates.
No deadline was changed. At `2cc3a26d` the actual browser workspace case passes,
including drag and subsequent dialog, clipboard, file, navigation and tab checks.
The complete Make run also passes in the GUI app resource role described below.

Private receipts: `gui-native-make.json`, `gui-native-make-interruption.json`,
`browser-image-coordinates-before.log` and the owned GUI job directory under
`_build/release-targets-767/`. The original fixture attachment is retained under
`webmux/test-results/run-2bc10958-ca3f-4be3-ae60-7d10e37d6ab6/spec-11/`.

## Complete native Make run and qualifier role: #770

The unmodified `make test` at `2cc3a26d249f9b04e17e0a85f49466128ba47855`
completed with exit 0 in `gui/501`, with unchanged source and the private job
unregistered afterward. All 500 frontend and 297 helper tests pass; E2E has
1,418 passes, zero failures and 22 original opt-in/platform skips. Those skips
retain their actual native integration and hardware requirements.

The preceding complete GUI run at the same source exited 2: 1,417 E2E passes,
one underline-v2 replay deadline failure and the same 22 skips. Its first 1,355
replay controls passed. The private GUI wrapper had omitted launchd's ProcessType.
The installed `launchd.plist(5)` manual describes the resulting Standard role's
CPU/I/O throttling. An instrumented copy of the unchanged case established:

| GUI wrapper role | Result at the original 30-second deadline |
| --- | --- |
| Standard (implicit), first control | Timeout; 1,024 callbacks completed by 28.9 seconds. |
| Interactive | All 256 reference controls and 2,120 callbacks completed in 12.59 seconds; all assertions passed. |
| Standard (implicit), reversal | Timeout again; 192/256 reference controls completed by 28.0 seconds. |

The original uninstrumented test then passed in 12.8 seconds under Interactive.
In the final whole Make run it passed in 12.7 seconds, and all 1,356 replay cases
passed. Only the task-private GUI wrapper's resource role changed: no product or
test source, test concurrency, deadline, assertion or retry policy changed.
The original failures and the rejected diagnostic setup (a grep matching no
tests) remain retained separately. This is a qualifier environment correction,
not a terminal parser repair.

Receipts: `gui-interactive-make.json`, `gui-interactive-make-summary.json`,
`snapshot-role-comparison.json` and `gui-snapshot-original.json` under
`_build/release-targets-767/`. The [sanitized receipt](make-target-audit-2026-11-26.json)
binds the source, command outcomes, logs and local archive hashes.

## Current native packaging and lifecycle

The same frozen source passed the actual public command
`make package package-server package-client`; all three archive sidecars match.
The combined archive passed relocated standalone runtime smoke, and the newly
built images passed public manual start/status/restart/stop and private GUI
combined install/uninstall. The installer check includes active PTY shutdown,
transcript drain, restart and a client bindir containing spaces. Build prerequisites
were qualified separately before lifecycle helpers selected the prebuilt images.
Earlier archive bytes were preserved before the public package recipes replaced
the local outputs. These are local candidate archives, using the development
version 1.4.5; published release assets retain their own separate identities.

Receipts: `gui-native-targets.json`, `native-targets.json`, and
`native-targets-__4xobqn/archives.json` under the private audit directory.
The operator service and its real terminal sessions were preserved.

## Home-directory relocation

Seven existing top-level directories were inspected and found to have no open
files immediately before relocation. The saved-profile directory moved to
`~/.config/webmux/browser/profiles/`; four temporary browser directories and two
transfer directories moved below `~/.config/webmux/browser/tmp/`. All 4,208
entries retained identical file hashes, permissions, and symlink targets. Nothing
was deleted. The private relocation receipt contains paths and counts, not
browser data, and remains outside version control. Top-level `~/webmux-*`
directories were absent after relocation and after the complete E2E run. The
existing operator service was left undisturbed; the source fix in PR #766 has not
been deployed to that service.

The [2026-11-28 browser-storage acceptance reconciliation](browser-storage-acceptance-2026-11-28/README.md)
recovers the original successful relocation execution/receipt, verifies the
actual native GUI log and Snap archive hashes, and binds the unchanged storage
sources to current main and v1.4.7. It completes the scoped source/data acceptance
under #763 while retaining installed-service and broader platform/roadmap limits.

## Remaining roadmap scope

### Release-control follow-up: #767 and #768

The real preparation failure path was reproduced independently: after the test
command failed, the helper returned to `main` with dirty release edits and
deleted the release branch. Preparation now retains the branch and edits, with
no release-branch push or PR creation on test failure.

Publication now requires the latest exact-main runs of CI, Packaging and Windows
ARM64 native qualification to complete successfully before tag creation. The
revision is rechecked and the annotated tag explicitly names the checked SHA.
Nine focused controls cover patch/minor/major preparation, retained failure
state, dirty/non-main refusals, tag/notes/publication sequencing, absent or
unqualified workflow observations, API ordering and a main change during
readiness checks. All 297 integrated helper tests also pass, with no failures or
skips in that run. The Git transitions and Make recipes are real; the GitHub
service and application test result are controlled fixture boundaries. This is
not release publication or native packaging qualification.

The source-bound fixtures use isolated Git configuration and local bare origins.
Their logs are retained in `_build/release-targets-767/`: the original failing
`before-valid-fixture.log`, passing `final-controls.log`, and the read-only
`real-main-readiness.log`/JSON receipt. The final read-only recheck at main
`74200614` still has no qualifying exact-head CI run; the readiness command exits
1 as required. No
hosted job, repository release tag or GitHub release was created by these checks.
The final readiness output is `real-main-readiness-final.log`/JSON.

### Open acceptance

The starting open-issue snapshot contained 70 issues. #762–#765 and
#767–#771 added nine issues during this audit (79 before subsequent closures).
#764, #765 and #770 completed their scoped investigations/qualification.
This checkpoint does not close the unrelated native
runtime, integration, distribution, or acceptance issues.

- #661 still requires physical gesture-capable trackpad and remaining
  platform/surface acceptance; browser wheel controls do not establish it.
- #631 still requires its complete discovery/enrollment/platform and adverse
  network acceptance, beyond the retained two-host component receipts.
- #664 and ordered #673–#678 remain unfinished. PR #682 remains a conflicting
  draft with a canonical authority observation prerequisite. Its read-only
  component evidence cannot qualify admission, leases, publication refusal, or
  the three-host human/agent workflow.
- Native daemon/recovery, platform failures and activation gates, including
  #706/#707, #752 and #761, remain independently tracked. PR #754's private
  continuity evidence does not close all those issues.
- Release/distribution gates retain their existing issue-specific requirements.
  On Puck, GUI `notarytool history` lookups for profile `webmux` in default
  storage and the explicit login Keychain report a missing item (native exit
  69), rather than the earlier remote locked-Keychain error. #152 still needs
  profile setup/location, signing, notarization/stapling/Gatekeeper and signed
  upgrade permission acceptance. The initial private observer misparsed the
  formatted exit 69; its retained observation-limit receipt has an explicit
  correction note and an independent completed explicit-login lookup.
- #63 still has no repository signing secrets or dedicated release signing
  environment in the read-only metadata check; provider enrollment and native
  signature/installer acceptance remain outstanding.
- #65's current native 1.4.5 submission is microsoft/winget-pkgs#482084, open at
  `0052405113c622f6b2b0276c876da931351bd505`. All eleven non-skipped checks now
  pass, including installation and CLA. Moderator approval, merge and actual
  community-source discovery remain pending. The old 1.3.10 submission #432896
  closed without merging; it is historical and superseded.

The unchecked requirements in `docs/roadmap.md` remain unchecked. Complete their
actual acceptance before publishing a release or claiming the overall goal done.

## Native Linux Snap storage and shutdown: #763 / #771

At `169dc3da16e2dc7979de7145a3644046c563ba20`, Ubuntu 22.04.5 ARM64
(Linux 5.15.0-203) with Canonical strict Snap Chromium 158.0.8127.36 revision
3606 passes the complete native browser race package: 50 top-level passes,
zero failures and one fixture-child skip. The original two Windows-only
NetLog subcases remain skipped. The four original workspace, saved-profile,
accessible-action and callback UI files pass all ten tests, including exact
upload/download and saved-cookie worker replacement. The Linux process control
passes mismatched identity, bounded timeout, final child file operation and
unrelated sibling survival checks. Its private child entry point is skipped in
the ordinary parent invocation, then actually executed by the control.

The first owned QA fixture precreated browser storage with mode 0775; production
correctly refused it. Correcting only those owned fixture directories to 0700
exposed a separate ENOTEMPTY removal failure after direct WebSocket shutdown:
48 top-level native tests passed and one group failed. A metadata-only original
control reproduced it. Inotify observed TransportSecurity/Network Persistent
State files created or renamed while removal proceeded, and observed browser
children outlasted the main process. This supports a shutdown ordering gap;
the observer does not identify the exact writer PID. All failed outputs remain
retained. A separate unchanged-source run already passed the ten UI cases;
that did not turn its failed native cleanup control into acceptance.

Linux now retains pidfds for its private CDP browser process set before
cancellation, rechecking account, executable and immutable process start time.
It waits on those stable handles before the single directory-removal attempt.
The existing two-second collection/wait bounds, saved-profile graceful-close
bound, browser protections, test deadlines and retry policy are unchanged.
Unverifiable identity, unsupported pidfds or failed exit waits retain the files
and return a fixed error. The code never signals a process selected by name or
follows a newly resolved PID after cancellation. This was a Linux no-op before
#771; Windows retains its existing handle wait and macOS behavior is unchanged.

The current-source full package and UI run complete in an owned copy-on-write
QA VM, followed by normal guest shutdown. All retained backing disk, firmware
and seed hashes are unchanged; no top-level WebMux home entries were added.
The source is unchanged during qualification. macOS browser race tests and
Linux ARM64 vet pass; Linux ARM64 and Windows ARM64 test binaries compile.
Cross-compilation does not qualify native Windows behavior. The complete macOS
Make result above retains its original `2cc3a26d` source binding.

Private receipts: `snap-native.json`, `snap-mode-fixed.json`,
`snap-cleanup-diagnostic.json`, `snap-pidfd.json` and their `gui-*` outer receipts
under the audit directory. The adjacent sanitized receipt records source and
log/archive hashes for the successful Linux run. Earlier failure archives and
the metadata-only observer remain preserved. This qualifies the changed Snap
storage and scoped shutdown behavior; broader platform, SSH/provider, roadmap,
signing and release gates remain open. The operator service was not deployed
or restarted.
