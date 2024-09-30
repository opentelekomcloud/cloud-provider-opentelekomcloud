export GO111MODULE=on
export PATH:=/usr/local/go/bin:$(PATH)
exec_path := /usr/local/bin/
exec_name := cloud-provider-opentelekomcloud


default: linters
linters: lint

fmt:
	@echo Running go fmt
	@go fmt

lint:
	@echo Running go lint
	@golangci-lint run --timeout=300s

vet:
	@echo "go vet ."
	@go vet ./...
