---
kind: specification
---

# Requirements registry

**Status:** Draft

Index of the normative `REQ-NNN` identifiers for `github.com/cadasto/openehr-sdk-go`.
Each requirement's normative text lives in the linked topic spec, never here.

The table is generated from [`traceability.yaml`](traceability.yaml) by `make spec-generate`, and `make spec-check` fails when it is stale.
To change a title, a canonical link, the spec stability or the implementation status, edit the map and regenerate; do not edit the table.

**Adding a requirement:** take the next free number (see [Numbering policy](#numbering-policy)), write the normative text in the canonical topic spec, add an entry to `traceability.yaml`, then run `make spec-generate`.

---

## Registry

<!-- BEGIN GENERATED: registry (make spec-generate — edit traceability.yaml, not this table) -->
| ID | Title | Canonical | Stability | Impl. |
|---|---|---|---|---|
| REQ-001 | Module path | [module-layout.md](module-layout.md#req-001--module-path) | draft | landed |
| REQ-004 | Semantic versioning | [module-layout.md](module-layout.md#req-004--semantic-versioning) | draft | landed |
| REQ-013 | Building-block independence | [module-layout.md](module-layout.md#req-013--building-block-independence) | draft | landed |
| REQ-020 | Context-first I/O | [idiom.md](idiom.md#context-propagation-req-020) | draft | landed |
| REQ-021 | Injected `*http.Client` | [idiom.md](idiom.md#http-client-injection-req-021) | draft | landed |
| REQ-025 | Error wrapping | [idiom.md](idiom.md#errors-req-025) | draft | landed |
| REQ-026 | Goroutine-safe clients | [idiom.md](idiom.md#concurrency-req-026) | draft | landed |
| REQ-034 | openEHR terminology vocabulary | [reference-model.md](reference-model.md#openehr-terminology-vocabulary-req-034) | draft | landed |
| REQ-040 | Type registry | [reference-model.md](reference-model.md#type-registry-req-040) | draft | landed |
| REQ-041 | Pinned BMM sources | [reference-model.md](reference-model.md#req-041--pinned-bmm-sources) | draft | landed |
| REQ-045 | BMM loader building block | [reference-model.md](reference-model.md#req-045--bmm-loader-is-a-building-block) | draft | landed |
| REQ-048 | RM meta-model introspection | [reference-model.md](reference-model.md#req-048--rm-meta-model-introspection-surface) | draft | landed |
| REQ-050 | REST 1.1.0 pin | [wire.md](wire.md#req-050) | draft | landed |
| REQ-052 | Canonical JSON | [wire.md](wire.md#req-052) | draft | landed |
| REQ-053 | FLAT and STRUCTURED | [wire.md](wire.md#req-053) | draft | landed |
| REQ-054 | Optimistic concurrency | [wire.md](wire.md#req-054) | draft | landed |
| REQ-055 | AQL wire boundary | [wire.md](wire.md#req-055--wire-boundary) | draft | landed |
| REQ-056 | Canonical XML | [wire.md](wire.md#req-056) | draft | landed |
| REQ-057 | Stored AQL queries | [wire.md](wire.md#req-057) | draft | landed |
| REQ-058 | Datamap V2 | [module-layout.md](module-layout.md#req-058--datamap-v2) | draft | planned |
| REQ-059 | openEHR custom headers | [wire.md](wire.md#req-059) | draft | landed |
| REQ-060 | TokenSource interface | [auth.md](auth.md#req-060) | draft | landed |
| REQ-061 | SMART-on-openEHR PKCE | [auth.md](auth.md#req-061--pkce-flow) | draft | landed |
| REQ-062 | JWKS rotation | [auth.md](auth.md#req-062--jwks-rotation) | draft | landed |
| REQ-063 | Token refresh | [auth.md](auth.md#req-063--token-refresh) | draft | landed |
| REQ-064 | Launch context | [auth.md](auth.md#req-064--launch-context) | draft | landed |
| REQ-066 | Caller attribution | [transport.md](transport.md#req-066--caller-attribution) | draft | landed |
| REQ-068 | SMART flows and launch modes | [auth.md](auth.md#req-068--flow-and-launch-mode-coverage) | draft | landed |
| REQ-069 | HTTP Basic on openEHR REST | [auth.md](auth.md#req-069) | draft | landed |
| REQ-070 | First-class discovery | [auth.md](auth.md#req-070) | draft | landed |
| REQ-071 | Discovery cache | [auth.md](auth.md#req-071) | draft | landed |
| REQ-072 | Discovery validation | [auth.md](auth.md#req-072) | draft | landed |
| REQ-073 | Discovery trust posture | [auth.md](auth.md#req-073--discovery-trust-posture) | draft | landed |
| REQ-082 | Probe runnability | [conformance.md](conformance.md#req-082--runnability) | draft | landed |
| REQ-083 | `cadasto/admin` health checks | [conformance.md](conformance.md#req-083--cadastoadmin-health-checks) | draft | landed |
| REQ-090 | OpenTelemetry hooks | [transport.md](transport.md#req-090--opentelemetry-hooks) | draft | landed |
| REQ-091 | Retry policy | [transport.md](transport.md#req-091--retry-policy) | draft | landed |
| REQ-092 | TLS posture | [transport.md](transport.md#req-092--tls-posture) | draft | landed |
| REQ-093 | Error envelope mapping | [transport.md](transport.md#req-093--openehr-error-envelope-mapping) | draft | landed |
| REQ-094 | `Prefer` negotiation | [transport.md](transport.md#req-094--prefer-response-shape-negotiation) | draft | landed |
| REQ-098 | Request-level observer hook | [transport.md](transport.md#req-098--request-level-observer-hook) | draft | landed |
| REQ-099 | ITS-REST Admin client surface | [wire.md](wire.md#req-099--its-rest-admin-client-surface) | draft | landed |
| REQ-101 | Generic OPT-driven composition builder | [clinical-modeling.md](clinical-modeling.md#req-101--generic-opt-driven-composition-builder) | draft | landed |
| REQ-102 | Composition validation | [clinical-modeling.md](clinical-modeling.md#req-102--composition-validation) | draft | landed |
| REQ-103 | Primitive constraint introspection | [clinical-modeling.md](clinical-modeling.md#req-103--primitive-constraint-introspection) | draft | landed |
| REQ-106 | WebTemplate JSON export | [clinical-modeling.md](clinical-modeling.md#req-106--webtemplate-json-export) | draft | landed |
| REQ-107 | Template-driven RM instance example generator | [clinical-modeling.md](clinical-modeling.md#req-107--template-driven-rm-instance-example-generator) | draft | landed |
| REQ-108 | Untrusted document bounds | [clinical-modeling.md](clinical-modeling.md#req-108--untrusted-document-bounds) | draft | landed |
| REQ-109 | AQL static lint | [aql.md](aql.md#req-109--aql-static-lint) | draft | landed |
| REQ-111 | Public compiled-template bridge | [clinical-modeling.md](clinical-modeling.md#req-111--public-compiled-template-bridge) | draft | landed |
| REQ-112 | Template-less Reference Model validation floor | [clinical-modeling.md](clinical-modeling.md#req-112--template-less-reference-model-validation-floor) | draft | landed |
| REQ-113 | Execution-oriented parsed AQL AST | [aql.md](aql.md#req-113--execution-oriented-parsed-aql-ast) | draft | landed |
| REQ-119 | Re-parseable canonical AQL emission | [aql.md](aql.md#req-119--re-parseable-canonical-aql-emission) | draft | landed |
| REQ-120 | RM identifier parsing and derivation | [reference-model.md](reference-model.md#req-120--rm-identifier-parsing-and-derivation) | draft | landed |
| REQ-121 | Locatable path read access | [reference-model.md](reference-model.md#req-121--locatable-path-read-access) | draft | landed |
| REQ-123 | Temporal data-value helpers | [reference-model.md](reference-model.md#req-123--temporal-data-value-helpers) | draft | landed |
| REQ-130 | Contribution builder | [wire.md](wire.md#req-130--contribution-builder) | draft | landed |
| REQ-140 | Underscore-prefixed RM attributes (simplified formats) | [wire.md](wire.md#req-140--underscore-prefixed-rm-attributes) | draft | landed |
| REQ-144 | Definition metadata decoding | [wire.md](wire.md#req-144--definition-metadata-decoding) | draft | landed |
| REQ-150 | Path-parameter segment validation | [transport.md](transport.md#req-150--path-parameter-segment-validation) | draft | landed |
| REQ-151 | Typed 2xx decode failure | [transport.md](transport.md#req-151--typed-2xx-decode-failure) | draft | landed |
| REQ-160 | AQL containment admissibility relation | [aql.md](aql.md#req-160--aql-containment-admissibility-relation) | draft | landed |
| REQ-161 | AQL semantic and portability lint | [aql.md](aql.md#req-161--aql-semantic-and-portability-lint) | draft | landed |
| REQ-163 | AQL write-side expressivity parity | [aql.md](aql.md#req-163--aql-write-side-expressivity-parity) | draft | landed |
| REQ-165 | openEHR scope syntax | [auth.md](auth.md#req-165--openehr-scope-syntax) | draft | landed |
| REQ-166 | Bearer challenge on 401 and 403 | [transport.md](transport.md#req-166--bearer-challenge-on-401-and-403) | draft | landed |
| REQ-167 | Token revocation | [auth.md](auth.md#req-167--token-revocation) | draft | landed |
| REQ-168 | Value-free validation diagnostics | [clinical-modeling.md](clinical-modeling.md#req-168--value-free-validation-diagnostics) | draft | landed |
| REQ-169 | Interval boundary flag defaults | [wire.md](wire.md#req-169--interval-boundary-flag-defaults) | draft | landed |
| REQ-170 | Unmatched content on decode | [wire.md](wire.md#req-170--unmatched-content-on-decode) | draft | landed |
| REQ-171 | Validation engine | [clinical-modeling.md](clinical-modeling.md#req-171--validation-engine) | draft | landed |
| REQ-173 | Composition representation formats | [wire.md](wire.md#req-173--composition-representation-formats) | draft | landed |
| REQ-174 | Versioned-object reads | [wire.md](wire.md#req-174--versioned-object-reads) | draft | landed |
| REQ-175 | Missing members on decode | [wire.md](wire.md#req-175--missing-members-on-decode) | draft | landed |
| REQ-176 | ADL 1.4 operational template model | [clinical-modeling.md](clinical-modeling.md#req-176--adl-14-operational-template-model) | draft | landed |
| REQ-177 | Template representations | [wire.md](wire.md#req-177--template-representations) | draft | landed |
<!-- END GENERATED: registry -->

**Impl.** column: `landed` (code + tests), `partial` (subset), `planned` (spec only). A retired requirement has no row; [retired-requirements.md](retired-requirements.md) lists it.

---

## Numbering policy

A new requirement takes the **next number above the highest in the registry or in [retired-requirements.md](retired-requirements.md)**, whatever its topic.
The number carries no topic meaning; the canonical link says where the requirement belongs.

Identifiers are stable once published: never renumbered and never reused.
A retired requirement, or a number released without becoming one, is listed in [retired-requirements.md](retired-requirements.md) and nowhere else.
