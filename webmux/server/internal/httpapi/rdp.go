package httpapi

import (
	"errors"
	"net"
	"os"
	"strconv"
)

// guacd is an operator-configured proxy endpoint. The remote desktop itself is
// separately resolved and validated, and only its pinned IP enters the handshake.
func (s *Server) guacdAddress() (string, error) {
	var doc struct {
		App struct {
			Guacd struct {
				Host string `yaml:"host"`
				Port int    `yaml:"port"`
			} `yaml:"guacd"`
		} `yaml:"app"`
	}
	if err := s.store.ReadConfig("app.yaml", &doc); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	host, port := doc.App.Guacd.Host, doc.App.Guacd.Port
	if host == "" {
		host = "127.0.0.1"
	}
	if port == 0 {
		port = 4822
	}
	if port < 1 || port > 65535 {
		return "", errors.New("invalid guacd port")
	}
	return net.JoinHostPort(host, strconv.Itoa(port)), nil
}
