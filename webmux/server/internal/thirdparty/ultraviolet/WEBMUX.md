# Retained ultraviolet model extension

This directory retains every file from
`github.com/charmbracelet/ultraviolet@v0.0.0-20260812204455-68fa937c71be`
(commit `68fa937c71be6e737923b7c2793c117a94b6ef3d`) under its original MIT
license. `WEBMUX_UPSTREAM.json` records original and explicitly modified hashes.
The global module cache is unchanged.

WebMux adds an optional, immutable-by-value `UnderlineFlags` field to Style.
It preserves independent renderer underline/extended flag words instead of
conflating them with raw underline style/color and hyperlink data. Zero keeps
ordinary style-derived behavior. Cell copies, equality and zero checks retain
the flag value; ordinary visual rendering projects only visible style/link.
The model extension changes cell styling and the two visual output paths.

This is a model extension for #427 under #355. It does not enable a production
snapshot path, remove refusals, or qualify native coding-session lifecycle.
Tests for ordinary ultraviolet behavior remain part of the required gates.


For #532, native kqueue/epoll/select readers wait for readiness through their
cancellable Poll before entering a blocking file Read. The terminal input loop
calls Read directly, so checking the canceled flag alone could leave it blocked
on an empty pipe after Cancel. Zero-length reads remain nonblocking. Windows
console and generic fallback readers are unchanged by this POSIX fix.

The original TestReader also reused its reader/result variables after a timeout
while the original goroutine could still read. Its replacement uses distinct
channel results and joins the reader before any fresh-reader data assertion.
It retains the original 100 ms cancellation bound and checks cancellation before
and after read start, zero-length reads, fresh-reader payload and EOF. Added
select coverage gates descriptor lookup after Poll's canceled check, forcing
that cancellation ordering without sleeps or scheduler assumptions. All readers
and pipes are closed. The original CI failure and deterministic pre-fix native
empty-pipe reproduction are retained as #532 evidence.
