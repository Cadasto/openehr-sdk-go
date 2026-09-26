# Examples

If you are new to the SDK, run `go run ./cmd/examples/canonical_json` and then follow the [suggested learning order](#suggested-learning-order). Every example works offline: the REST ones use an in-process `httptest` backend, so nothing needs a clinical data repository (CDR).

The 17 runnable programs under [`cmd/examples/`](../cmd/examples/) demonstrate each major SDK surface. They are **reference shapes**. Production tools (benchmark harnesses, MCP servers, federators) live in their own repositories but follow the same patterns. Each entry below ends with a **What to copy into your app** note. If you are here to build something, read that part.

Each entry says what the program shows, which packages it uses, how to run it, and what the output means. Fixture paths resolve relative to the source file, so `go run ./cmd/examples/<name>` works from any working directory inside a clone. Build them all with `make build` (or `go build ./cmd/examples/...`).

A few openEHR terms recur throughout. A **COMPOSITION** is the top-level clinical document. An **OPT** (operational template) is the deployable form of a template: every archetype it uses, flattened into one XML file with the template's constraints applied; it fixes which archetypes, nodes and value constraints a composition may contain. **Canonical JSON** is the openEHR REST wire format for those documents. **AQL** (Archetype Query Language) is the openEHR query language. A **Web Template** is the JSON form of a compiled OPT that form renderers and the FLAT / STRUCTURED simplified formats work from.

---

## At a glance

The Packages column lists the SDK packages each program imports, by short name (`canjson` is `openehr/serialize/canjson`, `discovery` is `smart/discovery`); each section below gives the full import paths. The `testkit/fixtures` helper that locates the bundled files is left out.

| Example | Network | Packages | Demonstrates |
|---|---|---|---|
| [canonical_json](#canonical_json) | No | `rm`, `canjson` | Decode canonical JSON → typed `Composition` |
| [canxml_roundtrip](#canxml_roundtrip) | No | `rm`, `canjson`, `canxml` | JSON ↔ XML cross-format round-trip |
| [opt-parse](#opt-parse) | No | `template` | Parse an ADL 1.4 OPT, walk paths |
| [primitive-validate](#primitive-validate) | No | `template`, `constraints` | Single values against one OPT leaf constraint |
| [validate-composition](#validate-composition) | No | `template`, `templatecompile`, `validation`, `rm`, `terminology` | In-memory composition vs OPT |
| [validate-from-json](#validate-from-json) | No | `canjson`, `template`, `templatecompile`, `validation`, `rm` | Wire bytes → validate |
| [generate-example](#generate-example) | No | `template`, `templatecompile`, `instance`, `canjson`, `rm` | OPT → generated RM instance → JSON |
| [aql-build](#aql-build) | No | `aql`, `aql/contain` | Struct + verb builders → byte-identical AQL; nested CONTAINS + in-text paging; opt-in RM containment check |
| [aql-parse-structured](#aql-parse-structured) | No | `aql`, `aql/parse` | Parse AQL → structured tree, print each clause, emit canonical text back |
| [lint-aql](#lint-aql) | No | `aql`, `template`, `templatecompile`, `validation` | AQL lint through `ValidateAQL`: syntax, shape, RM, template |
| [compile-build-validate](#compile-build-validate) | No | `template`, `templatecompile`, `composition`, `validation`, `canjson`, `rm` | Compile → build → round-trip → validate, public imports only |
| [template-explore](#template-explore) | No | `template`, `templatecompile` | Walk a compiled OPT: structure tree + leaf paths |
| [webtemplate-export](#webtemplate-export) | No | `template`, `templatecompile`, `template/webtemplate` | Compiled OPT → Web Template JSON |
| [flat-roundtrip](#flat-roundtrip) | No | `serialize/simplified`, `template`, `template/webtemplate`, `templatecompile`, `canjson`, `validation`, `rm` | COMPOSITION ↔ FLAT / STRUCTURED simplified formats + template-aware `WithTemplate` decode |
| [ehr_create](#ehr_create) | Mock (`httptest`) | `discovery`, `transport`, `client/ehr` | Smallest REST create path |
| [contribution-build](#contribution-build) | Optional mock (`-commit`) | `client/ehr/contribution`, `client/ehr`, `canjson`, `rm`, `discovery`, `transport` | Multi-version `Contribution_create` assembly, optionally committed |
| [smart-launch](#smart-launch) | Mock (`httptest`) | `auth/smart`, `auth`, `discovery` | Standalone PKCE launch; **state + verifier persistence** across the redirect |

---

## Building blocks

### canonical_json

**Purpose:** Decode an openEHR COMPOSITION from canonical JSON into the typed `rm.Composition` struct and print a few of its fields. This is the smallest useful program in the SDK: no HTTP, no auth, no discovery, just bytes in and Go structs out.

```bash
go run ./cmd/examples/canonical_json
```

**Packages:** `openehr/rm`, `openehr/serialize/canjson` (plus `testkit/fixtures`, which locates the cassette)

**Fixture:** `testkit/cassettes/compositions/body_weight.json`

**Sample output:**

```text
composition: archetype_node_id=openEHR-EHR-COMPOSITION.encounter.v1
  name="body_weight"
  language=nl (terminology=ISO_639-1)
  territory=NL
  category=event
  content items=1
OK: canonical-JSON Composition decoded from body_weight.json
```

`archetype_node_id` names the archetype the document is built on. `language` and `territory` are CODE_PHRASE values: a code plus the terminology it comes from. `category` is the composition category from the openEHR terminology, and `content items` counts the entries (observations, evaluations, ...) the document carries.

**What to copy into your app:** `canjson.Unmarshal(body, &composition)` into an `rm.Composition`. The codec reads the `_type` discriminator on every object and fills the matching `rm` struct, polymorphic children included, so you never switch on `_type` yourself. A decode error means the bytes are not well-formed canonical JSON; report it before any template gets involved.

---

### canxml_roundtrip

**Purpose:** Take one COMPOSITION through both canonical formats and back: JSON to structs, structs to canonical XML, XML back to structs, structs to JSON. The program then compares the JSON it started from with the JSON it ended with, which shows that the two codecs (`canjson` and `canxml`) describe the same document.

```bash
go run ./cmd/examples/canxml_roundtrip
```

**Packages:** `openehr/rm`, `openehr/serialize/canjson`, `openehr/serialize/canxml`

**Fixture:** same `body_weight.json` cassette as `canonical_json`.

**Sample output:**

```text
input JSON: 10947 bytes
canonical XML: 4961 bytes
  preview: <composition xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" archetype_node_id="openEHR-EHR-COMPOSITION.encounter.v1"><name xsi:type="DV_TEXT"><value>body_w...
re-encoded JSON: 4626 bytes
OK: JSON ↔ XML cross-format round-trip preserves the Composition structurally
```

The byte counts differ because the cassette is pretty-printed and the SDK's encoders are compact; the comparison is on the decoded content, not on the bytes. In the XML, the JSON `_type` discriminator becomes `xsi:type` and the element names match the JSON keys. Before comparing, the program encodes its starting point through `canjson` too and drops null members on both sides: the SDK treats a null member and an absent member the same, and the two codecs may pick either spelling for an empty optional field.

**What to copy into your app:** `canxml.Marshal` / `canxml.Unmarshal` are drop-in counterparts of the `canjson` pair. When you compare documents that crossed formats, encode both sides through the same codec and treat null and absent members as equal, as `sameJSON` in the program does.

---

### opt-parse

**Purpose:** Parse an ADL 1.4 operational template (OPT) and look around inside it: identity, provenance metadata, the root node with its attributes, and path resolution. Nothing here needs a network or a compiled template.

```bash
go run ./cmd/examples/opt-parse
go run ./cmd/examples/opt-parse path/to/template.opt
```

**Packages:** `openehr/template`

**Surfaces shown:**

- `ParseFileStrict`, which rejects an unknown node type that has attributes under it, and the lenient `ParseFile`, which keeps such a node as a leaf and silently drops everything beneath it (an unknown node with no attributes under it is a leaf in both modes)
- `TemplateID`, `Concept`, `UID`, `Language`, `Description`, `Annotations`
- `Root`, the `ObjectNode` interface a tree walker dispatches on, and `Attributes`
- `ParsePath`, `ValidatePath`, `NodeAt`, `WithStrictPaths`, `ErrAmbiguousPath`

**Default fixture:** `testkit/cassettes/templates/vital_signs.opt`

**Sample output:**

```text
template_id : vital_signs
concept     : vital_signs
uid         : a4571722-13bb-43f2-b9a2-9d0e9aaa7745
language    : en
lifecycle   : Initial
authors     : map[Original Author:Not Specified]
root        : COMPOSITION [at0000]
archetype   : openEHR-EHR-COMPOSITION.encounter.v1
attributes  :
  category (single, children=1)
  content (multiple, children=4)
NodeAt(/content): OBSERVATION [at0000] archetype=openEHR-EHR-OBSERVATION.blood_pressure.v1
strict       : /content is ambiguous (multiple children) — add an [archetype-id] or [at-code] predicate
```

The root is the COMPOSITION archetype the template is built on. `content` is the attribute that holds its clinical entries and has four child nodes here. `NodeAt(/content)` in the default (lenient) mode picks the first child; in strict mode the same lookup returns `ErrAmbiguousPath`, and the caller adds a predicate such as `/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]` to say which child it means.

**What to copy into your app:** `template.ParseFileStrict` in a validator that must fail loudly on a template shape the parser does not support; `ParseFile` when forward compatibility matters more, knowing that it drops the subtree under such a node. Call `opt.ParsePath` once per path, then `ValidatePath` for a precondition check or `NodeAt` for the node itself. Validators and code generators should pass `template.WithStrictPaths()` and handle `template.ErrAmbiguousPath` with `errors.Is`, so a path never silently resolves to the wrong child.

---

### primitive-validate

**Purpose:** Validate single values (a `DV_QUANTITY` magnitude and units) against one leaf constraint of an OPT, without compiling the template or building a composition. The embedded OPT constrains a DV_QUANTITY to a magnitude of 0..300 in `mm[Hg]`; the program resolves that leaf by path and checks three values against it, two of which fail on purpose.

```bash
go run ./cmd/examples/primitive-validate
```

**Packages:** `openehr/template`, `openehr/template/constraints`

**Fixture:** none; the minimal OPT is embedded in the program.

**Sample output:**

```text
template_id : example_primitive
constraint  : constraints.DvQuantity at /content
  in-range               OK
  out-of-range magnitude 1 violation(s)
    [out_of_range] magnitude 500 outside [0..300] for units "mm[Hg]"
  unknown unit           1 violation(s)
    [unit_unknown] units "psi" not in allowed [mm[Hg]]
summary     : 2/3 cases failed validation (expected for demo)
```

Each violation carries a `Code`, the stable identifier a program branches on, and a `Detail`, the explanation for people.

**What to copy into your app:** resolve the leaf with `opt.ParsePath` and `opt.NodeAt`, assert `*template.ComplexObject`, and read its `PrimitiveConstraint()` (nil on a structural node). `constraint.Validate(constraints.QuantityValue{Magnitude: m, Units: u})` returns nil when every clause holds, otherwise one `Violation` per failed clause. Use this for field-level checks in a form; use the composition validators when you hold a whole document.

---

### validate-composition

**Purpose:** Build a composition in memory as plain Reference Model structs, compile an OPT, and validate the composition against it with `validation.ValidateComposition`. This is the smallest validation path: no JSON, no HTTP, just typed RM values, a compiled template, and the list of issues the validator returns.

```bash
go run ./cmd/examples/validate-composition
go run ./cmd/examples/validate-composition path/to/template.opt
go run ./cmd/examples/validate-composition -invalid   # clear a required attribute first
```

**Packages:** `openehr/rm`, `openehr/template`, `openehr/templatecompile`, `openehr/terminology`, `openehr/validation`. **No `internal/` import.**

It uses the same public `templatecompile.Compile` bridge as [compile-build-validate](#compile-build-validate), so the program can be copied into another module unchanged.

**Default fixture:** `vital_signs.opt`, with a hand-built composition that matches it: an encounter holding one blood-pressure OBSERVATION with a systolic reading.

**Sample output:**

```text
template    : vital_signs (vital_signs.opt)
compiled    : root COMPOSITION
result      : OK — no issues
```

With `-invalid` the program clears the composition's category before validating. The validator then reports one issue, `/category [required] required attribute "category" absent on COMPOSITION`, and the program exits 1.

**What to copy into your app:** compile once (`templatecompile.Compile(opt)`) and reuse the `*Compiled` for every composition. `validation.ValidateComposition(comp, compiled)` collects every issue in one pass; read `result.OK` for the verdict and `result.Issues` for the list, each with `Path` (where), `Code` (the stable identifier to branch on) and `Detail` (the explanation). Look coded labels up in `openehr/terminology` (`terminology.CompositionCategory.Rubric("433")`) instead of typing them next to the code, so the two cannot drift apart.

---

### validate-from-json

**Purpose:** Validate a COMPOSITION that arrives as canonical JSON against an OPT, the way a CI check or an inbound gateway would: read the bytes, decode them into RM structs, compile the OPT, and list every constraint the document breaks.

```bash
go run ./cmd/examples/validate-from-json
go run ./cmd/examples/validate-from-json -cassette          # demo data with expected issues
go run ./cmd/examples/validate-from-json comp.json tmpl.opt # your own files
```

**Flags:**

| Flag | Effect |
|---|---|
| `-cassette` | Validate `testkit/cassettes/compositions/vital_signs.json`, demo data that reports issues, instead of the clean local fixture |

The exit status is 1 when the composition does not validate (and on a usage error), so the command can gate a pipeline. Validation issues are a result the program prints; only a program error, such as a bad path or an unreadable OPT, is reported as a failure.

**Default JSON fixture:** `cmd/examples/validate-from-json/testdata/minimal_blood_pressure.json`, a hand-made composition that validates cleanly against `vital_signs.opt`.

**Packages:** `openehr/rm`, `openehr/serialize/canjson`, `openehr/template`, `openehr/templatecompile`, `openehr/validation`. **No `internal/` import.**

**Sample output:**

```text
json        : minimal_blood_pressure.json (2084 bytes)
composition : archetype_node_id=openEHR-EHR-COMPOSITION.encounter.v1 content_items=1
template    : vital_signs (vital_signs.opt)
result      : OK — JSON validates against OPT
```

With `-cassette` the result line reports the issue count, one `path [code] detail` line follows per issue, and a note says the issues are expected.

**What to copy into your app:** the three steps in order: `canjson.Unmarshal` (a document that is not well-formed canonical JSON fails here, before any template is involved), `template.ParseFile` plus `templatecompile.Compile` once per template, then `validation.ValidateComposition`. Map `result.OK` to your exit status and print `result.Issues`. The same compiled template also feeds the composition builder, the instance generator and the AQL lint.

---

### generate-example

**Purpose:** Generate an RM instance from an OPT and print it as canonical JSON. The template alone decides the shape of the document: the SDK creates every node the template requires and fills the leaves with placeholder values. Seeders and fixture generators use this shape.

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
| `--opt` | `vital_signs.opt` fixture | Path to an ADL 1.4 OPT |
| `--policy` | `example` | `minimal` creates only the nodes the template requires; `example` also fills every primitive leaf with its example value |
| `--territory` | `NL` | ISO 3166-1 code; a COMPOSITION root requires one |
| `--composer-name` | `Example Composer` | Name recorded as the composition's composer |

**Packages:** `openehr/template`, `openehr/templatecompile`, `openehr/instance`, `openehr/rm`, `openehr/serialize/canjson`. **No `internal/` import.**

The output is one line of JSON. Each run gets fresh uids and a wall-clock context start time, so two runs are never byte-identical; that is why this entry shows no sample block. Pipe the output to a file or into `validate-from-json` to see that generated data passes the same template:

```bash
go run ./cmd/examples/generate-example --policy minimal > /tmp/generated.json
go run ./cmd/examples/validate-from-json /tmp/generated.json testkit/cassettes/templates/vital_signs.opt
```

**What to copy into your app:** `instance.Generate(ctx, compiled, instance.Options{Policy: ..., Territory: ..., Composer: ...})`. It returns the root as `any`, because a template can be rooted on any archetypeable type; `canjson.Marshal` encodes it as is, and `instance.AsComposition` (with siblings for the other root types) gives you the typed value. Optional RM strings are pointers, hence `new(name)` for the composer name. An unknown policy name is an error, so a typo on the command line does not silently pick a default.

---

### aql-build

**Purpose:** Build AQL queries with `openehr/aql` and print the text each one produces. The builder only produces text and never talks to a server; executing a built query is the job of `openehr/client/query`.

The program shows three things. First, the same query built two ways, with the chained `aql.Builder` and with free-standing verb functions, producing byte-identical text. Second, the containment algebra (`aql.Class` / `Contains` / `NotContains` / `ContainsOr`) for nested CONTAINS clauses, together with in-text paging (`LimitInline` / `OffsetInline`). Third, `Builder.VerifyContainment`, the opt-in check of whether the classes in a query can contain one another under the openEHR Reference Model (RM); `Build` never asks that question. The check runs over a clean containment tree and over one that is well-formed AQL but can never return a row.

```bash
go run ./cmd/examples/aql-build
```

**Packages:** `openehr/aql`, `openehr/aql/contain`

**Sample output:**

```text
struct-builder : SELECT o FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.body_temperature.v2] WHERE e/ehr_id/value = $ehr_id AND o/data[at0001]/events[at0006]/data/items[at0004]/value/magnitude > 37.5
verb-functions : SELECT o FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.body_temperature.v2] WHERE e/ehr_id/value = $ehr_id AND o/data[at0001]/events[at0006]/data/items[at0004]/value/magnitude > 37.5
byte-identical : true

containment algebra + in-text paging:
  SELECT c FROM EHR e CONTAINS COMPOSITION c CONTAINS ((OBSERVATION o[openEHR-EHR-OBSERVATION.body_temperature.v2] NOT CONTAINS CLUSTER cl) OR EVALUATION ev) WHERE e/ehr_id/value = $ehr_id ORDER BY c/context/start_time/value DESC LIMIT 20 OFFSET 40
  envelope paging unused — Fetch/Offset stay zero: 0 0

containment verification (opt-in; Build never runs it):
  == containment algebra ==
  SELECT c FROM EHR e CONTAINS COMPOSITION c CONTAINS ((OBSERVATION o[openEHR-EHR-OBSERVATION.body_temperature.v2] NOT CONTAINS CLUSTER cl) OR EVALUATION ev) WHERE e/ehr_id/value = $ehr_id ORDER BY c/context/start_time/value DESC LIMIT 20 OFFSET 40
  result : no findings — every containment step is admissible under the pinned RM

  == RM-impossible query ==
  SELECT ev FROM OBSERVATION o CONTAINS EVALUATION ev[openEHR-EHR-OBSERVATION.body_temperature.v2]
  aql_archetype_class_mismatch
    archetype openEHR-EHR-OBSERVATION.body_temperature.v2 does not conform to declared class EVALUATION, so this class expression can never match
  aql_impossible_containment
    no containment route under the pinned RM connects OBSERVATION to EVALUATION, so this CONTAINS can never match
```

In the first section the verb style gives its clauses in a different order on purpose; the emitter fixes the clause order, so the text still comes out identical. `$ehr_id` is a placeholder from `aql.Param` that is bound at execution time. In the second section the in-text `LIMIT 20 OFFSET 40` leaves the envelope fields `Fetch` and `Offset` at zero. In the third, each finding carries a stable `Code` and a `Detail`; the RM-impossible query builds and emits like any other, because `Build` only answers whether the AQL is well-formed.

**What to copy into your app:** compose with the style you prefer; bind caller data with `aql.Param` (never paste it into the query text), then hand the built `aql.Query` to `query.Execute`. Keep paging on one channel: the envelope (`Limit` / `Offset`) by default, or the in-text form only when the bound must survive registration as a stored query. Requesting both is a build-time error. `VerifyContainment` is opt-in and checks the query against the RM, which `Build` never does; dispatch on `contain.Finding.Code`. A nil relation uses the default RM relation, and `contain.Default().WithOverlay(...)` widens it for a CDR that admits more routes.

### aql-parse-structured

**Purpose:** Parse AQL text into the structured query tree (`parse.Query`) that `openehr/aql/parse` returns, print each clause of that tree, and emit the tree back to canonical text with `Query.Emit()`. It is the read-side mirror of `aql.Builder`: the builder goes from Go values to text, the parser from text to Go values. The tree covers the whole grammar the SDK accepts, including the deprecated `SELECT TOP n [FORWARD|BACKWARD]` clause. The one shape it cannot hold is a numeric literal outside the range of the Go value it maps to (an integer beyond `int64`, a real beyond `float64`); `ParseQuery` reports that with `aql.ErrIncompleteAST` rather than silently dropping a clause.

With no argument the program walks three queries: the representative one below; one that mixes `*` with column projections, a projected literal and function calls under a FROM-root `OR`; and one with the deprecated `SELECT TOP` clause plus two literals whose source text differs from their canonical rendering (`1.50` → `1.5`, `"quoted"` → `'quoted'`). The openEHR result set names an unaliased column by its expression text, so `parse.LiteralExpr.Raw` keeps what was written while emission stays canonical. Pass your own query as the argument to walk that instead.

```bash
go run ./cmd/examples/aql-parse-structured
go run ./cmd/examples/aql-parse-structured "SELECT c FROM EHR e CONTAINS COMPOSITION c WHERE c/uid/value = \$id"
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

--- mixed SELECT list, function calls, FROM-root junction ---

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

--- deprecated SELECT TOP and literal source text ---

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

The type tags in parentheses (`(param)`, `(string)`, `(func)`, `(path)`) are added by the program to make the tree readable; the wire form of each value comes from `aql.FormatValue`, so quoting and escaping match what the emitter writes.

**What to copy into your app:** use `parse.ParseQuery(src)` when you need to look inside a caller-supplied query (highlight paths, swap a comparison value, audit alias bindings), and `errors.Is(err, aql.ErrIncompleteAST)` to branch on a shape the tree cannot hold. `Query.Emit()` turns the tree back into AQL for execution; it re-parses its own output first, so what it returns always parses. Type-switch over `parse.SelectExpr` / `aql.WhereExpr` / `aql.Value` and report an unrecognised case instead of panicking, because those sets grow. Check `From.Junction` before `From.Root`: a FROM-root junction leaves `Root` zero.

### lint-aql

**Purpose:** Check AQL queries for problems before they reach a server. The linter parses the text against the grammar the SDK accepts, then runs its checks in layers: syntax; shape (alias binding, parameter binding); FROM / CONTAINS against the openEHR Reference Model (RM) built into the SDK, which needs no template; path-shape and paging advisories over the query text, likewise template-free; and, when given a compiled OPT, the archetypes and paths against that template.

The program calls `validation.ValidateAQL`; the underlying building block is `openehr/aql/lint` (`LintString` / `Lint`), which has no transport or auth. A lint-clean query is **not** proven spec-conformant, nor guaranteed to execute. The CDR remains the authority on paths.

```bash
go run ./cmd/examples/lint-aql
go run ./cmd/examples/lint-aql path/to/template.opt
```

**Packages:** `openehr/aql`, `openehr/template`, `openehr/templatecompile`, `openehr/validation` (`openehr/aql/parse` and `openehr/aql/lint` run underneath `ValidateAQL`)

**Sample output:**

```text
template : vital_signs (vital_signs.opt)

== clean query ==
SELECT o/data[at0001]/events[at0006]/data[at0003]/items[at0004]/value/magnitude AS magnitude FROM EHR e CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v1] WHERE e/ehr_id/value = $ehr_id
result   : OK — no issues

== broken query ==
SELECT o FROM OBSERVATION o[openEHR-EHR-OBSERVATION.lab_result.v1] WHERE o/data/events/value/magnitude > $threshold
result   : not OK — 2 errors, 2 advisories
  [warning] aql_path_repeating_unpredicated (o/data/events/value/magnitude): segment "events" steps through the multi-valued HISTORY.events with no predicate; which occurrence is meant is engine-defined
  [warning] aql_select_no_alias (o): SELECT item 1 carries no AS alias; the result column's name is then engine-defined, and a stored-query contract depends on a stable one
  [error] aql_unbound_param (-): $threshold is referenced but not bound in Query.Parameters
  [error] aql_archetype_not_in_template (openEHR-EHR-OBSERVATION.lab_result.v1): archetype openEHR-EHR-OBSERVATION.lab_result.v1 is not in template vital_signs

== semantic finding (RM-impossible containment) ==
SELECT o FROM OBSERVATION o CONTAINS COMPOSITION c
result   : not OK — 1 error, 2 advisories
  [warning] aql_from_archetype (-): FROM/CONTAINS names no archetype, $param, VERSION, or EHR scope
  [error] aql_impossible_containment (COMPOSITION c): no containment route under the pinned RM connects OBSERVATION to COMPOSITION, so this CONTAINS can never match
  [warning] aql_select_no_alias (o): SELECT item 1 carries no AS alias; the result column's name is then engine-defined, and a stored-query contract depends on a stable one

== advisory only (OK, but not issue-free) ==
SELECT c FROM FOLDER f CONTAINS COMPOSITION c
result   : OK — no errors, 3 advisories
  [warning] aql_from_archetype (-): FROM/CONTAINS names no archetype, $param, VERSION, or EHR scope
  [warning] aql_containment_by_reference (COMPOSITION c): FOLDER reaches COMPOSITION only across a reference hop; whether that counts as containment is engine-specific, so verify this step against the target CDR
  [warning] aql_select_no_alias (c): SELECT item 1 carries no AS alias; the result column's name is then engine-defined, and a stored-query contract depends on a stable one
```

Four queries: a clean one; a broken one, where an archetype missing from the template and an unbound `$threshold` are errors while an unpredicated `events` step and a missing alias are advisories; a well-formed query that can never match, because under the RM an OBSERVATION never contains a COMPOSITION; and one that is OK yet not issue-free, because a FOLDER reaches a COMPOSITION only through a reference. `Result.OK` is false only when an error-severity issue is present, so an OK result can still carry warnings.

**What to copy into your app:** for CI or pre-flight checks call `lint.LintString(q, nil)` (syntax, shape and RM checks, no template needed); when you hold a compiled OPT, pass it via `lint.Options{Compiled: c}` (or call `validation.ValidateAQL`) to add the archetype and path checks. Dispatch on `Issue.Code` and treat only `Error`-severity issues as hard failures. Still read `Result.Issues`, not only `Result.OK`: OK means *no errors*, not *no issues*. Most of the portability codes and all of the path-shape codes are advisory (the last block above).

---

### compile-build-validate

**Purpose:** Drive the whole clinical pipeline through public packages only, as a program in another Go module would. Parse an OPT, compile it with `openehr/templatecompile.Compile`, build a `*rm.Composition` with the builder, serialise it to canonical JSON and decode it again, and validate the result against the same compiled template.

```bash
go run ./cmd/examples/compile-build-validate
go run ./cmd/examples/compile-build-validate path/to/template.opt
```

**Packages:** `openehr/template`, `openehr/templatecompile`, `openehr/composition`, `openehr/serialize/canjson`, `openehr/validation`, `openehr/rm`. **No `internal/` import.**

**Sample output:**

```text
template : vital_signs (vital_signs.opt)
composition: 3162 bytes canonical JSON, round-tripped
validation : OK — round-tripped composition conforms to the OPT
ehr_status : ValidateEHRStatus callable — 6 issue(s), root type mismatch as expected
```

The builder starts from a skeleton generated from the compiled template, with the mandatory structure already in place, so the program sets only the one leaf it cares about (the systolic value, addressed by its template path). The last line shows that the validator also has typed entry points for other RM roots (`ValidateEHRStatus`, with `ValidateFolder` and `ValidateDemographic` as siblings). An EHR_STATUS can never satisfy a template whose root is a COMPOSITION, so the issues and the root type mismatch are the expected result.

**What to copy into your app:** `templatecompile.Compile(opt)` once per template, then reuse the `*Compiled` across many `composition.NewBuilder` / `validation.Validate*` calls; the compiled template is the single artefact the builder and the validator share. Address leaves by template path with `SetQuantity`, `SetText`, `SetCodedText` or `Set`; the [template-explore](#template-explore) example prints every such path of a template.

---

### template-explore

**Purpose:** Walk a compiled OPT through its public introspection tree and print two views of it. The first is the node structure a form generator would render: RM type, archetype id or at-code, attribute cardinality, whether the Reference Model requires the attribute, the human label, and slot and primitive markers. The second is the list of primitive-leaf paths a composition builder can assign values to, which are the `composition.Builder.Set` targets.

```bash
go run ./cmd/examples/template-explore
go run ./cmd/examples/template-explore path/to/template.opt
```

**Packages:** `openehr/template`, `openehr/templatecompile`. **No `internal/` import.**

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

In the structure view, `[1]` marks a single-valued attribute and `[*]` a multi-valued one; `required` means the Reference Model makes the attribute mandatory on that type. The bracketed identity is the archetype id where an archetype is plugged in, otherwise the at-code from the archetype's own definition; a data value such as DV_QUANTITY has neither. `(slot)` marks an opaque fill point another archetype plugs into, and `·primitive` marks a node with a value constraint, the editable leaf. The quoted text is the term the archetype defines for the node's at-code.

**What to copy into your app:** hold `*templatecompile.CompiledNode` / `*templatecompile.CompiledAttribute` in your own walker. `node.RMTypeName()` plus `attr.Cardinality()` / `Required()` drive widget choice and required markers, and `node.Term(code, "")` gives the label. `node.PrimitiveConstraint()` marks the editable leaves, and `node.AQLPath()` yields the `Builder.Set` path.

---

### webtemplate-export

**Purpose:** Export a compiled OPT as a Web Template, the JSON form that EHRbase-style form renderers and FLAT-format mappers consume (the SDK follows the EHRbase `openEHR_SDK` v2.3 shape). The program prints a short summary (template id, version, default language, size of the JSON document) and the form-oriented tree (FLAT-path `id`, RM type, occurrences, input widgets); `-json` prints the full indented Web Template document instead.

```bash
go run ./cmd/examples/webtemplate-export
go run ./cmd/examples/webtemplate-export path/to/template.opt
go run ./cmd/examples/webtemplate-export -json path/to/template.opt
```

**Packages:** `openehr/template`, `openehr/templatecompile`, `openehr/template/webtemplate`. **No `internal/` import.**

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

Each line is one node as a form renderer reads it: the `id` is the segment a FLAT path is built from, then the RM type and the min..max occurrences (`*` for unbounded). After the dash come the inputs a data-entry client draws for a leaf, as `suffix:type` pairs; the suffix is the FLAT-path suffix the value is posted under (`|magnitude`, `|unit`, `|code`), and a coded input also reports how many codes its list offers. A hint to rerun with `-json` goes to stderr, so stdout stays the summary alone.

**What to copy into your app:** `webtemplate.Marshal(compiled)` for the bytes (`application/openehr.wt+json`), or `webtemplate.Build(compiled)` when you post-process the typed tree first. Each `Node.ID` is the FLAT-path segment consumers bind to, and each leaf's `Inputs` (`suffix` / `type` / `list` / `validation`) drives the widget. Both fail loudly (`ErrEmptyTemplate` / `ErrNoDefaultLanguage` / `ErrIDCollision`) instead of emitting ambiguous output. The package's `deviations.md` lists the accepted reference deltas.

---

### flat-roundtrip

**Purpose:** Convert a canonical COMPOSITION to the FLAT and STRUCTURED simplified formats and back. These formats address values by short Web Template ids instead of full RM paths, so converting between a COMPOSITION and FLAT or STRUCTURED needs the composition's Web Template. The program builds that from the OPT, encodes a vendored composition as FLAT, restructures it as STRUCTURED (no template needed for that step), decodes the FLAT back into a composition, and finally shows the template-aware decode (`WithTemplate`) whose result validates against the OPT. No transport or auth is involved.

```bash
go run ./cmd/examples/flat-roundtrip
```

**Packages:** `openehr/serialize/simplified`, `openehr/template`, `openehr/template/webtemplate`, `openehr/templatecompile`, `openehr/serialize/canjson`, `openehr/validation`, `openehr/rm`. **No `internal/` import.**

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

STRUCTURED (application/openehr.wt.structured+json): 1325 bytes

OK: FLAT -> COMPOSITION -> FLAT round-trips for Test_dv_quantity_open_constraint.v0
OK: WithTemplate decode validates against the OPT
```

Every FLAT key is a path of Web Template ids, with an optional `|suffix` naming the part of a value it carries (`|magnitude`, `|unit`, `|code`); composition-level metadata sits under `ctx/`. Without a compiled template the decode keeps exactly what the format carries, so encoding the result reproduces the first document key for key. The formats carry no node names and omit attributes the Reference Model requires (HISTORY.origin, EVENT.time, ...); `WithTemplate` restores the names from the compiled template and fills the other required attributes with synthesised defaults (from `ctx/` values and RM conventions, not recovered data), which is why only that decode validates against the OPT. It needs `ctx/time` in the input when the template has HISTORY or EVENT nodes; this fixture carries it.

**What to copy into your app:** build the Web Template once (`templatecompile.Compile` plus `webtemplate.Build`), then `simplified.MarshalFlat(comp, wt)` / `UnmarshalFlat(data, wt)` (and the `…Structured` pair) for template-driven conversion, or `FlatToStructured` / `StructuredToFlat` for template-free interconversion. Pass `simplified.WithTemplate(compiled)` to `Unmarshal*` when you need an OPT-validatable composition (names restored from the template, other RM-mandatory attributes synthesised as defaults) rather than a format-idempotent one. Composition-level metadata rides `ctx/`; decorated or exotic datatypes ride `|raw`. The codec is strict on decode: unknown paths or suffixes, wrong-typed ctx values, index games, and malformed input return an error instead of dropping data. See the package's `deviations.md`.

---

## REST client

### ehr_create

**Purpose:** Create an EHR through the SDK's REST client path. Three layers take part: a static service catalog says where the openEHR REST API lives, a transport client carries the injected `*http.Client`, and the typed `ehr.Create` call sends `POST /ehr` and decodes the answer. A throwaway `httptest` server plays the backend, so nothing needs a CDR. Every other REST call in the SDK is wired the same way.

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

The fake backend answers the way an openEHR server does: `201 Created`, a `Location` header for the new EHR, and the EHR itself in the body (because `ehr.Create` sends `Prefer: return=representation` by default). `Location` is the new resource's path as the server sent it; `VersionUID` is its last path segment, which for an EHR is the ehr_id and for versioned resources such as compositions is the version uid.

**What to copy into your app:**

1. Build a `discovery.ServiceCatalog` (static, or fetched from a SMART issuer).
2. `transport.New(catalog, transport.WithHTTPClient(yourClient))`; the SDK never allocates an `*http.Client`, so connection pooling, TLS and timeouts stay under your control.
3. Call leaf clients (`ehr.Create`, `query.Execute`, …) with a `context.Context`.

To hit a real backend, swap the catalog base URL and add `transport.WithTokenSource`. See [quick-start.md](quick-start.md#path-b--rest-client-live-or-mocked-backend).

---

### contribution-build

**Purpose:** Assemble a CONTRIBUTION with `contribution.Builder` and print the `Contribution_create` request body the builder produces. A CONTRIBUTION is the openEHR unit of commit: several versions written to one EHR in a single atomic request. Two vendored canonical compositions go in, one as a first version and one as an amendment of a version that already exists. `-commit` additionally POSTs the body through `contribution.Commit` to an in-process fake CDR and checks that the request it received is byte-identical to what was built.

```bash
go run ./cmd/examples/contribution-build
go run ./cmd/examples/contribution-build -commit
```

**Packages:** `openehr/client/ehr/contribution`, `openehr/client/ehr`, `openehr/rm`, `openehr/serialize/canjson`, `testkit/fixtures` (`smart/discovery` and `transport` are used only under `-commit`)

**Sample output** (body elided):

```text
batch audit change_type: creation (249) — declared, not derived
versions[0]: ORIGINAL_VERSION<COMPOSITION> change_type=creation/249 lifecycle_state=complete/532 preceding=(none — a first version)
versions[1]: ORIGINAL_VERSION<COMPOSITION> change_type=amendment/250 lifecycle_state=complete/532 preceding=8849182c-82ad-4088-a07f-48ead4180515::cdr.example::1
```

The indented JSON body is printed first; the summary lines pick out the fields worth noticing so you need not scan it. The builder derives each version's `change_type` from the operation (`Creation`, `Amendment`, ...) and copies the committer and system id from the batch audit into every version. The batch audit's own `change_type` describes the contribution as a whole and is never derived from the versions, so the caller declares it. Under `-commit`, two more lines follow:

```text
committed: 11889 bytes reached the wire; Location="/openehr/v1/ehr/f0e1d2c3-b4a5-6789-0123-456789abcdef/contribution/8849182c-82ad-4088-a07f-48ead4180515::cdr.example::1"
OK: the captured request body is byte-identical to the built body
```

**What to copy into your app:**

1. `contribution.NewBuilder()`, then declare the batch audit once: committer, system id, and the batch `change_type` (which is *never* derived from the versions).
2. Accumulate one `Change` per version: `contribution.Creation(payload)`, or `Amendment` / `Modification` / `Deletion` with the preceding version uid. Each is generic over the four versionable RM types, so a wrong payload type is a compile error.
3. `Build()` once. It returns a `*contribution.Submission` that has already passed `Validate`, or every accumulated error joined.
4. `contribution.Commit(ctx, client, ehrID, submission)`.

Per-version overrides (`WithLifecycleState`, `WithVersionCommitter`, `WithVersionDescription`, `WithVersionSystemID`, `WithVersionUID`) refine what a version would otherwise inherit from the batch audit.

---

### smart-launch

**Purpose:** Walk through a standalone SMART-on-openEHR launch for a public client: the OAuth 2.0 authorization-code flow with PKCE, which is how a browser-based or native app that cannot keep a client secret obtains an access token. The program plays every party in turn (the app, the user's browser, and the authorization server) against an in-process stub, so it needs no account, no secret and no network. `go test ./cmd/examples/smart-launch` runs the same flow.

The one thing to take away is where the `AuthorizationRequest` lives. It carries the CSRF `state` and the PKCE `code_verifier`, both created before the redirect and both needed after it, so your app has to store it across the redirect and look it up again on the callback. Everything else is one SDK call per step.

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
  access_token : stub-acc…
  token_type   : Bearer
  scope        : openid launch/patient offline_access
  expires_at   : …
  refresh_token: stub-ref…
  ehrId        : 00000000-0000-0000-0000-000000000001
OK: standalone SMART PKCE launch flow completed (in-process stub)
```

The `state`, the verifier and `expires_at` differ on every run (shown as `…` above), so this block is not compared verbatim. The authorize URL carries the client id, the redirect URI, the scopes, the state and the PKCE challenge, never the verifier. The stub skips the login screen and grants at once; a real server also hashes the `code_verifier` it receives on the token endpoint against the `code_challenge` it saw on `/authorize`, which is the PKCE proof.

**What to copy into your app:**

1. Call `BeginAuthorization("")` to get an `AuthorizationRequest` with a random `state` and PKCE pair.
2. Persist the `AuthorizationRequest` in a session store keyed by `state` **before** redirecting the user.
3. On the redirect callback, retrieve the stored `AuthorizationRequest` by `callbackState` and delete it in the same operation (the program's `sessionStore.take`), so a replayed callback finds nothing; then pass it to `ExchangeAuthorizationCode`.
4. `ExchangeAuthorizationCode` re-validates `state` internally (CSRF guard) and sends the `code_verifier` to the token endpoint (PKCE proof).

See [specifications/auth.md § PKCE flow](specifications/auth.md#req-061--pkce-flow) for the normative rules.

---

## Suggested learning order

```text
1. canonical_json          ← RM + canjson basics
2. opt-parse               ← understand templates and paths
3. validate-from-json      ← wire bytes + validation (CI pattern)
4. generate-example        ← generate data from templates
5. ehr_create              ← REST wiring (mock first, then real CDR)
6. smart-launch            ← SMART PKCE auth (standalone, public client)
```

Optional depth: `canxml_roundtrip` (multi-format), `primitive-validate` (leaf constraints), `validate-composition` (in-memory RM construction), `contribution-build` (batched atomic writes).

---

## Fixtures and testkit

Examples depend on [`testkit/fixtures`](../testkit/fixtures/) and cassettes under `testkit/cassettes/`. These are stable, checked-in artefacts, not generated at runtime. The exception is `validate-from-json/testdata/`, produced once via `gen_fixture.go`.

When writing your own tests, prefer importing fixtures from `testkit` rather than copying paths by hand.

---

## Maintaining this catalog

Agents and contributors: when you add or materially change an example under `cmd/examples/`, update this file, [`cmd/examples/doc.go`](../cmd/examples/doc.go), and [`quick-start.md`](quick-start.md) (if onboarding changes) in the **same PR**. Checklist: [ai-workflow.md § Examples](ai-workflow.md#examples).

[`cmd/examples/transcripts_test.go`](../cmd/examples/transcripts_test.go) checks the **Sample output:** blocks named in its allowlist against real program runs. That allowlist alone decides which blocks are checked, and its exclusion census accounts for every other example. A deliberate output change therefore means regenerating the block verbatim from `go run ./cmd/examples/<name>`, not editing it by hand.

An example that gains a verbatim sample-output block must be added to that allowlist in the same PR. `TestSampleMarkerCensus` fails the build if a section publishes a bare sample-output marker without an allowlist entry, so an unlisted block cannot go unverified. The one recorded exception is smart-launch, whose PKCE state, verifier, and `expires_at` differ every run: `bareMarkerException` in that test names it, and the test also fails if that exception ever outlives the marker it excuses.

---

## Related documentation

- [quick-start.md](quick-start.md): install, idioms, REST wiring
- [architecture.md](architecture.md): package map and dependency rules
- [specifications/use-cases.md](specifications/use-cases.md): benchmark, seeder, MCP, federator consumers
- [roadmap.md](roadmap.md): what is landed vs planned
