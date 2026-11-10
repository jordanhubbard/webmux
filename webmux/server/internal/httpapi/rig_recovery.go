package httpapi

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jordanhubbard/webmux/server/internal/openrigcoord"
	"github.com/jordanhubbard/webmux/server/internal/session"
)

func (s *Server) captureRigRecovery(ctx context.Context, owner, id string) error {
	return s.sessions.CaptureManagedRecovery(ctx, owner, id, func(ctx context.Context, identity session.RigIdentity) (session.RigIdentity, error) {
		if s.openRigPermission(owner) != nil || s.coordinationProvider(owner, "rig") == nil {
			return identity, fmt.Errorf("Rig access is unavailable")
		}
		input, err := json.Marshal(identity)
		if err != nil {
			return identity, err
		}
		output, err := openrigcoord.Run(ctx, s.store.Home, owner, "recovery-observe", string(input))
		if err != nil {
			return identity, err
		}
		var observed session.RigIdentity
		if err := json.Unmarshal(output, &observed); err != nil {
			return identity, fmt.Errorf("Rig recovery metadata is invalid")
		}
		if s.openRigPermission(owner) != nil || s.coordinationProvider(owner, "rig") == nil {
			return identity, fmt.Errorf("Rig access changed")
		}
		return observed, nil
	})
}
