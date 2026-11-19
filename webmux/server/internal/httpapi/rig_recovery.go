package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/openrigcoord"
	"github.com/jordanhubbard/webmux/server/internal/session"
)

func (s *Server) captureRigRecovery(ctx context.Context, owner, id string) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	return s.sessions.CaptureManagedRecovery(ctx, owner, id, func(ctx context.Context, identity session.RigIdentity) (session.RigIdentity, error) {
		if identity.Host == "" {
			return identity, fmt.Errorf("Rig execution host is unverified; use its lifecycle manager on the original host")
		}
		if isRemoteRigTarget(identity.Target) {
			return identity, fmt.Errorf("remote Rig recovery belongs to the execution host")
		}
		if s.openRigPermission(owner) != nil || s.coordinationProvider(owner, "rig") == nil {
			return identity, fmt.Errorf("Rig access is unavailable")
		}
		input, err := json.Marshal(identity)
		if err != nil {
			return identity, err
		}
		action := "recovery-observe"
		nativeCodex := identity.Host == "localhost" && identity.Runtime == "codex" && identity.TmuxServerFlag != ""
		if nativeCodex {
			action = "native-recovery-observe"
		}
		output, err := openrigcoord.Run(ctx, s.store.Home, owner, action, string(input))
		if err != nil {
			return identity, err
		}
		var observed session.RigIdentity
		if nativeCodex {
			var reference session.NativeRigRecoveryReference
			if json.Unmarshal(output, &reference) != nil {
				return identity, fmt.Errorf("Rig native recovery reference is invalid")
			}
			encoded, _ := json.Marshal(reference)
			observed = identity
			observed.NativeRigRecovery = string(encoded)
			observed.NativeSessionID, observed.NativeStateFile, observed.NativeSource = reference.NativeSessionID, reference.NativeStateFile, "rig-native-codex"
			observed.ManagerRigID, observed.ManagerNodeID, observed.ManagerLogicalID = reference.RigID, reference.NodeID, reference.Pod+"."+reference.Member
			observed.ManagerSnapshotID, observed.ManagerSnapshotHash = reference.SnapshotID, reference.SnapshotSHA256
			observed.ManagerPolicyHash, observed.ManagerRecordIdentity = reference.PlanFingerprint, reference.NativeRecordIdentity
			observed.ManagerProcess = ""
		} else if err := json.Unmarshal(output, &observed); err != nil {
			return identity, fmt.Errorf("Rig recovery metadata is invalid")
		}
		if s.openRigPermission(owner) != nil || s.coordinationProvider(owner, "rig") == nil {
			return identity, fmt.Errorf("Rig access changed")
		}
		return observed, nil
	})
}

// Runs in the existing coordination heartbeat, with a shared round deadline and
// a rotating start so one unavailable seat cannot starve the others.
func (s *Server) captureNativeRigConversations(parent context.Context) {
	values := s.sessions.NativeRigCaptureCandidates()
	if len(values) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()
	start := s.rigCaptureCursor % len(values)
	for offset := 0; offset < len(values) && ctx.Err() == nil; offset++ {
		index := (start + offset) % len(values)
		s.rigCaptureCursor = (index + 1) % len(values)
		value := values[index]
		if err := s.captureRigRecovery(ctx, value.Owner, value.ID); err != nil && ctx.Err() == nil {
			s.logger.Debug("native Rig conversation capture unavailable", "session_id", value.ID)
		}
	}
}
