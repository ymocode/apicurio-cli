.PHONY: build clean test install help lint fmt vet docker-build docker-run release deploy

BINARY_NAME=apicurio-client
BUILD_DIR=bin
DOCKER_IMAGE=apicurio-client
CMD_PATH=./cmd/apicurio-client
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-X main.Version=$(VERSION)"
LDFLAGS_TINY=-ldflags "-s -w -X main.Version=$(VERSION)"

# GitLab Package Registry settings
GITLAB_URL=""
GITLAB_PROJECT_ID=3998
GITLAB_TOKEN?=$(PRIVATE_TOKEN)

help: ## Show this help message
	@echo "Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-15s %s\n", $$1, $$2}'

build: ## Build the CLI binary
	@echo "Building $(BINARY_NAME) version $(VERSION)..."
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_PATH)

build-tiny: ## Build tiny binary (stripped + UPX compressed)
	@echo "Building tiny $(BINARY_NAME) version $(VERSION)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build $(LDFLAGS_TINY) -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_PATH)
	@command -v upx >/dev/null 2>&1 || { echo "upx not installed"; exit 1; }
	upx --best --lzma $(BUILD_DIR)/$(BINARY_NAME)

build-all: ## Build for all platforms
	@echo "Building for all platforms..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(CMD_PATH)
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 $(CMD_PATH)
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(CMD_PATH)
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(CMD_PATH)
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(CMD_PATH)
	GOOS=windows GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-arm64.exe $(CMD_PATH)

deploy: build-tiny ## Build tiny and upload to GitLab Package Registry
	@if [ -z "$(GITLAB_TOKEN)" ]; then echo "Error: PRIVATE_TOKEN not set"; exit 1; fi
	@echo "Uploading $(BINARY_NAME) version $(VERSION) to GitLab..."
	curl --fail --header "PRIVATE-TOKEN: $(GITLAB_TOKEN)" \
		--upload-file $(BUILD_DIR)/$(BINARY_NAME) \
		"$(GITLAB_URL)/api/v4/projects/$(GITLAB_PROJECT_ID)/packages/generic/$(BINARY_NAME)/$(VERSION)/$(BINARY_NAME)"
	@echo "\nDeployed $(BINARY_NAME) version $(VERSION)"

clean: ## Remove build artifacts
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@rm -f coverage.out coverage.html gosec-report.json

test: ## Run tests
	@echo "Running tests..."
	go test -v -race -coverprofile=coverage.out ./...

coverage: test ## Generate and view coverage report
	@echo "Generating coverage report..."
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

install: build ## Install binary to GOPATH/bin
	@echo "Installing $(BINARY_NAME) to GOPATH/bin..."
	go install $(LDFLAGS) $(CMD_PATH)

fmt: ## Format code
	@echo "Formatting code..."
	go fmt ./...

vet: ## Run go vet
	@echo "Running go vet..."
	go vet ./...

lint: ## Run golangci-lint
	@echo "Running golangci-lint..."
	@command -v golangci-lint >/dev/null 2>&1 || { echo "golangci-lint not installed. Install from https://golangci-lint.run/"; exit 1; }
	golangci-lint run --timeout=5m

deps: ## Download dependencies
	@echo "Downloading dependencies..."
	go mod download
	go mod tidy

deps-update: ## Update dependencies
	@echo "Updating dependencies..."
	go get -u ./...
	go mod tidy

security: ## Run security scan
	@echo "Running security scan..."
	@command -v gosec >/dev/null 2>&1 || { echo "gosec not installed. Install with: go install github.com/securego/gosec/v2/cmd/gosec@latest"; exit 1; }
	gosec -no-fail -fmt json -out gosec-report.json ./...
	@echo "Security report: gosec-report.json"

docker-build: ## Build Docker image
	@echo "Building Docker image..."
	docker build -t $(DOCKER_IMAGE):$(VERSION) -t $(DOCKER_IMAGE):latest .

docker-run: docker-build ## Run Docker container
	docker run --rm $(DOCKER_IMAGE):latest

docker-shell: docker-build ## Run Docker container with shell
	docker run --rm -it --entrypoint /bin/sh $(DOCKER_IMAGE):latest

release-dry-run: build-all ## Simulate release process
	@echo "Creating release checksums..."
	@cd $(BUILD_DIR) && sha256sum $(BINARY_NAME)-* > checksums.txt
	@cat $(BUILD_DIR)/checksums.txt

ci: lint vet test ## Run all CI checks locally

pre-commit: fmt lint vet test ## Run pre-commit checks

.DEFAULT_GOAL := help
