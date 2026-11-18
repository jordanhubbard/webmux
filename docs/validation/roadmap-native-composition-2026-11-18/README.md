# Native creation and qualification composition — 2026-11-18

This checkpoint composes canonical roadmap #500 (`4468d49`), the targeted
Ubuntu libc security minimum #518 (`ae00c27`), the private-tmux test receipt
publication fix #520 (`ce7546d`) and the internal native Codex creation checkpoint
#511 (`fa8ad07`). Source merge: `dc030e45b1712154e4cc29291d7ac6e1e73dd91f`. All four inputs remain ancestors;
no input is replaced with a reconstruction or cherry-pick.

The coordinator source is byte-identical to #511. Frontend and private-tmux
source/tests are byte-identical to #520. The npm lockfile is unchanged from main.
The workflow merge retains the libc minimum and adds #511's native-witness
controls. This integration introduces no further production behavior.

Local combined-source validation passes 425 frontend tests, 191 helpers with
zero skips, ten native-witness controls, all five strict TypeScript projects,
frontend lint, the application build, 272 installed lock identities, affected
HTTP API/coordinator Go race suites and vet, and whitespace checks. The initial
Go invocation accidentally selected Node 26; the native creator correctly
refused that unsupported version before the test's expected stage. That failed
log is retained. The same coordinator source passes with the selected Node 24;
the original HTTP API race run passed. Nothing is silently relabeled.

#511 and #518 have all seven non-audit CI jobs and ARM qualification passing;
#520 has all seven non-audit CI jobs passing, with its existing ARM run still
active at this composition checkpoint. #511's downloaded ARM artifact matches
the published digest: 28 captures and 1,381 comparisons match at all three
stages, with zero replayed queries. Go/Node/capture are native ARM64; the browser
is AMD64 under emulation. These component results do not replace fresh checks
on the final combined head.

The internal native factory remains partial: public HTTP/UI creation, durable
namespace allocation, typed uncertain-request controls and native recovery
composition remain open. Retained native proofs keep their original source,
version and platform scope. They do not establish combined browser or lifecycle
acceptance, authenticated ARM64 recovery or completion of #318/#438/#194.

GitHub withdrew the picomatch advisory, but the unchanged npm audit still
reports twelve high package entries. #429 remains open without a gate waiver,
dependency substitution or suppression. No main merge, tag, version bump or
release is performed. Full roadmap/release acceptance remains required.

The [receipt](receipt.json) binds source identities, component runs, local log
hashes, excluded evidence and scope. Raw local logs are retained privately under
`composed-native452/`; component receipts remain under their own validation
folders. Refs #452, #438, #318, #517, #519, #429.
