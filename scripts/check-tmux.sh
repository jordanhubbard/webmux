#!/bin/sh
# Check the runtime dependency before installing or starting the Unix server.
set -eu

if command -v tmux >/dev/null 2>&1; then
    printf 'tmux found: %s\n' "$(command -v tmux)"
    exit 0
fi

printf '%s\n' 'tmux is required for local terminals but was not found on PATH.' >&2
case "$(uname -s)" in
    Darwin) printf '%s\n' 'Install it with: brew install tmux' >&2 ;;
    Linux)
        printf '%s\n' 'Install it using your package manager, for example:' \
            '  Debian/Ubuntu: sudo apt-get install -y tmux' \
            '  Fedora/RHEL:   sudo dnf install -y tmux' \
            '  Alpine:        sudo apk add tmux' >&2 ;;
esac
printf '%s\n' 'Ensure tmux is on the server PATH, then retry the command.' >&2
exit 1
