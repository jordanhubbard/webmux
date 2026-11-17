package httpapi

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/rigobservation"
)

const remoteRigPrefix = "host-rig/"

type remoteRigSeat struct {
	client      clientRecord
	host        catalogRecord
	seat        rigobservation.Seat
	observation string
	at          string
}

func remoteRigTarget(clientID string, seat rigobservation.Seat) string {
	return remoteRigPrefix + clientID + "/" + seat.Key()
}

// Only a bound, owner-scoped credential supplies authority. Hostname, IP,
// terminal title and self-reported host identifiers never bind a report.
func (s *Server) remoteRigSeats(owner string) []remoteRigSeat {
	clients, err := s.readClients()
	if err != nil {
		return nil
	}
	var hosts hostsConfig
	if s.store.ReadConfig("hosts.yaml", &hosts) != nil {
		return nil
	}
	result := []remoteRigSeat{}
	s.clientMu.Lock()
	defer s.clientMu.Unlock()
	for _, client := range clients.Clients {
		if client.Owner != owner || client.HostID == "" {
			continue
		}
		var host catalogRecord
		for _, h := range hosts.Hosts {
			if h["id"] == client.HostID {
				host = h
				break
			}
		}
		if host == nil {
			continue
		}
		facts := client.Snapshot
		if latest, ok := s.clientFacts[client.ID]; ok {
			facts = &latest
		}
		if facts == nil || facts.Rig == nil || facts.LastRig == nil || rigobservation.Validate(facts.LastRig) != nil {
			continue
		}
		state := "current"
		at, err := time.Parse(time.RFC3339, facts.RigAt)
		if s.clientSockets[client.ID] == nil {
			state = "offline"
		} else if err != nil || time.Since(at) > 50*time.Second || time.Until(at) > time.Second {
			state = "stale"
		}
		if state != "offline" && facts.Rig.State != "ok" {
			state = "unavailable"
		}
		for _, seat := range facts.LastRig.Seats {
			result = append(result, remoteRigSeat{client, host, seat, state, facts.RigAt})
		}
	}
	return result
}

func (s *Server) hasRemoteRigCapability(owner string) bool {
	clients, err := s.readClients()
	if err != nil {
		return false
	}
	var hosts hostsConfig
	if s.store.ReadConfig("hosts.yaml", &hosts) != nil {
		return false
	}
	s.clientMu.Lock()
	defer s.clientMu.Unlock()
	for _, client := range clients.Clients {
		if client.Owner != owner || client.HostID == "" {
			continue
		}
		bound := false
		for _, host := range hosts.Hosts {
			if host["id"] == client.HostID {
				bound = true
				break
			}
		}
		if !bound {
			continue
		}
		facts := client.Snapshot
		if latest, ok := s.clientFacts[client.ID]; ok {
			facts = &latest
		}
		if facts != nil && facts.Rig != nil && rigobservation.Validate(facts.Rig) == nil {
			return true
		}
	}
	return false
}

func (p *rigProvider) remoteStatus() coordinationProviderStatus {
	result := coordinationProviderStatus{Projects: []coordinationProject{}, Seats: []coordinationSeat{}}
	if p.server == nil {
		return result
	}
	groups := map[string]int{}
	for _, row := range p.server.remoteRigSeats(p.owner) {
		host, _ := row.host["hostname"].(string)
		project := remoteRigPrefix + row.client.ID + "/" + row.seat.RigID
		i, exists := groups[project]
		if !exists {
			i = len(result.Projects)
			groups[project] = i
			result.Projects = append(result.Projects, coordinationProject{Provider: "rig", ID: project, Name: row.seat.RigName, Host: host, State: "observed", ReadOnly: true, Detail: "Host client " + row.client.Label + " · " + rigobservation.Contract + " · " + row.observation})
		}
		result.Projects[i].SeatCount++
		lifecycle := row.seat.LifecycleState
		if lifecycle == "" {
			lifecycle = "unknown"
		}
		if lifecycle == "running" {
			result.Projects[i].RunningCount++
		}
		transport := row.observation
		openable := row.observation == "current" && row.seat.TmuxTarget != ""
		if row.observation == "current" {
			transport = "unavailable"
			if openable {
				transport = "reported"
			}
		}
		result.Seats = append(result.Seats, coordinationSeat{Provider: "rig", ID: remoteRigTarget(row.client.ID, row.seat), ProjectID: project, ProjectName: row.seat.RigName, Name: row.seat.LogicalID, Host: host, Runtime: row.seat.Runtime, State: lifecycle, TransportState: transport, ObservationState: row.observation, ObservedAt: row.at, Context: row.seat.PodNamespace, Openable: openable, Managed: false})
	}
	return result
}

func (p *rigProvider) remoteSeat(target string) (remoteRigSeat, error) {
	if p.server != nil {
		for _, row := range p.server.remoteRigSeats(p.owner) {
			if remoteRigTarget(row.client.ID, row.seat) == target {
				if row.observation != "current" || row.seat.TmuxTarget == "" {
					return remoteRigSeat{}, fmt.Errorf("remote Rig attachment is unavailable (%s); inspect the execution host without restarting its seat", row.observation)
				}
				return row, nil
			}
		}
	}
	return remoteRigSeat{}, fmt.Errorf("remote Rig seat is no longer reported by a linked host client")
}

var remoteSSHHost = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]*$|^\[[0-9a-fA-F:]+\]$`)
var remoteSSHUser = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

func (p *rigProvider) remoteAttach(_ context.Context, target string) (coordinationLaunch, error) {
	row, err := p.remoteSeat(target)
	if err != nil {
		return coordinationLaunch{}, err
	}
	host, _ := row.host["hostname"].(string)
	user, _ := row.host["username"].(string)
	if !remoteSSHHost.MatchString(host) || (user != "" && !remoteSSHUser.MatchString(user)) {
		return coordinationLaunch{}, fmt.Errorf("linked SSH host is invalid")
	}
	port := 22
	if value, ok := row.host["port"].(int); ok {
		port = value
	}
	if port < 1 || port > 65535 {
		return coordinationLaunch{}, fmt.Errorf("linked SSH port is invalid")
	}
	binary, err := exec.LookPath("ssh")
	if err != nil {
		return coordinationLaunch{}, err
	}
	args := []string{binary, "-tt", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes", "-o", "ConnectTimeout=8", "-o", "ServerAliveInterval=15", "-o", "ServerAliveCountMax=2", "-p", strconv.Itoa(port)}
	if user != "" {
		args = append(args, "-l", user)
	}
	if keyID, _ := row.host["key_id"].(string); keyID != "" {
		var keys keysConfig
		if err := p.server.store.ReadConfig("keys.yaml", &keys); err != nil {
			return coordinationLaunch{}, err
		}
		path := ""
		for _, key := range keys.Keys {
			if key["id"] == keyID {
				path, _ = key["private_key_path"].(string)
				break
			}
		}
		if path == "" {
			return coordinationLaunch{}, fmt.Errorf("linked SSH key is unavailable")
		}
		args = append(args, "-i", path, "-o", "IdentitiesOnly=yes")
	}
	// Target passes the strict contract token validation. Never execute a reported
	// command, or a Rig launch/recovery verb. Failed attach cannot create a seat.
	args = append(args, "--", host, "exec tmux attach-session -t '="+row.seat.TmuxTarget+"'")
	return coordinationLaunch{Provider: "rig", Target: target, Project: row.seat.RigName, Name: row.seat.LogicalID, Title: row.seat.LogicalID + " · " + host + " · " + row.seat.RigName, Host: user + "@" + host + ":" + strconv.Itoa(port), Runtime: row.seat.Runtime, Argv: args, Remote: true}, nil
}

func isRemoteRigTarget(target string) bool { return strings.HasPrefix(target, remoteRigPrefix) }
