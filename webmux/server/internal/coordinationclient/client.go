// Package coordinationclient exposes the scoped coordination interface to
// ordinary sessions. It never reads administrator or federation credentials.
package coordinationclient

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/coordination"
)

const LeaseProtocolHeader = "X-Webmux-Coordination-Lease"

type Access struct {
	Contract     string           `json:"contract"`
	Endpoint     string           `json:"endpoint"`
	Capability   string           `json:"capability"`
	Subject      coordination.Ref `json:"subject"`
	Instructions []string         `json:"instructions"`
}

func Locator(session, key string) (string, error) {
	if len(session) > 128 || session == "" || strings.ContainsAny(session, "/\\.:\x00\r\n") || len(key) < 32 {
		return "", errors.New("session coordination locator is unavailable")
	}
	root := os.Getenv("WEBMUX_COORDINATION_CACHE_HOME")
	if root == "" {
		var err error
		root, err = os.UserCacheDir()
		if err != nil {
			return "", err
		}
	}
	if !filepath.IsAbs(root) {
		return "", errors.New("invalid coordination cache root")
	}
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write([]byte("webmux-coordination-locator/v1"))
	return filepath.Join(root, "webmux", "coordination", session, hex.EncodeToString(mac.Sum(nil))+".json"), nil
}
func WriteAccess(name string, access Access) error {
	if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(access, "", "  ")
	if err != nil {
		return err
	}
	// Avoid replacing an unchanged file on every membership maintenance tick.
	if old, err := os.ReadFile(name); err == nil && string(old) == string(data) {
		return nil
	}
	file, err := os.CreateTemp(filepath.Dir(name), ".coordination-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), name)
}
func AccessPath() (string, error) {
	name := os.Getenv("WEBMUX_COORDINATION_FILE")
	if name != "" {
		if !filepath.IsAbs(name) {
			return "", errors.New("coordination access file must use an absolute location")
		}
		return name, nil
	}
	return Locator(os.Getenv("WEBMUX_RECOVERY_SESSION"), os.Getenv("WEBMUX_RECOVERY_KEY"))
}
func Load() (Access, error) {
	name, err := AccessPath()
	if err != nil {
		return Access{}, err
	}
	info, err := os.Stat(name)
	if err != nil {
		return Access{}, errors.New("coordination interface has not been provisioned for this execution host/session")
	}
	if !info.Mode().IsRegular() || info.Size() > 16<<10 || (runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0) {
		return Access{}, errors.New("coordination access file is not private")
	}
	data, err := os.ReadFile(name)
	if err != nil {
		return Access{}, err
	}
	var access Access
	if json.Unmarshal(data, &access) != nil || access.Contract != coordination.Contract || access.Capability == "" {
		return Access{}, errors.New("invalid coordination access file")
	}
	expected := os.Getenv("WEBMUX_COORDINATION_EXPECTED_SESSION")
	if expected == "" {
		expected = os.Getenv("WEBMUX_RECOVERY_SESSION")
	}
	if expected != "" && access.Subject.Session != expected {
		return Access{}, errors.New("coordination interface belongs to another session")
	}
	if err := validateAccess(access); err != nil {
		return Access{}, err
	}
	return access, nil
}
func Request(ctx context.Context, access Access, method, path string, body io.Reader, out io.Writer) error {
	if err := validateAccess(access); err != nil {
		return err
	}
	route, err := url.Parse(path)
	if err != nil || route.IsAbs() || route.Host != "" || route.Fragment != "" || route.RawPath != "" || (route.Path != "/view" && route.Path != "/act") {
		return errors.New("unknown coordination operation")
	}
	query, err := url.ParseQuery(route.RawQuery)
	if err != nil {
		return errors.New("invalid coordination cursor")
	}
	for key, values := range query {
		if route.Path != "/view" || key != "after" || len(values) != 1 {
			return errors.New("unknown coordination query")
		}
		if _, err := strconv.ParseUint(values[0], 10, 64); err != nil {
			return errors.New("invalid coordination cursor")
		}
	}
	if (route.Path == "/view" && method != "GET") || (route.Path == "/act" && method != "POST") {
		return errors.New("invalid coordination method")
	}
	timeout := 5 * time.Second
	if route.Path == "/act" {
		if body == nil {
			return errors.New("coordination operation requires a JSON body")
		}
		data, err := io.ReadAll(io.LimitReader(body, (16<<10)+1))
		if err != nil || len(data) > 16<<10 {
			return errors.New("coordination operation exceeds 16 KiB")
		}
		var operation struct {
			Action string `json:"action"`
		}
		_ = json.Unmarshal(data, &operation)
		if operation.Action == "reserve" || operation.Action == "renew-reservation" || operation.Action == "release-reservation" {
			timeout = 12 * time.Second
		}
		body = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(access.Endpoint, "/")+path, body)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+access.Capability)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(LeaseProtocolHeader, "1")
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return errors.New("local coordination service is unavailable")
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, (8<<20)+1))
	if err != nil || len(data) > 8<<20 {
		return errors.New("invalid coordination response")
	}
	if response.StatusCode != 200 {
		return errors.New("coordination request was refused; check session activity, global policy and project availability")
	}
	_, err = out.Write(data)
	return err
}
func Run(args []string, in io.Reader, out io.Writer) error {
	access, err := Load()
	if err != nil {
		return err
	}
	if len(args) == 0 || args[0] == "view" {
		path := "/view"
		if len(args) > 1 {
			if len(args) != 3 || args[1] != "--after" {
				return errors.New("usage: coordinate view [--after cursor]")
			}
			cursor, err := strconv.ParseUint(args[2], 10, 64)
			if err != nil {
				return err
			}
			path += "?after=" + strconv.FormatUint(cursor, 10)
		}
		return Request(context.Background(), access, "GET", path, nil, out)
	}
	if args[0] == "guide" {
		data, _ := json.MarshalIndent(struct {
			Contract     string           `json:"contract"`
			Subject      coordination.Ref `json:"subject"`
			Instructions []string         `json:"instructions"`
		}{access.Contract, access.Subject, access.Instructions}, "", "  ")
		_, err = out.Write(append(data, '\n'))
		return err
	}
	if args[0] == "act" {
		body, err := io.ReadAll(io.LimitReader(in, (16<<10)+1))
		if err != nil || len(body) > 16<<10 {
			return errors.New("coordination operation exceeds 16 KiB")
		}
		var response bytes.Buffer
		if err := Request(context.Background(), access, "POST", "/act", strings.NewReader(string(body)), &response); err != nil {
			return err
		}
		if _, err := out.Write(response.Bytes()); err != nil {
			return err
		}
		var result coordination.Result
		if json.Unmarshal(response.Bytes(), &result) != nil {
			return errors.New("invalid coordination operation result")
		}
		return AdmissionError(result)
	}
	return errors.New("usage: webmux coordinate [view|guide|act|mcp]; act reads a JSON request with a stable request_id from stdin")
}

// AdmissionError preserves denial, expiry and advisory-conflict semantics for all clients.
func AdmissionError(result coordination.Result) error {
	switch result.Status {
	case "updated", "claimed", "released", "admitted", "recorded", "reserved", "renewed":
	default:
		return errors.New("coordination operation was not admitted: " + result.Status)
	}
	if (result.Status == "reserved" || result.Status == "renewed") && (result.Lease == nil || !result.Lease.Current || result.Expires == nil || result.Expires.IsZero()) {
		return errors.New("a verified current lease and expiry are required before work; historical or older-controller success is insufficient")
	}
	if result.Lease != nil && !result.Lease.Current {
		return errors.New("reservation is not a current verified lease; do not run work under its original receipt")
	}
	if result.Expires != nil && !time.Now().Before(*result.Expires) {
		return errors.New("original coordination admission has expired; it did not acquire a new lease")
	}
	if len(result.Conflicts) != 0 {
		return errors.New("claim recorded with overlapping ownership; negotiate before editing")
	}
	for _, peer := range result.PeerClaims {
		if peer.State != "current" {
			return errors.New("claim recorded with incomplete remote ownership context; recheck peers before editing")
		}
	}
	return nil
}

func validateAccess(access Access) error {
	endpoint, err := url.Parse(access.Endpoint)
	if err != nil || endpoint.Scheme != "http" || (endpoint.Hostname() != "127.0.0.1" && endpoint.Hostname() != "::1") || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" || (endpoint.Path != "" && endpoint.Path != "/") {
		return errors.New("coordination capability requires the local agent endpoint")
	}
	if port, err := strconv.Atoi(endpoint.Port()); err != nil || port < 1 || port > 65535 {
		return errors.New("invalid coordination endpoint port")
	}
	return nil
}
