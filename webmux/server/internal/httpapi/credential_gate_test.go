package httpapi

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCredentialExchangeGateBoundsDistributedAndPerSourceTraffic(t *testing.T) {
	var gate credentialExchangeGate
	now := time.Now()
	var admitted atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if gate.allow(fmt.Sprintf("192.0.%d.%d:1234", i/250, i%250+1), now) {
				admitted.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if admitted.Load() != 128 || len(gate.sources) > 128 {
		t.Fatal("unbounded distributed admission", admitted.Load(), len(gate.sources))
	}
	if !gate.allow("192.0.2.1:1234", now.Add(time.Second)) {
		t.Fatal("window did not recover")
	}
	for i := 1; i < 32; i++ {
		if !gate.allow("192.0.2.1:1234", now.Add(time.Second)) {
			t.Fatal("source prematurely limited")
		}
	}
	if gate.allow("192.0.2.1:4567", now.Add(time.Second)) {
		t.Fatal("source-port changes bypassed source limit")
	}
	if !gate.allow("192.0.2.2:1234", now.Add(time.Second)) {
		t.Fatal("one source exhausted another source")
	}
	if gate.allow("not-an-IP:1234", now.Add(time.Second)) {
		t.Fatal("unparseable source admitted")
	}
}

func TestCredentialExchangeGateCanonicalizesSourceAddresses(t *testing.T) {
	var gate credentialExchangeGate
	now := time.Now()
	for range 32 {
		if !gate.allow("192.0.2.1:1234", now) {
			t.Fatal("source prematurely limited")
		}
	}
	if gate.allow("[::ffff:192.0.2.1]:5678", now) {
		t.Fatal("mapped IPv4 bypassed source limit")
	}
	for range 32 {
		if !gate.allow("[fe80::1%eth0]:1234", now) {
			t.Fatal("kernel IPv6 zone refused")
		}
	}
	if gate.allow("[fe80::1%eth1]:5678", now) {
		t.Fatal("zone spelling bypassed source limit")
	}
	if !gate.allow("[fe80::2%eth0]:1234", now) {
		t.Fatal("independent IPv6 source refused")
	}
}
