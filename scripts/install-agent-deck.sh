#!/bin/sh
set -eu

# Install the Agent Deck version qualified by WebMux. The upstream installer
# verifies the release checksum before extracting the binary; pinning the
# installer itself to the same tag keeps source installs reproducible.
version=${AGENT_DECK_VERSION:-v1.16.34}
bindir=${AGENT_DECK_BINDIR:-"$HOME/.local/bin"}
installer_url=${AGENT_DECK_INSTALLER_URL:-"https://raw.githubusercontent.com/asheshgoplani/agent-deck/${version}/install.sh"}

case "$version" in
  v[0-9]*.[0-9]*.[0-9]*) ;;
  *) printf 'Invalid AGENT_DECK_VERSION: %s\n' "$version" >&2; exit 2 ;;
esac

minimum=${version#v}

version_at_least() {
  awk -v have="$1" -v need="$2" 'BEGIN {
    split(have, h, "."); split(need, n, ".")
    for (i = 1; i <= 3; i++) {
      if ((h[i] + 0) > (n[i] + 0)) exit 0
      if ((h[i] + 0) < (n[i] + 0)) exit 1
    }
    exit 0
  }'
}

candidate=
if command -v agent-deck >/dev/null 2>&1; then
  candidate=$(command -v agent-deck)
elif [ -x "$bindir/agent-deck" ]; then
  candidate="$bindir/agent-deck"
fi

if [ -n "$candidate" ]; then
  output=$("$candidate" version 2>/dev/null || true)
  installed=$(printf '%s\n' "$output" | sed -n 's/^Agent Deck v\([0-9][0-9.]*\).*$/\1/p' | head -1)
  if [ -n "$installed" ] && version_at_least "$installed" "$minimum"; then
    printf '\033[32m✓\033[0m Agent Deck %s found: %s\n' "$installed" "$candidate"
    exit 0
  fi
  printf 'Agent Deck at %s is older than WebMux requires (%s); upgrading.\n' "$candidate" "$minimum"
fi

command -v curl >/dev/null 2>&1 || {
  printf 'curl is required to install Agent Deck from %s\n' "$installer_url" >&2
  exit 1
}
command -v bash >/dev/null 2>&1 || {
  printf 'bash is required by the Agent Deck installer.\n' >&2
  exit 1
}

temporary=$(mktemp "${TMPDIR:-/tmp}/webmux-agent-deck.XXXXXX")
trap 'rm -f "$temporary"' EXIT HUP INT TERM
curl -fsSL "$installer_url" -o "$temporary"
bash "$temporary" --version "$version" --dir "$bindir" --non-interactive

installed_output=$("$bindir/agent-deck" version 2>/dev/null || true)
installed=$(printf '%s\n' "$installed_output" | sed -n 's/^Agent Deck v\([0-9][0-9.]*\).*$/\1/p' | head -1)
if [ -z "$installed" ] || ! version_at_least "$installed" "$minimum"; then
  printf 'Agent Deck installation did not provide version %s or newer at %s/agent-deck\n' "$minimum" "$bindir" >&2
  exit 1
fi
printf '\033[32m✓\033[0m Installed Agent Deck %s: %s/agent-deck\n' "$installed" "$bindir"
