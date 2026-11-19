# Offline pinned USB fixture sources

The original4860 USB job135399141113 fails before its guest/kernel acceptance.
Its source receipt shows vudc_rx.c receiving HTTP429 on all three attempts; total
acquisition is13.218s of the unchanged90-second budget. The earlier checksum
traceback is an expected passing negative test, not the failure. The original
773-byte artifact, source receipt and job log remain retained.

The [receipt](receipt.json) records eight unmodified Linux files (60152bytes),
all matching the existing commit and SHA-256 manifest. They retain original
SPDX/copyright headers, upstream license text and exact provenance. A verified
cache remains first choice, followed by the repository bundle; unbundled commits
keep the original network path. Source hash/size and regular-file/link checks
precede cache/build writes. A corrupt/incomplete bundle refuses without network
fallback. The manifest, patches, retry count,90-second transport budget and all
native bad/fixed/USB-over-SSH assertions remain unchanged.

Eleven acquisition controls and nine qualifier/builder controls pass locally.
The real bundle fills a cold private cache with all eight exact files while
network calls are forbidden, then that cache works with the bundle disabled.
Corrupt, oversized, linked, nonregular and incomplete inputs refuse. Existing
network/retry/budget checks still pass. An original-parent control fails its
blocked transport even with the same bundle present, demonstrating the removed
network dependency. These are acquisition controls, not native guest results.

The bundle is solely disposable kernel-test source, outside the WebMux runtime.
Deployment-specific vendor/license review under #91 remains separate. Final
native qualification needs the combined source, including #549's evidenced
signed-HTTPS relay provisioning correction. No unchanged failed native job was
restarted, no timeout increased and no native gate was exempted.

Refs #550, #120, #445, #452, #91.

The subsequent [composition](../relay-usb-composition-2026-11-19/README.md)
now includes the published signed-HTTPS relay build inputs. Its combined native
qualification remains pending; local/static pass is not guest or relay acceptance.
