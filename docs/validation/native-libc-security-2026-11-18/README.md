# Ubuntu qualification C-library security baseline

Issue [#517](https://github.com/jordanhubbard/webmux/issues/517), within the
native-component review #91. Retained native Linux inventory on #515 source
`6ceeca4`, CI `44323084812`, observes the actual guacd process mapping a file
attributed by dpkg to `libc6:amd64` version `2.35-0ubuntu3.15` at
2026-11-18T14:43:11Z. Artifact `14436833717`, SHA-256
`43b2b543c58dc0790aa19e23e3a6fc18de01dc99ca5eb87f689f82373dfb0cd6`.
Package attribution does not prove unmodified installed package bytes.

[Canonical USN-9065-1](https://ubuntu.com/security/notices/USN-9065-1)
publishes Jammy libc6 `2.35-0ubuntu3.18` as the fixed revision. The CI step
updates only the native-architecture libc6 package and its required dependencies
through the existing signed Ubuntu sources after Playwright refreshes indexes,
before browser/native qualification. It forbids removals and does not perform
a general distribution upgrade. Native dpkg ordering admits that revision or a
later one; unavailable updates or a lower installed revision fail the job.
Before/candidate/after evidence joins the existing Linux artifact even if a
later step fails. Only the disposable Ubuntu22.04 CI job is changed.

The precursor OVAL version screen retained the 2026-11-17T08:57:09 Canonical
feed, compressed SHA-256
`a202e26afa0feed7bd411a8cbd30c7c4f2a1fd1105179cd5c87b6cf9004166f4`.
It compared 43 of 69 observed package names with supported version/presence
constraints and identified six glibc rows below the fixed boundary. This is
not a complete Boolean OVAL evaluator or vulnerability clearance. In particular
CVE-2026-97399 is Power8-specific, not an AMD64 finding; other advisories have
individual environment, resolver or AT_SECURE prerequisites. No exploit or
application reachability is inferred from those package-version comparisons.

Local workflow YAML parsing, exact extracted Bash syntax and whitespace checks
pass. Existing APT controls pass six cases with one explicit native-APT skip
on macOS. The installed Playwright dependency installer includes apt-get update;
no new index refresh or broad upgrade is substituted for that existing step.

Fresh native package evidence, unchanged iframe/browser/desktop workflows,
and the final mapped-library inventory are still required on this head.
Local YAML/shell checks alone cannot qualify the installed revision. Existing
component licenses, other deployment inventories, source-built guacd/tmux and
all other #91/#429/roadmap/release acceptance remain separate and open.
