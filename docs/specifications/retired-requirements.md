---
kind: reference
---

# Retired requirements

A requirement is retired when its rule is withdrawn, or moves into another requirement or into plain prose.
Its entry moves here and its row leaves [traceability.yaml](traceability.yaml) and the [registry](REQ.md#registry), so this page is the only place its number appears.
The number is never reused: a new requirement takes the next number above the highest in the registry or on this page ([REQ.md § Numbering policy](REQ.md#numbering-policy)).

Nothing on this page binds; the rules are in the topic specs listed in [README.md § Index](README.md#index).
An entry says where its rule lives now.

## REQ-002 — Go version

The rule for the `go.mod` `go` directive is now prose in [releases.md § Minimum Go version](../releases.md#minimum-go-version).

## REQ-003 — License

The licence rule is now prose in [licensing.md](../licensing.md).

## REQ-005 — Internal boundary

The `internal/` boundary is now prose in [module-layout.md § The `internal/` boundary](module-layout.md#the-internal-boundary).

## REQ-010 — `cadasto/` cut line

The `cadasto/` cut line is now prose in [module-layout.md § Boundary rules](module-layout.md#boundary-rules).

## REQ-011 — No sideways `cadasto/` imports

The rule against sideways imports inside `cadasto/` is now prose in [module-layout.md § Boundary rules](module-layout.md#boundary-rules).

## REQ-012 — Auth layering

The layering of `auth/` is now prose in [module-layout.md § Boundary rules](module-layout.md#boundary-rules).

## REQ-014 — Dependency direction

The downward dependency direction is now prose in [module-layout.md § Boundary rules](module-layout.md#boundary-rules).

## REQ-022 — Functional options

Its conventions are now prose in [idiom.md § API conventions](idiom.md#api-conventions).

## REQ-023 — Package-level functions

Its conventions are now prose in [idiom.md § API conventions](idiom.md#api-conventions).

## REQ-024 — Generics, no reflection

Its conventions, the no-reflection rule included, are now prose in [idiom.md § API conventions](idiom.md#api-conventions).

## REQ-030 — Concrete RM structs

Its rule is now [reference-model.md § The shape of the RM in Go](reference-model.md#the-shape-of-the-rm-in-go).

## REQ-031 — Embedded base structs

Its rule is now [reference-model.md § The shape of the RM in Go](reference-model.md#the-shape-of-the-rm-in-go).

## REQ-032 — Interfaces for abstract RM

Its rule is now [reference-model.md § The shape of the RM in Go](reference-model.md#the-shape-of-the-rm-in-go).

## REQ-033 — No inheritance emulation

Its rule is now [reference-model.md § The shape of the RM in Go](reference-model.md#the-shape-of-the-rm-in-go).

## REQ-042 — Generated code, drift-detected

Generation and its drift check are now prose under [REQ-041](reference-model.md#req-041--pinned-bmm-sources).

## REQ-043 — P_BMM → Go mapping rules

The mapping rules now bind through [REQ-041](reference-model.md#req-041--pinned-bmm-sources) and live in [reference-model.md § Mapping rules](reference-model.md#mapping-rules).

## REQ-044 — Hand-written extensions isolated

The separation of generated and hand-written files is now prose under [REQ-041](reference-model.md#req-041--pinned-bmm-sources).

## REQ-046 — Primitive type mapping

The fixed primitive-type mapping is now the *Primitive types* rule of [reference-model.md § REQ-041](reference-model.md#req-041--pinned-bmm-sources), with its table in [§ Primitive type mapping](reference-model.md#primitive-type-mapping).

## REQ-047 — BMM authoritative on divergence

The rule that the BMM wins over spec prose is now prose under [REQ-041](reference-model.md#req-041--pinned-bmm-sources).

## REQ-049 — RM class-universe absence reasons

Its absence reasons are now [REQ-048 § Absence reasons](reference-model.md#absence-reasons).

## REQ-051 — Cadasto spec-version header

The rule for the `Cadasto-OpenEhr-Spec-Version` header moved to [wire.md § REQ-050](wire.md#req-050) *The Cadasto spec-version header*.

## REQ-065 — Per-client tenant binding

Replaced by the one-Platform catalog of [auth.md § REQ-070](auth.md#req-070) and the audience default of [auth.md § REQ-061](auth.md#req-061--pkce-flow).

## REQ-067 — Platform principal claims

Its rules are now [auth.md § REQ-064](auth.md#req-064--launch-context) *[Platform principal claims](auth.md#platform-principal-claims)*.

## REQ-073 — Discovery trust posture

Its rules are now [auth.md § REQ-072](auth.md#req-072) *[Trust posture](auth.md#trust-posture)*.

## REQ-080 — openEHR wire conformance

Its test-writing rules moved to [probe-harness.md § Writing a probe](../probe-harness.md#writing-a-probe), and the round-trip comparison it restated is [wire.md § REQ-052](wire.md#req-052)'s.

## REQ-081 — Wire-level parity

Withdrawn; [scope.md § Out of scope](scope.md#out-of-scope-v1) says what the SDK checks instead.

## REQ-095 — OpenAPI authoritative source

Its rules are now [wire.md § REQ-050](wire.md#req-050) *[The OpenAPI files are authoritative](wire.md#the-openapi-files-are-authoritative)*, and path-parameter encoding is [transport.md § REQ-150](transport.md#encoding).

## REQ-096 — Unambiguous "disable retry"

The rule that tells "no retries" apart from "use the package default" moved to [transport.md § REQ-091](transport.md#req-091--retry-policy) *Disabling retries*.

## REQ-097 — First-class `Idempotency-Key`

Withdrawn: the SDK sends no `Idempotency-Key` header, which Cadasto openEHR services do not accept.

## REQ-100 — ADL 1.4 operational template (OPT) parse and paths

Its path grammar, resolution rules and path errors moved to [clinical-modeling.md § REQ-111](clinical-modeling.md#req-111--public-compiled-template-bridge) *Path lookup*; the operational template model of [REQ-176](clinical-modeling.md#req-176--adl-14-operational-template-model) replaced the parser.

## REQ-104 — Slot assertion grammar

Its rules moved to [clinical-modeling.md § REQ-103](clinical-modeling.md#req-103--primitive-constraint-introspection) *Slot assertions*.

## REQ-105 — Terminology bindings

Its rules moved to [clinical-modeling.md § REQ-111](clinical-modeling.md#req-111--public-compiled-template-bridge) *Term definitions and bindings*.

## REQ-110 — Template-driven validation beyond COMPOSITION

Its rules moved to [clinical-modeling.md § REQ-102](clinical-modeling.md#req-102--composition-validation) *Roots beyond COMPOSITION*.

## REQ-116 — Template-level node naming and name-predicated paths

Its rules are now [clinical-modeling.md § REQ-111](clinical-modeling.md#req-111--public-compiled-template-bridge) *[Node names and name predicates](clinical-modeling.md#node-names-and-name-predicates)*.

## REQ-117 — AQL expression-catalogue completion

Its rules moved to [aql.md § REQ-113](aql.md#req-113--execution-oriented-parsed-aql-ast) (the read-side catalogue and its residual), [aql.md § REQ-109](aql.md#req-109--aql-static-lint) (alias resolution in the lint) and [aql.md § REQ-163](aql.md#req-055--aql-builder) (the builder's containment algebra and in-text paging).

## REQ-118 — Deprecated `SELECT TOP` clause and literal source text

Its rules moved to [aql.md § REQ-113](aql.md#req-113--execution-oriented-parsed-aql-ast) (the `TOP` carrier and literal source text), [aql.md § REQ-163](aql.md#req-055--aql-builder) (the builder's `TOP` and the row-bound channels) and [aql.md § REQ-109](aql.md#req-109--aql-static-lint) (the `TOP` lint codes).

## REQ-122 — Version-control derived helpers

Its rule is now [reference-model.md § REQ-120](reference-model.md#req-120--rm-identifier-parsing-and-derivation) *[Version-control derived helpers](reference-model.md#version-control-derived-helpers)*.

## REQ-142 — Contribution read

The contribution read moved to [wire.md § REQ-174](wire.md#req-174--versioned-object-reads) *The contribution read*.

## REQ-143 — Template list filters

The list filters are a template operation: they moved to [wire.md § REQ-177](wire.md#req-177--template-representations) *[List filters](wire.md#list-filters)*.

## REQ-162 — Builder containment verification

Its rules moved to [aql.md § REQ-161 § Builder adapter](aql.md#builder-adapter).

## REQ-163 — AQL write-side expressivity parity

Its carriers and their canonical spellings are now part of [aql.md § REQ-055](aql.md#req-055--aql-builder), the AQL builder.

## REQ-164 — AQL path-shape and paging lint

Its checks are now [aql.md § REQ-161](aql.md#req-161--aql-semantic-and-portability-lint) *[Path-shape checks](aql.md#path-shape-checks)*.

## REQ-167 — Token revocation

Its rules are now [auth.md § REQ-063](auth.md#req-063--token-refresh-and-revocation) *[Revocation](auth.md#revocation)*.

## REQ-172 — Typed 2xx body-read failure

Its rules moved to [transport.md § REQ-151](transport.md#req-151--typed-2xx-decode-failure), and its retry rule to [transport.md § REQ-091](transport.md#req-091--retry-policy).

## REQ-178 — Generic templates shipped with the SDK

Withdrawn; [scope.md § Out of scope](scope.md#out-of-scope-v1) says where the generic templates live.

## Numbers never used

REQ-114, REQ-115, REQ-124, REQ-125 and REQ-141 were reserved and released without becoming a requirement: an OPT author validator and a FLAT author linter that were never started, the RM-function stubs [reference-model.md](reference-model.md) records as out of scope, and one number released unused.
