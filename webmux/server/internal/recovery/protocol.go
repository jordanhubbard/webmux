// Package recovery records terminal activity separately from display output.
package recovery

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"unicode"
)

const Prefix = "\x1b]777;webmux-recovery;"
const MaxPacket = 32 << 10

// Event is emitted by the shell/CLI helper on the execution host. It is
// evidence for a recovery preview, never an instruction to execute a command.
type Event struct {
	Mode      string   `json:"mode,omitempty"`
	Endpoint  string   `json:"endpoint,omitempty"`
	Helper    string   `json:"helper,omitempty"`
	Platform  string   `json:"platform,omitempty"`
	Signature string   `json:"signature,omitempty"`
	Kind      string   `json:"kind"`
	Instance  string   `json:"instance,omitempty"`
	Cwd       string   `json:"cwd,omitempty"`
	Command   string   `json:"command,omitempty"`
	CLI       string   `json:"cli,omitempty"`
	Version   string   `json:"version,omitempty"`
	SessionID string   `json:"session_id,omitempty"`
	StateFile string   `json:"state_file,omitempty"`
	Argv      []string `json:"argv,omitempty"`
	Survived  bool     `json:"survived,omitempty"`
	Redacted  bool     `json:"redacted,omitempty"`
	Ready     bool     `json:"ready,omitempty"`
}

func (e Event) Validate() error {
	switch e.Kind {
	case "shell", "command", "complete", "cli", "provider", "control":
	default:
		return errors.New("unknown recovery event")
	}
	for _, value := range []string{e.Mode, e.Endpoint, e.Helper, e.Platform, e.Instance, e.Cwd, e.CLI, e.Version, e.SessionID, e.StateFile} {
		if len(value) > 4096 || strings.ContainsFunc(value, unicode.IsControl) {
			return errors.New("invalid recovery metadata")
		}
	}
	if len(e.Command) > 16384 || strings.ContainsRune(e.Command, 0) || len(e.Argv) > 128 {
		return errors.New("recovery command is too large or invalid")
	}
	for _, arg := range e.Argv {
		if len(arg) > 16384 || strings.ContainsRune(arg, 0) {
			return errors.New("invalid recovery argument")
		}
	}
	return nil
}

func Packet(event Event) (string, error) {
	return SignedPacket(event, os.Getenv("WEBMUX_RECOVERY_KEY"))
}

func SignedPacket(event Event, key string) (string, error) {
	if err := event.Validate(); err != nil {
		return "", err
	}
	if key != "" {
		event.Signature = ""
		data, err := json.Marshal(event)
		if err != nil {
			return "", err
		}
		mac := hmac.New(sha256.New, []byte(key))
		_, _ = mac.Write(data)
		event.Signature = base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	}
	data, err := json.Marshal(event)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(data)
	if len(encoded) > MaxPacket {
		return "", errors.New("recovery event exceeds packet limit")
	}
	return Prefix + encoded + "\a", nil
}

// Decoder strips metadata before logging, scrollback and browser delivery.
// Its pending buffer remains bounded even for a malformed unterminated packet.
// Verify binds metadata to this persisted terminal. Output copied from a log
// cannot masquerade as a fresh shell/CLI observation in another terminal.
func Verify(event Event, key string) bool {
	if key == "" || event.Signature == "" {
		return false
	}
	signature, err := base64.RawURLEncoding.DecodeString(event.Signature)
	if err != nil {
		return false
	}
	event.Signature = ""
	data, err := json.Marshal(event)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write(data)
	return hmac.Equal(signature, mac.Sum(nil))
}

type Decoder struct {
	pending  string
	dropping bool
}

func (d *Decoder) Feed(chunk string) (string, []Event) {
	data := d.pending + chunk
	d.pending = ""
	var visible strings.Builder
	var events []Event
	for data != "" {
		if d.dropping {
			end := strings.IndexByte(data, '\a')
			if end < 0 {
				return visible.String(), events
			}
			d.dropping, data = false, data[end+1:]
			continue
		}
		start := strings.Index(data, Prefix)
		if start < 0 {
			for n := min(len(data), len(Prefix)-1); n > 0; n-- {
				if strings.HasSuffix(data, Prefix[:n]) {
					visible.WriteString(data[:len(data)-n])
					d.pending = data[len(data)-n:]
					return visible.String(), events
				}
			}
			visible.WriteString(data)
			break
		}
		visible.WriteString(data[:start])
		data = data[start+len(Prefix):]
		end := strings.IndexByte(data, '\a')
		if end < 0 {
			if len(data) > MaxPacket {
				d.dropping = true
			} else {
				d.pending = Prefix + data
			}
			break
		}
		if end <= MaxPacket {
			raw, err := base64.RawURLEncoding.DecodeString(data[:end])
			var event Event
			if err == nil && json.Unmarshal(raw, &event) == nil && event.Validate() == nil {
				events = append(events, event)
			}
		}
		data = data[end+1:]
	}
	return visible.String(), events
}
