# OSC 8 restoration qualification

Issue #404, under #355. Portable and native ConPTY validation pass. Fresh exact-head hosted gates
remain required after the workspace-readiness fixture correction. This does not qualify the
other terminal profile gaps or authenticated coding-session recovery under #318.

The renderer oracle compares link destinations, equivalence classes of linked
cells, and the active pen, alongside text, colors, modes, cursor and wrap state.
Renderer-local numeric handles are excluded from identity. Unicode IDs remain
distinct from literal percent escapes. Anonymous openings have separate groups,
even with equal URIs; their active pen continues the same group after attachment.
Named IDs still join later live output that reuses them.

The mirror retains bounded, passive OSC 8. Other private string and query requests
remain excluded. URI and parameter limits match the bounded VT history
representation; unsafe or oversized state refuses attachment. A screen-local
random namespace separates anonymous groups without rewriting the live stream.
Cursor restoration preserves xterm's live hyperlink object. The painter closes
the main link after switching buffers so it cannot leak into blank alternate cells.
Hidden extended-link states that the VT representation cannot encode refuse new
snapshots, consistently with the existing extended-underline profile; reset repairs
the mirror and earlier captured images remain immutable.

## Active-link lifetime

xterm 6.0.0 stores the current hyperlink outside its buffers, but its link service
counts row markers. Disposing the final marker can discard the URI while the pen
still references its numeric handle. A following snapshot would revive a target
that an existing viewer could no longer resolve. The original failing comparison
is retained and now passes with the production frontend profile installed on all
three viewers.

The frontend adapter retains one markerless active entry. Printing on a new row
returns ownership to normal markers. A new link, close or reset releases the held
entry. Full resets also dispose old buffer markers before xterm replaces the
buffers. Disposal removes the adapter and its retained entry. Runtime shape checks
and actual-renderer controls qualify these xterm 6.0.0 internals; renderer upgrades
must pass the same controls.

## Evidence

- Focused Go race suites and vet pass for replayfilter and screenstate, including
  arbitrary read boundaries, Unicode IDs, anonymous groups, payload/control bounds,
  exclusion of private OSCs, hidden-state refusal and capture immutability.
- All ten new snapshot cases fail against pre-feature Go source through a
  read-only overlay. Working source files are unchanged by that control.
- All 61 portable initial/continuing/second-attachment comparisons pass, including
  the previous 51 controls and ten hyperlink cases.
- A separate actual-renderer lifecycle control reproduces the stock lost target,
  proves the frontend fix, and checks continued input, scrolling, 64 open/close
  cycles, explicit/RIS reset, disposal and bounded target retention.
- Windows AMD64 job 133944222609 at revision
  `0ddb8819c314db62c285633c06760801e682f97d` executed all 13 real ConPTY cases
  with zero skips, including four hyperlink cases. The renderer then passed all
  75 portable/native/lifecycle checks. Each native record was admitted with one
  owned job. These are component controls, separate from #318 conversation recovery.

Local baseline and intermediate failure evidence is retained in
`_build/osc8-404-baseline/` and `_build/osc8-404-current/` in the canonical checkout.
Final passing evidence is retained separately under `_build/osc8-404-fixed/`.
Fixtures use synthetic URLs and terminal output.

The installed renderer's [OSC link service](https://github.com/xtermjs/xterm.js/blob/6.0.0/src/common/services/OscLinkService.ts)
and [input handler](https://github.com/xtermjs/xterm.js/blob/6.0.0/src/common/InputHandler.ts)
define this profile. Upstream also tracks marker retention after overwritten cells
in [xterm.js #6303](https://github.com/xtermjs/xterm.js/issues/6303); that report alone
is not acceptance evidence for this implementation.

## History navigation readiness

The Linux workspace case exposed a separate address-versus-interactivity race
(#409). Its fixture now delays hover/popup activation for 1.5 seconds on history
return, covering reload and back/forward-cache restoration. The old input ordering
fails that control. The corrected case requires a fresh independently observed
native hover before clicking the popup. Popup creation and navigation are not
retried, and the original popup-count and 120-second case deadlines remain.
Local before/after logs are retained under `_build/browser-409/`.
