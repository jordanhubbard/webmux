# Copying wrapped commands from coding tools

The owner reported added line breaks when copying commands between Codex
sessions, mostly in full-screen coding-tool output (WebMux #800). The evidence
currently points toward application formatting. No WebMux newline-rewriting
change is justified by this investigation.

## Reproduction and distinction

Against unchanged WebMux application source `5c38c2ce877b11783d439cfe420694a85b9908c8`,
actual browser tests create a PTY, print one long logical line followed by an
intentional newline, select through multiple rendered rows with the mouse, and
inspect the real host clipboard. Fresh output and reconnected/snapshot output
preserve the continuous logical line and the intentional break. Automatic copy,
keyboard copy and the toolbar Copy action have explicit clipboard checks.

A controlled application that instead emits CRLF between display-width chunks
reproduces the reported symptom. The original diagnostic assertion expected
one logical line and failed, showing the application-provided row breaks in the
clipboard. Its failed log and browser attachment remain private. This is an
intentional wire-level reproduction, not an actual Codex runtime trace.

The regression distinguishes these cases: terminal soft wraps join; application
hard newlines stay. The same PTY fixture then supplies original source text
through OSC 52, and the clipboard receives the exact original logical line plus
its intentional newline. Platform CRLF/LF encoding is normalized only in the
test comparison; additional line breaks and characters are still significant.
A preliminary narrow-viewport run also passed, but it did not establish a PTY
column change and is not counted as terminal-resize qualification.

## Codex source evidence

The read-only Codex checkout at `00d3e8c2ac9694e5c0fa4891f198b52455a17467`
has two explicit policies. The committed
[history policy selector](https://github.com/jordanhubbard/codex/blob/00d3e8c2ac9694e5c0fa4891f198b52455a17467/codex-rs/tui/src/app/resize_reflow.rs#L245)
chooses terminal wrapping for raw output and pre-wrapping otherwise. The
[history writer](https://github.com/jordanhubbard/codex/blob/00d3e8c2ac9694e5c0fa4891f198b52455a17467/codex-rs/tui/src/insert_history.rs#L123)
splits pre-wrapped lines and writes CRLF between rendered rows. Once those
bytes reach a terminal, a visual formatting break is indistinguishable from
an intentional command newline without application-level source information.

This establishes a concrete source mechanism consistent with the report; it
does not prove the exact bytes emitted by the owner's affected session. The
installed Puck CLI reports 0.172.0; its executable contains the copy menu and
raw-output feature descriptions. That is command-surface evidence, not an
end-to-end test of that installed Codex session. The inspected source checkout
is separate from that installed executable. No Codex source, owner profile,
credentials or running session was modified.

## Copy commands from their source

In the source Codex session, use `/copy` and choose the relevant code block or
response. In Codex fullscreen mode, use its transcript block selection and
Enter to copy. Use `/raw on` when ordinary terminal mouse selection is preferred;
`/raw off` restores formatted output. See the current official
[Codex command reference](https://learn.chatgpt.com/docs/developer-commands?surface=cli#copy-the-latest-response-with-copy).
These commands retain the application's source text rather than asking the
terminal to infer which rendered rows belong together.

Do not globally strip clipboard newlines: that would join separate shell
commands and damage multiline code. The current WebMux tests establish its
soft-wrap, hard-break and source-text clipboard behavior on the tested browser;
other providers and the owner's exact native reproduction remain unverified.
Issue #800 remains open for that provider-specific follow-up.
