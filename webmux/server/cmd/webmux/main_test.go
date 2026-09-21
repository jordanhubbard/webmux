package main

import "testing"

func TestSlavePortNumericFormsAndInvalidValues(t *testing.T) {
	for raw, want := range map[string]int{"": 0, " ": 0, "0": 0, "22": 22, " 3389 ": 3389, "0x16": 22, "2.2e1": 22} {
		got, err := parseSlavePort(raw)
		if err != nil || got != want {
			t.Fatal(raw, got, err)
		}
	}
	for _, raw := range []string{"NaN", "Infinity", "-1", "65536", "1.5", "garbage"} {
		if _, err := parseSlavePort(raw); err == nil {
			t.Fatal("invalid port accepted", raw)
		}
	}
}
