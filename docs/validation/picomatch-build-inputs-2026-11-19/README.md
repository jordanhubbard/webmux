# Picomatch build-input review

On clean canonical `c195e020d3ee176b00b976cb76a22d23c6b9e5b3`, the actual
production build and all 444 frontend tests pass with a private Node load hook
recording calls through the published picomatch entry and callable properties.
Positive entry/makeRe controls verify instrumentation. No installed dependency
file changes. The [receipt](receipt.json) binds source, dependency and trace hashes.

The build records one compilation of the six fixed Vite default deny patterns.
The test run records thirteen compile/scan calls: Vite/Vitest defaults, the
committed test include, and tinyglobby subdivisions. Static input review confirms
that Vite reads resolved deny configuration, Vitest forwards include/exclude
configuration, and tinyglobby compiles those lists before matching discovered
paths. File names are match subjects on those paths. The production graph has
145 modules without glob/test tooling; all six final output hashes exactly match
the earlier uninstrumented clean4b production build.

For these inspected default invocations, the evidence supports trusted build
pattern inputs and the earlier native-runtime non-presence disposition. It does
not establish arbitrary-pattern safety. Source/configuration/plugin authors
already execute JavaScript as the builder; edited configurations, development
servers and arbitrary external manager deployments need their own review.

The [upstream maintainer explanation](https://github.com/micromatch/picomatch/pull/235#issuecomment-6315749381)
recommends trusted pattern definitions. Advisory withdrawal and the unchanged
[exact-c195 audit job](https://github.com/jordanhubbard/webmux/actions/runs/44454616832/job/135326510506)
reflect classification, not an algorithm repair. Generic4.0.7 still fails three
of six retained bounded repeated-extglob probes, and the CVE remains PUBLISHED.
The original failures are retained. No override, downgrade, source substitution,
audit threshold change or release waiver is introduced.

This completes the default build-input technical assessment and proposes a
repository exposure disposition for review under #429. Full exact-head gates,
review/reconciliation and broader deployment/vendor clearance remain separate.
The later Windows Stop/Hide composition changes frontend source; this receipt
keeps its exactc195 build/output scope rather than claiming a new artifact audit.

A fresh build on preparation merge4ef additionally records146 modules with no
matcher and the same single compilation of fixed Vite defaults. All six final
output hashes match the actual frontend files on disk. Its
[separate receipt](preparation4ef.json) does not claim packaged-server HTTP or
archive inspection for that newer source.
