#!/bin/sh
# Provision missing source-build tools without sudo or changes to the user's PATH.
# Pinned digests: https://nodejs.org/dist/v24.22.3/SHASUMS256.txt
# and https://go.dev/dl/?mode=json&include=all (go1.27.4).
set -eu
tool=$1
command=$2
root=$3
case "$tool" in
  node) version=24.22.3; minimum=24.0.0 ;;
  go) version=1.27.4; minimum=$(awk '$1 == "go" { print $2; exit }' "$4") ;;
  *) echo "Unknown build tool: $tool" >&2; exit 1 ;;
esac
at_least() {
  awk -v actual="$1" -v required="$2" 'BEGIN {
    split(actual, a, "."); split(required, b, ".");
    for (i=1; i<=3; i++) {
      if (a[i]+0 > b[i]+0) exit 0;
      if (a[i]+0 < b[i]+0) exit 1;
    }
  }'
}
usable() {
  case "$tool" in
    node) actual=$("$command" --version 2>/dev/null) || return 1; actual=${actual#v} ;;
    go) actual=$("$command" version 2>/dev/null | awk '{ sub(/^go/, "", $3); print $3 }') || return 1 ;;
  esac
  case "$actual" in ''|*[!0-9.]*) return 1 ;; esac
  at_least "$actual" "$minimum"
}
if usable; then
  # npm ships with Node, but some OS distributions split it into another package.
  if [ "$tool" != node ] || command -v npm >/dev/null 2>&1; then exit 0; fi
fi
if [ "$command" != "$tool" ]; then
  echo "Explicit $tool override '$command' must provide $tool >= $minimum." >&2
  exit 1
fi
case "$(uname -s)" in Linux) os=linux ;; Darwin) os=darwin ;; *) echo "Install $tool >= $minimum for this platform, or set ${tool} explicitly." >&2; exit 1 ;; esac
case "$(uname -m)" in x86_64|amd64) arch=x64 ;; arm64|aarch64) arch=arm64 ;; *) echo "Install $tool >= $minimum for this architecture." >&2; exit 1 ;; esac
if [ "$tool" = node ]; then
  archive=node-v$version-$os-$arch.tar.gz
  url=https://nodejs.org/dist/v$version/$archive
  top=node-v$version-$os-$arch
  case "$os-$arch" in
    linux-x64) sha=061ec3e3062115705b56b51fa9cc330ab50d34b6969387d4e4b1a50c6809acc2 ;;
    linux-arm64) sha=261af2af250dfd7e9f25cf151d1b2713a0adf8f10a9ad116c4947f68627affe8 ;;
    darwin-x64) sha=977614af0504c81045e7fd85596dbca0e5f951b96681516dadafedcca8c447f7 ;;
    darwin-arm64) sha=ff6e17d4874b76d272212b908cc020d6e815fba5a6c7eaaa06add725840307fd ;;
  esac
else
  if ! at_least "$version" "$minimum"; then
    echo "Update the pinned Go bootstrap ($version) to satisfy go.mod ($minimum)." >&2; exit 1
  fi
  goarch=$arch; [ "$arch" != x64 ] || goarch=amd64
  archive=go$version.$os-$goarch.tar.gz
  url=https://go.dev/dl/$archive
  top=go
  case "$os-$arch" in
    linux-x64) sha=7226736c2339719cda28fbf3a54d71aa63bcefe376e98cef00034268dab24840 ;;
    linux-arm64) sha=3392582988a273a87ab5043ee1b2c9c17a2c3c00580bf9326c47a2e0aaaeac92 ;;
    darwin-x64) sha=2b799377704ef12db39bd254995d5c348c9cc8d86562ca817724390e56c38f43 ;;
    darwin-arm64) sha=91a44a2b8a716600ad94a4f143e156116620db8828086a6e3602291be9bd0bb4 ;;
  esac
fi
mkdir -p "$root"
lock=$root/.$tool.lock
attempt=0
until mkdir "$lock" 2>/dev/null; do
  attempt=$((attempt + 1))
  if [ "$attempt" -ge 120 ]; then echo "Timed out waiting for $lock; inspect the other build before removing its lock." >&2; exit 1; fi
  sleep 1
done
stage=
trap 'test -z "$stage" || rm -rf "$stage"; rmdir "$lock"' EXIT
trap 'exit 1' HUP INT TERM
# Another make invocation may have finished while this one waited for the lock.
if usable && { [ "$tool" != node ] || command -v npm >/dev/null 2>&1; }; then exit 0; fi
if [ -e "$root/$tool" ] || [ -L "$root/$tool" ]; then
  echo "Existing $root/$tool is unusable; preserve and move it before retrying." >&2; exit 1
fi
stage=$(mktemp -d "$root/.$tool.XXXXXX")
echo "Installing $tool $version into $root/$tool" >&2
if command -v curl >/dev/null 2>&1; then
  curl --fail --location --show-error --connect-timeout 15 --max-time 300 --output "$stage/$archive" "$url"
elif command -v wget >/dev/null 2>&1; then
  wget --timeout=30 --tries=2 -O "$stage/$archive" "$url"
else
  echo 'Source bootstrap requires curl or wget.' >&2; exit 1
fi
if command -v sha256sum >/dev/null 2>&1; then
  actual_sha=$(sha256sum "$stage/$archive" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
  actual_sha=$(shasum -a 256 "$stage/$archive" | awk '{print $1}')
else
  echo 'Source bootstrap requires sha256sum or shasum.' >&2; exit 1
fi
[ "$actual_sha" = "$sha" ] || { echo "Checksum mismatch for $archive" >&2; exit 1; }
tar -xzf "$stage/$archive" -C "$stage"
case "$tool" in
  node) "$stage/$top/bin/node" --version >/dev/null ;;
  go) "$stage/$top/bin/go" version >/dev/null ;;
esac
mv -f "$stage/$top" "$root/$tool"
usable || { echo "Bootstrapped $tool is not selected; check PATH and overrides." >&2; exit 1; }
