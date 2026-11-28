package maccoord

import (
	"context"
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

func registrationFixture() map[string]any {
	return map[string]any{"schema": RegistrationSchema, "id": "project-canonical-alpha", "name": "alpha",
		"description": "PRIVATE_DESCRIPTION_MUST_NOT_ESCAPE", "status": "paused", "created_at": "2026-11-22T10:00:00Z",
		"updated_at": "2026-11-23T10:00:00+00:00", "repository_registration": "https://private-owner:PRIVATE_REPO_SECRET@example.invalid/repository#main"}
}

func registrationServer(t *testing.T, positive http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/projects/alpha/registration" || r.URL.RawQuery != "" {
			t.Error("registration reader left the fixed exact read route")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+accessToken {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		positive(w, r)
	}))
	t.Cleanup(server.Close)
	return server
}

func TestRegistrationCanonicalRecordAndRevocation(t *testing.T) {
	var allowed atomic.Bool
	allowed.Store(true)
	var mu sync.Mutex
	var credentials []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/projects/alpha/registration" || r.URL.RawQuery != "" {
			t.Error("wrong registration operation")
		}
		credential := r.Header.Get("Authorization")
		mu.Lock()
		credentials = append(credentials, credential)
		mu.Unlock()
		if credential != "Bearer "+accessToken || !allowed.Load() {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_ = json.NewEncoder(w).Encode(registrationFixture())
	}))
	defer server.Close()
	b := accessBindings(t, server.URL)
	got, err := b.ObserveProjectRegistration(t.Context(), "alice", repository(), accessToken, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Schema != RegistrationObservationSchema || got.ProjectID != "project-canonical-alpha" || got.ProjectKey != "alpha" || got.AuthorityID != "fleet-a" || got.BindingRevision == "" || got.RegistrationStatus != "paused" || got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() || got.ObservedAt.IsZero() {
		t.Fatalf("incorrect canonical observation: %+v", got)
	}
	auth := got.ReadAuthentication
	if got.RepositoryLinkState != "unverified" || got.Freshness != "unqualified" || !got.RepositoryRegistrationReported || auth.UpstreamSchema != RegistrationSchema || auth.PrincipalIdentity != "unverified" || auth.OtherOperations != "unqualified" || !auth.MissingCredentialsRefused || !auth.InvalidCredentialsRefused || !auth.SuppliedCredentialAccepted {
		t.Fatalf("observation invented authority: %+v", got)
	}
	encoded, _ := json.Marshal(got)
	for _, forbidden := range []string{"PRIVATE_DESCRIPTION", "PRIVATE_REPO_SECRET", "private-owner", "example.invalid", accessToken, server.URL} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatal("unprojected or credential data escaped")
		}
	}
	allowed.Store(false)
	if next, err := b.ObserveProjectRegistration(t.Context(), "alice", repository(), accessToken, nil); !errors.Is(err, ErrReadAccess) || next.Schema != "" {
		t.Fatal("revocation reused prior observation", next, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(credentials) != 6 || credentials[0] != "" || credentials[3] != "" || credentials[2] != "Bearer "+accessToken || credentials[5] != credentials[2] || !strings.HasPrefix(credentials[1], "Bearer webmux-denied-") || !strings.HasPrefix(credentials[4], "Bearer webmux-denied-") || credentials[1] == credentials[4] {
		t.Fatal("authentication controls missing or reused")
	}
}

func TestRegistrationNoTransportForUnmappedOrInvalidInput(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.WriteHeader(403) }))
	defer server.Close()
	b := accessBindings(t, server.URL)
	for _, owner := range []string{"bob", "unknown", ""} {
		got, err := b.ObserveProjectRegistration(t.Context(), owner, repository(), accessToken, nil)
		if !errors.Is(err, ErrUnmapped) || got.Schema != "" {
			t.Fatal("foreign account got registration", err)
		}
	}
	changed := repository()
	changed.ID = "same-remote-different-root"
	if _, err := b.ObserveProjectRegistration(t.Context(), "alice", changed, accessToken, nil); !errors.Is(err, ErrUnmapped) {
		t.Fatal("changed repository was accepted")
	}
	for _, token := range []string{"", "invalid\ntoken"} {
		if _, err := b.ObserveProjectRegistration(t.Context(), "alice", repository(), token, nil); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid intended token accepted")
		}
	}
	if _, err := b.ObserveProjectRegistration(nil, "alice", repository(), accessToken, nil); !errors.Is(err, ErrInvalid) {
		t.Fatal("nil context accepted")
	}
	if requests.Load() != 0 {
		t.Fatal("unmapped/invalid request reached authority")
	}
}

func TestRegistrationRejectsMalformedOrForeignRecord(t *testing.T) {
	cases := []struct {
		name string
		edit func(map[string]any)
		want error
	}{
		{"future schema", func(m map[string]any) { m["schema"] = "mac.project_registration.v2" }, ErrUnsupported},
		{"foreign project", func(m map[string]any) { m["name"] = "beta" }, ErrUnmapped},
		{"empty canonical id", func(m map[string]any) { m["id"] = "" }, ErrInvalid},
		{"null canonical id", func(m map[string]any) { m["id"] = nil }, ErrInvalid},
		{"unsafe canonical id", func(m map[string]any) { m["id"] = "../other" }, ErrInvalid},
		{"null status", func(m map[string]any) { m["status"] = nil }, ErrInvalid},
		{"oversized status", func(m map[string]any) { m["status"] = strings.Repeat("x", 65) }, ErrInvalid},
		{"null created", func(m map[string]any) { m["created_at"] = nil }, ErrInvalid},
		{"invalid update", func(m map[string]any) { m["updated_at"] = "not-a-time" }, ErrInvalid},
		{"null description", func(m map[string]any) { m["description"] = nil }, ErrInvalid},
		{"missing nullable relation", func(m map[string]any) { delete(m, "repository_registration") }, ErrInvalid},
		{"object relation", func(m map[string]any) { m["repository_registration"] = map[string]any{"secret": "PRIVATE"} }, ErrInvalid},
		{"empty relation", func(m map[string]any) { m["repository_registration"] = "" }, ErrInvalid},
		{"unexpected metadata", func(m map[string]any) { m["metadata"] = map[string]any{"secret": "PRIVATE"} }, ErrInvalid},
		{"case alias", func(m map[string]any) { m["ID"] = m["id"]; delete(m, "id") }, ErrInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := registrationServer(t, func(w http.ResponseWriter, r *http.Request) {
				m := registrationFixture()
				tc.edit(m)
				_ = json.NewEncoder(w).Encode(m)
			})
			got, err := accessBindings(t, server.URL).ObserveProjectRegistration(t.Context(), "alice", repository(), accessToken, nil)
			if !errors.Is(err, tc.want) || got.Schema != "" || got.ProjectID != "" {
				t.Fatalf("bad response became partial registration: %+v %v", got, err)
			}
			if err != nil && (strings.Contains(err.Error(), "PRIVATE") || strings.Contains(err.Error(), server.URL)) {
				t.Fatal("error leaked response or endpoint")
			}
		})
	}
}

func TestRegistrationNullableRelationDoesNotInventRepositoryLink(t *testing.T) {
	server := registrationServer(t, func(w http.ResponseWriter, r *http.Request) {
		m := registrationFixture()
		m["repository_registration"] = nil
		_ = json.NewEncoder(w).Encode(m)
	})
	got, err := accessBindings(t, server.URL).ObserveProjectRegistration(t.Context(), "alice", repository(), accessToken, nil)
	if err != nil || got.RepositoryRegistrationReported || got.RepositoryLinkState != "unverified" {
		t.Fatal("null relation changed registration or proved a repository", got, err)
	}
}

func TestRegistrationWireBoundsAndMissingRoute(t *testing.T) {
	var sinkRequests atomic.Int32
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { sinkRequests.Add(1) }))
	defer sink.Close()
	cases := []struct {
		name  string
		reply func(http.ResponseWriter)
		want  error
	}{
		{"duplicate id", func(w http.ResponseWriter) {
			data, _ := json.Marshal(registrationFixture())
			fmt.Fprintf(w, `{"id":"foreign",%s`, data[1:])
		}, ErrInvalid},
		{"escaped duplicate", func(w http.ResponseWriter) {
			data, _ := json.Marshal(registrationFixture())
			fmt.Fprintf(w, `{"\u0069d":"foreign",%s`, data[1:])
		}, ErrInvalid},
		{"oversized body", func(w http.ResponseWriter) {
			m := registrationFixture()
			m["description"] = strings.Repeat("x", registrationResponseLimit)
			_ = json.NewEncoder(w).Encode(m)
		}, ErrInvalid},
		{"invalid UTF8", func(w http.ResponseWriter) { _, _ = w.Write([]byte{'{', '"', 0xff, '"', ':', '1', '}'}) }, ErrInvalid},
		{"graph is not registration", func(w http.ResponseWriter) { graphResponse(w) }, ErrInvalid},
		{"unknown record or unsupported route", func(w http.ResponseWriter) { w.WriteHeader(404); fmt.Fprint(w, `{"detail":"PRIVATE"}`) }, ErrUnsupported},
		{"redirect", func(w http.ResponseWriter) { w.Header().Set("Location", sink.URL); w.WriteHeader(302) }, ErrUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := registrationServer(t, func(w http.ResponseWriter, r *http.Request) { tc.reply(w) })
			got, err := accessBindings(t, server.URL).ObserveProjectRegistration(t.Context(), "alice", repository(), accessToken, nil)
			if !errors.Is(err, tc.want) || got.Schema != "" {
				t.Fatal("unqualified wire produced evidence", got, err)
			}
		})
	}
	if sinkRequests.Load() != 0 {
		t.Fatal("registration reader followed redirect")
	}
}

func TestRegistrationCancellationReturnsNoPartialEvidence(t *testing.T) {
	for _, stage := range []string{"before_headers", "during_body"} {
		t.Run(stage, func(t *testing.T) {
			started := make(chan struct{})
			server := registrationServer(t, func(w http.ResponseWriter, r *http.Request) {
				if stage == "during_body" {
					w.WriteHeader(200)
					fmt.Fprint(w, `{"schema":`)
					w.(http.Flusher).Flush()
				}
				close(started)
				<-r.Context().Done()
			})
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() {
				got, err := accessBindings(t, server.URL).ObserveProjectRegistration(ctx, "alice", repository(), accessToken, nil)
				if got.Schema != "" {
					done <- errors.New("partial registration on cancellation")
					return
				}
				done <- err
			}()
			<-started
			cancel()
			if err := <-done; !errors.Is(err, context.Canceled) || !errors.Is(err, ErrUnavailable) {
				t.Fatal("lost cancellation or leaked raw transport error", err)
			}
		})
	}
}

func TestRegistrationRefusesUnauthenticatedAuthority(t *testing.T) {
	for _, mode := range []string{"anonymous", "any_bearer"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if mode == "any_bearer" && r.Header.Get("Authorization") == "" {
					w.WriteHeader(403)
					return
				}
				_ = json.NewEncoder(w).Encode(registrationFixture())
			}))
			defer server.Close()
			got, err := accessBindings(t, server.URL).ObserveProjectRegistration(t.Context(), "alice", repository(), accessToken, nil)
			if !errors.Is(err, ErrReadAccess) || got.Schema != "" {
				t.Fatal("open authority accepted as authenticated registration", got, err)
			}
		})
	}
}

func TestRegistrationRejectsUnsafeProjectKeysBeforeTransport(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.WriteHeader(403) }))
	defer server.Close()
	for _, tc := range []struct {
		key           string
		parserRejects bool
	}{
		{"../alpha", true}, {"alpha/beta", true}, {"alpha?include=secrets", true},
		{"alpha%2fbeta", false}, {"alpha\\beta", true}, {".", false}, {"..", false},
	} {
		t.Run(tc.key, func(t *testing.T) {
			doc := bindingFixture()
			doc.Authorities[0].Origin = server.URL
			doc.Bindings[0].ProjectKey = tc.key
			b, err := ParseBindings(bindingBytes(t, doc))
			if tc.parserRejects {
				if !errors.Is(err, ErrInvalid) {
					t.Fatal("binding parser admitted unsafe project key")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got, err := b.ObserveProjectRegistration(t.Context(), "alice", repository(), accessToken, nil)
			if !errors.Is(err, ErrInvalid) || got.Schema != "" {
				t.Fatal("unsafe selected project escaped fixed route", got, err)
			}
		})
	}
	if requests.Load() != 0 {
		t.Fatal("unsafe project selection reached authority")
	}
}

func TestRegistrationEscapesExactProjectSelection(t *testing.T) {
	const project = "alpha beta"
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.EscapedPath() != "/projects/alpha%20beta/registration" || r.URL.RawQuery != "" || r.Method != http.MethodGet {
			t.Error("project was not an exact escaped path segment")
		}
		if r.Header.Get("Authorization") != "Bearer "+accessToken {
			w.WriteHeader(403)
			return
		}
		m := registrationFixture()
		m["name"] = project
		_ = json.NewEncoder(w).Encode(m)
	}))
	defer server.Close()
	doc := bindingFixture()
	doc.Authorities[0].Origin = server.URL
	doc.Bindings[0].ProjectKey = project
	b, err := ParseBindings(bindingBytes(t, doc))
	if err != nil {
		t.Fatal(err)
	}
	got, err := b.ObserveProjectRegistration(t.Context(), "alice", repository(), accessToken, nil)
	if err != nil || got.ProjectKey != project || requests.Load() != 3 {
		t.Fatal("exact escaped selection failed", got, err)
	}
}
