//go:build !linux

package main

import (
	"context"
	"net"
	"time"
)

func clientDialContext() func(context.Context, string, string) (net.Conn, error) {
	return (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext
}
