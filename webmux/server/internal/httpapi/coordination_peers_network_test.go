package httpapi

import (
	"context"
	"crypto/x509"
	"encoding/json"
	plane "github.com/jordanhubbard/webmux/server/internal/coordination"
	"github.com/jordanhubbard/webmux/server/internal/coordinationclient"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func deterministicCoordinationGit(t *testing.T) {
	t.Helper()
	// Both hosts create the same empty-root history through real Git, then use
	// independent repositories/worktrees and broker-owned ordinary child PTYs.
	t.Setenv("GIT_AUTHOR_DATE", "2026-11-21T00:00:00+00:00")
	t.Setenv("GIT_COMMITTER_DATE", "2026-11-21T00:00:00+00:00")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
}
func TestCoordinationPeerNativeNetworkClient(t *testing.T) {
	if os.Getenv("WEBMUX_NODE_NETWORK_ROLE") != "client" || os.Getenv("WEBMUX_COORDINATION_PEER_NETWORK") != "1" {
		t.Skip("explicit isolated two-host coordination component only")
	}
	deterministicCoordinationGit(t)
	dir := os.Getenv("WEBMUX_NODE_NETWORK_DIR")
	secret := nodeNetworkSecret(t, dir)
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest nodeNetworkManifest
	if err = json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	a, ah, agent, _, access := coordinationAPI(t)
	a.peerRoots = x509.NewCertPool()
	if !a.peerRoots.AppendCertsFromPEM([]byte(manifest.Certificate)) {
		t.Fatal("fixture TLS certificate missing")
	}
	federationConfig(t, a, serverDirectory{Version: 1, Servers: []serverPeer{{ID: "remote", Name: "Native coordination peer", URL: manifest.Original, Credential: secret}}})
	admin := token(t, a, "owner")
	requireStatus(t, request(ah, "POST", "/api/servers/remote/verify-identity", "{}", admin), 204)
	requireStatus(t, request(ah, "PUT", "/api/servers/remote", directoryJSON(map[string]any{"name": "Native coordination peer", "url": manifest.Original, "coordination_owner": "owner"}), admin), 204)
	view := func() plane.View {
		response := request(agent, "GET", "/view", "", access[0].Capability)
		requireStatus(t, response, 200)
		var out plane.View
		if err := json.Unmarshal(response.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	waitState := func(state string) plane.View {
		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) {
			observed := view()
			if len(observed.Peers) == 1 && observed.Peers[0].State == state {
				return observed
			}
			time.Sleep(200 * time.Millisecond)
		}
		t.Fatal("native background refresh did not reach expected peer state", state)
		return plane.View{}
	}
	if got := waitState("unauthorized"); got.Peers[0].Snapshot != nil {
		t.Fatal("native terminal-only grant disclosed coordination")
	}
	control := func(path string) {
		peer := pinnedPeer(t, a)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.Proxy = nil
		transport.ForceAttemptHTTP2 = false
		transport.DialTLSContext = a.nodePeerDialer(peer, secret)
		defer transport.CloseIdleConnections()
		req, _ := http.NewRequestWithContext(ctx, "POST", manifest.Original+path, nil)
		req.Header = nodeScopeHeaders(peer, secret)
		response, err := (&http.Client{Transport: transport}).Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != 204 {
			t.Fatal("native fixture control refused", response.StatusCode)
		}
	}
	control("/_qualification/share-coordination")
	current := waitState("current")
	if len(current.Peers) != 1 || current.Peers[0].State != "current" || current.Peers[0].Snapshot == nil || len(current.Peers[0].Snapshot.Members) != 2 || current.Self == nil || current.Self.Project == nil || !current.Peers[0].Snapshot.Valid(manifest.NodeID, "owner", current.Self.Project.ID) {
		t.Fatal("native cross-host verified project/account discovery failed")
	}
	foreign := request(agent, "GET", "/view", "", access[2].Capability)
	requireStatus(t, foreign, 200)
	var isolated plane.View
	if json.Unmarshal(foreign.Body.Bytes(), &isolated) != nil || len(isolated.Peers) != 0 {
		t.Fatal("foreign account borrowed native peer credential")
	}
	if os.Getenv("WEBMUX_COORDINATION_MESSAGE_NETWORK") == "1" {
		node, err := a.coordinationNode()
		if err != nil {
			t.Fatal(err)
		}
		requireStatus(t, request(ah, "PUT", "/api/servers/remote", directoryJSON(map[string]any{"name": "Native coordination peer", "url": manifest.Original, "coordination_messages": true}), admin), 204)
		control("/_qualification/allow-messages?source_node=" + node)
		deadline := time.Now().Add(20 * time.Second)
		var ready plane.View
		for time.Now().Before(deadline) {
			ready = view()
			if len(ready.Peers) == 1 && ready.Peers[0].Snapshot != nil && ready.Peers[0].Snapshot.Messaging {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if len(ready.Peers) != 1 || ready.Peers[0].Snapshot == nil || !ready.Peers[0].Snapshot.Messaging {
			t.Fatal("native message permission not observed")
		}
		target := ready.Peers[0].Snapshot.Members[1].Ref
		intent := plane.Request{ID: "native-cross-host-message", Action: "message", Kind: "handoff", Target: &target, Text: "Native remote inbox: review the ownership change."}
		sent := request(agent, "POST", "/act", directoryJSON(intent), access[0].Capability)
		requireStatus(t, sent, 200)
		var result plane.Result
		if json.Unmarshal(sent.Body.Bytes(), &result) != nil || result.Transport != "pending" {
			t.Fatal("native outbox admission failed", sent.Body.String())
		}
		pair := &coordinationPair{a: a, agent: agent, access: access, remote: []coordinationclient.Access{{Subject: target}}, secret: secret}
		coordinationMessageWait(t, pair, result.Record, "admitted", "remote-admitted")
		control("/_qualification/complete-message?record=" + result.Record + "&session=" + target.Session)
		coordinationMessageWait(t, pair, result.Record, "completed", "remote-admitted")
		repeat := request(agent, "POST", "/act", directoryJSON(intent), access[0].Capability)
		requireStatus(t, repeat, 200)
		if repeat.Body.String() != sent.Body.String() {
			t.Fatal("native exact intent retry changed admission")
		}

		if os.Getenv("WEBMUX_COORDINATION_CLAIM_NETWORK") == "1" {
			claim := plane.Request{ID: "native-overlap-file", Action: "claim", Kind: "file", Resource: "native-overlap/file.go"}
			response := request(agent, "POST", "/act", directoryJSON(claim), access[0].Capability)
			requireStatus(t, response, 200)
			var initial plane.Result
			if json.Unmarshal(response.Body.Bytes(), &initial) != nil || initial.Status != "claimed" || len(initial.Conflicts) != 0 || len(initial.PeerClaims) != 1 || initial.PeerClaims[0].State != "current" {
				t.Fatal("native initial claim scope failed", response.Body.String())
			}
			control("/_qualification/claim-overlap?session=" + target.Session)
			incoming, outgoing := waitPeerOverlap(t, pair)
			if incoming.Transport != "peer-observation" || incoming.Overlap == nil {
				t.Fatal("native local event provenance missing")
			}
			coordinationMessageWait(t, pair, outgoing.ID, "admitted", "remote-admitted")
			control("/_qualification/complete-message?record=" + outgoing.ID + "&session=" + target.Session)
			coordinationMessageWait(t, pair, outgoing.ID, "completed", "remote-admitted")
			claim.ID = "native-overlap-check"
			response = request(agent, "POST", "/act", directoryJSON(claim), access[0].Capability)
			requireStatus(t, response, 200)
			var checked plane.Result
			if json.Unmarshal(response.Body.Bytes(), &checked) != nil || checked.Record != initial.Record || len(checked.Conflicts) != 1 || checked.Conflicts[0].Actor != target {
				t.Fatal("native fresh claim conflict not reported", response.Body.String())
			}
			repeat := request(agent, "POST", "/act", directoryJSON(claim), access[0].Capability)
			requireStatus(t, repeat, 200)
			if repeat.Body.String() != response.Body.String() {
				t.Fatal("native claim retry changed result")
			}
			t.Log("native cross-host claims passed: concurrent claims before refresh, browser-independent overlap reconciliation, local observation and one remote notice, explicit recipient completion, fresh conflict check and frozen retry; ordinary PTY fixtures, not real coding-agent acceptance")
		}
		t.Log("native two-host message passed: durable admission, lost reply, status reconciliation, explicit recipient receipts, exact request retry; ordinary PTY child fixtures, not LLM agent pair acceptance")
	}

	if os.Getenv("WEBMUX_COORDINATION_RESERVATION_NETWORK") == "1" {
		node, err := a.coordinationNode()
		if err != nil {
			t.Fatal(err)
		}
		deniedRequest := plane.Request{ID: "native-reservation-disabled", Action: "reserve", Resource: "node:" + manifest.NodeID + ":heavy"}
		response := request(agent, "POST", "/act", directoryJSON(deniedRequest), access[0].Capability)
		requireStatus(t, response, 200)
		var denied plane.Result
		if json.Unmarshal(response.Body.Bytes(), &denied) != nil || denied.Status != "peer-unavailable" {
			t.Fatal("native summary/message scope acquired reservation authority", response.Body.String())
		}
		requireStatus(t, request(ah, "PUT", "/api/servers/remote", directoryJSON(map[string]any{"name": "Native coordination peer", "url": manifest.Original, "coordination_reservations": true}), admin), 204)
		control("/_qualification/allow-reservations?source_node=" + node)
		deadline := time.Now().Add(20 * time.Second)
		ready := false
		for time.Now().Before(deadline) {
			v := view()
			if len(v.Peers) == 1 && v.Peers[0].Snapshot != nil && v.Peers[0].Snapshot.Reservations {
				ready = true
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if !ready {
			t.Fatal("native shared reservation permission not observed")
		}
		pair := &coordinationPair{a: a, agent: agent, access: access, secret: secret}
		intent := deniedRequest
		intent.ID = "native-remote-worker-reserve"
		pending := sharedHTTPAct(t, pair, intent)
		if pending.Status != "reservation-pending" || pending.Shared == nil || pending.Shared.Transport != "uncertain" || pending.Lease != nil {
			t.Fatal("lost native grant reply did not remain uncertain", pending)
		}
		granted := sharedHTTPAct(t, pair, intent)
		if granted.Status != "reserved" || granted.Fence != 1 || granted.Lease == nil || !granted.Lease.Current {
			t.Fatal("native exact grant recovery failed", granted)
		}
		control("/_qualification/local-worker?phase=contend")
		renewal := plane.Request{ID: "native-remote-worker-renew", Action: "renew-reservation", Resource: intent.Resource, Record: granted.Record, Fence: granted.Fence}
		renewed := sharedHTTPAct(t, pair, renewal)
		if renewed.Status != "renewed" || renewed.Fence != 1 || renewed.Record != granted.Record || renewed.Lease == nil || !renewed.Lease.Current {
			t.Fatal("native original-fence renewal failed", renewed)
		}
		released := sharedHTTPAct(t, pair, plane.Request{ID: "native-remote-worker-release", Action: "release-reservation", Resource: intent.Resource, Record: granted.Record, Fence: granted.Fence})
		if released.Status != "released" {
			t.Fatal("native release failed", released)
		}
		old := sharedHTTPAct(t, pair, intent)
		if old.Status != "reserved" || old.Lease == nil || old.Lease.Current || !old.Expires.Equal(*granted.Expires) {
			t.Fatal("native old receipt revived a released grant", old)
		}
		control("/_qualification/local-worker?phase=after")
		renewal.ID = "native-stale-worker-renew"
		lost := sharedHTTPAct(t, pair, renewal)
		if lost.Status != "lease-lost" {
			t.Fatal("native stale remote fence renewed", lost)
		}
		t.Log("native shared worker protocol passed: explicit separate permission, lost grant reply recovered by status, local/remote contention, original-fence renewal, release, historical receipt refused, local fence advancement and stale remote renewal refusal; ordinary PTY/API fixtures, not real coding-agent job acceptance")
	}
	control("/_qualification/revoke-coordination")
	if got := waitState("unauthorized").Peers[0]; got.Snapshot != nil {
		t.Fatal("native revocation retained data")
	}
	control("/_qualification/stop")
	t.Log("native two-host background pinned HTTPS project discovery passed (no browser or manual refresh): terminal-only refusal, explicit node/account sharing, two same-project remote PTY members, foreign-account isolation and revocation; no native coding-agent acceptance")
}
