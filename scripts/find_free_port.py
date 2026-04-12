#!/usr/bin/env python3
"""
find_free_port.py — find an available TCP port, starting from a preferred port.

Usage:
    python3 scripts/find_free_port.py <preferred_port> [--host HOST] [--range N]

Outputs the free port number to stdout (just the number).
Informational messages go to stderr so they don't pollute shell capture.
Exit 0 on success, 1 if no free port found within the search range.

Works on macOS, Linux, and Windows.
"""
from __future__ import annotations

import argparse
import socket
import sys


def is_port_free(port: int, host: str = '0.0.0.0') -> bool:
    """Return True if host:port can be bound (i.e. nothing is using it)."""
    try:
        with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
            # SO_REUSEADDR intentionally NOT set — we want a strict availability check.
            s.bind((host, port))
        return True
    except OSError:
        return False


def find_free_port(preferred: int, host: str = '0.0.0.0', search_range: int = 20) -> int:
    """Return the first free port in [preferred, preferred + search_range).

    Returns -1 if no free port is found in that window.
    """
    for port in range(preferred, min(preferred + search_range, 65536)):
        if is_port_free(port, host):
            return port
    return -1


def main() -> None:
    parser = argparse.ArgumentParser(
        description='Find a free TCP port starting from a preferred port.'
    )
    parser.add_argument(
        'port',
        type=int,
        help='Preferred port number (e.g. 8080)',
    )
    parser.add_argument(
        '--host',
        default='0.0.0.0',
        help='Address to bind-check (default: 0.0.0.0 — all interfaces)',
    )
    parser.add_argument(
        '--range',
        type=int,
        default=20,
        dest='search_range',
        metavar='N',
        help='How many ports above preferred to search (default: 20)',
    )
    args = parser.parse_args()

    if not (1 <= args.port <= 65535):
        print(f'error: port {args.port} is out of range (1-65535)', file=sys.stderr)
        sys.exit(1)

    free = find_free_port(args.port, args.host, args.search_range)

    if free < 0:
        hi = min(args.port + args.search_range - 1, 65535)
        print(
            f'error: no free port found in range {args.port}-{hi}',
            file=sys.stderr,
        )
        sys.exit(1)

    if free != args.port:
        print(
            f'warning: port {args.port} is in use; using {free} instead',
            file=sys.stderr,
        )

    print(free)


if __name__ == '__main__':
    main()
