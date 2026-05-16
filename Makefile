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

# Auxiliary tools that are easier to run in their pinned image directly.
LINT_IMAGE      ?= golangci/golangci-lint:v2.11.4-alpine

DOCKER_MOUNT = -v $(CURDIR):/app -w /app

# Toolchain shim. Detect host Go 1.25.x; otherwise shell through compose.
HOST_GO_OK := $(shell command -v go >/dev/null 2>&1 && go version 2>/dev/null | grep -qE 'go1\.25(\.|$$|[[:space:]])' && echo yes)

ifeq ($(HOST_GO_OK),yes)
  GO       = go
  GOFMT    = gofmt
else
  DOCKER_GO = $(COMPOSE) -p $(COMPOSE_PROJECT) --profile dev run --rm --no-deps go
  GO       = $(DOCKER_GO) go
  GOFMT    = $(DOCKER_GO) gofmt
endif

# ---- targets -------------------------------------------------------------

.PHONY: help go-version fmt fmt-check vet test test-race lint lint-ci mod-tidy mod-tidy-check build clean doctor image-dev codegen codegen-verify ci

help: ## Show targets and tooling policy
	@echo "openehr-sdk-go — Makefile"
	@echo ""
	@if [ "$(HOST_GO_OK)" = "yes" ]; then \
		echo "Policy: host Go 1.25.x active — fast path."; \
		echo "  detected: $$(go version 2>/dev/null)"; \
	else \
		echo "Policy: host Go 1.25.x NOT detected — Docker fallback via compose 'dev' profile."; \
		echo "  toolchain: $(DOCKER_GO) <cmd>"; \
		echo "  build once: make image-dev"; \
	fi
	@echo ""
	@echo "Auxiliary images (bump when stable releases ship):"
	@echo "  LINT_IMAGE=$(LINT_IMAGE)"
	@echo ""
	@grep -hE '^[a-zA-Z0-9_-]+:.*?##' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

doctor: ## Diagnose toolchain availability
	@echo "host go         : $$(command -v go || echo 'not installed')"
	@echo "host go version : $$(go version 2>/dev/null || echo 'n/a')"
	@echo "docker          : $$(command -v docker || echo 'not installed')"
	@echo "docker compose  : $$(docker compose version 2>/dev/null | head -1 || echo 'n/a')"
	@echo "active GO       : $(GO)"

go-version: ## Print Go version in the active toolchain
	@$(GO) version

image-dev: ## Build the dev toolchain image (Dockerfile dev stage)
	@$(COMPOSE) -p $(COMPOSE_PROJECT) --profile dev build go

fmt: ## gofmt -w -s on the whole tree
	@$(GOFMT) -w -s .

fmt-check: ## Fail if any file needs gofmt -s
	@files=$$($(GOFMT) -l -s .); \
	if [ -n "$$files" ]; then \
		echo "gofmt -s needed:"; \
		echo "$$files"; \
		exit 1; \
	fi

vet: ## go vet ./...
	@$(GO) vet ./...

codegen: ## Run the BMM-driven code generator (RM + AOM 1.4)
	@$(GO) run ./cmd/bmmgen -resources ./resources/bmm -out .

codegen-verify: ## Verify generated code is in sync with BMM sources (RM + AOM 1.4)
	@$(GO) run ./cmd/bmmgen -resources ./resources/bmm -out . -verify

test: codegen-verify ## go test ./... -count=1 (also verifies BMM-generated code is in sync)
	@$(GO) test ./... -count=1

test-race: ## go test -race ./...
	@$(GO) test -race -count=1 ./...

lint-ci: ## golangci-lint run ./... (host binary if present, else Docker LINT_IMAGE)
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		docker run --rm $(DOCKER_MOUNT) $(LINT_IMAGE) golangci-lint run ./...; \
	fi

lint: lint-ci ## golangci-lint run ./...

mod-tidy: ## go mod tidy
	@$(GO) mod tidy

mod-tidy-check: ## Fail if go mod tidy would change go.mod or go.sum
	@$(GO) mod tidy
	@git diff --exit-code go.mod
	@if test -f go.sum; then git diff --exit-code go.sum; fi

ci: fmt-check mod-tidy-check vet test lint build ## Full PR gate (test includes codegen-verify; excludes test-race)

build: ## go build ./... (compile every package; primarily for examples in cmd/)
	@$(GO) build ./...

clean: ## Remove build artefacts
	@rm -rf bin/ coverage.* *.out
