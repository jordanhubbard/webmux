package terminal

import (
	"errors"
	"regexp"
)

var hostname = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9._-]*[a-zA-Z0-9.])?$|^\[[0-9a-fA-F:]+\]$`)
var username = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

// ValidateSSHAddress applies the terminal launcher's address contract at every
// SSH helper boundary too. Stored sessions can retain an invalid failed launch;
// presence in the broker is not evidence that an address passed validation.
// Port zero retains the existing default-port convention.
func ValidateSSHAddress(host, user string, port int) error {
	if host == "" {
		return errors.New("Hostname is required")
	}
	if len(host) > 255 {
		return errors.New("Hostname too long")
	}
	if !hostname.MatchString(host) {
		return errors.New("Invalid hostname: " + host)
	}
	if len(user) > 64 {
		return errors.New("Username too long")
	}
	if user != "" && !username.MatchString(user) {
		return errors.New("Invalid username: " + user)
	}
	if port != 0 && (port < 1 || port > 65535) {
		return errors.New("port must be an integer between 1 and 65535")
	}
	return nil
}
