# WebMux — web-native terminal multiplexer
#
# Usage:
#   make                  Build server and client
#   make start            Build and start the server (background)
#   make stop             Stop the running server
#   make restart          Restart the server
#   make status           Check if the server is running
#   make test             Run all tests
#   make lint             Lint all code
#   make clean            Remove build artifacts and dependencies
#   make install          Install the client executable only
#   make install-agent-deck Install the qualified Agent Deck coordinator
#   make install-server   Install the server as an OS service
#   make install-all      Install both client and server
#   make uninstall        Remove the client executable
#   make uninstall-server Remove the server service
#   make release          Test, bump patch version, open a release PR
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
#   CLIENT_BINDIR      Client install directory (default: ~/.local/bin)
#   AGENT_DECK_BINDIR  Agent Deck install directory (default: ~/.local/bin)
#   INSTALL_AGENT_DECK Install Agent Deck with the server (default: no)
#
# Examples:
#   make start HTTP_PORT=9090
#   make configure AUTH_MODE=none    # then make start
#   make configure SECURE_MODE=true # then make start JWT_SECRET=...

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
CLIENT_BINDIR ?= $(HOME)/.local/bin
AGENT_DECK_BINDIR ?= $(HOME)/.local/bin
AGENT_DECK_VERSION ?= v1.16.39
INSTALL_AGENT_DECK ?= no
PIDFILE      := $(WEBMUX_HOME)/.webmux.pid
LOGFILE      := $(WEBMUX_HOME)/logs/webmux.log
NODE         := node
NPM          := npm
GO           := go
OS           := $(shell uname)
SERVER_COMMAND := "$(WEBMUX_DIR)/bin/webmux"
E2E_SCRIPT := test:e2e
NPM_WORKSPACES := --workspace=frontend --include-workspace-root
TOOLCHAIN_DIR := $(CURDIR)/.build/toolchains
export PATH := $(TOOLCHAIN_DIR)/node/bin:$(TOOLCHAIN_DIR)/go/bin:$(PATH)
# Avoid stale/root-owned host caches on newly provisioned build machines.
# Explicit environment or command-line cache locations remain authoritative.
export GOMODCACHE ?= $(CURDIR)/.build/go-mod
export GOCACHE ?= $(CURDIR)/.build/go-cache
export PLAYWRIGHT_BROWSERS_PATH ?= $(CURDIR)/.build/playwright

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
.PHONY: all build build-server build-client deps toolchain-node toolchain-go check-tmux check-guacd start stop restart status test test-unit test-e2e lint clean configure help \
       install install-client install-agent-deck install-optional-agent-deck install-server install-all uninstall uninstall-client uninstall-server uninstall-all \
       release release-minor release-major release-publish changelog-init \
       package package-server package-client package-assets _start_manual _stop_manual

all: build

# Platform-specific runtime bundles; the combined archive retains its existing name.
package-assets: deps
	@cd "$(WEBMUX_DIR)" && $(NPM) run build --workspace=frontend

package: package-assets toolchain-go
	@WEBMUX_NATIVE_COMPONENT=all $(NODE) scripts/package-native.mts

package-server: package-assets toolchain-go
	@WEBMUX_NATIVE_COMPONENT=server $(NODE) scripts/package-native.mts

package-client: toolchain-node toolchain-go
	@WEBMUX_NATIVE_COMPONENT=client $(NODE) scripts/package-native.mts

help:
	@printf "$(C_BLD)$(C_MAG)▦ WebMux$(C_RST)$(C_DIM) — web-native terminal multiplexer$(C_RST)\n\n"
	@printf "$(C_BLD)Targets:$(C_RST)\n"
	@printf "  $(C_CYN)make package$(C_RST)        Package server and client together\n"
	@printf "  $(C_CYN)make package-server$(C_RST) Package the server only\n"
	@printf "  $(C_CYN)make package-client$(C_RST) Package the client only\n"
	@printf "  $(C_CYN)make$(C_RST)               Build server and client\n"
	@printf "  $(C_CYN)make build-server$(C_RST)  Build the server and browser UI\n"
	@printf "  $(C_CYN)make build-client$(C_RST)  Build the standalone client\n"
	@printf "  $(C_CYN)make start$(C_RST)          Build and start the server\n"
	@printf "  $(C_CYN)make stop$(C_RST)           Stop the running server\n"
	@printf "  $(C_CYN)make restart$(C_RST)        Restart the server\n"
	@printf "  $(C_CYN)make status$(C_RST)         Check if the server is running\n"
	@printf "  $(C_CYN)make test$(C_RST)           Run all tests (unit + e2e)\n"
	@printf "  $(C_CYN)make test-unit$(C_RST)      Typecheck + unit tests (no browser needed)\n"
	@printf "  $(C_CYN)make test-e2e$(C_RST)       E2E tests (auto-provisions a browser)\n"
	@printf "  $(C_CYN)make lint$(C_RST)           Lint all code\n"
	@printf "  $(C_CYN)make clean$(C_RST)          Remove build artifacts\n"
	@printf "  $(C_CYN)make install$(C_RST)        Install the client executable only\n"
	@printf "  $(C_CYN)make install-agent-deck$(C_RST) Install the qualified Agent Deck coordinator\n"
	@printf "  $(C_CYN)make install-server$(C_RST) Install the server service\n"
	@printf "  $(C_CYN)make install-all$(C_RST)    Install both client and server\n"
	@printf "  $(C_CYN)make uninstall$(C_RST)      Remove the client executable\n"
	@printf "  $(C_CYN)make uninstall-server$(C_RST) Remove the server service\n"
	@printf "  $(C_CYN)make uninstall-all$(C_RST)  Remove both installations\n"
	@printf "  $(C_CYN)make configure$(C_RST)      Update runtime configuration\n"
	@printf "  $(C_CYN)make release$(C_RST)        Bump patch version, test, open release PR\n"
	@printf "  $(C_CYN)make release-minor$(C_RST)  Bump minor version, test, open release PR\n"
	@printf "  $(C_CYN)make release-major$(C_RST)  Bump major version, test, open release PR\n"
	@printf "  $(C_CYN)make release-publish$(C_RST) Tag and publish after the release PR is merged\n"
	@printf "  $(C_CYN)make check-guacd$(C_RST)    Check guacd (RDP proxy) installation\n"
	@printf "  $(C_CYN)make check-tmux$(C_RST)     Check tmux (local terminals) installation\n"
	@printf "  $(C_CYN)make help$(C_RST)           Show this help\n"
	@printf "\n$(C_BLD)Configuration:$(C_RST)\n"
	@printf "  $(C_YLW)WEBMUX_HOME$(C_RST)=$(C_DIM)~/.config/webmux$(C_RST)   Runtime config/data directory\n"
	@printf "  $(C_YLW)HTTP_PORT$(C_RST)=$(C_DIM)8080$(C_RST)              HTTP listen port\n"
	@printf "  $(C_YLW)HTTPS_PORT$(C_RST)=$(C_DIM)8443$(C_RST)             HTTPS listen port\n"
	@printf "  $(C_YLW)LISTEN_HOST$(C_RST)=$(C_DIM)0.0.0.0$(C_RST)          Bind address\n"
	@printf "  $(C_YLW)CLIENT_BINDIR$(C_RST)=$(C_DIM)~/.local/bin$(C_RST)     Client install directory\n"
	@printf "  $(C_YLW)AGENT_DECK_BINDIR$(C_RST)=$(C_DIM)~/.local/bin$(C_RST) Agent Deck install directory\n"
	@printf "  $(C_YLW)INSTALL_AGENT_DECK$(C_RST)=$(C_DIM)no$(C_RST)          Install Agent Deck with server\n"

check-tmux:
	@sh scripts/check-tmux.sh

check-guacd:
	@sh scripts/check-guacd.sh

toolchain-node:
	@sh scripts/bootstrap-toolchain.sh node "$(NODE)" "$(TOOLCHAIN_DIR)"

toolchain-go:
	@sh scripts/bootstrap-toolchain.sh go "$(GO)" "$(TOOLCHAIN_DIR)" "$(WEBMUX_DIR)/server/go.mod"

deps: toolchain-node
	@cd "$(WEBMUX_DIR)" && $(NPM) ci $(NPM_WORKSPACES) --include=dev --no-audit --no-fund

build: build-server build-client
	@printf "$(C_GRN)✓$(C_RST) Server and client build complete.\n"

build-server: deps toolchain-go
	@printf "$(C_BLU)▸$(C_RST) Building webmux…\n"
	@cd "$(WEBMUX_DIR)" && $(NPM) run build:helpers --silent && $(NPM) run build --workspace=frontend --silent
	@mkdir -p "$(WEBMUX_DIR)/bin"
	@cd "$(WEBMUX_DIR)/server" && $(GO) run ./cmd/build -o ../bin/webmux
	@printf "$(C_GRN)✓$(C_RST) Server build complete.\n"

build-client: toolchain-go
	@printf "$(C_BLU)▸$(C_RST) Building webmux-client…\n"
	@mkdir -p "$(WEBMUX_DIR)/bin"
	@cd "$(WEBMUX_DIR)/server" && $(GO) build -o ../bin/webmux-client ./cmd/webmux-client
	@printf "$(C_GRN)✓$(C_RST) Client build complete.\n"

configure: export LISTEN_HOST := $(LISTEN_HOST)
configure: export HTTP_PORT := $(HTTP_PORT)
configure: export HTTPS_PORT := $(HTTPS_PORT)
configure: export SECURE_MODE := $(SECURE_MODE)
configure: export AUTH_MODE := $(AUTH_MODE)
configure: deps
	@$(NODE) scripts/configure.mts

# ── Manual (non-service) process control ────────────────────────────
# Direct pidfile-based start/stop used for dev runs and hosts where webmux is
# NOT installed as an OS service. Dispatched from start/stop below.
_start_manual: check-tmux
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
		exec $(SERVER_COMMAND) >> "$(LOGFILE)" 2>&1 & echo $$! > "$(PIDFILE)"; \
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

_stop_manual:
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

# ── Service-aware start/stop/restart/status ─────────────────────────
# When webmux is installed as an OS service, drive the platform's service
# manager so a restart is a *deliberate* restart (not a crash the manager
# would auto-recover or count as a failure). Otherwise fall back to the
# manual pidfile helpers above.
start: check-tmux build-server check-guacd
	@mkdir -p "$(WEBMUX_HOME)/logs"
	@if $(SVC_INSTALLED); then \
		printf "$(C_BLU)▸$(C_RST) Starting webmux via $(SVC_MGR)…\n"; \
		$(SVC_START) || exit $$?; \
		sleep 1; \
		$(SVC_STATUS) 2>/dev/null | sed 's/^/  /' || true; \
		printf "$(C_GRN)●$(C_RST) webmux started $(C_DIM)(managed by $(SVC_MGR))$(C_RST)\n"; \
	else \
		$(MAKE) --no-print-directory _start_manual; \
	fi

stop:
	@if $(SVC_INSTALLED); then \
		printf "$(C_BLU)▸$(C_RST) Stopping webmux via $(SVC_MGR)…\n"; \
		$(SVC_STOP) 2>/dev/null || true; \
		printf "$(C_DIM)●$(C_RST) webmux stopped $(C_DIM)(managed by $(SVC_MGR))$(C_RST)\n"; \
	else \
		$(MAKE) --no-print-directory _stop_manual; \
	fi

restart: check-tmux
	@if $(SVC_INSTALLED); then \
		$(MAKE) --no-print-directory build-server || exit $$?; \
		printf "$(C_BLU)▸$(C_RST) Restarting webmux via $(SVC_MGR) $(C_DIM)(deliberate restart)$(C_RST)…\n"; \
		$(SVC_RESTART) || exit $$?; \
		sleep 1; \
		$(SVC_STATUS) 2>/dev/null | sed 's/^/  /' || true; \
		printf "$(C_GRN)●$(C_RST) webmux restarted $(C_DIM)(managed by $(SVC_MGR))$(C_RST)\n"; \
	else \
		$(MAKE) --no-print-directory stop || exit $$?; \
		$(MAKE) --no-print-directory start; \
	fi

status:
	@if $(SVC_INSTALLED); then \
		printf "$(C_BLD)webmux$(C_RST) $(C_DIM)(managed by $(SVC_MGR))$(C_RST)\n"; \
		STATUS_OUTPUT=$$($(SVC_STATUS) 2>/dev/null); \
		if [ -n "$$STATUS_OUTPUT" ]; then printf '%s\n' "$$STATUS_OUTPUT" | sed 's/^/  /'; \
		else printf "  $(C_DIM)●$(C_RST) not running\n"; fi; \
	elif [ -f "$(PIDFILE)" ] && kill -0 $$(cat "$(PIDFILE)") 2>/dev/null; then \
		printf "$(C_GRN)●$(C_RST) webmux is running $(C_DIM)(pid $$(cat "$(PIDFILE)"))$(C_RST)\n"; \
	else \
		printf "$(C_DIM)●$(C_RST) webmux is not running\n"; \
	fi

# ── Testing ─────────────────────────────────────────────────────────
# Split so a browser/environment problem in e2e can't mask passing unit tests,
# and so `make test-unit` is a fast, dependency-light gate.
#   make test        typecheck + unit + e2e
#   make test-unit   typecheck + unit only (no browser required)
#   make test-e2e    e2e only (provisions a Chromium first)
test: test-unit test-e2e
	@printf "$(C_GRN)✓$(C_RST) All tests passed.\n"

test-unit: build
	@cd "$(WEBMUX_DIR)/server" && $(GO) test -race ./...
	@printf "$(C_BLU)▸$(C_RST) Type-checking…\n"
	@cd "$(WEBMUX_DIR)" && $(NPM) run typecheck --silent
	@printf "$(C_GRN)✓$(C_RST) Types OK.\n"
	@printf "$(C_BLU)▸$(C_RST) Running unit tests…\n"
	@cd "$(WEBMUX_DIR)" && $(NPM) test
	@printf "$(C_GRN)✓$(C_RST) Unit tests passed.\n"

# E2E needs a Playwright Chromium. ensure-e2e-browser.sh provisions the managed
# browser, verifies it actually launches (catching partial/corrupt downloads),
# and otherwise falls back to a system Chrome — turning the old cryptic
# "download browsers" failure into a resolved, clearly-reported browser.
test-e2e: build
	@EXE="$$("$(CURDIR)/scripts/ensure-e2e-browser.sh")" || { \
		printf "$(C_RED)✗$(C_RST) E2E skipped — no usable browser (see message above).\n"; exit 1; }; \
	printf "$(C_BLU)▸$(C_RST) Running E2E tests…\n"; \
	cd "$(WEBMUX_DIR)" && PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH="$$EXE" $(NPM) run $(E2E_SCRIPT)
	@printf "$(C_GRN)✓$(C_RST) E2E tests passed.\n"

lint: deps toolchain-go
	@cd "$(WEBMUX_DIR)" && $(NPM) run lint

clean: stop
	@printf "$(C_BLU)▸$(C_RST) Cleaning build artifacts…\n"
	@rm -rf "$(WEBMUX_DIR)/scripts/dist" "$(WEBMUX_DIR)/web" "$(WEBMUX_DIR)/bin"
	@rm -rf "$(WEBMUX_DIR)/node_modules" "$(WEBMUX_DIR)/frontend/node_modules"
	@rm -f "$(PIDFILE)"
	@printf "$(C_GRN)✓$(C_RST) Clean.\n"

# ── Service management ──────────────────────────────────────────

MACOS_APP   := $(HOME)/Applications/WebMux.app
PLIST       := $(HOME)/Library/LaunchAgents/com.webmux.server.plist
UNIT        := $(HOME)/.config/systemd/user/webmux.service
SYSTEMD_UNIT := $(notdir $(UNIT))
LAUNCHD_SVC := gui/$(shell id -u)/com.webmux.server

# ── Service-manager awareness ───────────────────────────────────────
# When webmux is installed as an OS service, start/stop/restart must drive the
# platform's service manager so a restart is a *deliberate* restart rather than
# a process kill the manager treats as a crash (launchd KeepAlive / systemd
# Restart=on-failure would otherwise fight us or count failures). When no
# service is installed (typical dev use), we fall back to direct pidfile-based
# process control. Windows drives the SCM via `npm run service:*`
# (service/windows-service.ps1) and is already deliberate.
#
# SVC_INSTALLED is a shell test: true (exit 0) when the service unit exists.
ifeq ($(OS),Darwin)
  export WEBMUX_SERVICE_OUTPUT := $(PLIST)
  export WEBMUX_EXECUTABLE := $(MACOS_APP)/Contents/MacOS/webmux
  SVC_MGR       := launchd
  SVC_INSTALLED := [ -f "$$WEBMUX_SERVICE_OUTPUT" ]
  SVC_START     := launchctl kickstart "$(LAUNCHD_SVC)" 2>/dev/null || launchctl bootstrap "$(patsubst %/,%,$(dir $(LAUNCHD_SVC)))" "$$WEBMUX_SERVICE_OUTPUT"
  SVC_STOP      := launchctl kill TERM "$(LAUNCHD_SVC)"
  SVC_RESTART   := launchctl kickstart -k "$(LAUNCHD_SVC)"
  SVC_STATUS    := launchctl print "$(LAUNCHD_SVC)" 2>/dev/null | grep -E 'state = |pid = '
else
  export WEBMUX_SERVICE_OUTPUT := $(UNIT)
  SVC_MGR       := systemd
  SVC_INSTALLED := [ -f "$$WEBMUX_SERVICE_OUTPUT" ]
  SVC_START     := systemctl --user start "$(SYSTEMD_UNIT)"
  SVC_STOP      := systemctl --user stop "$(SYSTEMD_UNIT)"
  SVC_RESTART   := systemctl --user restart "$(SYSTEMD_UNIT)"
  SVC_STATUS    := systemctl --user --no-pager --lines=0 status "$(SYSTEMD_UNIT)"
endif

install: install-client

install-client: build-client
	@mkdir -p "$(CLIENT_BINDIR)"
	@install -m 0755 "$(WEBMUX_DIR)/bin/webmux-client" "$(CLIENT_BINDIR)/webmux-client"
	@printf "$(C_GRN)✓$(C_RST) Installed client: %s/webmux-client\n" "$(CLIENT_BINDIR)"
	@printf "  Discover the server's advertised port, then enroll:\n"
	@printf "  %s/webmux-client --server WEBMUX_HOST --background\n" "$(CLIENT_BINDIR)"

install-agent-deck:
	@AGENT_DECK_BINDIR="$(AGENT_DECK_BINDIR)" AGENT_DECK_VERSION="$(AGENT_DECK_VERSION)" \
		sh scripts/install-agent-deck.sh

install-optional-agent-deck:
	@if [ "$(INSTALL_AGENT_DECK)" = "yes" ]; then \
		AGENT_DECK_BINDIR="$(AGENT_DECK_BINDIR)" AGENT_DECK_VERSION="$(AGENT_DECK_VERSION)" \
			sh scripts/install-agent-deck.sh; \
	elif [ "$(INSTALL_AGENT_DECK)" = "no" ]; then \
		printf "$(C_YLW)! $(C_RST) Agent Deck installation skipped (INSTALL_AGENT_DECK=no).\n"; \
	else \
		printf "$(C_RED)✗$(C_RST) INSTALL_AGENT_DECK must be yes or no.\n" >&2; exit 2; \
	fi

# Finish client installation before restarting the server that may own this
# terminal. A recursive target also preserves that order under make -j.
install-all: install-client
	@$(MAKE) install-server

# Finish fallible preparation before interrupting the installed service.
# Do not list stop as a prerequisite: make -j can run it beside the build.
install-server: check-tmux install-optional-agent-deck build-server
ifeq ($(shell uname),Darwin)
	@printf "$(C_BLU)▸$(C_RST) Installing launchd service…\n"
	@$(NODE) scripts/macos-app.mts "$(WEBMUX_DIR)/bin/webmux" "$(MACOS_APP)"
	@PATH="$(AGENT_DECK_BINDIR):$$PATH" $(NODE) scripts/render-service.mts
	@$(NODE) scripts/check-launchd.mts "$(LAUNCHD_SVC)" "$(LOGFILE)" "$$WEBMUX_SERVICE_OUTPUT"
	@printf "$(C_GRN)✓$(C_RST) Installed: $(C_CYN)%s$(C_RST)\n" "$$WEBMUX_SERVICE_OUTPUT"
	@printf "  $(C_DIM)config:$(C_RST) %s\n" "$$WEBMUX_HOME"
	@printf "  $(C_DIM)logs:$(C_RST)   %s/logs/webmux.log\n" "$$WEBMUX_HOME"
	@printf "$(C_GRN)●$(C_RST) WebMux will start automatically on login.\n"
	@printf "  Open WebMux in your browser to complete Mac permissions setup: http://localhost:8080\n"
else
	@printf "$(C_BLU)▸$(C_RST) Installing systemd user service…\n"
	@PATH="$(AGENT_DECK_BINDIR):$$PATH" $(NODE) scripts/render-service.mts
	@systemctl --user daemon-reload
	@systemctl --user stop "$(SYSTEMD_UNIT)"
	@systemctl --user enable --now "$(SYSTEMD_UNIT)"
	@printf "$(C_GRN)✓$(C_RST) Installed: $(C_CYN)%s$(C_RST)\n" "$$WEBMUX_SERVICE_OUTPUT"
	@printf "  $(C_DIM)config:$(C_RST) %s\n" "$$WEBMUX_HOME"
	@printf "  $(C_DIM)logs:$(C_RST)   %s/logs/webmux.log\n" "$$WEBMUX_HOME"
	@printf "$(C_GRN)●$(C_RST) WebMux will start automatically on login.\n"
	@printf "  $(C_DIM)Hint: run$(C_RST) loginctl enable-linger $(USER) $(C_DIM)to start without logging in.$(C_RST)\n"
endif

uninstall: uninstall-client

uninstall-client:
	@rm -f "$(CLIENT_BINDIR)/webmux-client"
	@printf "$(C_GRN)✓$(C_RST) Client executable removed from %s\n" "$(CLIENT_BINDIR)"

uninstall-all: uninstall-server uninstall-client

uninstall-server:
ifeq ($(shell uname),Darwin)
	@printf "$(C_BLU)▸$(C_RST) Removing launchd service…\n"
	@launchctl bootout $(LAUNCHD_SVC) 2>/dev/null || true
	@rm -f "$$WEBMUX_SERVICE_OUTPUT"
	@printf "$(C_GRN)✓$(C_RST) Uninstalled.\n"
else
	@printf "$(C_BLU)▸$(C_RST) Removing systemd user service…\n"
	@systemctl --user disable --now "$(SYSTEMD_UNIT)" 2>/dev/null || true
	@rm -f "$$WEBMUX_SERVICE_OUTPUT"
	@systemctl --user daemon-reload
	@printf "$(C_GRN)✓$(C_RST) Uninstalled.\n"
endif

# ── Release management ──────────────────────────────────────────────
# Checks prerequisites, runs tests, bumps version in package.json files,
# updates CHANGELOG.md, commits, and opens a release pull request.
# Run `make release-publish` after that PR is reviewed and merged to
# tag the merge commit and create the GitHub release.
#
#   make release          Bump patch version (x.y.Z → x.y.Z+1) and open a PR
#   make release-minor    Bump minor version (x.Y.z → x.Y+1.0) and open a PR
#   make release-major    Bump major version (X.y.z → X+1.0.0) and open a PR
#   make release-publish  Tag and publish after the release PR is merged
#
# Non-interactive batch mode: BATCH=yes make release

release: toolchain-node
	@BATCH=$(BATCH) ./scripts/release.sh prepare patch

release-minor: toolchain-node
	@BATCH=$(BATCH) ./scripts/release.sh prepare minor

release-major: toolchain-node
	@BATCH=$(BATCH) ./scripts/release.sh prepare major

release-publish: toolchain-node
	@BATCH=$(BATCH) ./scripts/release.sh publish

changelog-init:
	@if [ -f CHANGELOG.md ]; then \
		printf "$(C_YLW)!$(C_RST) CHANGELOG.md already exists\n"; \
	else \
		printf '# Changelog\n\nAll notable changes to WebMux are documented here.\nFormat follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).\n\n## [Unreleased]\n' > CHANGELOG.md; \
		printf "$(C_GRN)✓$(C_RST) CHANGELOG.md created\n"; \
	fi
