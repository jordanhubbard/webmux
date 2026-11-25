# Private Codex daemon upgrade qualification

`webmux/scripts/qualify-codex-daemon-upgrade.mjs` checks an initial and successor
macOS Codex executable against a synthetic local model provider. It creates a
fresh private home, project and Unix socket. It never accepts an existing home,
socket or thread ID, replaces an installed daemon, or sends a real coding task.
Node 24, Python 3, `codesign`, and the existing locked WebMux dependencies are
required; the tool does not install dependencies.

Run from the checkout with exact executable SHA-256 values and a new output path:

```sh
node webmux/scripts/qualify-codex-daemon-upgrade.mjs \
  --old /absolute/path/to/old/codex \
  --old-sha256 OLD_SHA256 \
  --candidate /absolute/path/to/candidate/codex \
  --candidate-sha256 CANDIDATE_SHA256 \
  --output /absolute/path/to/new-receipt-directory
```

The argument names describe the forward upgrade direction: `--old` runs first
and `--candidate` runs second. To qualify backward state compatibility, supply
the newer image as `--old` and the previous image as `--candidate`, with their
matching hashes and another new output directory. Keep both receipts. A passing
forward run does not establish backward compatibility.

The tool verifies both hashes and strict code signatures, then launches only its
own private server processes. Socket ownership is checked against each child
using macOS `SO_PEERPID`, in addition to private socket permissions. One fixture
turn starts on the old server. During its managed shutdown, the fixture completes
a synthetic response; the successor must continue the persisted active goal
before a client reconnects. Assertions cover loaded active and idle thread IDs,
model, cwd, approval policy, dynamic tool, goal objective/status, creation time,
10,000-token budget, 20 tokens already used, and unchanged configuration bytes.

`receipt.json`, private server logs and synthetic request bodies are retained in
the output directory. A slow private listener also gets a bounded process sample.
The receipt records both server PIDs and their terminal status. The tool sends
cleanup signals only through child process objects it created. A nonzero exit or
incomplete cleanup is a failure. Keep failed receipts; a later pass does not
explain an earlier startup failure.

The private fixture home is retained at the path recorded in the receipt for
inspection. It has no copied user credentials. Test credentials and model replies
are synthetic. Remove only that recorded fixture after preserving any useful
evidence; do not use this tool to clean other homes.

A passing receipt proves this private protocol scenario for the exact two images.
It does not prove uninterrupted old-turn completion: the shutdown snapshot can
record an interrupted turn, which the receipt reports explicitly. It also does
not prove actual four-session model work, original viewer input handling, all
helper process settlement, rollback of a live workload, final native CI, or an installed
upgrade. Those remain separate requirements before a checkpointed activation.

Tracked in [#752](https://github.com/jordanhubbard/webmux/issues/752).
