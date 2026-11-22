package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pion/mdns/v2"
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
	options := NodeOptions{ID: id, Host: host, HTTPPort: 19310, Port: 19311, TLS: true, Browse: role != "advertise", Advertise: role != "browse"}
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
				if os.Getenv("WEBMUX_TEST_NODE_INSTANCE_RECORDS") == "1" {
					records, err := nativePeerListenerRecords(ctx, candidate)
					if err != nil {
						t.Fatal(err)
					}
					write("listener-instance-records.json", records)
				}
				if os.Getenv("WEBMUX_TEST_NODE_LOOKUP") == "1" {
					// Resolve the hostname actually returned by DNS-SD. On macOS
					// Bonjour owns the current host name and its address records.
					host := strings.TrimSuffix(candidate.Host, ".")
					results := make([]map[string]any, 0, 2)
					for _, allowHTTP := range []bool{false, true} {
						scheme, port := "https", "19311"
						if allowHTTP {
							scheme, port = "http", "19310"
						}
						want := (&url.URL{Scheme: scheme, Host: net.JoinHostPort(host, port)}).String()
						got, err := Resolve(ctx, host, allowHTTP)
						errText := ""
						if err != nil {
							errText = err.Error()
						}
						results = append(results, map[string]any{"allow_http": allowHTTP, "want": want, "got": got, "error": errText})
						write("listener-lookup-results.json", results)
						if err != nil || got != want {
							actual, _ := url.Parse(got)
							actualPort := ""
							if actual != nil {
								actualPort = actual.Port()
							}
							t.Fatalf("native peer listener lookup differed (allowHTTP=%t, expectedPort=%s, actualPort=%s): %v", allowHTTP, port, actualPort, err)
						}
					}
					write("listener-lookups.json", map[string]bool{"https": true, "http": true})
				}
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

// Inspect the exact peer's legacy records on the real network, independently of
// host-only selection when another server advertises the same system hostname.
func nativePeerListenerRecords(ctx context.Context, peer Candidate) (map[string]uint16, error) {
	v4, v6, err := multicastSockets()
	if err != nil {
		return nil, err
	}
	conn, err := mdns.NewServer(packet4(v4), packet6(v6), mdns.WithCacheRefresh(false))
	if err != nil {
		closeSockets(v4, v6)
		return nil, err
	}
	defer conn.Close()
	found := make(chan mdns.ServiceEvent, 32)
	names := map[string]string{clearService: peer.Instance + " HTTP", secureService: peer.Instance + " HTTPS"}
	conn.OnServiceDiscovered(func(event mdns.ServiceEvent) {
		if names[event.Instance.Service] != event.Instance.Instance || !strings.EqualFold(strings.TrimSuffix(event.Instance.Host, "."), strings.TrimSuffix(peer.Host, ".")) {
			return
		}
		select {
		case found <- event:
		default:
		}
	})
	for service := range names {
		if err := conn.Browse(ctx, service); err != nil {
			return nil, err
		}
	}
	records := map[string]uint16{}
	for len(records) < len(names) {
		select {
		case event := <-found:
			want := uint16(19311)
			if event.Instance.Service == clearService {
				want = 19310
			}
			if event.Instance.Port != want {
				return nil, fmt.Errorf("exact native listener instance advertises port %d instead of %d", event.Instance.Port, want)
			}
			records[event.Instance.Service] = event.Instance.Port
		case <-ctx.Done():
			return nil, fmt.Errorf("exact native listener records not observed within the original observation bound: %w", ctx.Err())
		}
	}
	return records, nil
}
