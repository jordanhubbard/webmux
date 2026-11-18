package usbforward

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type DashboardLink struct {
	Version   int    `json:"version"`
	Server    string `json:"server"`
	ID        string `json:"id"`
	PairCode  string `json:"pair_code"`
	AllowHTTP bool   `json:"allow_http"`
}

func decodeDashboardLink(encoded string) (DashboardLink, error) {
	var link DashboardLink
	if len(encoded) > 4096 {
		return link, errors.New("invalid USB dashboard link")
	}
	b, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || decodeDashboardJSON(b, &link) != nil {
		return link, errors.New("invalid USB dashboard link")
	}
	u, err := url.Parse(link.Server)
	if err != nil || u.Host == "" || u.User != nil || u.Opaque != "" || u.RawQuery != "" || u.Fragment != "" || u.ForceQuery || (u.Path != "" && u.Path != "/") ||
		(u.Scheme != "https" && (u.Scheme != "http" || !link.AllowHTTP)) || link.Version != 1 || len(link.ID) != 32 || len(link.PairCode) != 48 {
		return link, errors.New("invalid USB dashboard link")
	}
	if _, err := hex.DecodeString(link.ID + link.PairCode); err != nil {
		return link, errors.New("invalid USB dashboard link")
	}
	link.Server = strings.TrimSuffix(link.Server, "/")
	return link, nil
}
func decodeDashboardJSON(b []byte, out any) error {
	if len(b) > 8192 {
		return errors.New("oversized USB dashboard response")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return err
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return errors.New("invalid USB dashboard response")
	}
	return nil
}

type dashboardStatusError int

func (e dashboardStatusError) Error() string { return "USB dashboard rejected the request" }

type dashboardReporter struct {
	link       DashboardLink
	credential string
	client     *http.Client
	mu         sync.Mutex
	state      LeaseReport
	changed    chan struct{}
	done       chan struct{}
	cancel     context.CancelFunc
}

func (r *dashboardReporter) call(ctx context.Context, operation string, body, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, r.link.Server+"/api/usb-forwardings/"+r.link.ID+"/"+operation, bytes.NewReader(b))
	if err != nil {
		return errors.New("invalid USB dashboard request")
	}
	request.Header.Set("Content-Type", "application/json")
	if r.credential != "" {
		request.Header.Set("Authorization", "Bearer "+r.credential)
	}
	response, err := r.client.Do(request)
	if err != nil {
		return errors.New("USB dashboard unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return dashboardStatusError(response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 8193))
	if err != nil {
		return errors.New("USB dashboard response incomplete")
	}
	return decodeDashboardJSON(data, out)
}
func registerDashboard(ctx context.Context, encoded string, spec LeaseSpec, cancelForward context.CancelFunc) (*dashboardReporter, error) {
	link, err := decodeDashboardLink(encoded)
	if err != nil {
		return nil, err
	}
	r := &dashboardReporter{link: link, client: &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("USB dashboard redirects are refused") }}, state: LeaseReport{Phase: "starting"}, changed: make(chan struct{}, 1), done: make(chan struct{})}
	var credential LeaseCredential
	if err := r.call(ctx, "claim", struct {
		Code string    `json:"pair_code"`
		Spec LeaseSpec `json:"spec"`
	}{Code: link.PairCode, Spec: spec}, &credential); err != nil {
		return nil, err
	}
	if len(credential.Credential) != 64 || credential.ExpiresAt.IsZero() {
		return nil, errors.New("invalid USB dashboard credential")
	}
	if _, err := hex.DecodeString(credential.Credential); err != nil {
		return nil, errors.New("invalid USB dashboard credential")
	}
	r.credential = credential.Credential
	// Pairing is consumed; no future request uses or logs the one-use code.
	r.link.PairCode = ""
	reporting, stop := context.WithCancel(context.WithoutCancel(ctx))
	r.cancel = stop
	go r.monitor(reporting, cancelForward)
	return r, nil
}
func (r *dashboardReporter) current() LeaseReport { r.mu.Lock(); defer r.mu.Unlock(); return r.state }
func (r *dashboardReporter) Ready(port int) {
	r.mu.Lock()
	r.state = LeaseReport{Phase: "ready", RemotePort: port}
	r.mu.Unlock()
	select {
	case r.changed <- struct{}{}:
	default:
	}
}
func (r *dashboardReporter) monitor(ctx context.Context, cancelForward context.CancelFunc) {
	defer close(r.done)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		var control LeaseControl
		err := r.call(ctx, "report", r.current(), &control)
		var status dashboardStatusError
		if err == nil && control.Stop || errors.As(err, &status) && (status == 400 || status == 401 || status == 403 || status == 404 || status == 409 || status == 410) {
			cancelForward()
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-r.changed:
		}
	}
}
func (r *dashboardReporter) Finish(success, releaseConfirmed bool) error {
	r.cancel()
	<-r.done
	final := r.current()
	final.ReleaseConfirmed = releaseConfirmed
	if success && releaseConfirmed {
		final.Phase = "ended"
	} else if final.RemotePort != 0 && !releaseConfirmed {
		final.Phase = "uncertain"
	} else {
		final.Phase = "failed"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var control LeaseControl
	return r.call(ctx, "report", final, &control)
}
