// Package guacamole implements bounded, Unicode-aware Guacamole framing.
package guacamole

import (
	"bufio"
	"errors"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

const MaxInstructionBytes = 1 << 20

var ErrProtocol = errors.New("invalid Guacamole instruction")
var ErrRemote = errors.New("guacd error")

type Instruction struct {
	Opcode string
	Args   []string
	Raw    string
}
type Decoder struct{ reader *bufio.Reader }

func NewDecoder(reader io.Reader) *Decoder {
	return &Decoder{reader: bufio.NewReaderSize(reader, 32768)}
}
func Encode(opcode string, args ...string) string {
	values := append([]string{opcode}, args...)
	var result strings.Builder
	for i, value := range values {
		if i > 0 {
			result.WriteByte(',')
		}
		result.WriteString(strconv.Itoa(utf8.RuneCountInString(value)))
		result.WriteByte('.')
		result.WriteString(value)
	}
	result.WriteByte(';')
	return result.String()
}

// Lengths count Unicode characters, not UTF-8 bytes or UTF-16 code units. Read
// each declared value before interpreting separators, which may occur in data.
func (d *Decoder) Read() (Instruction, error) {
	var raw strings.Builder
	values := []string{}
	readByte := func() (byte, error) {
		v, err := d.reader.ReadByte()
		if err == io.EOF && raw.Len() > 0 {
			err = io.ErrUnexpectedEOF
		}
		if err != nil {
			return 0, err
		}
		raw.WriteByte(v)
		if raw.Len() > MaxInstructionBytes {
			return 0, ErrProtocol
		}
		return v, nil
	}
	for len(values) < 1024 {
		length, digits := 0, 0
		for {
			ch, err := readByte()
			if err != nil {
				return Instruction{}, err
			}
			if ch == '.' {
				if digits == 0 {
					return Instruction{}, ErrProtocol
				}
				break
			}
			if ch < '0' || ch > '9' || digits >= 7 {
				return Instruction{}, ErrProtocol
			}
			digits++
			length = length*10 + int(ch-'0')
			if length > MaxInstructionBytes {
				return Instruction{}, ErrProtocol
			}
		}
		var value strings.Builder
		for range length {
			r, size, err := d.reader.ReadRune()
			if err == io.EOF {
				err = io.ErrUnexpectedEOF
			}
			if err != nil {
				return Instruction{}, err
			}
			if r == utf8.RuneError && size == 1 {
				return Instruction{}, ErrProtocol
			}
			raw.WriteRune(r)
			if raw.Len() > MaxInstructionBytes {
				return Instruction{}, ErrProtocol
			}
			value.WriteRune(r)
		}
		values = append(values, value.String())
		separator, err := readByte()
		if err != nil {
			return Instruction{}, err
		}
		if separator == ';' {
			if values[0] == "" {
				return Instruction{}, ErrProtocol
			}
			return Instruction{Opcode: values[0], Args: values[1:], Raw: raw.String()}, nil
		}
		if separator != ',' {
			return Instruction{}, ErrProtocol
		}
	}
	return Instruction{}, ErrProtocol
}

type Parameters struct {
	Hostname                   string
	Port                       int
	Username, Password, Domain string
}

func (p Parameters) Value(name string) string {
	switch name {
	case "hostname":
		return p.Hostname
	case "port":
		return strconv.Itoa(p.Port)
	case "username":
		return p.Username
	case "password":
		return p.Password
	case "domain":
		return p.Domain
	}
	return map[string]string{
		"width": "1024", "height": "768", "dpi": "96", "color-depth": "24", "security": "any", "ignore-cert": "true", "client-name": "webmux", "resize-method": "display-update", "enable-font-smoothing": "true", "enable-desktop-composition": "true", "enable-full-window-drag": "false", "enable-menu-animations": "false", "disable-bitmap-caching": "false", "disable-offscreen-caching": "false", "disable-glyph-caching": "false", "disable-audio": "false", "enable-drive": "false", "enable-printing": "false", "sftp-enable": "false", "wol-send-packet": "false", "normalize-clipboard": "preserve", "enable-touch": "false", "recording-exclude-output": "false", "recording-exclude-mouse": "false", "recording-include-keys": "false", "create-recording-path": "false", "create-drive-path": "false",
	}[name]
}

// Handshake preserves the existing server's parameter defaults and version
// fallback. The caller supplies connection read/write deadlines.
func Handshake(d *Decoder, w io.Writer, p Parameters) (Instruction, error) {
	if _, err := io.WriteString(w, Encode("select", "rdp")); err != nil {
		return Instruction{}, err
	}
	connected := false
	for range 256 {
		instruction, err := d.Read()
		if err != nil {
			return Instruction{}, err
		}
		switch instruction.Opcode {
		case "args":
			if connected {
				return Instruction{}, ErrProtocol
			}
			values := make([]string, len(instruction.Args))
			for i, name := range instruction.Args {
				values[i] = p.Value(name)
			}
			if _, err := io.WriteString(w, Encode("connect", values...)); err != nil {
				return Instruction{}, err
			}
			connected = true
		case "ready":
			if !connected {
				return Instruction{}, ErrProtocol
			}
			return instruction, nil
		case "error":
			return Instruction{}, ErrRemote
		}
	}
	return Instruction{}, ErrProtocol
}
