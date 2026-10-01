# Requirements registry

**Status:** Draft

Index of the normative `REQ-NNN` identifiers for `github.com/cadasto/openehr-sdk-go`. Each requirement's normative text lives in the linked topic spec, never here.

The table is generated from [`traceability.yaml`](traceability.yaml) by `make spec-gen`, and `make spec-check` fails when it is stale. To change a title, a canonical link, the spec stability or the implementation status, edit the map and regenerate; do not edit the table.

**Adding a requirement:** take the next free number (see [Numbering policy](#numbering-policy)), write the normative text in the canonical topic spec, add an entry to `traceability.yaml`, then run `make spec-gen`.

---

## Registry

<!-- BEGIN GENERATED: registry (make spec-gen — edit traceability.yaml, not this table) -->
| ID | Title | Canonical | Stability | Impl. |
|---|---|---|---|---|
| REQ-001 | Module path | [packaging.md](packaging.md#req-001--module-path) | draft | landed |
| REQ-002 | Go version | [packaging.md](packaging.md#req-002--go-version) | draft | landed |
| REQ-003 | License | [packaging.md](packaging.md#req-003--license) | draft | landed |
| REQ-004 | Semantic versioning | [packaging.md](packaging.md#req-004--semantic-versioning) | draft | landed |
| REQ-005 | Internal boundary | [packaging.md](packaging.md#req-005--internal-boundary) | draft | landed |
| REQ-010 | `cadasto/` cut line | [module-layout.md](module-layout.md#req-010--cadasto-cut-line) | draft | landed |
| REQ-011 | No sideways `cadasto/` imports | [module-layout.md](module-layout.md#req-011--no-sideways-imports-inside-cadasto) | draft | landed |
| REQ-012 | Auth layering | [module-layout.md](module-layout.md#req-012--auth-layering) | draft | landed |
| REQ-013 | Building-block independence | [module-layout.md](module-layout.md#req-013--building-block-independence) | draft | landed |
| REQ-014 | Dependency direction | [module-layout.md](module-layout.md#req-014--dependency-direction) | draft | landed |
| REQ-020 | Context-first I/O | [idiom.md](idiom.md#context-propagation-req-020) | draft | landed |
| REQ-021 | Injected `*http.Client` | [idiom.md](idiom.md#http-client-injection-req-021) | draft | landed |
| REQ-022 | Functional options | [idiom.md](idiom.md#functional-options-req-022) | draft | landed |
| REQ-023 | Package-level functions | [idiom.md](idiom.md#surface-shape-req-023) | draft | landed |
| REQ-024 | Generics, no reflection | [idiom.md](idiom.md#generics-policy-req-024) | draft | landed |
| REQ-025 | Error wrapping | [idiom.md](idiom.md#errors-req-025) | draft | landed |
| REQ-026 | Goroutine-safe clients | [idiom.md](idiom.md#concurrency-req-026) | draft | landed |
| REQ-030 | Concrete RM structs | [rm-modeling.md](rm-modeling.md#concrete-types-req-030) | draft | landed |
| REQ-031 | Embedded base structs | [rm-modeling.md](rm-modeling.md#embedded-base-structs-req-031) | draft | landed |
| REQ-032 | Interfaces for abstract RM | [rm-modeling.md](rm-modeling.md#abstract-categories-req-032) | draft | landed |
| REQ-033 | No inheritance emulation | [rm-modeling.md](rm-modeling.md#no-inheritance-emulation-req-033) | draft | landed |
| REQ-034 | openEHR terminology vocabulary | [rm-modeling.md](rm-modeling.md#openehr-terminology-vocabulary-req-034) | draft | landed |
| REQ-040 | Type registry | [rm-modeling.md](rm-modeling.md#type-registry-req-040) | draft | landed |
| REQ-041 | Pinned BMM sources | [bmm-conformance.md](bmm-conformance.md#req-041--pinned-bmm-sources) | draft | landed |
| REQ-042 | Generated code, drift-detected | [bmm-conformance.md](bmm-conformance.md#req-042--generated-code-drift-detected) | draft | landed |
| REQ-043 | P_BMM → Go mapping rules | [bmm-conformance.md](bmm-conformance.md#mapping-rules) | draft | landed |
| REQ-044 | Hand-written extensions isolated | [bmm-conformance.md](bmm-conformance.md#req-044--hand-written-extensions-are-isolated) | draft | landed |
| REQ-045 | BMM loader building block | [bmm-conformance.md](bmm-conformance.md#req-045--bmm-loader-is-a-building-block) | draft | landed |
| REQ-046 | Primitive type mapping | [bmm-conformance.md](bmm-conformance.md#primitive-type-mapping) | draft | landed |
| REQ-047 | BMM authoritative on divergence | [bmm-conformance.md](bmm-conformance.md#req-047--bmm-spec-divergence-resolution) | draft | landed |
| REQ-048 | RM meta-model introspection | [bmm-conformance.md](bmm-conformance.md#req-048--rm-meta-model-introspection-surface) | draft | landed |
| REQ-049 | RM class-universe absence reasons | [bmm-conformance.md](bmm-conformance.md#req-049--rm-class-universe-absence-reasons) | draft | landed |
| REQ-050 | REST 1.1.0-development pin | [wire.md](wire.md#req-050) | draft | landed |
| REQ-051 | Cadasto spec-version header | [wire.md](wire.md#req-051) | draft | landed |
| REQ-052 | Canonical JSON | [wire.md](wire.md#req-052) | draft | landed |
| REQ-053 | FLAT and STRUCTURED | [wire.md](wire.md#req-053) | draft | landed |
| REQ-054 | Optimistic concurrency | [wire.md](wire.md#req-054) | draft | landed |
| REQ-055 | AQL wire boundary | [wire.md](wire.md#req-055--wire-boundary) | draft | landed |
| REQ-056 | Canonical XML | [wire.md](wire.md#req-056) | draft | landed |
| REQ-057 | Stored AQL queries | [wire.md](wire.md#req-057) | draft | landed |
| REQ-058 | Datamap V2 | [module-layout.md](module-layout.md#req-058--datamap-v2) | draft | planned |
| REQ-059 | openEHR custom headers | [wire.md](wire.md#req-059) | draft | partial |
| REQ-060 | TokenSource interface | [auth.md](auth.md#req-060) | draft | landed |
| REQ-061 | SMART-on-openEHR PKCE | [auth.md](auth.md#req-061--pkce-flow) | draft | landed |
| REQ-062 | JWKS rotation | [auth.md](auth.md#req-062--jwks-rotation) | draft | landed |
| REQ-063 | Token refresh | [auth.md](auth.md#req-063--token-refresh) | draft | landed |
| REQ-064 | Launch context | [auth.md](auth.md#req-064--launch-context) | draft | landed |
| REQ-065 | Per-client tenant binding | [auth.md](auth.md#req-065) | draft | landed |
| REQ-066 | Caller attribution | [auth.md](auth.md#req-066) | draft | landed |
| REQ-067 | Platform principal claims | [auth.md](auth.md#req-067) | draft | landed |
| REQ-068 | SMART flows and launch modes | [auth.md](auth.md#req-068--flow-and-launch-mode-coverage) | draft | landed |
| REQ-069 | HTTP Basic on openEHR REST | [auth.md](auth.md#req-069) | draft | landed |
| REQ-070 | First-class discovery | [service-discovery.md](service-discovery.md#req-070) | draft | landed |
| REQ-071 | Discovery cache | [service-discovery.md](service-discovery.md#req-071) | draft | landed |
| REQ-072 | Discovery validation | [service-discovery.md](service-discovery.md#req-072) | draft | landed |
| REQ-073 | Discovery trust posture | [service-discovery.md](service-discovery.md#req-073--discovery-trust-posture) | draft | landed |
| REQ-080 | openEHR wire conformance | [conformance.md](conformance.md#conformance-scope) | draft | partial |
| REQ-081 | Wire-level parity (retired) | [conformance.md](conformance.md#req-081--wire-level-parity-retired) | deprecated | retired |
| REQ-082 | Probe runnability | [conformance.md](conformance.md#req-082--runnability) | draft | partial |
| REQ-083 | Cadasto platform API conformance | [conformance.md](conformance.md#req-083--cadasto-platform-api-conformance) | draft | partial |
| REQ-090 | OpenTelemetry hooks | [transport.md](transport.md#req-090--opentelemetry-hooks) | draft | landed |
| REQ-091 | Retry policy | [transport.md](transport.md#req-091--retry-policy) | draft | landed |
| REQ-092 | TLS posture | [transport.md](transport.md#req-092--tls-posture) | draft | landed |
| REQ-093 | Error envelope mapping | [transport.md](transport.md#req-093--openehr-error-envelope-mapping) | draft | landed |
| REQ-094 | `Prefer` negotiation | [transport.md](transport.md#req-094--prefer-response-shape-negotiation) | draft | landed |
| REQ-095 | OpenAPI authoritative source | [wire.md](wire.md#req-095) | draft | partial |
| REQ-096 | Unambiguous "disable retry" | [transport.md](transport.md#req-096--unambiguous-disable-retry) | draft | landed |
| REQ-097 | First-class `Idempotency-Key` (deprecated) | [transport.md](transport.md#req-097--first-class-idempotency-key-deprecated) | deprecated | retired |
| REQ-098 | Request-level observer hook | [transport.md](transport.md#req-098--request-level-observer-hook) | draft | landed |
| REQ-099 | ITS-REST Admin client surface | [module-layout.md](module-layout.md#req-099--its-rest-admin-client-surface) | draft | landed |
| REQ-100 | ADL 1.4 operational template (OPT) parse and paths | [clinical-modeling.md](clinical-modeling.md#req-100--adl-14-operational-template-opt-parse-and-paths) | draft | landed |
| REQ-101 | Generic OPT-driven composition builder | [clinical-modeling.md](clinical-modeling.md#req-101--generic-opt-driven-composition-builder) | draft | landed |
| REQ-102 | Composition validation | [clinical-modeling.md](clinical-modeling.md#req-102--composition-validation) | draft | landed |
| REQ-103 | Primitive constraint introspection | [clinical-modeling.md](clinical-modeling.md#req-103--primitive-constraint-introspection) | draft | landed |
| REQ-104 | Slot assertion grammar | [clinical-modeling.md](clinical-modeling.md#req-104--slot-assertion-grammar) | draft | landed |
| REQ-105 | Terminology bindings | [clinical-modeling.md](clinical-modeling.md#req-105--terminology-bindings) | draft | landed |
| REQ-106 | WebTemplate JSON export | [clinical-modeling.md](clinical-modeling.md#req-106--webtemplate-json-export) | draft | landed |
| REQ-107 | Template-driven RM instance example generator | [clinical-modeling.md](clinical-modeling.md#req-107--template-driven-rm-instance-example-generator) | draft | landed |
| REQ-108 | Untrusted document bounds | [clinical-modeling.md](clinical-modeling.md#req-108--untrusted-document-bounds) | draft | landed |
| REQ-109 | AQL static lint | [clinical-modeling.md](clinical-modeling.md#req-109--aql-static-lint) | draft | landed |
| REQ-110 | Template-driven validation beyond COMPOSITION | [clinical-modeling.md](clinical-modeling.md#req-110--template-driven-validation-beyond-composition) | draft | landed |
| REQ-111 | Public compiled-template bridge | [clinical-modeling.md](clinical-modeling.md#req-111--public-compiled-template-bridge) | draft | landed |
| REQ-112 | Template-less Reference Model validation floor | [clinical-modeling.md](clinical-modeling.md#req-112--template-less-reference-model-validation-floor) | draft | partial |
| REQ-113 | Execution-oriented parsed AQL AST | [clinical-modeling.md](clinical-modeling.md#req-113--execution-oriented-parsed-aql-ast) | draft | landed |
| REQ-116 | Template-level node naming and name-predicated paths | [clinical-modeling.md](clinical-modeling.md#req-116--template-level-node-naming-and-name-predicated-paths) | draft | landed |
| REQ-117 | AQL expression-catalogue completion | [clinical-modeling.md](clinical-modeling.md#req-117--aql-expression-catalogue-completion) | draft | landed |
| REQ-118 | Deprecated `SELECT TOP` clause and literal source text | [clinical-modeling.md](clinical-modeling.md#req-118--deprecated-select-top-clause-and-literal-source-text) | draft | landed |
| REQ-119 | Re-parseable canonical AQL emission | [clinical-modeling.md](clinical-modeling.md#req-119--re-parseable-canonical-aql-emission) | draft | landed |
| REQ-120 | RM identifier parsing and derivation | [rm-functions.md](rm-functions.md#req-120--rm-identifier-parsing-and-derivation) | draft | landed |
| REQ-121 | Locatable path read access | [rm-functions.md](rm-functions.md#req-121--locatable-path-read-access) | draft | landed |
| REQ-122 | Version-control derived helpers | [rm-functions.md](rm-functions.md#req-122--version-control-derived-helpers) | draft | landed |
| REQ-123 | Temporal data-value helpers | [rm-functions.md](rm-functions.md#req-123--temporal-data-value-helpers) | draft | landed |
| REQ-130 | Contribution builder | [wire.md](wire.md#req-130--contribution-builder) | draft | landed |
| REQ-140 | Underscore-prefixed RM attributes (simplified formats) | [wire.md](wire.md#req-140--underscore-prefixed-rm-attributes) | draft | landed |
| REQ-142 | Contribution read | [wire.md](wire.md#req-142--contribution-read) | draft | landed |
| REQ-143 | Template list filters | [wire.md](wire.md#req-143--template-list-filters) | draft | landed |
| REQ-144 | Definition metadata decoding | [wire.md](wire.md#req-144--definition-metadata-decoding) | draft | landed |
| REQ-150 | Path-parameter segment validation | [transport.md](transport.md#req-150--path-parameter-segment-validation) | draft | landed |
| REQ-151 | Typed 2xx decode failure | [transport.md](transport.md#req-151--typed-2xx-decode-failure) | draft | landed |
| REQ-160 | AQL containment admissibility relation | [clinical-modeling.md](clinical-modeling.md#req-160--aql-containment-admissibility-relation) | draft | landed |
| REQ-161 | AQL semantic and portability lint | [clinical-modeling.md](clinical-modeling.md#req-161--aql-semantic-and-portability-lint) | draft | landed |
| REQ-162 | Builder containment verification | [clinical-modeling.md](clinical-modeling.md#req-162--builder-containment-verification) | draft | landed |
| REQ-163 | AQL write-side expressivity parity | [clinical-modeling.md](clinical-modeling.md#req-163--aql-write-side-expressivity-parity) | draft | landed |
| REQ-164 | AQL path-shape and paging lint | [clinical-modeling.md](clinical-modeling.md#req-164--aql-path-shape-and-paging-lint) | draft | landed |
<!-- END GENERATED: registry -->

**Impl.** column: `landed` (code + tests), `partial` (subset), `planned` (spec only), `retired` (a withdrawn requirement, **Stability** `deprecated`: normative text retained, implementation removed or not shipped; removal target in canonical spec).

---

## Numbering policy

A new requirement takes the **next free number above the highest registered one**, whatever its topic. The number carries no topic meaning; the canonical link says where the requirement belongs. Earlier requirements were numbered in topic bands (001–005 packaging, 010–014 layout, and so on); those numbers stay as they are, but the bands are no longer allocated from.

Identifiers **MUST** be stable once published. Renumbering and reuse are prohibited.

A plan may reserve a number before its spec text exists; record the reservation in the list below, and remove the entry once the requirement is registered. The list is prose, not a table, so the registry stays the only index table in this file.

- REQ-114: reserved for the OPT author validator.
- REQ-115: reserved for the FLAT author linter.
- REQ-124 and REQ-125: reserved for the deferred RM-function stubs.
- REQ-141: retired, never reused.
