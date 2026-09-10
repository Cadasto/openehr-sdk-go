# openehr-sdk-go — host-first Go toolchain, Docker fallback for CI parity.
#
# Policy in AGENTS.md > Tooling policy. Single entry point — extend here,
# don't add ad-hoc scripts.
#
# Fast path  : host Go 1.27.x (recommended for daily development).
# Fallback   : `docker compose run --rm go …` using the `dev` stage in
#              Dockerfile (gated behind the `dev` compose profile).
.DEFAULT_GOAL := help

# ---- variables -----------------------------------------------------------

COMPOSE         ?= docker compose
COMPOSE_PROJECT ?= openehr-sdk-go
LINT_IMAGE      ?= golangci/golangci-lint:v2.13.2-alpine
DOCKER_MOUNT    = -v $(CURDIR):/app -w /app

# ANTLR codegen (maintainer-only). The generator is Java, confined to the
# Dockerfile `antlr` stage; the runtime is pure Go. Keep ANTLR_VERSION in
# lockstep with the antlr4-go/antlr runtime in go.mod (see grammar PIN).
ANTLR_VERSION   ?= 4.13.2
ANTLR_IMAGE     ?= openehr-sdk-go/antlr:$(ANTLR_VERSION)
AQL_GRAMMAR_DIR := resources/aql/grammar/active
AQL_GEN_DIR     := openehr/aql/parse/gen

HOST_GO_OK   := $(shell command -v go >/dev/null 2>&1 && go version 2>/dev/null | grep -qE 'go1\.27(\.|$$|[[:space:]])' && echo yes)
# Official golangci-lint binaries for this pin are built with Go 1.27; a
# host binary compiled with an older toolchain cannot load a go 1.27.0 module.
HOST_GLCI_OK := $(shell command -v golangci-lint >/dev/null 2>&1 && golangci-lint version 2>/dev/null | grep -qE 'built with go1\.27' && echo yes)

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
        termgen termgen-verify \
        its-rest-sync its-rest-check \
        terminology-sync terminology-check terminology-verify \
        test test-race \
        lint lint-ci \
        mod-tidy mod-tidy-check \
        spec-check spec-context probe-status probe-record \
        build clean \
        docs-sync docs-sync-offline docs-build docs-check docs-serve docs-clean \
        ci

# ---- help & toolchain ----------------------------------------------------

help: ## Show grouped targets and tooling policy
	@echo "openehr-sdk-go"
	@echo ""
	@if [ "$(HOST_GO_OK)" = "yes" ]; then \
		echo "Toolchain : host Go 1.27.x (fast path)"; \
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
	@echo "Docs site : make docs-serve   (http://127.0.0.1:8000)"
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

vet: ## Run go vet (generated ANTLR parser's unreachable code suppressed)
	@# The generated parser (openehr/aql/parse/gen) emits unreachable code after
	@# panics — inherent to ANTLR's Go target, not a defect. Go 1.27's `go vet`
	@# surfaces those diagnostics through the importing openehr/aql/parse package,
	@# so a package-list exclusion can't suppress them. Disable only the
	@# `unreachable` analyzer instead; `make lint` (golangci-lint, generated: lax)
	@# keeps `unreachable` on hand-written code.
	@$(GO) vet -unreachable=false ./...

##@ Codegen

codegen: ## Regenerate RM and AOM 1.4 from pinned BMM sources
	@$(GO) run ./cmd/bmmgen -resources ./resources/bmm -out .

codegen-verify: ## Fail if generated code drifts from resources/bmm
	@$(GO) run ./cmd/bmmgen -resources ./resources/bmm -out . -verify

termgen: ## Regenerate openehr/terminology from the pinned resources/terminology/openehr_terminology.xml
	@$(GO) run ./cmd/termgen -resources ./resources/terminology -out .

termgen-verify: ## Fail if openehr/terminology drifts from resources/terminology
	@$(GO) run ./cmd/termgen -resources ./resources/terminology -out . -verify

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

flat-conformance-sync: ## Vendor the upstream EHRbase FLAT conformance corpus into testkit/cassettes/flat-conformance/ (needs network; FLAT_CONFORMANCE_REF to pin)
	@./scripts/sync-flat-conformance.sh sync

flat-conformance-check: ## Verify the vendored FLAT conformance corpus matches MANIFEST + report upstream drift (offline integrity; network for drift)
	@./scripts/sync-flat-conformance.sh check

flat-conformance-verify: ## Offline sha256 integrity of the vendored FLAT corpus (no network, no curl/jq) — run by `make ci`
	@./scripts/sync-flat-conformance.sh verify

terminology-sync: ## Vendor the openEHR Terminology (openehr_terminology.xml) into resources/terminology/ and regenerate the accessor (needs network; TERMINOLOGY_REF to pin)
	@./scripts/sync-terminology.sh sync

terminology-check: ## Verify the vendored terminology matches MANIFEST + report a newer upstream release (offline integrity; network for drift)
	@./scripts/sync-terminology.sh check

terminology-verify: ## Offline sha256 integrity of the vendored openEHR Terminology (no network, no curl/jq) — run by `make ci`
	@./scripts/sync-terminology.sh verify

##@ Test

test: codegen-verify aqlgen-verify termgen-verify ## Run unit tests (includes codegen drift checks)
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
	@bash scripts/spec-check-selftest.sh
	@bash scripts/spec-check.sh

spec-context: ## Assemble the SDD context bundle for a REQ (usage: make spec-context REQ=094)
	@bash scripts/spec-context.sh $(REQ)

probe-status: ## Show each PROBE's status and whether its test file exists
	@bash scripts/probe-status.sh

probe-record: ## Capture a REQ-082 Cassette recording from a live CDR (usage: make probe-record ARGS="-base URL -deployment NAME -scenario ehr-lifecycle"; see testkit/recordings/README.md)
	@rev="$$(git rev-parse HEAD 2>/dev/null)"; \
	 if [ -n "$$(git status --porcelain 2>/dev/null)" ]; then rev="$$rev-dirty"; fi; \
	 $(GO) run ./cmd/probe-record -sdk-commit "$$rev" $(ARGS)

##@ Build

build: ## Compile all packages (cmd/examples when present)
	@$(GO) build ./...

clean: ## Remove bin/, coverage artefacts, and *.out files
	@rm -rf bin/ coverage.* *.out

##@ Documentation site

# Pinned: the published site is built from this image without human review.
MKDOCS_IMAGE ?= squidfunk/mkdocs-material:9.7.6
DOCS_BUILD   := site
FETCHED      := .fetched
DOCKER_USER  := $(shell id -u):$(shell id -g)
DOCKER_DOCS  := docker run --rm -u $(DOCKER_USER) -e PYTHONDONTWRITEBYTECODE=1 \
                  -v "$(CURDIR):/docs" -w /docs
MKDOCS_RUN   := $(DOCKER_DOCS) $(MKDOCS_IMAGE)
PYTHON_DOCS  := $(DOCKER_DOCS) --entrypoint python3 $(MKDOCS_IMAGE)
# Destinations of sources.json `theme.files`. Regenerated by `make docs-sync`.
THEME_FETCHED := \
	pages/stylesheets/tokens.css \
	pages/stylesheets/material.css \
	pages/stylesheets/landing.css \
	overrides/home.html \
	overrides/partials/copyright.html \
	pages/assets/cadasto-mark.png

docs-sync: ## Fetch the pinned docs-theme brand layer
	$(PYTHON_DOCS) scripts/sync_sources.py

docs-sync-offline: ## Reuse the cached brand files instead of fetching
	$(PYTHON_DOCS) scripts/sync_sources.py --offline

docs-build: docs-sync ## Build the documentation site to site/
	$(MKDOCS_RUN) build -d /docs/$(DOCS_BUILD)

docs-check: docs-build ## Build the site and assert the published output is complete
	@set -e; \
	test -s "$(DOCS_BUILD)/index.html" \
	  || { echo "docs-check: no index.html in $(DOCS_BUILD)/"; exit 1; }; \
	test -s "$(DOCS_BUILD)/stylesheets/tokens.css" \
	  || { echo "docs-check: tokens.css not emitted — is it inside pages/?"; exit 1; }; \
	test -s "$(DOCS_BUILD)/stylesheets/material.css" \
	  || { echo "docs-check: material.css not emitted — is it inside pages/?"; exit 1; }; \
	test -s "$(DOCS_BUILD)/stylesheets/landing.css" \
	  || { echo "docs-check: landing.css not emitted — is it inside pages/?"; exit 1; }; \
	test -s "$(DOCS_BUILD)/assets/cadasto-mark.png" \
	  || { echo "docs-check: company mark not emitted — is it inside pages/?"; exit 1; }; \
	grep -q 'home-nav' "$(DOCS_BUILD)/index.html" \
	  || { echo "docs-check: landing template not applied — is home.html fetched?"; exit 1; }; \
	for css in tokens material landing; do \
	  grep -q "stylesheets/$$css.css" "$(DOCS_BUILD)/install/index.html" \
	    || { echo "docs-check: $$css.css is present but not linked — check extra_css in mkdocs.yml"; exit 1; }; \
	done; \
	grep -q '\[data-md-color-scheme="slate"\]' "$(DOCS_BUILD)/stylesheets/tokens.css" \
	  || { echo "docs-check: tokens.css defines no dark scheme block"; exit 1; }; \
	grep -q '\[data-md-color-scheme="default"\]' "$(DOCS_BUILD)/stylesheets/tokens.css" \
	  || { echo "docs-check: tokens.css defines no light scheme block"; exit 1; }; \
	grep -q 'data-md-component="palette"' "$(DOCS_BUILD)/install/index.html" \
	  || { echo "docs-check: the palette toggle is missing from docs pages"; exit 1; }; \
	for scheme in slate default; do \
	  grep -q "data-md-color-scheme=\"$$scheme\"" "$(DOCS_BUILD)/install/index.html" \
	    || { echo "docs-check: docs pages offer no $$scheme palette option"; exit 1; }; \
	done; \
	grep -q 'cadasto-by' "$(DOCS_BUILD)/install/index.html" \
	  || { echo "docs-check: the fetched copyright partial did not render"; exit 1; }; \
	grep -q 'cadasto-mark.png' "$(DOCS_BUILD)/install/index.html" \
	  || { echo "docs-check: the company mark is not referenced from docs-page footers"; exit 1; }; \
	test -s "$(DOCS_BUILD)/assets/logo.svg" \
	  || { echo "docs-check: logo/favicon not emitted — is it inside pages/?"; exit 1; }; \
	grep -q 'github.com/cadasto/openehr-sdk-go' "$(DOCS_BUILD)/install/index.html" \
	  || { echo "docs-check: install page is missing the module path"; exit 1; }; \
	grep -q 'openehr-sdk-go@v0.27.0' "$(DOCS_BUILD)/install/index.html" \
	  || { echo "docs-check: install page is missing the pinned go get tag"; exit 1; }; \
	test -s "$(DOCS_BUILD)/examples/index.html" \
	  || { echo "docs-check: examples page not emitted"; exit 1; }; \
	grep -q 'cmd/examples' "$(DOCS_BUILD)/examples/index.html" \
	  || { echo "docs-check: examples page is missing the cmd/examples catalog"; exit 1; }; \
	test -s "$(DOCS_BUILD)/reference/index.html" \
	  || { echo "docs-check: reference page not emitted"; exit 1; }; \
	grep -q 'http-equiv="refresh"' "$(DOCS_BUILD)/reference/index.html" \
	  || { echo "docs-check: reference page is missing the pkg.go.dev redirect"; exit 1; }; \
	grep -q 'pkg.go.dev/github.com/cadasto/openehr-sdk-go' "$(DOCS_BUILD)/reference/index.html" \
	  || { echo "docs-check: reference page does not target the module on pkg.go.dev"; exit 1; }; \
	grep -q 'href="reference/"' "$(DOCS_BUILD)/index.html" \
	  || { echo "docs-check: landing nav is missing the Go reference item"; exit 1; }; \
	test -s "$(DOCS_BUILD)/contact/index.html" \
	  || { echo "docs-check: contact page not emitted"; exit 1; }; \
	grep -q 'info@cadasto.com' "$(DOCS_BUILD)/contact/index.html" \
	  || { echo "docs-check: contact page is missing the company email"; exit 1; }; \
	for fact in Alkmaar 98762893 NL868632867B01; do \
	  grep -q "$$fact" "$(DOCS_BUILD)/contact/index.html" \
	    || { echo "docs-check: contact page is missing $$fact"; exit 1; }; \
	done; \
	! grep -qE '<textarea|type="email"' "$(DOCS_BUILD)/contact/index.html" \
	  || { echo "docs-check: contact page has a form — enquiries go through cadasto.com"; exit 1; }; \
	! grep -rqE 'https?://fonts\.(googleapis|gstatic)\.com' "$(DOCS_BUILD)" \
	  || { echo "docs-check: remote font URLs in output — the privacy plugin did not localise them"; exit 1; }; \
	grep -rq 'font-display: *swap' "$(DOCS_BUILD)/assets/external/fonts.googleapis.com/" \
	  || { echo "docs-check: no localised font sheet"; exit 1; }; \
	for w in 500 700; do \
	  grep -rq "font-weight: *$$w" "$(DOCS_BUILD)/assets/external/fonts.googleapis.com/" \
	    || { echo "docs-check: no Fira Sans $$w face was localised"; exit 1; }; \
	done; \
	! grep -q 'markdown="1"' "$(DOCS_BUILD)/index.html" \
	  || { echo "docs-check: literal markdown=\"1\" reached the landing page"; exit 1; }; \
	! grep -rqE '(href|src)="[^":]*\.md"' "$(DOCS_BUILD)" \
	  || { echo "docs-check: an unresolved relative .md path reached the output"; exit 1; }
	@$(PYTHON_DOCS) scripts/check_brand_paths.py
	@echo "docs-check: OK"

docs-serve: docs-sync ## Preview the documentation site on http://127.0.0.1:8000
	docker run --rm -it -u $(DOCKER_USER) -e PYTHONDONTWRITEBYTECODE=1 \
	  -p 127.0.0.1:8000:8000 -v "$(CURDIR):/docs" -w /docs \
	  $(MKDOCS_IMAGE) serve -a 0.0.0.0:8000

docs-clean: ## Remove the site build, fetched brand layer, and MkDocs plugin cache
	@test -n "$(DOCS_BUILD)" || { echo "docs-clean: DOCS_BUILD must not be empty"; exit 1; }
	rm -rf "$(CURDIR)/$(DOCS_BUILD)" "$(CURDIR)/$(FETCHED)" "$(CURDIR)/.cache"
	rm -f $(THEME_FETCHED)

##@ CI

ci: fmt-check mod-tidy-check vet test lint spec-check flat-conformance-verify terminology-verify build ## Full local PR gate (see docs/ci.md)
