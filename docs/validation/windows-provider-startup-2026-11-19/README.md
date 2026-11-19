# Windows provider startup diagnostics and application gate

Source `5a3e289` addresses the evidence gap in #543; it does not claim a startup
cause or repair. The original failing Windows job135322901102 has seven generic
startup warnings. An error-state terminal can anchor browser-companion tests,
so those passes do not prove an ordinary local Windows shell works.

Bootstrap/owner failures now carry fixed phase names and recognized OS codes in
the existing private ready frame. Ensure emits only allowlisted phase, OS and
process-exit names plus the last record/probe stage. Raw child errors, argv,
environment, keys and filesystem paths are never included in these diagnostics.
An observation-only handle retained before releasing the original child handle
can distinguish a recognized early owner exit from a missing startup reply; it
owns no job and cannot terminate work. Arbitrary phase/code values are suppressed.

The mandatory Windows application browser test requires public connected state
and actual fresh shell output. Its complete expected marker is absent from typed
input: execution must expand the machine-name reference. It then freezes the
original Stop selection/UUID, requires matching success, observes owner and root
exit and deletes only afterward. Failed/uncertain cleanup uses the retained-access
fixture contract. The case runs with Windows' default application shell, without
replacing that shell or changing sandbox/account/protection settings. Non-Windows
platforms explicitly skip this Windows application test.

Portable privacy tests pass under race. A Windows owner-start fault control and
both application/provider test images cross-compile; five strict TypeScript
projects and exact one-case discovery pass. The first discovery's CommonJS module
mismatch was corrected before admission and remains retained. These are local
checks, not native Windows execution. The [receipt](receipt.json) keeps
nativeRuntimeReady false until hosted Windows runs the actual gate.

Authentication, lifetime mutex, deadlines and once-only/uncertain selection remain.
A timeout never authorizes a replacement. Existing native assertions, seeds and
operation counts are unchanged. Private authenticated conversation evidence,
#541 raw-archive review, #538 shell/network scope, #536/#450 and full roadmap or
release acceptance remain separate. No claim about Defender or outer-job cause
is inferred from the generic baseline warning.

The first native application run at `5b7ea984` failed in CI run44463053201,
Windows Tests and Build job135354201758, before browser input: public state was
error rather than connected. The safe diagnostic was
`phase=bootstrap-owner-start os=access_denied exit=exit_failure`. This identifies
the second-hop `exec.Cmd.Start` failure, without identifying which OS operation
or policy rejected it. The record-lookup file-not-found diagnostic is subsequent
evidence of no published record. The original failed browser ZIP and job log are
retained separately from successful workflow artifacts.

Prepared formatter correction `ea43787` distinguishes a process exit status in
the owner-exit frame from a Win32 error: exit 2 is not ERROR_FILE_NOT_FOUND.
The regression fails on the parent and passes twenty race repetitions; Windows
AMD64 vet passes. This does not change the observed owner-start access-denied
evidence or repair startup. Canonical `5b7ea984` remains the source of that run.
