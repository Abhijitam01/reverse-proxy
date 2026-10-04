.PHONY: build test clean run fmt lint

BINARY=reverse-proxy-go
MAIN=cmd/server/main.go

build:
	@echo "Building $(BINARY)..."
	@go build -o bin/$(BINARY) ./cmd/server
	@echo "Binary created at bin/$(BINARY)"

test:
	@echo "Running tests..."
	@go test -v -race -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated at coverage.html"

test-short:
	@echo "Running short tests..."
	@go test -v -short ./...

fmt:
	@echo "Formatting code..."
	@go fmt ./...

lint:
	@echo "Linting code..."
	@golangci-lint run ./... || true

clean:
	@echo "Cleaning..."
	@rm -f bin/$(BINARY)
	@rm -f coverage.out coverage.html
	@go clean

run: build
	@echo "Running $(BINARY)..."
	@./bin/$(BINARY) -config config.example.json

vet:
	@echo "Running go vet..."
	@go vet ./...

deps:
	@echo "Downloading dependencies..."
	@go mod download
	@go mod tidy
