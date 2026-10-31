package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jordanhubbard/webmux/server/internal/config"
	"github.com/jordanhubbard/webmux/server/internal/session"
	"testing"
	"time"
)

type inventoryProvider struct {
	id     string
	status func(context.Context) (coordinationProviderStatus, error)
}

func (p inventoryProvider) Descriptor() coordinationProviderDescriptor {
	return coordinationProviderDescriptor{ID: p.id, Label: p.id, Available: true}
}
func (p inventoryProvider) Status(ctx context.Context) (coordinationProviderStatus, error) {
	return p.status(ctx)
}
func (p inventoryProvider) Launch(context.Context, coordinationLaunchRequest) (coordinationLaunch, error) {
	return coordinationLaunch{}, errors.New("unexpected launch")
}
func (p inventoryProvider) Attach(context.Context, string) (coordinationLaunch, error) {
	return coordinationLaunch{}, errors.New("unexpected attach")
}

func TestEnabledIntegrationsDiscoveredTogetherWithIndependentFailures(t *testing.T) {
	started := make(chan string, 2)
	release := make(chan struct{})
	makeProvider := func(id string, fail bool) coordinationProvider {
		return inventoryProvider{id: id, status: func(ctx context.Context) (coordinationProviderStatus, error) {
			started <- id
			select {
			case <-release:
			case <-ctx.Done():
				return coordinationProviderStatus{}, ctx.Err()
			}
			if fail {
				return coordinationProviderStatus{}, errors.New("provider offline")
			}
			return coordinationProviderStatus{Seats: []coordinationSeat{{Provider: "spoofed", ID: "shared-id"}}, Messages: []coordinationMessage{{From: "worker"}}}, nil
		}}
	}
	app := config.Object{"coordination": config.Object{"enabled_providers": []string{"agent-deck", "rig"}, "default_launch_provider": "webmux", "default_view": "webmux"}}
	done := make(chan []coordinationDiscovery, 1)
	go func() {
		done <- discoverCoordination(context.Background(), []coordinationProvider{makeProvider("rig", true), makeProvider("agent-deck", false)}, app)
	}()
	for range 2 {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			close(release)
			t.Fatal("discovery serialized or skipped an enabled integration")
		}
	}
	close(release)
	results := <-done
	if len(results) != 2 || results[0].err == nil || results[1].err != nil || len(results[1].status.Seats) != 1 || results[1].status.Seats[0].Provider != "agent-deck" || results[1].status.Messages[0].Provider != "agent-deck" {
		t.Fatalf("failure leaked across providers: %+v", results)
	}
	app["coordination"] = config.Object{"enabled_providers": []string{}}
	for _, result := range discoverCoordination(context.Background(), []coordinationProvider{inventoryProvider{id: "rig", status: func(context.Context) (coordinationProviderStatus, error) {
		t.Fatal("disabled integration queried")
		return coordinationProviderStatus{}, nil
	}}}, app) {
		if result.descriptor.Enabled {
			t.Fatal("disabled integration marked enabled")
		}
	}
}

func TestNativeCoordinationInventoryReopensOwnedViewerWithoutLaunching(t *testing.T) {
	t.Setenv("WEBMUX_COORDINATION_DISABLED", "1")
	t.Setenv("WEBMUX_EXEC_COMMAND", "")
	s, handler := fixture(t, "none")
	setCoordinationIntegrations(t, s)
	own, err := s.sessions.Create("anonymous", session.CreateRequest{Hostname: "localhost", Username: "fixture", Transport: "exec"})
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := s.sessions.Create("other", session.CreateRequest{Hostname: "localhost", Username: "fixture", Transport: "exec"})
	if err != nil {
		t.Fatal(err)
	}
	hidden := true
	if _, err := s.sessions.Patch("anonymous", own.ID, session.Patch{Minimized: &hidden}); err != nil {
		t.Fatal(err)
	}
	response := request(handler, "GET", "/api/coordination", "", "")
	requireStatus(t, response, 200)
	var inventory struct {
		Seats []coordinationSeat `json:"seats"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &inventory); err != nil {
		t.Fatal(err)
	}
	if len(inventory.Seats) != 1 || inventory.Seats[0].Provider != "webmux" || inventory.Seats[0].ID != own.ID {
		t.Fatalf("native inventory leaked ownership: %s", response.Body.String())
	}
	opened := request(handler, "POST", "/api/coordination/attach", fmt.Sprintf(`{"provider":"webmux","target":%q}`, own.ID), "")
	requireStatus(t, opened, 200)
	var restored session.Session
	if err := json.Unmarshal(opened.Body.Bytes(), &restored); err != nil {
		t.Fatal(err)
	}
	if restored.ID != own.ID || restored.Minimized || restored.Rig != nil || len(s.sessions.List("anonymous")) != 1 {
		t.Fatalf("viewer was duplicated or reassigned: %+v", restored)
	}
	requireStatus(t, request(handler, "POST", "/api/coordination/attach", fmt.Sprintf(`{"provider":"webmux","target":%q}`, foreign.ID), ""), 404)
}
