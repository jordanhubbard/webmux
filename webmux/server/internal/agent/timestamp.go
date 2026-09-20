package agent

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Match the ISO forms accepted by Node, including reduced dates, compact zone
// offsets and lowercase separators. Legacy text dates are handled separately.
var isoTimestampPattern = regexp.MustCompile(`^([+-]\d{6}|\d{4})(?:-(\d{2})(?:-(\d{2}))?)?(?:[Tt](\d{2}):(\d{2})(?::(\d{2})(?:\.(\d+))?)?([Zz]|[+-]\d{2}:?\d{2})?)?$`)

const dateLimitMillis int64 = 8640000000000000

func parseISOTimestamp(fields []string, local *time.Location) int64 {
	integer := func(index, fallback int) int {
		if fields[index] == "" {
			return fallback
		}
		n, _ := strconv.Atoi(fields[index])
		return n
	}
	year, month, day := integer(1, 0), integer(2, 1), integer(3, 1)
	hour, minute, second := integer(4, 0), integer(5, 0), integer(6, 0)
	if fields[1] == "-000000" || month < 1 || month > 12 || day < 1 || day > 31 ||
		hour > 24 || minute > 59 || second > 59 {
		return 0
	}
	fraction := fields[7]
	if hour == 24 && (minute != 0 || second != 0 || strings.Trim(fraction, "0") != "") {
		return 0
	}
	millis, _ := strconv.Atoi((fraction + "000")[:3])
	zone := fields[8]
	location := local
	if fields[4] == "" || zone == "Z" || zone == "z" {
		location = time.UTC
	}
	if len(zone) > 1 {
		digits := strings.ReplaceAll(zone[1:], ":", "")
		hours, _ := strconv.Atoi(digits[:2])
		minutes, _ := strconv.Atoi(digits[2:])
		if hours > 23 || minutes > 59 {
			return 0
		}
		offset := (hours*60 + minutes) * 60
		if zone[0] == '-' {
			offset = -offset
		}
		location = time.FixedZone("", offset)
	}
	// Date normalizes February 30 and 24:00 after checking component ranges.
	instant := time.Date(year, time.Month(month), day, hour, minute, second, millis*int(time.Millisecond), location).UnixMilli()
	if instant < -dateLimitMillis || instant > dateLimitMillis {
		return 0
	}
	return instant
}

func isoTime(value string) int64 {
	return timestampInLocation(value, time.Local)
}
func timestampInLocation(value string, local *time.Location) int64 {
	if fields := isoTimestampPattern.FindStringSubmatch(value); fields != nil {
		return parseISOTimestamp(fields, local)
	}
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02 15:04", "1/2/2006 15:04:05", "1/2/2006"} {
		if parsed, err := time.ParseInLocation(layout, value, local); err == nil {
			return parsed.UnixMilli()
		}
	}
	// Date.toString() includes a descriptive zone name in parentheses. Its
	// numeric GMT offset determines the instant, not the host's zone database.
	legacy := strings.TrimSpace(value)
	if i := strings.LastIndex(legacy, " ("); i >= 0 && strings.HasSuffix(legacy, ")") {
		legacy = legacy[:i]
	}
	for _, layout := range []string{
		"Mon, 02 Jan 2006 15:04:05 GMT",
		"Mon Jan 02 2006 15:04:05 GMT-0700",
		"January 2, 2006 15:04:05 GMT",
	} {
		if parsed, err := time.Parse(layout, legacy); err == nil {
			return parsed.UnixMilli()
		}
	}
	return 0
}
