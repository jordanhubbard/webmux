package maccoord

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const accessToken = "fixture-only-read-token-private-canary"

func accessBindings(t *testing.T, origin string) *Bindings {
	t.Helper()
	document := bindingFixture()
	document.Authorities[0].Origin = origin
	bindings, err := ParseBindings(bindingBytes(t, document))
	if err != nil {
		t.Fatal(err)
	}
	return bindings
}

func graphResponse(w http.ResponseWriter) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"schema": GraphSchema, "project": "alpha",
		// These are deliberately not evidence that a project exists or the
		// requester may receive arbitrary raw graph or related-object fields.
		"found": true, "graph": map[string]any{"foreign_task": "not-for-the-receiving-account"},
	})
}

func TestReadAccessRequiresNegativeControlsAndRechecksRevocation(t *testing.T) {
	var allowed atomic.Bool
	allowed.Store(true)
	var mu sync.Mutex
	var credentials []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/dashboard/observe/projects/alpha/graph" || r.URL.RawQuery != "" {
			t.Error("unexpected operation or route")
		}
		credential := r.Header.Get("Authorization")
		mu.Lock()
		credentials = append(credentials, credential)
		mu.Unlock()
		if credential != "Bearer "+accessToken || !allowed.Load() {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		graphResponse(w)
	}))
	defer server.Close()
	bindings := accessBindings(t, server.URL)
	evidence, err := bindings.CheckReadAccess(t.Context(), "alice", repository(), accessToken, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !evidence.MissingCredentialsRefused || !evidence.InvalidCredentialsRefused ||
		!evidence.SuppliedCredentialAccepted || evidence.PrincipalIdentity != "unverified" ||
		evidence.OtherOperations != "unqualified" || evidence.BindingRevision == "" || evidence.ObservedAt.IsZero() {
		t.Fatalf("incorrect evidence boundaries: %+v", evidence)
	}
	encoded, _ := json.Marshal(evidence)
	for _, forbidden := range []string{accessToken, "not-for-the-receiving-account", server.URL} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatal("credential, endpoint or unprojected graph escaped into evidence")
		}
	}
	mu.Lock()
	first := append([]string(nil), credentials...)
	mu.Unlock()
	if len(first) != 3 || first[0] != "" || first[1] == first[2] ||
		!strings.HasPrefix(first[1], "Bearer webmux-denied-") || first[2] != "Bearer "+accessToken {
		t.Fatal("missing ordered no-token/invalid-token/intended-token controls")
	}
	// No cached positive result may authorize another observation.
	allowed.Store(false)
	next, err := bindings.CheckReadAccess(t.Context(), "alice", repository(), accessToken, nil)
	if !errors.Is(err, ErrReadAccess) || next.Schema != "" {
		t.Fatal("revoked credential reused earlier successful evidence:", next, err)
	}
}

func TestReadAccessRefusesDevelopmentModeAndUnmappedCaller(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		graphResponse(w) // Same HTTP200 behavior as MAC with no token map.
	}))
	defer server.Close()
	bindings := accessBindings(t, server.URL)
	if got, err := bindings.CheckReadAccess(t.Context(), "bob", repository(), accessToken, nil); !errors.Is(err, ErrUnmapped) || got.Schema != "" || requests.Load() != 0 {
		t.Fatal("foreign account reached an upstream authority")
	}
	if _, err := bindings.CheckReadAccess(t.Context(), "alice", repository(), "", nil); !errors.Is(err, ErrInvalid) || requests.Load() != 0 {
		t.Fatal("missing intended token reached an upstream authority")
	}
	if got, err := bindings.CheckReadAccess(t.Context(), "alice", repository(), accessToken, nil); !errors.Is(err, ErrReadAccess) || got.Schema != "" || requests.Load() != 1 {
		t.Fatal("development mode was accepted as authenticated access")
	}
}

func TestReadAccessRejectsPartialNegativeControls(t *testing.T) {
	for _, mode := range []string{"anonymous", "arbitrary-bearer"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				credential := r.Header.Get("Authorization")
				accepted := credential == "Bearer "+accessToken ||
					(mode == "anonymous" && credential == "") ||
					(mode == "arbitrary-bearer" && credential != "")
				if !accepted {
					w.WriteHeader(403)
					return
				}
				graphResponse(w)
			}))
			defer server.Close()
			evidence, err := accessBindings(t, server.URL).CheckReadAccess(t.Context(), "alice", repository(), accessToken, nil)
			if !errors.Is(err, ErrReadAccess) || evidence.Schema != "" {
				t.Fatal("partial authentication controls produced successful evidence")
			}
		})
	}
}

func TestReadAccessRejectsRedirectsAndUnqualifiedResponses(t *testing.T) {
	var sinkRequests atomic.Int32
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sinkRequests.Add(1)
	}))
	defer sink.Close()
	for _, test := range []struct {
		name     string
		positive func(http.ResponseWriter)
		want     error
	}{
		{"redirect", func(w http.ResponseWriter) { w.Header().Set("Location", sink.URL); w.WriteHeader(302) }, ErrUnavailable},
		{"missing route", func(w http.ResponseWriter) { w.WriteHeader(404) }, ErrUnsupported},
		{"wrong project", func(w http.ResponseWriter) { _, _ = fmt.Fprintf(w, `{"schema":%q,"project":"beta"}`, GraphSchema) }, ErrInvalid},
		{"future schema", func(w http.ResponseWriter) { _, _ = fmt.Fprint(w, `{"schema":"future","project":"alpha"}`) }, ErrUnsupported},
		{"case alias cannot replace schema", func(w http.ResponseWriter) {
			_, _ = fmt.Fprintf(w, `{"schema":"future","Schema":%q,"project":"alpha"}`, GraphSchema)
		}, ErrUnsupported},
		{"uppercase project", func(w http.ResponseWriter) {
			_, _ = fmt.Fprintf(w, `{"schema":%q,"Project":"alpha"}`, GraphSchema)
		}, ErrInvalid},
		{"duplicate project", func(w http.ResponseWriter) {
			_, _ = fmt.Fprintf(w, `{"schema":%q,"project":"beta","project":"alpha"}`, GraphSchema)
		}, ErrInvalid},
		{"oversized", func(w http.ResponseWriter) {
			_, _ = fmt.Fprintf(w, `{"schema":%q,"project":"alpha","ignored":%q}`, GraphSchema, strings.Repeat("x", 1<<20))
		}, ErrInvalid},
		{"body must not become error", func(w http.ResponseWriter) { w.WriteHeader(500); _, _ = fmt.Fprint(w, accessToken) }, ErrUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer "+accessToken {
					w.WriteHeader(403)
					return
				}
				test.positive(w)
			}))
			defer server.Close()
			got, err := accessBindings(t, server.URL).CheckReadAccess(t.Context(), "alice", repository(), accessToken, nil)
			if !errors.Is(err, test.want) || got.Schema != "" || strings.Contains(err.Error(), accessToken) || strings.Contains(err.Error(), server.URL) {
				t.Fatalf("unqualified response escaped boundary: %+v, %v", got, err)
			}
		})
	}
	if sinkRequests.Load() != 0 {
		t.Fatal("a redirect received an upstream request")
	}
}

func TestReadAccessRequiresTLSTrustAndPreservesCancellation(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+accessToken {
			w.WriteHeader(403)
			return
		}
		graphResponse(w)
	}))
	defer server.Close()
	bindings := accessBindings(t, server.URL)
	if got, err := bindings.CheckReadAccess(t.Context(), "alice", repository(), accessToken, nil); !errors.Is(err, ErrUnavailable) || got.Schema != "" {
		t.Fatal("untrusted TLS authority accepted")
	}
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	if _, err := bindings.CheckReadAccess(t.Context(), "alice", repository(), accessToken, roots); err != nil {
		t.Fatal("explicitly trusted authority refused:", err)
	}
	reached := make(chan struct{})
	stalled := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(reached)
		<-r.Context().Done()
	}))
	defer stalled.Close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := accessBindings(t, stalled.URL).CheckReadAccess(ctx, "alice", repository(), accessToken, nil)
		done <- err
	}()
	select {
	case <-reached:
	case <-time.After(3 * time.Second):
		t.Fatal("owned request did not reach its fixture")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) || !errors.Is(err, ErrUnavailable) || strings.Contains(err.Error(), stalled.URL) {
			t.Fatal("cancellation or redaction lost:", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("canceled request did not finish")
	}
}
