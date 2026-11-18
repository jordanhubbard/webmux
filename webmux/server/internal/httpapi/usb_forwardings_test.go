package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/jordanhubbard/webmux/server/internal/usbforward"
)

func TestUSBForwardingHTTPAuthenticationIsolationAndRestart(t *testing.T) {
	s, h := fixture(t, "local")
	alice := responseToken(t, request(h, "POST", "/api/auth/bootstrap", `{"username":"alice","password":"testpass"}`, ""))
	requireStatus(t, request(h, "POST", "/api/auth/register", `{"username":"bob","password":"testpass"}`, alice), 201)
	bob := responseToken(t, request(h, "POST", "/api/auth/login", `{"username":"bob","password":"testpass"}`, ""))
	requireStatus(t, request(h, "GET", "/api/usb-forwardings", "", ""), 401)
	spec := usbforward.LeaseSpec{Host: "operator@receiver", SSHPort: 22, ExportPort: 3240, Seconds: 600}
	encoded, _ := json.Marshal(spec)
	w := request(h, "POST", "/api/usb-forwardings", string(encoded), alice)
	requireStatus(t, w, 201)
	var ticket usbforward.LeaseTicket
	if err := json.Unmarshal(w.Body.Bytes(), &ticket); err != nil {
		t.Fatal(err)
	}
	if ticket.PairCode == "" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("pairing receipt/cache policy missing")
	}
	w = request(h, "GET", "/api/usb-forwardings", "", bob)
	requireStatus(t, w, 200)
	if strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("foreign lease disclosure: %s", w.Body.String())
	}
	path := "/api/usb-forwardings/" + ticket.Lease.ID
	requireStatus(t, request(h, "POST", path+"/stop", "", bob), 404)
	claim, _ := json.Marshal(map[string]any{"pair_code": ticket.PairCode, "spec": spec})
	w = request(h, "POST", path+"/claim", string(claim), "")
	requireStatus(t, w, 200)
	var credential usbforward.LeaseCredential
	if err := json.Unmarshal(w.Body.Bytes(), &credential); err != nil {
		t.Fatal(err)
	}
	requireStatus(t, request(h, "POST", path+"/claim", string(claim), ""), 404)
	requireStatus(t, request(h, "GET", "/api/sessions", "", credential.Credential), 401)
	requireStatus(t, request(h, "POST", path+"/report", `{"phase":"ready","remote_port":23240}`, alice), 404)
	w = request(h, "POST", path+"/report", `{"phase":"ready","remote_port":23240}`, credential.Credential)
	requireStatus(t, w, 200)
	requireStatus(t, request(h, "POST", path+"/stop", "", alice), 200)

	replacement, err := New(s.store, Options{Name: "replacement"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = replacement.Close() })
	w = request(replacement.Handler(), "POST", path+"/report", `{"phase":"ready","remote_port":23240}`, credential.Credential)
	requireStatus(t, w, 200)
	if strings.TrimSpace(w.Body.String()) != `{"stop":true}` {
		t.Fatalf("stop lost on reconstruction: %s", w.Body.String())
	}
	w = request(h, "GET", "/api/usb-forwardings", "", alice)
	requireStatus(t, w, 200)
	if strings.Contains(w.Body.String(), ticket.PairCode) || strings.Contains(w.Body.String(), credential.Credential) || strings.Contains(w.Body.String(), "hash") {
		t.Fatal("credential material in browser list")
	}
}

func TestUSBForwardingHTTPBoundedClaimsAndOwnerRevocation(t *testing.T) {
	_, h := fixture(t, "local")
	alice := responseToken(t, request(h, "POST", "/api/auth/bootstrap", `{"username":"alice","password":"testpass"}`, ""))
	requireStatus(t, request(h, "POST", "/api/auth/register", `{"username":"bob","password":"testpass"}`, alice), 201)
	bob := responseToken(t, request(h, "POST", "/api/auth/login", `{"username":"bob","password":"testpass"}`, ""))
	w := request(h, "POST", "/api/usb-forwardings", `{"host":"receiver","ssh_port":22,"export_port":3240,"seconds":60}`, bob)
	requireStatus(t, w, 201)
	var ticket usbforward.LeaseTicket
	if err := json.Unmarshal(w.Body.Bytes(), &ticket); err != nil {
		t.Fatal(err)
	}
	path := "/api/usb-forwardings/" + ticket.Lease.ID
	requireStatus(t, request(h, "POST", path+"/claim", `{"pair_code":"`+strings.Repeat("x", 8192)+`"}`, ""), http.StatusRequestEntityTooLarge)
	claim, _ := json.Marshal(map[string]any{"pair_code": ticket.PairCode, "spec": ticket.Lease.Spec})
	w = request(h, "POST", path+"/claim", string(claim), "")
	requireStatus(t, w, 200)
	var credential usbforward.LeaseCredential
	if err := json.Unmarshal(w.Body.Bytes(), &credential); err != nil {
		t.Fatal(err)
	}
	requireStatus(t, request(h, "DELETE", "/api/auth/users/bob", "", alice), 204)
	requireStatus(t, request(h, "POST", path+"/report", `{"phase":"starting"}`, credential.Credential), 404)
}
