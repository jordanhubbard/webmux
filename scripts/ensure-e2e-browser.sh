#!/usr/bin/env bash
# Resolve a working Chromium for the Playwright e2e suite.
#
# Prints the chosen browser executable path to STDOUT (empty string means
# "use Playwright's managed browser as-is"). All diagnostics go to STDERR so
# the caller can capture the path cleanly:
#
#     EXE=$(scripts/ensure-e2e-browser.sh)
#     PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH="$EXE" npm run test:e2e
#
# Resolution order:
#   1. PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH — explicit override wins.
#   2. Playwright's managed Chromium — installed (idempotent) AND verified to
#      actually launch. The launch check guards against partial/corrupt
#      downloads that install "successfully" but are missing the binary.
#   3. A system Google Chrome / Chromium — fallback for offline or sandboxed
#      environments where the managed download can't complete.
# Exits non-zero with an actionable message if none work.

set -euo pipefail

err() { printf '%s\n' "$*" >&2; }

# Portable timeout wrapper (macOS has no `timeout`/`gtimeout` by default): run a
# command with a watchdog so a wedged download/launch can never hang the test
# run. Returns the command's exit code, or non-zero if the watchdog killed it.
run_bounded() {
  local secs=$1; shift
  "$@" &
  local pid=$!
  ( sleep "$secs"; kill -0 "$pid" 2>/dev/null && kill -9 "$pid" 2>/dev/null ) &
  local watcher=$!
  local rc=0
  wait "$pid" 2>/dev/null || rc=$?
  kill "$watcher" 2>/dev/null || true
  wait "$watcher" 2>/dev/null || true
  return "$rc"
}

# 1. Explicit override.
if [[ -n "${PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH:-}" ]]; then
  err "▸  e2e browser: using PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH override"
  printf '%s' "$PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH"
  exit 0
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WEBMUX_DIR="$SCRIPT_DIR/../webmux"
cd "$WEBMUX_DIR"

launches_ok() {
  # True only if the managed Chromium can actually start a headless browser.
  # Bounded so a hung launch can't stall provisioning.
  run_bounded 30 node -e \
    "require('playwright-core').chromium.launch({headless:true}).then(b=>b.close()).catch(()=>process.exit(1))" \
    >/dev/null 2>&1
}

# 2. Managed browser. Fast-path: if one is already present and launches, skip the
# (slow) install entirely. Otherwise attempt a bounded install, then re-check.
err "▸  Provisioning Playwright Chromium…"
if launches_ok; then
  err "✓  e2e browser: Playwright managed Chromium (already installed)"
  printf ''
  exit 0
fi
if run_bounded "${E2E_BROWSER_INSTALL_TIMEOUT:-240}" npx playwright install chromium chromium-headless-shell >/dev/null 2>&1 && launches_ok; then
  err "✓  e2e browser: Playwright managed Chromium"
  printf ''
  exit 0
fi
err "!  Managed Chromium unavailable or failed to launch (offline/sandbox?); trying system Chrome…"

# 3. System browser fallback.
for candidate in \
  "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" \
  "/Applications/Chromium.app/Contents/MacOS/Chromium" \
  "$(command -v google-chrome 2>/dev/null || true)" \
  "$(command -v google-chrome-stable 2>/dev/null || true)" \
  "$(command -v chromium 2>/dev/null || true)" \
  "$(command -v chromium-browser 2>/dev/null || true)"; do
  if [[ -n "$candidate" && -x "$candidate" ]]; then
    err "✓  e2e browser: system Chrome at $candidate"
    printf '%s' "$candidate"
    exit 0
  fi
done

err "✗  No usable Chromium for e2e."
err "   Install one with: (cd webmux && npx playwright install chromium)"
err "   or set PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH to a Chrome/Chromium binary."
exit 1
