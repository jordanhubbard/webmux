# OSC 8 restoration checkpoint — not accepted

Issue #404, under #355. This branch is a work in progress and must remain a draft
until the remaining renderer mismatch and native qualification are resolved.

The renderer oracle now compares link destinations, equivalence classes of linked
cells, and the current pen, alongside text, colors, modes, cursor and wrap state.
It compares observable link identity rather than renderer-local numeric handles.
Unicode IDs remain distinct from literal percent escapes. Anonymous openings need
separate identities, including when their URIs are equal; their active pen must
continue the correct group after attachment. Existing named IDs must still join
later live output that reuses them.

The implementation admits bounded, passive OSC 8 into the xterm mirror. Other
string requests remain excluded. Link payload limits match the VT ring's bounded
URI/parameter representation; unsafe or oversized state refuses attachment. A
screen-local random namespace separates anonymous groups without rewriting the
original live output. Cursor restoration preserves xterm's live hyperlink object.
The painter closes the main link after switching buffers so it cannot leak into
blank alternate-screen cells.

## Evidence so far

- Focused Go replay-filter and screen-state suites pass, including arbitrary
  read boundaries, Unicode IDs, separate anonymous groups, payload/control bounds,
  exclusion of other private OSCs and capture immutability.
- All ten new renderer cases fail against pre-feature source using a Go overlay;
  the working source files were not replaced for that control.
- The working implementation passes 60 of 61 portable renderer cases: all 51
  previous cases and nine new hyperlink cases. The alternate case passes initial
  attachment and continued output, but fails the second attachment comparison.
- Four actual ConPTY hyperlink controls are added to the existing collector.
  Their execution and hosted gates are still pending; portable results do not
  establish native Windows acceptance.

Raw local evidence is retained under `_build/osc8-404-baseline/` and
`_build/osc8-404-current/` in the canonical checkout. These are synthetic page and
terminal fixtures, separate from authenticated coding-session acceptance.

## Remaining failure

On xterm 6.0.0, leaving an alternate buffer can dispose the last marker of a link
while its numeric reference remains in the global extended pen. Later cells retain
that attribute but have no resolvable destination. Both existing viewers agree;
the Go mirror retains the URI and revives it on a later snapshot. The failing case
is intentionally retained. It must not be removed, weakened or counted as accepted.

The implementation must account for link lifetimes and hidden extended attributes,
including the effects of row/marker disposal, buffer switches, cursor restore and
bounded history. The fix must preserve destinations, grouping, visual attributes
and active pen state across all three comparisons. Unrepresentable states must
refuse attachment without replacing or stopping the job.

The installed renderer's [OSC link service](https://github.com/xtermjs/xterm.js/blob/6.0.0/src/common/services/OscLinkService.ts)
and [input handler](https://github.com/xtermjs/xterm.js/blob/6.0.0/src/common/InputHandler.ts)
are the authority for this profile. Upstream also tracks marker retention after
cell overwrite in [xterm.js #6303](https://github.com/xtermjs/xterm.js/issues/6303);
that report does not establish a fix for this branch's failing transition.
