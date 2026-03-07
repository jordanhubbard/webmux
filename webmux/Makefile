WEBMUX_ROOT := $(CURDIR)
PIDFILE      := $(WEBMUX_ROOT)/.webmux.pid
NODE         := node
NPM          := npm
export WEBMUX_ROOT

.PHONY: build install start stop restart status test lint clean

install:
	$(NPM) install

build: install
	$(NPM) run build

start: build
	@if [ -f $(PIDFILE) ] && kill -0 $$(cat $(PIDFILE)) 2>/dev/null; then \
		echo "webmux is already running (pid $$(cat $(PIDFILE)))"; \
		exit 1; \
	fi
	@$(NODE) backend/dist/index.js & echo $$! > $(PIDFILE)
	@echo "webmux started (pid $$(cat $(PIDFILE)))"

stop:
	@if [ -f $(PIDFILE) ]; then \
		PID=$$(cat $(PIDFILE)); \
		if kill -0 $$PID 2>/dev/null; then \
			kill $$PID; \
			echo "webmux stopped (pid $$PID)"; \
		else \
			echo "webmux was not running (stale pidfile)"; \
		fi; \
		rm -f $(PIDFILE); \
	else \
		echo "webmux is not running (no pidfile)"; \
	fi

restart: stop
	@$(MAKE) start

status:
	@if [ -f $(PIDFILE) ] && kill -0 $$(cat $(PIDFILE)) 2>/dev/null; then \
		echo "webmux is running (pid $$(cat $(PIDFILE)))"; \
	else \
		echo "webmux is not running"; \
	fi

test:
	$(NPM) test

lint:
	$(NPM) run lint

clean:
	rm -rf backend/dist web node_modules backend/node_modules frontend/node_modules
	rm -f $(PIDFILE)
