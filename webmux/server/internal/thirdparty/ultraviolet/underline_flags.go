package uv

import "errors"

// UnderlineFlags retains renderer flag words independently of raw underline
// style/color and link data. Its zero value uses ordinary style-derived flags.
// The private representation is copied by value with cells and saved pens.
// This WebMux extension is opt-in; it does not change ordinary uv styles.
type UnderlineFlags struct{ code uint8 }

func NewUnderlineFlags(enabled, extended bool) UnderlineFlags {
	code := uint8(1)
	if enabled {
		code++
	}
	if extended {
		code += 2
	}
	return UnderlineFlags{code: code}
}

func (f UnderlineFlags) IsSet() bool    { return f.code != 0 }
func (f UnderlineFlags) Enabled() bool  { return f.code == 2 || f.code == 4 }
func (f UnderlineFlags) Extended() bool { return f.code >= 3 }

// Code is the bounded wire representation: zero is unspecified; 1..4 are flags.
func (f UnderlineFlags) Code() uint8 { return f.code }
func UnderlineFlagsFromCode(code uint8) (UnderlineFlags, bool) {
	if code > 4 {
		return UnderlineFlags{}, false
	}
	return UnderlineFlags{code: code}, true
}

// VisibleStyle projects retained renderer state for ordinary visual rendering.
// Snapshot encoders must preserve the original flags and raw extended values.
func (s Style) VisibleStyle() Style {
	f := s.UnderlineFlags
	if !f.IsSet() {
		return s
	}
	if !f.Enabled() {
		s.Underline = UnderlineNone
	} else if !f.Extended() {
		s.Underline = UnderlineSingle
	}
	if !f.Extended() {
		s.UnderlineColor = nil
	}
	s.UnderlineFlags = UnderlineFlags{}
	return s
}

func (c *Cell) VisibleLink() Link {
	if c.Style.UnderlineFlags.IsSet() && !c.Style.UnderlineFlags.Extended() {
		return Link{}
	}
	return c.Link
}

// MarshalJSON keeps the optional metadata bounded when a caller serializes Style.
func (f UnderlineFlags) MarshalJSON() ([]byte, error) { return []byte{byte('0' + f.code)}, nil }
func (f *UnderlineFlags) UnmarshalJSON(data []byte) error {
	if len(data) != 1 || data[0] < '0' || data[0] > '4' {
		return errors.New("invalid underline flag code")
	}
	f.code = data[0] - '0'
	return nil
}
