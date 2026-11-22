package discovery

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Two independent hosts use distinct IDs and private output directories. This
// exercises production discovery without starting HTTP or touching a service.
func TestNodeNativeEndpoint(t *testing.T) {
	role := os.Getenv("WEBMUX_TEST_NODE_ROLE")
	if role == "" {
		t.Skip("explicit native two-host discovery fixture only")
	}
	if role != "advertise" && role != "browse" && role != "both" {
		t.Fatal("invalid native endpoint role")
	}
	id, expected := os.Getenv("WEBMUX_TEST_NODE_ID"), os.Getenv("WEBMUX_TEST_NODE_EXPECTED")
	output := os.Getenv("WEBMUX_TEST_NODE_OUTPUT")
	if !ValidNodeID(id) || !filepath.IsAbs(output) || (role != "advertise" && (!ValidNodeID(expected) || expected == id)) {
		t.Fatal("explicit unique identities and a private absolute output path required")
	}
	if err := os.Mkdir(output, 0700); err != nil {
		t.Fatal(err)
	}
	write := func(name string, value any) {
		t.Helper()
		data, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(output, name), append(data, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
	duration := 8 * time.Second
	if role == "advertise" {
		duration = 12 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()
	host := os.Getenv("WEBMUX_TEST_NODE_HOST")
	if host == "" {
		// Match the production command's hostname input. An explicit override
		// supports controlled comparisons with synthetic host records.
		var err error
		host, err = os.Hostname()
		if err != nil {
			t.Fatal(err)
		}
	}
	options := NodeOptions{ID: id, Host: host, Port: 19311, TLS: true, Browse: role != "advertise", Advertise: role != "browse"}
	if selected := os.Getenv("WEBMUX_TEST_NODE_INTERFACES"); selected != "" {
		options.Interfaces = strings.Split(selected, ",")
	}
	endpoint, err := StartNodes(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := endpoint.Close(); err != nil {
			t.Error(err)
		}
	}()
	write("ready.json", map[string]any{"role": role, "id": id, "pid": os.Getpid(), "options": options})
	if role == "advertise" {
		<-ctx.Done()
		return
	}
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	observed := false
	for {
		candidates := endpoint.Snapshot(time.Now())
		matching := make([]Candidate, 0)
		for _, candidate := range candidates {
			if candidate.ClaimedID == expected {
				if candidate.Port != 19311 || !candidate.TLS || candidate.Instance != "WebMux-"+expected {
					t.Fatal("peer identity matched but advertised listener metadata differed")
				}
				matching = append(matching, candidate)
			}
			if candidate.ClaimedID == expected && !observed {
				write("observed.json", candidate)
				t.Log("observed the exact independently advertised native peer")
				observed = true
				if role == "browse" {
					write("observed-all.json", matching)
					return
				}
			}
		}
		select {
		case <-ctx.Done():
			if observed {
				write("observed-all.json", matching)
				return
			}
			write("failed-snapshot.json", candidates)
			t.Fatal("native peer was not discovered within the original eight-second observation bound")
		case <-ticker.C:
		}
	}
}
