//go:build linux || darwin

package credentialhub

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func fixture(t *testing.T) (*Vault, Scope, string) {
	t.Helper()
	v := New(t.TempDir())
	if e := v.Setup("alice"); e != nil {
		t.Fatal(e)
	}
	value := "dedicated-fixture-api-key-no-real-provider"
	s, e := v.Put("alice", "", "Fixture", value, "ai.openai", true)
	if e != nil {
		t.Fatal(e)
	}
	sc := Scope{Owner: "alice", SecretID: s.ID, EnrollmentID: "spoke", EnrollmentFingerprint: strings.Repeat("a", 64), SourceNode: strings.Repeat("b", 32), SourceOwner: "alice", Consumer: "ai.openai"}
	if _, e = v.Grant(sc); e != nil {
		t.Fatal(e)
	}
	return v, sc, value
}
func TestEncryptedPersistenceAndMetadata(t *testing.T) {
	v, s, value := fixture(t)
	_, p := v.paths()
	b, e := os.ReadFile(p)
	if e != nil || bytes.Contains(b, []byte(value)) || bytes.Contains(b, []byte("Fixture")) {
		t.Fatalf("unencrypted document or read failure: %v", e)
	}
	view, e := v.View("alice")
	if e != nil || !view.Enabled || len(view.Secrets) != 1 || len(view.Grants) != 1 {
		t.Fatal(view, e)
	}
	b, _ = json.Marshal(view)
	if bytes.Contains(b, []byte(value)) || bytes.Contains(b, []byte(s.EnrollmentFingerprint)) {
		t.Fatal("metadata exposed private material")
	}
	other, e := v.View("bob")
	if e != nil || len(other.Secrets) != 0 || len(other.Grants) != 0 || len(other.Audit) != 0 {
		t.Fatal("foreign account metadata", e)
	}
	token, _, e := v.Issue(s)
	if e != nil {
		t.Fatal(e)
	}
	restart := New(filepath.Dir(filepath.Dir(v.dir)))
	if _, e = restart.Check(s); e != nil {
		t.Fatal("persistent grant unavailable", e)
	}
	if _, e = restart.Redeem(s, token); !errors.Is(e, ErrDenied) {
		t.Fatal("handle survived restart", e)
	}
}
func TestExactlyOneConcurrentRedemption(t *testing.T) {
	v, s, value := fixture(t)
	token, _, e := v.Issue(s)
	if e != nil {
		t.Fatal(e)
	}
	var successes atomic.Int32
	var wg sync.WaitGroup
	for range 24 {
		wg.Go(func() {
			got, e := v.Redeem(s, token)
			if e == nil {
				if got != value {
					t.Error("wrong value")
				}
				successes.Add(1)
			} else if !errors.Is(e, ErrDenied) {
				t.Error(e)
			}
		})
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatal("redemption count", successes.Load())
	}
}
func TestScopeAndRevocationFences(t *testing.T) {
	cases := map[string]func(*Scope){"owner": func(s *Scope) { s.Owner = "bob" }, "secret": func(s *Scope) { s.SecretID = randomID() }, "enrollment": func(s *Scope) { s.EnrollmentID = "other" }, "generation": func(s *Scope) { s.EnrollmentFingerprint = strings.Repeat("c", 64) }, "node": func(s *Scope) { s.SourceNode = strings.Repeat("d", 32) }, "source-account": func(s *Scope) { s.SourceOwner = "bob" }, "consumer": func(s *Scope) { s.Consumer = "ai.nvidia" }, "project": func(s *Scope) { s.Project = "foreign" }}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			v, s, _ := fixture(t)
			token, _, e := v.Issue(s)
			if e != nil {
				t.Fatal(e)
			}
			other := s
			change(&other)
			if _, _, e = v.Issue(other); !errors.Is(e, ErrDenied) {
				t.Fatal("issue accepted foreign scope", e)
			}
			if _, e = v.Redeem(other, token); !errors.Is(e, ErrDenied) {
				t.Fatal("redeem accepted foreign scope", e)
			}
			if _, e = v.Redeem(s, token); e != nil {
				t.Fatal("foreign scope burned owner handle", e)
			}
		})
	}
	for _, action := range []string{"expire", "rotate", "disable", "delete", "revoke", "regrant"} {
		t.Run(action, func(t *testing.T) {
			v, s, _ := fixture(t)
			now := time.Now()
			v.now = func() time.Time { return now }
			token, _, e := v.Issue(s)
			if e != nil {
				t.Fatal(e)
			}
			switch action {
			case "expire":
				now = now.Add(handleTTL)
			case "rotate":
				_, e = v.Put("alice", s.SecretID, "Fixture", "rotated-fixture-key", "ai.openai", true)
			case "disable":
				_, e = v.Put("alice", s.SecretID, "Fixture", "", "ai.openai", false)
			case "delete":
				e = v.Delete("alice", s.SecretID)
			default:
				view, _ := v.View("alice")
				e = v.Revoke("alice", view.Grants[0].ID)
				if action == "regrant" && e == nil {
					_, e = v.Grant(s)
				}
			}
			if e != nil {
				t.Fatal(e)
			}
			if _, e = v.Redeem(s, token); !errors.Is(e, ErrDenied) {
				t.Fatal("stale handle accepted", e)
			}
		})
	}
}
func TestUnsafeOrLostCustodyFailsClosed(t *testing.T) {
	for _, action := range []string{"key-permissions", "dir-permissions", "key-symlink", "key-removed", "ciphertext-corrupt"} {
		t.Run(action, func(t *testing.T) {
			v, s, _ := fixture(t)
			kp, dp := v.paths()
			switch action {
			case "key-permissions":
				if e := os.Chmod(kp, 0644); e != nil {
					t.Fatal(e)
				}
			case "dir-permissions":
				if e := os.Chmod(v.dir, 0755); e != nil {
					t.Fatal(e)
				}
			case "key-symlink":
				dest := kp + ".old"
				if e := os.Rename(kp, dest); e != nil {
					t.Fatal(e)
				}
				if e := os.Symlink(dest, kp); e != nil {
					t.Fatal(e)
				}
			case "key-removed":
				if e := os.Remove(kp); e != nil {
					t.Fatal(e)
				}
			case "ciphertext-corrupt":
				if e := os.WriteFile(dp, []byte("bad cipher"), 0600); e != nil {
					t.Fatal(e)
				}
			}
			if _, e := v.Check(s); !errors.Is(e, ErrUnavailable) {
				t.Fatal("unsafe custody accepted", e)
			}
			if e := v.Setup("alice"); !errors.Is(e, ErrUnavailable) {
				t.Fatal("setup silently replaced unsafe/missing material", e)
			}
		})
	}
}
func TestOptInAndBounds(t *testing.T) {
	v := New(t.TempDir())
	view, e := v.View("alice")
	if e != nil || view.Enabled {
		t.Fatal(view, e)
	}
	if _, e = os.Lstat(v.dir); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("read created vault")
	}
	v, s, _ := fixture(t)
	if _, e = v.Put("bob", s.SecretID, "other", "foreign-key", "ai.openai", true); !errors.Is(e, ErrDenied) {
		t.Fatal("foreign write", e)
	}
	if _, e = v.Put("alice", s.SecretID, "Fixture", "key", "ai.nvidia", true); !errors.Is(e, ErrDenied) {
		t.Fatal("consumer rebound", e)
	}
	for range handlesPerEnrollment {
		if _, _, e = v.Issue(s); e != nil {
			t.Fatal(e)
		}
	}
	if _, _, e = v.Issue(s); !errors.Is(e, ErrLimit) {
		t.Fatal("unbounded handles", e)
	}
	view, e = v.View("alice")
	if e != nil || len(view.Audit) > 512 {
		t.Fatal("unbounded audit", e)
	}
}

func TestDeniedCredentialIdentifierIsNotEchoedInAudit(t *testing.T) {
	v, scope, _ := fixture(t)
	for _, value := range []string{"mistaken-provider-key-value", randomID()} {
		scope.SecretID = value
		if _, e := v.Check(scope); !errors.Is(e, ErrDenied) {
			t.Fatal(e)
		}
		view, e := v.View(scope.Owner)
		if e != nil {
			t.Fatal(e)
		}
		b, _ := json.Marshal(view)
		if bytes.Contains(b, []byte(value)) {
			t.Fatal("refused raw credential identifier echoed in metadata")
		}
	}
}

// All current grant recipients retain their own capacity, including when one
// recipient has already exhausted its allowance. Regranting cannot leak slots.
func TestHandleCapacityIsReservedForEveryEnrollment(t *testing.T) {
	v, scope, _ := fixture(t)
	now := time.Now()
	v.now = func() time.Time { return now }
	scopes := []Scope{scope}
	for i := 1; i < maxGrants; i++ {
		other := scope
		other.EnrollmentID = fmt.Sprintf("receiver-%d", i)
		if _, e := v.Grant(other); e != nil {
			t.Fatal(e)
		}
		scopes = append(scopes, other)
	}
	tokens := make([]string, 0, maxGrants)
	for _, s := range scopes {
		for n := 0; n < handlesPerEnrollment; n++ {
			token, _, e := v.Issue(s)
			if e != nil {
				t.Fatal("one enrollment starved another", s.EnrollmentID, e)
			}
			if n == 0 {
				tokens = append(tokens, token)
			}
		}
		if _, _, e := v.Issue(s); !errors.Is(e, ErrLimit) {
			t.Fatal("enrollment exceeded its capacity", e)
		}
	}
	if len(v.handles) != maxHandles {
		t.Fatal("unexpected bounded global capacity", len(v.handles))
	}
	// Existing handles remain usable after every other recipient has filled up.
	for i, s := range scopes {
		if _, e := v.Redeem(s, tokens[i]); e != nil {
			t.Fatal("recipient lost reserved handle", e)
		}
	}
	// Expiry reclaims all capacity without a restart.
	now = now.Add(handleTTL)
	if _, _, e := v.Issue(scope); e != nil {
		t.Fatal(e)
	}
	if len(v.handles) != 1 {
		t.Fatal("expired capacity retained", len(v.handles))
	}
}
func TestInvalidatedHandlesDoNotConsumeEnrollmentCapacity(t *testing.T) {
	for _, action := range []string{"rotate", "regrant"} {
		t.Run(action, func(t *testing.T) {
			v, s, _ := fixture(t)
			for range handlesPerEnrollment {
				if _, _, e := v.Issue(s); e != nil {
					t.Fatal(e)
				}
			}
			if action == "rotate" {
				if _, e := v.Put(s.Owner, s.SecretID, "Fixture", "rotated-fixture-key", "ai.openai", true); e != nil {
					t.Fatal(e)
				}
			} else {
				view, e := v.View(s.Owner)
				if e != nil {
					t.Fatal(e)
				}
				if e = v.Revoke(s.Owner, view.Grants[0].ID); e != nil {
					t.Fatal(e)
				}
				if _, e = v.Grant(s); e != nil {
					t.Fatal(e)
				}
			}
			if _, _, e := v.Issue(s); e != nil {
				t.Fatal("invalidated handles retained quota", e)
			}
			if len(v.handles) != 1 {
				t.Fatal("invalidated handles retained", len(v.handles))
			}
		})
	}
}
