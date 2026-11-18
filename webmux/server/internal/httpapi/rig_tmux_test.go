package httpapi

import (
	"reflect"
	"testing"
)

func TestRigTmuxSelectionIsLiteralAndValidated(t *testing.T) {
	socket := "/private/tmp/a space/'literal'/socket"
	got, err := rigTmuxArgs("-S", socket, "attach-session", "agents.first@project")
	want := []string{"-S", socket, "attach-session", "-t", "=agents.first@project"}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("literal socket argv: %q, %v", got, err)
	}
	for _, pair := range [][2]string{{"", socket}, {"-S", ""}, {"-S", "relative"}, {"-L", "label"}, {"-f", socket}, {"-S", "/tmp/x\nnext"}} {
		if _, err := rigTmuxArgs(pair[0], pair[1], "has-session", "seat"); err == nil {
			t.Fatalf("accepted invalid server selection: %q", pair)
		}
	}
	for _, target := range []string{"", "x;kill-server", "x\nnext", "x y"} {
		if _, err := rigTmuxArgs("", "", "has-session", target); err == nil {
			t.Fatalf("accepted unsafe target %q", target)
		}
	}
	got, err = rigTmuxArgs("", "", "has-session", "legacy")
	if err != nil || !reflect.DeepEqual(got, []string{"has-session", "-t", "=legacy"}) {
		t.Fatalf("legacy selection changed: %q, %v", got, err)
	}
}
