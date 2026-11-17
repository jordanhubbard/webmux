package screenstate

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"unicode/utf8"
)

// XtermRendererProfile requires the version-qualified WebMux parser-state
// adapter. It is not a capability of an arbitrary ANSI terminal.
const XtermRendererProfile = "webmux-xterm-state-v1"

// XtermUnderlineRendererProfile additionally restores the independent current
// pen flags. Production negotiation remains on v1 until the complete encoder
// and native attach path qualify this extension.
const XtermUnderlineRendererProfile = "webmux-xterm-state-v2"

// EncodedSnapshot names any renderer extension actually used by Data. Owners
// must negotiate RequiredRenderer before sending these bytes to a viewer.
type EncodedSnapshot struct {
	Data             []byte
	RequiredRenderer string
	RendererToken    string
}

func repeatStatePacket(cluster string) (packet, token string, err error) {
	if cluster == "" || !validRepeatCluster(cluster) {
		return "", "", ErrUnsupported
	}
	return rendererStatePacket("v1", []byte(cluster))
}

// underlineStatePacket combines both flags and the final parser state in one
// one-use packet. An empty cluster explicitly restores a cleared REP state;
// sending two packets would consume the token before the second could apply.
func underlineStatePacket(flags uint8, cluster string) (packet, token string, err error) {
	if flags < 1 || flags > 4 || !validRepeatCluster(cluster) {
		return "", "", ErrUnsupported
	}
	data, err := json.Marshal(struct {
		Underline uint8  `json:"u"`
		Repeat    string `json:"r"`
	}{flags, cluster})
	if err != nil || len(data) > 160 {
		return "", "", ErrUnsupported
	}
	return rendererStatePacket("v2", data)
}

func validRepeatCluster(cluster string) bool {
	if len(cluster) > 64 || !utf8.ValidString(cluster) {
		return false
	}
	for _, r := range cluster {
		if r < 0x20 || r >= 0x7f && r <= 0x9f {
			return false
		}
	}
	return true
}

func rendererStatePacket(version string, data []byte) (packet, token string, err error) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", "", ErrUnsupported
	}
	token = hex.EncodeToString(nonce[:])
	return "\x1b]7777;webmux.replay." + version + ";" + token + ";" + base64.StdEncoding.EncodeToString(data) + "\a", token, nil
}
