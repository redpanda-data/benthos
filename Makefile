.PHONY: deps test test-race fmt lint

GOMAXPROCS ?= 1

build:
	@go build ./cmd/benthos

build-wasm:
	@GOOS=js GOARCH=wasm go build -o benthos.wasm ./cmd/benthos

export GOBIN ?= $(CURDIR)/bin
export PATH  := $(GOBIN):$(PATH)

include .versions

install-tools:
	@go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v$(GOLANGCI_LINT_VERSION)

install:
	@go install ./cmd/benthos

deps:
	@go mod tidy

fmt:
	@golangci-lint fmt cmd/... internal/... public/...
	@go mod tidy

lint:
	@golangci-lint run cmd/... internal/... public/...

test:
	@go test -timeout 3m ./...
	@go run ./cmd/benthos template lint $(TEMPLATE_FILES)
	@go run ./cmd/benthos test ./config/test/...

test-race:
	@go test -timeout 3m -race ./...
