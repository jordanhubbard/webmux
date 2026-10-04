// Package browseropen carries shell browser requests over the existing PTY.
package browseropen

import (
	"encoding/base64"
	"net/url"
	"strings"
)

const Prefix = "\x1b]777;webmux-browser;"
const MaxURL = 16384
const maxPacket = MaxURL*4/3 + 16

func ValidURL(raw string) bool {
	u, err := url.Parse(raw)
	return len(raw) <= MaxURL && err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" && u.User == nil
}

func Packet(raw string) string {
	return Prefix + base64.RawURLEncoding.EncodeToString([]byte(raw)) + "\a"
}

// Decoder removes requests before terminal output is logged or retained. It
// tolerates arbitrary PTY read boundaries without buffering unlimited output.
type Decoder struct {
	pending  string
	dropping bool
}

func (d *Decoder) Feed(chunk string) (string, []string) {
	data := d.pending + chunk
	d.pending = ""
	var visible strings.Builder
	var urls []string
	for data != "" {
		if d.dropping {
			i := strings.IndexByte(data, '\a')
			if i < 0 {
				return visible.String(), urls
			}
			d.dropping = false
			data = data[i+1:]
			continue
		}
		i := strings.Index(data, Prefix)
		if i < 0 {
			for n := min(len(data), len(Prefix)-1); n > 0; n-- {
				if strings.HasSuffix(data, Prefix[:n]) {
					visible.WriteString(data[:len(data)-n])
					d.pending = data[len(data)-n:]
					return visible.String(), urls
				}
			}
			visible.WriteString(data)
			break
		}
		visible.WriteString(data[:i])
		data = data[i+len(Prefix):]
		end := strings.IndexByte(data, '\a')
		if end < 0 {
			if len(data) > maxPacket {
				d.dropping = true
			} else {
				d.pending = Prefix + data
			}
			break
		}
		if end <= maxPacket {
			decoded, err := base64.RawURLEncoding.DecodeString(data[:end])
			if err == nil && ValidURL(string(decoded)) {
				urls = append(urls, string(decoded))
			}
		}
		data = data[end+1:]
	}
	return visible.String(), urls
}
