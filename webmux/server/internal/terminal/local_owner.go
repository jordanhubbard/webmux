package terminal

import "context"

// LocalOwnerTarget is private lifecycle state, not browser-supplied process
// selection. It contains no launch environment or credentials.
type LocalOwnerTarget struct {
	Version       int    `json:"version" yaml:"version"`
	Scope         string `json:"scope" yaml:"scope"`
	Session       string `json:"session" yaml:"session"`
	Instance      string `json:"instance" yaml:"instance"`
	Endpoint      string `json:"endpoint" yaml:"endpoint"`
	OwnerPID      int    `json:"owner_pid" yaml:"owner_pid"`
	RootPID       int    `json:"root_pid" yaml:"root_pid"`
	BinarySHA256  string `json:"binary_sha256" yaml:"binary_sha256"`
	Replay        string `json:"replay" yaml:"replay"`
	OwnerCreation uint64 `json:"owner_creation" yaml:"owner_creation"`
}

func (p *Process) LocalOwnerTarget() *LocalOwnerTarget {
	if source, ok := p.native.(interface{ LocalOwnerTarget() LocalOwnerTarget }); ok {
		target := source.LocalOwnerTarget()
		return &target
	}
	return nil
}

type LocalOwnerOperation func(context.Context, LocalOwnerTarget, string) error
