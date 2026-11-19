# Combined-source Windows VT qualification

Source `f4c2651fb4dd28686bab76e2caad69ee5fd0a450` imports the already merged
PR #503 mitigation into the PR #511 recovery candidate. The
[combined local receipt](combined-receipt.json) records identical imported
sources, byte-for-byte inversion of the test-only change and unchanged labels.
The [native receipt](combined-native-receipt.json) binds the sources, artifacts,
executables and complete observed inventories.

| Evidence | Result |
| --- | --- |
| [Required Windows VT repetition, CI 44426943873](https://github.com/jordanhubbard/webmux/actions/runs/44426943873) | Package passed: 21,860 test pass events, 2,186 passing identities repeated ten times. AMD EPYC 9V74, XState 0x8e7, stock Go 1.27.4. Broader CI is separate. |
| [Windows diagnostics 44426948090](https://github.com/jordanhubbard/webmux/actions/runs/44426948090) | Stock and guarded native processes both exit zero. Each has the identical complete pass/skip inventory as the required repetition. AMD EPYC 7763, XState 0x807. |
| Native supervisor controls | All four pass on the diagnostic worker, including native high-bit exit preservation. The native exits, rather than only shell/pipeline success, are verified. |
| Private Windows Ice Lake attempt | Rejected setup evidence: fixture files were omitted and cwd was incorrect. Missing-file failures preceded the ten-minute test timeout; native exit 2 is retained. This does not explain the timeout's duration or reproduce the original corruption. |
| Corrected private attempt | Running separately with all 19 tracked fixtures hash-checked and cwd set to the bundle root. Same stock executable, full count and deadline; no result claimed. |

All three completed hosted inventories include the same three pre-existing
skipped identities on each repetition: `TestDepDiffVT`, `TestDepDiffResize` and
`TestGhosttyParkedCursorJoinsLines`. The receipt records these explicitly.
There are no failed test events in those hosted inventories. Executable hashes
match their retained sidecars; the diagnostic guard string is absent from the
stock executable and present in the private guarded executable. The guard is a
detection overlay, not a release runtime or repair. The SDK remains unchanged.

The private bundle's first detached driver ended during supervisor setup before
VT execution; owned-process and descendant absence was confirmed before the
foreground attempt. Both failed setup attempts and their raw evidence are
retained privately. The incomplete-fixture attempt passed its four supervisor
controls but has a different, incomplete test inventory and is explicitly
excluded from acceptance. Its offline test2json timestamps describe parsing,
not native timing; the retained native process report supplies elapsed time.
The corrected attempt changes only fixture admission and the working directory.
No test filter, seed, assertion, GC setting, runtime workaround, count or timeout
is changed. No physical reboot, SDK installation or cache deletion was performed.

These workers do not cover an AMX-class target. The passing hosted runs qualify
the combined source in their recorded environments; they do not attribute or
repair the original Windows corruption. Original failed CI 44420470196 and its
native artifact remain retained under #450. Keep #450 open. Authenticated native
coding-session recovery, ARM64 qualification, broader full CI, integration review
and release acceptance remain separate requirements.
