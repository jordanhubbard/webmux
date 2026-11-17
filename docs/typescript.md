# TypeScript source coverage

`npm --prefix webmux run typecheck` checks every maintained TypeScript file
against the five strict project configurations and rejects maintained JavaScript.
The inventory includes tracked and untracked files, honors Git ignores, resolves
real paths, and rejects source outside the compiler's checked graph.

The checker uses the compiler CLI: `--showConfig` resolves inherited strict
options; `--listFilesOnly` supplies the actual source/import graph; the normal
`--noEmit` invocation performs type checking. `--showConfig` alone is insufficient:
it can omit files matched by include patterns. Config errors, non-strict projects,
missing inventories and compiler failures stop the gate.

This avoids the JavaScript compiler API removed from the native TypeScript 7
package. Real compiler controls cover inherited configs, paths with spaces,
included and excluded files, tracked/untracked inventory, ignored generated
JavaScript, non-strict overrides and invalid config inheritance. CI runs them
with the project compiler and an isolated pinned TypeScript 7.0.2 toolchain on
Linux and Windows; that probe does not alter the workspace lockfile.

[#264](https://github.com/jordanhubbard/webmux/issues/264) remains open for the
complete dependency migration. On 2026-11-17 UTC, the current project compiler
5.9.3 and native 7.0.2 both passed the discovery controls; all five actual project
configurations also type-checked with the isolated native compiler. However,
`@typescript-eslint/parser` 8.73.0 still declares TypeScript `>=4.8.4 <6.1.0`.
Keep the supported project compiler until the lint toolchain admits TypeScript 7,
then update the lockfile and qualify lint, helpers, builds and native platforms.
