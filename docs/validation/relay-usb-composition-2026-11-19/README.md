# Relay and USB provisioning composition

The combined source keeps4860's bounded command evidence and offline USB bundle,
then adopts published4623b22 from #551 for #549's observed HTTP APT stall. The
original diagnostic artifact records the pinned image arriving quickly while
HTTP archive index acquisition consumes the180-second bound. Historical runs
that discarded output cannot establish that same detailed cause.

The relay build now uses the existing official signed Ubuntu HTTPS selector and
network settings. The pinned minimal Noble image has its Ubuntu archive keyring
but no CA bundle; only public host trust roots are exported into the allowlisted
context and explicitly selected as APT CAInfo. No TLS/package verification is
disabled, vendor feeds are not admitted, and the SSH private key stays excluded.
The original base digest, release/suites/components/packages,180-second bound,
native Stop/SSH assertions and cleanup stay fixed. Host APT configuration is
untouched. Its refactored configure output byte-matches the old helper on a
private mixed official/vendor source tree.

The [receipt](receipt.json) verifies the HTTPS generator AST matches published
4623, bounded logger AST matches4860, and native assertions/run calls/finally
cleanup remain identical. Published logger tests retain their original child,
return/error and partial-UTF8 assertions, adapting only to the structured log
representation already in4860. Both logger and HTTPS controls run before the
unchanged native Linux qualification.

Local validation has37 passes and two explicit macOS APT-executable skips:
11 USB acquisition,9 builder/qualifier,11 relay and6 host-APT controls pass.
The skipped real APT selections must run on Linux. Published4623's separate
Ubuntu source qualification is historical input evidence; it is not this
composition's minimal-image/relay or guest-kernel result. The complete native
CI on this source remains required.

Main/version/tags/publication stay unchanged. No timeout/assertion waiver,
protection/service/account/default-sandbox change or native credential copy.
Refs #545, #549, #550, #548, #551, #552, #452.
