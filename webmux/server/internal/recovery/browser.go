package recovery

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"time"
)

const MaxBrowserURL = 16384
const BrowserRequestLifetime = 5 * time.Minute

// BrowserRequest is a sealed, session-bound pending launch, never activity or
// terminal output. The recovery cache contains ciphertext, not an OAuth URL.
type BrowserRequest struct {
	ID         string `json:"id"`
	Expires    int64  `json:"expires"`
	Ciphertext []byte `json:"ciphertext"`
}

func ValidBrowserURL(raw string) bool {
	u, err := url.Parse(raw)
	return len(raw) <= MaxBrowserURL && err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" && u.User == nil
}

func browserCipher(key string) (cipher.AEAD, error) {
	if decoded, err := hex.DecodeString(key); err != nil || len(decoded) != 16 {
		return nil, errors.New("invalid browser control authentication")
	}
	digest := sha256.Sum256([]byte("webmux-browser-control.v1\x00" + key))
	block, err := aes.NewCipher(digest[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func (r BrowserRequest) binding(session string) []byte {
	return []byte(fmt.Sprintf("webmux-browser-control.v1:%s:%s:%d", session, r.ID, r.Expires))
}

// SealBrowserRequest does not log or execute the URL. Authentication binds all
// routing/expiry metadata as well as the contents to this session's secret.
func SealBrowserRequest(session, key, raw string, now time.Time) (*BrowserRequest, error) {
	if !nativeUUID.MatchString(session) || !ValidBrowserURL(raw) {
		return nil, errors.New("invalid browser launch request")
	}
	aead, err := browserCipher(key)
	if err != nil {
		return nil, err
	}
	id, err := NewInstance()
	if err != nil {
		return nil, err
	}
	r := &BrowserRequest{ID: id, Expires: now.Add(BrowserRequestLifetime).UnixMilli()}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	r.Ciphertext = aead.Seal(nonce, nonce, []byte(raw), r.binding(session))
	return r, nil
}

func OpenBrowserRequest(session, key string, r BrowserRequest, now time.Time) (string, error) {
	if !nativeUUID.MatchString(session) || len(r.ID) != 32 || r.Expires <= now.UnixMilli() || r.Expires > now.Add(BrowserRequestLifetime+30*time.Second).UnixMilli() {
		return "", errors.New("expired or invalid browser launch request")
	}
	if id, err := hex.DecodeString(r.ID); err != nil || len(id) != 16 {
		return "", errors.New("invalid browser launch identity")
	}
	aead, err := browserCipher(key)
	if err != nil {
		return "", err
	}
	if len(r.Ciphertext) < aead.NonceSize()+aead.Overhead() || len(r.Ciphertext) > MaxBrowserURL+aead.NonceSize()+aead.Overhead() {
		return "", errors.New("invalid sealed browser launch")
	}
	nonce := r.Ciphertext[:aead.NonceSize()]
	data, err := aead.Open(nil, nonce, r.Ciphertext[aead.NonceSize():], r.binding(session))
	if err != nil || !ValidBrowserURL(string(data)) {
		return "", errors.New("unauthenticated browser launch request")
	}
	return string(data), nil
}

func QueueBrowserRequest(session, key, raw string) error {
	now := time.Now()
	r, err := SealBrowserRequest(session, key, raw, now)
	if err != nil {
		return err
	}
	_, err = updateHostState(session, key, false, func(state *HostState) error {
		if !state.BrowserControl {
			return errors.New("browser control is unavailable; use the printed sign-in link")
		}
		state.BrowserRequest = r
		return nil
	})
	return err
}
