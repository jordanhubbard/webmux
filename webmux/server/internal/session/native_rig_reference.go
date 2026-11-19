package session

import (
	"bytes"
	"encoding/json"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/jordanhubbard/webmux/server/internal/recovery"
	"github.com/jordanhubbard/webmux/server/internal/rigobservation"
)

// NativeRigRecoveryReference is evidence captured from the original native
// registration/generation/snapshot. The browser never supplies this record.
type NativeRigRecoveryReference struct {
	Contract             string `json:"contract"`
	RigID                string `json:"rigId"`
	NodeID               string `json:"nodeId"`
	Pod                  string `json:"pod"`
	Member               string `json:"member"`
	Cwd                  string `json:"cwd"`
	SessionName          string `json:"sessionName"`
	SnapshotID           string `json:"snapshotId"`
	HistorySessionID     string `json:"historySessionId"`
	NativeSessionID      string `json:"nativeSessionId"`
	NativeStateFile      string `json:"nativeStateFile"`
	NativeRecordIdentity string `json:"nativeRecordIdentity"`
	SnapshotSHA256       string `json:"snapshotSHA256"`
	PlanFingerprint      string `json:"planFingerprint"`
}

func NativeRigReference(value RigIdentity) (NativeRigRecoveryReference, bool) {
	var ref NativeRigRecoveryReference
	if value.NativeRigRecovery == "" || len(value.NativeRigRecovery) > 32768 {
		return ref, false
	}
	decoder := json.NewDecoder(bytes.NewBufferString(value.NativeRigRecovery))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&ref) != nil || decoder.Decode(new(any)) != io.EOF {
		return ref, false
	}
	clean := func(s string) bool { return s != "" && len(s) <= 4096 && !strings.ContainsFunc(s, unicode.IsControl) }
	socket := value.TmuxServerFlag == "-S" && clean(value.TmuxServerValue) && len(value.TmuxServerValue) <= 2048 && filepath.IsAbs(value.TmuxServerValue)
	valid := ref.Contract == "webmux-rig-native-codex-recovery/v1" && localRigProvider(value.Provider) && value.Host == "localhost" && value.Runtime == "codex" && socket &&
		value.NativeSource == "rig-native-codex" && value.Target == ref.SessionName && value.AttachmentTarget == ref.SessionName &&
		value.SeatID == "local-rig/"+(rigobservation.Seat{RigID: ref.RigID, LogicalID: ref.Pod + "." + ref.Member}).Key() &&
		value.Workspace == ref.Cwd && filepath.IsAbs(ref.Cwd) && clean(ref.Cwd) && value.NativeSessionID == ref.NativeSessionID && recovery.ValidNativeID("codex", ref.NativeSessionID) &&
		value.NativeStateFile == ref.NativeStateFile && filepath.IsAbs(ref.NativeStateFile) && clean(ref.NativeStateFile) &&
		regexp.MustCompile(`^[a-z][a-z0-9-]{0,47}$`).MatchString(ref.Pod) && regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`).MatchString(ref.Member) &&
		regexp.MustCompile(`^[a-zA-Z0-9_.:@-]{1,256}$`).MatchString(ref.SessionName) &&
		rigRecoveryULID.MatchString(ref.RigID) && rigRecoveryULID.MatchString(ref.NodeID) && rigRecoveryULID.MatchString(ref.SnapshotID) && rigRecoveryULID.MatchString(ref.HistorySessionID) &&
		rigRecoveryHash.MatchString(ref.NativeRecordIdentity) && rigRecoveryHash.MatchString(ref.SnapshotSHA256) && rigRecoveryHash.MatchString(ref.PlanFingerprint)
	return ref, valid
}

func nativeRigPending(state *recovery.State) bool {
	return state != nil && state.Intent != nil && state.Intent.ManagedRequest != "" && (state.Intent.Outcome == "requested" || state.Intent.Outcome == "uncertain")
}

// Only live, logged local Codex viewers can produce new conversation evidence.
// The existing integration heartbeat owns scheduling; this method starts no job.
func (b *Broker) NativeRigCaptureCandidates() []Session {
	b.mu.Lock()
	defer b.mu.Unlock()
	var values []Session
	if b.closed {
		return values
	}
	for _, id := range b.order {
		e := b.entries[id]
		v := e.value
		if v.Rig != nil && localRigProvider(v.Rig.Provider) && v.Rig.Host == "localhost" && v.Rig.Runtime == "codex" && v.Rig.TmuxServerFlag != "" &&
			e.run != nil && e.run.log.active() && !nativeRigPending(v.Recovery) {
			values = append(values, v.clone())
		}
	}
	return values
}
