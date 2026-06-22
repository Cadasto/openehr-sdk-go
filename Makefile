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

# ANTLR codegen (maintainer-only). The generator is Java, confined to the
# Dockerfile `antlr` stage; the runtime is pure Go. Keep ANTLR_VERSION in
# lockstep with the antlr4-go/antlr runtime in go.mod (see grammar PIN).
ANTLR_VERSION   ?= 4.13.2
ANTLR_IMAGE     ?= openehr-sdk-go/antlr:$(ANTLR_VERSION)
AQL_GRAMMAR_DIR := resources/aql/grammar/active
AQL_GEN_DIR     := openehr/aql/parse/gen

HOST_GO_OK   := $(shell command -v go >/dev/null 2>&1 && go version 2>/dev/null | grep -qE 'go1\.25(\.|$$|[[:space:]])' && echo yes)
HOST_GLCI_OK := $(shell command -v golangci-lint >/dev/null 2>&1 && echo yes)

ifeq ($(HOST_GO_OK),yes)
  GO = go
else
  DOCKER_GO = $(COMPOSE) -p $(COMPOSE_PROJECT) --profile dev run --rm --no-deps go
  GO        = $(DOCKER_GO) go
endif

# golangci-lint shim — host binary (fast path) or the pinned image, which
# bundles the v2 formatters (gofumpt + goimports), so `fmt` and `lint` share
# one pinned toolchain. --user keeps rewritten files owned by the host user.
ifeq ($(HOST_GLCI_OK),yes)
  GOLANGCI = golangci-lint
else
  GOLANGCI = docker run --rm $(DOCKER_MOUNT) --user $$(id -u):$$(id -g) \
             -e HOME=/tmp -e GOCACHE=/tmp/.gocache -e GOLANGCI_LINT_CACHE=/tmp/.glcache \
             $(LINT_IMAGE) golangci-lint
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
        codegen codegen-verify antlr-image aqlgen aqlgen-verify \
        its-rest-sync its-rest-check \
        test test-race \
        lint lint-ci \
        mod-tidy mod-tidy-check \
        spec-check spec-context probe-status \
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

fmt: ## Apply gofumpt + goimports via golangci-lint (formatters in .golangci.yml)
	@$(GOLANGCI) fmt ./...

fmt-check: ## Fail if any file needs formatting (gofumpt/goimports)
	@# Key off the exit code, not captured output: `fmt --diff` exits non-zero
	@# and writes the diff to stdout when reformatting is needed. Capturing
	@# stderr here would also swallow Docker image-pull progress (cold runner)
	@# into the check and fail spuriously.
	@$(GOLANGCI) fmt --diff ./... || { \
		echo "formatting needed: run 'make fmt'"; \
		exit 1; \
	}

vet: ## Run go vet (excludes the generated ANTLR parser)
	@# The generated parser (openehr/aql/parse/gen) emits unreachable code after
	@# panics — inherent to ANTLR's Go target, not a defect. golangci-lint already
	@# skips generated files (generated: lax); mirror that for plain `go vet`.
	@$(GO) vet $$($(GO) list ./... | grep -v '/openehr/aql/parse/gen$$')

##@ Codegen

codegen: ## Regenerate RM and AOM 1.4 from pinned BMM sources
	@$(GO) run ./cmd/bmmgen -resources ./resources/bmm -out .

codegen-verify: ## Fail if generated code drifts from resources/bmm
	@$(GO) run ./cmd/bmmgen -resources ./resources/bmm -out . -verify

antlr-image: ## Build the ANTLR codegen image (maintainer-only; needs Docker + network)
	@docker build --target antlr --build-arg ANTLR_VERSION=$(ANTLR_VERSION) -t $(ANTLR_IMAGE) .

aqlgen: antlr-image ## Regenerate the AQL parser from active/ grammar (maintainer-only; needs Docker)
	@docker run --rm -v $(CURDIR):/app -w /app/$(AQL_GRAMMAR_DIR) --user $$(id -u):$$(id -g) \
	  $(ANTLR_IMAGE) -Dlanguage=Go -o /app/$(AQL_GEN_DIR).tmp -package gen AqlLexer.g4 AqlParser.g4
	@rm -f $(AQL_GEN_DIR)/*.go && cp $(AQL_GEN_DIR).tmp/*.go $(AQL_GEN_DIR)/ && rm -rf $(AQL_GEN_DIR).tmp
	@echo "regenerated $(AQL_GEN_DIR)/ from $(AQL_GRAMMAR_DIR)/"

aqlgen-verify: antlr-image ## Fail if the committed AQL parser drifts from active/ grammar
	@docker run --rm -v $(CURDIR):/app -w /app/$(AQL_GRAMMAR_DIR) --user $$(id -u):$$(id -g) \
	  $(ANTLR_IMAGE) -Dlanguage=Go -o /app/$(AQL_GEN_DIR).verify -package gen AqlLexer.g4 AqlParser.g4
	@status=0; for f in $(AQL_GEN_DIR)/*.go; do \
	  diff -u "$$f" "$(AQL_GEN_DIR).verify/$$(basename $$f)" || status=1; \
	done; \
	rm -rf $(AQL_GEN_DIR).verify; \
	if [ $$status -ne 0 ]; then echo "aqlgen-verify: AQL parser drifts from active/ — run 'make aqlgen'"; exit 1; fi; \
	echo "aqlgen-verify: OK"

##@ Resources

its-rest-sync: ## Vendor openEHR ITS-REST OpenAPI specs into resources/its-rest/ (needs network; ITS_REST_REF to pin)
	@./scripts/sync-its-rest-specs.sh sync

its-rest-check: ## Verify vendored ITS-REST specs match MANIFEST + report upstream drift (needs network)
	@./scripts/sync-its-rest-specs.sh check

##@ Test

test: codegen-verify aqlgen-verify ## Run unit tests (includes codegen drift checks)
	@$(GO) test ./... -count=1

test-race: ## Run unit tests with -race (main-branch CI job)
	@$(GO) test -race -count=1 ./...

##@ Lint

lint-ci: ## Run golangci-lint (host binary or pinned Docker image)
	@$(GOLANGCI) run ./...

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

spec-context: ## Assemble the SDD context bundle for a REQ (usage: make spec-context REQ=094)
	@bash scripts/spec-context.sh $(REQ)

probe-status: ## Show each PROBE's status and whether its test file exists
	@bash scripts/probe-status.sh

##@ Build

build: ## Compile all packages (cmd/examples when present)
	@$(GO) build ./...

clean: ## Remove bin/, coverage artefacts, and *.out files
	@rm -rf bin/ coverage.* *.out

##@ CI

ci: fmt-check mod-tidy-check vet test lint spec-check build ## Full local PR gate (see docs/ci.md)
