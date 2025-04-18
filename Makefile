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
	@go list -f {{.Dir}} ./... | xargs -I{} gofmt -w -s {}
	@go list -f {{.Dir}} ./... | xargs -I{} goimports -w -local github.com/redpanda-data/benthos/v4 {}
	@go mod tidy

lint:
	@go vet ./...
	@golangci-lint -j $(GOMAXPROCS) run --timeout 5m internal/... public/...

test:
	@go test -timeout 3m ./...
	@go run ./cmd/benthos template lint $(TEMPLATE_FILES)
	@go run ./cmd/benthos test ./config/test/...

test-race:
	@go test -timeout 3m -race ./...
