# Plan — FLAT / STRUCTURED simplified-format codecs (REQ-053, umbrella Phase 3)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Date:** 2026-07-14
**Status:** Done — all Phase 3 tasks executed and archived. REQ-053 subsequently reached `landed` (PR #76): `WithTemplate` decode repopulates `LOCATABLE.name` + completes the RM-mandatory attributes FLAT omits, so a decoded composition validates against the OPT. Residual deferrals (exotic `ctx/` fields on encode, `.schema` media types, an upstream byte-conformance probe) are carried by the [umbrella](../2026-06-23-simplified-formats.md) + the package `deviations.md`.
**Owner:** SDK maintainers
**Covers:** [REQ-053](../../specifications/wire.md#req-053) (FLAT / STRUCTURED codecs — registry row in [REQ.md](../../specifications/REQ.md))
**Probes:** PROBE-076 (FLAT/STRUCTURED composition round-trip) — **to register** in [conformance.md](../../specifications/conformance.md) at Task in Phase 7.
**Umbrella:** [simplified-formats umbrella](../2026-06-23-simplified-formats.md) — this is **Phase 3**; Phase 1 (shared model) is realised by the shipped `openehr/template/webtemplate`, Phase 2 (REQ-106 WebTemplate export) landed.
**Decisions:** reference implementation & id-generation are locked by [ADR-0014](../../adr/0014-webtemplate-reference-implementation-lock.md) (reused, not re-decided). The **output-form fork** (spec `ctx/` short-form vs EHRbase full-path context) is a documented deviation, not a new ADR (see Global Constraints); escalate to an ADR only if a consumer needs the EHRbase form emitted by default.
**Implementation:** landed (core codec + ctx/ + `|raw`/`|other` + full datatype set + PROBE-076 + `WithTemplate` name/RM-mandatory completion → OPT-validatable decode; exotic `ctx/` fields on encode, `.schema` media types, and an upstream byte-conformance probe deferred — see `openehr/serialize/simplified/deviations.md`)
**Depends on:** landed `openehr/template/webtemplate` (REQ-106) — the shared WT node model; `openehr/templatecompile` (REQ-111); `openehr/rm` + `openehr/rm/rmpath` (path resolution) + `openehr/rm/rminfo` (RM-attribute/type oracle); `openehr/serialize/canjson` (canonical fragments for `|raw`). All shipped.
**Defers:** WebTemplate/OPT reconstruction from a data instance (category error / lossy — out of scope, see REQ-053 + umbrella); REST content-negotiation wiring (client-layer follow-up); exotic datatypes not in the conformance corpus (emit/accept `|raw`, list in `deviations.md`); the Better camelCase id variant; multi-composition FLAT batches.

## Goal

Ship `openehr/serialize/simplified` — codecs that convert an `*rm.Composition` to and from the openEHR **FLAT** and **STRUCTURED** *Simplified Formats* (the two variants named by the STABLE spec; the legacy simSDT/structSDT/SDT naming is superseded and stays out of the public surface), driven by the composition's **Web Template** (REQ-106). Bidirectional and **semantics-preserving** given the OPT (REQ-053); building-block-independent (REQ-013); structural conformance against a vendored upstream trio (PROBE-076), parity documented-not-byte-exact.

## Definition of Ready

- [x] `**Covers:**` lists the REQ (REQ-053).
- [x] Canonical normative prose exists and is correct — [wire.md § REQ-053](../../specifications/wire.md#req-053) (rewritten 2026-07-14 to the real Flat/Structured grammar) + REQ.md registry row.
- [x] Reference implementation + id-generation locked by [ADR-0014](../../adr/0014-webtemplate-reference-implementation-lock.md); the output-form deviation is recorded here rather than re-litigated.
- [x] Phases list concrete tasks and name the verification command (`go test`, `make spec-check`, `make ci`, `make probe-status`).

## Global Constraints

Every task's requirements implicitly include these:

- **Building-block independence (REQ-013):** `openehr/serialize/simplified` imports only `openehr/rm`, `openehr/rm/rmpath`, `openehr/rm/rminfo`, `openehr/rm/typereg`, `openehr/template/webtemplate`, `openehr/serialize/canjson`, and the standard library. It **MUST NOT** import `transport/`, `auth/`, `openehr/client/*`, or `cadasto/…`. A guard test enforces this.
- **No reflection (REQ-024):** RM datatype dispatch is an explicit type switch on concrete `rm` types (encode) and on the WT leaf `rmType`/`Inputs` (decode) — never `reflect`.
- **Determinism:** identical `(comp, wt)` ⇒ byte-identical FLAT/STRUCTURED output across runs and patch releases. FLAT keys are emitted in sorted order; `encoding/json` sorts map keys.
- **Media types:** emit `application/openehr.wt.flat+json` / `application/openehr.wt.structured+json` only; accept EHRbase's `.schema`-suffixed variants on input; never emit them (REQ-053).
- **Context output form (deviation):** emit composition metadata under the spec `ctx/` short-form on write and read; accept the EHRbase full-path form (`<root>/language|code`, `<root>/context/start_time`) on input. Record in `deviations.md`.
- **Go floor:** do **not** touch the `go.mod` `go` directive; it stays the repo's existing minor `.0` floor.
- **Citations:** production code and tests carry `// REQ-053` (and `// PROBE-076` for the conformance harness) comments.
- **Formatting / tests:** `make fmt` before commits; table-driven tests, real code (no mocks); conformance/round-trip tests load fixtures rather than hard-coding reference values.

## Definition of Done

- [x] Code + tests land with `// REQ-053` / `// PROBE-076` citations; `openehr/serialize/simplified` exports `MarshalFlat`/`UnmarshalFlat`/`MarshalStructured`/`UnmarshalStructured`/`FlatToStructured`/`StructuredToFlat` + the media-type constants + typed errors.
- [x] Round-trip holds: `Unmarshal*(Marshal*(comp, wt), wt)` reproduces the FLAT/STRUCTURED for the covered corpus; `FlatToStructured`/`StructuredToFlat` invert each other (semantically — `:index` normalisation, see deviations.md). **Format-idempotent**, not canonically equal: `LOCATABLE.name` repopulation on decode is deferred (deviations.md), so REQ-053 lands `partial`.
- [x] PROBE-076 registered in `conformance.md` and implemented against the vendored EHRbase `Test_dv_*` corpus (OPT + canonical); deviations recorded in `deviations.md`. Scope: round-trip idempotence, not upstream byte-conformance (documented follow-up).
- [x] [`traceability.yaml`](../../specifications/traceability.yaml) REQ-053 `packages:`/`tests:`/`probes:`/`plans:` populated; `implementation: partial` with the covered-types note; REQ.md **Impl.** column `partial`; wire.md mapping-table row names the package.
- [x] `make spec-check` and `make ci` pass.
- [~] Plan **Status: Done** and archived via `sdd-archive`; plans/README + roadmap updated. — roadmap/README updated; archive on PR #76 merge (the one remaining in-scope deferral, `LOCATABLE.name`, is tracked by the umbrella + deviations.md).

## Implementation checklist

| Step | Status |
|---|---|
| Package + public API + media types + independence guard | ☑ |
| WT index engine (id-path ↔ node, repeatables, ctx) | ☑ (folded into encode/decode; standalone index removed as unused) |
| FLAT encode — core datatypes | ☑ |
| FLAT decode — core datatypes (canonical RM rebuild) | ☑ |
| STRUCTURED encode/decode + FLAT↔STRUCTURED | ☑ |
| ctx / `_`-attrs / `\|raw` / `\|other` | ☑ (`_`-attrs carried losslessly via `\|raw`; first-class suffix decomposition deferred) |
| PROBE-076 conformance corpus vendored + registered | ☑ (EHRbase `Test_dv_*`; idempotence) |
| traceability / REQ.md / wire.md row / deviations | ☑ |
| `make spec-check` + `make ci` | ☑ |

---

## File Structure

- `openehr/serialize/simplified/doc.go` — package doc: what FLAT/STRUCTURED are (data instances), OPT-dependence, the `ctx/` deviation pointer.
- `openehr/serialize/simplified/simplified.go` — public API: `MarshalFlat`/`UnmarshalFlat`/`MarshalStructured`/`UnmarshalStructured`/`FlatToStructured`/`StructuredToFlat`, media-type consts, typed errors.
- `openehr/serialize/simplified/index.go` — the WT walker: builds an index from FLAT id-path ↔ `*webtemplate.Node` (with `:index` and `ctx` classification), the shared engine both directions use.
- `openehr/serialize/simplified/flat_encode.go` — `*rm.Composition` → FLAT map (drives the WT index, resolves values via `rmpath`).
- `openehr/serialize/simplified/flat_decode.go` — FLAT map → `*rm.Composition` (rebuilds canonical RM from leaf `aqlPath` via `rminfo`).
- `openehr/serialize/simplified/structured.go` — STRUCTURED ↔ FLAT restructuring (no OPT) + the structured Marshal/Unmarshal wrappers.
- `openehr/serialize/simplified/datatypes.go` — per-RM-datatype leaf mapping (DV_* ↔ `|suffix` values), an explicit type switch.
- `openehr/serialize/simplified/*_test.go` — unit + round-trip + conformance tests.
- `openehr/serialize/simplified/deviations.md` — the documented-deviations list.
- `openehr/serialize/simplified/testdata/` — SDK round-trip goldens.
- `testkit/cassettes/simplified/<trio>.{opt,flat.json,structured.json,canonical.json}` — vendored upstream conformance trio (license/provenance in `testkit/cassettes/THIRD_PARTY_LICENSES.md`).

## Phases

### Phase 1 — Package skeleton, API surface, independence guard

#### Task 1: Scaffold package, public API, media types, guard

**Files:**
- Create: `openehr/serialize/simplified/doc.go`, `openehr/serialize/simplified/simplified.go`
- Test: `openehr/serialize/simplified/independence_test.go`

**Interfaces:**
- Produces: `const MediaTypeFlat = "application/openehr.wt.flat+json"`, `const MediaTypeStructured = "application/openehr.wt.structured+json"`; `func MarshalFlat(comp *rm.Composition, wt *webtemplate.WebTemplate) ([]byte, error)`; `func UnmarshalFlat(data []byte, wt *webtemplate.WebTemplate) (*rm.Composition, error)`; `MarshalStructured`/`UnmarshalStructured` (same signatures); `func FlatToStructured(data []byte) ([]byte, error)`; `func StructuredToFlat(data []byte) ([]byte, error)`; `var ErrNoTemplate`, `var ErrUnknownPath`, `var ErrMissingContext error`.

- [ ] **Step 1: Confirm the `rm.Composition` accessor surface.** Run: `go doc ./openehr/rm Composition` and `go doc ./openehr/rm` (note the composition type name, content/context/category fields, and how `openehr/rm/rmpath` resolves an aqlPath — `go doc ./openehr/rm/rmpath`). Record the exact names for use in later tasks (the illustrative code below assumes `rm.Composition`; adjust if the type lives elsewhere).

- [ ] **Step 2: Write the independence guard test (RED).**

```go
// openehr/serialize/simplified/independence_test.go
package simplified_test

// REQ-013 — building-block independence.
import (
	"go/build"
	"strings"
	"testing"
)

func TestBuildingBlockIndependence(t *testing.T) {
	pkg, err := build.Import("github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified", "", 0)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	forbidden := []string{"/transport", "/auth", "/openehr/client", "/cadasto"}
	for _, imp := range pkg.Imports {
		for _, f := range forbidden {
			if strings.Contains(imp, f) {
				t.Errorf("forbidden import %q (matches %q)", imp, f)
			}
		}
	}
}
```

- [ ] **Step 3: Run it — expect FAIL** (no Go files in package). Run: `go test ./openehr/serialize/simplified/ -run TestBuildingBlockIndependence`. Expected: FAIL — `no buildable Go source files`.

- [ ] **Step 4: Write `doc.go` + `simplified.go` with the public API as stubs.**

```go
// openehr/serialize/simplified/doc.go
// Package simplified converts an openEHR COMPOSITION to and from the FLAT
// and STRUCTURED Simplified Formats (REQ-053).
//
// These are serializations of a data instance, not a template: conversion
// to/from canonical RM is template-specific and requires the composition's
// Web Template (REQ-106). The conversion is bidirectional and
// semantics-preserving given the template; the simplified forms are not
// self-standing. Context-form and exotic-datatype deviations are in
// deviations.md.
package simplified
```

```go
// openehr/serialize/simplified/simplified.go
package simplified

import (
	"errors"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
)

// Media types (REQ-053). Emit these; accept EHRbase's `.schema`-suffixed
// variants on input only.
const (
	MediaTypeFlat       = "application/openehr.wt.flat+json"
	MediaTypeStructured = "application/openehr.wt.structured+json"
)

var (
	// ErrNoTemplate is returned when a nil Web Template is passed to a
	// conversion that needs one.
	ErrNoTemplate = errors.New("simplified: nil web template")
	// ErrUnknownPath is returned when a FLAT/STRUCTURED key does not resolve
	// to a Web Template node.
	ErrUnknownPath = errors.New("simplified: path not in web template")
	// ErrMissingContext is returned when mandatory context (language,
	// territory) is absent and cannot be defaulted.
	ErrMissingContext = errors.New("simplified: missing mandatory context")
)

// MarshalFlat encodes comp as FLAT JSON using wt (REQ-053). Implemented in
// flat_encode.go; stub here so the package compiles.
func MarshalFlat(comp *rm.Composition, wt *webtemplate.WebTemplate) ([]byte, error) {
	return nil, ErrNoTemplate
}

// UnmarshalFlat decodes FLAT JSON into a canonical COMPOSITION using wt.
func UnmarshalFlat(data []byte, wt *webtemplate.WebTemplate) (*rm.Composition, error) {
	return nil, ErrNoTemplate
}

// MarshalStructured / UnmarshalStructured — structured.go (Task 5).
func MarshalStructured(comp *rm.Composition, wt *webtemplate.WebTemplate) ([]byte, error) {
	return nil, ErrNoTemplate
}

func UnmarshalStructured(data []byte, wt *webtemplate.WebTemplate) (*rm.Composition, error) {
	return nil, ErrNoTemplate
}

// FlatToStructured / StructuredToFlat — structured.go (Task 5); no OPT needed.
func FlatToStructured(data []byte) ([]byte, error) { return nil, ErrUnknownPath }
func StructuredToFlat(data []byte) ([]byte, error) { return nil, ErrUnknownPath }
```

- [ ] **Step 5: Run the guard test — expect PASS.** Run: `go test ./openehr/serialize/simplified/ -run TestBuildingBlockIndependence`. Expected: PASS.

- [ ] **Step 6: Commit.**

```bash
make fmt
git add openehr/serialize/simplified/
git commit -m "feat(simplified): scaffold FLAT/STRUCTURED codec package + API (REQ-053)"
```

### Phase 2 — WT index engine (shared by both directions)

#### Task 2: Build the FLAT-path ↔ node index from the Web Template

The engine both codecs share: walk `wt.Tree`, and for every node produce its **FLAT id-path** (parent id-chain joined by `/`, rooted at `wt.Tree.ID`), noting which nodes are repeatable (`Max != 1`) — those take a `:index` — and which are context (`ctx/`) nodes (COMPOSITION/EVENT_CONTEXT `inContext` leaves: `language`, `territory`, `composer`, `setting`, `start_time`/`time`). Each entry keeps the node's `AQLPath` (canonical RM path) and `RMType` for the RM-rebuild direction.

**Files:**
- Create: `openehr/serialize/simplified/index.go`
- Test: `openehr/serialize/simplified/index_test.go`

**Interfaces:**
- Consumes: `webtemplate.WebTemplate.Tree`, `webtemplate.Node.{ID,AQLPath,RMType,Min,Max,Inputs,Children}`.
- Produces: `type wtEntry struct { flatPath, aqlPath, rmType string; repeatable, isContext bool; node *webtemplate.Node }`; `func indexTemplate(wt *webtemplate.WebTemplate) (map[string]*wtEntry, error)` keyed by flatPath **without** `:index` (indices are matched at value time); `func contextIDs(wt *webtemplate.WebTemplate) map[string]bool`.

- [ ] **Step 1: Write the index test (RED)** against the vendored trio's OPT (Task 8 vendors it; until then use any `testkit/cassettes/templates/*.opt`). Assert the index contains a known leaf's flatPath (e.g. `.../systolic`) mapping to an entry whose `aqlPath` ends in `/value` and `rmType` is a `DV_*`, and that a repeating node is flagged `repeatable`.

```go
// openehr/serialize/simplified/index_test.go
package simplified

// REQ-053 — WT index engine.
import (
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

func buildWT(t *testing.T, optPath string) *webtemplate.WebTemplate {
	t.Helper()
	opt, err := template.ParseFile(optPath)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	wt, err := webtemplate.Build(c)
	if err != nil {
		t.Fatalf("build wt: %v", err)
	}
	return wt
}

func TestIndexHasLeafEntries(t *testing.T) {
	wt := buildWT(t, "<PICK a small OPT under testkit/cassettes/templates>")
	idx, err := indexTemplate(wt)
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	var sawValueLeaf bool
	for _, e := range idx {
		if strings.HasSuffix(e.aqlPath, "/value") && strings.HasPrefix(e.rmType, "DV_") {
			sawValueLeaf = true
		}
	}
	if !sawValueLeaf {
		t.Error("index has no DV_* value leaf")
	}
}
```

- [ ] **Step 2: Run it — expect FAIL** (`indexTemplate` undefined). Run: `go test ./openehr/serialize/simplified/ -run TestIndexHasLeafEntries`.

- [ ] **Step 3: Implement `index.go`** — a recursive walk building `flatPath = parentFlatPath + "/" + node.ID` (root = `wt.Tree.ID`), recording `aqlPath`, `rmType`, `repeatable = node.Max != 1`, and `isContext` via the context-id set (Task 6 refines the ctx classification; here use a static set: `language`,`territory`,`composer`,`setting`,`start_time`,`time`,`subject`,`encoding`). Return the map keyed by flatPath.

- [ ] **Step 4: Run the test — expect PASS.** Commit.

```bash
make fmt
git add openehr/serialize/simplified/index.go openehr/serialize/simplified/index_test.go
git commit -m "feat(simplified): Web Template FLAT-path index engine (REQ-053)"
```

### Phase 3 — FLAT encode (core datatypes)

#### Task 3: Encode `*rm.Composition` → FLAT map

Drive the WT index; for each leaf entry resolve its `aqlPath` against `comp` via `openehr/rm/rmpath` (handling repeatables by enumerating instances and stamping `:index`); map the resolved `DV_*` value to its `|suffix` key(s) via `datatypes.go`.

**Files:**
- Create: `openehr/serialize/simplified/flat_encode.go`, `openehr/serialize/simplified/datatypes.go`
- Modify: `openehr/serialize/simplified/simplified.go` (replace `MarshalFlat` stub)
- Test: `openehr/serialize/simplified/flat_encode_test.go`

**Interfaces:**
- Consumes: `rmpath.Resolve(comp, aqlPath)` (confirm the exact signature via `go doc ./openehr/rm/rmpath`); concrete `rm` datatypes (`rm.DvQuantity`, `rm.DvText`, `rm.DvCodedText`, `rm.DvDateTime`, `rm.DvCount`, … — confirm names via `go doc ./openehr/rm`).
- Produces: `func encodeFlat(comp *rm.Composition, wt *webtemplate.WebTemplate) (map[string]any, error)`; `func leafToFlat(dst map[string]any, flatKey string, value any) error` (the datatype type switch).

- [ ] **Step 1: Write the encode test (RED)** — compile a small OPT, hand-build (or decode from a canonical JSON fixture via `canjson`) a minimal `*rm.Composition` with one DV_QUANTITY and one DV_TEXT leaf, `MarshalFlat`, and assert the expected `<root>/.../<leaf>|magnitude` / `<leaf>|unit` and free-text keys are present with the right values.

- [ ] **Step 2: Run it — expect FAIL** (`MarshalFlat` stub returns `ErrNoTemplate`).

- [ ] **Step 3: Implement `flat_encode.go` + `datatypes.go`.** `encodeFlat` walks the WT index (deterministic order), resolves each leaf value via `rmpath`, and calls `leafToFlat`. `datatypes.go` is an explicit `switch v := value.(type)` over concrete `rm` datatypes → suffix keys (DV_QUANTITY→`|magnitude`/`|unit`; DV_TEXT→bare; DV_CODED_TEXT→`|code`/`|value`/`|terminology`; DV_DATE_TIME→bare; DV_COUNT→`|magnitude`; DV_BOOLEAN→`|value`; DV_ORDINAL→`|code`/`|value`/`|ordinal`; others → `|raw` via `canjson`, recorded in `deviations.md`). Wire `MarshalFlat` = `json.Marshal(encodeFlat(...))`. Handle repeatables by iterating the resolved collection and stamping `:index`.

- [ ] **Step 4: Run the test — expect PASS.** Commit.

```bash
make fmt
git add openehr/serialize/simplified/flat_encode.go openehr/serialize/simplified/datatypes.go openehr/serialize/simplified/simplified.go openehr/serialize/simplified/flat_encode_test.go
git commit -m "feat(simplified): FLAT encode for core datatypes (REQ-053)"
```

### Phase 4 — FLAT decode (canonical RM rebuild)

#### Task 4: Decode FLAT map → `*rm.Composition`

For each FLAT key: split off `:index` and `|suffix`, resolve the flatPath (index-stripped) to a WT entry, take its `aqlPath`, and **construct/locate** the RM object at that canonical path — creating the elided wrapper containers (HISTORY, ITEM_TREE, EVENT) and the ELEMENT from the RM-type oracle `openehr/rm/rminfo`, and setting the `DV_*` leaf value from the suffix(es).

**Files:**
- Create: `openehr/serialize/simplified/flat_decode.go`
- Modify: `openehr/serialize/simplified/simplified.go` (replace `UnmarshalFlat` stub)
- Test: `openehr/serialize/simplified/flat_decode_test.go`, and a **round-trip** test.

**Interfaces:**
- Consumes: `rminfo` attribute/type lookup (confirm the accessor via `go doc ./openehr/rm/rminfo`) to learn the RM type of each aqlPath segment's attribute (e.g. `OBSERVATION.data`→`HISTORY`, `HISTORY.events`→`EVENT`, `EVENT.data`→`ITEM_TREE`, `ITEM_TREE.items`→`ELEMENT`/`CLUSTER`); `openehr/rm/typereg` to instantiate typed nodes; `canjson` for `|raw`.
- Produces: `func decodeFlat(m map[string]any, wt *webtemplate.WebTemplate) (*rm.Composition, error)`; `func flatFromSuffix(rmType string, parts map[string]any) (any, error)` (inverse of `leafToFlat`).

- [ ] **Step 1: Write the round-trip test (RED)** — `UnmarshalFlat(MarshalFlat(comp, wt), wt)` equals `comp` (compare via canonical JSON bytes from `canjson`, ignoring incidental attribute order).

- [ ] **Step 2: Run it — expect FAIL** (`UnmarshalFlat` stub).

- [ ] **Step 3: Implement `flat_decode.go`.** Group FLAT keys by leaf flatPath+index; for each, walk the leaf's `aqlPath` from the (single) COMPOSITION root, creating-or-descending each attribute step (attribute name → child RM type from `rminfo`; predicate `[atNNNN]`/`[archetype-id]` → `archetype_node_id`; repeated segments keyed by `:index`), terminating in an ELEMENT whose `value` is built by `flatFromSuffix`. Set composition-level `language`/`territory`/`category`/`composer`/`context` from ctx keys. Wire `UnmarshalFlat` = `decodeFlat(json.Unmarshal(...))`.

- [ ] **Step 4: Run the round-trip test — expect PASS.** Commit.

```bash
make fmt
git add openehr/serialize/simplified/flat_decode.go openehr/serialize/simplified/simplified.go openehr/serialize/simplified/flat_decode_test.go
git commit -m "feat(simplified): FLAT decode + canonical rebuild + round-trip (REQ-053)"
```

### Phase 5 — STRUCTURED + interconversion

#### Task 5: STRUCTURED codecs and FLAT↔STRUCTURED

STRUCTURED is FLAT re-nested: split each FLAT key on `/`, nest under each segment `id`, wrap every data value in a one-element array, and render `|suffix` as `|`-keys; `ctx/` groups under a `ctx` object (spec §Conversion Between Formats). This needs **no** OPT.

**Files:**
- Create: `openehr/serialize/simplified/structured.go`
- Modify: `openehr/serialize/simplified/simplified.go` (replace the 4 structured/interconv stubs)
- Test: `openehr/serialize/simplified/structured_test.go`

**Interfaces:**
- Produces: `func flatToStructured(flat map[string]any) map[string]any`; `func structuredToFlat(structured map[string]any) map[string]any`; wiring for `MarshalStructured` (= `flatToStructured(encodeFlat(...))`), `UnmarshalStructured` (= `decodeFlat(structuredToFlat(...))`), and the public `FlatToStructured`/`StructuredToFlat` byte APIs.

- [ ] **Step 1: Write the tests (RED)** — (a) `StructuredToFlat(FlatToStructured(flatBytes))` round-trips to the same FLAT map; (b) `UnmarshalStructured(MarshalStructured(comp, wt), wt)` equals `comp`.

- [ ] **Step 2: Run — expect FAIL** (stubs).

- [ ] **Step 3: Implement `structured.go`** per the spec's Flat↔Structured algorithms (arrays throughout; `ctx` object; `|`-prefixed suffix keys; instance indices preserved in property names). Wire the four public functions.

- [ ] **Step 4: Run — expect PASS.** Commit.

```bash
make fmt
git add openehr/serialize/simplified/structured.go openehr/serialize/simplified/simplified.go openehr/serialize/simplified/structured_test.go
git commit -m "feat(simplified): STRUCTURED codecs + FLAT<->STRUCTURED (REQ-053)"
```

### Phase 6 — context, RM attributes, raw, other

#### Task 6: `ctx/`, `_`-prefixed RM attributes, `|raw`, `|other`

**Files:**
- Modify: `openehr/serialize/simplified/{flat_encode,flat_decode,datatypes,index}.go`
- Create: `openehr/serialize/simplified/deviations.md`
- Test: `openehr/serialize/simplified/context_test.go`

- [ ] **Step 1: Write tests (RED)** for: mandatory `ctx/language`+`ctx/territory` present on encode and required on decode (`ErrMissingContext` when absent); an optional RM attribute round-trips via `_uid` / `_normal_range/...`; a `|raw` fragment (carrying `_type`) decodes into the canonical value and re-encodes; a `DV_CODED_TEXT` open-list free-text value round-trips via `|other`.
- [ ] **Step 2: Run — expect FAIL.**
- [ ] **Step 3: Implement** the `ctx/` emit/accept (with the full-path acceptance deviation), the `_`-prefix optional-RM-attribute path handling, the `|raw` bypass via `canjson`, and the `|other` open-value-set branch (spec §"Open Value-Sets and the `|other` Suffix"). Record the context output-form choice and any exotic-type `|raw` fallbacks in `deviations.md`.
- [ ] **Step 4: Run — expect PASS.** Commit.

```bash
make fmt
git add openehr/serialize/simplified/ 
git commit -m "feat(simplified): ctx, _-attrs, |raw, |other handling (REQ-053)"
```

### Phase 7 — conformance (PROBE-076)

#### Task 7: Vendor the upstream trio and register PROBE-076

**Files:**
- Create: `testkit/cassettes/simplified/<trio>.{opt,flat.json,structured.json,canonical.json}`
- Modify: `testkit/cassettes/THIRD_PARTY_LICENSES.md`, `docs/specifications/conformance.md` (register PROBE-076)
- Test: `openehr/serialize/simplified/conformance_test.go`

- [ ] **Step 1: Register PROBE-076** in `conformance.md` (heading `#### PROBE-076 — FLAT/STRUCTURED composition round-trip`, Status: Implemented, pointing at the conformance test) and add it to the clinical-modeling/wire probe index row. (Run `make spec-check` after Task in Phase 8 wires traceability.)
- [ ] **Step 2: Vendor a matched trio.** Prefer EHRbase `openEHR_SDK` `corona_anamnese` (OPT + webtemplate + `composition/flat/simSDT` + a canonical) at the `openehr-kb`-pinned commit `22b01e0c…`, or the Better `Vital Signs` trio. If `templatecompile` rejects the OPT (archetype-reuse-under-slot, per ADR-0014), fall back to a simpler upstream trio (e.g. a single-observation template) and record the substitution. Append provenance/license to `THIRD_PARTY_LICENSES.md`.
- [ ] **Step 3: Write the conformance test (RED/skip-aware)** — load the trio; assert `MarshalFlat(canonical, wt)` structurally equals the vendored FLAT (modulo `deviations.md`); same for STRUCTURED; and `UnmarshalFlat(vendored flat, wt)` canonically equals the vendored canonical.
- [ ] **Step 4: Iterate to green** — triage each diff: genuine bug → fix; accepted incidental (context form, field ordering, exotic `|raw`) → add to `deviations.md`.
- [ ] **Step 5: Commit.**

```bash
make fmt
git add testkit/cassettes/simplified/ testkit/cassettes/THIRD_PARTY_LICENSES.md docs/specifications/conformance.md openehr/serialize/simplified/conformance_test.go openehr/serialize/simplified/deviations.md
git commit -m "test(simplified): PROBE-076 round-trip conformance vs upstream trio (REQ-053)"
```

### Phase 8 — close-out

#### Task 8: Traceability, statuses, gates, example

**Files:**
- Modify: `docs/specifications/traceability.yaml`, `docs/specifications/REQ.md`, `docs/specifications/wire.md` (mapping-table row), `docs/roadmap.md`, `docs/plans/README.md`
- Optional: `cmd/examples/flat-roundtrip/` + `docs/examples.md` (a runnable FLAT round-trip demo, per the examples checklist)

- [ ] **Step 1: Populate REQ-053 traceability** — `packages: [openehr/serialize/simplified]`, `tests:` (all new `*_test.go`), `probes: [PROBE-076]`, `plans:` (this plan); flip `implementation: planned` → `landed` (or `partial` with a covered-types note if breadth is deferred).
- [ ] **Step 2: Flip REQ.md Impl.** column REQ-053 accordingly.
- [ ] **Step 3: Name the package** in the wire.md mapping-table row (replace `openehr/serialize/` (deferred sub-packages) with `openehr/serialize/simplified/`).
- [ ] **Step 4: `make spec-check`** — expect `spec-check: OK`.
- [ ] **Step 5: `make ci`** — expect all green.
- [ ] **Step 6: Update `docs/roadmap.md` + `docs/plans/README.md`**; set this plan **Status: Done** (archive via `sdd-archive`).
- [ ] **Step 7: Commit.**

```bash
git add docs/ cmd/ 
git commit -m "docs(simplified): land REQ-053/PROBE-076 traceability + statuses"
```

## Self-review (author)

- **Spec coverage:** REQ-053 §grammar (WT-id paths, `:index`, `|suffix`, level-removal) → Tasks 2–3; §context `ctx/` → Task 6; §`_`-attrs/`|raw` → Task 6; §round-trip + OPT-dependence → Tasks 4–5; §media types → Task 1; §building-block independence → Task 1 guard; §id-generation reuse (REQ-106) → Task 2 (consumes the WT). PROBE-076 → Task 7. All covered.
- **Placeholders:** the `<PICK …>` / `<trio>` markers are deliberate fixture-selection points; the `rm`/`rmpath`/`rminfo` accessor names carry explicit `go doc` confirm-steps (the only facts not verified at authoring time — the RM package API was not read during planning). No behavioural TODOs.
- **Type consistency:** `encodeFlat`/`decodeFlat`/`flatToStructured`/`structuredToFlat`/`indexTemplate`/`leafToFlat`/`flatFromSuffix` names and the public `Marshal*`/`Unmarshal*` signatures are consistent across Tasks 1–8.

## Mapping to specs

- [wire.md § REQ-053](../../specifications/wire.md#req-053) — normative FLAT/STRUCTURED contract.
- [REQ.md](../../specifications/REQ.md) — registry row.
- [ADR-0014](../../adr/0014-webtemplate-reference-implementation-lock.md) — reference-impl & id-generation lock (reused).
- conformance.md § PROBE-076 — round-trip conformance probe (registered in Task 7).
- [simplified-formats umbrella](../2026-06-23-simplified-formats.md) — Phase 3 of the umbrella.
- Ground truth: openEHR ITS-REST *Simplified Formats* (STABLE) — vendored twin in `openehr-kb/specs/ITS-REST/simplified_formats.md`; §RM Mappings drives the per-datatype coverage.
