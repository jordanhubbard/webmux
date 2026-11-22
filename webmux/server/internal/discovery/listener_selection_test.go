package discovery

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pion/mdns/v2"
)

type multicastSelectionResult struct {
	endpoint Endpoint
	err      error
}

func listenerEvent(service, host, instance string, port uint16) mdns.ServiceEvent {
	return mdns.ServiceEvent{Instance: mdns.ServiceInstance{Service: service, Host: host, Instance: instance, Port: port}}
}

// Commit observations through the production callback accumulator, then end the
// window. The collector must retain later conflicting observations.
func selectListenerEvents(t *testing.T, allowHTTP bool, events []mdns.ServiceEvent) (Endpoint, error) {
	t.Helper()
	parent, cancelParent := context.WithTimeout(context.Background(), time.Second)
	defer cancelParent()
	query, endObservation := context.WithCancel(parent)
	defer endObservation()
	listeners := &multicastListeners{}
	done := make(chan multicastSelectionResult, 1)
	go func() {
		endpoint, err := collectMulticastEndpoints(parent, query, "builder.local", allowHTTP, listeners)
		done <- multicastSelectionResult{endpoint, err}
	}()
	for _, event := range events {
		listeners.observeHost("builder.local", event)
		select {
		case result := <-done:
			t.Fatalf("selected before the complete observation window: %+v %v", result.endpoint, result.err)
		default:
		}
	}
	endObservation()
	select {
	case result := <-done:
		return result.endpoint, result.err
	case <-parent.Done():
		t.Fatal("listener collector did not finish after observation")
		return Endpoint{}, parent.Err()
	}
}

func TestMulticastListenerSelectionRejectsCompetingPorts(t *testing.T) {
	for _, ports := range [][3]uint16{{8443, 19311, 8443}, {19311, 8443, 19311}} {
		events := []mdns.ServiceEvent{}
		for index, port := range ports {
			events = append(events, listenerEvent(secureService, "builder.local.", fmt.Sprintf("instance-%d", index), port))
		}
		endpoint, err := selectListenerEvents(t, false, events)
		if err == nil || !strings.Contains(err.Error(), "explicit URL") || endpoint != (Endpoint{}) {
			t.Fatalf("ambiguous secure listeners selected %+v: %v", endpoint, err)
		}
	}
}
func TestMulticastListenerSelectionSamePortDuplicatesAndForeignRecords(t *testing.T) {
	endpoint, err := selectListenerEvents(t, false, []mdns.ServiceEvent{
		listenerEvent(secureService, "foreign.local.", "foreign", 8443),
		listenerEvent(NodeService, "builder.local.", "unrelated-service", 8443),
		listenerEvent(secureService, "builder.local.", "zero-port", 0),
		listenerEvent(secureService, "BUILDER.LOCAL.", "first-interface", 19311),
		listenerEvent(secureService, "builder.local.", "second-instance", 19311),
	})
	if err != nil || endpoint != (Endpoint{Scheme: "https", Port: 19311}) {
		t.Fatalf("equal endpoint or unrelated records broke selection: %+v %v", endpoint, err)
	}
}
func TestMulticastListenerSelectionProtocolPreference(t *testing.T) {
	events := []mdns.ServiceEvent{
		listenerEvent(secureService, "builder.local.", "secure", 19311),
		listenerEvent(clearService, "builder.local.", "clear-one", 8080),
		listenerEvent(clearService, "builder.local.", "clear-two", 19310),
	}
	endpoint, err := selectListenerEvents(t, false, events)
	if err != nil || endpoint != (Endpoint{Scheme: "https", Port: 19311}) {
		t.Fatalf("nonpreferred cleartext conflict changed secure choice: %+v %v", endpoint, err)
	}
	endpoint, err = selectListenerEvents(t, true, events)
	if err == nil || endpoint != (Endpoint{}) {
		t.Fatalf("explicit HTTP chose a conflicting cleartext listener: %+v %v", endpoint, err)
	}
	endpoint, err = selectListenerEvents(t, true, []mdns.ServiceEvent{
		listenerEvent(secureService, "builder.local.", "secure-one", 8443),
		listenerEvent(secureService, "builder.local.", "secure-two", 19311),
		listenerEvent(clearService, "builder.local.", "clear", 19310),
	})
	if err != nil || endpoint != (Endpoint{Scheme: "http", Port: 19310}) {
		t.Fatalf("explicit unique HTTP choice was lost: %+v %v", endpoint, err)
	}
}
func TestMulticastListenerSelectionKeepsSecurityAndLegacyDefaults(t *testing.T) {
	endpoint, err := selectListenerEvents(t, false, []mdns.ServiceEvent{listenerEvent(clearService, "builder.local.", "http-only", 19310)})
	if err == nil || !strings.Contains(err.Error(), "allow-insecure-http") || endpoint != (Endpoint{}) {
		t.Fatalf("cleartext admitted implicitly: %+v %v", endpoint, err)
	}
	for _, allowHTTP := range []bool{false, true} {
		endpoint, err := selectListenerEvents(t, allowHTTP, nil)
		want := Endpoint{Scheme: "https", Port: 8443}
		if allowHTTP {
			want = Endpoint{Scheme: "http", Port: 8080}
		}
		if err != nil || endpoint != want {
			t.Fatalf("legacy no-record default changed: %+v %v", endpoint, err)
		}
	}
}
func TestMulticastListenerSelectionHonorsParentCancellation(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	cancel()
	endpoint, err := collectMulticastEndpoints(parent, parent, "builder.local", false, &multicastListeners{})
	if err != context.Canceled || endpoint != (Endpoint{}) {
		t.Fatalf("canceled query fabricated an endpoint: %+v %v", endpoint, err)
	}
}

func TestMulticastListenerSelectionSameInstancePortUpdates(t *testing.T) {
	endpoint, err := selectListenerEvents(t, false, []mdns.ServiceEvent{
		listenerEvent(secureService, "builder.local.", "first", 8443),
		listenerEvent(secureService, "builder.local.", "second", 19311),
		listenerEvent(secureService, "builder.local.", "first", 19311),
	})
	if err != nil || endpoint != (Endpoint{Scheme: "https", Port: 19311}) {
		t.Fatalf("same-instance update left a stale conflicting port: %+v %v", endpoint, err)
	}
}
func TestMulticastListenerSelectionIncludesCommittedDeadlineObservations(t *testing.T) {
	query, end := context.WithCancel(context.Background())
	listeners := &multicastListeners{}
	listeners.observeHost("builder.local", listenerEvent(secureService, "builder.local.", "one", 8443))
	listeners.observeHost("builder.local", listenerEvent(secureService, "builder.local.", "two", 19311))
	end()
	endpoint, err := collectMulticastEndpoints(context.Background(), query, "builder.local", false, listeners)
	if !errors.Is(err, ErrAmbiguousListener) || endpoint != (Endpoint{}) {
		t.Fatalf("queued conflict was lost at deadline: %+v %v", endpoint, err)
	}
}
func TestMulticastListenerSelectionRefusesIncompleteInventory(t *testing.T) {
	events := make([]mdns.ServiceEvent, 0, 257)
	for index := 0; index < 257; index++ {
		events = append(events, listenerEvent(secureService, "builder.local.", fmt.Sprintf("instance-%d", index), 19311))
	}
	endpoint, err := selectListenerEvents(t, false, events)
	if !errors.Is(err, errIncompleteListenerDiscovery) || endpoint != (Endpoint{}) {
		t.Fatalf("instance flood silently selected an endpoint: %+v %v", endpoint, err)
	}
}

func TestMulticastListenerSelectionConcurrentAddressRefreshes(t *testing.T) {
	listeners := &multicastListeners{}
	var writers sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		writers.Add(1)
		go func() {
			defer writers.Done()
			for iteration := 0; iteration < 10000; iteration++ {
				listeners.observeHost("builder.local", listenerEvent(secureService, "builder.local.", "primary", 19311))
			}
		}()
	}
	listeners.observeHost("builder.local", listenerEvent(secureService, "builder.local.", "competitor", 8443))
	writers.Wait()
	endpoint, err := listeners.choose(false)
	if !errors.Is(err, ErrAmbiguousListener) || endpoint != (Endpoint{}) {
		t.Fatalf("duplicate refreshes lost a conflicting listener: %+v %v", endpoint, err)
	}
	// Replacing the competitor's port converges to one endpoint even after
	// the refresh burst; duplicates consume only one slot per instance.
	listeners.observeHost("builder.local", listenerEvent(secureService, "builder.local.", "competitor", 19311))
	endpoint, err = listeners.choose(false)
	if err != nil || endpoint != (Endpoint{Scheme: "https", Port: 19311}) {
		t.Fatalf("refresh burst exhausted state or retained stale port: %+v %v", endpoint, err)
	}
}
