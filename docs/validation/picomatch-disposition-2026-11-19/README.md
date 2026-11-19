# Picomatch candidate exposure disposition

The technical exposure review accepts the native-runtime non-presence and
trusted default build/test pattern boundary for candidate
`b155f4ca58a895c17fbaf8dc484ae44478bcac51`. This records the review decision
proposed in the earlier [runtime review](../picomatch-runtime-boundary-2026-11-19/README.md)
and [build-input review](../picomatch-build-inputs-2026-11-19/README.md), with
fresh current-source execution and package evidence. The
[receipt](receipt.json) binds the evidence and its limits.

**[#429](https://github.com/jordanhubbard/webmux/issues/429) stays open** for
reviewed documentation integration, final-source gates and exact-main
reconciliation before publication. No native release is cut by this decision.
The generic matcher is not repaired.

## Decision and scope

| Surface | Reviewed disposition | Basis and limit |
| --- | --- | --- |
| Inspected native candidate archives and embedded browser | Matcher not present in the inspected runtime distribution | Twelve native archive inventories, both independently unpacked MSI payloads, fresh production module graph and matching actual packaged browser bytes. This is not exhaustive arbitrary-code absence or external-manager inventory clearance. |
| Current default production frontend build | Reviewed pattern definitions are trusted configuration | One observed compilation of six fixed Vite defaults. No browser/API/terminal input defines these patterns on the reviewed path. |
| Current default frontend tests | Reviewed pattern definitions are trusted configuration | Thirteen compile/scan calls use locked defaults, the committed include literal and tinyglobby subdivisions. Crawled names are match subjects on the reviewed path. |
| Generic picomatch 4.0.7 with arbitrary patterns | No algorithm repair or safety disposition | Three of six retained bounded probes still time out on byte-identical matcher code. The failures remain retained. |
| Modified plugins/configuration, development servers and optional external managers/deployments | Outside this evidence | Reassess their actual pattern-definition boundary and component inventories. This review does not close #91. |

The runtime disposition does not depend on an audit scan being green. Native
archives do not ship an adjacent Node runtime, JavaScript module tree or matcher
in their browser dependency inventories. The actual browser build graph has
146 modules and no picomatch, micromatch, tinyglobby, fdir or Vitest module. The
six final build outputs match the actual candidate macOS server's HTTP bodies.
The server runs with a private home and owned loopback listener, coordination
disabled, then stops cleanly with exit zero. Its clean revision/version metadata
is a producer self-report, not independent build attestation.

All five JS/CSS bodies occur byte-identically in all four native server images.
The strict original Windows HTML comparison remains **false**. A separate full
current-source build with LF versus CRLF index input reproduces the exact
Windows HTML range and preserves every JS/CSS output. That experiment explains
the content difference; normalization is not substituted for the strict check.
Both MSI payloads contain twelve files whose byte/length multiset matches the
corresponding all-native ZIP, with exactly the known twenty-four physical
metadata/single-CAB streams. Neither installer was executed in this review.

## Current-source build/test review

A private synchronous Node load hook observes the published picomatch entry and
callable properties without editing installed files. Positive entry and makeRe
controls confirm observation. On clean b155 source, the actual production build
and all **450 tests in 44 frontend files** pass. In-memory build output avoids
changing generated product files. All six fresh final hashes equal the actual
packaged HTTP bodies, independently of the older graph result.

The build compiles these fixed definitions:

```text
**/.env
**/.env.*
**/*.{crt,pem,key,p12,pfx,cer,der}
**/.npmrc
**/.yarnrc.yml
**/.git/**
```

The frontend-test calls add locked Vitest exclusions, the committed test include
and its tinyglobby subdivisions. None of these observations supplies a repeated
plus/star extglob. Static review of the hash-verified Vite, Vitest and tinyglobby
call sites confirms compilation of configuration lists before discovered names
are matched. Source/configuration/plugin authors already execute JavaScript as
the builder. This does not establish that arbitrary edited configurations,
checkout paths or development-server deployments are safe.

The exact dependency lock and named installed matcher/build-tool source hashes
remain bound to the retained evidence. Root package metadata differs from the
earlier c195/4ef build review only by adding two helper tests; the dependency
and build commands are unchanged. Earlier 444-test/145-module results retain
their historical source scope and are not relabeled as this 450-test/146-module
execution.

## Advisory and remaining release conditions

The [GitHub advisory](https://github.com/advisories/GHSA-fc5q-39xc-vjpf) is
withdrawn and lists no patched version at this review. The
[CVE record](https://cveawg.mitre.org/api/cve/CVE-2026-112366) remains PUBLISHED;
the [registry](https://registry.npmjs.org/picomatch/latest) reports 4.0.7.
The [maintainer's explanation](https://github.com/micromatch/picomatch/pull/235#issuecomment-6315749381)
recommends trusted pattern definitions. These status facts corroborate the
review context; they do not repair the matcher or replace the runtime/input
boundary evidence above.

The unchanged exact-b155 audit and all ten original qualification workflows
pass on first attempts, with twenty-seven source/digest/safe-extracted artifact
ZIPs and NAS checksum readbacks. The later automatic canonical checks remain a
separate cohort. A passing current-source cohort does not prove exact-main CI.

Before #429 closes or a release is published, integrate the reviewed disposition,
retain the unchanged final-source audit and relevant frontend/helper/build/browser
checks, and reconcile the actual main/release source. Reassess after a change to
runtime contents, build inputs, dependency graph or supported deployment scope.
No override, downgrade, source-cache patch, audit threshold change or gate waiver
is introduced. Full roadmap, native recovery, signing/notarization and deployment
security/license acceptance remain separate requirements.
