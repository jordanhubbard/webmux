// Command webmux is the native Go server. During migration use a separate home
// and port; the Node server remains the supported default until parity passes.
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/httpapi"
	"github.com/jordanhubbard/webmux/server/internal/storage"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run() (resultErr error) {
	root := flag.String("root", envDefault("WEBMUX_ROOT", "."), "installation directory containing config.defaults")
	home := flag.String("home", os.Getenv("WEBMUX_HOME"), "writable configuration/data directory")
	listen := flag.String("listen", "", "override HTTP listen address (e.g. 127.0.0.1:18080)")
	flag.Parse()
	if *home == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		*home = filepath.Join(userHome, ".config", "webmux")
	}
	store, err := storage.Open(*home, filepath.Join(*root, "config.defaults"))
	if err != nil {
		return err
	}
	var config struct {
		App struct {
			Name       string `yaml:"name"`
			ListenHost string `yaml:"listen_host"`
			HTTPPort   int    `yaml:"http_port"`
			HTTPSPort  int    `yaml:"https_port"`
			SecureMode bool   `yaml:"secure_mode"`
		} `yaml:"app"`
	}
	if err := store.ReadConfig("app.yaml", &config); err != nil {
		return fmt.Errorf("load app config: %w", err)
	}
	httpPort, err := port("HTTP_PORT", config.App.HTTPPort)
	if err != nil {
		return err
	}
	httpsPort, err := port("HTTPS_PORT", config.App.HTTPSPort)
	if err != nil {
		return err
	}
	api, err := httpapi.New(store, httpapi.Options{Name: config.App.Name, SecureMode: config.App.SecureMode, JWTSecret: os.Getenv("JWT_SECRET")})
	if err != nil {
		return fmt.Errorf("initialize API: %w", err)
	}
	defer func() {
		if err := api.Close(); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("close sessions: %w", err))
		}
	}()
	var certificate *tls.Certificate
	certFile, keyFile := store.ConfigPath("tls/cert.pem"), store.ConfigPath("tls/key.pem")
	_, certErr := os.Stat(certFile)
	_, keyErr := os.Stat(keyFile)
	if certErr == nil || keyErr == nil || config.App.SecureMode {
		pair, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return fmt.Errorf("load TLS certificate: %w", err)
		}
		certificate = &pair
	} else if !errors.Is(certErr, os.ErrNotExist) || !errors.Is(keyErr, os.ErrNotExist) {
		return fmt.Errorf("inspect TLS files: %w", errors.Join(certErr, keyErr))
	}
	address := net.JoinHostPort(config.App.ListenHost, strconv.Itoa(httpPort))
	if *listen != "" {
		address = *listen
	}
	httpListener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	defer httpListener.Close()
	listeners := []net.Listener{httpListener}
	if certificate != nil {
		listener, err := net.Listen("tcp", net.JoinHostPort(config.App.ListenHost, strconv.Itoa(httpsPort)))
		if err != nil {
			return err
		}
		defer listener.Close()
		listeners = append(listeners, tls.NewListener(listener, &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{*certificate}}))
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := api.RestoreSessions(); err != nil {
		return fmt.Errorf("restore sessions: %w", err)
	}
	handler := api.Handler()
	servers := make([]*http.Server, 0, len(listeners))
	failures := make(chan error, len(listeners))
	for i, listener := range listeners {
		server := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, IdleTimeout: time.Minute, MaxHeaderBytes: 1 << 20}
		servers = append(servers, server)
		go func() { failures <- server.Serve(listener) }()
		scheme := "http"
		if i > 0 {
			scheme = "https"
		}
		slog.Info("listening", "url", scheme+"://"+listener.Addr().String())
	}
	var serveErr error
	select {
	case <-ctx.Done():
	case serveErr = <-failures:
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, server := range servers {
		if err := server.Shutdown(shutdown); err != nil {
			_ = server.Close()
			serveErr = errors.Join(serveErr, err)
		}
	}
	if errors.Is(serveErr, http.ErrServerClosed) {
		return nil
	}
	return serveErr
}

func envDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func port(name string, fallback int) (int, error) {
	value := fallback
	if raw := os.Getenv(name); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return 0, fmt.Errorf("invalid %s", name)
		}
		value = parsed
	}
	if value < 0 || value > 65535 {
		return 0, fmt.Errorf("invalid %s", name)
	}
	return value, nil
}
