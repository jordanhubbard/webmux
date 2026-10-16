package terminal

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
)

// A private directory belongs to one backend lifetime, so old and new servers
// cannot accidentally share authenticated masters. Keep Unix socket names short.
func SSHControlPath(directory, sessionID string) string {
	if directory == "" {
		return ""
	}
	hash := sha256.Sum256([]byte(sessionID))
	return filepath.Join(directory, hex.EncodeToString(hash[:8]))
}
