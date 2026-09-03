# Makefile Patterns

Reference file for the bash skill. Read this when writing or reviewing Makefiles.

## Fundamentals

### Basic Structure

```makefile
# Variables at the top
APP_NAME := myapp
VERSION  := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_DIR := ./build

# Default target (first target in the file)
.PHONY: all
all: build

# Actual targets
.PHONY: build
build:
	go build -ldflags "-X main.version=$(VERSION)" -o $(BUILD_DIR)/$(APP_NAME) ./cmd/$(APP_NAME)
```

### Key Rules

1. **Always declare `.PHONY`** for targets that are not files. Without it, `make` skips the target if a file with that name exists.
2. **Use `:=` for immediate assignment**, `=` for recursive (lazy) assignment. Prefer `:=` to avoid surprises.
3. **Use `?=` for user-overridable defaults**: `PORT ?= 8080` lets users run `make run PORT=3000`.
4. **Indent with tabs, not spaces.** Make requires tabs for recipe lines. This is non-negotiable.
5. **One responsibility per target.** Keep targets focused.

---

## Variables

### Assignment Types

```makefile
# Immediate — evaluated once at parse time
CC := gcc

# Recursive — evaluated every time the variable is used
CC = gcc

# Conditional — set only if not already defined
CC ?= gcc

# Append
CFLAGS += -Wall
```

### Automatic Variables

| Variable | Meaning | Example |
|----------|---------|---------|
| `$@` | Target name | The file being built |
| `$<` | First prerequisite | The first dependency |
| `$^` | All prerequisites | All dependencies (deduped) |
| `$?` | Prerequisites newer than target | Changed deps only |
| `$*` | Stem of pattern match | `%` in pattern rules |

### Shell Variables vs Make Variables

```makefile
# Make variable — use $(VAR) or ${VAR}
GREETING := hello

# Shell variable inside a recipe — use $$VAR
.PHONY: example
example:
	name="world"; echo "$(GREETING) $$name"
```

---

## Common Targets

### Standard Project Makefile

```makefile
APP_NAME  := myapp
BUILD_DIR := ./build
GO_FILES  := $(shell find . -name '*.go' -not -path './vendor/*')

.PHONY: all build run test lint clean fmt help

all: lint test build  ## Build after lint and test pass

build:  ## Build the application
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(APP_NAME) ./cmd/$(APP_NAME)

run:  ## Run the application locally
	go run ./cmd/$(APP_NAME)

test:  ## Run all tests
	go test -race -count=1 ./...

lint:  ## Run linters
	golangci-lint run ./...
	shellcheck scripts/*.sh

fmt:  ## Format code
	gofmt -w $(GO_FILES)
	goimports -w $(GO_FILES)

clean:  ## Remove build artifacts
	rm -rf $(BUILD_DIR)
	go clean -cache

help:  ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*##' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'
```

### Self-Documenting Help

The `help` target above scans for `## comments` after target names. Always add `## description` to public targets.

---

## Docker Integration

```makefile
DOCKER     := docker
IMAGE_NAME := myapp
IMAGE_TAG  := $(VERSION)
REGISTRY   := us-docker.pkg.dev/$(GCP_PROJECT)/$(REPO_NAME)

.PHONY: docker-build docker-push docker-run

docker-build:  ## Build Docker image
	$(DOCKER) build -t $(IMAGE_NAME):$(IMAGE_TAG) .
	$(DOCKER) tag $(IMAGE_NAME):$(IMAGE_TAG) $(IMAGE_NAME):latest

docker-push:  ## Push to Artifact Registry
	$(DOCKER) tag $(IMAGE_NAME):$(IMAGE_TAG) $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)
	$(DOCKER) push $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)

docker-run:  ## Run container locally
	$(DOCKER) run --rm -p 8080:8080 $(IMAGE_NAME):$(IMAGE_TAG)
```

---

## Multi-Platform Builds

```makefile
GOOS   ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

.PHONY: build-all
build-all: build-linux-amd64 build-linux-arm64 build-darwin-arm64

build-linux-amd64:
	GOOS=linux GOARCH=amd64 go build -o $(BUILD_DIR)/$(APP_NAME)-linux-amd64 ./cmd/$(APP_NAME)

build-linux-arm64:
	GOOS=linux GOARCH=arm64 go build -o $(BUILD_DIR)/$(APP_NAME)-linux-arm64 ./cmd/$(APP_NAME)

build-darwin-arm64:
	GOOS=darwin GOARCH=arm64 go build -o $(BUILD_DIR)/$(APP_NAME)-darwin-arm64 ./cmd/$(APP_NAME)
```

---

## Environment & Config

### Loading .env Files

```makefile
# Load .env if it exists (don't fail if missing)
-include .env
export

# Or explicitly:
ifneq (,$(wildcard ./.env))
  include .env
  export
endif
```

### Environment-Specific Targets

```makefile
ENV ?= development

.PHONY: deploy
deploy:  ## Deploy to the target environment
ifeq ($(ENV),production)
	@echo "Deploying to production..."
	gcloud run deploy $(APP_NAME) --image=$(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG) --region=us-central1
else
	@echo "Deploying to staging..."
	gcloud run deploy $(APP_NAME)-staging --image=$(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG) --region=us-central1
endif
```

---

## Pattern Rules

```makefile
# Build any .o from a .c
%.o: %.c
	$(CC) $(CFLAGS) -c $< -o $@

# Generate Go mocks from interfaces
mocks/%_mock.go: internal/%.go
	mockgen -source=$< -destination=$@ -package=mocks
```

---

## Guards & Prerequisites

### Check Required Tools

```makefile
REQUIRED_BINS := go docker shellcheck
$(foreach bin,$(REQUIRED_BINS),\
  $(if $(shell command -v $(bin) 2>/dev/null),,$(error "$(bin) is required but not installed")))
```

### Check Required Variables

```makefile
.PHONY: check-env
check-env:
ifndef GCP_PROJECT
	$(error GCP_PROJECT is not set. Run: export GCP_PROJECT=your-project)
endif
ifndef REGISTRY
	$(error REGISTRY is not set)
endif
```

---

## Tips

- **Silence noise** with `@` prefix: `@echo "Building..."` hides the echo command itself.
- **Continue on error** with `-` prefix: `-rm -f file.txt` won't fail if the file doesn't exist.
- **Multi-line recipes** use `\` continuation, but each logical line runs in a separate shell. Use `.ONESHELL:` or chain with `&&` if you need shared state.
- **Parallel execution**: `make -j4` runs independent targets in parallel. Ensure your dependency graph is correct.
- **Recursive make is fragile**: prefer a single top-level Makefile with includes over `$(MAKE) -C subdir`.
