package httpapi

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"

	"github.com/jordanhubbard/webmux/server/internal/openrigcoord"
	"github.com/jordanhubbard/webmux/server/internal/session"
)

type nativeRigAttachment struct {
	State           string `json:"state"`
	NodeID          string `json:"nodeId"`
	Target          string `json:"target"`
	TmuxServerFlag  string `json:"tmuxServerFlag"`
	TmuxServerValue string `json:"tmuxServerValue"`
}
type nativeRigReply struct {
	State           string               `json:"state"`
	Code            string               `json:"code"`
	Posted          bool                 `json:"posted"`
	OperationID     string               `json:"operationId"`
	NativeSessionID string               `json:"nativeSessionId"`
	LogicalID       string               `json:"logicalId"`
	Attachment      *nativeRigAttachment `json:"attachment"`
}

func nativeRigAttachmentArguments(saved session.RigIdentity, ref session.NativeRigRecoveryReference, a *nativeRigAttachment) ([]string, error) {
	if a == nil || a.State != "attached" || a.NodeID != ref.NodeID || a.Target != ref.SessionName || a.TmuxServerFlag != saved.TmuxServerFlag || a.TmuxServerValue != saved.TmuxServerValue {
		return nil, managedRecoveryError("Rig did not verify the original provider attachment.")
	}
	executable, err := exec.LookPath("tmux")
	if err != nil {
		return nil, err
	}
	args, err := rigTmuxArgs(a.TmuxServerFlag, a.TmuxServerValue, "attach-session", a.Target)
	if err != nil {
		return nil, err
	}
	return append([]string{executable}, args...), nil
}
func (s *Server) nativeRigOriginalAttachment(ctx context.Context, owner string, saved session.RigIdentity) ([]string, error) {
	ref, ok := session.NativeRigReference(saved)
	if !ok || s.openRigPermission(owner) != nil || s.coordinationProvider(owner, "rig") == nil {
		return nil, managedRecoveryError("The original Rig conversation is unavailable.")
	}
	payload, _ := json.Marshal(map[string]any{"identity": saved})
	output, err := openrigcoord.Run(ctx, s.store.Home, owner, "native-recovery-attachment", string(payload))
	if err != nil {
		return nil, err
	}
	var reply nativeRigReply
	if json.Unmarshal(output, &reply) != nil || reply.State != "attached" || reply.Posted || reply.NativeSessionID != ref.NativeSessionID || reply.LogicalID != ref.Pod+"."+ref.Member {
		return nil, managedRecoveryError("Rig could not verify the surviving original conversation.")
	}
	if s.openRigPermission(owner) != nil || s.coordinationProvider(owner, "rig") == nil {
		return nil, managedRecoveryError("Rig access changed.")
	}
	return nativeRigAttachmentArguments(saved, ref, reply.Attachment)
}
func (s *Server) nativeRigRecoveryOperation(ctx context.Context, owner string, saved session.RigIdentity, input session.RecoveryRequest, prepare bool) (session.NativeRigOperationResult, error) {
	ref, ok := session.NativeRigReference(saved)
	if !ok || s.openRigPermission(owner) != nil || s.coordinationProvider(owner, "rig") == nil {
		return session.NativeRigOperationResult{}, managedRecoveryError("The original Rig recovery destination is unavailable.")
	}
	payload, _ := json.Marshal(map[string]any{"identity": saved, "operationId": input.ID, "prepare": prepare, "inspect": input.Inspect && !prepare})
	output, err := openrigcoord.Run(ctx, s.store.Home, owner, "native-recovery-resume", string(payload))
	if err != nil {
		if strings.Contains(err.Error(), "native_resume_policy_changed") || strings.Contains(err.Error(), "native_resume_policy_unavailable") {
			return session.NativeRigOperationResult{}, managedRecoveryError("Codex permissions or settings changed, or could not be verified. No new Resume was sent. Continue manually with the original conversation and review its permissions.")
		}
		return session.NativeRigOperationResult{}, managedRecoveryError("Rig could not verify the original recovery: " + err.Error())
	}
	var result nativeRigReply
	if json.Unmarshal(output, &result) != nil || result.OperationID != input.ID || result.NativeSessionID != ref.NativeSessionID || result.LogicalID != ref.Pod+"."+ref.Member || !creationCode.MatchString(result.Code) {
		return session.NativeRigOperationResult{}, managedRecoveryError("Rig returned an unverified original operation.")
	}
	value := session.NativeRigOperationResult{State: result.State, Code: result.Code}
	if prepare && result.State == "prepared" && result.Code == "native_resume_prepared" && !result.Posted && result.Attachment == nil {
		return value, nil
	}
	if !prepare && result.State == "uncertain" {
		return value, nil
	}
	if !(prepare && result.State == "surviving" && result.Code == "native_original_surviving" && !result.Posted) && !(!prepare && result.State == "confirmed" && result.Posted) {
		return session.NativeRigOperationResult{}, managedRecoveryError("Rig did not confirm the original recovery operation.")
	}
	if s.openRigPermission(owner) != nil || s.coordinationProvider(owner, "rig") == nil {
		return session.NativeRigOperationResult{}, managedRecoveryError("Rig access changed during recovery.")
	}
	value.Argv, err = nativeRigAttachmentArguments(saved, ref, result.Attachment)
	return value, err
}
