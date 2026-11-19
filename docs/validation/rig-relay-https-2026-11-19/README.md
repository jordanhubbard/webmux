# Signed HTTPS provisioning for the isolated Rig relay

The diagnostic source `d37ca20` retains the original timeout output from
[CI 44475746089](https://github.com/jordanhubbard/webmux/actions/runs/44475746089).
The pinned Ubuntu image downloads and extracts in about two seconds. The build
then spends the remaining 180-second budget in HTTP index acquisition from
`archive.ubuntu.com`; package installation and relay/browser qualification never
begin. Earlier failures lacked this output, so their detailed cause remains
unproven. See the [source, artifact and validation receipt](receipt.json).

The isolated image now uses the project's official Ubuntu HTTPS source
selection and bounded APT settings. Its pinned Noble base, original suites and
components, Ubuntu archive signing keyring, and `openssh-server`/`tmux` packages
remain. A read-only, digest-verified inspection of the actual AMD64 base layer
confirmed that it contains the archive keyring but no CA bundle. The build
therefore exports public CA certificates from the build host's TLS trust store
and includes those public roots in the disposable image. APT selects that PEM
bundle through its documented [CAInfo option](https://manpages.debian.org/testing/apt/apt-transport-https.1.en.html).
It copies no private
key or native account credential. The context allowlist still excludes the
generated SSH private key and all unrelated files.

The shared APT helper exposes its existing settings as a pure function; its
ordinary host configuration output stays byte-identical. The fixture reuses
that function and the existing signed-source parser. It selects only its
generated HTTPS sources while retaining the base's original source files.
TLS verification and Ubuntu package-signature verification stay enabled. Missing
or oversized public trust roots refuse the build rather than lowering trust.

Local checks pass 13 tests, with two explicit skips because this macOS host has
no APT executable. Controls cover public CA parsing, build-context exclusion,
signed HTTPS sources, normal command results and preserved partial timeout
output. Linux CI must execute both effective-APT-selection checks and the
original complete relay qualification. No native pass is claimed here.

The Docker build still has its original 180-second deadline. All native relay,
SSH, ownership, sibling and cleanup assertions, and their operation deadlines,
are unchanged. No host APT settings change, and no local Docker/native coding
fixture runs as part of these controls. Historical failures remain retained;
changed-source native CI, review, integration and the broader roadmap remain
required.
