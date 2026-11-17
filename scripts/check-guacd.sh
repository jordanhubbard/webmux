#!/bin/sh
# RDP is optional; warn without blocking unrelated terminal startup.
if ! command -v guacd >/dev/null 2>&1; then
    printf '%s\n' '! guacd not found. RDP requires guacd 1.6.0 or newer; see docs/rdp-proxy.md.'
    exit 0
fi
output=$(guacd -v 2>&1) || output='unknown'
version=$(printf '%s\n' "$output" | awk '/version [0-9]+\.[0-9]+\.[0-9]+/ { for (i=1;i<=NF;i++) if ($i=="version") {print $(i+1);exit} }')
if [ -n "$version" ] && printf '%s\n' "$version" | awk -F. '{exit !($1>1 || ($1==1 && $2>=6))}'; then
    printf '%s\n' "✓ guacd $version found: $(command -v guacd)"
else
    printf '%s\n' "! guacd ${version:-version unknown}: RDP requires guacd 1.6.0 or newer."
    printf '%s\n' '  Older daemons can lose the initial desktop size during connection setup; see docs/rdp-proxy.md.'
fi
