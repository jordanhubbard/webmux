# Fresh Unix source build (#758)

The native Linux x86_64 build at `228bae10a8d2762c53545d0e9d99a17bfcf9fe75`
completed with no initial checkout-local toolchains, node_modules or binaries.
The host had Node 22 and no Go on its ordinary PATH. The Make entry point
installed checksum-verified Node 24.22.3 and Go 1.27.4, installed the locked
JavaScript build dependencies, and produced both native executables. The source
lockfile was unchanged. See [receipt.json](receipt.json).

The actual build command was `NODE_ENV=production GOMAXPROCS=2 GOFLAGS=-p=2 make`.
Only compiler concurrency was capped. Using production NODE_ENV confirms the
build includes its development tools explicitly. The fixture was an exported
source archive rather than a linked Git worktree or an installed service.

Validation on the same native worker:

- `python3 -m unittest scripts/test_bootstrap_toolchain.py -v`: 7 passed.
- `go test -count=1 -v ./cmd/build ./internal/buildinfo`: 3 passed.
- `node --test scripts/make-controls.test.mts`: 12 passed, no skips.

The existing running-server metadata regression now puts an invalid Git marker
above its temporary staging directory. It confirms staging succeeds and that
WebMux still reports the original observed source identity; the Go builder uses
its explicit metadata rather than automatic VCS discovery in the copied tree.
The source-observation test also retains clean, dirty, unknown and foreign Git
cases. Existing service-ordering tests remain unchanged.

The original missing-Go failure, the first root-owned Go-cache failure and the
subsequent automatic-VCS failure are retained in private evidence with their
raw logs. No host cache ownership, installed service, original terminal session
or Git configuration was changed. Existing modified primary-checkout content
was preserved. These results do not qualify other operating systems or complete
the separate four-session continuity and downstream native CI requirements.
No GitHub Actions jobs were used.
