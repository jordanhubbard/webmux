# Observe native components on a deployment

WebMux uses separately installed OpenSSH, tmux for built-in Unix local terminals,
optional mosh and a separately managed guacd endpoint. An npm or Go scan does not inventory these tools or
guacd's loaded native libraries. Collect an observation on each relevant host:

```sh
python3 scripts/native-components.py --output native-components.json
```

The command hashes PATH-selected executables and records their bounded version
probes. Missing tools, failed queries and unknown package ownership remain
explicit. Use `--tool NAME=/path/to/executable` to select a different `ssh`,
`mosh`, `mosh-client`, `mosh-server`, `tmux` or `guacd`. The mosh wrapper and its
native client/server are inventoried separately. The command does not install software, connect to an
SSH host, read process arguments/environment, or change services. Output files
are created exclusively, with mode 0600 on POSIX; select a new output name for
each observation. They contain local filesystem paths, so review them before
sharing.

## Loaded guacd plugins and libraries on Linux

Choose an active guacd connection process owned by your current account, after
its RDP plugin has loaded. The daemon's listening parent alone may not have
the same plugins as its connection workers. Set `GUACD_PID` to the selected
process ID. For a private build from [the RDP guide](rdp-proxy.md):

```sh
LD_LIBRARY_PATH="$HOME/guacd-1.6/lib" python3 scripts/native-components.py \
  --tool guacd="$HOME/guacd-1.6/sbin/guacd" \
  --guacd-pid "$GUACD_PID" --output guacd-components.json
```

The collector reads one [`/proc/PID/maps` snapshot](https://man7.org/linux/man-pages/man5/proc_pid_maps.5.html),
records mapped ELF files and observed plugin names, and checks the process start
identity before and after collection. It compares each mapped device/inode with
the file it hashes. Deleted or replaced files remain unknown; it cannot claim
that their current path describes the loaded file. Ambiguous escaped paths,
an inaccessible/exited/replaced process or an oversized mapping inventory refuse
the process report. No successful JSON is written for a refused process.

The process's `versionProbe` is supplied only when the selected guacd executable
matches the recorded process image. A different PATH installation cannot supply
that version. Hashes describe matching **on-disk** files, not process memory.
The observation does not cover another worker, an unloaded plugin, future
library loads, or a remote guacd endpoint. No plugin names means none were
observed, not proof that no plugins are installed.

On Debian/Ubuntu, exact package-path ownership supplies installed package
versions, architectures, source package names and hashed distributor copyright
documents. Declared `License:` fields are recorded where present; these are
package declarations, not a license determination. Package ownership does not
prove the installed bytes match the distributor's archive. Custom source builds
are marked unowned: retain their signed-source/build receipts separately.
Without dpkg, package attribution is explicitly unsupported. Live process maps
are Linux-only; macOS qualification here covers tool identities/version probes.

## Review and qualification

Use distributor security trackers for the **exact package revision and OS**, and
retain source provenance for custom builds. A version banner alone does not
establish whether an OS vendor backported a fix. Primary component references
include [OpenSSH security](https://www.openssh.org/security.html),
[Mosh releases](https://github.com/mobile-shell/mosh/releases),
[tmux releases](https://github.com/tmux/tmux/releases) and
[Apache Guacamole security reports](https://guacamole.apache.org/security/).
This collector does not contact these services or issue a vulnerability-clean
verdict. The deployment inventory, license review and vulnerability assessment
remain distinct work under [#91](https://github.com/jordanhubbard/webmux/issues/91).

Portable controls cover process replacement, mapped-file identity/deletion,
account isolation, exact package ownership, failed queries, lost-process refusal
and non-overwriting output. The opt-in live test loads a real guacd RDP plugin
on loopback without credentials or an RDP target. It checks native library hashes,
package metadata, executable/version identity, mismatched PATH refusal and owned
listener/process cleanup. CI runs it against its verified private proxy and
retains raw JSON/logs in `browser-parity-linux`.

The Linux ARM64 live test passed with 79 mapped ELF files, 76 package records
and the loaded RDP plugin. macOS tool observations recorded OpenSSH 10.5p1,
mosh 1.4.0, a custom tmux `next-3.7`, and guacd 1.6.0. The tmux banner does not
establish its source revision. These are host observations, not a complete fleet
SBOM, a clean security verdict, functional RDP qualification or authenticated
coding-session recovery. [Source bindings and results](validation/native-components-2026-11-18/README.md)
record their scope.
