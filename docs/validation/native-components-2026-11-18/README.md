# Native component collector qualification

Candidate under [#91](https://github.com/jordanhubbard/webmux/issues/91), stacked
on the combined roadmap/ARM64 candidate `4475544`. This change adds a read-only
operator collector and CI controls; it does not change application transport,
recovery, authentication or native library selection.

The final executed Python sources were SHA-256 compared with the local source
after qualification. Results:

- Nine portable controls pass on macOS and Linux ARM64, covering process/file
  identity, deletion/replacement, account isolation, exact package ownership,
  incomplete queries, tool replacement during a version probe, lost-process
  refusal and exclusive output. The optional live case is an explicit skip in
  ordinary discovery; its separate native run executes and passes.
- A private Linux ARM64 guacd 1.6.0 process loads the actual RDP plugin on
  loopback. Its inventory records 79 ELF files and 76 package ownership records
  across 69 distinct Debian binary packages. It independently rechecks every
  recorded file hash, requires a matched process/version executable, refuses
  a different PATH installation's version, and preserves the first report
  when another request uses its output path. Listener and parent exit checks pass.
- macOS version probes and file identities are observed for OpenSSH 10.5p1,
  mosh 1.4.0, custom tmux `next-3.7` and guacd 1.6.0. These version banners do
  not establish vendor patch status or the custom tmux source revision.
- Python compilation, workflow YAML parsing, unchanged-existing-step invariants
  and whitespace checks pass. Existing CI jobs/assertions/deadlines and audit
  remain; two inventory steps and an evidence path are added.

The initial private fixture looked only at the daemon's main task's children
and found no plugin process. Both failed attempts and their successful cleanup
receipts are retained. Inspection showed that the connection-handling task
forks the worker; the reproducible live fixture checks only its own daemon's
tasks/children. The collector itself takes an explicit PID and does not infer
process ownership from that fixture discovery.

[Results and source bindings](results.json) retain sanitized file/package/license
declarations and the private raw archive/log hashes. Local paths, process IDs
and connection IDs are excluded from the public receipt. Raw evidence remains
private. The reports do not contain credentials or process environments.

This establishes the collector on those targets. It is not functional RDP or
authenticated recovery acceptance, a memory-content attestation, a license
approval, a complete fleet SBOM or a clean vulnerability assessment. Actual
deployment inventories and vendor/source advisory review still require each
operator's target; WiX approval remains open. Audit #429 blocks merge/release.
Hosted exact-head CI is separate from these local/native results and remains
required before merge.
