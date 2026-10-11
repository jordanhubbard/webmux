// Package authhandoff delivers a user-supplied OAuth callback to a CLI's loopback
// listener. It never exchanges codes for tokens or returns the listener's body.
package authhandoff

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const Lifetime = 5 * time.Minute

var ErrInvalid = errors.New("Expected an HTTP loopback redirect with an explicit unprivileged port and OAuth state")
var ErrCallback = errors.New("Callback must match this sign-in's loopback address, path and state, with one code or error")
var ErrExpired = errors.New("Sign-in expired, was replaced, or has already been delivered; start again in the terminal")

type Payload struct {
	Authorization string `json:"authorization"`
	Callback      string `json:"callback"`
}
type pending struct {
	id, authorization string
	expires           time.Time
}
type Manager struct {
	mu       sync.Mutex
	requests map[string]pending
}

func parse(raw string) (*url.URL, error) {
	if len(raw) > 16384 || strings.ContainsAny(raw, "\r\n\t") {
		return nil, ErrInvalid
	}
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Fragment != "" || u.Opaque != "" || u.Hostname() == "" {
		return nil, ErrInvalid
	}
	return u, nil
}

func loopback(u *url.URL) bool {
	port, err := strconv.Atoi(u.Port())
	return u.Scheme == "http" && err == nil && port >= 1024 && port <= 65535 &&
		(u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1")
}

func Target(raw string) (*url.URL, string, error) {
	u, err := parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, "", ErrInvalid
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil || len(q["redirect_uri"]) != 1 || len(q["state"]) != 1 || q.Get("state") == "" {
		return nil, "", ErrInvalid
	}
	target, err := parse(q.Get("redirect_uri"))
	if err != nil || !loopback(target) || target.RawQuery != "" {
		return nil, "", ErrInvalid
	}
	return target, q.Get("state"), nil
}

func Validate(p Payload) (*url.URL, error) {
	target, state, err := Target(p.Authorization)
	if err != nil {
		return nil, err
	}
	callback, err := parse(p.Callback)
	if err != nil || callback.Scheme != target.Scheme || callback.Host != target.Host || callback.EscapedPath() != target.EscapedPath() {
		return nil, ErrCallback
	}
	q, err := url.ParseQuery(callback.RawQuery)
	if err != nil || len(q["state"]) != 1 || q.Get("state") != state {
		return nil, ErrCallback
	}
	if !((len(q["code"]) == 1 && q.Get("code") != "" && len(q["error"]) == 0) || (len(q["error"]) == 1 && q.Get("error") != "" && len(q["code"]) == 0)) {
		return nil, ErrCallback
	}
	return callback, nil
}

func (m *Manager) Begin(owner, session, authorization string) (string, error) {
	if _, _, err := Target(authorization); err != nil {
		return "", err
	}
	var bytes [24]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	id := hex.EncodeToString(bytes[:])
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.requests == nil {
		m.requests = make(map[string]pending)
	}
	for key, p := range m.requests {
		if time.Now().After(p.expires) {
			delete(m.requests, key)
		}
	}
	key := owner + "\x00" + session
	if len(m.requests) >= 256 {
		if _, ok := m.requests[key]; !ok {
			return "", errors.New("Too many pending sign-ins")
		}
	}
	m.requests[key] = pending{id, authorization, time.Now().Add(Lifetime)}
	return id, nil
}

// Take consumes a valid callback before I/O so retries cannot replay a code.
func (m *Manager) Take(owner, session, id, callback string) (Payload, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := owner + "\x00" + session
	p, ok := m.requests[key]
	if !ok || p.id != id || time.Now().After(p.expires) {
		return Payload{}, ErrExpired
	}
	payload := Payload{p.authorization, callback}
	if _, err := Validate(payload); err != nil {
		return Payload{}, err
	}
	delete(m.requests, key)
	return payload, nil
}

func (m *Manager) End(owner, session string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.requests, owner+"\x00"+session)
}

// Cancel only the caller's ticket. An older viewer must not cancel a newer login.
func (m *Manager) Cancel(owner, session, id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := owner + "\x00" + session
	if p, ok := m.requests[key]; ok && p.id == id {
		delete(m.requests, key)
	}
}

func Deliver(ctx context.Context, p Payload) error {
	u, err := Validate(p)
	if err != nil {
		return err
	}
	// Bypass DNS and environment proxies. Only numeric loopback is ever dialed.
	host := "127.0.0.1"
	if u.Hostname() == "::1" {
		host = "::1"
	}
	transport := &http.Transport{DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort(host, u.Port()))
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return ErrCallback
	}
	response, err := client.Do(req)
	if err != nil {
		return errors.New("Cannot reach the CLI callback listener; restart sign-in in the terminal")
	}
	response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 400 {
		return errors.New("CLI callback rejected the request; check the terminal and restart sign-in")
	}
	return nil
}

// Worker runs over private stdio, including SSH. Never print callback URLs/errors
// from HTTP libraries, which can include authorization codes.
func Worker(in io.Reader, out io.Writer) error {
	var p Payload
	err := json.NewDecoder(io.LimitReader(in, 65536)).Decode(&p)
	if err == nil {
		err = Deliver(context.Background(), p)
	}
	result := struct {
		Error string `json:"error,omitempty"`
	}{}
	if err != nil {
		result.Error = "Callback delivery failed; check the CLI listener and restart sign-in"
	}
	return json.NewEncoder(out).Encode(result)
}
