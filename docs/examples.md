# Examples

**New here?** Run `go run ./cmd/examples/canonical_json` and then follow the [suggested learning order](#suggested-learning-order). Every example works offline — the REST ones use an in-process `httptest` backend, so nothing needs a CDR.

The 17 runnable programs under [`cmd/examples/`](../cmd/examples/) demonstrate each major SDK surface. They are **reference shapes** — production tools (benchmark harnesses, MCP servers, federators) live in their own repositories but follow the same patterns. Each entry below ends with a **What to copy into your app** note, which is the part worth reading if you are here to build something.

Fixture paths resolve relative to the source file, so `go run ./cmd/examples/<name>` works from **any working directory** inside a clone. Build them all with `make build` (or `go build ./cmd/examples/...`).

---

## At a glance

| Example | Network | Packages | Demonstrates |
|---|---|---|---|
| [canonical_json](#canonical_json) | No | `rm`, `canjson` | Decode canonical JSON → typed `Composition` |
| [canxml_roundtrip](#canxml_roundtrip) | No | `canjson`, `canxml` | JSON ↔ XML cross-format invariant |
| [opt-parse](#opt-parse) | No | `template` | Parse ADL 1.4 OPT, walk paths |
| [primitive-validate](#primitive-validate) | No | `template`, `constraints` | Primitive constraint validation (REQ-103) |
| [validate-composition](#validate-composition) | No | `template`, `validation` | In-memory composition vs OPT |
| [validate-from-json](#validate-from-json) | No | `canjson`, `template`, `validation` | Wire bytes → validate |
| [generate-example](#generate-example) | No | `template`, `instance`, `canjson` | OPT → synthesised RM instance → JSON |
| [aql-build](#aql-build) | No | `aql` | Struct + verb builders → byte-identical AQL (REQ-055); containment algebra + in-text paging (REQ-117) |
| [aql-parse-structured](#aql-parse-structured) | No | `aql`, `aql/parse` | Parse AQL → structured `parse.Query` AST + round-trip emit (REQ-113), incl. the REQ-117 catalogue closures and the REQ-118 `SELECT TOP` carrier + literal source text |
| [lint-aql](#lint-aql) | No | `aql/parse`, `aql/lint`, `validation` | AQL static lint + `ValidateAQL` (REQ-109) |
| [compile-build-validate](#compile-build-validate) | No | `template`, `templatecompile`, `composition`, `validation`, `canjson` | Public compile → build → validate, public-only imports (REQ-111) |
| [template-explore](#template-explore) | No | `template`, `templatecompile` | Introspect a compiled OPT: structure tree + leaf paths (REQ-111) |
| [webtemplate-export](#webtemplate-export) | No | `template`, `templatecompile`, `template/webtemplate` | Compiled OPT → EHRbase v2.3 WebTemplate JSON (REQ-106) |
| [flat-roundtrip](#flat-roundtrip) | No | `serialize/simplified`, `template/webtemplate`, `canjson`, `validation` | COMPOSITION ↔ FLAT / STRUCTURED simplified formats + conformant `WithTemplate` decode (REQ-053) |
| [ehr_create](#ehr_create) | Mock (`httptest`) | `discovery`, `transport`, `client/ehr` | Smallest REST create path |
| [contribution-build](#contribution-build) | Optional mock (`-commit`) | `client/ehr/contribution`, `canjson` | Fluent multi-version `Contribution_create` assembly (REQ-130) |
| [smart-launch](#smart-launch) | Mock (`httptest`) | `auth/smart`, `auth` | Standalone PKCE launch; **state + verifier persistence** across redirect (REQ-061) |

---

## Building blocks

### canonical_json

**Purpose:** Smallest end-to-end decode — prove canonical JSON round-trips into Go RM types without HTTP, auth, or discovery.

```bash
go run ./cmd/examples/canonical_json
```

**Packages:** `openehr/rm`, `openehr/serialize/canjson`

**Fixture:** `testkit/cassettes/compositions/body_weight.json`

**Sample output:**

```text
composition: archetype_node_id=openEHR-EHR-COMPOSITION.encounter.v1
  name="body_weight"
  language=nl (terminology=ISO_639-1)
  territory=NL
  ...
OK: canonical-JSON Composition decoded from body_weight.json
```

---

### canxml_roundtrip

**Purpose:** Verify the JSON → struct → XML → struct → JSON invariant that canonical serializers must preserve.

```bash
go run ./cmd/examples/canxml_roundtrip
```

**Packages:** `openehr/rm`, `openehr/serialize/canjson`, `openehr/serialize/canxml`

**Fixture:** same `body_weight.json` cassette as `canonical_json`.

---

### opt-parse

**Purpose:** Parse an operational template (OPT), print identity metadata, and resolve template paths.

```bash
go run ./cmd/examples/opt-parse
go run ./cmd/examples/opt-parse path/to/your-template.opt
```

**Packages:** `openehr/template`

**Surfaces shown:**

- `ParseFileStrict` / lenient `ParseFile`
- `TemplateID`, `Concept`, `Description`, `Annotations`
- `ParsePath`, `NodeAt`, `ValidatePath`, `WithStrictPaths`

**Default fixture:** `testkit/cassettes/templates/vital_signs.opt`

---

### primitive-validate

**Purpose:** Validate individual primitive values (e.g. `DV_QUANTITY` magnitude and units) against OPT leaf constraints — no full composition walker.

```bash
go run ./cmd/examples/primitive-validate
```

**Packages:** `openehr/template`, `openehr/template/constraints`

Uses an embedded minimal OPT (same shape as conformance PROBE-024). Expects some demo cases to **fail** validation intentionally.

---

### validate-composition

**Purpose:** Build an in-memory `*rm.Composition`, compile an OPT, and run template-driven validation (`validation.ValidateComposition`).

```bash
go run ./cmd/examples/validate-composition
go run ./cmd/examples/validate-composition path/to/template.opt
go run ./cmd/examples/validate-composition -invalid   # demo a required-field failure
```

**Packages:** `openehr/template`, `openehr/validation`, `internal/templatecompile`

**Note:** this example calls the internal `templatecompile.Compile` directly (it lives in-repo). External modules use the public `openehr/templatecompile.Compile` bridge instead — see [compile-build-validate](#compile-build-validate) (REQ-111, [ADR 0010](adr/0010-public-compiled-template-bridge.md)).

**Default fixture:** hand-built vital-signs composition matching `vital_signs.opt`.

---

### validate-from-json

**Purpose:** The pipeline most CI validators use — read canonical JSON from disk, decode, compile OPT, validate.

```bash
go run ./cmd/examples/validate-from-json
go run ./cmd/examples/validate-from-json -cassette          # demo data with expected issues
go run ./cmd/examples/validate-from-json comp.json tmpl.opt # custom paths
```

**Flags:**

| Flag | Effect |
|---|---|
| `-cassette` | Use `testkit/cassettes/compositions/vital_signs.json` instead of the clean local fixture |

**Default JSON fixture:** `cmd/examples/validate-from-json/testdata/minimal_blood_pressure.json` (validates cleanly against `vital_signs.opt`).

**Packages:** `openehr/serialize/canjson`, `openehr/template`, `openehr/validation`

---

### generate-example

**Purpose:** Synthesise an RM instance graph from a compiled OPT and emit canonical JSON to stdout — useful for seeders and fixture generation.

```bash
go run ./cmd/examples/generate-example
go run ./cmd/examples/generate-example \
  --opt testkit/cassettes/templates/vital_signs.opt \
  --territory NL \
  --composer-name "Test Composer" \
  --policy example
```

**Flags:**

| Flag | Default | Values |
|---|---|---|
| `--opt` | `vital_signs.opt` fixture | Path to ADL 1.4 OPT |
| `--policy` | `example` | `minimal` or `example` |
| `--territory` | `NL` | ISO 3166-1 code (required for composition roots) |
| `--composer-name` | `Example Composer` | Composer party name |

**Packages:** `openehr/template`, `openehr/instance`, `openehr/serialize/canjson`, `internal/templatecompile`

Pipe output to a file or pipe into `validate-from-json`:

```bash
go run ./cmd/examples/generate-example --policy minimal > /tmp/generated.json
go run ./cmd/examples/validate-from-json /tmp/generated.json testkit/cassettes/templates/vital_signs.opt
```

---

### aql-build

**Purpose:** Build the same logical AQL query two ways — the struct-builder and the verb-functions — and prove both emit the same canonical string on the wire (REQ-055, PROBE-020). A third query demonstrates the REQ-117 containment algebra (`aql.Class` / `Contains` / `NotContains` / `ContainsOr`) and opt-in in-text paging (`LimitInline` / `OffsetInline`). Pure building block: no transport, no auth. The executor lives at `openehr/client/query`.

```bash
go run ./cmd/examples/aql-build
```

**Packages:** `openehr/aql`

**Sample output:**

```text
struct-builder : SELECT o FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.body_temperature.v2] WHERE e/ehr_id/value = $ehr_id AND o/data[at0001]/events[at0006]/data/items[at0004]/value/magnitude > 37.5
verb-functions : SELECT o FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.body_temperature.v2] WHERE e/ehr_id/value = $ehr_id AND o/data[at0001]/events[at0006]/data/items[at0004]/value/magnitude > 37.5
byte-identical : true

containment algebra + in-text paging (REQ-117):
  SELECT c FROM EHR e CONTAINS COMPOSITION c CONTAINS ((OBSERVATION o[openEHR-EHR-OBSERVATION.body_temperature.v2] NOT CONTAINS CLUSTER cl) OR EVALUATION ev) WHERE e/ehr_id/value = $ehr_id ORDER BY c/context/start_time/value DESC LIMIT 20 OFFSET 40
  envelope paging unused — Fetch/Offset stay zero: 0 0
```

**What to copy into your app:** compose with the style you prefer; bind caller data with `aql.Param` (never interpolate into a path), then hand the built `aql.Query` to `query.Execute`. Keep paging on **one** channel — the envelope (`Limit`/`Offset`) by default, the in-text form only when the bound must survive stored-query registration; requesting both is a build-time error.

### aql-parse-structured

**Purpose:** Parse an AQL string into the structured `parse.Query` AST (Tier 2, REQ-113) — the read-side mirror of `aql.Builder` — and emit it back to canonical text via `Query.Emit()`. Since REQ-117 the catalogue covers the whole SDK grammar profile, and since REQ-118 that includes the deprecated `SELECT TOP n [FORWARD|BACKWARD]` clause; the residual `aql.ErrIncompleteAST` is a numeric literal the AST cannot represent, surfaced by `ParseQuery` rather than silently dropping a clause. With no argument the program walks three queries: the representative one below, a REQ-117 query exercising the closed shapes, and a REQ-118 query showing the `TOP` carrier alongside two literals whose **source text** differs from their canonical rendering (`1.50` → `1.5`, `"quoted"` → `'quoted'`) — the openEHR result schema names an unaliased column by its expression text, so `parse.LiteralExpr.Raw` keeps what was written while emission stays canonical. Pure building block: no transport, no auth.

```bash
go run ./cmd/examples/aql-parse-structured
```

**Packages:** `openehr/aql`, `openehr/aql/parse`

**Sample output:**

```text
input AQL:
  SELECT
    c/uid/value,
    c/name/value
  FROM EHR e
    CONTAINS COMPOSITION c
  WHERE c/uid/value = $cid AND c/name/value LIKE 'Vital%'
  ORDER BY c/uid/value DESC
  LIMIT 50 OFFSET 100

structured AST:
  SELECT:
    [0] c/uid/value
    [1] c/name/value
  FROM EHR e
    CONTAINS COMPOSITION c
  WHERE:
    AND:
      c/uid/value = $cid (param)
      c/name/value LIKE 'Vital%' (string)
  ORDER BY:
    [0] c/uid/value DESC
  LIMIT 50 (int)
  OFFSET 100 (int)

canonical emission:
  SELECT c/uid/value, c/name/value FROM EHR e CONTAINS COMPOSITION c WHERE c/uid/value = $cid AND c/name/value LIKE 'Vital%' ORDER BY c/uid/value DESC LIMIT 50 OFFSET 100

--- REQ-117 catalogue closures ---

input AQL:
  SELECT *, 1 AS rank, LENGTH(c/name/value)
  FROM COMPOSITION c OR EHR e
  WHERE LENGTH(c/name/value) > $min AND c/uid/value = c/name/value

structured AST:
  SELECT:
    [0] *
    [1] 1 (int) AS rank
    [2] LENGTH(c/name/value)
  FROM OR (root junction, 2 operands):
    COMPOSITION c
    EHR e
  WHERE:
    AND:
      LENGTH(c/name/value) (func) > $min (param)
      c/uid/value = c/name/value (path)

canonical emission:
  SELECT *, 1 AS rank, LENGTH(c/name/value) FROM COMPOSITION c OR EHR e WHERE LENGTH(c/name/value) > $min AND c/uid/value = c/name/value

--- REQ-118 deprecated SELECT TOP + literal source text ---

input AQL:
  SELECT TOP 5 BACKWARD c/uid/value, 1.50, "quoted"
  FROM COMPOSITION c
  ORDER BY c/uid/value DESC

structured AST:
  SELECT TOP 5 BACKWARD:
    [0] c/uid/value
    [1] 1.5 (real) (source text: 1.50)
    [2] 'quoted' (string) (source text: "quoted")
  FROM COMPOSITION c
  ORDER BY:
    [0] c/uid/value DESC

canonical emission:
  SELECT TOP 5 BACKWARD c/uid/value, 1.5, 'quoted' FROM COMPOSITION c ORDER BY c/uid/value DESC
```

**What to copy into your app:** use `parse.ParseQuery(src)` when you need to introspect a caller-supplied query (highlight paths, swap a comparison value, audit alias bindings), and `errors.Is(err, aql.ErrIncompleteAST)` to branch on catalogue gaps; `Query.Emit()` round-trips the AST back to AQL for execution. Type-switch over `parse.SelectExpr` / `aql.WhereExpr` / `aql.Value` and treat an unrecognised case as out-of-catalogue — those sets grow additively. Check `From.Junction` before `From.Root`: a FROM-root junction leaves `Root` zero.

### lint-aql

**Purpose:** Statically lint AQL before it reaches the CDR (REQ-109): parse against the SDK grammar profile (ADR 0007), then run the lint layers — syntax, shape (alias binding, parameter binding), RM containment and portability semantics against the pinned BMM (REQ-160/161, runs unconditionally, no template needed), and template-aware archetype / path checks against a compiled OPT. Shown via `validation.ValidateAQL`; the building block is `openehr/aql/lint` (`LintString` / `Lint`). Pure building block: no transport, no auth. Lint-clean is **not** spec-conformance and not execute-success — the CDR remains the path authority (PROBE-021).

```bash
go run ./cmd/examples/lint-aql
```

**Packages:** `openehr/aql`, `openehr/aql/parse`, `openehr/aql/lint`, `openehr/validation`

**Sample output:**

```text
== broken query ==
SELECT o FROM OBSERVATION o[openEHR-EHR-OBSERVATION.lab_result.v1] WHERE o/data/events/value/magnitude > $threshold
  [error] aql_unbound_param (-): $threshold is referenced but not bound in Query.Parameters
  [error] aql_archetype_not_in_template (openEHR-EHR-OBSERVATION.lab_result.v1): archetype openEHR-EHR-OBSERVATION.lab_result.v1 is not in template vital_signs

== semantic finding (RM-impossible containment) ==
SELECT o FROM OBSERVATION o CONTAINS COMPOSITION c
  [warning] aql_from_archetype (-): FROM/CONTAINS names no archetype, $param, VERSION, or EHR scope
  [error] aql_impossible_containment (COMPOSITION c): no containment route under the pinned RM connects OBSERVATION to COMPOSITION, so this CONTAINS can never match
```

**What to copy into your app:** for CI / pre-flight checks call `lint.LintString(q, nil)` (Layers 1–2, no template needed); when you hold a compiled OPT, pass it via `lint.Options{Compiled: c}` (or `validation.ValidateAQL`) to add archetype / path checks. Dispatch on `Issue.Code`; treat only `Error`-severity issues as hard failures.

---

### compile-build-validate

**Purpose:** Drive the whole clinical pipeline through **public packages only** (REQ-111) — the shape an external module uses. Parse an OPT, compile it with `openehr/templatecompile.Compile`, build a `*rm.Composition` with the REQ-101 builder, serialise to canonical JSON, round-trip it, and validate. Before REQ-111 the compiled template was only constructable inside the SDK module, so this exact program could not be written downstream.

```bash
go run ./cmd/examples/compile-build-validate
go run ./cmd/examples/compile-build-validate path/to/template.opt
```

**Packages:** `openehr/template`, `openehr/templatecompile`, `openehr/composition`, `openehr/serialize/canjson`, `openehr/validation`, `openehr/rm` — **no `internal/` import.**

**Sample output:**

```text
template : vital_signs (vital_signs.opt)
composition: 7550 bytes canonical JSON, round-tripped
validation : OK — round-tripped composition conforms to the OPT
ehr_status : ValidateEHRStatus callable (OK=false against a COMPOSITION OPT)
```

**What to copy into your app:** `templatecompile.Compile(opt)` once per template, then reuse the `*Compiled` across many `composition.NewBuilder` / `validation.Validate*` calls. The compiled template is the single artifact the builder and validator share.

---

### template-explore

**Purpose:** Introspect a compiled OPT through the public node-level types (REQ-111) — the building block for a form generator or a path-discovery tool. Walks the `templatecompile.CompiledNode` tree to print the template structure (RM type, pinned archetype id / at-code, cardinality + required, term label, slot / primitive markers), then lists the addressable primitive-leaf paths — the canonical `composition.Builder.Set` targets.

```bash
go run ./cmd/examples/template-explore
go run ./cmd/examples/template-explore path/to/template.opt
```

**Packages:** `openehr/template`, `openehr/templatecompile` — **no `internal/` import.**

**Sample output (abridged):**

```text
root     : COMPOSITION

structure (node → attribute → child node):
COMPOSITION [openEHR-EHR-COMPOSITION.encounter.v1]  "Encounter"
  .content [*]
    OBSERVATION [openEHR-EHR-OBSERVATION.blood_pressure.v1]  "Blood Pressure"
      ...
        ELEMENT [at0004]  "Systolic"
          .value [1]
            DV_QUANTITY  ·primitive

addressable primitive-leaf paths (6) — Builder.Set targets:
  /category/defining_code
  /content[openEHR-EHR-OBSERVATION.blood_pressure.v1]/data/events[at0006]/data/items[at0004]/value
  ...
```

**What to copy into your app:** hold `*templatecompile.CompiledNode` / `*templatecompile.CompiledAttribute` in your own walker; `node.RMTypeName()` + `attr.Cardinality()`/`Required()` drive widget choice and required-markers, `node.Term(code, "")` gives the label, `node.PrimitiveConstraint()` marks the editable leaves, and `node.AQLPath()` yields the `Builder.Set` path.

---

### webtemplate-export

**Purpose:** Export a compiled OPT as EHRbase `openEHR_SDK` v2.3 **WebTemplate JSON** (REQ-106, ADR 0014) — the lossy, UI-oriented projection form renderers and FLAT-path mappers consume. Prints the form-oriented tree (FLAT-path `id`, RM type, occurrences, input widgets), then the deterministic document; `-json` dumps the full indented WebTemplate instead.

```bash
go run ./cmd/examples/webtemplate-export
go run ./cmd/examples/webtemplate-export path/to/template.opt
go run ./cmd/examples/webtemplate-export -json path/to/template.opt
```

**Packages:** `openehr/template`, `openehr/templatecompile`, `openehr/template/webtemplate` — **no `internal/` import.**

**Sample output (abridged):**

```text
template : vital_signs (vital_signs.opt)
version  : 2.3   defaultLanguage: en
document : 9839 bytes deterministic JSON (application/openehr.wt+json)

form tree (id [rmType] occurrences — inputs):
encounter [COMPOSITION] 1..1
  category [DV_CODED_TEXT] 1..1 — code:CODED_TEXT(1 codes)
  blood_pressure [OBSERVATION] 0..*
    any_event [EVENT] 0..*
      systolic [DV_QUANTITY] 0..1 — magnitude:DECIMAL, unit:CODED_TEXT(1 codes)
      time [DV_DATE_TIME] 0..1 — DATETIME
    language [CODE_PHRASE] 0..1
    subject [PARTY_PROXY] 0..1 — id:TEXT, id_scheme:TEXT, id_namespace:TEXT, name:TEXT
  ...
```

**What to copy into your app:** `webtemplate.Marshal(compiled)` for the bytes (`application/openehr.wt+json`), or `webtemplate.Build(compiled)` when you post-process the typed tree first — each `Node.ID` is the FLAT-path segment consumers bind to, and each leaf's `Inputs` (`suffix`/`type`/`list`/`validation`) drives the widget. Both fail loudly (`ErrEmptyTemplate` / `ErrNoDefaultLanguage` / `ErrIDCollision`) rather than emit ambiguous output; accepted reference deltas are documented in the package's `deviations.md`.

---

### flat-roundtrip

**Purpose:** Convert a canonical `COMPOSITION` to the **FLAT** and **STRUCTURED** Simplified Formats and back (REQ-053), driven by the composition's Web Template (REQ-106). Shows the encode/decode entry points, the OPT-free `FlatToStructured`, the `COMPOSITION → FLAT → COMPOSITION → FLAT` round-trip, and the **conformant decode** (`WithTemplate`) whose result validates against the OPT — with no transport or auth.

```bash
go run ./cmd/examples/flat-roundtrip
```

**Packages:** `openehr/serialize/simplified`, `openehr/template/webtemplate`, `openehr/templatecompile`, `openehr/serialize/canjson`, `openehr/validation` — **no `internal/` import.**

**Sample output (abridged, keys sorted):**

```text
FLAT (application/openehr.wt.flat+json):
  ctx/composer_name = Max Mustermann
  ctx/language = en
  ctx/territory = DE
  ctx/time = 2022-02-03T04:05:06.000
  test_dv_quantity_open_constraint.v0/category|code = 433
  test_dv_quantity_open_constraint.v0/test123/any_event:0/my_dv_quantity|magnitude = 130
  test_dv_quantity_open_constraint.v0/test123/any_event:0/my_dv_quantity|unit = mmHg
  ...

STRUCTURED (application/openehr.wt.structured+json): 412 bytes

OK: FLAT -> COMPOSITION -> FLAT round-trips for Test_dv_quantity_open_constraint.v0
OK: WithTemplate decode validates against the OPT
```

**What to copy into your app:** build the Web Template once (`templatecompile.Compile` + `webtemplate.Build`), then `simplified.MarshalFlat(comp, wt)` / `UnmarshalFlat(data, wt)` (and the `…Structured` pair) for OPT-driven conversion, or `FlatToStructured` / `StructuredToFlat` for OPT-free interconversion. Pass `simplified.WithTemplate(compiled)` to `Unmarshal*` when you need an OPT-validatable composition (names + RM-mandatory attributes repopulated) rather than a format-idempotent one. Composition-level metadata rides `ctx/`; decorated or exotic datatypes ride `|raw`. The codec is strict on decode (unknown paths/suffixes, wrong-typed ctx values, index games, and malformed input error rather than drop data) — see the package's `deviations.md`.

---

## REST client

### ehr_create

**Purpose:** End-to-end EHR creation — static service catalog → transport client → typed `ehr.Create`, backed by an in-process `httptest` server (no external CDR).

```bash
go run ./cmd/examples/ehr_create
```

**Packages:** `smart/discovery`, `transport`, `openehr/client/ehr`

**Sample output:**

```text
created EHR: id=f0e1d2c3-b4a5-6789-0123-456789abcdef
  system_id=example.system
  metadata: VersionUID="f0e1d2c3-b4a5-6789-0123-456789abcdef" Location="/openehr/v1/ehr/f0e1d2c3-b4a5-6789-0123-456789abcdef"
OK: end-to-end EHR creation against in-process httptest backend
```

**What to copy into your app:**

1. Build a `discovery.ServiceCatalog` (static or fetched from a SMART issuer).
2. `transport.New(catalog, transport.WithHTTPClient(yourClient))`.
3. Call leaf clients (`ehr.Create`, `query.Execute`, …) with `context.Context`.

To hit a real backend, swap the catalog base URL and add `transport.WithTokenSource`. See [quick-start.md](quick-start.md#path-b--rest-client-live-or-mocked-backend).

---

### contribution-build

**Purpose:** Assemble a multi-version CONTRIBUTION with `contribution.Builder` (REQ-130) — two vendored canonical compositions committed in one batch, one as a first version and one as an amendment — and print the `Contribution_create` body. `-commit` additionally POSTs it through `contribution.Commit` to an in-process fake CDR and asserts the captured request is byte-identical to what was built.

```bash
go run ./cmd/examples/contribution-build
go run ./cmd/examples/contribution-build -commit
```

**Packages:** `openehr/client/ehr/contribution`, `openehr/serialize/canjson`, `testkit/fixtures` (+ `smart/discovery`, `transport` under `-commit`)

**Sample output** (body elided):

```text
batch audit change_type: creation (249) — declared, not derived
versions[0]: ORIGINAL_VERSION<COMPOSITION> change_type=creation/249 lifecycle_state=complete/532 preceding=(none — a first version)
versions[1]: ORIGINAL_VERSION<COMPOSITION> change_type=amendment/250 lifecycle_state=complete/532 preceding=8849182c-82ad-4088-a07f-48ead4180515::cdr.example::1
```

**What to copy into your app:**

1. `contribution.NewBuilder()`, then declare the batch audit once — committer, system id, and the batch `change_type` (which is *never* derived from the versions).
2. Accumulate one `Change` per version: `contribution.Creation(payload)`, or `Amendment` / `Modification` / `Deletion` with the preceding version uid. Each is generic over the four versionable RM types, so a wrong payload type is a compile error.
3. `Build()` once — it returns a `*contribution.Submission` that has already passed `Validate`, or every accumulated error joined.
4. `contribution.Commit(ctx, client, ehrID, submission)`.

Per-version overrides (`WithLifecycleState`, `WithVersionCommitter`, `WithVersionDescription`, `WithVersionSystemID`, `WithVersionUID`) refine what a version would otherwise inherit from the batch audit.

---

### smart-launch

**Purpose:** Demonstrate the full **standalone SMART-on-openEHR authorization-code + PKCE flow** for a public client (no client secret), backed by an in-process `httptest`-style stub server — no external network, no secrets, works offline.

The key teaching point is the **state + PKCE code_verifier persistence** across the redirect: `auth/smart.AuthorizationRequest` (returned by `BeginAuthorization`) must be stored server-side between the initial redirect and the callback, then retrieved by `state` and passed unchanged to `ExchangeAuthorizationCode`.

```bash
go run ./cmd/examples/smart-launch
```

**Packages:** `auth/smart`, `auth` (scope constants), `smart/discovery`

**Sample output:**

```text
step 1: Source built (public client, PKCE, standalone)
step 2: BeginAuthorization → state="…"  verifier="…"
step 3: authorize URL built (len=306)
step 4: AuthorizationRequest stored in session map (key="…")
step 5: redirect received  code="stub-code-…"  state="…"
step 6: AuthorizationRequest retrieved from session map (state validated)
step 7: token exchange complete
  access_token : stub-access-token-001
  token_type   : Bearer
  scope        : openid launch/patient offline_access
  expires_at   : …
  refresh_token: stub-refresh-token-001
  ehrId        : 00000000-0000-0000-0000-000000000001
OK: standalone SMART PKCE launch flow completed (in-process stub)
```

**What to copy into your app:**

1. Call `BeginAuthorization("")` to get an `AuthorizationRequest` with a random `state` and PKCE pair.
2. Persist the `AuthorizationRequest` in a session store keyed by `state` **before** redirecting the user.
3. On the redirect callback, retrieve the stored `AuthorizationRequest` by `callbackState`, delete it (replay prevention), and pass it to `ExchangeAuthorizationCode`.
4. `ExchangeAuthorizationCode` re-validates `state` internally (CSRF guard) and sends the `code_verifier` to the token endpoint (PKCE proof).

See [specifications/auth.md § REQ-061](specifications/auth.md#req-061--pkce-flow) for the normative rules.

---

## Suggested learning order

```text
1. canonical_json          ← RM + canjson basics
2. opt-parse               ← understand templates and paths
3. validate-from-json      ← wire bytes + validation (CI pattern)
4. generate-example        ← synthesise data from templates
5. ehr_create              ← REST wiring (mock first, then real CDR)
6. smart-launch            ← SMART PKCE auth (standalone, public client)
```

Optional depth: `canxml_roundtrip` (multi-format), `primitive-validate` (leaf constraints), `validate-composition` (in-memory RM construction), `contribution-build` (batched atomic writes).

---

## Fixtures and testkit

Examples depend on [`testkit/fixtures`](../testkit/fixtures/) and cassettes under `testkit/cassettes/`. These are stable, checked-in artefacts — not generated at runtime (except `validate-from-json/testdata/`, produced once via `gen_fixture.go`).

When writing your own tests, prefer importing fixtures from `testkit` rather than copying paths by hand.

---

## Maintaining this catalog

Agents and contributors: when you add or materially change an example under `cmd/examples/`, update this file, [`cmd/examples/doc.go`](../cmd/examples/doc.go), and [`quick-start.md`](quick-start.md) (if onboarding changes) in the **same PR**. Checklist: [ai-workflow.md § Examples](ai-workflow.md#examples).

---

## Related documentation

- [quick-start.md](quick-start.md) — install, idioms, REST wiring
- [architecture.md](architecture.md) — package map and dependency rules
- [specifications/use-cases.md](specifications/use-cases.md) — benchmark, seeder, MCP, federator consumers
- [roadmap.md](roadmap.md) — what is landed vs planned
