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

// XtermUnderlineRendererProfile restores independent pen flags, logical cursor
// columns and REP state after ANSI painting. Production negotiation remains on
// v1 until native and actual application acceptance qualify this extension.
const XtermUnderlineRendererProfile = "webmux-xterm-state-v2"

// RendererProfileKnown excludes future names until their wire semantics have
// been qualified. An empty name advertises ordinary ANSI only.
func RendererProfileKnown(profile string) bool {
	return profile == "" || profile == XtermRendererProfile || profile == XtermUnderlineRendererProfile
}

// RendererSupports encodes the known backwards compatibility relation, not a
// lexical version comparison. Every viewer must support the required profile.
func RendererSupports(offered, required string) bool {
	if !RendererProfileKnown(offered) || !RendererProfileKnown(required) {
		return false
	}
	return required == "" || offered == required || offered == XtermUnderlineRendererProfile && required == XtermRendererProfile
}

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
type underlineRendererState struct {
	Underline uint8  `json:"u"`
	Repeat    string `json:"r"`
	X         *int   `json:"x,omitempty"`
	NormalX   *int   `json:"n,omitempty"`
	AltX      *int   `json:"a,omitempty"`
}

func underlineStatePacket(state underlineRendererState) (packet, token string, err error) {
	if state.Underline < 1 || state.Underline > 4 || !validRepeatCluster(state.Repeat) {
		return "", "", ErrUnsupported
	}
	for _, column := range []*int{state.X, state.NormalX, state.AltX} {
		if column != nil && (*column < 0 || *column > 500) {
			return "", "", ErrUnsupported
		}
	}
	data, err := json.Marshal(state)
	if err != nil || len(data) > 192 {
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
