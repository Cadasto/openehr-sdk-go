---
kind: reference
---

# Retired requirements

A requirement is retired when its rule is withdrawn, or moves into another requirement or into plain prose. Its identifier is never reused: it keeps its `deprecated` / `retired` row in [traceability.yaml](traceability.yaml), and that row's canonical link opens its entry below, which says where the rule lives now.

Nothing on this page binds. The rules are in the topic specs listed in [README.md § Index](README.md#index). Numbers that were reserved and released without becoming a requirement are listed in [REQ.md § Numbering policy](REQ.md#numbering-policy).

## REQ-002 — Go version

The rule for the `go.mod` `go` directive is now prose in [releases.md § Minimum Go version](../releases.md#minimum-go-version).

## REQ-003 — License

The licence rule is now prose in [licensing.md](../licensing.md).

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

The rule is now [ADR 0002](../adr/0002-bmm-codegen-decisions.md) D4, summarised in [rm-modeling.md § The shape of the RM in Go](rm-modeling.md#the-shape-of-the-rm-in-go).

## REQ-031 — Embedded base structs

The rule is now [ADR 0002](../adr/0002-bmm-codegen-decisions.md) D4, summarised in [rm-modeling.md § The shape of the RM in Go](rm-modeling.md#the-shape-of-the-rm-in-go).

## REQ-032 — Interfaces for abstract RM

The rule is now [ADR 0002](../adr/0002-bmm-codegen-decisions.md) D4, summarised in [rm-modeling.md § The shape of the RM in Go](rm-modeling.md#the-shape-of-the-rm-in-go).

## REQ-033 — No inheritance emulation

The rule is now [ADR 0002](../adr/0002-bmm-codegen-decisions.md) D4, summarised in [rm-modeling.md § The shape of the RM in Go](rm-modeling.md#the-shape-of-the-rm-in-go).

## REQ-042 — Generated code, drift-detected

Generation and its drift check are now prose under [REQ-041](bmm-conformance.md#req-041--pinned-bmm-sources).

## REQ-043 — P_BMM → Go mapping rules

The mapping rules now bind through [REQ-041](bmm-conformance.md#req-041--pinned-bmm-sources) and live in [bmm-conformance.md § Mapping rules](bmm-conformance.md#mapping-rules).

## REQ-044 — Hand-written extensions isolated

The separation of generated and hand-written files is now prose under [REQ-041](bmm-conformance.md#req-041--pinned-bmm-sources).

## REQ-047 — BMM authoritative on divergence

The rule that the BMM wins over spec prose is now prose under [REQ-041](bmm-conformance.md#req-041--pinned-bmm-sources).

## REQ-049 — RM class-universe absence reasons

Its absence reasons are now [REQ-048 § Absence reasons](bmm-conformance.md#absence-reasons).

## REQ-051 — Cadasto spec-version header

The rule for the `Cadasto-OpenEhr-Spec-Version` header moved to [wire.md § REQ-050](wire.md#req-050) *The Cadasto spec-version header*.

## REQ-065 — Per-client tenant binding

A client takes the catalog of one Platform ([service-discovery.md § REQ-070](service-discovery.md#req-070)), and a SMART source built from a catalog defaults its audience to that Platform's base URL ([auth.md § REQ-061](auth.md#req-061--pkce-flow)).

## REQ-080 — openEHR wire conformance

Its test-writing rules moved to [development-process.md § Writing a probe](../development-process.md#writing-a-probe), and the round-trip comparison it restated is [wire.md § REQ-052](wire.md#req-052)'s.

## REQ-081 — Wire-level parity

The SDK checks no parity with another SDK: its wire output is checked against the openEHR specification by the [probe catalog](conformance.md#probe-catalog), and its Go surface follows [idiom.md § Surface shape](idiom.md#surface-shape).

## REQ-096 — Unambiguous "disable retry"

The rule that tells "no retries" apart from "use the package default" moved to [transport.md § REQ-091](transport.md#req-091--retry-policy) *Disabling retries*.

## REQ-097 — First-class `Idempotency-Key`

Cadasto openEHR services no longer accept the `Idempotency-Key` header, so the SDK does not send it.

## REQ-104 — Slot assertion grammar

Its rules moved to [clinical-modeling.md § REQ-103](clinical-modeling.md#req-103--primitive-constraint-introspection) *Slot assertions*.

## REQ-105 — Terminology bindings

Its rules moved to [clinical-modeling.md § REQ-111](clinical-modeling.md#req-111--public-compiled-template-bridge) *Term definitions and bindings*.

## REQ-110 — Template-driven validation beyond COMPOSITION

Its rules moved to [clinical-modeling.md § REQ-102](clinical-modeling.md#req-102--composition-validation) *Roots beyond COMPOSITION*.

## REQ-117 — AQL expression-catalogue completion

Its rules moved to [clinical-modeling.md § REQ-113](clinical-modeling.md#req-113--execution-oriented-parsed-aql-ast) (the read-side catalogue and its residual), [clinical-modeling.md § REQ-109](clinical-modeling.md#req-109--aql-static-lint) (alias resolution in the lint) and [clinical-modeling.md § REQ-163](clinical-modeling.md#req-163--aql-write-side-expressivity-parity) (the builder's containment algebra and in-text paging).

## REQ-118 — Deprecated `SELECT TOP` clause and literal source text

Its rules moved to [clinical-modeling.md § REQ-113](clinical-modeling.md#req-113--execution-oriented-parsed-aql-ast) (the `TOP` carrier and literal source text), [clinical-modeling.md § REQ-163](clinical-modeling.md#req-163--aql-write-side-expressivity-parity) (the builder's `TOP` and the row-bound channels) and [clinical-modeling.md § REQ-109](clinical-modeling.md#req-109--aql-static-lint) (the `TOP` lint codes).

## REQ-142 — Contribution read

The contribution read moved to [wire.md § REQ-174](wire.md#req-174--versioned-object-reads) *The contribution read*.

## REQ-162 — Builder containment verification

Its rules moved to [clinical-modeling.md § REQ-161 § Builder adapter](clinical-modeling.md#builder-adapter).

## REQ-172 — Typed 2xx body-read failure

Its rules moved to [transport.md § REQ-151](transport.md#req-151--typed-2xx-decode-failure), and its retry rule to [transport.md § REQ-091](transport.md#req-091--retry-policy).
