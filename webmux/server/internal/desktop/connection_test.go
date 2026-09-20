package desktop

import (
	"errors"
	"testing"
)

func TestConnectionOwnershipAndConcurrentStates(t *testing.T) {
	b, _ := fixture(t, VNC)
	s := create(t, b, "owner")
	if _, err := b.Connect("other", s.ID); !errors.Is(err, ErrForbidden) {
		t.Fatal(err)
	}
	first, err := b.Connect("owner", s.ID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := b.Connect("owner", s.ID)
	if err != nil {
		t.Fatal(err)
	}
	first.SetState("connected")
	second.SetState("connected")
	first.Finish("disconnected")
	first.SetState("error")
	value, _ := b.Get("owner", s.ID)
	if value.State != "connected" {
		t.Fatal("old connection changed active viewer", value)
	}
	if first.Password != "" {
		t.Fatal("closed connection retained password")
	}
	second.Finish("disconnected")
	value, _ = b.Get("owner", s.ID)
	if value.State != "disconnected" {
		t.Fatal(value)
	}
	third, err := b.Connect("owner", s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Delete("owner", s.ID); err != nil {
		t.Fatal(err)
	}
	third.SetState("connected")
	third.Finish("error")
	if _, err := b.Get("owner", s.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}
