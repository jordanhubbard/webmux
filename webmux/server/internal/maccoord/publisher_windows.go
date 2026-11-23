package maccoord

import (
	"context"
	"net/url"
)

// Native Windows loopback publisher attestation is not implemented. HTTPS uses
// normal certificate verification; never send a credential to unqualified HTTP.
func qualifyPublisher(_ context.Context, origin string, _ *publisherPin) error {
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "https" {
		return ErrReadAccess
	}
	return nil
}
