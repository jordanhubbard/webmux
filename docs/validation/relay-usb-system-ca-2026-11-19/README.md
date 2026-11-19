# Combined offline USB inputs and explicit public CA bootstrap

Compose published CA input `14ad699b57b62ef556f4e76b74362d447bd8e6b1` from #553 with frozen USB/HTTPS candidate `2618d8de244b86780a4dbec2b091a34679a245e1` from #552. Retain the stronger actual OpenSSL unavailable-default-store positive and missing-store refusal controls prepared in #554. The published CA generator is AST-identical to14ad; the entire native main body and bounded logging helpers remain AST-identical to2618. No native assertion, timeout, SSH behavior or cleanup changes.

The early native APT workflow now runs both command-evidence suites plus all bootstrap controls, so the environment-sensitive CA prerequisite is checked before full browser/relay qualification. Preserve explicit public trust selection, bounds, TLS/hostname verification, archive signatures and malformed/missing-all-trust refusal. The USB acquisition implementation, original manifest and host APT helper remain byte-identical to2618.

Local composition controls:14 pass and1 explicit APT-platform skip on macOS. In an existing isolated Linux AMD64 container on an ARM64 host, all15 controls pass with ordinary defaults, then all15 pass with unavailable Python default CA paths. This is CPU-emulated Linux control evidence, with networking disabled, read-only source, no image pull or package installation. It is not native Docker/SSH/browser relay acceptance.

Input evidence remains bound to its original source:2618 actual USB job135420216749 passed all four admitted kernel controls, including the expected unpatched negative; all8 initial source files came from the immutable repository bundle with zero network attempts.14ad original APT run44484153258 passed on Ubuntu22.04 and24.04, with all9 bootstrap controls independently confirmed in each original job log and zero skips. Source/artifact digests and NAS readbacks are retained separately. Neither result qualifies the new composition by inheritance.

Original2618 CI44483208381 and14ad CI44484204564 remain observed without retry. The initial4623 HTTPS candidate's CA refusal and earlier4860 Docker HTTP-index timeout/USB429 remain retained. Additional exact-source native results and review are required before integration, main changes or release. Full roadmap acceptance, authenticated/reboot/service/bridge/mobile coverage and vendor/signing gates remain separate.

Refs #549, #550, #552, #553, #554, #452.
