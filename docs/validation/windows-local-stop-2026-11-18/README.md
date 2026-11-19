# Native Windows local Hide and Stop

The ordinary-local Windows lifecycle now retains authenticated access on Hide
and confirms the original provider's exit before deleting a stopped terminal.
Actual Windows AMD64 application checks cover a native authenticated Codex job,
controller restarts, lost Stop replies, an unresolved Stop, siblings, two live
browser viewers and retained logs. This qualifies the scoped implementation in
draft PR #511; review, CI and integration remain required before closing #541.

The [receipt](receipt.json) binds two clean canonical embedded application builds:
`a7a6298dd67fae33360983e6044bff62ef5a22ba` for the lifecycle checks and
`346de8a111494934f2091f9bf623827cef0b2ea2` for the final Hide correction.
Server source is identical between those revisions. The correction changes the
workspace refresh and adds an integration regression test. This is a private
user-run application qualification, not an installed-service/MSI or release test.

| Actual check | Result |
| --- | --- |
| Authenticated native work | Existing Codex 0.153.0 conversation resumes through the normal wrapper; explicit cmd.exe/Python tool reads the fresh fixture nonce and exits zero |
| Hide and controller crash | Saved session remains discoverable; eleven provider/job identities retain PID, creation time and image hash across controller replacement |
| Restore from window list | Original native conversation recalls the remembered marker without tools or a new conversation |
| Old-client DELETE on live work | HTTP 409 retains the session instead of losing its only saved access record |
| Lost actual Stop response | Browser loses the backend's successful response, retains the original UUID/selection and does not delete the session |
| Duplicate original Stop | Identical request returns the same stopped operation before and after another controller crash; sibling provider, shell and environment remain unchanged |
| Unresolved original Stop | Bounded pause of the verified private provider causes a real handshake timeout; HTTP 202 retains uncertainty and disables persistent relaunch |
| Reconstruction with unresolved Stop | Controller crash/restart retains the same operation and original live job; UI inspection returns uncertain without dispatching another Stop or attaching/replacing the provider |
| Late original completion | All twelve paused threads resume; the queued first Stop completes; original-only UI inspection confirms exit and deletes the saved session |
| Two viewers on corrected build | Hide immediately hides one tile; the other viewer reads the original shell environment; restore retains output and original selection; both sockets stay connected until explicit Stop |
| Explicit viewer Stop | HTTP 200 confirms original owner exit, both sockets disconnect, then DELETE succeeds and subsequent GET returns 404 |
| History | Seven terminal logs and the original native journal remain; all fourteen final prefix comparisons match |
| Cleanup | All twenty recorded process objects are gone; Restart Manager finds zero holders for twelve private files; application listener and SSH tunnel are gone |

The native journal contains four completed turns: preparation, remembered marker,
one actual tool call and no-tool recall. The existing account is used in place.
Launch arguments select read-only/never and the legacy unelevated Windows
sandbox; controller survival retains the same native process. This case does
not add a new filesystem/network-denial qualification or repair #538.

The first two-viewer case exposed a real UI defect: Hide persisted `minimized`
but refreshed only session records, leaving the current page's collapsed layout
unchanged. The new Workspace integration test fails on `a7a6298` with expected
hidden/observed visible. Reusing the existing request-ordered `loadSessions`
refresh repairs both states and preserves the mounted terminal. All 437 frontend
tests, TypeScript, lint and production build pass. The final native two-viewer
case passes on the hash-verified clean `346de8a` image.

Failed raw evidence remains retained. The first viewer harness's incorrect
WebSocket URL filter produced zero counters and is not positive viewer evidence.
The corrected harness observes `/api/term/`, checks actual hidden/visible state
and retained output, and records 88/83 received frames before Stop with no socket
closures. A timed-out partial image transfer was retained and never executed;
the completed transfer's SHA-256 was checked before replacement.

The unresolved native case crashes after timeout persistence, not during the
persistence/dispatch window. Broker fault and concurrency tests separately cover
those boundaries, stale/foreign owners, failed saves, duplicate requests and
original-only deliberate continuation. The native paused case completes its
queued first Stop and does not exercise a continuation dispatch. All twelve
threads resume after 36.843 seconds, within the fixture's forty-second bound.

The private archive freezes 146 raw/source artifacts with a SHA-256 manifest;
authentication storage is excluded. Product Stop handles all four fixture
terminals, including the failed viewer fixture. Only verified private controllers
are terminated for crash injection/cleanup. No native authentication file is
read/copied; no new login, CLI update, reboot, service/account/firewall/Defender
or sandbox-default change occurs. Native project trust is reviewed in its UI.

Full exact-source CI, native ARM64 qualification and review/merge remain pending
at this receipt. Other lifecycle managers, accounts/policies, service/MSI
upgrades, authenticated ARM64 and full #318/#438 acceptance remain separate.
The legacy shell/network failures (#538), private VT timeout (#536), original
corruption cause (#450) and full roadmap/release completion remain open.

Subsequent hosted CI exposed browser-fixture cleanup assumptions; the
[fixture follow-up](ci-fixtures.md) preserves the Stop/retention contract and
separates the pre-existing hosted native-startup gap (#543) from browser controls.
