package httpapi

import (
	"context"
	"encoding/json"

	"github.com/jordanhubbard/webmux/server/internal/openrigcoord"
	"github.com/jordanhubbard/webmux/server/internal/session"
)

func (p *rigProvider) PlanNativeRecovery(ctx context.Context, saved session.RigIdentity) (func(context.Context) ([]string, error), error) {
	input, err := json.Marshal(saved)
	if err != nil {
		return nil, err
	}
	call := func(ctx context.Context, action string) error {
		output, err := openrigcoord.Run(ctx, p.home, p.owner, action, string(input))
		if err != nil {
			return managedRecoveryError("Rig could not verify recovery: " + err.Error())
		}
		var result struct {
			State    string `json:"state"`
			Outcome  string `json:"outcome"`
			Target   string `json:"target"`
			NativeID string `json:"native_session_id"`
		}
		if json.Unmarshal(output, &result) != nil || result.Target != saved.Target || result.NativeID != saved.NativeSessionID {
			return managedRecoveryError("Rig returned an unverified recovery destination.")
		}
		if action == "recovery-prepare" && result.State != "surviving" && result.State != "missing" || action == "recovery-resume" && result.Outcome != "reattached" && result.Outcome != "resumed" {
			return managedRecoveryError("Rig did not confirm this recovery operation.")
		}
		return nil
	}
	if err := call(ctx, "recovery-prepare"); err != nil {
		return nil, err
	}
	return func(ctx context.Context) ([]string, error) {
		if err := call(ctx, "recovery-resume"); err != nil {
			return nil, err
		}
		launch, err := p.Attach(ctx, saved.Target)
		if err != nil {
			return nil, err
		}
		if launch.Provider != "rig" || launch.Target != saved.Target || launch.Host != saved.Host || launch.Workspace != saved.Workspace || launch.Runtime != saved.Runtime || launch.Profile != "" && launch.Profile != saved.ManagerProfile {
			return nil, managedRecoveryError("The Rig viewer destination changed after recovery.")
		}
		return launch.Argv, nil
	}, nil
}
