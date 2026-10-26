//go:build linux

package main

import (
	"context"
	"os"
	"testing"
	"time"
)

// This opt-in check is run during native Linux qualification. Unit coverage in
// main_test.go remains deterministic and does not depend on multicast routing.
func TestLiveMulticastHostnameResolution(t *testing.T) {
	host := os.Getenv("WEBMUX_TEST_MDNS_HOST")
	if host == "" {
		t.Skip("WEBMUX_TEST_MDNS_HOST is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	addresses, err := lookupMulticastHost(ctx, host)
	if err != nil {
		t.Fatal(err)
	}
	if len(addresses) == 0 {
		t.Fatal("mDNS returned no addresses")
	}
}
