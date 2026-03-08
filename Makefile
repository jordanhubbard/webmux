# WebMux — web-native terminal multiplexer
#
# Usage:
#   make              Build the application
#   make start        Build and start the server (background)
#   make stop         Stop the running server
#   make restart      Restart the server
#   make status       Check if the server is running
#   make test         Run all tests
#   make lint         Lint all code
#   make clean        Remove build artifacts and dependencies
#   make help         Show this help
#
# Configuration (override via environment or make args):
#   HTTP_PORT          HTTP listen port        (default: 8080)
#   HTTPS_PORT         HTTPS listen port       (default: 8443)
#   LISTEN_HOST        Bind address            (default: 0.0.0.0)
#   AUTH_MODE          none | local            (default: local)
#   JWT_SECRET         Token signing secret    (default: dev secret)
#   SECURE_MODE        true | false            (default: false)
#
# Examples:
#   make start HTTP_PORT=9090
#   make start AUTH_MODE=none
#   make start SECURE_MODE=true JWT_SECRET=$(openssl rand -hex 32)

# ── Paths ──────────────────────────────────────────────────────────
WEBMUX_DIR   := $(CURDIR)/webmux
WEBMUX_ROOT  := $(WEBMUX_DIR)
PIDFILE      := $(WEBMUX_DIR)/.webmux.pid
LOGFILE      := $(WEBMUX_DIR)/logs/webmux.log
NODE         := node
NPM          := npm

export WEBMUX_ROOT

# ── Tunables (override on command line or env) ─────────────────────
HTTP_PORT    ?=
HTTPS_PORT   ?=
LISTEN_HOST  ?=
AUTH_MODE    ?=
JWT_SECRET   ?=
SECURE_MODE  ?=

# Build the env-var exports only for values that are set
RUNTIME_ENV :=
ifneq ($(HTTP_PORT),)
  RUNTIME_ENV += HTTP_PORT=$(HTTP_PORT)
endif
ifneq ($(HTTPS_PORT),)
  RUNTIME_ENV += HTTPS_PORT=$(HTTPS_PORT)
endif
ifneq ($(JWT_SECRET),)
  RUNTIME_ENV += JWT_SECRET=$(JWT_SECRET)
endif

# ── Targets ────────────────────────────────────────────────────────
.PHONY: all build install start stop restart status test lint clean configure help

all: build

help:
	@sed -n '/^# /s/^# //p' $(MAKEFILE_LIST) | grep -v '^──' | head -22

install:
	@cd $(WEBMUX_DIR) && $(NPM) install --silent

build: install
	@echo "Building webmux…"
	@cd $(WEBMUX_DIR) && $(NPM) run build --silent
	@echo "Build complete."

configure:
	@echo "Configuring webmux…"
ifneq ($(LISTEN_HOST),)
	@cd $(WEBMUX_DIR) && sed -i.bak 's/listen_host:.*/listen_host: $(LISTEN_HOST)/' config/app.yaml && rm -f config/app.yaml.bak
endif
ifneq ($(HTTP_PORT),)
	@cd $(WEBMUX_DIR) && sed -i.bak 's/http_port:.*/http_port: $(HTTP_PORT)/' config/app.yaml && rm -f config/app.yaml.bak
endif
ifneq ($(HTTPS_PORT),)
	@cd $(WEBMUX_DIR) && sed -i.bak 's/https_port:.*/https_port: $(HTTPS_PORT)/' config/app.yaml && rm -f config/app.yaml.bak
endif
ifneq ($(SECURE_MODE),)
	@cd $(WEBMUX_DIR) && sed -i.bak 's/secure_mode:.*/secure_mode: $(SECURE_MODE)/' config/app.yaml && rm -f config/app.yaml.bak
endif
ifneq ($(AUTH_MODE),)
	@cd $(WEBMUX_DIR) && sed -i.bak 's/mode:.*/mode: $(AUTH_MODE)/' config/auth.yaml && rm -f config/auth.yaml.bak
endif
	@echo "Current configuration:"
	@echo "  app.yaml:"
	@grep -E '(listen_host|http_port|https_port|secure_mode):' $(WEBMUX_DIR)/config/app.yaml | sed 's/^/    /'
	@echo "  auth.yaml:"
	@grep -E 'mode:' $(WEBMUX_DIR)/config/auth.yaml | sed 's/^/    /'

start: build
	@mkdir -p $(WEBMUX_DIR)/logs
	@if [ -f $(PIDFILE) ] && kill -0 $$(cat $(PIDFILE)) 2>/dev/null; then \
		echo "webmux is already running (pid $$(cat $(PIDFILE)))"; \
		exit 1; \
	fi
	@cd $(WEBMUX_DIR) && $(RUNTIME_ENV) exec $(NODE) backend/dist/index.js >> $(LOGFILE) 2>&1 & echo $$! > $(PIDFILE)
	@sleep 0.5
	@if kill -0 $$(cat $(PIDFILE)) 2>/dev/null; then \
		echo "webmux started (pid $$(cat $(PIDFILE)))"; \
		echo "  logs: $(LOGFILE)"; \
		grep -E '(listening|http|https)' $(LOGFILE) 2>/dev/null | tail -3 | sed 's/^/  /'; \
	else \
		echo "webmux failed to start — check $(LOGFILE)"; \
		tail -5 $(LOGFILE) 2>/dev/null; \
		rm -f $(PIDFILE); \
		exit 1; \
	fi

stop:
	@if [ -f $(PIDFILE) ]; then \
		PID=$$(cat $(PIDFILE)); \
		if kill -0 $$PID 2>/dev/null; then \
			kill $$PID; \
			for i in 1 2 3 4 5 6 7 8 9 10; do \
				kill -0 $$PID 2>/dev/null || break; \
				sleep 0.5; \
			done; \
			if kill -0 $$PID 2>/dev/null; then \
				echo "webmux did not stop gracefully, sending SIGKILL"; \
				kill -9 $$PID 2>/dev/null; \
				sleep 0.5; \
			fi; \
			echo "webmux stopped (pid $$PID)"; \
		else \
			echo "webmux was not running (stale pidfile)"; \
		fi; \
		rm -f $(PIDFILE); \
	else \
		echo "webmux is not running"; \
	fi

restart: stop
	@$(MAKE) --no-print-directory start

status:
	@if [ -f $(PIDFILE) ] && kill -0 $$(cat $(PIDFILE)) 2>/dev/null; then \
		echo "webmux is running (pid $$(cat $(PIDFILE)))"; \
	else \
		echo "webmux is not running"; \
	fi

test:
	@cd $(WEBMUX_DIR) && $(NPM) test

lint:
	@cd $(WEBMUX_DIR) && $(NPM) run lint

clean:
	@echo "Cleaning build artifacts…"
	@rm -rf $(WEBMUX_DIR)/backend/dist $(WEBMUX_DIR)/web
	@rm -rf $(WEBMUX_DIR)/node_modules $(WEBMUX_DIR)/backend/node_modules $(WEBMUX_DIR)/frontend/node_modules
	@rm -f $(PIDFILE)
	@echo "Clean."
