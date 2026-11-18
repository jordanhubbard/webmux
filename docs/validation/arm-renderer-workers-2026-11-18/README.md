# Complete native ARM renderer qualification within the existing budget

[#456](https://github.com/jordanhubbard/webmux/issues/456) includes native Windows
ARM64 server/retained-engine tests, 28 actual ConPTY captures and 1,384 renderer
checks. Exact Mosh candidate `b357ca694b390bbca1377a0a759d44ede0331220`, run
[`44260396875`](https://github.com/jordanhubbard/webmux/actions/runs/44260396875),
reached the existing 60-minute job limit. The annotation explicitly reports the
execution limit. The serial renderer step began at 07:46:50 UTC; checks 1,382 and
1,383 passed at 08:37:53 and 08:37:57, then cancellation cut off the final saved
attributes/cursor-columns control. This is incomplete qualification.

Artifact `14411377406` SHA-256
`6170eb6a569e862b4e414243ac28739688e8a5347224cf6078bb84e6f1a9098b`
contains all 28 actual ARM ConPTY captures and 1,381 complete comparison receipts,
equal at initial/continued/recaptured stages, with no replay queries. Reference
profiles remain 259 bare and 1,122 shared-hyperlink. These receipts do not replace
the missing terminal gate success. The recorded worker had 13,531,432 KiB free
of 16,767,020 KiB memory at admission. Chromium's separately recorded AMD64
emulation remains explicit; this is not a native ARM browser claim.

The candidate changes only the native ARM renderer command to two workers with
fully parallel scheduling. Test source, renderer profiles, assertions, retries,
case set, native admission/capture gates, per-test timeouts and the 60-minute job
budget remain unchanged. Each worker has a private ephemeral HTTP listener,
private browser pages and a worker-indexed generated fixture artifact. The
supplied native captures/logs are read-only. Every worker still generates and
checks the complete ordered fixture inventory; initial/continued/recaptured
operations within each test retain their original ordering.

List-only verification finds exactly the same 1,384 named native-mode checks for
both schedules. A parsed-workflow comparison permits only the command's two
scheduling flags. Local/native execution evidence is recorded separately; no
passing source gate is inferred from test listing or cross-compilation.

Local real macOS Chrome execution passes all 1,356 available cases with two
workers in 2.3 minutes. Its 1,353 comparison receipts all match at three stages,
with zero replayed queries. The final saved-attributes/cursor-columns control
passes too. These local cases do not include Windows ConPTY captures. Native
Windows ARM execution of the changed schedule remains required.
