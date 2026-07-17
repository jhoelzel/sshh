APP_NAME := shhh
WAILS_PROJECT := cmd/shhh

.PHONY: build run dev test test-go test-frontend lint tidy clean

build:
	cd $(WAILS_PROJECT) && ../../bin/wails build -clean -o $(APP_NAME)

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
