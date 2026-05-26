# Glossary

**Status:** Draft

Terms used in the SDK specifications, source code, and AI-agent prompts. When a term has an authoritative external definition, the entry quotes or paraphrases that definition; when the term is SDK-internal, the entry names where the contract lives.

---

## openEHR core

**Reference Model (RM)**
The information model that defines the abstract data structures of openEHR — `LOCATABLE`, `PATHABLE`, `COMPOSITION`, `OBSERVATION`, `EVALUATION`, `INSTRUCTION`, `ACTION`, `DATA_VALUE` and its specialisations, the demographic `PARTY` hierarchy, etc. Authoritative definition: openEHR Reference Model specification. In this SDK the RM Go types are **generated** from the pinned `openehr_rm_*.bmm.json` schema (REQ-041, REQ-042).

**BMM (Basic Meta-Model)**
The openEHR meta-model that describes information models (RM, AM, BASE, LANG, TERM) as a compact, computable alternative to UML/XMI. Defined in the openEHR LANG specification. In this SDK the BMM files in [`../resources/bmm/`](../resources/bmm/) are the source of truth for the openEHR domain model; see [bmm-conformance.md](bmm-conformance.md).

**P_BMM**
The persistence (serialisation) form of BMM — typically a JSON document (the `.bmm.json` files in [`../resources/bmm/`](../resources/bmm/)) keyed by `schema_id` (e.g. `openehr_rm_1.2.0`). On-disk attribute names appear as `P_BMM_*` (e.g. `P_BMM_SINGLE_PROPERTY`, `P_BMM_CONTAINER_TYPE`, `P_BMM_GENERIC_PROPERTY`); the in-memory model uses the abstract `BMM_*` equivalents.

**Archetype Object Model (AOM)**
The in-memory model representing an *archetype* after parsing ADL. Sibling of the Reference Model — both are top-level openEHR information models. Two versions exist: **AOM 1.4** (for ADL 1.4 archetypes) and **AOM 2** (for ADL 2). The SDK generates `openehr/aom/aom14/` for v1; AOM 2 is deferred. Templates *consume* AOM (an OPT contains flattened archetype definitions) but do not own it.

**Archetype**
A constraint model on an RM class, expressed in ADL (Archetype Definition Language). Defines the structural and value constraints under which a domain concept (e.g. "blood pressure measurement") is recorded. Archetypes are reusable across templates.

**Operational Template (OPT)**
A flattened, validation-ready artifact derived from an authoring template (`.oet`) and its referenced archetypes. The OPT is what a CDR uses to validate incoming Compositions (`OPERATIONAL_TEMPLATE` XML, typically `.opt`). The SDK v1 parser in `openehr/template/` consumes **OPT only**; the parsed type is `OperationalTemplate`. OET is out of scope for v1.

**Composition**
The primary openEHR clinical document unit — a versioned, signed record of one or more clinical observations / evaluations / instructions / actions, anchored to an EHR and produced under a Template.

**EHR / EHR_STATUS**
An EHR is the patient-bound container for clinical Compositions and the Directory. `EHR_STATUS` is its singleton metadata record (queryability, modifiability, subject linkage).

**Directory**
The folder structure inside an EHR — a tree of `FOLDER`s referencing Compositions. Versioned.

**Contribution**
A multi-resource atomic commit envelope — one Contribution carries one or more `VERSION<T>` payloads that succeed or fail together.

**Demographic resources**
The openEHR demographic side — `PARTY`, `PERSON`, `ORGANISATION`, `AGENT`, `ROLE`, `CAPABILITY`, identities, relationships. Hosted separately from clinical EHRs in some deployments; on Cadasto the demographic surface is provided by the platform.

**AQL (Archetype Query Language)**
The openEHR query language. Path-based, archetype-aware; structurally similar to SQL but operating on Compositions and demographic resources. The SDK exposes AQL via `openehr/aql` (builders + models) and `openehr/client/query` (executor).

**Canonical JSON**
The openEHR ITS-JSON canonical serialization of an RM instance. Distinguished from `FLAT` and `STRUCTURED` by retaining full RM structure (every `_type` discriminator, every nested object). The wire format for openEHR REST 1.1.0-development.

**FLAT format**
A path-keyed, value-flat serialization optimised for end-user input forms — keys are openEHR paths, values are leaf scalars. The SDK supports it as an input/output format on the composition surface.

**STRUCTURED format**
A nested-JSON serialization that preserves RM structure but omits `_type` discriminators where the path is unambiguous. Intermediate between canonical JSON and FLAT.

**ITS-REST**
"Implementation Technology Specification — REST" — the openEHR REST API specification family. The SDK targets the `1.1.0-development` release line; the term "REST" without further qualification in these specs refers to ITS-REST. The per-endpoint **OpenAPI YAML** files at `github.com/openEHR/specifications-ITS-REST` are the authoritative source for paths, parameters, and status codes (REQ-095).

**ItemTags / `openehr-item-tag`**
A small key-value annotation surface attached to versioned resources, introduced in openEHR REST 1.1.0. The SDK exposes it under `openehr/client/ehr/itemtags/` and carries the `openehr-item-tag` header (REQ-059) on the relevant operations.

**openEHR custom header family (`openehr-*`)**
The set of openEHR-specific HTTP headers defined by ITS-REST 1.1.0: `openehr-version`, `openehr-audit-details`, `openehr-template-id`, `openehr-uri`, `openehr-item-tag`. The SDK exposes them as typed per-call options, never as raw strings (REQ-059).

**`openehr-audit-details`**
A request-side header carrying the commit-time audit envelope (committer, time, change-type, optional `description`) on versioned writes. The SDK builds it from a typed `*rm.AuditDetails` and serialises it via the canonical-JSON codec.

**`Prefer: return=…`**
Standard HTTP header used by openEHR REST 1.1.0 to negotiate the response body on write paths: `minimal`, `identifier`, or `representation`. The SDK exposes a typed `transport.Prefer` enum and a per-call option `WithPrefer(Prefer)`. Defaults: `minimal` for writes, none for reads (REQ-094).

**Error envelope (openEHR REST)**
The structured `{message, code, coded_text[]}` JSON body returned on non-2xx responses. Decoded by the SDK into `transport.OpenEHRErrorDetail` attached to a `transport.WireError`; consumers detect class via `errors.Is`, inspect detail via `errors.As` (REQ-093).

---

## Authentication and authorisation

**SMART-on-openEHR**
The SMART App Launch protocol adapted to openEHR. Defines an OAuth2 authorization-code-with-PKCE flow with openEHR-specific scope syntax (`<compartment>/<resource>.<permission>`), launch context (patient, user, encounter), and a service catalog discovery document.

**PKCE (Proof Key for Code Exchange)**
RFC 7636. The mandatory OAuth2 extension SMART-on-openEHR uses to bind an authorization code to the client without a shared secret. Implemented in `auth/smart`.

**JWKS (JSON Web Key Set)**
The endpoint and document publishing the authorization server's signing keys. The SDK consumes the issuer's JWKS to validate ID tokens and (in some deployments) access tokens.

**Launch context**
The set of contextual values delivered alongside a SMART access token — `patient`, `encounter`, `user`, plus any `launch-*` parameters defined by the deployment. Exposed by `smart/` as typed values.

**TokenSource**
The SDK's generic auth abstraction (`auth.TokenSource`). Returns a bearer token and an expiry. Implemented by `auth/smart`, `auth/clientcreds`, `auth/jwtbearer`, and any future provider.

**Client Credentials**
OAuth2 grant for service-to-service callers. No interactive user; client authenticates with its own credentials. Implemented in `auth/clientcreds`.

**JWT Bearer**
OAuth2 grant (RFC 7523) where the client presents a signed JWT assertion and exchanges it for an access token. Implemented in `auth/jwtbearer`.

---

## Discovery

**ServiceCatalog**
The SDK's first-class representation of a SMART-on-openEHR deployment's service base URLs (`org.openehr.rest`, Cadasto-specific service identifiers, etc.). Defined in `smart/discovery`. SDK constructors take a `ServiceCatalog`, never a single base URL (REQ-070).

**Discovery document**
The SMART-on-openEHR configuration document advertised by the deployment — typically at a well-known URL — that declares the authorization endpoint, token endpoint, registration endpoint, JWKS URI, and the service catalog.

---

## SDK internals

**Conformance probe**
An executable assertion that the SDK exercises against either the sandbox transport, a recorded cassette, or a live deployment, to verify wire-level conformance to openEHR REST + SMART-on-openEHR. Each probe has a stable `PROBE-NNN` ID (see [conformance.md](conformance.md)).

**Building-block use case**
A consumer that imports one core package (`openehr/rm`, `openehr/serialize`, `openehr/validation`, `openehr/aql` models-only, `openehr/template`) without constructing an authenticated client. The SDK's surface MUST support this (REQ-013).

**Sandbox**
The in-memory + recorded-fixture transport in `sandbox/`, implementing the same client interfaces as the production REST clients. Used for hermetic tests in SDK consumers.

**Testkit**
The package `testkit/` carrying test doubles, fluent builders, recorder/replay helpers, and the conformance-probe runner.

**Cut line**
A package-tree boundary that nothing on the upstream side may import from. The `cadasto/` subtree is the load-bearing cut line for v1 (REQ-010, REQ-011).

---

## Cadasto-platform terms

**CDR (Clinical Data Repository)**
The Cadasto openEHR-on-Postgres service. The reference CDR implementation (private; first SDK consumer) is its Go codebase.

**Cadasto Extra API**
A Cadasto-specific REST surface that complements openEHR REST — convenience aggregates, deployment-specific endpoints. Client lives in `cadasto/extra`.

**Datamap (V2)**
A Cadasto-specific format for resource-free read and write of clinical and demographic data across the openEHR REST API surface (Compositions, Demographics, EHR Status). The SDK targets **Datamap V2** (REQ-058); older versions are out of scope. Client in `cadasto/datamap`.

**MPI (Master Patient Index)**
Identity resolution and patient-merging across deployments. The SDK exposes a preview shape in `cadasto/mpi/`; the full design is the subject of a separate research strand.

**Care aggregates**
Cadasto's opinionated application-level aggregates over EHR + Demographic resources — `Patient`, `User`, `CaseLoad`, `CareTeam`, `Episode`. Live in `cadasto/care/`.

**Federator**
A consumer pattern: multiple SDK clients pointed at multiple openEHR backends, with per-node spec pinning, partial-failure semantics, and a merge / authority policy. The SDK provides the per-node primitives; the federation policy is bespoke per deployment.

---

## Cross-language / cross-SDK

**Cadasto SDK Specification proposal**
The pre-implementation design document for this SDK, maintained alongside Cadasto architectural sources. Its content is reflected in this `docs/specifications/` tree; this tree is the day-to-day source of truth.

**PHP SDK / Cadasto PHP SDK**
The sister SDK targeting the same openEHR REST surface, same SMART-on-openEHR conformance probe set, with PHP-idiomatic APIs (repositories + builders, exceptions). Cross-language parity is enforced at the wire level (REQ-081).

**Conformance probe parity**
The contract that the same `PROBE-NNN` is implementable identically in both Go and PHP SDKs and produces the same pass/fail against a reference deployment (REQ-080).

---

## RFC 2119 keywords

See [README.md § How to read these specs](README.md#how-to-read-these-specs). The keywords appear in `**bold**` in spec files to make them grep-able.
