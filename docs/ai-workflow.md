# AI workflow

How AI assistant agents (Claude Code, Cursor, Copilot, Codex, …) should work in this repo. Read [AGENTS.md](../AGENTS.md) and [architecture.md](architecture.md) first — they're the ground-truth references.

## Order of operations

1. **Read** [AGENTS.md](../AGENTS.md) (1-page) and this file.
2. **Locate** the spec that covers your task — `specs/` is the normative tree:
   - requirement index → [`../specs/REQ.md`](../specs/REQ.md) (registry row → **canonical** topic spec)
   - traceability (packages, probes, tests) → [`../specs/traceability.yaml`](../specs/traceability.yaml)
   - packaging REQ-001–005 → [`../specs/packaging.md`](../specs/packaging.md)
   - module layout / dependency / boundary → [`../specs/module-layout.md`](../specs/module-layout.md)
   - idiomatic surface (ctx, http.Client, options, errors, generics) → [`../specs/idiom.md`](../specs/idiom.md)
   - RM modeling rules → [`../specs/rm-modeling.md`](../specs/rm-modeling.md)
   - wire format (REST, AQL, canonical JSON, FLAT, STRUCTURED) → [`../specs/wire.md`](../specs/wire.md)
   - transport (OTel, retry, TLS, errors, Prefer) → [`../specs/transport.md`](../specs/transport.md)
   - auth contracts → [`../specs/auth.md`](../specs/auth.md)
   - discovery flow → [`../specs/service-discovery.md`](../specs/service-discovery.md)
   - conformance probes (PROBE-NNN) → [`../specs/conformance.md`](../specs/conformance.md)
   - use cases, POC milestones → [`../specs/use-cases.md`](../specs/use-cases.md)
   - open research strand → [`../specs/research-strands.md`](../specs/research-strands.md)
   - design narrative / mermaid → [architecture.md](architecture.md)
   - closed architectural decision → [adr/](adr/)
3. **Cite identifiers** when working: read the **canonical** spec section from the REQ.md registry row (not duplicate prose in REQ.md); every plan in [plans/](plans/) MUST list REQ-IDs; update [`../specs/traceability.yaml`](../specs/traceability.yaml) when landing packages or probes; tests SHOULD cite REQ-IDs and PROBE-IDs; ADRs MUST cite STRAND-IDs they resolve.
4. **Inspect ground truth** before editing:
   - For openEHR RM type shapes, prefer **MCP `type_specification_get`** over inferring.
   - For terminology codes, prefer **MCP `terminology_resolve`** — never hardcode a numeric literal without verifying.
   - For ITS-REST envelope semantics, use the **openehr-assistant** skills (see table below) and the spec MD twin (see `guide_get(category="howto", name="spec-lookup")`).
   - For RM mapping during CDR extraction, cross-check against the sibling `openehr-cdr` repo's structures — but do not blindly port; the SDK has stricter boundary rules.
5. **Build** before claiming done: `make ci` (includes `make spec-check`). See [ci.md](ci.md) for what GitHub runs on every PR.

## Editing rules

### Always

- **Use the Makefile** for any Go toolchain invocation. Host `go` is for `gopls` / direct development; the Makefile gives you the same toolchain CI uses.
- **Respect the boundary contract** in [architecture.md § Boundary rules](architecture.md#boundary-rules). The `cadasto/` cut line is load-bearing.
- **Use `context.Context` as the first parameter** on every method that does I/O.
- **Inject `*http.Client`** — the SDK never allocates a transport.
- **Functional options** for configuration. Constructors take options, not config structs.
- **`internal/` for implementation helpers** with no public-surface rationale — document the rationale in [architecture.md](architecture.md) when adding to `internal/`.
- **Conventional Commits** for commit messages (see [AGENTS.md § Code style and conventions](../AGENTS.md#code-style-and-conventions)).
- **CHANGELOG.md** — short, high-level `## [Unreleased]` bullets only. File-level detail belongs in commit messages and PR bodies. **Pre-1.0:** only `### Added` is used; `### Changed` / `### Fixed` / `### Removed` are reserved for post-v1.0 entries. See [AGENTS.md § Code style and conventions](../AGENTS.md#code-style-and-conventions).

### Never

- **Don't import `cadasto/…` from `openehr/…`, `auth/…`, `smart/…`, `transport/…`, `sandbox/…`, or `testkit/…`.** This is the v1 cut line; the future-extraction option must survive.
- **Don't import one `cadasto/<X>` from another `cadasto/<Y>` directly.** Share through openEHR-core types or interface contracts.
- **Don't add inheritance-emulation patterns** for the RM. Use concrete structs + embedded base structs + interfaces for abstract categories + the `typereg` central registry for `_type` decoding.
- **Don't silently resolve open research strands** (STRAND-NN in [`../specs/research-strands.md`](../specs/research-strands.md)) in code. Open a discussion or draft an ADR — code is the *output* of the decision, not the decision.
- **Don't introduce a new normative statement** (a MUST / SHOULD / MAY) without a REQ-NNN, PROBE-NNN, or STRAND-NN to anchor it. New requirements go in the canonical topic spec and [`../specs/REQ.md`](../specs/REQ.md) registry + [`traceability.yaml`](../specs/traceability.yaml) before code.
- **Don't renumber** REQ / PROBE / STRAND identifiers. They are stable once published.
- **Don't introduce a reflection-based decoder for RM types** without benchmarking against the typed-generic path.
- **Don't allocate `*http.Client` inside the SDK.** Inject it.
- **Don't add a single hard-coded base URL.** Use a `smart/discovery.ServiceCatalog` (or a hand-built equivalent for non-discovering backends).

## MCP & openEHR skills

This repo is registered as an openEHR workspace. Available skills (invoke via the `Skill` tool with `openehr-assistant:<name>`):

| Skill | Use when |
|---|---|
| `openehr-assistant:type-spec` | Exact attribute list / invariant / function signature for an RM class — **before locking goldens or types** |
| `openehr-assistant:terminology` | Resolve a numeric terminology code to a term, or vice versa |
| `openehr-assistant:format-data` | Validate the shape of a sample Composition / FLAT / STRUCTURED instance |
| `openehr-assistant:guide` | How-to (spec-lookup methodology, ITS-REST envelope cookbook, simplified-formats design) |
| `openehr-assistant:rm-structure` | Domain overview (composition categories, ISM states, versioning, demographic PARTY hierarchy) |
| `openehr-assistant:archetype-explain` | Semantics of an archetype (when needed for OPT-driven builders or validation tests) |
| `openehr-assistant:template-explain` | Semantics of a template (OPT/OET) — input to the validation and composition packages |
| `openehr-assistant:aql-designer` | Design / explain / review AQL (for the `openehr/aql/` builder semantics) |

When MCP is unavailable, fall back to the BMM in [`../openehr-bmm/`](../openehr-bmm/) (sibling repo under `/src/cadasto/`).

## Hooks (Claude Code)

PostToolUse hook in [`.claude/settings.json`](../.claude/settings.json):

- After `Write` / `Edit` on a `*.go` file, [`.claude/hooks/gofmt-on-save.sh`](../.claude/hooks/gofmt-on-save.sh) runs `gofmt -w -s` on that file.
- **Host-only by design** — gofmt is cheap; a Docker round-trip per save would dominate latency. Contributors without host Go still get formatting on next `make fmt` (which routes through the Dockerfile `dev` stage).

## Sibling-repo references

`/src/cadasto/` houses the related local clones. From this SDK's perspective:

| Repo | Why it matters |
|---|---|
| `architecture/` (private) | Source of truth for the SDK Specification proposal and related decisions |
| `openehr-cdr/` | First SDK consumer — CDR extraction target |
| `openehr-bmm/` | BMM dictionaries (fallback when MCP is unavailable) |
| `openehr-assistant-mcp/`, `openehr-assistant-plugin/` | MCP server / Claude Code skill plugin powering the `openehr-assistant:*` skills |

## When you're stuck

1. The decision is open (STRAND-NN in [`../specs/research-strands.md`](../specs/research-strands.md)) → don't decide it in a PR. Draft an ADR under [`adr/`](adr/) or surface the question to the user.
2. The spec is ambiguous (in `specs/` or the openEHR spec) → use `openehr-assistant:guide` (`howto/spec-lookup`) to find the canonical wording.
3. The PHP SDK does it differently → cross-language parity is **wire-level** (REQ-081), not source-level. The PHP SDK uses Eloquent-flavored fluent APIs; the Go SDK uses struct-builders and verb-functions. Both produce identical AQL on the wire. Same for repositories vs package-level methods, exceptions vs typed errors, etc.
4. The normative spec is missing something → add a `REQ-NNN` in [`../specs/REQ.md`](../specs/REQ.md) (Status: Draft) and elaborate in the relevant spec file, then implement. Never add a normative rule that exists only in code or in a comment.
