export GO111MODULE=on
export PATH:=/usr/local/go/bin:$(PATH)

BINARY_NAME := cloud-provider-opentelekomcloud
BIN_DIR := bin

default: build

build:
	@echo "Building $(BINARY_NAME)"
	@mkdir -p $(BIN_DIR)
	@go build -o $(BIN_DIR)/$(BINARY_NAME) ./cmd/cloud-controller-manager

test:
	@echo "Running unit tests"
	@go test ./... -v -count=1

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

.PHONY: build test fmt lint vet clean
