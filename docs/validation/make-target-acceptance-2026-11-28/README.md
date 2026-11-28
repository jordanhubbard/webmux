# Make target acceptance, 2026-11-28

This reconciles the original #762 request: initialize fresh configuration without
losing existing settings, delegate the public Make surface from either workspace,
and retain build, install, lifecycle and quality evidence. It completes that
source-workflow audit. Other native integrations, signing, deployment and the
remaining roadmap keep their own acceptance requirements.

## Requirement evidence

| Original requirement | Verified evidence |
| --- | --- |
| Fresh configuration and preservation | At current main `f02f158c`, actual root and application-directory `configure` recipes both exit 0 against one private path containing spaces. The first creates configuration with HTTP 19271/HTTPS 0; the second changes HTTP to 19272 and preserves an added unrelated setting. Dependencies were prepared separately, so `-o deps` excludes only repeated installation. Existing credentials and services were untouched. |
| Public delegation from either workspace | The unchanged current regression enumerates all 39 public root targets and invokes each through the actual nested Makefile against a parent fixture in a checkout path containing spaces. It also checks the default target. The original native nested `build-client` succeeded. This separates real Make delegation from executing destructive or publication side effects for every spelling. |
| Meaningful configuration/lifecycle controls | All 25 current Make/release/readiness controls pass, with zero failures/skips. They include malformed/invalid configuration refusal without partial writes, launchd failure propagation, configured service units, optional/idempotent Agent Deck installation, client-only installation, literal runtime values and zero-port behavior. These use isolated fixtures; they do not qualify real optional-service deployment. |
| Native build, packaging, install and lifecycle | The [original audit](../make-target-audit-2026-11-26.md) retains native macOS ARM64 builds, package variants/checksums, relocated runtime, public manual lifecycle, and private real GUI service/combined client installation and uninstall. The four original command logs were rehashed against their receipts: all match, all original commands exited 0. Candidate archives were development version 1.4.5; they are separate from release assets. |
| Quality and current-source relationship | The original GUI `make test` at `2cc3a26d` passes 500 frontend/297 helpers/1418 E2E, with 22 original platform/integration skips. [Storage acceptance](../browser-storage-acceptance-2026-11-28/README.md) independently rechecked its actual log hashes. Later full Linux `make test` at `5b8d882b` exits 0; its Makefiles, configure/release/runtime scripts and Make controls are byte-identical to current main. Current focused controls also pass. This is source-specific evidence, not a new full-suite run on this documentation commit. |
| Release recipes and publication | Actual patch/minor/major preparation and publication recipes pass real local Git transition controls with a private bare origin; only application-test outcome and GitHub service are substituted. Failure preserves the branch/edits, and unqualified or moved main cannot be tagged. The separately published [v1.4.7](https://github.com/jordanhubbard/webmux/releases/tag/v1.4.7) binds source `4b8c59a1` and has 28 assets. Prior release admission and asset-verification evidence retains its own source/CI boundary. No new release was invoked for this reconciliation. |

The adjacent [receipt](receipt.json) records all 39 target names, source hashes,
current commands, rechecked native log identities and the focused test log hash.
Its original logs remain private. Earlier failed native and test attempts remain
in the original audit; they have not been replaced with successful results.

The current checkout initially lacked Node dependencies, then the qualification
wrapper omitted Go from PATH. Those failed runs are retained privately. After
locked dependency preparation and explicit qualified Node 24.22.3/Go 1.27.4 PATH,
the unchanged 25 controls pass. No product source, assertion, timeout or retry
policy changed.

## Remaining acceptance

The original full macOS run is tied to its recorded source. Later Make changes
preserve literal runtime values and configuration-selected/disabled ports; the
current regression exercises those changes. The root Makefile is not claimed
byte-identical to the original macOS source.

Agent Deck's explicit installer is covered by its pinned/idempotent fixture;
the actual default optional-install command skipped as configured. No real
optional integration was activated. Windows/platform coverage, external provider
and hardware acceptance, signing/notarization, installed-service rollout and
#110/#631/#661/#664 remain separate. In particular, the preserved Puck app bundle
still reports 1.4.5 and its source revision is unattested. This reconciliation
neither upgrades it nor restarts an operator service.
