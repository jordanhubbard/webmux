// credential-hub-fixture is a finite, explicitly synthetic qualification server.
// It embeds the production HTTP API; its fake provider is absent from WebMux.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/auth"
	"github.com/jordanhubbard/webmux/server/internal/discovery"
	"github.com/jordanhubbard/webmux/server/internal/httpapi"
	"github.com/jordanhubbard/webmux/server/internal/storage"
)

type config struct {
	SessionSecret   string   `json:"session_secret"`
	AccountPassword string   `json:"account_password"`
	TestOnly        bool     `json:"test_only"`
	Home            string   `json:"home"`
	Defaults        string   `json:"defaults"`
	Web             string   `json:"web"`
	Listen          string   `json:"listen"`
	Certificate     string   `json:"certificate"`
	Key             string   `json:"key"`
	CA              string   `json:"ca"`
	NodeID          string   `json:"node_id"`
	ProviderURL     string   `json:"provider_url"`
	FakeKeys        []string `json:"fake_keys"`
	ReadyFile       string   `json:"ready_file"`
}
type providerTransport struct {
	target    *url.URL
	transport *http.Transport
}

func (p providerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Scheme != "https" || r.URL.Path != "/v1/chat/completions" || (r.URL.Host != "api.openai.com" && r.URL.Host != "integrate.api.nvidia.com") {
		return nil, errors.New("fixture refuses unexpected provider destination")
	}
	next := r.Clone(r.Context())
	u := *p.target
	u.Path = "/fixture/provider"
	u.RawQuery = ""
	u.Fragment = ""
	next.URL = &u
	next.Host = u.Host
	return p.transport.RoundTrip(next)
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "credential fixture failed:", e)
		os.Exit(1)
	}
}
func run() error {
	// Bound even failures before readiness; qualification never leaves a daemon.
	watchdog := time.AfterFunc(16*time.Minute, func() { os.Exit(124) })
	defer watchdog.Stop()
	filename := flag.String("config", "", "explicit private synthetic fixture config")
	initialize := flag.String("initialize", "", "create a NEW private synthetic fixture root, without starting servers")
	defaults := flag.String("defaults", "", "public source configuration defaults")
	web := flag.String("web", "", "built public frontend assets")
	hosts := flag.String("hosts", "hub=127.0.0.1,receiver=127.0.0.1", "two or three fixture role=IP pairs")
	flag.Parse()
	if *initialize != "" {
		return initializeFixtures(*initialize, *defaults, *web, *hosts)
	}
	if *filename == "" {
		return errors.New("--config required")
	}
	b, e := os.ReadFile(*filename)
	if e != nil {
		return e
	}
	var c config
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if dec.Decode(&c) != nil || dec.Decode(new(any)) != io.EOF || !c.TestOnly {
		return errors.New("invalid synthetic fixture config")
	}
	if _, e := os.Stat(filepath.Join(c.Web, "index.html")); e != nil {
		return errors.New("built public web/index.html assets required")
	}
	if _, e := os.Stat(filepath.Join(c.Defaults, "auth.yaml")); e != nil {
		return errors.New("public configuration defaults required")
	}
	marker, e := os.ReadFile(filepath.Join(c.Home, ".webmux-credential-fixture"))
	if e != nil || string(marker) != "webmux-credential-qualification-v1\n" || !filepath.IsAbs(c.Home) || !strings.HasPrefix(filepath.Base(c.Home), "wmx-cred-") {
		return errors.New("refusing an unmarked home")
	}
	if !discovery.ValidNodeID(c.NodeID) || len(c.FakeKeys) != 2 {
		return errors.New("fixture identities required")
	}
	for _, key := range c.FakeKeys {
		if !strings.HasPrefix(key, "wmx-canary-") || len(key) < 32 {
			return errors.New("only dedicated synthetic provider keys permitted")
		}
	}
	if !strings.HasPrefix(c.SessionSecret, "wmx-canary-") || len(c.SessionSecret) < 64 || !strings.HasPrefix(c.AccountPassword, "wmx-canary-") || len(c.AccountPassword) < 64 {
		return errors.New("independent private fixture authentication required")
	}
	listener, e := net.Listen("tcp", c.Listen)
	if e != nil {
		return e
	}
	defer listener.Close()
	if c.ProviderURL == "" {
		c.ProviderURL = "https://" + listener.Addr().String()
	}
	ca, e := os.ReadFile(c.CA)
	if e != nil {
		return e
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		return errors.New("invalid private fixture CA")
	}
	target, e := url.Parse(c.ProviderURL)
	if e != nil || target.Scheme != "https" || target.User != nil || target.RawQuery != "" || target.Fragment != "" {
		return errors.New("private HTTPS provider target required")
	}
	tr := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}, Proxy: nil}
	client := &http.Client{Transport: providerTransport{target, tr}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	store, e := storage.Open(c.Home, c.Defaults)
	if e != nil {
		return e
	}
	var authConfig auth.Config
	if e = store.ReadConfig("auth.yaml", &authConfig); e != nil {
		return e
	}
	authConfig.Auth.Mode = "local"
	if len(authConfig.Auth.Users) == 0 {
		hash, e := auth.HashPassword(c.AccountPassword)
		if e != nil {
			return e
		}
		authConfig.Auth.Users = []auth.User{{Username: "fixture", PasswordHash: hash, Admin: true}}
	}
	if e = store.WriteConfig("auth.yaml", authConfig); e != nil {
		return e
	}
	// Restart preserves accounts, vault, selection and identity. It never reseeds a vault.
	policy := discovery.NodePolicy{Version: 1, ID: c.NodeID, Interfaces: []string{}}
	if e = store.WriteConfig("node-discovery.yaml", policy); e != nil {
		return e
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	api, e := httpapi.New(store, httpapi.Options{Name: "credential-qualification", JWTSecret: c.SessionSecret, WebDir: c.Web, AIEnvironment: func(k string) string {
		if k == "NVIDIA_API_KEY" {
			return c.FakeKeys[1]
		}
		return ""
	}, AIHTTPClient: client, PeerRoots: roots, Logger: logger})
	if e != nil {
		return e
	}
	defer api.Close()
	defer tr.CloseIdleConnections()
	handler := api.Handler()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /fixture/provider", func(w http.ResponseWriter, r *http.Request) {
		for i, key := range c.FakeKeys {
			if r.Header.Get("Authorization") == "Bearer "+key {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": fmt.Sprintf("CREDENTIAL_CANARY_PROVIDER_%d", i+1)}}}})
				return
			}
		}
		w.WriteHeader(401)
	})
	mux.Handle("/", handler)

	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, MaxHeaderBytes: 32 << 10}
	done := make(chan error, 1)
	go func() { done <- srv.ServeTLS(listener, c.Certificate, c.Key) }()
	ready := map[string]any{"ready": true, "address": listener.Addr().String(), "pid": os.Getpid(), "os": runtime.GOOS, "arch": runtime.GOARCH, "synthetic_provider": true}
	if build, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range build.Settings {
			switch setting.Key {
			case "vcs.revision":
				ready["build_source_head"] = setting.Value
			case "vcs.modified":
				ready["build_source_modified"] = setting.Value
			}
		}
	}
	data, _ := json.Marshal(ready)
	if e = os.WriteFile(c.ReadyFile, append(data, '\n'), 0600); e != nil {
		return e
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	timer := time.NewTimer(15 * time.Minute)
	defer timer.Stop()
	select {
	case e = <-done:
		if !errors.Is(e, http.ErrServerClosed) {
			return e
		}
	case <-ctx.Done():
	case <-timer.C:
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdown)
}
