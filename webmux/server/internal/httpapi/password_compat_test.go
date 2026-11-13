package httpapi

import (
	"crypto/rand"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jordanhubbard/webmux/server/internal/auth"
)

func TestLoginWithNodeArgon2ParameterOrder(t *testing.T) {
	s, handler := fixture(t, "local")
	config, err := auth.LoadConfig(s.store)
	if err != nil {
		t.Fatal(err)
	}
	password := rand.Text()
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	// Use the order emitted by node-argon2 with fresh fixture credentials.
	hash = strings.Replace(hash, "m=65536,t=3,p=4", "m=65536,p=4,t=3", 1)
	credentials := func(value string) string {
		t.Helper()
		body, err := json.Marshal(map[string]string{"username": "imported", "password": value})
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}
	config.Auth.Users = []auth.User{{Username: "imported", PasswordHash: hash, Admin: true}}
	if err := s.store.WriteConfig("auth.yaml", config); err != nil {
		t.Fatal(err)
	}
	requireStatus(t, request(handler, "POST", "/api/auth/login", credentials(rand.Text()), ""), 401)
	token := responseToken(t, request(handler, "POST", "/api/auth/login", credentials(password), ""))
	requireStatus(t, request(handler, "GET", "/api/auth/me", "", token), 200)
	stored, err := auth.LoadConfig(s.store)
	if err != nil || len(stored.Auth.Users) != 1 || stored.Auth.Users[0].PasswordHash != hash {
		t.Fatalf("login rewrote the imported password hash: %v", err)
	}
}
