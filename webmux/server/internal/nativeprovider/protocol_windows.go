// Package nativeprovider owns a Windows terminal independently of its viewers.
// Configuration and credentials travel only through private stdin/socket frames;
// durable records contain identity and a key digest, never launch environment.
package nativeprovider

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"regexp"
	"strings"
)

const maxFrame = 256 << 10
const maxData = 32 << 10
const historyLimit = 1 << 20

var sessionPattern = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)

type Launch struct {
	Program     string
	Args        []string
	Directory   string
	Environment []string
	Cols, Rows  int
}

type Config struct {
	Root       string
	Session    string
	Key        string
	Executable string
	// DriverArgs is internal helper dispatch, not a shell command. Production
	// uses no prefix; native tests use their test binary's helper entry point.
	DriverArgs []string
	Launch     Launch
}

type Identity struct {
	Session      string `json:"session"`
	Instance     string `json:"instance"`
	Endpoint     string `json:"endpoint"`
	OwnerPID     int    `json:"owner_pid"`
	RootPID      int    `json:"root_pid"`
	BinarySHA256 string `json:"binary_sha256"`
}

type record struct {
	Identity  Identity `json:"identity"`
	KeyDigest string   `json:"key_digest"`
}

type frame struct {
	Protocol int       `json:"protocol,omitempty"`
	Kind     string    `json:"kind"`
	Session  string    `json:"session,omitempty"`
	Instance string    `json:"instance,omitempty"`
	Key      string    `json:"key,omitempty"`
	Identity *Identity `json:"identity,omitempty"`
	Error    string    `json:"error,omitempty"`
	Data     []byte    `json:"data,omitempty"`
	Cols     int       `json:"cols,omitempty"`
	Rows     int       `json:"rows,omitempty"`
	Code     int       `json:"code,omitempty"`
}

func secretDigest(key string) string {
	s := sha256.Sum256([]byte(key))
	return hex.EncodeToString(s[:])
}
func equal(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }
func instance() (string, error) {
	var b [16]byte
	_, err := rand.Read(b[:])
	return hex.EncodeToString(b[:]), err
}
func validSize(cols, rows int) bool { return cols >= 1 && cols <= 500 && rows >= 1 && rows <= 200 }

func validate(c Config) error {
	if !sessionPattern.MatchString(c.Session) || len(c.Key) != 32 {
		return errors.New("invalid native provider identity")
	}
	if _, err := hex.DecodeString(c.Key); err != nil {
		return errors.New("invalid native provider key")
	}
	if !filepath.IsAbs(c.Root) || filepath.Base(c.Root) != c.Session || filepath.Base(filepath.Dir(c.Root)) != "native-providers" {
		return errors.New("invalid native provider directory")
	}
	if !filepath.IsAbs(c.Executable) || !strings.EqualFold(filepath.Ext(c.Executable), ".exe") || !filepath.IsAbs(c.Launch.Program) || !validSize(c.Launch.Cols, c.Launch.Rows) {
		return errors.New("invalid native provider launch")
	}
	for _, value := range append(append(append([]string{c.Root, c.Executable, c.Launch.Program, c.Launch.Directory}, c.DriverArgs...), c.Launch.Args...), c.Launch.Environment...) {
		if strings.ContainsRune(value, 0) {
			return errors.New("invalid native provider launch value")
		}
	}
	return nil
}

func writeFrame(w io.Writer, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(data) > maxFrame {
		return errors.New("native provider frame too large")
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(data)))
	for _, part := range [][]byte{header[:], data} {
		for len(part) > 0 {
			n, err := w.Write(part)
			if err != nil {
				return err
			}
			if n == 0 {
				return io.ErrShortWrite
			}
			part = part[n:]
		}
	}
	return nil
}

func readFrame(r io.Reader, value any) error {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return err
	}
	size := binary.BigEndian.Uint32(header[:])
	if size == 0 || size > maxFrame {
		return errors.New("invalid native provider frame size")
	}
	data := make([]byte, int(size))
	if _, err := io.ReadFull(r, data); err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return errors.New("native provider frame accepts one value")
	}
	return nil
}

// A byte ring bounds unattended output without blocking the running job.
// Its snapshot is a raw tail, not a claim of a complete terminal screen.
type history struct {
	data       []byte
	next, used int
}

func (h *history) add(data []byte) {
	if h.data == nil {
		h.data = make([]byte, historyLimit)
	}
	if len(data) >= len(h.data) {
		data = data[len(data)-len(h.data):]
		copy(h.data, data)
		h.next = 0
		h.used = len(h.data)
		return
	}
	n := copy(h.data[h.next:], data)
	copy(h.data, data[n:])
	h.next = (h.next + len(data)) % len(h.data)
	h.used = min(len(h.data), h.used+len(data))
}
func (h *history) snapshot() []byte {
	if h.used == 0 {
		return nil
	}
	out := make([]byte, h.used)
	start := (h.next - h.used + len(h.data)) % len(h.data)
	n := copy(out, h.data[start:min(len(h.data), start+h.used)])
	copy(out[n:], h.data[:h.used-n])
	return out
}
