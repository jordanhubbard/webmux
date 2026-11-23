package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestProbeThroughAuthenticatedConnect(t *testing.T) {
	for _, test := range []struct {
		name, proxyPassword, credential string
		wantSuccess, wantRevoked        bool
		wantBackendRequests             int64
	}{
		{"authenticated tunnel", "proxy", "fixture", true, false, 1},
		{"proxy refuses credentials", "wrong", "fixture", false, false, 0},
		{"server refuses credentials", "proxy", "wrong", false, true, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			type observation struct {
				authorization, proxyAuthorization string
				snapshot                          bool
				err                               error
			}
			backendSeen := make(chan observation, 1)
			var backendRequests atomic.Int64
			backend := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				backendRequests.Add(1)
				seen := observation{authorization: r.Header.Get("Authorization"), proxyAuthorization: r.Header.Get("Proxy-Authorization")}
				defer func() { backendSeen <- seen }()
				if r.URL.Path != "/api/clients/connect" || seen.authorization != "Bearer fixture" {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}
				var socket *websocket.Conn
				socket, seen.err = (&websocket.Upgrader{}).Upgrade(w, r, nil)
				if seen.err != nil {
					return
				}
				defer socket.Close()
				_ = socket.SetReadDeadline(time.Now().Add(10 * time.Second))
				if seen.err = socket.WriteJSON(frame{Type: "hello", Protocol: protocol}); seen.err != nil {
					return
				}
				var report frame
				seen.err = socket.ReadJSON(&report)
				seen.snapshot = report.Type == "snapshot" && report.Snapshot != nil && report.Snapshot.CPUs > 0
			}))
			defer backend.Close()
			backendURL, err := url.Parse(backend.URL)
			if err != nil {
				t.Fatal(err)
			}

			proxySeen := make(chan observation, 1)
			proxyDone := make(chan error, 1)
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				proxySeen <- observation{authorization: r.Header.Get("Authorization"), proxyAuthorization: r.Header.Get("Proxy-Authorization")}
				var proxyErr error
				defer func() { proxyDone <- proxyErr }()
				if r.Method != http.MethodConnect || r.Host != backendURL.Host {
					http.Error(w, "unexpected target", http.StatusForbidden)
					return
				}
				if r.Header.Get("Proxy-Authorization") != "Basic "+base64.StdEncoding.EncodeToString([]byte("fixture:proxy")) {
					http.Error(w, "proxy authentication required", http.StatusProxyAuthRequired)
					return
				}
				var upstream net.Conn
				upstream, proxyErr = net.DialTimeout("tcp", backendURL.Host, 5*time.Second)
				if proxyErr != nil {
					http.Error(w, "unavailable", http.StatusBadGateway)
					return
				}
				defer upstream.Close()
				client, buffered, hijackErr := w.(http.Hijacker).Hijack()
				if proxyErr = hijackErr; proxyErr != nil {
					return
				}
				defer client.Close()
				_ = client.SetDeadline(time.Now().Add(10 * time.Second))
				_ = upstream.SetDeadline(time.Now().Add(10 * time.Second))
				// CONNECT changes this connection into a tunnel. Returning from an
				// ordinary HTTP handler after WriteHeader(200) is not a tunnel.
				if _, proxyErr = buffered.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); proxyErr != nil {
					return
				}
				if proxyErr = buffered.Flush(); proxyErr != nil {
					return
				}
				done := make(chan struct{})
				go func() {
					_, _ = io.Copy(upstream, buffered)
					_ = upstream.Close()
					close(done)
				}()
				_, _ = io.Copy(client, upstream)
				_ = client.Close()
				<-done
			}))
			defer proxy.Close()
			proxyURL, err := url.Parse(proxy.URL)
			if err != nil {
				t.Fatal(err)
			}
			proxyURL.User = url.UserPassword("fixture", test.proxyPassword)
			roots := x509.NewCertPool()
			roots.AddCert(backend.Certificate())
			previous := websocket.DefaultDialer
			dialer := *previous
			dialer.Proxy = http.ProxyURL(proxyURL)
			dialer.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
			websocket.DefaultDialer = &dialer
			defer func() { websocket.DefaultDialer = previous }()

			err = probeConnection(backend.URL, test.credential)
			if (err == nil) != test.wantSuccess || errors.Is(err, errRevoked) != test.wantRevoked {
				t.Fatalf("unexpected probe result: %v", err)
			}
			select {
			case err := <-proxyDone:
				if err != nil {
					t.Fatal("CONNECT fixture failed", err)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("CONNECT fixture did not finish")
			}
			if seen := <-proxySeen; seen.authorization != "" || seen.proxyAuthorization == "" {
				t.Fatal("CONNECT mixed client and proxy credentials")
			}
			if got := backendRequests.Load(); got != test.wantBackendRequests {
				t.Fatalf("backend requests: got %d, want %d", got, test.wantBackendRequests)
			}
			if test.wantBackendRequests != 0 {
				seen := <-backendSeen
				if seen.err != nil || seen.authorization != "Bearer "+test.credential || seen.proxyAuthorization != "" || seen.snapshot != test.wantSuccess {
					t.Fatal("backend authentication, protocol or credential isolation failed", seen.err)
				}
			}
		})
	}
}
