# Native Windows ARM64 qualification candidate

[#456](https://github.com/jordanhubbard/webmux/issues/456) adds a separate native
Windows 11 ARM64 gate under #318/#355. The existing AMD64, macOS and Linux CI and
packaging workflows remain byte-identical to the updated parent `3b80d0d`. This candidate does
not replace their race suites with non-race testing.

The new workflow checks native OS/Node/Go host and target architecture, and
requires ARM64 PE machine headers for the actual Node, Go and capture images.
Image SHA-256 values, the exact checked-out source commit, worker headroom,
Go test JSON, raw ConPTY captures and renderer receipts are retained. It checks
out the exact PR head rather than relying on an implicit merge ref. Pull
requests against stacked branches also run this independent gate.

Server and retained UV/VT suites run natively without a race-detector claim.
The existing native screen collector must execute all 28 distinct ConPTY cases
with no failed or skipped capture cases. The frontend then compares those real
captures at initial/continued/second attachment, retaining the existing bare
versus shared-hyperlink reference distinctions. The browser PE machine is
recorded separately: an AMD64 browser under emulation is not native ARM64
browser execution. The renderer is pinned to that same inspected executable,
so a default headless-shell selection cannot invalidate its architecture receipt.

Local checks pass for the two architecture-admission tests, including wrong
OS/Node/Go target/host, AMD64/IA32 images, truncated headers, invalid offsets,
bad signatures and unknown machines. The positive PE fixture is never executed;
these portable controls are not native Windows acceptance. Strict project types,
YAML structure and unchanged existing-gate checks also pass. Source bindings
are in [source-manifest.json](source-manifest.json).

Native ARM64 results are **pending**, not established by this documentation.
Authenticated coding-conversation recovery, installed-service recovery and the
separate unexplained #450 panic remain open. The pre-existing #429 audit gate
continues to block merge/release. Actual native results will be recorded in the
issue and private receipts without moving this qualification head for every
result update.


The combined candidate also retains the native-qualified VT aggregate-budget
correction (#458, source `52a8527`) and exact-payload USB fixture readiness
(#459, source `3b80d0d`). Their input artifacts are bound in the manifest. They
remain component evidence; this combined head still needs its own full CI and
native ARM64 results. The original ARM64 input reached actual native server,
UV/VT and 28-case ConPTY success, but its renderer comparison was not explicitly
bound to the PE-inspected executable. The new explicit binding closes that
provenance gap without claiming the old browser receipt proved actual selection.
