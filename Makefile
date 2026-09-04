BINARY   ?= cozyphi
MAIN_SRC  = ./cmd

GOBIN    ?= $(shell go env GOBIN)
GOPATH   ?= $(shell go env GOPATH)
ifeq ($(GOBIN),)
GOBIN     = $(GOPATH)/bin
endif

GO       ?= go
GOFLAGS  ?= -ldflags="-s -w"
CGO      ?= 0

.PHONY: all build install run clean test test-race cover fmt fmt-check lint lint-install help

all: build

build:
	CGO_ENABLED=$(CGO) $(GO) build $(GOFLAGS) -o $(BINARY) $(MAIN_SRC)

install: build
	@mkdir -p $(GOBIN)
	mv $(BINARY) $(GOBIN)/$(BINARY)
	@echo "installed $(BINARY) -> $(GOBIN)/$(BINARY)"

run: build
	./$(BINARY)

clean:
	rm -f $(BINARY)
	$(GO) clean

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

# Run tests with coverage; leaves coverage.out behind and prints the total.
cover:
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out | tail -n 1

# Apply gofumpt / goimports / golines via .golangci.yml formatters.
fmt:
	golangci-lint fmt ./...

# Fail if formatting would change files (used by CI).
fmt-check:
	golangci-lint fmt --diff ./...

lint:
	golangci-lint run ./...

# One pinned version for CI and the local binary; see .golangci-lint-version.
lint-install:
	$(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(shell cat .golangci-lint-version)

help:
	@echo "Usage:"
	@echo "  make          - build binary ($(BINARY))"
	@echo "  make install  - build & install to \$$GOBIN ($(GOBIN))"
	@echo "  make run      - build & run"
	@echo "  make clean    - remove binary & cache"
	@echo "  make test     - run all tests"
	@echo "  make test-race - run all tests under the race detector"
	@echo "  make cover     - run tests with coverage (coverage.out, total printed)"
	@echo "  make fmt      - format Go sources (gofumpt/goimports/golines)"
	@echo "  make fmt-check - check formatting without writing (CI)"
	@echo "  make lint     - run golangci-lint"
	@echo "  make lint-install - install the golangci-lint version CI pins"
