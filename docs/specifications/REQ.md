# Requirements registry

**Status:** Draft

Index of normative `REQ-NNN` identifiers for `github.com/cadasto/openehr-sdk-go`. **Canonical prose lives in the linked spec file** — not in this registry. Machine-readable traceability (packages, probes, tests) is in [`traceability.yaml`](traceability.yaml).

Conventions: RFC 2119 keywords — see [README.md § How to read these specs](README.md#how-to-read-these-specs).

**Adding a requirement:** assign the next ID in the topic range (decadal gaps — see [Numbering policy](#numbering-policy)); write normative text in the canonical spec; add a row here and an entry in `traceability.yaml`.

---

## Registry

| ID | Title | Canonical | Impl. |
|---|---|---|---|
| REQ-001 | Module path | [packaging.md § REQ-001](packaging.md#req-001--module-path) | landed |
| REQ-002 | Go version | [packaging.md § REQ-002](packaging.md#req-002--go-version) | landed |
| REQ-003 | License | [packaging.md § REQ-003](packaging.md#req-003--license) | landed |
| REQ-004 | Semantic versioning | [packaging.md § REQ-004](packaging.md#req-004--semantic-versioning) | landed |
| REQ-005 | Internal boundary | [packaging.md § REQ-005](packaging.md#req-005--internal-boundary) | landed |
| REQ-010 | `cadasto/` cut line | [module-layout.md § REQ-010](module-layout.md#req-010--cadasto-cut-line) | landed |
| REQ-011 | No sideways `cadasto/` imports | [module-layout.md § REQ-011](module-layout.md#req-011--no-sideways-imports-inside-cadasto) | landed |
| REQ-012 | Auth layering | [module-layout.md § REQ-012](module-layout.md#req-012--auth-layering) | landed |
| REQ-013 | Building-block independence | [module-layout.md § REQ-013](module-layout.md#req-013--building-block-independence) | landed |
| REQ-014 | Dependency direction | [module-layout.md § REQ-014](module-layout.md#req-014--dependency-direction) | landed |
| REQ-020 | Context-first I/O | [idiom.md § Context propagation](idiom.md#context-propagation-req-020) | landed |
| REQ-021 | Injected `*http.Client` | [idiom.md § HTTP client injection](idiom.md#http-client-injection-req-021) | landed |
| REQ-022 | Functional options | [idiom.md § Functional options](idiom.md#functional-options-req-022) | landed |
| REQ-023 | Package-level functions | [idiom.md § Surface shape](idiom.md#surface-shape-req-023) | landed |
| REQ-024 | Generics, no reflection | [idiom.md § Generics policy](idiom.md#generics-policy-req-024) | landed |
| REQ-025 | Error wrapping | [idiom.md § Errors](idiom.md#errors-req-025) | landed |
| REQ-026 | Goroutine-safe clients | [idiom.md § Concurrency](idiom.md#concurrency-req-026) | landed |
| REQ-030 | Concrete RM structs | [rm-modeling.md § Concrete types](rm-modeling.md#concrete-types-req-030) | landed |
| REQ-031 | Embedded base structs | [rm-modeling.md § Embedded base structs](rm-modeling.md#embedded-base-structs-req-031) | landed |
| REQ-032 | Interfaces for abstract RM | [rm-modeling.md § Abstract categories](rm-modeling.md#abstract-categories-req-032) | landed |
| REQ-033 | No inheritance emulation | [rm-modeling.md § No inheritance emulation](rm-modeling.md#no-inheritance-emulation-req-033) | landed |
| REQ-040 | Type registry | [rm-modeling.md § Type registry](rm-modeling.md#type-registry-req-040) | landed |
| REQ-041 | Pinned BMM sources | [bmm-conformance.md § REQ-041](bmm-conformance.md#req-041--pinned-bmm-sources) | landed |
| REQ-042 | Generated code, drift-detected | [bmm-conformance.md § REQ-042](bmm-conformance.md#req-042--generated-code-drift-detected) | landed |
| REQ-043 | P_BMM → Go mapping rules | [bmm-conformance.md § Mapping rules](bmm-conformance.md#mapping-rules) | landed |
| REQ-044 | Hand-written extensions isolated | [bmm-conformance.md § REQ-044](bmm-conformance.md#req-044--hand-written-extensions-are-isolated) | landed |
| REQ-045 | BMM loader building block | [bmm-conformance.md § REQ-045](bmm-conformance.md#req-045--bmm-loader-is-a-building-block) | landed |
| REQ-046 | Primitive type mapping | [bmm-conformance.md § Primitive type mapping](bmm-conformance.md#primitive-type-mapping) | landed |
| REQ-047 | BMM authoritative on divergence | [bmm-conformance.md § REQ-047](bmm-conformance.md#req-047--bmm-spec-divergence-resolution) | landed |
| REQ-050 | REST 1.1.0-development pin | [wire.md § REQ-050](wire.md#req-050) | landed |
| REQ-051 | Cadasto spec-version header | [wire.md § REQ-051](wire.md#req-051) | landed |
| REQ-052 | Canonical JSON | [wire.md § REQ-052](wire.md#req-052) | landed |
| REQ-053 | FLAT and STRUCTURED | [wire.md § REQ-053](wire.md#req-053) | planned |
| REQ-054 | Optimistic concurrency | [wire.md § REQ-054](wire.md#req-054) | landed |
| REQ-055 | AQL wire boundary | [wire.md § REQ-055](wire.md#req-055--wire-boundary) | landed |
| REQ-056 | Canonical XML | [wire.md § REQ-056](wire.md#req-056) | landed |
| REQ-057 | Stored AQL queries | [wire.md § REQ-057](wire.md#req-057) | landed |
| REQ-058 | Datamap V2 | [module-layout.md](module-layout.md), [scope.md](scope.md) | planned |
| REQ-059 | openEHR custom headers | [wire.md § REQ-059](wire.md#req-059) | partial |
| REQ-060 | TokenSource interface | [auth.md § REQ-060](auth.md#req-060) | landed |
| REQ-061 | SMART-on-openEHR PKCE | [auth.md § REQ-061](auth.md#req-061--pkce-flow) | landed |
| REQ-062 | JWKS rotation | [auth.md § REQ-062](auth.md#req-062--jwks-rotation) | landed |
| REQ-063 | Token refresh | [auth.md § REQ-063](auth.md#req-063--token-refresh) | landed |
| REQ-064 | Launch context | [auth.md § REQ-064](auth.md#req-064--launch-context) | landed |
| REQ-065 | Per-client tenant binding | [auth.md § REQ-065](auth.md#req-065) | landed |
| REQ-066 | Caller attribution | [auth.md § REQ-066](auth.md#req-066) | landed |
| REQ-067 | Platform principal claims | [auth.md § REQ-067](auth.md#req-067) | landed |
| REQ-068 | SMART flows and launch modes | [auth.md § REQ-068](auth.md#req-068--flow-and-launch-mode-coverage) | landed |
| REQ-069 | HTTP Basic on openEHR REST | [auth.md § REQ-069](auth.md#req-069) | landed |
| REQ-070 | First-class discovery | [service-discovery.md § REQ-070](service-discovery.md#req-070) | landed |
| REQ-071 | Discovery cache | [service-discovery.md § REQ-071](service-discovery.md#req-071) | landed |
| REQ-072 | Discovery validation | [service-discovery.md § REQ-072](service-discovery.md#req-072) | landed |
| REQ-073 | Discovery trust posture | [service-discovery.md § REQ-073](service-discovery.md#req-073--discovery-trust-posture) | landed |
| REQ-080 | openEHR wire conformance | [conformance.md § Conformance scope](conformance.md#conformance-scope) | planned |
| REQ-081 | Wire-level parity (retired) | [conformance.md § REQ-081](conformance.md#req-081--wire-level-parity-retired) | deprecated |
| REQ-082 | Probe runnability | [conformance.md § Runnability](conformance.md#req-082--runnability) | partial |
| REQ-083 | Cadasto platform API conformance | [conformance.md § REQ-083](conformance.md#req-083--cadasto-platform-api-conformance) | planned |
| REQ-090 | OpenTelemetry hooks | [transport.md § REQ-090](transport.md#req-090--opentelemetry-hooks) | landed |
| REQ-091 | Retry policy | [transport.md § REQ-091](transport.md#req-091--retry-policy) | landed |
| REQ-092 | TLS posture | [transport.md § REQ-092](transport.md#req-092--tls-posture) | landed |
| REQ-093 | Error envelope mapping | [transport.md § REQ-093](transport.md#req-093--openehr-error-envelope-mapping) | landed |
| REQ-094 | `Prefer` negotiation | [transport.md § REQ-094](transport.md#req-094--prefer-response-shape-negotiation) | landed |
| REQ-095 | OpenAPI authoritative source | [wire.md § REQ-095](wire.md#req-095) | partial |
| REQ-096 | Unambiguous "disable retry" | [transport.md § REQ-096](transport.md#req-096--unambiguous-disable-retry) | landed |
| REQ-097 | First-class `Idempotency-Key` (deprecated) | [transport.md § REQ-097](transport.md#req-097--first-class-idempotency-key-deprecated) | deprecated |
| REQ-098 | Request-level observer hook | [transport.md § REQ-098](transport.md#req-098--request-level-observer-hook) | landed |
| REQ-099 | ITS-REST Admin client surface | [module-layout.md § REQ-099](module-layout.md#req-099--its-rest-admin-client-surface) | landed |
| REQ-100 | ADL 1.4 operational template (OPT) parse and paths | [clinical-modeling.md § REQ-100](clinical-modeling.md#req-100--adl-14-operational-template-opt-parse-and-paths) | landed |
| REQ-101 | Generic OPT-driven composition builder | [clinical-modeling.md § REQ-101](clinical-modeling.md#req-101--generic-opt-driven-composition-builder) | landed |
| REQ-102 | Composition validation | [clinical-modeling.md § REQ-102](clinical-modeling.md#req-102--composition-validation) | landed |
| REQ-103 | Primitive constraint introspection | [clinical-modeling.md § REQ-103](clinical-modeling.md#req-103--primitive-constraint-introspection) | landed |
| REQ-104 | Slot assertion grammar | [clinical-modeling.md § REQ-104](clinical-modeling.md#req-104--slot-assertion-grammar) | landed |
| REQ-105 | Terminology bindings | [clinical-modeling.md § REQ-105](clinical-modeling.md#req-105--terminology-bindings) | landed |
| REQ-107 | Template-driven RM instance example generator | [clinical-modeling.md § REQ-107](clinical-modeling.md#req-107--template-driven-rm-instance-example-generator) | landed |
| REQ-108 | Untrusted document bounds | [clinical-modeling.md § REQ-108](clinical-modeling.md#req-108--untrusted-document-bounds) | landed |
| REQ-109 | AQL static lint | [clinical-modeling.md § REQ-109](clinical-modeling.md#req-109--aql-static-lint) | landed |
| REQ-110 | Template-driven validation beyond COMPOSITION | [clinical-modeling.md § REQ-110](clinical-modeling.md#req-110--template-driven-validation-beyond-composition) | landed |
| REQ-111 | Public compiled-template bridge | [clinical-modeling.md § REQ-111](clinical-modeling.md#req-111--public-compiled-template-bridge) | landed |
| REQ-120 | RM identifier parsing and derivation | [rm-functions.md § REQ-120](rm-functions.md#req-120--rm-identifier-parsing-and-derivation) | landed |
| REQ-121 | Locatable path read access | [rm-functions.md § REQ-121](rm-functions.md#req-121--locatable-path-read-access) | landed |
| REQ-122 | Version-control derived helpers | [rm-functions.md § REQ-122](rm-functions.md#req-122--version-control-derived-helpers) | landed |
| REQ-123 | Temporal data-value helpers | [rm-functions.md § REQ-123](rm-functions.md#req-123--temporal-data-value-helpers) | landed |

**Impl.** column: `landed` (code + tests), `partial` (subset), `planned` (spec only), `deprecated` (normative text retained; implementation removed or not shipped — removal target in canonical spec). Detail in [`traceability.yaml`](traceability.yaml).

---

## Numbering policy

| Topic | REQ range | Headroom |
|---|---|---|
| Module identity / packaging | 001–005 | 006–009 |
| Boundaries / layout | 010–014 | 015–019 |
| Idiomatic surface | 020–026 | 027–029 |
| Reference Model | 030–040 | — |
| BMM conformance | 041–047 | 048–049 |
| Wire format | 050–059 | — |
| Authentication | 060–068 | 069 |
| Service discovery | 070–073 | 074–079 |
| openEHR conformance | 080–082 | 083–089 |
| Transport / observability | 090–092 | — |
| REST binding | 093–095 | — |
| Transport / REST extensions | 096–099 | — |
| Clinical modeling | 100–109 | 110–119 |
| RM behavioural functions | 120–123 | 124–129 |

Identifiers **MUST** be stable once published. Renumbering is prohibited.
