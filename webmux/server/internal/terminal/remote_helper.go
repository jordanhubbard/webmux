package terminal

import "strings"

// SSH commands do not read interactive login profiles. Resolve the same helper
// for capability checks, authenticated RPC and attachment, without sourcing user
// profiles (which can consume RPC stdin or print into the protocol stream).
func remoteHelperScript(args ...string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = shellQuote(arg)
	}
	return `PATH="$PATH:$HOME/.local/bin:/opt/homebrew/bin:/usr/local/bin"; export PATH
if command -v webmux >/dev/null 2>&1; then exec webmux ` + strings.Join(quoted, " ") + `; fi
for helper in "$HOME/Applications/WebMux.app/Contents/MacOS/webmux" /Applications/WebMux.app/Contents/MacOS/webmux; do
  if [ -x "$helper" ]; then exec "$helper" ` + strings.Join(quoted, " ") + `; fi
done
exit 127`
}

func remoteHelperCommand(args ...string) string {
	return "sh -c " + shellQuote(remoteHelperScript(args...))
}
