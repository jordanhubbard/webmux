package screenstate

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"unicode/utf8"
)

// XtermRendererProfile requires the version-qualified WebMux parser-state
// adapter. It is not a capability of an arbitrary ANSI terminal.
const XtermRendererProfile = "webmux-xterm-state-v1"

// EncodedSnapshot names any renderer extension actually used by Data. Owners
// must negotiate RequiredRenderer before sending these bytes to a viewer.
type EncodedSnapshot struct {
	Data             []byte
	RequiredRenderer string
	RendererToken    string
}

func repeatStatePacket(cluster string) (packet, token string, err error) {
	if len(cluster) == 0 || len(cluster) > 64 || !utf8.ValidString(cluster) {
		return "", "", ErrUnsupported
	}
	for _, r := range cluster {
		if r < 0x20 || r >= 0x7f && r <= 0x9f {
			return "", "", ErrUnsupported
		}
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", "", ErrUnsupported
	}
	token = hex.EncodeToString(nonce[:])
	return "\x1b]7777;webmux.replay.v1;" + token + ";" + base64.StdEncoding.EncodeToString([]byte(cluster)) + "\a", token, nil
}
