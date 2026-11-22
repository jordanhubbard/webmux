package discovery

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"

	"github.com/jordanhubbard/webmux/server/internal/storage"
)

// NodePolicy is independent of optional agent providers and applies to every
// session on this server. Policy edits take effect at server restart.
type NodePolicy struct {
	Version    int      `yaml:"version" json:"version"`
	ID         string   `yaml:"id" json:"id"`
	Browse     bool     `yaml:"browse" json:"browse"`
	Advertise  bool     `yaml:"advertise" json:"advertise"`
	Interfaces []string `yaml:"interfaces" json:"interfaces"`
}

// LoadNodePolicy assigns one random persistent identity per WEBMUX_HOME. It is
// public metadata, not a credential. Invalid existing configuration is preserved
// and refused rather than silently changing identity or broadening policy.
func LoadNodePolicy(store *storage.Store) (NodePolicy, error) {
	var p NodePolicy
	if err := store.ReadConfig("node-discovery.yaml", &p); err == nil {
		return p, validateNodePolicy(p)
	} else if !errors.Is(err, os.ErrNotExist) {
		return p, err
	}
	seed := make([]byte, 16)
	if _, err := rand.Read(seed); err != nil {
		return p, err
	}
	initial := NodePolicy{Version: 1, ID: hex.EncodeToString(seed), Browse: true, Advertise: true, Interfaces: []string{}}
	err := storage.UpdateConfigOrCreate(store, "node-discovery.yaml", initial, func(saved *NodePolicy) error { p = *saved; return validateNodePolicy(p) })
	return p, err
}

func validateNodePolicy(p NodePolicy) error {
	if p.Version != 1 || !ValidNodeID(p.ID) || len(p.Interfaces) > 32 {
		return errors.New("invalid node-discovery.yaml version, identity or interfaces")
	}
	seen := map[string]bool{}
	for _, name := range p.Interfaces {
		if name == "" || len(name) > 128 || seen[name] {
			return errors.New("invalid node-discovery.yaml interface list")
		}
		seen[name] = true
	}
	return nil
}
