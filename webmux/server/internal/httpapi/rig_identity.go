package httpapi

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/jordanhubbard/webmux/server/internal/rigobservation"
)

const localRigPrefix = "local-rig/"

// This key identifies a logical seat in this controller's configured local Rig
// namespace. Attachment addresses and native coding conversations are separate.
func localRigSeatID(rigID, logicalID string) string {
	for _, value := range []string{rigID, logicalID} {
		if value == "" || len(value) > 256 || strings.ContainsFunc(value, unicode.IsControl) {
			return ""
		}
	}
	return localRigPrefix + (rigobservation.Seat{RigID: rigID, LogicalID: logicalID}).Key()
}

func localRigIdentityCounts(raw rigStatusEnvelope) map[string]int {
	counts := map[string]int{}
	for _, seat := range raw.Seats {
		counts[localRigSeatID(seat.RigID, seat.LogicalID)]++
	}
	return counts
}

// Old clients may still supply an exact attachment address. Resolve it through
// current canonical inventory; never interpret an opaque key as a tmux target.
func resolveLocalRigSeat(raw rigStatusEnvelope, target string) (rigCanonicalSeat, error) {
	counts := localRigIdentityCounts(raw)
	addresses := map[string]int{}
	for _, seat := range raw.Seats {
		addresses[seat.CanonicalSessionName]++
	}
	var matches []rigCanonicalSeat
	for _, seat := range raw.Seats {
		id := localRigSeatID(seat.RigID, seat.LogicalID)
		if id == target || seat.CanonicalSessionName != "" && seat.CanonicalSessionName == target {
			if id == "" || counts[id] != 1 || seat.CanonicalSessionName != "" && addresses[seat.CanonicalSessionName] != 1 {
				return rigCanonicalSeat{}, fmt.Errorf("canonical Rig identity is missing or ambiguous")
			}
			matches = append(matches, seat)
		}
	}
	if len(matches) != 1 {
		return rigCanonicalSeat{}, fmt.Errorf("unknown or ambiguous Rig seat; refresh the canonical project list")
	}
	return matches[0], nil
}
