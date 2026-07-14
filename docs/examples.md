# Examples

Runnable programs under [`cmd/examples/`](../cmd/examples/) demonstrate each major SDK surface. They are **reference shapes** — production tools (benchmark harnesses, MCP servers, federators) live in their own repositories but follow the same patterns.

All examples resolve fixture paths relative to the source file, so `go run ./cmd/examples/<name>` works from **any working directory** inside a clone.

Build every example at once:

```bash
make build
# or: go build ./cmd/examples/...
```

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
| [aql-build](#aql-build) | No | `aql` | Struct + verb builders → byte-identical AQL (REQ-055) |
| [aql-parse-structured](#aql-parse-structured) | No | `aql`, `aql/parse` | Parse AQL → structured `parse.Query` AST + round-trip emit (REQ-113) |
| [lint-aql](#lint-aql) | No | `aql/parse`, `aql/lint`, `validation` | AQL static lint + `ValidateAQL` (REQ-109) |
| [compile-build-validate](#compile-build-validate) | No | `template`, `templatecompile`, `composition`, `validation`, `canjson` | Public compile → build → validate, public-only imports (REQ-111) |
| [template-explore](#template-explore) | No | `template`, `templatecompile` | Introspect a compiled OPT: structure tree + leaf paths (REQ-111) |
| [webtemplate-export](#webtemplate-export) | No | `template`, `templatecompile`, `template/webtemplate` | Compiled OPT → EHRbase v2.3 WebTemplate JSON (REQ-106) |
| [ehr_create](#ehr_create) | Mock (`httptest`) | `discovery`, `transport`, `client/ehr` | Smallest REST create path |
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

**Purpose:** Build the same logical AQL query two ways — the struct-builder and the verb-functions — and prove both emit the same canonical string on the wire (REQ-055, PROBE-020). Pure building block: no transport, no auth. The executor lives at `openehr/client/query`.

```bash
go run ./cmd/examples/aql-build
```

**Packages:** `openehr/aql`

**Sample output:**

```text
struct-builder : SELECT o FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.body_temperature.v2] WHERE e/ehr_id/value = $ehr_id AND o/data[at0001]/events[at0006]/data/items[at0004]/value/magnitude > 37.5
verb-functions : SELECT o FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.body_temperature.v2] WHERE e/ehr_id/value = $ehr_id AND o/data[at0001]/events[at0006]/data/items[at0004]/value/magnitude > 37.5
byte-identical : true
```

**What to copy into your app:** compose with the style you prefer; bind caller data with `aql.Param` (never interpolate into a path), then hand the built `aql.Query` to `query.Execute`.

### aql-parse-structured

**Purpose:** Parse an AQL string into the structured `parse.Query` AST (Tier 2, REQ-113) — the read-side mirror of `aql.Builder` — and emit it back to canonical text via `Query.Emit()`. Inputs outside the v1 catalogue surface as `aql.ErrIncompleteAST` from `ParseQuery` rather than silently dropping a clause. Pure building block: no transport, no auth.

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
```

**What to copy into your app:** use `parse.ParseQuery(src)` to get the structured AST when you need to introspect a caller-supplied query (highlight paths, swap a comparison value, audit alias bindings); check `errors.Is(err, aql.ErrIncompleteAST)` to branch on catalogue gaps. `Query.Emit()` round-trips the AST back to AQL for execution against the CDR.

### lint-aql

**Purpose:** Statically lint AQL before it reaches the CDR (REQ-109): parse against the SDK grammar profile (ADR 0007), then run the three lint layers — syntax, shape (alias binding, parameter binding), and template-aware archetype / path checks against a compiled OPT. Shown via `validation.ValidateAQL`; the building block is `openehr/aql/lint` (`LintString` / `Lint`). Pure building block: no transport, no auth. Lint-clean is **not** spec-conformance and not execute-success — the CDR remains the path authority (PROBE-021).

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

Optional depth: `canxml_roundtrip` (multi-format), `primitive-validate` (leaf constraints), `validate-composition` (in-memory RM construction).

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
