.DEFAULT_GOAL := help

# Project variables
BINARY_NAME=plexctl
VERSION ?= 0.0.0
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Build flags
LDFLAGS := -X 'github.com/ygelfand/plexctl/internal/config.Version=$(VERSION)' \
    -X 'github.com/ygelfand/plexctl/internal/config.GitCommit=$(GIT_COMMIT)' \
    -X 'github.com/ygelfand/plexctl/internal/config.BuildDate=$(BUILD_DATE)'

##@ Development

.PHONY: build
build: ## Build the plexctl binary
	go fmt ./...
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY_NAME) main.go

.PHONY: run
run: ## Run the plexctl binary
	go run main.go

.PHONY: test
test: ## Run tests
	go test -v ./...

.PHONY: lint
lint: ## Run linter
	golangci-lint run

.PHONY: icons-gen
icons-gen: ## Generate Nerd Font icons list
	go run internal/tools/genicons/main.go

##@ Documentation

.PHONY: docs-gen
docs-gen: ## Generate CLI markdown documentation
	go run internal/tools/gendocs/main.go

.PHONY: docs-dev
docs-dev: docs-gen ## Run Nextra documentation site in dev mode
	cd docs && npm run dev

.PHONY: docs-lint
docs-lint: ## Run ESLint on the documentation site
	cd docs && npm run lint

.PHONY: docs-build
docs-build: docs-gen ## Build Nextra documentation site for production
	cd docs && npm run build

##@ Build & Release

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf bin/ docs/pages/cli/*

.PHONY: help
help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)
