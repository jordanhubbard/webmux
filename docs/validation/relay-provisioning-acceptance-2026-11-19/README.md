# Relay provisioning acceptance and remaining USB cache gate

The requirements of #545, #546 and #549 are supported by retained changed-source
native execution and the current-source reconciliation in [receipt.json](receipt.json).
The separate #550 cache correction has local coverage and remains open for its
complete changed-source native guest gate. Full roadmap and release acceptance
remain incomplete.

| Requirement | Evidence and boundary |
| --- | --- |
| Keep partial timeout output and original failure | The original d37ca20 diagnostic run 44475746089 retains actual Docker output and its failed 180-second timeout. Current real-child controls preserve both streams, partial UTF-8, command/stage/deadline, nonzero exits and spawn errors. The log keeps at most 1 MiB of original bytes per stream plus omission metadata; subprocess capture memory is unchanged. |
| Diagnose the observed provisioning stall | In that diagnostic run, the pinned image arrives in about two seconds, then HTTP Ubuntu index acquisition consumes the build budget. Package installation and native relay checks are not reached. Earlier 67019/312 failures discarded output; their detailed cause remains unproven. A later pass is not attributed as their repair. |
| Keep signed, verified fixture inputs | Exact source 0056000 uses official signed Noble HTTPS feeds, the original Ubuntu archive keyring and package choices, and 120 public system trust roots in an allowlisted context. The pinned base, 180-second build deadline and 120-second browser deadline are unchanged. TLS/hostname and package-signature verification remain enabled. |
| Complete actual native relay and cleanup | Original full CI 44541506649 is successful in all eight jobs. The uncached fixture build exits 0 in 8.917 seconds. All eight native Rig/WebMux/Chrome/SSH browser checks and four post-browser/cleanup checks pass, including local SSH exit, original remote PID/executable and sibling survival, duplicate consumption, retained viewer/history, owned container/image removal and generated client-key removal. |
| Exercise real Linux APT and failure controls | Original APT run 44541577712 passes on Ubuntu 22.04 and 24.04. Each raw job log runs all 15 relay/bootstrap controls with zero skips, including unavailable default CA paths, missing-store refusal, effective HTTPS selection and retained partial timeout output. Actual signed APT updates, vendor-feed exclusion and untrusted-key refusal also pass. |

The native source is `0056000940ec0dbc96a010c3b88d349759c51c48`. Its relay qualifier,
bootstrap/logging tests, APT helper, browser case and both Stop effect modules
remain byte-identical to the audit baseline `a29daa0`. Four source-bound artifact
ZIPs and all 3,031 uploaded file members are rehashed; exact member names are
checked for traversal, duplicates and symbolic links. The native application
hash is recorded by the qualifier; the executable is not independently available
inside the relay ZIP. This establishes scoped fixture/relay evidence, not a
whole-current-main binary attestation or authenticated coding recovery.

## USB cache correction still awaiting native qualification

The historical 0056000 USB artifact admits its expected unpatched failure and
corrected-held, direct-20 and WebMux-over-SSH positives. All eight initial sources
come from the bundle with zero network attempts; later builds use verified cache
entries. Those results retain their original source and do not qualify the new
cache reader.

Inspection found that the old cache branch calls unbounded `read_bytes()` after
a link check, without checking for a regular file. The new real FIFO regression
blocks the original reader until its five-second child deadline. Source
`1d3652578f57c32e856b90a280699e25e9f75d00` uses a nonblocking Unix open, refuses
links, checks the opened descriptor for a regular file, closes it on all paths
and reads no more than the existing 1 MiB source limit plus one byte. Cache and
bundle data still require their original SHA-256 before writes or compilation;
corruption never falls back to the network.

All original acquisition test methods remain verbatim. Literal source reversal
recovers every other acquisition line, including retry count, request/wait and
90-second acquisition bounds. The eight original source files (60,152 bytes),
manifest, patches, license and provenance are unchanged. The real cold bundle,
verified-cache reuse, FIFO/link/directory/oversize refusal, corruption and network
failure controls pass locally. Across the five local suites there are 43 passes
and two explicit macOS APT-executable skips. The original native Linux logs above
cover those unchanged relay/APT branches separately. An initial descriptor-check
ordering error in the new helper failed locally; that log is retained, and the
check now occurs before creating a Python file object.

#550 stays open: no complete native guest run has executed the changed cache
helper. Original native failures/results are preserved; no unchanged native run
was retried. No hosted job, host service/protection change, native account copy,
policy relaxation or release publication was used for this audit.
