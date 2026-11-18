# Native VT process exit evidence

Exact combined source `d77c29a`, CI `44288848654`, Windows job `134748491744`,
fails the unchanged VT repetition gate. Artifact `14420483094` retains 1,951
test passes, five skips and a package failure after 25.384 seconds. The last
started case is `TestGridFillAreaMatchesUVBuffer`; no per-test failure or Go
traceback is present. The retained worker is Xeon 6973P-C with enabled XSTATE
`0x608e7`, official Go 1.27.4 Windows AMD64. The native child exit code and cause
are **unknown**; the ordinary command's exit 1 is not the child NT status.

The Go 1.27.4 test2json driver intentionally omits a child error message after
the child has written output. A separate diagnostic supervisor now inherits
stdout/stderr unchanged, records the direct child's return code, and reports
failure for any nonzero code or spawn refusal. On Windows the report also stores
the complete low 32 bits as hexadecimal. This preserves a native high-bit status
when test2json normalizes the driver failure to exit 1. It retains no environment,
arguments, arbitrary error text, dumps or process memory.

Controls execute real subprocesses: zero exit with framed stdout/stderr, exit 7,
spawn refusal, and a Windows-only ExitProcess control using a deliberately
chosen high-bit status. That synthetic sentinel is not the observed VT status.
Local controls pass with the native Windows case explicitly skipped; actual
Windows qualification remains pending. Both complete VT inventories still run
ten times with their original ten-minute deadlines; the diagnostic job retains
its thirty-minute deadline, binary/source admission and installed-SDK check.
The controls and both native/driver outcomes remain required in the final result.
Application source, VT tests and the required CI workflow are unchanged.

The existing overlay is only a diagnostic guard. Upstream documents a
[reproducer that the guard misses](https://github.com/golang/go/issues/81238#issuecomment-5490816222).
A passing guarded run therefore does not exclude that upstream defect. The
[host-dependent reserve change CL 828724](https://go-review.googlesource.com/c/go/+/828724)
remains separate and is not applied here. Matching a reported CPU/context class
is circumstantial evidence, not attribution. Keep #450 and all release gates open.
