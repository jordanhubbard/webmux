# Explicit public Ubuntu trust for the disposable relay image

HTTPS-only source `4623b2290d4664b752bd6c2fe9b4b4b9928717cf` failed in original CI44479889268, Linux job135409295257. Checked local Stop passed; three build-input controls then refused because public trust-root enumeration was empty or oversized. Docker relay construction was never reached. The exact hosted Python/OpenSSL default-path cause was not recorded and remains unproven.

[Python documents lazy CA-directory enumeration](https://docs.python.org/3/library/ssl.html#ssl.SSLContext.get_ca_certs). A real OpenSSL control with unavailable default paths reproduces the parent refusal. A separate hashed-directory control also returns zero eagerly loaded roots. Neither establishes the exact hosted configuration.

On Linux, load the Ubuntu runner's existing public `/etc/ssl/certs/ca-certificates.crt` explicitly before exporting the bounded DER-to-PEM certificate set. Other platforms retain their default public store. This changes only disposable fixture build inputs; no host trust settings or private keys change. Missing, malformed, empty or oversized trust material still refuses before writing build inputs. TLS verification and Ubuntu archive signatures remain required.

Thirteen of fourteen controls pass on macOS; the effective APT case explicitly skips. All fourteen pass in an existing isolated Linux AMD64 container on the ARM64 Docker host, including effective HTTPS APT selection. All fourteen also pass with unavailable Python default CA paths; no network, host writes, package installation or image pull is permitted in these controls. The original parent fails under the same unavailable-default-store control; the correction exports exactly the supplied public root through real OpenSSL parsing. Missing/invalid bundle refusals and certificate/hostname verification controls pass.

The entire native main function and bounded logger/stream helpers are AST-identical to published parent2618. Native assertions, timeouts, SSH control and cleanup remain unchanged. These controls are not a successful native Docker/SSH/browser relay qualification. Parent2618 CI44483208381 and ARM44482973915 remain original handles; this correction does not restart them.

References: #549, #550, #552, #452.
