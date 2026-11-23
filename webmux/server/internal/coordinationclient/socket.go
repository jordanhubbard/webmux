package coordinationclient

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/jordanhubbard/webmux/server/internal/coordination"
)

// SocketEndpoint is an HTTP authority label, never a network destination.
// Socket access always overrides DialContext, disables proxies and redirects,
// and connects only to the private execution-host Unix socket.
const SocketEndpoint = "http://127.0.0.1"

// PrepareSocket allocates only a private directory, never a listener. SSH owns
// the stream-local listener. A new channel cannot unlink another live channel.
func PrepareSocket(session, key, channel string) (string, error) {
	if _, err := Locator(session, key); err != nil {
		return "", err
	}
	nonce := strings.ReplaceAll(channel, "-", "")
	if len(nonce) != 32 {
		return "", errors.New("invalid coordination channel identity")
	}
	if _, err := hex.DecodeString(nonce); err != nil {
		return "", errors.New("invalid coordination channel identity")
	}
	root, err := cacheRoot()
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write([]byte("webmux-coordination-socket/v1\n" + session + "\n" + channel))
	directory := filepath.Join(root, "webmux", "cc", hex.EncodeToString(mac.Sum(nil)[:12]))
	name := filepath.Join(directory, "s")
	// OpenSSH stream-local syntax and native sockaddr_un limits. Unsupported
	// execution-host paths fail visibly without altering the ordinary SSH shell.
	if !strings.HasPrefix(name, "/") || len(name) > 100 || strings.ContainsAny(name, ":\x00\r\n") {
		return "", errors.New("execution-host coordination socket path is unsupported")
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return "", err
	}
	if err := privateSocketDirectory(name); err != nil {
		return "", err
	}
	return name, nil
}

func privateSocketDirectory(name string) error {
	info, err := os.Lstat(filepath.Dir(name))
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return errors.New("coordination socket directory is not private")
	}
	return nil
}
func validateSocketAccess(access Access) error {
	if access.Endpoint != SocketEndpoint || !filepath.IsAbs(access.Socket) || len(access.Socket) > 100 || strings.ContainsAny(access.Socket, "\x00\r\n") {
		return errors.New("invalid private coordination socket")
	}
	if err := privateSocketDirectory(access.Socket); err != nil {
		return err
	}
	info, err := os.Lstat(access.Socket)
	if err != nil || info.Mode()&os.ModeSocket == 0 || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("coordination socket is unavailable")
	}
	return nil
}

// InstallSocket tests the complete forwarded channel before atomically replacing
// the locator. A refused or misrouted channel leaves existing agents' access
// unchanged. This runs only after recovery RPC authenticates the session key.
func InstallSocket(ctx context.Context, session, key, channel string, access Access) error {
	if access.Contract != coordination.Contract || access.Subject.Session != session || !strings.HasPrefix(access.Capability, "wmc1.") || access.Socket != "" {
		return errors.New("invalid source coordination access")
	}
	if err := validateAccess(access); err != nil {
		return err
	}
	name, err := PrepareSocket(session, key, channel)
	if err != nil {
		return err
	}
	access.Socket, access.Endpoint = name, SocketEndpoint
	var out bytes.Buffer
	if err := Request(ctx, access, http.MethodGet, "/view", nil, &out); err != nil {
		return err
	}
	var view coordination.View
	if json.Unmarshal(out.Bytes(), &view) != nil || view.Contract != coordination.Contract || !view.Enabled || view.Self == nil || view.Self.Ref != access.Subject {
		return errors.New("forwarded coordination identity does not match this session")
	}
	locator, err := Locator(session, key)
	if err != nil {
		return err
	}
	data, err := json.Marshal(access)
	if err != nil || len(data) > 16<<10 {
		return errors.New("coordination access exceeds its limit")
	}
	return WriteAccess(locator, access)
}
