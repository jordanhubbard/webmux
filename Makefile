# WebMux — web-native terminal multiplexer
#
# Usage:
#   make                  Build the application
#   make start            Build and start the server (background)
#   make stop             Stop the running server
#   make restart          Restart the server
#   make status           Check if the server is running
#   make test             Run all tests
#   make lint             Lint all code
#   make clean            Remove build artifacts and dependencies
#   make install          Install as OS service (launchd/systemd)
#   make uninstall        Remove OS service
#   make release          Test, bump patch version, tag, push, create GitHub release
#   make release-minor    Same as release but bumps the minor version
#   make release-major    Same as release but bumps the major version
#   make help             Show this help
#
# Configuration (override via environment or make args):
#   WEBMUX_HOME        Runtime config/data dir  (default: ~/.config/webmux)
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

# ── Colors ────────────────────────────────────────────────────────
C_RST  := \033[0m
C_BLD  := \033[1m
C_DIM  := \033[2m
C_GRN  := \033[32m
C_YLW  := \033[33m
C_BLU  := \033[34m
C_MAG  := \033[35m
C_CYN  := \033[36m
C_RED  := \033[31m
C_WHT  := \033[37m

# ── Paths ──────────────────────────────────────────────────────────
WEBMUX_DIR   := $(CURDIR)/webmux
WEBMUX_ROOT  := $(WEBMUX_DIR)
WEBMUX_HOME  ?= $(HOME)/.config/webmux
PIDFILE      := $(WEBMUX_HOME)/.webmux.pid
LOGFILE      := $(WEBMUX_HOME)/logs/webmux.log
NODE         := node
NPM          := npm

export WEBMUX_ROOT
export WEBMUX_HOME

# ── Tunables (override on command line or env) ─────────────────────
HTTP_PORT    ?=
HTTPS_PORT   ?=
LISTEN_HOST  ?=
AUTH_MODE    ?=
JWT_SECRET   ?=
SECURE_MODE  ?=

# Build the env-var exports only for values that are set.
# HTTP_PORT and HTTPS_PORT are resolved at start time (with port-conflict check)
# and passed explicitly; only JWT_SECRET needs to be forwarded here.
RUNTIME_ENV :=
ifneq ($(JWT_SECRET),)
  RUNTIME_ENV += JWT_SECRET=$(JWT_SECRET)
endif

# ── Targets ────────────────────────────────────────────────────────
.PHONY: all build deps start stop restart status test lint clean configure help \
       install uninstall release release-minor release-major changelog-init

all: build

help:
	@printf "$(C_BLD)$(C_MAG)▦ WebMux$(C_RST)$(C_DIM) — web-native terminal multiplexer$(C_RST)\n\n"
	@printf "$(C_BLD)Targets:$(C_RST)\n"
	@printf "  $(C_CYN)make$(C_RST)               Build the application\n"
	@printf "  $(C_CYN)make start$(C_RST)          Build and start the server\n"
	@printf "  $(C_CYN)make stop$(C_RST)           Stop the running server\n"
	@printf "  $(C_CYN)make restart$(C_RST)        Restart the server\n"
	@printf "  $(C_CYN)make status$(C_RST)         Check if the server is running\n"
	@printf "  $(C_CYN)make test$(C_RST)           Run all tests\n"
	@printf "  $(C_CYN)make lint$(C_RST)           Lint all code\n"
	@printf "  $(C_CYN)make clean$(C_RST)          Remove build artifacts\n"
	@printf "  $(C_CYN)make install$(C_RST)        Install as OS service (launchd/systemd)\n"
	@printf "  $(C_CYN)make uninstall$(C_RST)      Remove OS service\n"
	@printf "  $(C_CYN)make configure$(C_RST)      Update runtime configuration\n"
	@printf "  $(C_CYN)make release$(C_RST)        Bump patch version, test, tag, push, GitHub release\n"
	@printf "  $(C_CYN)make release-minor$(C_RST)  Bump minor version and release\n"
	@printf "  $(C_CYN)make release-major$(C_RST)  Bump major version and release\n"
	@printf "  $(C_CYN)make help$(C_RST)           Show this help\n"
	@printf "\n$(C_BLD)Configuration:$(C_RST)\n"
	@printf "  $(C_YLW)WEBMUX_HOME$(C_RST)=$(C_DIM)~/.config/webmux$(C_RST)   Runtime config/data directory\n"
	@printf "  $(C_YLW)HTTP_PORT$(C_RST)=$(C_DIM)8080$(C_RST)              HTTP listen port\n"
	@printf "  $(C_YLW)HTTPS_PORT$(C_RST)=$(C_DIM)8443$(C_RST)             HTTPS listen port\n"
	@printf "  $(C_YLW)LISTEN_HOST$(C_RST)=$(C_DIM)0.0.0.0$(C_RST)          Bind address\n"

deps:
	@cd "$(WEBMUX_DIR)" && $(NPM) install --silent

build: deps
	@printf "$(C_BLU)▸$(C_RST) Building webmux…\n"
	@cd "$(WEBMUX_DIR)" && $(NPM) run build --silent
	@printf "$(C_GRN)✓$(C_RST) Build complete.\n"

configure:
	@printf "$(C_BLU)▸$(C_RST) Configuring webmux…\n"
	@mkdir -p "$(WEBMUX_HOME)/config"
ifneq ($(LISTEN_HOST),)
	@sed -i.bak 's/listen_host:.*/listen_host: $(LISTEN_HOST)/' "$(WEBMUX_HOME)/config/app.yaml" && rm -f "$(WEBMUX_HOME)/config/app.yaml.bak"
endif
ifneq ($(HTTP_PORT),)
	@sed -i.bak 's/http_port:.*/http_port: $(HTTP_PORT)/' "$(WEBMUX_HOME)/config/app.yaml" && rm -f "$(WEBMUX_HOME)/config/app.yaml.bak"
endif
ifneq ($(HTTPS_PORT),)
	@sed -i.bak 's/https_port:.*/https_port: $(HTTPS_PORT)/' "$(WEBMUX_HOME)/config/app.yaml" && rm -f "$(WEBMUX_HOME)/config/app.yaml.bak"
endif
ifneq ($(SECURE_MODE),)
	@sed -i.bak 's/secure_mode:.*/secure_mode: $(SECURE_MODE)/' "$(WEBMUX_HOME)/config/app.yaml" && rm -f "$(WEBMUX_HOME)/config/app.yaml.bak"
endif
ifneq ($(AUTH_MODE),)
	@sed -i.bak 's/mode:.*/mode: $(AUTH_MODE)/' "$(WEBMUX_HOME)/config/auth.yaml" && rm -f "$(WEBMUX_HOME)/config/auth.yaml.bak"
endif
	@printf "\n$(C_BLD)Current configuration:$(C_RST)\n"
	@printf "  $(C_DIM)app.yaml:$(C_RST)\n"
	@grep -E '(listen_host|http_port|https_port|secure_mode):' "$(WEBMUX_HOME)/config/app.yaml" | sed 's/^/    /'
	@printf "  $(C_DIM)auth.yaml:$(C_RST)\n"
	@grep -E 'mode:' "$(WEBMUX_HOME)/config/auth.yaml" | sed 's/^/    /'

start: build
	@mkdir -p "$(WEBMUX_HOME)/logs"
	@if [ -f "$(PIDFILE)" ] && kill -0 $$(cat "$(PIDFILE)") 2>/dev/null; then \
		printf "$(C_YLW)●$(C_RST) webmux is already running $(C_DIM)(pid $$(cat "$(PIDFILE)"))$(C_RST)\n"; \
		exit 1; \
	fi
	@WANT_HTTP="$(HTTP_PORT)"; [ -z "$$WANT_HTTP" ] && WANT_HTTP=8080; \
	WANT_HTTPS="$(HTTPS_PORT)"; [ -z "$$WANT_HTTPS" ] && WANT_HTTPS=8443; \
	if command -v python3 >/dev/null 2>&1; then PYTHON=python3; \
	elif command -v python >/dev/null 2>&1; then PYTHON=python; \
	else PYTHON=""; fi; \
	if [ -n "$$PYTHON" ]; then \
		ACTUAL_HTTP=$$($$PYTHON "$(CURDIR)/scripts/find_free_port.py" "$$WANT_HTTP") || exit 1; \
		ACTUAL_HTTPS=$$($$PYTHON "$(CURDIR)/scripts/find_free_port.py" "$$WANT_HTTPS") || exit 1; \
	else \
		printf "$(C_YLW)!$(C_RST) python3/python not found — skipping port availability check\n"; \
		ACTUAL_HTTP=$$WANT_HTTP; ACTUAL_HTTPS=$$WANT_HTTPS; \
	fi; \
	LOG_START=$$(wc -l < "$(LOGFILE)" 2>/dev/null || echo 0); \
	cd "$(WEBMUX_DIR)" && $(RUNTIME_ENV) HTTP_PORT=$$ACTUAL_HTTP HTTPS_PORT=$$ACTUAL_HTTPS \
		WEBMUX_ROOT="$(WEBMUX_ROOT)" WEBMUX_HOME="$(WEBMUX_HOME)" \
		exec $(NODE) backend/dist/index.js >> "$(LOGFILE)" 2>&1 & echo $$! > "$(PIDFILE)"; \
	sleep 0.5; \
	if kill -0 $$(cat "$(PIDFILE)") 2>/dev/null; then \
		printf "$(C_GRN)●$(C_RST) webmux started $(C_DIM)(pid $$(cat "$(PIDFILE)"))$(C_RST)\n"; \
		printf "  $(C_DIM)config:$(C_RST) $(WEBMUX_HOME)\n"; \
		printf "  $(C_DIM)logs:$(C_RST)   $(LOGFILE)\n"; \
		tail -n +$$(($$LOG_START + 1)) "$(LOGFILE)" 2>/dev/null | grep -E '(listening|http|https)' | tail -3 | sed 's/^/  /'; \
	else \
		printf "$(C_RED)✗$(C_RST) webmux failed to start — check $(LOGFILE)\n"; \
		tail -5 "$(LOGFILE)" 2>/dev/null; \
		rm -f "$(PIDFILE)"; \
		exit 1; \
	fi

stop:
	@if [ -f "$(PIDFILE)" ]; then \
		PID=$$(cat "$(PIDFILE)"); \
		if kill -0 $$PID 2>/dev/null; then \
			kill $$PID; \
			for i in 1 2 3 4 5 6 7 8 9 10; do \
				kill -0 $$PID 2>/dev/null || break; \
				sleep 0.5; \
			done; \
			if kill -0 $$PID 2>/dev/null; then \
				printf "$(C_YLW)!$(C_RST) webmux did not stop gracefully, sending SIGKILL\n"; \
				kill -9 $$PID 2>/dev/null; \
				sleep 0.5; \
			fi; \
			printf "$(C_DIM)●$(C_RST) webmux stopped $(C_DIM)(pid $$PID)$(C_RST)\n"; \
		else \
			printf "$(C_DIM)●$(C_RST) webmux was not running $(C_DIM)(stale pidfile)$(C_RST)\n"; \
		fi; \
		rm -f "$(PIDFILE)"; \
	else \
		printf "$(C_DIM)●$(C_RST) webmux is not running\n"; \
	fi

restart: stop
	@$(MAKE) --no-print-directory start

status:
	@if [ -f "$(PIDFILE)" ] && kill -0 $$(cat "$(PIDFILE)") 2>/dev/null; then \
		printf "$(C_GRN)●$(C_RST) webmux is running $(C_DIM)(pid $$(cat "$(PIDFILE)"))$(C_RST)\n"; \
	else \
		printf "$(C_DIM)●$(C_RST) webmux is not running\n"; \
	fi

test: build
	@printf "$(C_BLU)▸$(C_RST) Type-checking…\n"
	@cd "$(WEBMUX_DIR)" && $(NPM) run typecheck --silent
	@printf "$(C_GRN)✓$(C_RST) Types OK.\n"
	@printf "$(C_BLU)▸$(C_RST) Running unit tests…\n"
	@cd "$(WEBMUX_DIR)" && $(NPM) test
	@printf "$(C_BLU)▸$(C_RST) Running E2E tests…\n"
	@cd "$(WEBMUX_DIR)" && $(NPM) run test:e2e
	@printf "$(C_GRN)✓$(C_RST) All tests passed.\n"

lint:
	@cd "$(WEBMUX_DIR)" && $(NPM) run lint

clean: stop
	@printf "$(C_BLU)▸$(C_RST) Cleaning build artifacts…\n"
	@rm -rf "$(WEBMUX_DIR)/backend/dist" "$(WEBMUX_DIR)/web"
	@rm -rf "$(WEBMUX_DIR)/node_modules" "$(WEBMUX_DIR)/backend/node_modules" "$(WEBMUX_DIR)/frontend/node_modules"
	@rm -f "$(PIDFILE)"
	@printf "$(C_GRN)✓$(C_RST) Clean.\n"

# ── Service management ──────────────────────────────────────────
SERVICE_DIR     := $(WEBMUX_DIR)/service
NODE_PATH       := $(shell which node)
CURRENT_PATH    := $(shell echo $$PATH)

PLIST       := $(HOME)/Library/LaunchAgents/com.webmux.server.plist
UNIT        := $(HOME)/.config/systemd/user/webmux.service
LAUNCHD_SVC := gui/$(shell id -u)/com.webmux.server

install: stop build
	@mkdir -p "$(WEBMUX_HOME)/logs"
ifeq ($(shell uname),Darwin)
	@printf "$(C_BLU)▸$(C_RST) Installing launchd service…\n"
	@mkdir -p "$(HOME)/Library/LaunchAgents"
	@launchctl bootout $(LAUNCHD_SVC) 2>/dev/null || true
	@sed \
		-e 's|__NODE_PATH__|$(NODE_PATH)|g' \
		-e 's|__WEBMUX_DIR__|$(WEBMUX_DIR)|g' \
		-e 's|__WEBMUX_HOME__|$(WEBMUX_HOME)|g' \
		-e 's|__PATH__|$(CURRENT_PATH)|g' \
		"$(SERVICE_DIR)/com.webmux.server.plist.template" \
		> "$(PLIST)"
	@launchctl bootstrap gui/$$(id -u) "$(PLIST)"
	@launchctl kickstart -k $(LAUNCHD_SVC) 2>/dev/null || true
	@printf "$(C_GRN)✓$(C_RST) Installed: $(C_CYN)$(PLIST)$(C_RST)\n"
	@printf "  $(C_DIM)config:$(C_RST) $(WEBMUX_HOME)\n"
	@printf "  $(C_DIM)logs:$(C_RST)   $(WEBMUX_HOME)/logs/webmux.log\n"
	@printf "$(C_GRN)●$(C_RST) WebMux will start automatically on login.\n"
else
	@printf "$(C_BLU)▸$(C_RST) Installing systemd user service…\n"
	@mkdir -p $(dir $(UNIT))
	@sed \
		-e 's|__NODE_PATH__|$(NODE_PATH)|g' \
		-e 's|__WEBMUX_DIR__|$(WEBMUX_DIR)|g' \
		-e 's|__WEBMUX_HOME__|$(WEBMUX_HOME)|g' \
		-e 's|__PATH__|$(CURRENT_PATH)|g' \
		"$(SERVICE_DIR)/webmux.service.template" \
		> "$(UNIT)"
	@systemctl --user daemon-reload
	@systemctl --user enable --now webmux.service
	@printf "$(C_GRN)✓$(C_RST) Installed: $(C_CYN)$(UNIT)$(C_RST)\n"
	@printf "  $(C_DIM)config:$(C_RST) $(WEBMUX_HOME)\n"
	@printf "  $(C_DIM)logs:$(C_RST)   $(WEBMUX_HOME)/logs/webmux.log\n"
	@printf "$(C_GRN)●$(C_RST) WebMux will start automatically on login.\n"
	@printf "  $(C_DIM)Hint: run$(C_RST) loginctl enable-linger $(USER) $(C_DIM)to start without logging in.$(C_RST)\n"
endif

uninstall:
ifeq ($(shell uname),Darwin)
	@printf "$(C_BLU)▸$(C_RST) Removing launchd service…\n"
	@launchctl bootout $(LAUNCHD_SVC) 2>/dev/null || true
	@rm -f $(PLIST)
	@printf "$(C_GRN)✓$(C_RST) Uninstalled.\n"
else
	@printf "$(C_BLU)▸$(C_RST) Removing systemd user service…\n"
	@systemctl --user disable --now webmux.service 2>/dev/null || true
	@rm -f $(UNIT)
	@systemctl --user daemon-reload
	@printf "$(C_GRN)✓$(C_RST) Uninstalled.\n"
endif

# ── Release management ──────────────────────────────────────────────
# Checks prerequisites, runs tests, bumps version in package.json files,
# updates CHANGELOG.md, commits, tags, pushes, and creates a GitHub release.
#
#   make release          Bump patch version (x.y.Z → x.y.Z+1)
#   make release-minor    Bump minor version (x.Y.z → x.Y+1.0)
#   make release-major    Bump major version (X.y.z → X+1.0.0)
#
# Non-interactive batch mode: BATCH=yes make release

release:
	@BATCH=$(BATCH) ./scripts/release.sh patch

release-minor:
	@BATCH=$(BATCH) ./scripts/release.sh minor

release-major:
	@BATCH=$(BATCH) ./scripts/release.sh major

changelog-init:
	@if [ -f CHANGELOG.md ]; then \
		printf "$(C_YLW)!$(C_RST) CHANGELOG.md already exists\n"; \
	else \
		printf '# Changelog\n\nAll notable changes to WebMux are documented here.\nFormat follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).\n\n## [Unreleased]\n' > CHANGELOG.md; \
		printf "$(C_GRN)✓$(C_RST) CHANGELOG.md created\n"; \
	fi
