package coordination

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProjectSnapshotProjectionAndValidation(t *testing.T) {
	h := fixture(t)
	target := h.inputs[1].Ref
	h.execute(t, 0, Request{ID: "snapshot-private-handoff", Action: "message", Kind: "handoff", Target: &target, Text: "private-message-content"})
	h.execute(t, 0, Request{ID: "snapshot-shared-file", Action: "claim", Kind: "file", Resource: "src/shared.go"})
	h.execute(t, 0, Request{ID: "snapshot-shared-reserve", Action: "reserve", Resource: "node:" + testNode + ":heavy"})
	view, err := h.s.OwnerView(testNode, "owner", h.inputs)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := Snapshot(view, testNode, "owner", "repository:one")
	if !snapshot.Valid(testNode, "owner", "repository:one") || len(snapshot.Members) != 2 || len(snapshot.Claims) != 1 || len(snapshot.Resources) != 1 || snapshot.Resources[0].Holder == nil {
		t.Fatal("wrong projection")
	}
	data, _ := json.Marshal(snapshot)
	if strings.Contains(string(data), "private-message-content") || strings.Contains(string(data), `"inbox"`) || strings.Contains(string(data), `"outbox"`) {
		t.Fatal("snapshot contains private messaging")
	}
	other := Snapshot(view, testNode, "owner", "repository:other")
	if len(other.Members) != 1 || len(other.Claims) != 0 || other.Resources[0].Holder != nil || !other.Valid(testNode, "owner", "repository:other") {
		t.Fatal("foreign project holder leaked")
	}
	for _, change := range []func(*ProjectSnapshot){
		func(s *ProjectSnapshot) { s.Owner = "foreign" },
		func(s *ProjectSnapshot) { s.Members[0].Ref.Owner = "foreign" },
		func(s *ProjectSnapshot) { s.Members[0].Project.ID = "unrelated" },
		func(s *ProjectSnapshot) { s.Members[0].Task = "bad\x1bcontrol" },
		func(s *ProjectSnapshot) { s.Members[0].Interface = "bad\ninterface" },
		func(s *ProjectSnapshot) { s.Claims[0].Actor.Node = strings.Repeat("b", 32) },
		func(s *ProjectSnapshot) { s.Resources[0].Authority = strings.Repeat("b", 32) },
		func(s *ProjectSnapshot) { s.Resources[0].Holder = &h.inputs[2].Ref },
		func(s *ProjectSnapshot) { s.Enabled = false },
	} {
		var copy ProjectSnapshot
		if err := json.Unmarshal(data, &copy); err != nil {
			t.Fatal(err)
		}
		change(&copy)
		if copy.Valid(testNode, "owner", "repository:one") {
			t.Fatal("accepted inconsistent remote scope")
		}
	}
	view.Enabled = false
	disabled := Snapshot(view, testNode, "owner", "repository:one")
	if !disabled.Valid(testNode, "owner", "repository:one") || len(disabled.Members) != 0 || len(disabled.Claims) != 0 || len(disabled.Resources) != 0 {
		t.Fatal("disabled policy disclosed data")
	}
}
