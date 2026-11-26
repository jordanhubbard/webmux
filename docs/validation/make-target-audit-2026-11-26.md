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

Private logs are retained in `_build/make-audit-2026-11-26/`; they are not release
artifacts. Make prerequisites are shared, so exercising a parent target also
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
| `test`, `test-unit`, `test-e2e` | The completed clean managed-browser run exited 2: race tests, Go/bundled-library tests, 499 frontend tests and 287 helper tests passed (one GUI-dependent helper skipped at that stage); E2E had 1,417 passes, one saved-cookie restart failure and 22 opt-in/platform skips. This is not full-suite green. |
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

Saved-cookie restart qualification fails on both the changed code and unchanged
baseline browser source. A separate minimal real-Chrome probe retains a synthetic
cookie during its first process, closes gracefully, and loses it after reopening.
Chrome reports Keychain OSStatus `-25308` (`errSecInteractionNotAllowed`) and
`Encryption is not available`. #764 retains this environment prerequisite and
the required rerun. The GUI-available native recheck still fails; a new minimal
probe still reports -25308 with both graceful closes succeeding. GUI-domain
availability alone does not establish usable Chrome Safe Storage access. Browser
encryption, sandboxing, and user Keychain policy were not weakened. The changed Snap data location still needs native Snap acceptance.

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
`real-main-readiness.log`/JSON receipt. Actual main `74200614` currently has no
qualifying exact-head CI run; the readiness command exits 1 as required. No
hosted job, repository release tag or GitHub release was created by these checks.

### Open acceptance

The starting open-issue snapshot contained 70 issues. #762–#765 were
added during this audit. This checkpoint does not close the unrelated native
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
- Release/distribution gates, including Developer ID notarization, Windows
  signing and WinGet, retain their existing issue-specific evidence requirements.

The unchecked requirements in `docs/roadmap.md` remain unchecked. Complete their
actual acceptance before publishing a release or claiming the overall goal done.
