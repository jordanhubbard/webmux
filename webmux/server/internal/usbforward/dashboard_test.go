package usbforward

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func exerciseDashboardSSH(t *testing.T, binary string, sshPort, exportPort int, keyFile, known string, gate *cleanupGate) {
	t.Helper()
	m, _ := leaseFixture(t)
	spec := LeaseSpec{Host: "dashboard-held-eof@127.0.0.1", SSHPort: sshPort, ExportPort: exportPort, Seconds: 30}
	ticket, err := m.Create("alice", spec)
	if err != nil {
		t.Fatal(err)
	}
	readyReported := make(chan struct{})
	var onceReady sync.Once
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/usb-forwardings/{id}/claim", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Code string    `json:"pair_code"`
			Spec LeaseSpec `json:"spec"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			w.WriteHeader(400)
			return
		}
		current := LeaseManager{Store: m.Store, now: m.now}
		credential, err := current.Claim(r.PathValue("id"), body.Code, body.Spec, allowLeaseOwner)
		if err != nil {
			w.WriteHeader(404)
			return
		}
		_ = json.NewEncoder(w).Encode(credential)
	})
	mux.HandleFunc("POST /api/usb-forwardings/{id}/report", func(w http.ResponseWriter, r *http.Request) {
		var report LeaseReport
		if json.NewDecoder(r.Body).Decode(&report) != nil {
			w.WriteHeader(400)
			return
		}
		current := LeaseManager{Store: m.Store, now: m.now}
		control, err := current.Report(r.PathValue("id"), strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), report, allowLeaseOwner)
		if err != nil {
			w.WriteHeader(404)
			return
		}
		if report.Phase == "ready" {
			onceReady.Do(func() { close(readyReported) })
		}
		_ = json.NewEncoder(w).Encode(control)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	link, _ := json.Marshal(DashboardLink{Version: 1, Server: server.URL, ID: ticket.Lease.ID, PairCode: ticket.PairCode, AllowHTTP: true})
	encoded := base64.RawURLEncoding.EncodeToString(link)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "usb-forward", "--host", spec.Host, "--identity", keyFile, "--known-hosts", known,
		"--ssh-port", strconv.Itoa(sshPort), "--export-port", strconv.Itoa(exportPort), "--duration", "30s", "--dashboard-link", encoded)
	cmd.WaitDelay = time.Second
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	var waitOnce sync.Once
	var waitErr error
	wait := func() error { waitOnce.Do(func() { waitErr = cmd.Wait() }); return waitErr }
	ports := make(chan int, 1)
	readDone := make(chan struct{})
	var diagnostic bytes.Buffer
	go func() {
		defer close(readDone)
		scanner := bufio.NewScanner(out)
		for scanner.Scan() {
			line := scanner.Text()
			diagnostic.WriteString(line + "\n")
			var port int
			if _, err := fmt.Sscanf(line, "USB transport ready on development host 127.0.0.1:%d", &port); err == nil {
				ports <- port
			}
		}
	}()
	defer func() { gate.release(); cancel(); _ = out.Close(); _ = wait(); <-readDone }()
	var port int
	select {
	case port = <-ports:
	case <-readDone:
		_ = wait()
		t.Fatalf("dashboard CLI ended before readiness: %v: %s", waitErr, diagnostic.String())
	case <-ctx.Done():
		t.Fatal("dashboard readiness deadline")
	}
	select {
	case <-readyReported:
	case <-ctx.Done():
		t.Fatal("native helper did not report readiness")
	}
	c := dial(t, port)
	defer c.Close()
	payload := bytes.Repeat([]byte{0, 255, 42, 13, 10}, 15000)
	if _, err := c.Write(payload); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(payload))
	if _, err := io.ReadFull(c, got); err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("monitored SSH bytes: %v", err)
	}
	replacement := LeaseManager{Store: m.Store, now: m.now}
	if _, err := replacement.Stop("alice", ticket.Lease.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-gate.started:
	case <-ctx.Done():
		t.Fatal("dashboard Stop did not reach remote cleanup")
	}
	values, err := replacement.List("alice")
	if err != nil || len(values) != 1 || values[0].Phase != "stopping" || values[0].ReleaseConfirmed {
		t.Fatalf("claimed release before remote worker exit: %+v %v", values, err)
	}
	probe := dial(t, port)
	probe.Close()
	gate.release()
	if err := wait(); err != nil {
		<-readDone
		t.Fatalf("dashboard Stop failed: %v: %s", err, diagnostic.String())
	}
	<-readDone
	values, err = replacement.List("alice")
	if err != nil || values[0].Phase != "ended" || !values[0].ReleaseConfirmed {
		t.Fatalf("missing remote release acknowledgment: %+v %v", values, err)
	}
	if conn, err := net.DialTimeout("tcp4", fmt.Sprintf("127.0.0.1:%d", port), time.Second); err == nil {
		conn.Close()
		t.Fatal("remote listener remains after Stop")
	}
	_ = c.SetReadDeadline(time.Now().Add(time.Second))
	var tail [1]byte
	if _, err := c.Read(tail[:]); err == nil {
		t.Fatal("active forwarded stream remains after Stop")
	}
	if strings.Contains(diagnostic.String(), ticket.PairCode) || strings.Contains(diagnostic.String(), encoded) {
		t.Fatal("dashboard pairing material logged")
	}
}
func TestDashboardLinkRejectsCredentialRedirect(t *testing.T) {
	var mu sync.Mutex
	forwarded := false
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { mu.Lock(); forwarded = true; mu.Unlock() }))
	defer other.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	link, _ := json.Marshal(DashboardLink{Version: 1, Server: source.URL, ID: strings.Repeat("a", 32), PairCode: strings.Repeat("b", 48), AllowHTTP: true})
	_, err := registerDashboard(t.Context(), base64.RawURLEncoding.EncodeToString(link), testLeaseSpec(), func() {})
	if err == nil {
		t.Fatal("credential redirect accepted")
	}
	mu.Lock()
	defer mu.Unlock()
	if forwarded {
		t.Fatal("pairing credential forwarded to redirect target")
	}
}

func TestDashboardReporterRevocationCancelsForwarding(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/usb-forwardings/{id}/claim", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(LeaseCredential{Credential: strings.Repeat("c", 64), ExpiresAt: time.Now().Add(time.Minute)})
	})
	mux.HandleFunc("POST /api/usb-forwardings/{id}/report", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) })
	server := httptest.NewServer(mux)
	defer server.Close()
	link, _ := json.Marshal(DashboardLink{Version: 1, Server: server.URL, ID: strings.Repeat("a", 32), PairCode: strings.Repeat("b", 48), AllowHTTP: true})
	canceled := make(chan struct{})
	var once sync.Once
	reporter, err := registerDashboard(t.Context(), base64.RawURLEncoding.EncodeToString(link), testLeaseSpec(), func() { once.Do(func() { close(canceled) }) })
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reporter.Finish(false, false) }()
	select {
	case <-canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("revoked reporter did not cancel its forwarding context")
	}
}
