export GO111MODULE=on
export PATH:=/usr/local/go/bin:$(PATH)

BINARY_NAME := cloud-provider-opentelekomcloud
BIN_DIR := bin
# component-base requires a vX.Y.Z-prefixed version string
VERSION ?= $(shell git describe --tags 2>/dev/null || echo "v0.0.0-$(shell git rev-parse --short HEAD 2>/dev/null || echo dev)")
COMMIT ?= $(shell git rev-parse HEAD 2>/dev/null || echo unknown)
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
# Stamp k8s.io/component-base/version so --version reports the real build.
LDFLAGS := -X k8s.io/component-base/version.gitVersion=$(VERSION) \
	-X k8s.io/component-base/version.gitCommit=$(COMMIT) \
	-X k8s.io/component-base/version.buildDate=$(BUILD_DATE)

default: build

build:
	@echo "Building $(BINARY_NAME) $(VERSION)"
	@mkdir -p $(BIN_DIR)
	@go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY_NAME) ./cmd/cloud-controller-manager

test:
	@echo "Running unit tests"
	@go test ./... -v -count=1

test-e2e:
	@echo "Running e2e tests against the current KUBECONFIG cluster"
	@go test -tags e2e ./test/e2e/ -v -count=1 -timeout 40m

test-smoke:
	@echo "Running cloud smoke test (clouds.yaml / OS_* credentials)"
	@go test -tags smoke ./test/smoke/ -v -count=1 -timeout 20m

image:
	@echo "Building container image $(BINARY_NAME):$(VERSION)"
	@docker build --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) \
		-t $(BINARY_NAME):$(VERSION) .

fmt:
	@echo "Running go fmt"
	@go fmt ./...

lint:
	@echo "Running golangci-lint"
	@golangci-lint run --timeout=300s

vet:
	@echo "Running go vet"
	@go vet ./...

clean:
	@rm -rf $(BIN_DIR)

.PHONY: build test test-e2e test-smoke image fmt lint vet clean
