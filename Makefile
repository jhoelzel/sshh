APP_NAME := shhh
WAILS_PROJECT := cmd/shhh
VERSION ?= 0.1.0-dev
COMMIT ?= $(shell git rev-parse --short=12 HEAD 2>/dev/null || printf unknown)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
DIRTY ?= $(shell if test -n "$$(git status --porcelain 2>/dev/null)"; then printf true; else printf false; fi)
BUILD_LDFLAGS = \
	-X shh-h/internal/buildinfo.Version=$(VERSION) \
	-X shh-h/internal/buildinfo.Commit=$(COMMIT) \
	-X shh-h/internal/buildinfo.BuildDate=$(BUILD_DATE) \
	-X shh-h/internal/buildinfo.Dirty=$(DIRTY)

.PHONY: build run dev test test-go test-frontend lint tidy clean

build:
	cd $(WAILS_PROJECT) && ../../bin/wails build -clean -trimpath -ldflags "$(BUILD_LDFLAGS)" -o $(APP_NAME)

run:
	cd $(WAILS_PROJECT) && ../../bin/wails dev

dev: run

test: test-go test-frontend

test-go:
	go test ./...

test-frontend:
	cd frontend && npm test

lint:
	cd frontend && npm run lint
	go vet ./...

tidy:
	go mod tidy

clean:
	rm -rf build/bin internal/frontendassets/dist/assets internal/frontendassets/dist/index.html frontend/coverage coverage.out
