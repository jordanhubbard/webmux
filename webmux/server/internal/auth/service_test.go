package auth

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jordanhubbard/webmux/server/internal/storage"
)

func testService(t *testing.T) (*Service, *storage.Store) {
	t.Helper()
	defaults := t.TempDir()
	if err := os.WriteFile(filepath.Join(defaults, "auth.yaml"), []byte("auth:\n  mode: local\n  users: []\n  custom: preserve\n"), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := storage.Open(t.TempDir(), defaults)
	if err != nil {
		t.Fatal(err)
	}
	service, err := New(store, "")
	if err != nil {
		t.Fatal(err)
	}
	return service, store
}

func TestTokensSurviveRestartAndEnforceExpiryAndAlgorithm(t *testing.T) {
	s, store := testService(t)
	now := time.Unix(1800000000, 0)
	s.now = func() time.Time { return now }
	token, err := s.Sign("owner")
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := New(store, "")
	if err != nil {
		t.Fatal(err)
	}
	restarted.now = s.now
	if user, err := restarted.Verify(token); err != nil || user != "owner" {
		t.Fatalf("token lost on restart: %s %v", user, err)
	}
	config, err := LoadConfig(store)
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Auth.JWTSecret) != 64 || config.Auth.Extra["custom"] != "preserve" {
		t.Fatal("secret not persisted or unknown config lost")
	}
	now = now.Add(TokenTTL)
	if _, err := restarted.Verify(token); err == nil {
		t.Fatal("expired token accepted")
	}
	for _, method := range []jwt.SigningMethod{jwt.SigningMethodHS384, jwt.SigningMethodHS512} {
		wrong, err := jwt.NewWithClaims(method, jwt.RegisteredClaims{Subject: "owner", ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour))}).SignedString(s.secret)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := restarted.Verify(wrong); err == nil {
			t.Fatal("unexpected signing algorithm accepted")
		}
	}
	noExpiry, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{Subject: "owner"}).SignedString(s.secret)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.Verify(noExpiry); err == nil {
		t.Fatal("non-expiring token accepted")
	}
	if _, err := restarted.Verify(token + "corrupted"); err == nil {
		t.Fatal("invalid signature accepted")
	}
}

func TestTicketSingleUseUnderConcurrencyAndExpiry(t *testing.T) {
	s, _ := testService(t)
	now := time.Now()
	s.now = func() time.Time { return now }
	ticket, err := s.IssueTicket("owner")
	if err != nil {
		t.Fatal(err)
	}
	var successes atomic.Int32
	var workers sync.WaitGroup
	for range 50 {
		workers.Go(func() {
			if username, ok := s.ConsumeTicket(ticket); ok {
				if username != "owner" {
					t.Errorf("wrong owner: %s", username)
				}
				successes.Add(1)
			}
		})
	}
	workers.Wait()
	if successes.Load() != 1 {
		t.Fatalf("ticket consumed %d times", successes.Load())
	}
	expiring, err := s.IssueTicket("owner")
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(TicketTTL)
	if _, ok := s.ConsumeTicket(expiring); ok {
		t.Fatal("expired ticket accepted")
	}
}

func TestLegacyAdminResolution(t *testing.T) {
	users := []User{{Username: "first"}, {Username: "second"}}
	if !IsAdmin(users, "first") || IsAdmin(users, "second") {
		t.Fatal("legacy owner resolution failed")
	}
	users[1].Admin = true
	if IsAdmin(users, "first") || !IsAdmin(users, "second") {
		t.Fatal("explicit admin resolution failed")
	}
}

func TestPasswordHashAndMalformedParameters(t *testing.T) {
	hash, err := HashPassword("unicode-🔐-password")
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := VerifyPassword(hash, "unicode-🔐-password"); err != nil || !ok {
		t.Fatalf("password mismatch: %v", err)
	}
	if ok, err := VerifyPassword(hash, "wrong"); err != nil || ok {
		t.Fatalf("incorrect password accepted: %v", err)
	}
	for _, invalid := range []string{
		"", "$argon2id$v=19$m=0,t=0,p=0$c2FsdA$YWJj",
		"$argon2id$v=19$m=4294967295,t=3,p=4$c2FsdA$YWJj",
		"$argon2id$v=19$m=65536,t=3,p=256$c2FsdA$YWJj",
		"$argon2id$v=16$m=65536,t=3,p=4$c2FsdA$YWJj",
	} {
		if ok, err := VerifyPassword(invalid, "test"); err == nil || ok {
			t.Errorf("malformed hash accepted: %s", invalid)
		}
	}
}
