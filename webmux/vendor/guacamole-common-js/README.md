# Apache Guacamole browser library

`package.tgz` is WebMux's local ESM packaging of **Apache Guacamole 1.6.0**,
not a release published by Apache to npm. It replaces the third-party npm
wrapper at 1.5.0. The package's `1.6.0-webmux.1` version identifies this packaging;
`Guacamole.API_VERSION` remains upstream's `1.6.0`.

The input is the official `guacamole-client-1.6.0.tar.gz`, SHA-256
`81f9fd5a7b4377fb0ee295d0d4fec92e9667f2aafaa3d0ed8937f535deabdee4`.
Its detached signature is verified against Apache's KEYS and pinned signer
`F467E54ACC52F1D2778826865B2977AEE5E4518F` (Michael Jumper's code signing key).
See [Apache's release](https://guacamole.apache.org/releases/1.6.0/).

The generator combines upstream `common/license.js`, then its sorted
`modules/*.js`, preserving source bytes and adding `export default Guacamole`.
There is no minification, runtime patch, install hook or additional dependency.
The tar members and gzip header have fixed metadata. `provenance.json` records
the source URL, archive digest, signer, per-module digests and package digest.
The workspace lockfile additionally pins the local package's SHA-512 integrity.
`LICENSE` and `NOTICE` are retained unchanged and copied into native server
bundles. Development-only DefinitelyTyped declarations remain separately
versioned; they do not supply runtime code.

To reproduce, download the archive and its `.asc` from the release page, and
`https://downloads.apache.org/guacamole/KEYS`. From the repository root:

```sh
python3 scripts/vendor-guacamole.py \
  --archive /path/to/guacamole-client-1.6.0.tar.gz \
  --signature /path/to/guacamole-client-1.6.0.tar.gz.asc \
  --keys /path/to/KEYS \
  --output webmux/vendor/guacamole-common-js/package.tgz --check
```

Omit `--check` to regenerate all four vendored files after reviewing an upstream
update and changing the pinned version, checksum and signer in the generator.
Ordinary builds use the checked-in package and need no Apache download or GPG.
This browser library does not bundle or inventory an operator's `guacd`, RDP
plugins, FreeRDP or other native libraries.
