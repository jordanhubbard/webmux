package authhandoff

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func authorization(target, state string) string {
	return "https://identity.example/authorize?" + url.Values{"redirect_uri": {target}, "state": {state}}.Encode()
}
func TestValidation(t *testing.T) {
	auth := authorization("http://127.0.0.1:1455/callback", "secret")
	for _, cb := range []string{
		"http://127.0.0.1:1455/callback?state=secret&code=ok",
		"http://127.0.0.1:1455/callback?state=secret&error=access_denied",
	} {
		if _, err := Validate(Payload{auth, cb}); err != nil {
			t.Fatal(err)
		}
	}
	for _, cb := range []string{
		"http://127.0.0.1:1456/callback?state=secret&code=ok",
		"http://localhost:1455/callback?state=secret&code=ok",
		"http://127.0.0.1:1455/other?state=secret&code=ok",
		"http://127.0.0.1:1455/callback?state=wrong&code=ok",
		"http://127.0.0.1:1455/callback?state=secret&state=secret&code=ok",
		"http://127.0.0.1:1455/callback?state=secret&code=ok&code=bad",
		"http://127.0.0.1:1455/callback?state=secret&code=ok&error=bad",
		"http://127.0.0.1:1455/callback?state=secret",
		"http://user@127.0.0.1:1455/callback?state=secret&code=ok",
		"http://127.0.0.1:1455/callback?state=secret&code=ok#fragment",
	} {
		if _, err := Validate(Payload{auth, cb}); err == nil {
			t.Fatalf("accepted %s", cb)
		}
	}
	for _, target := range []string{"http://localhost/callback", "https://localhost:1455/callback", "http://localhost:22/", "http://127.0.0.2:1455/", "http://localhost.evil:1455/", "http://2130706433:1455/", "http://169.254.169.254:8080/", "http://user@localhost:1455/", "http://localhost:1455/?next=evil"} {
		if _, _, err := Target(authorization(target, "secret")); err == nil {
			t.Fatalf("accepted %s", target)
		}
	}
	for _, raw := range []string{authorization("http://localhost:1455/", ""), auth + "&state=other", auth + "&redirect_uri=http://localhost:1455/", "javascript:alert(1)"} {
		if _, _, err := Target(raw); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}
func TestOwnershipReplayReplacementExpiryAndCancellation(t *testing.T) {
	var m Manager
	auth := authorization("http://localhost:1455/callback", "secret")
	callback := "http://localhost:1455/callback?state=secret&code=ok"
	id, err := m.Begin("owner", "session", auth)
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]string{{"other", "session"}, {"owner", "other"}} {
		if _, err := m.Take(pair[0], pair[1], id, callback); err == nil {
			t.Fatal("foreign consumption")
		}
	}
	if _, err := m.Take("owner", "session", id, callback+"&state=bad"); err == nil {
		t.Fatal("bad callback")
	}
	if _, err := m.Take("owner", "session", id, callback); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Take("owner", "session", id, callback); err == nil {
		t.Fatal("replay")
	}
	old, _ := m.Begin("owner", "session", auth)
	id, _ = m.Begin("owner", "session", auth)
	if _, err := m.Take("owner", "session", old, callback); err == nil {
		t.Fatal("superseded")
	}
	m.End("other", "session")
	p := m.requests["owner\x00session"]
	p.expires = time.Now().Add(-time.Second)
	m.requests["owner\x00session"] = p
	if _, err := m.Take("owner", "session", id, callback); err == nil {
		t.Fatal("expired")
	}
	id, _ = m.Begin("owner", "session", auth)
	m.End("owner", "session")
	if _, err := m.Take("owner", "session", id, callback); err == nil {
		t.Fatal("cancelled")
	}
}
func TestDeliveryDoesNotFollowRedirectOrUseProxyOrReturnBody(t *testing.T) {
	hits := 0
	trap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++; t.Error("followed redirect/proxy") }))
	defer trap.Close()
	t.Setenv("HTTP_PROXY", trap.URL)
	received := ""
	listener := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = r.URL.Query().Get("code")
		if r.Method != "GET" || r.Header.Get("Authorization") != "" {
			t.Error("unexpected request")
		}
		w.Header().Set("Location", trap.URL)
		w.WriteHeader(302)
		fmt.Fprint(w, "sensitive body")
	}))
	defer listener.Close()
	p := Payload{authorization(listener.URL+"/callback", "state"), listener.URL + "/callback?state=state&code=proof"}
	if err := Deliver(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if received != "proof" || hits != 0 {
		t.Fatal("delivery mismatch")
	}
	var output strings.Builder
	if err := Worker(strings.NewReader(`{"authorization":"invalid","callback":"sensitive-code"}`), &output); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "sensitive") {
		t.Fatal("worker leaked input")
	}
}

func TestConcurrentDeliveryConsumesOnlyOnce(t *testing.T) {
	var m Manager
	id, _ := m.Begin("owner", "session", authorization("http://localhost:1455/callback", "secret"))
	var count atomic.Int32
	var workers sync.WaitGroup
	for range 16 {
		workers.Go(func() {
			if _, err := m.Take("owner", "session", id, "http://localhost:1455/callback?state=secret&code=ok"); err == nil {
				count.Add(1)
			}
		})
	}
	workers.Wait()
	if count.Load() != 1 {
		t.Fatalf("callback delivered %d times", count.Load())
	}
}
