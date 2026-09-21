package config

import (
	"math"
	"strconv"
	"strings"
)

// Number accepts the numeric forms used in JavaScript configuration values.
func Number(value string) (float64, error) {
	value = trim(value)
	if strings.Contains(value, "_") {
		return 0, strconv.ErrSyntax
	}
	if value == "" {
		return 0, nil
	}
	if len(value) > 2 && value[0] == '0' {
		base := 0
		switch value[1] {
		case 'x', 'X':
			base = 16
		case 'b', 'B':
			base = 2
		case 'o', 'O':
			base = 8
		}
		if base != 0 {
			result, err := strconv.ParseUint(value[2:], base, 64)
			return float64(result), err
		}
	}
	return strconv.ParseFloat(value, 64)
}

func GridLimit(value any, label string) (*float64, error) {
	if value == nil {
		return nil, nil
	}
	var result float64
	valid := true
	fromString := false
	switch value := value.(type) {
	case string:
		fromString = true
		trimmed := strings.ToLower(trim(value))
		switch trimmed {
		case "", "0", "none", "unlimited", "infinite":
			return nil, nil
		}
		var err error
		result, err = Number(trimmed)
		valid = err == nil
	case int:
		result = float64(value)
	case int64:
		result = float64(value)
	case uint64:
		result = float64(value)
	case float64:
		result = value
	default:
		valid = false
	}
	if valid && !math.IsNaN(result) && !math.IsInf(result, 0) && math.Trunc(result) == result {
		if result == 0 && !fromString {
			return nil, nil
		}
		if result > 0 {
			return &result, nil
		}
	}
	return nil, invalid(label + " must be a positive integer, null, 0, or unlimited")
}
