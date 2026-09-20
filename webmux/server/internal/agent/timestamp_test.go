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
