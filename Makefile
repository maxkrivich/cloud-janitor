.PHONY: build test lint clean run help docker docker-build docker-run

# Build variables
BINARY_NAME=cloud-janitor
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT?=$(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
BUILD_DATE?=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS=-ldflags "-X github.com/maxkrivich/cloud-janitor/cmd.version=$(VERSION) -X github.com/maxkrivich/cloud-janitor/cmd.commit=$(COMMIT) -X github.com/maxkrivich/cloud-janitor/cmd.buildDate=$(BUILD_DATE)"

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=$(GOCMD) fmt

# Default target
all: lint test build

## help: Show this help message
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'

## build: Build the binary
build:
	$(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME) .

## test: Run all tests
test:
	$(GOTEST) -v -race -cover ./...

## test-coverage: Run tests with coverage report
test-coverage:
	$(GOTEST) -v -race -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

## lint: Run linter
lint:
	@which golangci-lint > /dev/null || (echo "Installing golangci-lint..." && go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
	$(shell go env GOPATH)/bin/golangci-lint run ./...

## fmt: Format code
fmt:
	$(GOFMT) ./...

## tidy: Tidy and verify dependencies
tidy:
	$(GOMOD) tidy
	$(GOMOD) verify

## clean: Clean build artifacts
clean:
	rm -f $(BINARY_NAME)
	rm -f coverage.out coverage.html

## run: Build and run the binary
run: build
	./$(BINARY_NAME)

## install: Install the binary to GOPATH/bin
install:
	$(GOCMD) install $(LDFLAGS) .

## deps: Download dependencies
deps:
	$(GOMOD) download

## update: Update dependencies
update:
	$(GOMOD) get -u ./...
	$(GOMOD) tidy

## scan: Run a quick scan (for development)
scan: build
	./$(BINARY_NAME) scan --dry-run

## version: Show version information
version: build
	./$(BINARY_NAME) version

# Docker variables
DOCKER_IMAGE=cloud-janitor
DOCKER_TAG?=$(VERSION)

## docker: Build Docker image (alias for docker-build)
docker: docker-build

## docker-build: Build Docker image
docker-build:
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(DOCKER_IMAGE):$(DOCKER_TAG) \
		-t $(DOCKER_IMAGE):latest \
		.

## docker-run: Run cloud-janitor in Docker (pass args via ARGS="...")
docker-run:
	docker run --rm \
		-e AWS_ACCESS_KEY_ID \
		-e AWS_SECRET_ACCESS_KEY \
		-e AWS_SESSION_TOKEN \
		-e AWS_REGION \
		-e AWS_PROFILE \
		-v ~/.aws:/root/.aws:ro \
		$(DOCKER_IMAGE):latest $(ARGS)
