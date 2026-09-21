package httpapi

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"testing"
)

func TestConcurrentCatalogWritesAndPrivateKeyRedaction(t *testing.T) {
	s, handler := fixture(t, "none")
	for name, value := range map[string]string{"hosts.yaml": "hosts: []\n", "keys.yaml": "keys: []\n"} {
		if err := os.WriteFile(s.store.ConfigPath(name), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var workers sync.WaitGroup
	for i := range 20 {
		workers.Go(func() {
			created := request(handler, "POST", "/api/hosts", fmt.Sprintf(`{"id":"host-%d","hostname":"fixture.invalid"}`, i), "")
			if created.Code != 201 {
				t.Errorf("host creation failed: %s", created.Body.String())
			}
			key := request(handler, "POST", "/api/keys", fmt.Sprintf(`{"id":"key-%d","private_key_path":"/private/fixture"}`, i), "")
			if key.Code != 201 {
				t.Errorf("key creation failed: %s", key.Body.String())
			}
		})
	}
	workers.Wait()
	for _, route := range []string{"/api/hosts", "/api/keys"} {
		response := request(handler, "GET", route, "", "")
		requireStatus(t, response, 200)
		var records []catalogRecord
		if err := json.Unmarshal(response.Body.Bytes(), &records); err != nil {
			t.Fatal(err)
		}
		if len(records) != 20 {
			t.Fatalf("lost catalog updates for %s: %d", route, len(records))
		}
		for _, record := range records {
			if _, exists := record["private_key_path"]; exists {
				t.Fatal("private key path exposed")
			}
		}
	}
}

func TestCatalogInvalidInputDoesNotChangeConfiguration(t *testing.T) {
	s, handler := fixture(t, "none")
	for name, value := range map[string]string{"hosts.yaml": "hosts: []\n", "keys.yaml": "keys: []\n"} {
		if err := os.WriteFile(s.store.ConfigPath(name), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, body := range []string{`{"hostname":true}`, `{"hostname":"host","port":"22"}`, `{"hostname":"host","tags":[7]}`} {
		requireStatus(t, request(handler, "POST", "/api/hosts", body, ""), 400)
	}
	requireStatus(t, request(handler, "POST", "/api/keys", `{"private_key_path":123}`, ""), 400)
	for name, want := range map[string]string{"hosts.yaml": "hosts: []\n", "keys.yaml": "keys: []\n"} {
		got, err := os.ReadFile(s.store.ConfigPath(name))
		if err != nil || string(got) != want {
			t.Fatalf("invalid request changed %s: %q %v", name, got, err)
		}
	}
}
