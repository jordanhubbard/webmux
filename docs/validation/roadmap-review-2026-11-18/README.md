# Consolidated roadmap review candidate

Compose pinned #496 with #497 (password Return and scoped recovery evidence),
#498 (native APT outage/signature controls), and #499 (native VT exit reporting).
All four input heads remain ancestors. The only application change from #496 is
LoginPage; Go server/retained VT source, dependency lock and required CI workflow
are unchanged. The APT provisioner/qualifier/controls match #498 exactly, and the
VT supervisor/workflow/controls match #499 exactly.

Current local qualification passes: 418 frontend tests, all five strict
TypeScript projects, frontend lint, application build and lock identity. Python
control discovery ran at the repository root: four supervisor cases (three pass,
one explicit native-Windows skip), seven APT cases (six pass, one explicit
missing-APT skip). Server/VT/ultraviolet race/vet and 129 helper results are retained
from the identical source components qualified for #496; they were not rerun for
this frontend/documentation/diagnostic composition. Hosted checks remain required.

Actual Chrome 158.0.8127.36, with the no-sandbox flag absent, signs in to the built
native Go controller in local-auth mode by pressing Return in the password field.
Exactly one login POST occurs, and the workspace appears. Under that owner, the
USB UI creates/pairs a lease, sees reported transport readiness, requests Stop,
keeps Stop pending until release and then shows confirmed release. Its helper
reporter is synthetic: no native device or authenticated coding claim. Both the
private controller and browser exit.

Initial local harness attempts are retained separately: Python discovery first
used the nested workspace, and a browser admission request lacked the explicit
automation-reporting flag. Neither is accepted as a passing case. Corrected
root discovery and private browser configuration pass without application edits.

Native component evidence remains scoped to its exact input source: #496's
virtual USB controls include dashboard/controller-loss/owner-Stop cleanup, and
#498/#499 have native APT/exit-supervisor receipts. Imported Windows installed-
service evidence still qualifies original main `8f5d322`, not this candidate or
all recovery scenarios. The original #496 Windows VT termination remains
unresolved; later passing diagnostics are not a repair. Audit #429, #450, #473,
review, native and broader roadmap/release gates remain open. Exact combined
hosted results are pending; no main, release, signing or tag mutation.
