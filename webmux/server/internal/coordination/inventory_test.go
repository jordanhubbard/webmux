package coordination

import (
	"reflect"
	"testing"
	"time"
)

func TestProjectInventoryRetainsRevokedIncarnationsWithoutRefreshingAuthority(t *testing.T) {
	h := fixture(t)
	before, err := h.s.ProjectInventory("owner", h.inputs[0].Project)
	if err != nil || len(before) != 2 {
		t.Fatal(before, err)
	}
	if err := h.s.Sync(testNode, h.inputs[1:]); err != nil {
		t.Fatal(err)
	}
	retained, err := h.s.ProjectInventory("owner", h.inputs[0].Project)
	if err != nil || len(retained) != 2 {
		t.Fatal(retained, err)
	}
	revoked := 0
	for _, member := range retained {
		if member.Revoked != nil {
			revoked++
			if member.Present || member.Active {
				t.Fatal("revoked session is live", member)
			}
		}
	}
	if revoked != 1 {
		t.Fatal("missing retained tombstone", retained)
	}
	h.now = h.now.Add(time.Minute)
	again, err := h.s.ProjectInventory("owner", h.inputs[0].Project)
	if err != nil || !reflect.DeepEqual(again, retained) {
		t.Fatal("read refreshed membership", again, err)
	}
	foreign, err := h.s.ProjectInventory("foreign", h.inputs[0].Project)
	if err != nil || len(foreign) != 1 || foreign[0].Ref.Owner != "foreign" {
		t.Fatal("inventory crossed account boundary", foreign, err)
	}
}
