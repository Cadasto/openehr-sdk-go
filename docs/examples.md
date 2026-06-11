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
| [ehr_create](#ehr_create) | Mock (`httptest`) | `discovery`, `transport`, `client/ehr` | Smallest REST create path |

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

**Note:** `templatecompile.Compile` is currently internal. This in-repo example is the supported call shape until a public `template.Compile` lands ([ADR 0005](adr/0005-compiled-template-foundation.md)).

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

## REST client

### ehr_create

**Purpose:** End-to-end EHR creation — static service catalog → transport client → typed `ehr.Create`, backed by an in-process `httptest` server (no external CDR).

```bash
go run ./cmd/examples/ehr_create
```

**Packages:** `smart/discovery`, `transport`, `openehr/client/ehr`

**What to copy into your app:**

1. Build a `discovery.ServiceCatalog` (static or fetched from a SMART issuer).
2. `transport.New(catalog, transport.WithHTTPClient(yourClient))`.
3. Call leaf clients (`ehr.Create`, `query.Execute`, …) with `context.Context`.

To hit a real backend, swap the catalog base URL and add `transport.WithTokenSource`. See [quick-start.md](quick-start.md#path-b--rest-client-live-or-mocked-backend).

---

## Suggested learning order

```text
1. canonical_json          ← RM + canjson basics
2. opt-parse               ← understand templates and paths
3. validate-from-json      ← wire bytes + validation (CI pattern)
4. generate-example        ← synthesise data from templates
5. ehr_create              ← REST wiring (mock first, then real CDR)
```

Optional depth: `canxml_roundtrip` (multi-format), `primitive-validate` (leaf constraints), `validate-composition` (in-memory RM construction).

---

## Fixtures and testkit

Examples depend on [`testkit/fixtures`](../testkit/fixtures/) and cassettes under `testkit/cassettes/`. These are stable, checked-in artefacts — not generated at runtime (except `validate-from-json/testdata/`, produced once via `gen_fixture.go`).

When writing your own tests, prefer importing fixtures from `testkit` rather than copying paths by hand.

---

## Maintaining this catalog

Agents and contributors: when you add or materially change an example under `cmd/examples/`, update this file, [`cmd/examples/doc.go`](../cmd/examples/doc.go), and [`quick-start.md`](quick-start.md) (if onboarding changes) in the **same PR**. Checklist: [ai-workflow.md § Developer examples & docs](ai-workflow.md#developer-examples--docs).

---

## Related documentation

- [quick-start.md](quick-start.md) — install, idioms, REST wiring
- [architecture.md](architecture.md) — package map and dependency rules
- [specifications/use-cases.md](specifications/use-cases.md) — benchmark, seeder, MCP, federator consumers
- [roadmap.md](roadmap.md) — what is landed vs planned
