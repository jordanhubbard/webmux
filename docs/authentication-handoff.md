# CLI authentication from any browser

WebMux mediates **where a sign-in link opens and how its response reaches the
originating terminal**. The CLI remains the OAuth client and owns its tokens.
Signing into WebMux itself is a separate operation.

## Choose the return path

| CLI flow | Sign in | How the CLI receives approval |
| --- | --- | --- |
| Device code (for example `gh auth login`) | Your current browser | The CLI polls the provider using its private device code. Enter the displayed user code in the provider page. No callback relay is needed. |
| Provider asks you to paste a code back | Your current browser | Paste the provider's return code into the same terminal when the CLI asks. |
| OAuth with `redirect_uri=http://localhost:PORT/...` and `state` | Your current browser, after **Prepare callback relay** | Copy the final localhost URL from the other tab's address bar into **Final callback URL**, then **Deliver callback**. |
| Host-only page or unsupported callback | **Use browser on terminal host** | The existing companion runs Chromium on that host. Its existing limitations still apply. |

The sign-in panel identifies the terminal, execution host and destination origin.
**Continue in this browser** opens a separate tab using your browser's existing
cookies, SSO and passkeys. It requires your click; CLI output cannot open tabs
without interaction. Opening a tab or delivering a callback is not proof of login:
check the originating CLI for success.

A device-code login works even if your browser is on a different computer. The
provider associates the code you approve with the CLI polling for it. An expired
code, wrong account, blocked polling connection or provider policy can still
prevent login; WebMux cannot infer or override those conditions.

[GitHub documents device polling](https://docs.github.com/en/apps/oauth-apps/building-oauth-apps/authorizing-oauth-apps#device-flow).
[Codex recommends device authentication for headless environments](https://learn.chatgpt.com/docs/auth#login-on-headless-devices):
use `codex login --device-auth` when enabled by your account/workspace. Otherwise,
a state-bound loopback redirect can use the relay. For Claude or another CLI,
follow its actual prompt: device approval, a returned code, or loopback OAuth.
WebMux does not rewrite CLI commands or assume every provider uses the same flow.

## Getting the link into WebMux

New Local and qualified SSH shells inherit the existing `BROWSER`/`GH_BROWSER`
and PATH opener helpers. They now request a sign-in panel, not an automatic remote
browser. Printed plain and OSC 8 links use the same panel when clicked. **Sign in**
on each tile accepts a pasted link, including for older tmux shells, Mosh, a
sandbox that strips the browser environment, or an absolute OS opener that bypasses
WebMux. No terminal keybindings or operating-system browser associations change.

Pending helper requests stay on the server until dismissed or five minutes pass,
so a viewer reload can recover the link. A new helper request replaces the previous
pending request for that terminal. Links and callback inputs are held in memory,
not browser storage. Dismissing acknowledges the helper request. Manual pasted and
clicked links are not retained across reloads. A callback ticket is not recovered
after a reload; prepare it again if needed. Completing sign-in remains a CLI action.

## Why paste the final callback URL?

A web page cannot read an unrelated provider tab or capture its localhost navigation
across origins. The same localhost address refers to your viewing device, not the
CLI host. WebMux leaves the provider's registered redirect URI unchanged. If that
tab fails to load localhost, copy its full final address back to the panel. Do not
paste it into a shell or a ticket. A browser extension or installed helper would
be necessary to eliminate this step reliably; neither is required here.

The relay accepts only HTTP callbacks to `localhost`, `127.0.0.1`, or `[::1]`, with
an explicit port from 1024 through 65535, a query-free registered redirect URI,
and a nonempty OAuth state. The pasted URL must match the exact registered host,
port, encoded path and state, and contain one code or one provider error. Other
flows keep the companion/manual-code fallback. An IPv4 listener is required for
`localhost`; IPv6 listeners can use an explicit `[::1]` redirect.

The server issues an opaque, owner-and-session-bound ticket, valid for five minutes.
Only one ticket per terminal is active. It consumes the ticket before delivery;
network failures cannot safely be retried because the CLI might already have
accepted the code. Restart login after a failed delivery. Delete, reconnect and
explicit cancellation invalidate the ticket. Server restart clears all tickets.

Delivery is a single GET to numeric loopback, without DNS, environment proxies,
cookies or authorization headers. Redirects are not followed and response bodies
are discarded. The server returns only delivery status. Codes never appear in
worker command-line arguments, application error messages, or terminal input.
Normal CLI output can still contain sign-in URLs and device codes; existing
terminal transcript logging retains ordinary output when enabled.

Local delivery runs on the WebMux host without Chromium. SSH/Mosh delivery uses a
short-lived `webmux --auth-callback` worker on the configured shell host, private
stdin/stdout, existing SSH keys/configuration and strict host-key checking. Install
a matching WebMux binary there; Chromium is unnecessary. Password-only SSH and
arbitrary exec transports cannot use this relay. A nested SSH session, container
or isolated network namespace is not automatically the configured terminal host:
use device/manual-code authentication or configure access to the actual listener.
The relay does not bypass a coding sandbox's restrictions.

## Evidence and boundaries

Automated browser fixtures prove that a real CLI receives device approval through
polling, and that a callback blocked on the viewer side reaches a real CLI listener
only after explicit relay. Tests also cover helper launch paths, terminal links,
reload recovery/dismissal, wrong state, owner isolation, expiry, replay, unsafe
addresses and redirect/proxy suppression. Companion callback tests remain intact.

These fixtures do not qualify live providers or a real SSH deployment. Provider
policy, sandbox egress, callback listener lifetime and credentials on the CLI host
remain relevant. Real SSH/provider qualification is tracked in
[#110](https://github.com/jordanhubbard/webmux/issues/110).
