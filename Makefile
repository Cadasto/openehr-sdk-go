# openehr-sdk-go — host-first Go toolchain, Docker fallback for CI parity.
#
# Policy in AGENTS.md > Tooling policy. Single entry point — extend here,
# don't add ad-hoc scripts.
#
# Fast path  : host Go 1.25.x (recommended for daily development).
# Fallback   : `docker compose run --rm go …` using the `dev` stage in
#              Dockerfile (gated behind the `dev` compose profile).
.DEFAULT_GOAL := help

# ---- variables -----------------------------------------------------------

COMPOSE         ?= docker compose
COMPOSE_PROJECT ?= openehr-sdk-go
LINT_IMAGE      ?= golangci/golangci-lint:v2.11.4-alpine
DOCKER_MOUNT    = -v $(CURDIR):/app -w /app

HOST_GO_OK := $(shell command -v go >/dev/null 2>&1 && go version 2>/dev/null | grep -qE 'go1\.25(\.|$$|[[:space:]])' && echo yes)

ifeq ($(HOST_GO_OK),yes)
  GO    = go
  GOFMT = gofmt
else
  DOCKER_GO = $(COMPOSE) -p $(COMPOSE_PROJECT) --profile dev run --rm --no-deps go
  GO    = $(DOCKER_GO) go
  GOFMT = $(DOCKER_GO) gofmt
endif

# Grouped help (##@ section, target: ## description). Keep targets in this
# order so `make help` lists them in the same sequence.
define PRINT_HELP
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} \
		/^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5); next } \
		/^[a-zA-Z0-9_-]+:.*?##/ && $$1 != "help" { printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2 }' \
		$(MAKEFILE_LIST)
endef

.PHONY: help doctor go-version image-dev \
        fmt fmt-check vet \
        codegen codegen-verify \
        test test-race \
        lint lint-ci \
        mod-tidy mod-tidy-check \
        spec-check \
        build clean \
        ci

# ---- help & toolchain ----------------------------------------------------

help: ## Show grouped targets and tooling policy
	@echo "openehr-sdk-go"
	@echo ""
	@if [ "$(HOST_GO_OK)" = "yes" ]; then \
		echo "Toolchain : host Go 1.25.x (fast path)"; \
		echo "  $$(go version 2>/dev/null)"; \
	else \
		echo "Toolchain : Docker fallback (compose profile dev)"; \
		echo "  run once: make image-dev"; \
		echo "  shim    : $(DOCKER_GO) <cmd>"; \
	fi
	@echo "Lint image: $(LINT_IMAGE)"
	$(PRINT_HELP)
	@echo ""
	@echo "PR gate   : make ci"
	@echo ""

##@ Toolchain

doctor: ## Diagnose host Go, Docker, and active toolchain shim
	@echo "host go         : $$(command -v go || echo 'not installed')"
	@echo "host go version : $$(go version 2>/dev/null || echo 'n/a')"
	@echo "docker          : $$(command -v docker || echo 'not installed')"
	@echo "docker compose  : $$(docker compose version 2>/dev/null | head -1 || echo 'n/a')"
	@echo "active GO       : $(GO)"

go-version: ## Print Go version from the active toolchain
	@$(GO) version

image-dev: ## Build the dev toolchain image (Dockerfile dev stage)
	@$(COMPOSE) -p $(COMPOSE_PROJECT) --profile dev build go

##@ Format & analyze

fmt: ## Apply gofmt -w -s to the tree
	@$(GOFMT) -w -s .

fmt-check: ## Fail if any file needs gofmt -s
	@files=$$($(GOFMT) -l -s .); \
	if [ -n "$$files" ]; then \
		echo "gofmt -s needed:"; \
		echo "$$files"; \
		exit 1; \
	fi

vet: ## Run go vet ./...
	@$(GO) vet ./...

##@ Codegen

codegen: ## Regenerate RM and AOM 1.4 from pinned BMM sources
	@$(GO) run ./cmd/bmmgen -resources ./resources/bmm -out .

codegen-verify: ## Fail if generated code drifts from resources/bmm
	@$(GO) run ./cmd/bmmgen -resources ./resources/bmm -out . -verify

##@ Test

test: codegen-verify ## Run unit tests (includes codegen drift check)
	@$(GO) test ./... -count=1

test-race: ## Run unit tests with -race (main-branch CI job)
	@$(GO) test -race -count=1 ./...

##@ Lint

lint-ci: ## Run golangci-lint (host binary or pinned Docker image)
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		docker run --rm $(DOCKER_MOUNT) $(LINT_IMAGE) golangci-lint run ./...; \
	fi

lint: lint-ci ## Alias for lint-ci

##@ Modules

mod-tidy: ## Run go mod tidy
	@$(GO) mod tidy

mod-tidy-check: ## Fail if go mod tidy would change go.mod or go.sum
	@$(GO) mod tidy
	@git diff --exit-code go.mod
	@if test -f go.sum; then git diff --exit-code go.sum; fi

##@ Specs

spec-check: ## Verify docs/specifications/traceability.yaml against repo artefacts
	@bash scripts/spec-check.sh

##@ Build

build: ## Compile all packages (cmd/examples when present)
	@$(GO) build ./...

clean: ## Remove bin/, coverage artefacts, and *.out files
	@rm -rf bin/ coverage.* *.out

##@ CI

ci: fmt-check mod-tidy-check vet test lint spec-check build ## Full local PR gate (see docs/ci.md)
