# Windows MCP fixture comparison

On 2026-11-26, the required live elicitation fixture passed the original native
135-case Windows cohort. This is a scoped diagnostic for
[#760](https://github.com/jordanhubbard/webmux/issues/760), not full Windows CI
or qualification of the installed runtime.

The warm-only control was `7a25e6ba907acbe3ebead28b304818e0794a1644`.
The candidate was `cdf75ab609e0a82fad4b6622dbcd746349f63672` in
[native PR #23](https://github.com/jordanhubbard/codex/pull/23).
Both warm the first thread's MCP server before starting its replacement. The
candidate additionally marks the synthetic server required; the sources differ
only by that setting and its two explanatory comments. Production behavior,
assertions, deadlines, native concurrency, and retry policy were unchanged.

The existing native wrapper selected the original fifth cohort of sixteen.
Every attempt contained the same 135 cases in batches of 66 and 69, including
the same three pre-existing ignores. Native test results were freshly executed;
compiled build actions could use the existing cache.

| Variant | Native attempts | Result |
| --- | --- | --- |
| Warm-only control | 3 | MCP startup failed on every attempt with an empty ready-server list. The first attempt also timed out in the screenshot case; that case passed on attempts two and three. |
| Required-server candidate | 1 | Passed: 132 cases passed, three ignored, zero failures. |

The candidate target completed in 47.4 seconds. All three owned normal Bazel
shutdowns succeeded for each variant. The controller restored the original
clean source revision. Preservation found no unresolved native helpers.

Both archives were copied off the worker and independently checked against
every manifest hash, native BEP, log, and XML reference:

| Variant | Bytes | Files | Archive SHA-256 |
| --- | ---: | ---: | --- |
| Control | 97138 | 27 | `a4c51ea3c47e9e9d472c4dc4841cf4af29d7080f99679c7641ebb40108f42c7d` |
| Candidate | 63101 | 23 | `f35e93e58862129d0aa43f944eed20010de472dc446abcad2a1469bf7026ee33` |

The full sixteen-shard target and remaining required platform gates still need
qualification. Existing failures in #742, #751, and #757 remain open. No hosted
Actions, installed services, original coding requests, or global configuration
were changed by this comparison.
