APP_NAME := shhh
WAILS_PROJECT := cmd/shhh
WAILS_VERSION := v2.13.0
TOOLS_DIR ?= $(CURDIR)/bin
WAILS := $(TOOLS_DIR)/wails
WAILS_STAMP := $(TOOLS_DIR)/.wails-$(WAILS_VERSION)
VERSION ?= 0.1.0-dev
COMMIT ?= $(shell git rev-parse --short=12 HEAD 2>/dev/null || printf unknown)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
DIRTY ?= $(shell if test -n "$$(git status --porcelain 2>/dev/null)"; then printf true; else printf false; fi)
BUILD_LDFLAGS = \
	-X shh-h/internal/buildinfo.Version=$(VERSION) \
	-X shh-h/internal/buildinfo.Commit=$(COMMIT) \
	-X shh-h/internal/buildinfo.BuildDate=$(BUILD_DATE) \
	-X shh-h/internal/buildinfo.Dirty=$(DIRTY)

.PHONY: bootstrap-wails build run dev test test-go test-frontend lint check-bindings tidy clean

$(WAILS_STAMP):
	mkdir -p $(TOOLS_DIR)
	GOBIN=$(abspath $(TOOLS_DIR)) go install github.com/wailsapp/wails/v2/cmd/wails@$(WAILS_VERSION)
	touch $(WAILS_STAMP)

bootstrap-wails: $(WAILS_STAMP)

build: $(WAILS_STAMP)
	cd $(WAILS_PROJECT) && $(WAILS) build -clean -trimpath -ldflags "$(BUILD_LDFLAGS)" -o $(APP_NAME)

run: $(WAILS_STAMP)
	cd $(WAILS_PROJECT) && $(WAILS) dev

dev: run

test: test-go test-frontend

test-go:
	go test ./...

test-frontend:
	cd frontend && npm test

lint:
	cd frontend && npm run lint
	go vet ./...

check-bindings: $(WAILS_STAMP)
	cd $(WAILS_PROJECT) && $(WAILS) build -clean -nopackage -trimpath -ldflags "$(BUILD_LDFLAGS)" -o $(APP_NAME)-bindings
	cd frontend && npm run bindings:normalize
	git diff --exit-code -- frontend/wailsjs

tidy:
	go mod tidy

clean:
	rm -rf build/bin internal/frontendassets/dist internal/frontendassets/bundle/dist frontend/coverage coverage.out
