package agent

import (
	"testing"
	"time"
)

func TestLegacyTimestampInstants(t *testing.T) {
	// Expected instants were also checked with Node 24 Date.parse. Use a fixed
	// local zone to catch accidentally interpreting zone-less datetimes as UTC.
	local := time.FixedZone("fixture", -7*60*60)
	for _, test := range []struct{ value, want string }{
		{"2026-09-20", "2026-09-20T00:00:00Z"},
		{"2026-09-20T12:34:56", "2026-09-20T19:34:56Z"},
		{"2026-09-20T12:34", "2026-09-20T19:34:00Z"},
		{"2026-09-20 12:34:56", "2026-09-20T19:34:56Z"},
		{"09/20/2026 12:34:56", "2026-09-20T19:34:56Z"},
		{"9/20/2026", "2026-09-20T07:00:00Z"},
		{"2026-09-20T12:34:56+0530", "2026-09-20T07:04:56Z"},
		{"2026-09-20T12:34+05:30", "2026-09-20T07:04:00Z"},
		{"Sun, 20 Sep 2026 12:34:56 GMT", "2026-09-20T12:34:56Z"},
		{"Sun Sep 20 2026 12:34:56 GMT+0530 (India Standard Time)", "2026-09-20T07:04:56Z"},
		{"Sun Sep 20 2026 12:34:56 GMT-0700 (Pacific Daylight Time)", "2026-09-20T19:34:56Z"},
		{"September 20, 2026 12:34:56 GMT", "2026-09-20T12:34:56Z"},
		{"2026-09-20T12:34:56.123456Z", "2026-09-20T12:34:56.123Z"},
	} {
		t.Run(test.value, func(t *testing.T) {
			want, err := time.Parse(time.RFC3339Nano, test.want)
			if err != nil {
				t.Fatal(err)
			}
			if got := timestampInLocation(test.value, local); got != want.UnixMilli() {
				t.Fatalf("timestamp %d, want %d", got, want.UnixMilli())
			}
		})
	}
	for _, invalid := range []string{"", "not a date", "2026-13-20", "2026-09-20T25:00:00", "20/09/2026", "Sun Sep 20 2026 12:34:56 Unknown"} {
		if got := timestampInLocation(invalid, local); got != 0 {
			t.Fatalf("accepted %q: %d", invalid, got)
		}
	}
}

func TestISOTimestampNodeCompatibility(t *testing.T) {
	// Millisecond expectations come from Node 24 Date.parse, including the
	// representable range endpoints and normalization of calendar overflow.
	for _, test := range []struct {
		value string
		want  int64
	}{
		{"2026", 1767225600000},
		{"2026-09", 1788220800000},
		{"2026T12:00Z", 1767268800000},
		{"2026-09T12:00Z", 1788264000000},
		{"2026-02-30T12:00Z", 1772452800000},
		{"2026-02-29", 1772323200000},
		{"2026-09-20T24:00:00Z", 1789948800000},
		{"2026-09-20T24:00:00.0000Z", 1789948800000},
		{"2026-09-20t12:00:00z", 1789905600000},
		{"+010000-01-01T00:00:00Z", 253402300800000},
		{"+275760-09-13T00:00:00Z", 8640000000000000},
		{"-271821-04-20T00:00:00Z", -8640000000000000},
		{"1969-12-31T23:59:59.9999Z", -1},
	} {
		if got := timestampInLocation(test.value, time.UTC); got != test.want {
			t.Errorf("%s: got %d, want %d", test.value, got, test.want)
		}
	}
	for _, invalid := range []string{
		"2026-09-20T24:00:00.0001Z", "2026-09-20T24:00:01Z",
		"2026-09-20T24:01:00Z", "2026-09-20T25:00Z",
		"2026-09-20T12:00:00,1Z", "2026-09-20T12:60:00Z",
		"2026-09-20T12:00:60Z", "2026-09-20T12:00:00+24:00",
		"2026-09-20T12:00:00+2360", "2026-00-01", "2026-13-01",
		"2026-01-00", "2026-01-32", "-000000-01-01T00:00:00Z",
		"+275760-09-13T00:00:00.001Z", "-271821-04-19T23:59:59.999Z",
	} {
		if got := timestampInLocation(invalid, time.UTC); got != 0 {
			t.Errorf("accepted %q: %d", invalid, got)
		}
	}
}
