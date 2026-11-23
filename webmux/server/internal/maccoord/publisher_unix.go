//go:build !windows

package maccoord

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func qualifyPublisher(parent context.Context, origin string, pin *publisherPin) error {
	u, err := url.Parse(origin)
	if err != nil {
		return ErrReadAccess
	}
	if u.Scheme == "https" {
		return nil
	}
	if pin == nil || u.Port() == "" || !validOrigin(origin) {
		return ErrReadAccess
	}
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "lsof", "-nP", "-iTCP:"+u.Port(), "-sTCP:LISTEN", "-Fpu").Output()
	if err != nil {
		return ErrReadAccess
	}
	pid, uid := false, false
	for _, line := range strings.Split(string(output), "\n") {
		if strings.HasPrefix(line, "p") {
			if line != "p"+strconv.Itoa(pin.PID) {
				return ErrReadAccess
			}
			pid = true
		}
		if strings.HasPrefix(line, "u") {
			if line != "u"+strconv.Itoa(os.Getuid()) {
				return ErrReadAccess
			}
			uid = true
		}
	}
	if !pid || !uid {
		return ErrReadAccess
	}
	identity, err := exec.CommandContext(ctx, "ps", "-p", strconv.Itoa(pin.PID), "-o", "pid=,uid=,lstart=,comm=").Output()
	if err != nil || len(identity) == 0 {
		return ErrReadAccess
	}
	sum := sha256.Sum256(identity)
	if hex.EncodeToString(sum[:]) != pin.Fingerprint {
		return ErrReadAccess
	}
	return nil
}
