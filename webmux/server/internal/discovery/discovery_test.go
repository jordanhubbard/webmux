package discovery

import (
	"context"
	"net"
	"os"
	"strings"
	"testing"
	"time"
)

func TestChooseAdvertisedPortAndSecurity(t *testing.T) {
	secure := &Endpoint{Scheme: "https", Port: 19443}
	clear := &Endpoint{Scheme: "http", Port: 19080}
	for _, test := range []struct {
		name        string
		https, http *Endpoint
		allowHTTP   bool
		want        Endpoint
		wantError   string
	}{
		{"secure by default", secure, clear, false, *secure, ""},
		{"explicit cleartext", secure, clear, true, *clear, ""},
		{"secure only", secure, nil, true, *secure, ""},
		{"cleartext rejected", nil, clear, false, Endpoint{}, "allow-insecure-http"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := choose(test.https, test.http, test.allowHTTP)
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Fatalf("error = %v, want %q", err, test.wantError)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("got %+v, %v; want %+v", got, err, test.want)
			}
		})
	}
}

func TestIPLiteralUsesDocumentedPort(t *testing.T) {
	for _, test := range []struct {
		host      string
		allowHTTP bool
		want      string
	}{
		{"127.0.0.1", false, "https://127.0.0.1:8443"},
		{"[::1]", true, "http://[::1]:8080"},
	} {
		got, err := Resolve(context.Background(), test.host, test.allowHTTP)
		if err != nil || got != test.want {
			t.Fatalf("Resolve(%q) = %q, %v; want %q", test.host, got, err, test.want)
		}
	}
}

func TestDNSRecordsSelectConfiguredPort(t *testing.T) {
	lookup := func(_ context.Context, service, proto, name string) (string, []*net.SRV, error) {
		if proto != "tcp" || name != "builder.example" {
			t.Fatalf("unexpected SRV question %s %s %s", service, proto, name)
		}
		port := uint16(19443)
		if service == "webmux" {
			port = 19080
		}
		return "", []*net.SRV{{Target: "builder.example.", Port: port}}, nil
	}
	for _, test := range []struct {
		allowHTTP bool
		want      Endpoint
	}{
		{false, Endpoint{Scheme: "https", Port: 19443}},
		{true, Endpoint{Scheme: "http", Port: 19080}},
	} {
		got, err := resolveDNSWithLookup(context.Background(), "builder.example", test.allowHTTP, lookup)
		if err != nil || got != test.want {
			t.Fatalf("DNS selection = %+v, %v; want %+v", got, err, test.want)
		}
	}
}

func TestMulticastAdvertisementRoundTrip(t *testing.T) {
	if os.Getenv("WEBMUX_TEST_MDNS") == "" {
		t.Skip("requires a multicast-capable network; set WEBMUX_TEST_MDNS=1")
	}
	server, err := Advertise("webmux-discovery-fixture", 19080, 19443, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		allowHTTP bool
		want      string
	}{
		{false, "https://webmux-discovery-fixture.local:19443"},
		{true, "http://webmux-discovery-fixture.local:19080"},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		got, err := Resolve(ctx, "webmux-discovery-fixture.local", test.allowHTTP)
		cancel()
		if err != nil || got != test.want {
			t.Fatalf("Resolve(allowHTTP=%t) = %q, %v; want %q", test.allowHTTP, got, err, test.want)
		}
	}
	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	server, err = Advertise("webmux-discovery-fixture", 19080, 29443, true)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	got, err := Resolve(ctx, "webmux-discovery-fixture.local", false)
	cancel()
	if err != nil || got != "https://webmux-discovery-fixture.local:29443" {
		t.Fatalf("changed port = %q, %v", got, err)
	}
	clearServer, err := Advertise("webmux-clear-fixture", 19081, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	defer clearServer.Close()
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	got, err = Resolve(ctx, "webmux-clear-fixture.local", true)
	if err != nil || got != "http://webmux-clear-fixture.local:19081" {
		t.Fatalf("cleartext service = %q, %v", got, err)
	}
}
