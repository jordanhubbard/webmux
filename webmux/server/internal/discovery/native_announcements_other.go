//go:build !darwin

package discovery

import (
	"context"
	"io"
	"net"

	"github.com/pion/mdns/v2"
)

const nativeNodeAnnouncements = false

func startNativeNodeAnnouncements(context.Context, []mdns.ServiceInstance, []net.Interface, func(error)) (io.Closer, error) {
	panic("native node announcements are only available on macOS")
}
