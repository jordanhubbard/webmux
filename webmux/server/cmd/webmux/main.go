// Command webmux serves the browser UI and native terminal/desktop APIs.
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"math"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/assets"
	appconfig "github.com/jordanhubbard/webmux/server/internal/config"
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
	root := flag.String("root", os.Getenv("WEBMUX_ROOT"), "optional directory overriding embedded web/ and config.defaults/")
	home := flag.String("home", os.Getenv("WEBMUX_HOME"), "writable configuration/data directory")
	listen := flag.String("listen", "", "override HTTP listen address (e.g. 127.0.0.1:18080)")
	flag.Parse()
	slaveHost := os.Getenv("WEBMUX_SLAVE_HOST")
	slavePort := 0
	if slaveHost != "" {
		var err error
		slavePort, err = parseSlavePort(os.Getenv("WEBMUX_SLAVE_PORT"))
		if err != nil {
			return err
		}
	}
	if *home == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		*home = filepath.Join(userHome, ".config", "webmux")
	}
	defaults, web, err := runtimeAssets(*root)
	if err != nil {
		return err
	}
	defer defaults.Close()
	store, err := storage.OpenFS(*home, defaults.FS())
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
	api, err := httpapi.New(store, httpapi.Options{Name: config.App.Name, SecureMode: config.App.SecureMode, JWTSecret: os.Getenv("JWT_SECRET"), WebDir: web.directory, WebFS: web.files})
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
	maintenanceCtx, cancelMaintenance := context.WithCancel(ctx)
	maintenanceDone := make(chan struct{})
	go func() { defer close(maintenanceDone); api.MaintainUploads(maintenanceCtx) }()
	defer func() { cancelMaintenance(); <-maintenanceDone }()
	if err := api.RestoreSessions(); err != nil {
		return fmt.Errorf("restore sessions: %w", err)
	}
	if slaveHost != "" {
		if err := api.StartSlave(slaveHost, slavePort); err != nil {
			return fmt.Errorf("initialize slave session: %w", err)
		}
		slog.Info("slave console initialized", "host", slaveHost, "port", slavePort)
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

type webAssets struct {
	directory string
	files     fs.FS
}
type defaultAssets struct {
	files fs.FS
	root  *os.Root
}

func (a defaultAssets) FS() fs.FS { return a.files }
func (a defaultAssets) Close() {
	if a.root != nil {
		_ = a.root.Close()
	}
}

func runtimeAssets(root string) (defaultAssets, webAssets, error) {
	defaults := defaultAssets{files: assets.Defaults()}
	web := webAssets{files: assets.Web()}
	// A standalone binary never discovers files from its current directory.
	// Unembedded go build retains the source-tree development fallback.
	if root == "" && defaults.files == nil {
		root = installationRoot()
	}
	if root != "" {
		for _, name := range []string{"config.defaults", "web"} {
			directory := filepath.Join(root, name)
			info, err := os.Stat(directory)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				defaults.Close()
				return defaultAssets{}, webAssets{}, err
			}
			if !info.IsDir() {
				defaults.Close()
				return defaultAssets{}, webAssets{}, fmt.Errorf("%s must be a directory", directory)
			}
			if name == "web" {
				web.directory = directory
				continue
			}
			opened, err := os.OpenRoot(directory)
			if err != nil {
				return defaultAssets{}, webAssets{}, err
			}
			defaults = defaultAssets{files: opened.FS(), root: opened}
		}
	}
	if defaults.files == nil {
		return defaults, web, errors.New("configuration defaults unavailable: build with go run ./cmd/build or set --root for development")
	}
	return defaults, web, nil
}

// Extracted bundles can start from any working directory. Source-tree runs
// retain the current-directory default when no adjacent installation exists.
func installationRoot() string {
	executable, err := os.Executable()
	if err != nil {
		return "."
	}
	if resolved, err := filepath.EvalSymlinks(executable); err == nil {
		executable = resolved
	}
	root := filepath.Dir(filepath.Dir(executable))
	if info, err := os.Stat(filepath.Join(root, "config.defaults")); err == nil && info.IsDir() {
		return root
	}
	return "."
}

func parseSlavePort(raw string) (int, error) {
	value, err := appconfig.Number(raw)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value || value < 0 || value > 65535 {
		return 0, errors.New("invalid WEBMUX_SLAVE_PORT")
	}
	return int(value), nil
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
