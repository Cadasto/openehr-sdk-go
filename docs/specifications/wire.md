# Wire format

**Status:** Draft

The normative contract between the SDK and any conformant openEHR backend (Cadasto CDR, EHRbase, others). Covers REQ-050 through REQ-059 (wire surface and openEHR headers), REQ-095 (OpenAPI authoritative source), REQ-130 (contribution builder), REQ-140 (underscore-prefixed RM attributes), REQ-142 (contribution read), and REQ-143 (template list filters). The wire-extension band 140–149 continues the exhausted 050–059 band; the SDK authoring & client-tooling band 130–139 opens here with REQ-130. Transport hygiene (REQ-090–094, REQ-150) lives in [transport.md](transport.md).

The premise: correctness is wire-level (REQ-080). The bytes on the wire and the AQL strings conform to the openEHR spec; the Go source shape is independent.

## Functional API areas

openEHR REST 1.1.0-development partitions the surface into **six** functional areas. The SDK provides a typed leaf package per area under `openehr/client/`:

| Area | Scope | SDK package |
|---|---|---|
| **System** | service capabilities, version, infrastructure discovery | `openehr/client/system/` |
| **EHR** | EHR + sub-resources: Composition, Contribution, Directory/Folder, EHR_STATUS, ItemTags | `openehr/client/ehr/` (+ sub-leaves) |
| **Query** | ad-hoc AQL execution; stored-query invocation; `RESULT_SET` shape | `openehr/client/query/` |
| **Definition** | ADL2/ADL1.4 archetypes, OPTs/templates, example generation, stored queries | `openehr/client/definition/` |
| **Demographic** | parties, relationships, identities (upstream Status: development) | `openehr/client/demographic/` |
| **Admin** | EHR physical delete, administrative lifecycle (upstream Status: development) | `openehr/client/admin/` |

The split is normative: a consumer who needs only AQL imports `openehr/client/query/` without pulling in the EHR or Definition surface (REQ-013).

## Authoritative source

### REQ-095

For per-endpoint detail (paths, parameters, request/response schemas, status codes), the **upstream openEHR OpenAPI YAML files** are authoritative — they are the openEHR Foundation's machine-readable form of the REST contract, the analogue of the BMM files (REQ-041) for the type surface.

Pinned source: `https://github.com/openEHR/specifications-ITS-REST/tree/master/computable/OAS`. The SDK's plans and test cassettes record the upstream commit they were validated against; bumping it is an explicit, reviewable change.

When the OpenAPI files and any in-repo prose disagree, the OpenAPI wins; the prose is updated, not the wire behaviour.

**Path-parameter encoding.** A request path **MUST** conform to the OAS path template — each path parameter is percent-encoded **exactly once** on the wire. The transport is the **single canonical path encoder**: [`transport.Request.Path`](../../transport/request.go) is a **decoded** path (`url.URL.Path` semantics) that `url.URL.String()` encodes once on the way out. Leaf clients (`openehr/client/*`) **MUST** interpolate the **raw**, decoded id into `Request.Path` and **MUST NOT** pre-escape it with `url.PathEscape` — a pre-escaped parameter is encoded twice (a template id `Referral Request.v1` → `%20` → `%2520`), which a strict server unescapes to a literal `%20` and answers `404`. Segment legality is a separate question from encoding: a path parameter containing `/` — or any other content [REQ-150](transport.md#req-150--path-parameter-segment-validation) forbids — is governed by that requirement, the transport's segment validator — which also forbids honouring `url.URL.RawPath` (the encoded hint), so there is no encoding-level escape hatch for a separator-bearing value; the MUST NOT lives there, not here. Decoding a server-supplied value (e.g. the `Location` header via `url.PathUnescape`) is unaffected — the rule is about **forming** the request path, not reading a response.

## REST version pin

### REQ-050

The SDK targets **openEHR REST `1.1.0-development`** as its primary contract surface. Concretely:

- Endpoint shapes, request/response envelopes, and status-code semantics follow the 1.1.0-development specification.
- The `Cadasto-OpenEhr-Spec-Version` header (REQ-051) is the wire signal that distinguishes Cadasto deployments from generic openEHR backends; it does not change the request bodies or paths.
- Earlier versions (1.0.3, 1.0.2) are not targeted. A consumer who needs to talk to a 1.0.3 deployment can do so against a build of the SDK that has the 1.0.3 quirks documented; v1 does not promise backwards compatibility across REST major/minor versions.

The version pin is enforced at discovery time (REQ-072), not on the first request. A mismatched advertised version **MUST** fail fast with a typed `DiscoveryError`.

## Cadasto spec-version header

### REQ-051

The SDK **MAY** send a `Cadasto-OpenEhr-Spec-Version` header on outgoing requests:

```
Cadasto-OpenEhr-Spec-Version: 1.1.0-development
```

This is a **Cadasto-platform-specific** signal. The SDK **MUST**:

- Keep the header **off by default**.
- Enable it only when a Cadasto deployment is detected (e.g. via the discovery document) or when a functional option (`transport.WithCadastoSpecVersionHeader(true)`) is set.
- Strip the header from any cross-origin (CORS) preflight if browsers are in the request path (the SDK is not directly used in a browser, but the rule documents the intent).

Sending this header to a non-Cadasto openEHR backend **MUST NOT** happen automatically — generic backends may reject unknown headers depending on policy.

## openEHR custom header family

### REQ-059

openEHR REST 1.1.0-development defines a family of `openehr-*` custom headers carrying RM and template-level metadata at the wire layer. The SDK **MUST** support them via **typed per-call options**, never via raw header maps on the consumer's surface.

| Header | Direction | Carries | Typed option |
|---|---|---|---|
| `openehr-version` | request (writes) | committed VERSION lifecycle state, as `lifecycle_state.code_string="<code>"` | `WithLifecycleState(code)` on the affected write |
| `openehr-audit-details` | request (writes) | commit-time audit envelope: committer, change-type, description, system_id | `transport.WithAuditDetails(*rm.AuditDetails)` |
| `openehr-template-id` | request (composition writes) | declares the template id the payload conforms to | `composition.WithTemplateID(string)` |
| `openehr-uri` | request / response | opaque openEHR resource pointer (selected endpoints) | typed on the affected method |
| `openehr-item-tag` | request / response | ItemTag operations (REST 1.1.0 new resource) | exposed on `openehr/client/ehr/itemtags/` |

When formatting `openehr-item-tag` header values, the SDK **MUST** reject keys, values, and target paths that contain control characters (bytes `< 0x20` except tab, and `DEL` `0x7F`) — a caller-supplied tag key or value with embedded CR/LF is a caller error, not a sanitisation opportunity (header injection).

Response headers in this family **MUST** be surfaced on the typed response metadata returned by each method (alongside `ETag`, `Location`).

The SDK **MUST NOT** require consumers to construct the audit envelope by hand — `*rm.AuditDetails` is a generated RM type per REQ-042.

**Header grammar (not JSON).** The `openehr-audit-details` and `openehr-version` **request headers** carry their payload as the openEHR dotted-attribute grammar — a comma-separated list of `attribute.path="value"` assignments — **not** as a JSON object. The SDK **MUST** encode `openehr-audit-details` from an `AUDIT_DETAILS` as the documented attributes (`change_type.code_string`, `description.value`, `committer.name`, `committer.external_ref.{id,namespace,type}`, `system_id`) and `openehr-version` as `lifecycle_state.code_string="<code>"`. Canonical-JSON / canonical-XML serialisation of `AUDIT_DETAILS` applies **only** to the contribution request **body** (the `commit_audit` / `UpdateAudit` field per REQ-057), never to these headers. The grammar and worked examples are normative in the upstream contract — `resources/its-rest/overview-validation.openapi.yaml` § *"openehr-version and openehr-audit-details"*. Per REQ-095 the OpenAPI contract is authoritative here. Header values **MUST** reject embedded control characters on the same header-injection grounds as `openehr-item-tag` above.

## `Prefer` negotiation and error envelope

REQ-094 (`Prefer`) and REQ-093 (structured error envelope) are normative in [transport.md](transport.md). Leaf clients under `openehr/client/*` consume them via `transport.Client`.

## Canonical JSON

### REQ-052

The SDK's primary write payload **MUST** be openEHR canonical JSON. Read payloads **MUST** be accepted in canonical JSON; FLAT and STRUCTURED inputs flow through codec conversion in `openehr/serialize`.

Canonical-JSON properties:

- Every RM type instance carries `_type`. The encoder **MUST** emit it; the decoder **MUST** consult the type registry (REQ-040).
- Field order **SHOULD** follow the openEHR canonical-JSON specification when one is published; until then the SDK **MUST** use this deterministic profile (see [`docs/plans/archive/2026-05-15-canonical-json-serialization.md`](../plans/archive/2026-05-15-canonical-json-serialization.md)):
  - `_type` is always the first key on every encoded concrete RM value.
  - Remaining object keys follow **BMM property declaration order** (the order code generation emits struct fields).
  - `Hash` (`map[K]V`) keys are serialized in **lexicographic key order** (independent of struct field order).
- Numbers, booleans, strings, arrays, objects are JSON-vanilla — no openEHR-flavoured encoding tricks.
- `DV_QUANTITY` magnitudes are emitted as JSON numbers, not strings, unless the spec mandates otherwise (some implementations have used strings to avoid float-precision loss; the SDK takes a position — see § Floating-point precision below).

**Encode-side refusal sentinel.** A `canjson.Marshal` or `canjson.MarshalIndent` failure **MUST** wrap the encode-only sentinel `canjson.ErrInvalidValue`, so a caller can tell "this value cannot be encoded" from "these bytes cannot be decoded" with `errors.Is` alone rather than by reading the error string. `canjson.ErrInvalidValue`, the decode-side `canjson.ErrInvalidShape`, and the transport-level `transport.ErrInvalidShape` **MUST** remain three distinct sentinel values: `errors.Is` against any one of them **MUST NOT** match an error raised for either of the other two, and `canjson.ErrInvalidValue` **MUST NOT** appear on any decode path. The underlying encoder error (for example `*json.UnsupportedTypeError`) **MUST** stay reachable through unwrapping, so the sentinel adds a classification without costing a diagnostic. Leaf packages that wrap `canjson.Marshal` in their own `fmt.Errorf` chains need no change — the sentinel travels inside those wraps. The canonical-XML codec (`openehr/serialize/canxml`) reuses a single sentinel across both directions; that asymmetry is observed here, not resolved — aligning it is out of scope for this clause.

### Floating-point precision

Numeric magnitudes are serialised as IEEE 754 double-precision JSON numbers. The SDK **MUST NOT** silently coerce a magnitude through `float32` or a similarly lossy intermediate. If a wire value exceeds JSON's number precision (rare in clinical data), the SDK **MUST** report this on decode as a typed error rather than silently rounding.

Some upstream producers (notably legacy CDR exporters) emit `Real` / `Integer` magnitudes as quoted decimal strings. The SDK adopts **asymmetric tolerance**: encode is strict (numbers only); decode accepts either a JSON number or a quoted decimal string. The full rule and its rationale live in [`docs/adr/0004-numeric-wire-tolerance.md`](../adr/0004-numeric-wire-tolerance.md). The asymmetric profile is part of the openEHR wire contract this SDK follows (REQ-080).

Golden canonical-JSON composition inputs for codec and PROBE-030 live under `testkit/cassettes/compositions/` and `testkit/cassettes/rm/` (see [Vendored fixtures](conformance.md#vendored-fixtures-testkitcassettes)). Example: `compositions/BMI.json` for quoted-number magnitudes ([ADR 0004](../adr/0004-numeric-wire-tolerance.md)).

### Polymorphic substitution

The openEHR RM permits Liskov substitution at every property slot: a slot whose declared type is `T` admits any concrete subtype of `T` as a runtime instance, by AOM `valid_value` semantics. Two cases the canonical-JSON codec **MUST** handle losslessly:

1. **Substitutable subtype in a concrete-typed slot.** When a property's declared type is itself concrete but has registered subtypes per the BMM `ancestors` graph (`LOCATABLE.name: DV_TEXT` admitting `DV_CODED_TEXT`, `EVENT_CONTEXT.health_care_facility: PARTY_IDENTIFIED` admitting `PARTY_RELATED`, etc.), the wire `_type` discriminator drives dispatch. The SDK surfaces this via **narrow Go interfaces** (`<Parent>Like` — `DVTextLike`, `PartyIdentifiedLike`, `AuditDetailsLike`, `DVURILike`, `ObjectRefLike`) generated by `bmmgen` from the ancestors graph; the wire decoder routes through `typereg.DecodeAs[<Like>]`.
2. **Generic type parameterised over an abstract bound.** When a generic class instantiates over an abstract bound (`DV_INTERVAL[T: DV_ORDERED]`), the field is dispatched via `typereg.DecodeAs[T]` at decode time; this handles both interface-T instantiations (`DVInterval[DVOrdered]` used by reference ranges) and concrete value-T instantiations (`DVInterval[DVQuantity]` used by `DVQuantity.NormalRange`).

**Missing-`_type` tolerance:** canonical JSON SHOULD carry `_type` everywhere, but real-world cassettes elide it on concrete-typed slots where the static field fixes the subtype (e.g. `"name": {"value": "Tree"}` on an `ITEM_TREE`). The decoder falls back to the **declared parent's concrete type** when the wire omits `_type` on a narrow-interface slot; this preserves backward compatibility with permissive producers without compromising the strict-abstract-slot rule (`DATA_VALUE`, `DV_ORDERED`, `ITEM_STRUCTURE`, `PARTY_PROXY` still require `_type`).

The full substitution semantics are pinned by [PROBE-038](conformance.md#probe-038--rm-polymorphic-decode-coverage) (decode + re-marshal preserves every input `_type` discriminator). On BMM bumps that introduce new subtypes ([ADR 0001](../adr/0001-bmm-version-bump-runbook.md) step 10), `make codegen` auto-extends the relevant `<Parent>Like` interface (marker methods on the new concrete class); the closed type-switches in [`openehr/rm/like_accessors.go`](../../openehr/rm/like_accessors.go) still need an explicit `case *NewSubtype:` arm per new descendant, plus a round-trip case in [`openehr/serialize/canjson/polymorphic_decode_test.go`](../../openehr/serialize/canjson/polymorphic_decode_test.go), so PROBE-038's substitution guarantee covers it.

## Canonical XML

### REQ-056

The SDK **MUST** provide a canonical XML codec in `openehr/serialize`, symmetric to the canonical JSON codec — same type-registry consultation (REQ-040), same OPT-driven validation hooks, same independence from `transport/` (REQ-013).

Canonical XML applies to the same RM surface as canonical JSON: Composition, EHR_STATUS, Directory, Contribution, demographic resources. Polymorphic discrimination uses the `xsi:type` attribute (XML Schema Instance namespace), not the JSON `_type` property. Element names **MUST** be snake_case BMM names (same as canonical JSON keys). The codec **MUST** carry the namespace declarations the openEHR XML schemas require (`http://schemas.openehr.org/v1` default namespace; `xmlns:xsi` when `xsi:type` is present).

Canonical ordering for XML **MUST** mirror the JSON profile (see [`docs/plans/archive/2026-05-15-canonical-xml-serialization.md`](../plans/archive/2026-05-15-canonical-xml-serialization.md)):

- Child elements follow **BMM property declaration order** (same order code generation emits struct fields).
- `xsi:type` is the **first attribute** on every encoded concrete RM value where a polymorphic site is being resolved; the encoder emits it on every concrete value boundary (deterministic profile), the decoder requires it at polymorphic sites unless [`WithRelaxedTypeDispatch`] is set.
- Nil-pointer optional fields and empty containers with `cardinality.lower == 0` are emitted as **ABSENT** (no element). Both ABSENT and an empty self-closing element are accepted on decode.
- ISO 8601 dates/times/durations are passed through as element text content; the codec does not parse them at codec layer (REQ-046).
- Numeric magnitudes use IEEE 754 double-precision (same posture as canonical JSON); decode also accepts quoted decimal strings per [`docs/adr/0004-numeric-wire-tolerance.md`](../adr/0004-numeric-wire-tolerance.md).
- Compact XML (no insignificant inter-element whitespace) is the byte-equality target for round-trip tests.
- `xmi:type` is **rejected** on decode with `ErrInvalidShape` and an explicit message — only `xsi:type` is recognised.
- `xsi:type` discriminator **values** **MUST** be matched as **unprefixed** RM class names; a leading `xsd:` on a foundation-primitive value (`xsd:string`) is stripped to the BMM primitive name. A **namespace-prefixed** discriminator value — the Better/Marand `xsi:type="ns2:DV_QUANTITY"` form, where the RM namespace is bound to a prefix rather than being the default — is **out of v1 scope**: the decoder **MUST NOT** resolve it and **MUST** fail closed with `ErrUnknownType` (never silently mis-resolved). The `xsi:type` attribute itself is recognised only when the `xsi` prefix is bound to the XSI namespace; an `xsi:type` written with no in-scope `xmlns:xsi` declaration is treated as a **missing** discriminator (`encoding/xml` yields the unresolved prefix, not a match). Supporting the prefixed form later means resolving the value's prefix against its in-scope `xmlns` binding (not a blind strip) plus a focused decode fixture — the boundary is pinned by `TestUnmarshalRejectsPrefixedXSIType`.

XML is a second-class format on the wire today (REST 1.1.0-development is JSON-first), but several integration scenarios pin to XML for legacy reasons. The SDK supports it without forcing it.

Golden canonical-XML inputs for codec and PROBE-033 live under `testkit/cassettes/compositions/` and `testkit/cassettes/rm/` (same layout as REQ-052; see [Vendored fixtures](conformance.md#vendored-fixtures-testkitcassettes)).

## Simplified formats

### REQ-053

The SDK **MUST** provide codecs in `openehr/serialize` for the openEHR **FLAT** and **STRUCTURED** *Simplified Formats* — the JSON serializations of a composition **data instance** standardised by the openEHR ITS-REST [*Simplified Formats*](https://specifications.openehr.org/releases/ITS-REST/development/simplified_formats.html) specification (STABLE, targeting 1.1.0). The spec names exactly these two variants — *Flat* and *Structured*; the earlier "Simplified Data Template (SDT)" naming (and EHRbase's `simSDT`/`structSDT` labels) is superseded and **MUST NOT** appear in the SDK's public surface. This section pins to that document for the wire grammar; it does not re-define it.

Both variants serialize the **same** RM data (a `COMPOSITION`) under **template-specific**, human-readable field identifiers taken from the template's *Web Template* projection (REQ-106) — **not** canonical AQL/AOM paths:

- Path segments are **Web Template `id`s** (e.g. `blood_pressure`, `systolic`), joined by `/` and rooted at the template id — never archetype at-codes.
- Repeating nodes carry a zero-based instance index `:0` / `:1`.
- Leaf attributes are pipe suffixes (`|magnitude`, `|unit`, `|code`, `|value`, `|terminology`, …); an `ELEMENT` collapses into its value (no trailing `/value`). Exactly **one subtype substitution** is carried in suffix form: a `DV_CODED_TEXT` stored at a `DV_TEXT`-typed leaf (legal RM substitution) **MUST** be emitted and accepted in the `DV_CODED_TEXT` suffix form — `|code` + `|value` + `|terminology` (plus the other modelled `DV_CODED_TEXT` suffixes, e.g. `|formatting`), with **no** bare-key spelling — matching the reference implementation, whose corpus carries the rubric under `|value` (`ehrbase_conformance_data_types_dv_coded_text_as_dv_text`); every other substituted subtype (the value's dynamic type differing from the leaf's declared type) rides `|raw`. The inverse substitution — an uncoded `DV_TEXT` at a `DV_CODED_TEXT` leaf — stays the `|other` open-value-set form.
- Structural levels are removed relative to the canonical path: container attributes (`content`, `data`, `events`, `items`, …) are elided, and the `ITEM_STRUCTURE` family, `HISTORY`, and single unnamed `EVENT`s are collapsed.
- Composition-level metadata is carried under the `ctx/` prefix (mandatory `language`, `territory`; optional `composer`, `time`, `setting`). The reference implementation instead spells several of these as real paths under the template root (`<root>/language|code`, `<root>/composer|name`, `<root>/context/start_time`, …), carrying the same information. Decode **MUST** accept either spelling for a field that has an exact `ctx/` equivalent; encode **MUST** emit only the `ctx/` short form. Where both spellings appear for one field and disagree, the codec **MUST** fail rather than choose. A real-path `|terminology` witness that differs from the terminology the `ctx/` form implies **MUST** be rejected rather than silently rewritten; a matching witness carries no information the `ctx/` form lacks and is discarded. `context/setting` **is** such a respelling: encode **MUST** emit `ctx/setting|code` + `ctx/setting|value` when the source `EVENT_CONTEXT.setting` is populated (the all-zero value writes nothing), and decode **MUST** accept either spelling under the same disagreement and witness rules — the implied terminology is `openehr`, and a setting coded in any other terminology is a typed error, not a silent rewrite. Implying it is not a guess: the RM invariant `Setting_valid` on `EVENT_CONTEXT` requires `setting.defining_code` to be a member of the openEHR terminology's `setting` group, so `openehr` is the only terminology a conformant setting can carry and decode completing it is spec-conformant — the same footing as `ISO_639-1` for `language`. A witness naming anything else therefore contradicts the RM, which is why it is refused rather than honoured. Encode **MUST** likewise refuse a populated setting it cannot carry in that form rather than omit it; producers of RM-invalid settings are the defect, not the wire form. The ctx-only emission rule is scoped to the **six respelled scalar fields** (`language`, `territory`, `composer_name`, `composer_self`, `time`, `setting`). One composition-level family is **not** a respelling: the composer's `external_ref` suffixes (`composer|id`, `|id_scheme`, `|id_namespace`) — and its `_identifier:N` list (§ REQ-140) — stay refused **on decode** and visible in the PROBE-086 census, because no `ctx/` short form can carry them. The `ctx/` short forms the *Simplified Formats* spec sketches for the **EVENT_CONTEXT optionals** (participations, `health_care_facility`, `end_time`, `location`) are not part of this contract: those attributes ride the underscore grammar under the real `context` segment (§ REQ-140, [ADR 0016](../adr/0016-event-context-optionals-underscore-spelling.md)). See [ADR 0015](../adr/0015-flat-metadata-spelling.md).
- Optional RM attributes the template does not constrain use an underscore prefix (`_uid`, `_link`, `_normal_range`, …) — the normative attribute vocabulary and value grammar are [§ REQ-140](#req-140--underscore-prefixed-rm-attributes); the `|raw` suffix embeds a pre-serialized canonical RM fragment, which **MUST** carry `_type`.

**FLAT** is a single-level map of `path → primitive | object`. **STRUCTURED** is the same data as nested JSON keyed by the same segment `id`s, where every data value is wrapped in an **array** (even at cardinality `0..1` / `1..1`) and attribute suffixes appear as `|`-prefixed keys. A leaf's **bare** FLAT value is the array element itself; where the same leaf also carries `|`-suffixes or nested members — a DV_MULTIMEDIA's uri beside its mandatory `|mediatype`, a DV_TEXT beside `|formatting`, any bare leaf carrying a `_`-attribute (§ REQ-140) — that element **MUST** be an object and the bare value **MUST** take the `"|"` member: the `"|"+suffix` convention with the empty suffix the FLAT key itself spells. OPT-free interconversion **MUST** read and write it as that empty suffix in **both** directions, which is what makes the mapping reversible without a template where `|value` would not be (DV_ORDINAL and DV_CODED_TEXT spell a real `|value`). The member appears only where the alternative is a scalar/object collision: a leaf carrying nothing but a bare value **MUST** stay a bare scalar, so a leaf that cannot collide is spelled identically either way. *Not corpus-pinned:* no vendored STRUCTURED body exercises the shape, so `"|"` is this specification's choice within the format's own conventions — the same standing as the DV_SCALE suffix set (§ REQ-140) — and [ADR 0014](../adr/0014-webtemplate-reference-implementation-lock.md) governs if a reference spelling later appears. Registered in the package [deviations register](../../openehr/serialize/simplified/deviations.md).

The identifier-generation algorithm is **normative** in the *Simplified Formats* spec (§Node ID Generation Rules); the SDK **MUST** generate segment identifiers by that algorithm, reusing the Web Template projection (REQ-106, [ADR-0014](../adr/0014-webtemplate-reference-implementation-lock.md)) as the single source of ids.

The codecs **MUST**:

- Be usable independently of the HTTP client and of `auth`/`transport` (REQ-013) — converting a FLAT or STRUCTURED document to or from canonical RM is a valid standalone use case.
- Require the composition's **operational template** (via its Web Template) to resolve identifiers, RM types, level-removal, and `:index` — conversion is template-specific and cannot proceed from the payload alone.
- **Round-trip** FLAT / STRUCTURED ↔ canonical `COMPOSITION` given the OPT: bidirectional and **semantics-preserving** (all archetype- and template-constrained clinical semantics are retained). The simplified forms are *not self-standing* — they depend on the template — which is distinct from being lossy; reconstructing the OPT itself from a data instance is out of scope.
- Interconvert FLAT ↔ STRUCTURED **without** an OPT (the two are mechanical restructurings of one identifier grammar).
- Report a missing or mismatched Web Template / OPT as a typed error when conversion cannot proceed without it.

The codecs **MUST** use the canonical media types `application/openehr.wt.flat+json` (FLAT) and `application/openehr.wt.structured+json` (STRUCTURED); they **SHOULD** accept EHRbase's non-conformant `.schema`-suffixed variants on input for interoperability, but **MUST NOT** emit them. (The `.schema` acceptance is a SHOULD the implementation currently defers — see the package [deviations register](../../openehr/serialize/simplified/deviations.md).)

#### Leaf datatypes

The *Simplified Formats* spec pins the suffix convention but does not spell every leaf datatype's suffix set. Three are stated here because the reference implementation's spelling ([ADR 0014](../adr/0014-webtemplate-reference-implementation-lock.md), pinned by the PROBE-086 corpus) is the only source for them. Each applies at a **Web Template leaf** — reachable with no `_`-segment in the path — and, unchanged, at every nested **value** position § REQ-140's grammar reaches one (`_thumbnail`, `original_content`, an interval bound); the codec **MUST** support each in both directions:

| Leaf datatype | Suffix set |
|---|---|
| DV_PARSABLE | bare value + `\|formalism`, both RM-mandatory — so a DV_PARSABLE at a leaf is always fully captured and **MUST NOT** ride `\|raw`. Its `_charset` / `_language` members are § REQ-140 rows |
| DV_MULTIMEDIA | the bare key is the **`uri`** (a plain DV_URI); `\|mediatype` and `\|size` are RM-mandatory; `\|data` and `\|integrity_check` carry the base64 of the RM's `Byte[]`; `\|alternatetext`, `\|integrity_check_algorithm`, `\|compression_algorithm` optional. Note the reference's spellings: `\|mediatype` and `\|alternatetext` carry no underscore where the RM attribute does. Its `_charset` / `_language` / `_thumbnail` members are § REQ-140 rows, and the bare-code rule for its three CODE_PHRASE-valued attributes is § REQ-140's *DV_MULTIMEDIA coded attributes* behavioural rule |
| DV_INTERVAL&lt;T&gt; | not a suffix set at all: the § REQ-140 **interval grammar** (`_normal_range`'s row and the *Interval boundary flags* rule govern it in both positions), anchored on the bound datatype the Web Template names inside the angle brackets (`DV_INTERVAL<DV_QUANTITY>`, …) — the only place the anchor can come from. `/lower` and `/upper` are not Web Template children, so the leaf's own keys reach no node and the codec **MUST** resolve them against the leaf itself |

### REQ-140 — Underscore-prefixed RM attributes

The FLAT / STRUCTURED codecs (REQ-053) **MUST** carry the *Simplified Formats* **underscore-prefixed RM attribute** grammar — the spec's *RM Attributes prefix* rule: an optional RM attribute the template does not constrain is addressed by prefixing its RM attribute name with `_` at the node it belongs to. The spec names the mechanism and the attribute set; the concrete suffix vocabulary below is the reference implementation's ([ADR 0014](../adr/0014-webtemplate-reference-implementation-lock.md)), pinned by the PROBE-086 corpus.

The grammar is **recursive and typed by the owning RM class**, not a flat key list: a `_`-attribute's value decomposes by its own RM type, and `_`-attributes nest (`…/_feeder_audit/originating_system_audit/provider/_identifier:0|id`, `…/dv_multimedia/_thumbnail|size`). The codec **MUST** support, in **both** directions — encode emitting every populated in-scope attribute from the RM instance, decode rebuilding canonical RM:

| Owner | Attributes | Value grammar |
|---|---|---|
| any LOCATABLE the Web Template **models** (composition root, ENTRY, SECTION, CLUSTER, collapsed `ELEMENT` leaf) — a folded structural wrapper has no FLAT key and therefore no channel, and its attributes are dropped on encode — one of the two out-of-scope classes named under the encode obligations below | `_uid` | bare value (UID-based id) |
| | `_link:N` | LINK — `\|meaning`, `\|type`, `\|target` |
| | `_feeder_audit` | FEEDER_AUDIT (below) |
| collapsed `ELEMENT` leaf | `_null_flavour` | DV_CODED_TEXT — `\|code` + `\|value`, with `\|terminology` optional and carried as written (the corpus spells `openehr` explicitly rather than implying it); legal beside an **absent** bare value |
| | `_null_reason` | DV_TEXT — bare value (its coded subtype rides § REQ-053's `\|code` substitution, as at any DV_TEXT position) |
| ENTRY subtypes | `_work_flow_id`, `_guideline_id` | OBJECT_REF — `\|id`, `\|id_scheme`, `\|namespace`, `\|type` |
| | `_other_participation:N` | PARTICIPATION (below) |
| | `_provider` | PARTY_PROXY (below) — the party grammar, with PARTY_SELF spelled by `\|_type` |
| EVENT_CONTEXT (under the real `context` segment) | `_health_care_facility` | PARTY_IDENTIFIED (below) |
| | `_participation:N` | PARTICIPATION (below) |
| | `_end_time` | bare value (DV_DATE_TIME) |
| | `_location` | bare value (String) |
| DV_ORDERED leaf | `_normal_range` | DV_INTERVAL — `/lower` + `/upper` carrying the anchor datatype's own suffix form, `\|lower_included`, `\|upper_included`, `\|lower_unbounded`, `\|upper_unbounded` |
| | `_other_reference_ranges:N` | REFERENCE_RANGE — the interval grammar (the RM's intervening `range` level **elided**) + `/meaning` (DV_TEXT / DV_CODED_TEXT) |
| DV_TEXT / DV_CODED_TEXT leaf | `_mapping:N` | TERM_MAPPING — `\|match`, `/target\|code` + `\|terminology`, `/purpose` (DV_CODED_TEXT) |
| | `_language`, `_encoding` | CODE_PHRASE — `\|code` + `\|terminology` + `\|preferred_term` |
| DV_DATE / DV_DATE_TIME / DV_TIME leaf | `_accuracy` | bare value (DV_DURATION) — DV_TEMPORAL redefines `accuracy` as a DV_DURATION **object** where DV_AMOUNT declares a Real, so it has no scalar `\|accuracy` suffix at these three types and only these three |
| DV_MULTIMEDIA / DV_PARSABLE value | `_charset`, `_language` | CODE_PHRASE — `\|code` + `\|terminology` + `\|preferred_term` |
| DV_MULTIMEDIA value | `_thumbnail` | nested DV_MULTIMEDIA (its own suffix set) |
| PARTY_IDENTIFIED / PARTY_RELATED (wherever the grammar reaches one) | — | `\|id`, `\|id_scheme`, `\|id_namespace`, `\|name`; nested `_identifier:N` (DV_IDENTIFIER — `\|id`, `\|issuer`, `\|assigner`, `\|type`); PARTY_RELATED adds `/relationship` (DV_CODED_TEXT). PARTY_SELF is spelled by the **absence** of every party key at an RM-**mandatory** PARTY_PROXY position, and by `\|_type: "PARTY_SELF"` at an RM-**optional** one (see the note below). `\|id` + `\|id_scheme` decompose the `external_ref`'s OBJECT_ID under the same discriminator rule as the OBJECT_REF families above (scheme present ⇒ GENERIC_ID), `\|id_namespace` its namespace; the reference writes no `\|type` and hardcodes `PARTY`, so absent **MUST** decode as `PARTY` and `\|type` **MUST** be emitted only where the reference's own value would be lost |
| PARTICIPATION | — | `\|function` (a **plain** DV_TEXT — the key has no coded channel), `\|mode` (the openEHR `participation mode` **rubric alone**, no code — see the note below), the performer's party suffixes inline, and the performer's identifiers as **inlined indexed suffixes** `\|identifiers_id:N` / `\|identifiers_issuer:N` / `\|identifiers_assigner:N` / `\|identifiers_type:N` (the reference's spelling — **not** the nested `_identifier:N` form standalone party nodes use). PARTICIPATION `time` has no channel and is **deferred** as a typed refusal |
| FEEDER_AUDIT | `originating_system_item_id:N`, `feeder_system_item_id:N` | DV_IDENTIFIER suffixes (the FLAT segment is **singular** where the RM attribute is plural) |
| | `originating_system_audit` (RM-mandatory), `feeder_system_audit` | FEEDER_AUDIT_DETAILS — `\|system_id` (RM-mandatory), `\|version_id`, `\|time` (bare DV_DATE_TIME), all three own suffixes; `/location` and `/provider` as PARTY_IDENTIFIED, `/subject` as PARTY_PROXY; `other_details` (ITEM_STRUCTURE) **deferred** as a typed refusal |
| | `original_content` **or** `original_content_multimedia` | DV_PARSABLE or DV_MULTIMEDIA per [§ REQ-053 *Leaf datatypes*](#leaf-datatypes) — the DV_ENCAPSULATED choice disambiguated **by key name** (reference spelling); both spellings present is a typed error |

DV_PARSABLE, DV_MULTIMEDIA and `DV_INTERVAL<T>` are **not** `_`-attributes — each is a leaf datatype reachable with no `_`-segment at all — so their suffix sets are specified with the other leaf grammars in [§ REQ-053 *Leaf datatypes*](#leaf-datatypes). The value positions above (`original_content`, `_thumbnail`, an interval bound) apply those leaf rules verbatim, per the `|raw`-carrier rule below.

Behavioural rules:

- The round-trip **MUST** be semantics-preserving in both directions, under REQ-053's fail-loud posture: an unrecognised `_`-segment **MUST** be a typed error (`ErrUnknownPath`), an unrecognised suffix inside a family a typed error — never a silent drop.
- Encode **MUST NOT** silently lose an in-scope attribute: a populated attribute is emitted in this grammar; what the grammar cannot carry either rides `|raw` (where a `|raw` carrier exists) or is a typed error. This **narrows the `|raw` boundary**: a decorated value whose extras are now expressible (a `normal_range`, `mappings`, …) rides suffixes plus `_`-attribute keys where it previously rode a whole `|raw` fragment. Two attribute classes are deliberately **out of scope**, and they are the only two. The first is the composition `composer`'s `external_ref` and `identifiers`: [ADR 0015](../adr/0015-flat-metadata-spelling.md) projects the composer onto the `ctx/` short forms, which carry the name alone, and decode refuses the real-path `composer/…` spellings that would carry the rest — so encode keeps the name and drops those two rather than refusing every composition whose composer is properly referenced (one vendored corpus body is). Registered in the package [deviations register](../../openehr/serialize/simplified/deviations.md); closing it needs a `ctx/` or real-path channel, not a codec change. The second is the `_uid` / `_link:N` / `_feeder_audit` of a **folded structural wrapper** — an `ITEM_TREE`, `ITEM_LIST`, `ITEM_TABLE`, `ITEM_SINGLE`, `HISTORY` or lifted `max: 1` `EVENT`, none of which the Web Template models as a node: FLAT has no key for one, so encode drops those attributes with no error and decode can never read them back. Unlike the composer boundary this one cannot be closed codec-side at all — it needs a FLAT spelling for a folded wrapper, a reference-side change ([ADR 0014](../adr/0014-webtemplate-reference-implementation-lock.md)). PROBE-086 is structurally blind to it, since its input is FLAT. Registered in the same [deviations register](../../openehr/serialize/simplified/deviations.md) § folded structural wrappers.
- STRUCTURED **MUST** carry the same vocabulary as nested members (`_`-keys as object members, arrays for `:N` lists), and OPT-free FLAT ↔ STRUCTURED interconversion **MUST** preserve it.
- **DV_SCALE** takes the `DV_ORDINAL` suffix set — `|code` + `|value` + `|ordinal`, with `|ordinal` carrying DV_SCALE's **Real** `value`. The two classes differ only in that value's type, so decode **MUST** accept and encode **MUST** emit the same three keys at a `DV_SCALE` leaf as at a `DV_ORDINAL` one. *Not corpus-pinned:* no vendored body and no reference sample spells a `DV_SCALE`, so this is this codec's choice within the format's conventions — the same standing as the STRUCTURED `"|"` member — and [ADR 0014](../adr/0014-webtemplate-reference-implementation-lock.md) governs if a reference spelling later appears. Registered in the package [deviations register](../../openehr/serialize/simplified/deviations.md).
- **Interval boundary flags.** The reference writes the four DV_INTERVAL Booleans only where they carry information the default does not, and the codec **MUST** read and write them under that same asymmetry: `|lower_unbounded` / `|upper_unbounded` are RM-mandatory Booleans whose `false` the reference omits, so absent **MUST** decode as `false` and only `true` is emitted; `|lower_included` / `|upper_included` are RM-**optional** (`Interval` declares them `0..1`), so absent **MUST** decode as the closed endpoint (`true`) and only `false` is emitted. An absent bound is the unbounded end and **MUST NOT** be emitted as a zero-valued one, and the converse pair is a typed error in **both** directions: a bound standing beside its own `|*_unbounded: true` contradicts the RM equivalence `lower_unbounded = (lower = Void)`, so decode **MUST** refuse it and encode **MUST NOT** drop it silently. What counts as a bound differs with the instantiation, and encode **MUST** apply both rules: an **interface**-typed bound (`DV_INTERVAL<DV_ORDERED>`) can spell Void as `Void`, so *any* value standing beside the flag contradicts it — including one whose fields are all zero; a **concrete**-typed bound cannot be Void at all, so absence has exactly one spelling, the type's zero, and only a bound differing from it can be reported. The residue is a stated limit, not a gap: for a concrete bound a zero that the producer meant (a `DV_COUNT` lower bound of 0) is indistinguishable from one it never set, so that pair is emitted as the unbounded end rather than refused — refusing it would refuse every half-open concrete range. A redundant `|*_included: true` is therefore normalised away on re-encode — it denotes the same RM value as its absence. *(Corpus-derived, 2026-08-05: `ehrbase_conformance_data_types_dv_count` omits the flags on an interval whose two bounds are present while `…_dv_quantity` spells both `false`, and `…_dv_ordinal`'s unbounded end pairs `|upper_unbounded: true` with `|upper_included: false` — this is the only mapping under which all three round-trip byte-exactly.)*
- **`|mode` carries no code.** PARTICIPATION.mode is a DV_CODED_TEXT whose `defining_code` is RM-mandatory, but the reference writes only the openEHR Terminology **`participation mode` rubric** (`"face-to-face communication"`, `"not specified"`). The codec **MUST** therefore rebuild the code from that closed, specification-versioned group (a small vendored code↔rubric table — the only vocabulary this codec carries; recorded in the package [deviations register](../../openehr/serialize/simplified/deviations.md)), **MUST** refuse a rubric outside the group rather than fabricate a code, and on encode **MUST** refuse a mode whose code, terminology or value that rebuild would not reproduce. *(Corpus-derived, 2026-08-05: verified against `ehrbase_conformance_composition` / `…_observation` and the `clinical_content_validation` canonical fixture, whose `217` / `"signing (face-to-face)"` pair matches the group exactly.)*
- **PARTY_SELF at an RM-optional PARTY_PROXY position carries `|_type`.** PARTY_SELF is spelled by the absence of every party key wherever that absence is unambiguous — a PARTICIPATION performer and an ENTRY `subject`, both RM-mandatory, so no-keys can only mean PARTY_SELF. At an RM-**optional** PARTY_PROXY the absence already means *absent*, so the reference writes the discriminator explicitly: the codec **MUST** accept and emit `|_type: "PARTY_SELF"` at FEEDER_AUDIT_DETAILS' `subject` and at ENTRY's `_provider`, and **MUST** refuse any other `|_type` value or a `|_type` standing beside party keys that contradict it — the suffix carries one subtype and reaches no other position. *(Corpus-derived, 2026-08-05: `ehrbase_conformance_party_self` writes `…/_feeder_audit/originating_system_audit/subject|_type: "PARTY_SELF"` and no other key there.)*
- **The DV_MULTIMEDIA coded attributes travel as a bare code.** `|mediatype` carries `media_type.code_string` alone, so the terminology is implied: it **MUST** be read as the openEHR `media types` code set (`IANA_media-types`, the identifier the RM names and REQ-107's template-instance writer already pins), and a `media_type` coded in any other terminology, or carrying a `preferred_term`, **MUST** ride `|raw` whole rather than be silently re-terminologised — the `|normal_status` rule one attribute up. An absent terminology contradicts nothing the implication asserts and survives the suffix form. `|integrity_check_algorithm` and `|compression_algorithm` carry a bare code with **no** implied terminology this specification can source, so they **MUST** decode to a code-only CODE_PHRASE and a value carrying a terminology **MUST** ride `|raw`. *(Corpus-derived, 2026-08-05: the FLAT `dv_multimedia` fixture writes `video/H261`, `SHA-256` and `zlib` as bare codes; the vendored canonical `Test_dv_multimedia_open_constraint.v0` and `Demonstration.v1` compositions carry `IANA_media-types` against that spelling.)*
- **The party `external_ref` `type`.** The reference writes no `|type` at a party and hardcodes `PARTY`, which is lossy for the `PERSON` and `ORGANISATION` references real compositions carry. The codec **MUST** default an absent `|type` to `PARTY` — so a reference-authored body round-trips byte-exactly and never gains the key — and **MUST** emit `|type` when, and only when, the value differs. That suffix is the reference's own spelling of the same RM attribute in the OBJECT_REF families, applied at one further position; an empty `type` is a typed error, not a defaulted one.
- **The grammar stops where the reference stops, and the `|raw` carrier takes the rest.** A nested value the grammar reaches through a *datatype* position — an interval bound, a REFERENCE_RANGE `/meaning`, a `_charset`, a `_thumbnail`, an `original_content` — is read and written by that datatype's own leaf rules, so one whose extras the suffix set cannot carry rides `|raw` **at its own key** rather than being refused: a thumbnail carrying its own charset is `_thumbnail|raw`, an `original_content` carrying one is `original_content|raw`. The families that reach a value through a *non-datatype* position instead (a party, a PARTICIPATION, a LINK, a FEEDER_AUDIT_DETAILS scalar) have no such carrier, and there every shape the suffixes cannot express **MUST** be a typed error.
- Deliberate exclusions, **stated by direction**. Refused on decode and visible in the PROBE-086 census — never silently dropped: the composer's `external_ref` suffixes and the composer's whole party sub-structure — `composer/_identifier:N` and, for a PARTY_RELATED composer, `composer/relationship` (no `ctx/` carrier — [ADR 0015](../adr/0015-flat-metadata-spelling.md) boundary). On **encode** exactly one of those shapes is *projected* rather than refused: a PARTY_IDENTIFIED composer **carrying a name** keeps the name and drops its `external_ref` and `identifiers`, per the carve-out above. Every other shape the `ctx/` forms cannot carry is a typed error on encode too — a PARTY_RELATED composer (what `composer/relationship` spells), a PARTY_SELF carrying an `external_ref`, and a PARTY_IDENTIFIED with no name. The rest are refusals in both directions: PARTICIPATION `time`; FEEDER_AUDIT_DETAILS `other_details`; `_instruction_details` (ACTION) and `_wf_definition` (INSTRUCTION) — named by the spec, unexercised by the pinned corpus, **deferred** as typed refusals until a pinnable fixture lands.

Verified by [PROBE-089](conformance.md#probe-089--underscore-attribute-round-trip) (per-family round-trip) and the [PROBE-086](conformance.md#probe-086--upstream-flat-serialisation-parity) census movement.

## ITS-REST envelopes

The openEHR REST 1.1.0-development specification defines envelope shapes for typical responses (collections, errors, version metadata). The SDK **MUST**:

- Decode well-formed envelopes into typed Go structs.
- Surface envelope-level metadata (e.g. paging hints, conformance hints) on the typed response, not as a parallel `map[string]any`.
- Reject malformed envelopes with a typed `WireError` carrying the parse failure.

The exact envelope shapes are openEHR REST 1.1.0-development; this spec does not re-define them, it pins to them.

### Request vs response shape asymmetry

Two endpoints carry **distinct request and response shapes** that are easy to conflate because the RM ships only the persisted (response) form:

- **`POST /ehr/{ehr_id}/composition`** and **`PUT /ehr/{ehr_id}/composition/{vo_uid}`** ([PROBE-071](conformance.md#probe-071--composition-postput-response-body-is-bare-composition)). Request body: a bare `COMPOSITION` payload. Response body under `Prefer: return=representation`: a bare `COMPOSITION` per ITS-REST `201_COMPOSITION` / `200_COMPOSITION_updated`, **not** the persisted `ORIGINAL_VERSION<COMPOSITION>` envelope. The persisted envelope is reached via `GET /versioned_composition/{vo_uid}/version/{version_uid}` (`UVersionOfComposition`). Same shape applies to `directory.Save` / `Update`.
- **`POST /ehr/{ehr_id}/contribution`** ([PROBE-072](conformance.md#probe-072--contribution-submission-body-matches-contribution_create)). Request body: ITS-REST `Contribution_create` — `{audit, versions: [ORIGINAL_VERSION<T> with inline data: T]}` for `T ∈ {COMPOSITION, EHR_STATUS, FOLDER, EHR_ACCESS}`. Response body: persisted `CONTRIBUTION` whose `versions[]` is `[]OBJECT_REF` (the references the server assigned). A submission body shaped like the persisted `CONTRIBUTION` is rejected by spec-conformant CDRs because its `OBJECT_REF`s point at versions that do not yet exist.
  - **Commit-audit DTO asymmetry (SPECITS-95 / [ITS-REST PR 131](https://github.com/openEHR/specifications-ITS-REST/pull/131)).** The request-side commit audit (the batch `audit` and each version's `commit_audit`) is the `UPDATE_AUDIT` DTO, **not** the persisted `AUDIT_DETAILS`: it MUST omit the server-assigned `time_committed`, treats `system_id` as optional, and types `change_type` (and `UpdateVersion.lifecycle_state`) as `DV_CODED_TEXT` — never the withdrawn flat `TERMINOLOGY_CODE`. A client SHOULD send `_type:"UPDATE_AUDIT"`; servers SHOULD accept `AUDIT_DETAILS` or an omitted `_type`. The Go SDK emits `AUDIT_DETAILS` by default (`contribution.UpdateAudit`) and exposes `AuditType` to fall back to `UPDATE_AUDIT` for non-conformant servers.

Implementations **MUST NOT** serialise the persisted shape on either submission path. The Go SDK enforces this via [`contribution.Submission`](../../openehr/client/ehr/contribution/submission.go) (distinct from `rm.Contribution`) and the composition / directory write surfaces that take bare RM types. Which version fields a *request* body carries — and which server-assigned ones it omits — is specified by [REQ-130 § Server-assigned fields](#req-130--contribution-builder).

## AQL

### REQ-055 — Wire boundary

`openehr/aql` ships two builder styles:

- **Struct-builder.** Build an `aql.Query` value by composing typed structs.
- **Verb-functions.** Compose a query via top-level functions (`aql.Select(...)`, `aql.From(...)`, `aql.Where(...)`).

Both styles **MUST** produce the **same AQL string on the wire** for the same logical query. Concrete rules:

- The serialised AQL string is the wire contract. Two queries that produce the same string are equivalent; two that produce different strings are different queries, even if logically equivalent.
- Whitespace, casing, and aliasing are subject to canonicalisation in the wire-output path — the SDK **MUST** produce a stable, canonicalised string so wire-output goldens are deterministic.
- Both styles are tested against the same wire-output golden cassettes; a builder change that produces different output is a breaking change.

**Canonicalisation (wire output).** Built queries emit a single stable form so the goldens are deterministic. Changing any rule below rewrites every golden and is a **semver-major** change to `openehr/aql`:

1. **Keywords** uppercase — `SELECT`, `FROM`, `WHERE`, `CONTAINS`, `AND`, `OR`, `ORDER BY`, `ASC`, `DESC`. (`OFFSET` / `LIMIT` are not emitted in the string — see rule 7.)
2. **Whitespace** — exactly one space between tokens; no leading or trailing space on the emitted string.
3. **Paths and archetype ids** — emitted verbatim; no case folding.
4. **Parameters** — caller values appear only as `$name` placeholders in the string, never interpolated as literals (the injection guard, below); placeholder keys carry no leading `$`. A bind whose name is empty after that strip **MUST** fail at `Build()` with an error wrapping `aql.ErrInvalidQuery` — no placeholder can match an empty name, so the value would ship only as a dead key.
5. **SELECT list** — comma-separated, one space after each comma, no trailing comma.
6. **FROM / CONTAINS** — every class carries an alias (`EHR e`, `OBSERVATION o`); archetype predicates attach in square brackets (`OBSERVATION o[openEHR-EHR-OBSERVATION.body_temperature.v2]`); consecutive `CONTAINS` express nested containment. The builders emit EHR scoping as a `WHERE <alias>/ehr_id/value = $param` condition so it composes with other conditions in one clause; the standing-predicate form (`EHR e[ehr_id/value=$param]`) is equally valid AQL but is not what the builders emit **at the FROM root** — since [clinical-modeling.md § REQ-163](clinical-modeling.md#req-163--aql-write-side-expressivity-parity) a `CONTAINS` operand does carry a standing predicate, through a carrier of its own; the root's choice above is the one that stands.
7. **Clause order** — the AQL string emits `SELECT … FROM … WHERE … ORDER BY`; the builder is the sole author of `Query.Q` for built queries. Paging (OFFSET / LIMIT) is carried in the request envelope (`Query.Offset` / `Query.Fetch`), not the string, so paging has a single channel.

Additively since REQ-117, the canonical forms for negated containment (`NOT CONTAINS`), sibling containment junctions (`AND` / `OR`, parenthesised only where precedence requires), and the **opt-in in-text `LIMIT` / `OFFSET`** — which relaxes rules 1 and 7 only for callers who explicitly request it, the envelope staying the default and combining the two channels being a build-time error — are specified by [clinical-modeling.md § REQ-117](clinical-modeling.md#req-117--aql-expression-catalogue-completion) and asserted by PROBE-088. The write-side constructs [clinical-modeling.md § REQ-163](clinical-modeling.md#req-163--aql-write-side-expressivity-parity) adds — the version predicate, the class standing predicate, and the typed projection (`DISTINCT`, `AS`, aggregates) — carry their canonical spellings in that section for the same reason: they are additions that rewrite no rule and no golden here.

The rules above are stated over clause structure and reach no further than the **shapes** a clause is built from. The canonical spelling of each VALUE inside them — string escaping, a real's fractional part, the `MATCHES {uri}` operand alphabet — is specified by [clinical-modeling.md § REQ-119 § Canonical value spellings](clinical-modeling.md#req-119--re-parseable-canonical-aql-emission) and asserted by PROBE-090. **REQ-119 changes two spellings** relative to v0.19.0: a literal carrying `'` or `\`, and a whole-valued real (`2` → `2.0`). The string's prior form was not re-parseable by this SDK's own parser; the real's prior form re-parsed as a *different* value (`2` came back an integer), already failing round-trip identity — on that basis the change is classified semver-**minor** by exception to the rule above, but it does rewrite a golden that pins such a literal.

The reference golden lives at [`openehr/aql/testdata/wire/`](../../openehr/aql/testdata/wire/) and is asserted by PROBE-020.

### AQL executor

`openehr/client/query` is the AQL executor. It:

- Accepts a built `aql.Query` (or a raw AQL string for advanced use).
- **MUST** send it to the route the operation names: `GET|POST /query/aql` for ad-hoc execution, `GET|POST /query/{qualified_query_name}` for a stored query, and `GET|POST /query/{qualified_query_name}/{version}` when pinned to a version — and **MUST NOT** synthesise a `/query/aql/{…}` route. The `aql` path segment belongs to the ad-hoc route alone: the vendored OAS declares exactly those three paths and no `/query/aql/{…}` route.
- **MUST** decode the response envelope's `meta`, `columns`, and `rows`. Row values are typed via generics where the caller pre-declares column types; otherwise they decode to `any` and the call site casts.
- Surfaces AQL-level errors as typed errors distinct from a generic `WireError`. The taxonomy is fixed by the HTTP status and the openEHR error envelope; each arm below states which of the two carries its signal:
  - **400** (bad AQL) and **408** (query timeout) **MUST** surface as `*query.AQLError`. One class covers bad AQL in every form: a syntax error, a semantically impossible containment, and a malformed path expression alike are `400`.
  - An envelope denoting path resolution — never one carried by a `501`, see below — **MUST** additionally satisfy `errors.Is(err, aql.ErrPathResolution)` when the signal is in the envelope's PHI-free **code**: a sub-kind of bad AQL. The classifier also reads anchored message clauses, but only where the deployment surfaces raw error bodies — the message is suppressed by default because it may carry PHI — so an envelope signalling path resolution in the message alone classifies only under that opt-in (PROBE-021).
  - **501** **MUST** surface as `*query.AQLError` satisfying `errors.Is(err, aql.ErrEngineCapability)`: a capability gap, **not** a client error — the query is valid AQL and this deployment does not implement the feature, so the caller retries elsewhere or degrades rather than correcting the query. Here the status alone classifies, with or without an envelope and **never** from message text, and a `501` **MUST NOT** also be classified as path resolution.

**AQL injection.** `ExecuteString` (raw AQL escape hatch) **MUST** be documented as unsafe for interpolating caller-supplied values into the query text — bind parameters via the typed `params` map (named placeholders the CDR binds server-side). String-built AQL from untrusted input is injectable.

**EHR scoping (verb-aware).** When execution is scoped to a single EHR, the SDK **MUST** apply the scope by the verb-appropriate mechanism the ITS-REST OAS declares. The rule is the **verb**, not the route: **every GET** query operation — ad-hoc (`/query/aql`), stored (`/query/{qualified_query_name}`), and stored-versioned (`/query/{qualified_query_name}/{version}`) — declares the `ehr_id` **query parameter**, and the SDK carries the scope there. **No POST** operation declares it, and neither request body schema (`AdhocQueryExecute` for ad-hoc, `Query` for stored) carries an `ehr_id` field, so the **`openehr-ehr-id` request header** is the only channel for both POST forms. The SDK **MUST NOT** scope POST via the query parameter: a strict-spec server that honours only the header would otherwise run the query population-wide.

### Stored AQL

### REQ-057

The platform supports **stored AQL queries** — queries registered ahead of time and executed by ID. The SDK **MUST** support both ends:

- **`openehr/client/definition/`** — register, list, get, delete stored queries. Operations map to the openEHR REST Definition API.
- **`openehr/client/query/`** — execute a stored query by ID (`GET|POST /query/{qualified_query_name}`) in addition to the ad-hoc execution path.

A stored query is identified by a qualified name (typically reverse-DNS, e.g. `org.example.queries.recent-observations`); the SDK **MUST** pass it through verbatim (the one reserved name below aside). Stored queries are expected to be faster than ad-hoc AQL on the same backend (materialised read models, known output schemas), but the SDK **MUST NOT** pre-validate the qualified name's *syntax* — that's the backend's responsibility.

**Reserved name `aql`.** That no-pre-validation stance is scoped to name *syntax*, which stays the backend's call. One collision is the SDK's own: it builds the stored path by concatenating `/query/` with the name, so the name `aql` addresses `/query/aql` — the ad-hoc route — and the caller would otherwise learn of it only through the server's "missing `q`" `400`. A qualified name that is exactly `aql` after trimming surrounding whitespace **MUST** therefore fail client-side before any request is issued, with an error satisfying `errors.Is(err, ErrInvalidConfig)` whose message names the collision. The comparison is byte-exact, matching the byte-level path routing that causes it: `AQL` and `aql.reports` are ordinary names and pass through verbatim.

**Store-response version recovery.** The Definition store operation (`PUT /definition/query/{qualified_query_name}[/{version}]`) returns the server-assigned `{name, version}` in a **`Location` response header** with an empty body — the canonical `200_StoredQuery_stored` OAS shape, and what a `text/plain` store returns. The SDK **MUST** recover the assigned identifier in order: (1) parse the `Location` header (canonical); (2) decode a JSON body if present (lenient — some deployments return one); (3) fall back to the caller's input `{name, version}` (graceful degradation). A malformed `Location` **MUST NOT** fail the call — it falls through to (2)/(3).

## Optimistic concurrency

### REQ-054

openEHR versioned resources (Composition, EHR_STATUS, Directory, Contribution) are versioned by `version_uid` with `If-Match` / `ETag` optimistic concurrency on the wire.

Rules the SDK **MUST** enforce:

- A PUT against a versioned resource **MUST** include `If-Match: "<preceding_version_uid>"` (the canonical form per the openEHR REST envelope). Omitting it **MUST** result in `428 Precondition Required` from the backend; the SDK **MUST NOT** retry without an `If-Match`.
- A `409 Conflict` response (stale `If-Match`) **MUST** map to `transport.ErrVersionConflict`.
- A `412 Precondition Failed` response **MUST** map to `transport.ErrPreconditionFailed`.
- A `428 Precondition Required` response **MUST** map to `transport.ErrPreconditionRequired`.
- The SDK **MUST NOT** synthesise these statuses client-side — they come from the backend.

ETag handling on reads is symmetric: the SDK **MUST** capture `ETag` from a response and expose it on the typed return value so the caller can use it for the next PUT.

## REST leaf operations

### REQ-142 — Contribution read

The EHR Contribution leaf **MUST** expose a read operation matching ITS-REST `contribution_get`:

`GET /ehr/{ehr_id}/contribution/{contribution_uid}`

The call **MUST** return the persisted `CONTRIBUTION` decoded as the SDK's contribution type. The leaf returns a `*VersionMetadata` for shape consistency with the other EHR leaves, populated from whatever response headers the server sends — the vendored pin defines only `Content-Type` on `200_CONTRIBUTION` (`ETag` / `Location` belong to `201_CONTRIBUTION`), so the SDK **MUST NOT** require populated version metadata on a contribution read. Empty `ehr_id` or `contribution_uid` **MUST** fail before any request is issued — an empty interpolated segment is exactly what [REQ-150](transport.md#req-150--path-parameter-segment-validation)'s validator refuses (`errors.Is(err, ErrInvalidConfig)` holds; the sentinel contract has that one home). A `404` **MUST** map to `ErrNotFound`. Path parameters are ordinary decoded strings; [REQ-150](transport.md#req-150--path-parameter-segment-validation) applies.

v1 of this leaf **MUST** request canonical JSON. Simplified-format `Accept` values (FLAT / STRUCTURED inner payloads) are out of scope — no other EHR Get leaf takes a format yet.

The leaf's repository interface **MUST** include the same read — no break for callers of the package functions, a compile-time break for interface implementers (precedent: `UploadTemplate`); the CHANGELOG `### Added` entry **MUST** name the interface growth.

### REQ-143 — Template list filters

`ListTemplates` **MUST** accept the ITS-REST list filters of `definition_template_adl1.4_list` in the vendored OpenAPI pin (the ADL 2 list operation references the same five parameter components, so the option set carries over unchanged when ADL 2 support lands):

| Query parameter | Option | Notes |
|---|---|---|
| `template_id` | `WithTemplateID` | Wildcard pattern as specified upstream |
| `concept` | `WithConcept` | Wildcard pattern as specified upstream |
| `version` | `WithVersion` | When omitted, the server returns only the latest version |
| `offset` | `WithOffset` | 0-based; an explicit `0` **MUST** be sent |
| `fetch` | `WithFetch` | An explicit `0` **MUST** be sent |

Unset options **MUST** omit the corresponding query key. A negative `offset` or `fetch` **MUST** fail with `ErrInvalidConfig` and **MUST NOT** issue a request. The existing `format` argument selects the list path; v1 supports `FormatADL14` — the only registered `TemplateFormat` value. The decoded result **MUST** remain the same template-metadata slice the unfiltered list already returns. Adding a trailing variadic option list **MUST** stay source-compatible with existing callers. The `Repository` interface **MUST** grow the same variadic options — no break for callers, a compile-time break for interface implementers (precedent: `UploadTemplate`); the CHANGELOG `### Added` entry **MUST** name the interface growth.

## Write-side authoring

### REQ-130 — Contribution builder

The contribution leaf **MUST** expose a builder that assembles a `Contribution_create` body — a [`contribution.Submission`](../../openehr/client/ehr/contribution/submission.go) — from caller payloads without hand-wiring version wrappers, change-type codes, or write-side audit fields. It is the first allocation in the **SDK authoring & client tooling** band (130–139) and is named as SDK-provided by [use-cases.md § Synthetic data seeder](use-cases.md#synthetic-data-seeder).

The builder is an authoring surface over the landed submission shape (REQ-050/095, [PROBE-072](conformance.md#probe-072--contribution-submission-body-matches-contribution_create)) and introduces no new wire shape: anything it emits, a caller **MUST** be able to hand-wire. Where the two could disagree, the builder **MUST** defer to the submission shape.

**Change types.** A version's `commit_audit.change_type` **MUST** carry the openEHR *audit change type* coded value for the operation the caller requested, `DV_CODED_TEXT`-shaped (nested `defining_code`, never the withdrawn flat `TERMINOLOGY_CODE`) with `terminology_id.value = "openehr"`:

| Operation | `value` | `code_string` |
|---|---|---|
| Creation | `creation` | `249` |
| Amendment | `amendment` | `250` |
| Modification | `modification` | `251` |
| Deletion | `deleted` | `523` |

This table is the code set's single home in these specs — `523` is the deletion code; `253` is *unknown*, not *deleted*. The batch-level `audit.change_type` describes the contribution as a whole and **MUST** be caller-supplied: the builder **MUST NOT** derive it from the versions it holds. The vendored corpus ([`testkit/cassettes/submissions/`](../../testkit/cassettes/README.md)) settles that — a batch audit there records `creation` over an all-`modification` version list, and `modification` over an all-`creation` one, so no derivation rule is faithful to it.

**Preceding version.** An amendment, modification, or deletion **MUST** carry `preceding_version_uid`; a creation **MUST NOT** — the version it would follow does not exist yet. The builder **MUST** refuse a preceding-version-bearing operation whose uid is empty rather than emitting a version the server cannot resolve.

**Lifecycle state.** Each built version **MUST** carry its `lifecycle_state` in the **version body**, and a builder-assembled batch **MUST NOT** convey that state as the `openehr-version` request header — not as a substitute for the body value and not alongside it, since one header value cannot describe several versions. Informative rationale: `lifecycle_state` is a required member of the pin's `UpdateVersion` DTO, and the header ([REQ-059](#req-059)) is per-*request*, so it cannot express a distinct state for each version of a batch that commits several. The emitted state **MUST** be a `DV_CODED_TEXT` from the openEHR *version lifecycle state* group — `complete` (`532`), `incomplete` (`553`), `deleted` (`523`) — defaulting to `complete` and overridable per version. The default **MUST NOT** be derived from the change type: in the vendored corpus most deletions carry a `complete` lifecycle state, so a derived `deleted` would contradict the wire it claims to follow.

**Audit.** The batch `audit` and every version's `commit_audit` are the write-side commit-audit DTO — `time_committed` is never emitted (§ [Request vs response shape asymmetry](#request-vs-response-shape-asymmetry)). The pin marks `change_type` and `committer` **required** on both. The builder **MUST** refuse at build time when either is missing, rather than emitting an audit the pin rejects, and **MUST** apply the batch committer, system id, and audit `_type` to every version whose own audit does not override them — one declaration, not one per version.

**Server-assigned fields.** The pin's `UpdateVersion` declares neither `contribution` nor `uid`; the server assigns both at commit. A write-side version body **MUST** omit each of them when the caller supplied neither, rather than emitting `"contribution":null` or an empty `uid` object — a field the request schema does not declare, sent with no value, is a body a strict server may refuse and no record in the vendored corpus carries. A caller-supplied `uid`, by contrast, **MUST** be emitted verbatim — the omission rule reaches only the value the caller never set. This clause is **implementation-aligned**: it amends what the landed write-side wrappers emit, and lands with the code that satisfies it.

**Builder contract.**

- `Build` **MUST** return a `*Submission` that passes `Validate`, or a typed error and no submission — never a partially populated one. It **MUST NOT** panic on any caller input (REQ-025).
- `Build` **MUST** be idempotent, and the submission it returns **MUST** be independent of the builder: mutating the builder afterwards **MUST NOT** change an already-built submission.
- An accumulation error (an unsupported operation, a missing preceding uid) **MUST** be reported at `Build`, not swallowed and not panicked at the call that made it — a fluent chain has nowhere to return an error.
- The closed versionable type-set — `COMPOSITION`, `EHR_STATUS`, `FOLDER`, `EHR_ACCESS`, the four the submission shape admits — **MUST** be enforced at compile time by explicit generic instantiation, with no reflection (REQ-024).
- A builder holding no versions **MUST** fail at `Build`: the pin requires a non-empty `versions` array.

**Out of scope for v1:** `IMPORTED_VERSION` authoring beyond the landed pass-through wrapper, multi-EHR batching, and checkpoint/resume — the seeder's responsibility per [use-cases.md](use-cases.md#synthetic-data-seeder).

## Transport cross-cutting concerns

REQ-090 (OpenTelemetry), REQ-091 (retry), REQ-092 (TLS posture), REQ-093 (error envelope), REQ-094 (`Prefer`), and REQ-150 (path-parameter segment validation) are specified in [transport.md](transport.md).

## Streaming and large payloads

Out of v1 scope:

- Streaming response bodies. v1 reads complete responses into memory. Streaming **MAY** be added when a documented consumer needs it.
- Multipart uploads beyond what openEHR REST 1.1.0-development requires.
- Range requests, partial reads.

## Coverage matrix

| Topic | REQ | Lives in |
|---|---|---|
| REST 1.1.0-dev pin | REQ-050 | `transport/`, `openehr/client/*` |
| Cadasto spec-version header | REQ-051 | `transport/` |
| Canonical JSON | REQ-052 | `openehr/serialize/canjson/` |
| Canonical XML | REQ-056 | `openehr/serialize/canxml/` |
| FLAT / STRUCTURED | REQ-053 | `openehr/serialize/simplified/` |
| Optimistic concurrency | REQ-054 | `transport/` (error mapping), `openehr/client/*` (header plumbing) |
| AQL wire | REQ-055 | `openehr/aql/`, `openehr/client/query/` |
| Stored AQL | REQ-057 | `openehr/client/definition/`, `openehr/client/query/` |
| openEHR custom header family | REQ-059 | `transport/` (option API), `openehr/client/*` (typed per-method options) |
| OpenAPI authoritative source | REQ-095 | `testkit/cassettes/its_rest/` (records upstream commit) |
| Path-parameter segment validation | REQ-150 | [transport.md](transport.md) → `transport/` |
| Contribution builder | REQ-130 | `openehr/client/ehr/contribution/` |
| Contribution read | REQ-142 | `openehr/client/ehr/contribution/` |
| Template list filters | REQ-143 | `openehr/client/definition/` |
| Shared RM / OPT fixtures | REQ-052, REQ-056 | `testkit/cassettes/{templates,compositions,rm}/` — resolve via `testkit/fixtures/`; index in [`testkit/cassettes/README.md`](../../testkit/cassettes/README.md). Bodies, not REQ-082 Cassette-mode recordings ([conformance.md § Vendored fixtures](conformance.md#vendored-fixtures-testkitcassettes)) |
| Transport (OTel, retry, TLS, errors, Prefer) | REQ-090–094 | [transport.md](transport.md) → `transport/`, `smart/discovery/` |
