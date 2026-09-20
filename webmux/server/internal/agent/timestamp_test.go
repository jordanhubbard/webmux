package agent

import (
	"testing"
	"time"
)

func TestEpochTimestampNodeCompatibility(t *testing.T) {
	// Captured from Node 24 Number(raw) and Date.toISOString().
	for _, test := range []struct{ raw, want string }{
		{"0.0001", "1970-01-01T00:00:00.000Z"},
		{"0.0019", "1970-01-01T00:00:00.001Z"},
		{"1", "1970-01-01T00:00:01.000Z"},
		{"  0x10  ", "1970-01-01T00:00:16.000Z"},
		{"0b11", "1970-01-01T00:00:03.000Z"},
		{"0o10", "1970-01-01T00:00:08.000Z"},
		{"253402300799.999", "9999-12-31T23:59:59.999Z"},
		{"253402300800", "+010000-01-01T00:00:00.000Z"},
		{"8640000000000", "+275760-09-13T00:00:00.000Z"},
		{"NaN", ""}, {"Infinity", ""}, {"-1", ""}, {"0", ""},
		// Out-of-range epochs remain ignored instead of throwing as Node does.
		{"8640000000000.001", ""}, {"1e300", ""},
	} {
		if got := epochISO(test.raw); got != test.want {
			t.Errorf("epochISO(%q) = %q, want %q", test.raw, got, test.want)
		}
	}
}

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

func TestLocalTimestampTransitions(t *testing.T) {
	// UTC expectations independently captured from Node 24 Date.parse.
	for _, test := range []struct{ zone, value, want string }{
		{"America/New_York", "2026-03-08T01:59:59.999", "2026-03-08T06:59:59.999Z"},
		{"America/New_York", "2026-03-08T02:00:00", "2026-03-08T07:00:00.000Z"},
		{"America/New_York", "2026-03-08T02:30:00", "2026-03-08T07:30:00.000Z"},
		{"America/New_York", "2026-03-08T03:00:00", "2026-03-08T07:00:00.000Z"},
		{"America/New_York", "2026-11-01T01:30:00", "2026-11-01T05:30:00.000Z"},
		{"America/New_York", "2026-11-01T02:00:00", "2026-11-01T07:00:00.000Z"},
		{"America/New_York", "2026-03-08 02:30:00", "2026-03-08T07:30:00.000Z"},
		{"America/New_York", "3/8/2026 02:30:00", "2026-03-08T07:30:00.000Z"},
		{"Europe/Berlin", "2026-03-29T02:30:00", "2026-03-29T01:30:00.000Z"},
		{"Europe/Berlin", "2026-10-25T02:30:00", "2026-10-25T00:30:00.000Z"},
		{"Australia/Lord_Howe", "2026-04-05T01:45:00", "2026-04-04T14:45:00.000Z"},
		{"Australia/Lord_Howe", "2026-10-04T02:15:00", "2026-10-03T15:45:00.000Z"},
		{"Pacific/Apia", "2011-12-30T12:00:00", "2011-12-30T22:00:00.000Z"},
	} {
		t.Run(test.zone+"/"+test.value, func(t *testing.T) {
			local, err := time.LoadLocation(test.zone)
			if err != nil {
				t.Fatal(err)
			}
			want, err := time.Parse(time.RFC3339Nano, test.want)
			if err != nil {
				t.Fatal(err)
			}
			if got := timestampInLocation(test.value, local); got != want.UnixMilli() {
				t.Fatalf("got %s, want %s", time.UnixMilli(got).UTC(), want)
			}
		})
	}
}
