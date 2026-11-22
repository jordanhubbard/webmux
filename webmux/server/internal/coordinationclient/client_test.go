package coordinationclient

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/jordanhubbard/webmux/server/internal/coordination"
)

func TestLocalClientScopesCapabilityAndRefusesRedirects(t *testing.T) {
	token := "wmc1.fixture.capability"
	arrived := false
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { arrived = true }))
	defer foreign.Close()
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Error("missing scoped authorization")
		}
		if r.URL.Path == "/act" {
			http.Redirect(w, r, foreign.URL, 302)
			return
		}
		_, _ = io.WriteString(w, "{\"contract\":\""+coordination.Contract+"\"}\n")
	}))
	defer local.Close()
	access := Access{Contract: coordination.Contract, Endpoint: local.URL, Capability: token}
	file := filepath.Join(t.TempDir(), "access.json")
	if err := WriteAccess(file, access); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WEBMUX_COORDINATION_FILE", file)
	var out bytes.Buffer
	if err := Run([]string{"view"}, strings.NewReader(""), &out); err != nil || !strings.Contains(out.String(), coordination.Contract) || strings.Contains(out.String(), token) {
		t.Fatal(out.String(), err)
	}
	if err := Run([]string{"act"}, strings.NewReader(`{"request_id":"one-original"}`), &out); err == nil || arrived {
		t.Fatal("capability followed a redirect", err, arrived)
	}
	access.Endpoint = "http://example.test:8080"
	if err := Request(context.Background(), access, "GET", "/view", nil, &out); err == nil {
		t.Fatal("capability sent to a nonlocal endpoint")
	}
}
func TestLegacySessionLocatorAndPrivateGuide(t *testing.T) {
	t.Setenv("WEBMUX_COORDINATION_CACHE_HOME", t.TempDir())
	t.Setenv("WEBMUX_COORDINATION_FILE", "")
	id := "10000000-0000-4000-8000-000000000001"
	key := strings.Repeat("a", 32)
	t.Setenv("WEBMUX_RECOVERY_SESSION", id)
	t.Setenv("WEBMUX_RECOVERY_KEY", key)
	locator, err := Locator(id, key)
	if err != nil {
		t.Fatal(err)
	}
	access := Access{Contract: coordination.Contract, Endpoint: "http://127.0.0.1:12345", Capability: "wmc1.fixture.private", Subject: coordination.Ref{Session: id}, Instructions: []string{"Read the scoped inbox."}}
	if err = WriteAccess(locator, access); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err = Run([]string{"guide"}, strings.NewReader(""), &out); err != nil || strings.Contains(out.String(), access.Capability) {
		t.Fatal("guide exposed its credential", out.String(), err)
	}
	var guide map[string]any
	if json.Unmarshal(out.Bytes(), &guide) != nil || guide["contract"] != coordination.Contract {
		t.Fatal(out.String())
	}
	other, _ := Locator(id, strings.Repeat("b", 32))
	if other == locator {
		t.Fatal("foreign recovery identity shared a locator")
	}
	if _, err = Locator("../other", key); err == nil {
		t.Fatal("locator path traversal accepted")
	}
	if runtime.GOOS != "windows" {
		if err = os.Chmod(locator, 0644); err != nil {
			t.Fatal(err)
		}
		if _, err = Load(); err == nil {
			t.Fatal("public capability file accepted")
		}
	}
}

func TestActRejectsBusyExpiredAndOverlappingAdmissions(t *testing.T) {
	result := `{"status":"busy"}`
	var mu sync.Mutex
	setResult := func(value string) { mu.Lock(); result = value; mu.Unlock() }
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		data := result
		mu.Unlock()
		_, _ = io.WriteString(w, data)
	}))
	defer local.Close()
	file := filepath.Join(t.TempDir(), "access.json")
	if err := WriteAccess(file, Access{Contract: coordination.Contract, Endpoint: local.URL, Capability: "wmc1.fixture.capability"}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WEBMUX_COORDINATION_FILE", file)
	var out bytes.Buffer
	if err := Run([]string{"act"}, strings.NewReader(`{}`), &out); err == nil || !strings.Contains(out.String(), "busy") {
		t.Fatal("busy reservation returned CLI success", err)
	}
	setResult(`{"status":"reserved","expires":"2000-01-01T00:00:00Z"}`)
	out.Reset()
	if err := Run([]string{"act"}, strings.NewReader(`{}`), &out); err == nil {
		t.Fatal("old receipt was treated as a new reservation")
	}
	setResult(`{"status":"claimed","conflicts":[{"id":"existing"}]}`)
	out.Reset()
	if err := Run([]string{"act"}, strings.NewReader(`{}`), &out); err == nil {
		t.Fatal("overlap was silently accepted by the CLI")
	}
	setResult(`{"status":"admitted","state":"admitted"}`)
	out.Reset()
	if err := Run([]string{"act"}, strings.NewReader(`{}`), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"state":"admitted"`) {
		t.Fatal("message admission was relabeled as delivery")
	}
}

func TestAdmissionErrorRequiresCompletePeerClaimContext(t *testing.T) {
	for _, state := range []string{"pending", "unavailable", "stale", "unauthorized", "disabled", "invalid"} {
		result := coordination.Result{Status: "claimed", PeerClaims: []coordination.PeerClaimCheck{{State: state}}}
		if err := AdmissionError(result); err == nil {
			t.Fatal("incomplete claim context reported success", state)
		}
	}
	if err := AdmissionError(coordination.Result{Status: "claimed", PeerClaims: []coordination.PeerClaimCheck{{State: "current"}}}); err != nil {
		t.Fatal(err)
	}
}

func TestAdmissionErrorRefusesHistoricalAndUncertainLease(t *testing.T) {
	for _, status := range []string{"reserved", "renewed"} {
		if err := AdmissionError(coordination.Result{Status: status, Lease: &coordination.LeaseCheck{Current: false}}); err == nil {
			t.Fatal("historical outcome granted work", status)
		}
	}
	if err := AdmissionError(coordination.Result{Status: "reservation-pending"}); err == nil {
		t.Fatal("pending intent granted a lease")
	}
}

func TestAdmissionErrorRefusesUnverifiedLegacyLease(t *testing.T) {
	if err := AdmissionError(coordination.Result{Status: "reserved"}); err == nil {
		t.Fatal("legacy result granted work without current lease evidence")
	}
}
