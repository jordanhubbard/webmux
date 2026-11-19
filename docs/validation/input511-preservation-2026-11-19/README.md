# Latest #511 input preservation

The canonical runtime source `b155f4ca58a895c17fbaf8dc484ae44478bcac51` had adopted much of #511 without retaining its latest input ancestry. Per-commit review of `312881c1957a2bf6851fe1eadf16b9c66f8dac96` found missing Windows diagnostic observations, native controls and historical records. This composition retains that input as a merge parent and resolves its effects against the newer implementations.

The current application case remains unchanged. The restored second case uses the original persistent TTY child, nonce receipt, original selection, confirmed Stop, child death before its independent 60-second expiry, and duplicate Stop. Its original 90-second case deadline and the fixture task/caller deadlines are unchanged. Both cases are required with zero skips, retries or flakes. The fixture also binds the child driver hash before and after execution.

Production changes query current-job membership and immediate-job flags only after owner creation fails. Fixed labels pass through the existing closed formatter; no launch flags, job limits, protection or startup budgets change. The current early-owner-exit handling, observation-only owner handle, granular owner stages and process-exit error domain remain intact. Nested parent restrictions and the original launch failure cause remain unknown.

| Input commit | Topic | Resolution |
| --- | --- | --- |
| `1eaff2d93d31` | Launchd fixture | Three implementation/test files already match the input byte for byte; retain current roadmap and receipts. |
| `720b452e38fd` | Safe Windows phases and live application | Retain the current closed formatter and granular owner stages, including process-exit/Win32 error separation. Restore malformed-owner, private-payload and native startup controls; append the original persistent-child application case alongside the unchanged current case. |
| `cc4316cb9c89` | Native workflow gate | Retain the current required native fixture and later resource controls. |
| `d4b3e457e8cf` | Job observation | Restore read-only current-job membership and immediate-job flags after owner creation failure, with fixed recognized labels. Neither observations nor breakaway bits establish a cause or parent-job policy. |
| `b07e4db9a867` | Native application and original evidence | Restore the original rich case and child driver byte for byte; keep current owned browser teardown. Historical receipts retain original source and target scope. |
| `18010565c998` | Preparation save failure | Broker and lifecycle tests already match the input byte for byte. |
| `4eee1bffd70d` | Save-failure evidence | Restore all three original historical records verbatim. |
| `025d24503acf` | Disposable SYSTEM fixture | Retain existing account/source/image/task cleanup guards. Require both application cases without skips/retries/flakes and bind the restored child driver hash. All original task/caller deadlines remain unchanged. |
| `1a1aa14ca4ed` | Required CI fixture | Retain the required CI gate and later browser/resource controls. |
| `475f2ffa528a` | Native Codex settings guard | Six capture/resume/API implementation and test files already match the input. Retain later project/pod discovery in runner.mts; the native guard sections are unchanged. |
| `312881c1957a` | Guard adoption evidence | Restore both historical adoption documents verbatim. |

The eleven restored documents are historical evidence, byte-identical to the input. Their source hashes, native targets, outcomes and private paths retain their original scope; older source filenames describe those revisions. They are not executions of this composition. The [receipt](receipt.json) lists each digest and the preserved test-body hashes.

Local strict TypeScript checks pass all five maintained projects. Go 1.27.4 common diagnostic tests pass on macOS; Windows AMD64/ARM64 test binaries cross-compile and vet passes. Fresh committed-source native Windows qualification is pending. Cross-building Windows test binaries does not execute Win32 calls or the application fixture. Restricted-host native test skips cannot establish acceptance. The b155 packaging/security disposition and prior native receipts remain bound to their original sources. Main and the released version remain unchanged; full roadmap and exact-main release gates remain open.
