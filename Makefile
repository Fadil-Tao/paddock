VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
GOFLAGS := -trimpath
BIN     := bin

export CGO_ENABLED := 0

.PHONY: all build test e2e lint fmt tidy assets pull image clean

all: lint test build

build: ## Build paddock and paddock-agent into ./bin
	go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BIN)/ ./cmd/...

test: ## Unit tests (no KVM needed)
	CGO_ENABLED=1 go test -race ./...

e2e: build ## End-to-end tests; boots real VMs, needs /dev/kvm and assets
	CGO_ENABLED=1 go test -race -tags e2e -count=1 ./test/e2e/...

lint: ## Lint and check formatting
	golangci-lint run

fmt: ## Apply formatters
	golangci-lint fmt

tidy:
	go mod tidy

assets: pull image ## Everything `make e2e` needs

pull: build ## Download firecracker + kernel (sha256-verified)
	$(BIN)/paddock pull

image: build ## Build the base rootfs from images/ubuntu (needs podman or docker, mke2fs)
	$(BIN)/paddock image build images/ubuntu

clean:
	rm -rf $(BIN)
