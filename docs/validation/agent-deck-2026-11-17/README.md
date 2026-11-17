# Agent Deck 1.16.39 qualification

Qualification is in progress under #414. This candidate advances the optional
coordinator minimum from 1.16.34 to 1.16.39 and removes the broker's startup-query
reply exception. Structured native recovery remains exact-version gated.
A bounded preflight in the attachment child rejects older coordinators with an
update instruction while preserving the existing provider. It does not read
terminal input, alter the attach environment or block the broker's other jobs.
Version output is limited to 256 bytes and the version process group to three
seconds. Its diagnostic-only child uses TERM=dumb; the actual attachment keeps
the original TERM and other environment values.
Do not infer acceptance from the upstream fix or a successful version command.

The release contains upstream agent-deck#3113 (66190b4485025e51bb61b66cd6154b765a764a34),
which removes the CLI initialization probe outside its TUI. Published archive
checksums and the staged binary digests are in sources.json. The ordinary
installed coordinator remains unchanged during qualification.

Required evidence before this candidate is integrated:

- Actual private PTY baseline/fixed comparisons at 0.3, 1 and 3 seconds, with
  native-client handoff before late control input, no fabricated terminal
  replies, exact payload delivery and verified cleanup. The legacy library
  waits five seconds per read; sending late control input before the native
  handoff would prolong that probe instead of testing delivery.
- Actual old-record/new-coordinator Agent Deck/Claude continuation through controller loss, exact native
  conversation/project/policy identity and once-only recovery.
- Actual disposable guest reboot and fresh remembered-marker answers, with
  original roots, native-file identity and durable duplicate receipts.
- Actual local/SSH/remote attachment and host-failure fallback, preserving private
  coordinator homes and unchanged startup deadlines.
- Relevant Go race/vet, installer/frontend checks and all native/hosted gates.

The retained reboot fixture runs from a private copy-on-write overlay on the
healthy NAS. QMP confirms its original backing layers are read-only; only the
owned guest is a reboot target. Its existing guest-native Claude sign-in is
used directly. No workstation reboot, installed service replacement or copied
authentication material is part of this qualification.
