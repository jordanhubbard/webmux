# Failure-only owned profile-file handle observation

[#473](https://github.com/jordanhubbard/webmux/issues/473) retains a Windows
sharing refusal after a successful single-request browser navigation. Existing
process observation found live validated browser children after the parent had
exited, but Restart Manager did not identify the actual failed-file holder.
That evidence does not establish a shutdown repair.

This diagnostic adds a different, failure-only observation:

- Before the unchanged allocator shutdown, a private test subprocess opens
  process handles from the fixture's previously validated CDP process set. It
  rechecks each PID's creation time before acknowledging readiness; reused PIDs
  and unavailable processes are counted separately.
- After a sharing refusal inside that fixture's profile, the helper opens only
  the failed regular file for attributes, without reading its contents. It uses
  documented [Process Snapshot](https://learn.microsoft.com/en-us/windows/win32/api/processsnapshot/nf-processsnapshot-psscapturesnapshot)
  handle-table capture with `PSS_CAPTURE_HANDLES` only: no names, process-memory
  clone, threads or stack capture.
- Duplicated disk handles are compared using the volume plus full 128-bit
  [file identity](https://learn.microsoft.com/en-us/windows/win32/api/winbase/ns-winbase-file_id_info).
  This can show that a creation-time-validated fixture process still holds that
  actual file. It does not by itself prove which handle denies deletion.
- Only fixed states, OS codes and counts leave the helper. Native handles, PIDs,
  creation times, file identities, paths and names stay private. The helper never
  closes a source handle, kills Chrome, or retries/waives the failed removal.

The helper is armed only with the existing explicit cleanup-process observation
flag. There are at most 64 process identities and 8,192 examined handles across
the query, a cooperative two-second query bound, and a parent-enforced eight-second
helper lifetime covering preparation through cleanup. Exit, API/query/duplication
failures and budget exhaustion remain visible. A zero match with unavailable
processes or handles does not establish that no holder exists.

The native controlled test requires an actual deny-delete file handle to be
identified, rejects the same PID with a different creation time, excludes an
unheld file's distinct identity, and checks a one-handle budget. The original
source handle must still refuse deletion after probing; releasing that fixture's
own handle must then permit removal. This control runs separately before any
manually requested fixed sampling cohort; a failed control prevents sampling.

The existing navigation modes, one-request/one-navigation assertions, cleanup
assertion, sandbox checks and test/workflow deadlines are retained. Production
worker shutdown is unchanged. Native Windows execution is required to validate
the new API path; cross-compilation is not native evidence. A passing cohort
would not erase prior failures or close #473. Retain and inspect the original
cleanup-failure records before proposing any shutdown change.

Local pre-publication checks passed: browser package race tests (1.727 seconds),
host and Windows AMD64 vet, Windows AMD64/ARM64 test-binary cross-compilation,
workflow YAML parsing and `git diff --check`. These checks do not prove that
Process Snapshot works on the native runner; the controlled native job is the
next gate. The source is stacked on #503's `473da1123a96dc10f9740ef57361c2b3571cf812`
and does not replace canonical integration #500.
