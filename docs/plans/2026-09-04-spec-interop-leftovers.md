# Plan — Spec and interop leftovers (System descriptor §, `.schema` media types, two strands, two censuses)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans (inline) or superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax. Before any Go edit load `go-coding:go-coding`, then `go-testing` (Tasks 2 and 4) and `go-layout` (Task 2 adds exported API).

**Date:** 2026-09-04
**Status:** Draft
**Owner:** SDK maintainers
**Covers:** implementation-aligned amendments to landed [REQ-050](../specifications/wire.md#req-050), [REQ-053](../specifications/wire.md#req-053), [REQ-095](../specifications/wire.md#req-095), [REQ-112](../specifications/clinical-modeling.md#req-112--template-less-reference-model-validation-floor); evidence for open [STRAND-13](../specifications/research-strands.md#strand-13--properties-inherited-from-a-primitive-mapped-ancestor-are-dropped); opens **STRAND-14** — **no new requirement id**
**Probes:** none new
**Implementation:** planned
**Depends on:** landed `openehr/client/system` (Extras handling, PR #140), `openehr/serialize/simplified` (REQ-053), `internal/bmmgen` + `openehr/bmm.LoadAll`, `openehr/validation` (REQ-102 / REQ-110 / REQ-112)
**Defers:** resolving STRAND-13 (the census is evidence; folding a primitive-mapped ancestor's properties needs an ADR and a regenerated tree, and REQ-048 forbids pre-empting it in code); resolving STRAND-14 (its ADR, if the answer is "yes, run the floor"); STRAND-10 and STRAND-12 (need an ADR and an upstream issue respectively); closing REQ-095's `partial` (this plan names what keeps it partial; it does not vendor the missing bodies)

**Goal:** Close the specification-side and interoperability leftovers that were verified open on 2026-09-04 and need no design decision: give the System descriptor's unknown-key rule a normative home, accept EHRbase's `.schema`-suffixed Simplified-Formats media types on input as REQ-053 has said the SDK SHOULD, fix a scope statement that promises example programs the tree does not carry, turn STRAND-13's "evidence needed" into a pinned census, record the open validation-composition question as a strand instead of leaving it in a PR body, and replace REQ-095's vague "not all surfaces covered" with the named list.

**Architecture:** Documentation lands where the fact lives (one home per fact): the System descriptor binds by citation to § REQ-144's *Unknown response keys* rule from § REQ-050; the media-type helper is a small exported surface in `simplified` (`Format`, `ParseMediaType`, `Format.MediaType`) so content negotiation has one place and the codecs keep taking bytes; the STRAND-13 census is a `bmmgen` test over every pinned schema root that pins the exact set of dropped inherited properties so growth is loud; STRAND-14 follows the strands file's own template; the REQ-095 coverage table lives in the cassette README beside the fixtures it describes.

**Tech Stack:** Go 1.27.0, stdlib `mime.ParseMediaType`, `openehr/bmm.LoadAll` over `resources/bmm/*.bmm.json`.

## Global Constraints

- **REQ-013:** `openehr/serialize/simplified` imports no `transport/` or `auth/`; the media-type helper is stdlib only.
- **REQ-024 / REQ-025:** no reflection in library code; `ParseMediaType` returns an error, never panics, on any string.
- **REQ-048 interim rule:** the STRAND-13 census MUST NOT change `rminfo` or the generated structs; it only pins the observed set.
- **One home per fact:** § REQ-144 keeps the unknown-key rule text; § REQ-050 binds the third descriptor by citation. STRAND-14 holds the open question; § REQ-112 points at it in one sentence.
- **Formatting / lint:** `/usr/local/go/bin/gofmt -l` (the module toolchain's gofmt), `make lint` through the pinned Docker image.
- **Commits:** Conventional Commits, one per task; CHANGELOG bullet only for the media-type helper (the docs-only tasks add none).
- **Verification commands:** `go build ./...`, `go test ./openehr/serialize/simplified/ ./internal/bmmgen/ ./openehr/client/system/ -count=1`, `make lint`, `make spec-check`, `make ci`.

## Definition of Ready

- **`Covers:`** lists every REQ amended and the strands touched — done above; every REQ has canonical prose and a registry row.
- No new REQ id is needed; STRAND-14 takes the next free strand id (13 is the last allocated).
- Each task names its files, tests and verification command.

## Definition of Done

- wire.md § REQ-050 binds `ServiceCapabilities` to the § REQ-144 unknown-key rule; the REQ-050 traceability note no longer says "no § of its own".
- `simplified.ParseMediaType` classifies both canonical and both `.schema`-suffixed media types (case-insensitive, parameters ignored) and refuses anything else with `ErrUnknownMediaType`; `Format.MediaType` emits only the canonical strings; deviations.md, § REQ-053 and the umbrella plan's residual list say so.
- scope.md's examples row describes what `cmd/examples/` actually holds.
- A `bmmgen` test pins the set of class-definition properties inherited from a primitive-mapped ancestor across every pinned schema root, and STRAND-13's *Evidence needed* carries the dated result.
- STRAND-14 exists with question, why-open, trade-off, evidence needed and resolution form; § REQ-112 cites it in one sentence; the strands index has the row.
- `testkit/cassettes/its_rest/README.md` carries a coverage table naming the client surfaces with and without vendored bodies; REQ-095's traceability note and the roadmap row point at it.
- `make spec-check` and `make ci` pass; plan archived under `archive/` in this PR.

## Implementation checklist

| Step | Status |
|---|---|
| Task 1 — § REQ-050 System-descriptor unknown-key binding | |
| Task 2 — `simplified.ParseMediaType` / `Format` (REQ-053 SHOULD) | |
| Task 3 — scope.md examples row | |
| Task 4 — STRAND-13 census test + evidence paragraph | |
| Task 5 — STRAND-14 opened; § REQ-112 sentence | |
| Task 6 — REQ-095 coverage table + traceability + roadmap pointer | |
| Task 7 — CHANGELOG, traceability, plan indexes, `make spec-check`, `make ci`, archive | |

---

## Task 1: § REQ-050 binds the System descriptor to the unknown-key rule

**Files:**
- Modify: `docs/specifications/wire.md` § REQ-050 (append one paragraph after "The version pin is enforced at discovery time (REQ-072)…")
- Modify: `docs/specifications/wire.md` § REQ-144 **Unknown response keys** — the sentence "Both descriptors carry an `Extras` map" gains "(and, by [§ REQ-050](#req-050), the System descriptor `ServiceCapabilities`)"
- Modify: `docs/specifications/traceability.yaml` REQ-050 `notes:` — replace "Convention alignment, no § of its own:" with "Bound by § REQ-050 to the § REQ-144 unknown-key rule:"

- [ ] **Step 1: Spec** — append to § REQ-050:

> **System descriptor.** `system.Capabilities` decodes the `OPTIONS /` response into `ServiceCapabilities`, the descriptor that advertises the pinned `restapi_specs_version`. Its unknown response keys **MUST** be preserved on decode and re-emitted on encode under exactly the rule [§ REQ-144 *Unknown response keys*](#req-144--definition-metadata-decoding) states for the Definition descriptors — documented fields authoritative, exact-name collision ignored on encode, case-variant keys preserved beside the field — so a deployment-specific capability the pin does not name survives a round trip through the SDK.

- [ ] **Step 2: Verify** — `make spec-check`; `go test ./openehr/client/system/ -run 'Extras|Collision' -count=1` (existing tests already pin the behaviour; no code change).

- [ ] **Step 3: Commit** — `git commit -m "docs(spec): bind the System descriptor's unknown keys to the § REQ-144 rule from § REQ-050"`

## Task 2: `simplified.ParseMediaType` accepts the `.schema`-suffixed variants on input (REQ-053)

**Files:**
- Create: `openehr/serialize/simplified/mediatype.go`
- Modify: `openehr/serialize/simplified/simplified.go` (the comment above the `MediaType*` constants no longer says "this package does not yet provide that")
- Modify: `openehr/serialize/simplified/deviations.md` (the `.schema` row: accepted on input via `ParseMediaType`, never emitted)
- Modify: `docs/specifications/wire.md` § REQ-053 line "(The `.schema` acceptance is a SHOULD the implementation currently defers …)" → "(`simplified.ParseMediaType` accepts them; `Format.MediaType` emits only the canonical strings; the codecs themselves take bytes, so negotiation happens one call before them.)"
- Modify: `docs/plans/2026-06-23-simplified-formats.md` § Residual scope first bullet → "closed 2026-09-05 by `simplified.ParseMediaType` (input only)"
- Test: `openehr/serialize/simplified/mediatype_test.go`

**Interfaces:**
- Produces:
  ```go
  type Format int
  const (
      FormatUnknown Format = iota
      FormatFlat
      FormatStructured
  )
  func (f Format) String() string     // "FLAT", "STRUCTURED", "unknown"
  func (f Format) MediaType() string  // MediaTypeFlat / MediaTypeStructured / ""
  var ErrUnknownMediaType = errors.New("simplified: unknown media type")
  func ParseMediaType(s string) (Format, error)
  ```

- [ ] **Step 1: Write the failing test**

```go
package simplified_test

func TestParseMediaType(t *testing.T) { // REQ-053
	t.Parallel()
	cases := []struct {
		in   string
		want simplified.Format
	}{
		{"application/openehr.wt.flat+json", simplified.FormatFlat},
		{"application/openehr.wt.structured+json", simplified.FormatStructured},
		{"application/openehr.wt.flat.schema+json", simplified.FormatFlat},             // EHRbase variant
		{"application/openehr.wt.structured.schema+json", simplified.FormatStructured}, // EHRbase variant
		{"Application/OpenEHR.WT.Flat+JSON", simplified.FormatFlat},                    // media types are case-insensitive
		{"application/openehr.wt.flat+json; charset=utf-8", simplified.FormatFlat},     // parameters ignored
	}
	for _, tc := range cases {
		got, err := simplified.ParseMediaType(tc.in)
		if err != nil || got != tc.want {
			t.Errorf("ParseMediaType(%q) = %v, %v; want %v, nil", tc.in, got, err, tc.want)
		}
	}
	for _, in := range []string{"", "application/json", "text/plain", "application/openehr.wt+json", "application/openehr.wt.flat.schema", "not a media type"} {
		got, err := simplified.ParseMediaType(in)
		if !errors.Is(err, simplified.ErrUnknownMediaType) || got != simplified.FormatUnknown {
			t.Errorf("ParseMediaType(%q) = %v, %v; want FormatUnknown, ErrUnknownMediaType", in, got, err)
		}
	}
}

func TestFormatMediaTypeEmitsCanonicalOnly(t *testing.T) { // REQ-053: MUST NOT emit .schema
	t.Parallel()
	if got := simplified.FormatFlat.MediaType(); got != simplified.MediaTypeFlat {
		t.Errorf("FormatFlat.MediaType() = %q, want %q", got, simplified.MediaTypeFlat)
	}
	if got := simplified.FormatStructured.MediaType(); got != simplified.MediaTypeStructured {
		t.Errorf("FormatStructured.MediaType() = %q, want %q", got, simplified.MediaTypeStructured)
	}
	if got := simplified.FormatUnknown.MediaType(); got != "" {
		t.Errorf("FormatUnknown.MediaType() = %q, want empty", got)
	}
	for _, f := range []simplified.Format{simplified.FormatFlat, simplified.FormatStructured} {
		if strings.Contains(f.MediaType(), ".schema") {
			t.Errorf("%v.MediaType() = %q emits the EHRbase .schema variant; REQ-053 forbids emitting it", f, f.MediaType())
		}
		if back, err := simplified.ParseMediaType(f.MediaType()); err != nil || back != f {
			t.Errorf("ParseMediaType(%v.MediaType()) = %v, %v; want %v", f, back, err, f)
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `go test ./openehr/serialize/simplified/ -run 'TestParseMediaType|TestFormatMediaType'`. Expected: compile error (undefined `Format`, `ParseMediaType`).

- [ ] **Step 3: Implement** `mediatype.go`:

```go
package simplified

import (
	"errors"
	"fmt"
	"mime"
	"strings"
)

// Format identifies which of the two Simplified Formats a media type names.
type Format int

const (
	FormatUnknown Format = iota
	FormatFlat
	FormatStructured
)

// ErrUnknownMediaType is returned by [ParseMediaType] for a value that names
// neither Simplified Format — a different media type, an unparseable header
// value, or the WebTemplate resource type, which is not a composition format.
var ErrUnknownMediaType = errors.New("simplified: unknown media type")

// String names the format for diagnostics.
func (f Format) String() string {
	switch f {
	case FormatFlat:
		return "FLAT"
	case FormatStructured:
		return "STRUCTURED"
	case FormatUnknown:
		return "unknown"
	default:
		return fmt.Sprintf("Format(%d)", int(f))
	}
}

// MediaType returns the canonical media type for f (REQ-053): the SDK emits
// the two Simplified Formats strings only, never EHRbase's `.schema` variants.
// FormatUnknown yields "".
func (f Format) MediaType() string {
	switch f {
	case FormatFlat:
		return MediaTypeFlat
	case FormatStructured:
		return MediaTypeStructured
	case FormatUnknown:
		return ""
	default:
		return ""
	}
}

// acceptedMediaTypes is the input-side vocabulary: the two canonical strings
// plus EHRbase's `.schema`-suffixed variants, which REQ-053 says the SDK SHOULD
// accept on input for interoperability while never emitting them.
var acceptedMediaTypes = map[string]Format{
	MediaTypeFlat:                                    FormatFlat,
	MediaTypeStructured:                              FormatStructured,
	"application/openehr.wt.flat.schema+json":       FormatFlat,
	"application/openehr.wt.structured.schema+json": FormatStructured,
}

// ParseMediaType classifies a Content-Type or Accept value as FLAT or
// STRUCTURED. The type is matched case-insensitively per RFC 2045 and any
// parameters (`; charset=utf-8`) are ignored. Anything else — including the
// WebTemplate resource type `application/openehr.wt+json` — fails with
// [ErrUnknownMediaType]. It never panics on any input (REQ-025).
func ParseMediaType(s string) (Format, error) {
	mt, _, err := mime.ParseMediaType(s)
	if err != nil {
		return FormatUnknown, fmt.Errorf("%w: %w", ErrUnknownMediaType, err)
	}
	if f, ok := acceptedMediaTypes[strings.ToLower(mt)]; ok {
		return f, nil
	}
	return FormatUnknown, fmt.Errorf("%w: %q", ErrUnknownMediaType, mt)
}
```

(`mime.ParseMediaType` already lowercases the type; `strings.ToLower` is belt and braces and costs nothing.)

- [ ] **Step 4: Docs** — the four documentation edits listed under Files; the `simplified.go` comment becomes: "Media types for the Simplified Formats (REQ-053). Emit these canonical types — [Format.MediaType] does. [ParseMediaType] also accepts EHRbase's non-conformant `.schema`-suffixed variants on input; the codecs themselves take bytes, so negotiation happens one call before them."

- [ ] **Step 5: Verify** — `go test ./openehr/serialize/simplified/ -count=1`; `/usr/local/go/bin/gofmt -l openehr/serialize/simplified`; `go vet ./openehr/serialize/simplified/`; `go test ./openehr/serialize/simplified/ -run TestIndependence` (REQ-013 import guard, if the package has one — `independence_test.go` exists).

- [ ] **Step 6: Commit** — `git commit -m "feat(simplified): ParseMediaType accepts the EHRbase .schema-suffixed media types on input (REQ-053)"`

## Task 3: scope.md's examples row describes the tree

**Files:**
- Modify: `docs/specifications/scope.md` — the row "| Examples per primary use case | Worked example programs under `cmd/examples/` for benchmark, seeder, MCP, federator |"

- [ ] **Step 1: Edit** the row to: "| Examples per primary use case | Worked example programs under `cmd/examples/`, one per SDK surface, catalogued in [`../examples.md`](../examples.md); the four primary consumers — benchmark, seeder, MCP, federator ([use-cases.md](use-cases.md)) — are downstream products that follow those shapes, not programs in this tree |"

- [ ] **Step 2: Verify** — `make spec-check` (the examples census guard counts `cmd/examples/` entries, not this row).

- [ ] **Step 3: Commit** — `git commit -m "docs(spec): scope.md names the examples the tree carries, not the downstream consumers"`

## Task 4: STRAND-13 census — properties inherited from a primitive-mapped ancestor

**Files:**
- Create: `internal/bmmgen/primitive_ancestor_census_test.go` (package `bmmgen`, so it can call `isPrimitive` and `classProperties`)
- Modify: `docs/specifications/research-strands.md` § STRAND-13 — add a dated **Evidence (2026-09-05 census)** paragraph after *Evidence needed*
- Modify: `openehr/rm/rminfo/probe_094_test.go` `unshippedProperties` comment — one sentence pointing at the bmmgen census as the cross-schema view (no behaviour change)

**Interfaces:**
- Consumes: `bmm.LoadAll(rootID, bmm.FSResolver{Root: testResources})` (`testResources` is defined in `plan_test.go`), `isPrimitive(name)` (`primitives.go`), `classProperties(c bmm.Class)` (`render_rminfo.go`).

- [ ] **Step 1: Write the census test**

```go
package bmmgen

// STRAND-13 evidence: which class_definitions classes inherit a property from
// a primitive_types ancestor the generator maps to a Go primitive (and so
// never plans as a class, dropping the property from both the emitted struct
// and the rminfo tables). The strand's "evidence needed" is exactly this census,
// across every pinned schema root rather than the RM reduction PROBE-094
// surfaces. The set is PINNED, not tolerated: a new entry must be added here by
// name (and the strand updated) before it can pass, and an entry that stops
// occurring is reported as stale. Folding anything in is forbidden ahead of the
// strand (REQ-048 § The attribute tables are complete against the BMM).

var pinnedSchemaRoots = []string{
	"openehr_base_1.3.0", "openehr_rm_1.2.0", "openehr_am_1.4.0",
	"openehr_am_2.4.0", "openehr_lang_1.1.0", "openehr_term_3.1.0",
}

// primitiveAncestorDrops is the census result as of 2026-09-05:
// "<Class>.<property> via <primitive-mapped ancestor>" → the roots it appears in.
var primitiveAncestorDrops = map[string][]string{
	"Iso8601_timezone.value via Iso8601_type": {"openehr_base_1.3.0", "openehr_rm_1.2.0", "openehr_am_1.4.0", "openehr_am_2.4.0", "openehr_lang_1.1.0", "openehr_term_3.1.0"},
}

func TestPrimitiveMappedAncestorPropertyCensus(t *testing.T) { // STRAND-13
	got := map[string][]string{}
	for _, root := range pinnedSchemaRoots {
		schema, err := bmm.LoadAll(root, bmm.FSResolver{Root: testResources})
		if err != nil {
			t.Fatalf("LoadAll(%s): %v", root, err)
		}
		lookup := func(name string) (bmm.Class, bool) {
			if c, ok := schema.ClassDefinitions[name]; ok {
				return c, true
			}
			c, ok := schema.PrimitiveTypes[name]
			return c, ok
		}
		for _, name := range slices.Sorted(maps.Keys(schema.ClassDefinitions)) {
			for _, anc := range transitiveAncestors(lookup, name) {
				if !isPrimitive(anc) {
					continue
				}
				ac, ok := lookup(anc)
				if !ok {
					continue
				}
				props, _ := classProperties(ac)
				for _, p := range slices.Sorted(maps.Keys(props)) {
					key := name + "." + p + " via " + anc
					got[key] = append(got[key], root)
				}
			}
		}
	}
	for key, roots := range got {
		want, pinned := primitiveAncestorDrops[key]
		if !pinned {
			t.Errorf("unpinned drop %s (in %v): a class_definitions property is inherited from a primitive-mapped ancestor and silently dropped — pin it here and record it under STRAND-13", key, roots)
			continue
		}
		if !slices.Equal(roots, want) {
			t.Errorf("%s: seen in %v, pinned for %v — update the pin and STRAND-13", key, roots, want)
		}
	}
	for key := range primitiveAncestorDrops {
		if _, ok := got[key]; !ok {
			t.Errorf("stale pin %s: no pinned schema exhibits it any more — drop the pin and close the STRAND-13 evidence", key)
		}
	}
}

// transitiveAncestors walks ancestors depth-first through both class and
// primitive definitions, each name once, in discovery order.
func transitiveAncestors(lookup func(string) (bmm.Class, bool), name string) []string {
	seen := map[string]bool{}
	var out []string
	var rec func(string)
	rec = func(n string) {
		c, ok := lookup(n)
		if !ok {
			return
		}
		for _, anc := range c.Ancestors() {
			if seen[anc] {
				continue
			}
			seen[anc] = true
			out = append(out, anc)
			rec(anc)
		}
	}
	rec(name)
	return out
}
```

- [ ] **Step 2: Run it** — `go test ./internal/bmmgen/ -run TestPrimitiveMappedAncestorPropertyCensus -v`. Expected: either PASS (the pin is complete) or a list of unpinned drops naming class, property, ancestor and roots. **Do not fold anything in**; add each reported drop to `primitiveAncestorDrops` verbatim and carry the list into Step 3. (The `roots` slice in the pin must match the order of `pinnedSchemaRoots`.)

- [ ] **Step 3: Strand** — under STRAND-13 *Evidence needed*, append:

> **Evidence (2026-09-05 census):** `internal/bmmgen/primitive_ancestor_census_test.go` walks every `class_definitions` class in each of the six pinned schema roots (`base 1.3.0`, `rm 1.2.0`, `am 1.4.0`, `am 2.4.0`, `lang 1.1.0`, `term 3.1.0`) and lists each property inherited from a primitive-mapped ancestor. Result: *[the pinned set, e.g. exactly one — `Iso8601_timezone.value` via `Iso8601_type`, present in every root because all six include `base`]*. The set is pinned by the test so growth is loud. What this settles: *[if one entry — the fold-into-both option's blast radius is one generated struct and one rminfo row; if more — name them and say the family is larger than the RM reduction showed]*. The strand stays open: the fold is still an ADR (§ Mapping rules inheritance) plus a regenerated tree.

  Replace the bracketed text with the actual census output; the plan carries no other placeholder.

- [ ] **Step 4: Verify** — `go test ./internal/bmmgen/ -count=1`; `/usr/local/go/bin/gofmt -l internal/bmmgen`; `make spec-check`.

- [ ] **Step 5: Commit** — `git commit -m "test(bmmgen): pin the primitive-mapped-ancestor property census across every pinned schema root (STRAND-13)"`

## Task 5: STRAND-14 — should template-driven validation also run the RM floor?

**Files:**
- Modify: `docs/specifications/research-strands.md` — new section before `## Index`; new index row after STRAND-13's
- Modify: `docs/specifications/clinical-modeling.md` § REQ-112 — one sentence after "(template validity implies RM validity, so the two compose)."

- [ ] **Step 1: Strand section**

```markdown
## STRAND-14 — Should template-driven validation also run the RM-floor invariants?

**Status:** Open — opened 2026-09-05 from the PR #145 review round (the plan that landed the `TERM_MAPPING` invariants claimed `ValidateComposition` would report them; it never has).

**Question:** should `ValidateComposition` (REQ-102) and the REQ-110 entry points run the REQ-112 per-type invariant catalogue as part of a template-driven pass, or stay exactly template-conformance with the floor a separate call?

**Why it's open:** today the two layers compose but do not chain. Template validity covers RM-mandatory presence, so a template-driven pass already reports the floor's (a) arm; the floor's (b) arm — `DV_INTERVAL` lower > upper, `TERM_MAPPING.match` outside its value set, `Mappings_valid`, `DV_QUANTITY.precision < 0` — fires only through `ValidateRM`. A caller who runs only `ValidateComposition` can therefore commit an RM-invalid composition that is template-valid. Whether that is a gap or a deliberate separation of concerns is the fork.

**The trade-off:**

- **Chain the floor into the template-driven pass.** One call, no way to forget the floor; issue codes stay disjoint (`term_mapping_match` beside the template codes). Against it: `Result` grows for every existing caller, the template walker and the floor walker visit the tree twice (or the floor's checks are re-hosted inside the template walk), and some floor findings duplicate template findings on the same node.
- **Keep them separate and document the composition.** No behaviour change; callers who want both call both, and REQ-112 says so. Against it: the gap stays reachable by default, and the plan-text defect that opened this strand shows how easily the two are assumed to chain.
- **Opt-in chaining** (a `WithRMFloor()` option on the template-driven entry points). Additive and reversible. Against it: two code paths to keep in agreement, and an option nobody sets is the second option wearing a hat.

**Evidence needed:** how often a template-valid, RM-invalid composition reaches a consumer in practice (the consuming CDR project's validation pipeline is the first place to ask); the duplicate-issue rate if the floor is chained over the vendored composition corpus; the cost of a second walk on the benchmark harness's composition sizes.

**Resolution form:** ADR-NNNN; amends REQ-102 / REQ-110 (if chaining) and REQ-112's composition sentence either way. Until then § REQ-112 states the current behaviour and points here.

**Affects:** REQ-102, REQ-110, REQ-112.

---
```

  Index row: `| [STRAND-14](#strand-14--should-template-driven-validation-also-run-the-rm-floor-invariants) | Template-driven validation and the RM floor | Open | REQ-102, REQ-110, REQ-112 |`

- [ ] **Step 2: § REQ-112 sentence** — after "(template validity implies RM validity, so the two compose)." append: " They compose but do not chain: `ValidateComposition` and the REQ-110 entry points do **not** run this floor's per-type invariant catalogue, so a caller wanting both runs both — whether the template-driven pass should chain the floor is open as [STRAND-14](research-strands.md#strand-14--should-template-driven-validation-also-run-the-rm-floor-invariants) and **MUST NOT** be pre-empted in code."

- [ ] **Step 3: Verify** — `make spec-check`.

- [ ] **Step 4: Commit** — `git commit -m "docs(spec): open STRAND-14 on chaining the RM floor into template-driven validation; § REQ-112 states the composition"`

## Task 6: REQ-095 coverage — name what keeps it partial

**Files:**
- Modify: `testkit/cassettes/its_rest/README.md` — new section **Coverage against the client surface** before `## Conventions`
- Modify: `docs/specifications/traceability.yaml` REQ-095 `notes:` — append a "What keeps this partial (2026-09-05 census):" sentence pointing at the README table
- Modify: `docs/roadmap.md` — the OpenAPI cassettes row's Notes cell: "Not all surfaces covered" → "Coverage table in [`testkit/cassettes/its_rest/README.md`](../testkit/cassettes/its_rest/README.md); stored-query metadata and ITEM_TAG bodies are the named gaps"

- [ ] **Step 1: Census the tree** — `ls testkit/cassettes/its_rest/*/` against `openehr/client/*` and the pinned OAS paths (`resources/its-rest/*.openapi.yaml`). As of 2026-09-04: `system/` (capabilities), `ehr/` (EHR, EHR_STATUS, FOLDER), `definition/` (template list, template metadata, one OPT — **no stored-query metadata or list body**), `demographic/` (five party kinds, ORIGINAL_VERSION, REVISION_HISTORY), `query/` (RESULT_SET), `errors/`, `discovery/`; composition bodies live under `../compositions/` and `../rm/`; contribution request bodies under `../submissions/`. Not vendored: stored-query metadata / list responses, ITEM_TAG bodies (the SDK reads tags from headers today, REQ-059 partial), Admin (no bodies by contract), the VERSIONED_COMPOSITION / VERSIONED_EHR_STATUS family (not implemented).

- [ ] **Step 2: README table**

```markdown
## Coverage against the client surface

What `openehr/client/*` decodes today, and whether a vendored body under this directory (or a sibling cassette tree) exercises it. This is the census behind REQ-095's `partial`: the rows marked **gap** are what keeps it partial.

| Client surface | Vendored body | Where |
|---|---|---|
| System `OPTIONS /` | yes | `system/capabilities.json` |
| EHR, EHR_STATUS, FOLDER reads | yes | `ehr/` |
| COMPOSITION reads and writes | yes | [`../compositions/`](../compositions/), [`../rm/`](../rm/) |
| CONTRIBUTION submission (request) | yes | [`../submissions/`](../submissions/) |
| CONTRIBUTION read (`GET …/contribution/{uid}`) | **gap** | decoded from a hand-built body in `contribution_test.go`; no upstream-authored persisted CONTRIBUTION |
| Definition — template list, metadata, OPT | yes | `definition/` |
| Definition — stored-query metadata and list | **gap** | hand-built in `stored_query_test.go`; no vendored `StoredQueryMetadata` body |
| Query — RESULT_SET | yes | `query/result_set.json` |
| Demographic — five party kinds, ORIGINAL_VERSION, REVISION_HISTORY | yes | `demographic/` |
| ITEM_TAG | **gap** | header-carried today (REQ-059 partial); no tag bodies until the dedicated endpoints land |
| Admin | n/a | `204` by contract, no bodies |
| VERSIONED_COMPOSITION / VERSIONED_EHR_STATUS | n/a | family not implemented (roadmap: deferred under STRAND-09) |
| Error envelopes | yes | `errors/` |
| SMART discovery | yes | `discovery/` |
```

  Verify the CONTRIBUTION-read row against `openehr/client/ehr/contribution/contribution_test.go` before committing (if a vendored body exists, the row is "yes" and names it).

- [ ] **Step 3: Verify** — `make spec-check`; every path in the table exists (`ls`).

- [ ] **Step 4: Commit** — `git commit -m "docs(cassettes): name the ITS-REST bodies still missing behind REQ-095's partial"`

## Task 7: Close-out

- [ ] **CHANGELOG** `## [Unreleased] / ### Added` — one bullet: **Simplified-Formats media-type negotiation (REQ-053).** `simplified.ParseMediaType` classifies a Content-Type or Accept value as FLAT or STRUCTURED, accepting EHRbase's `.schema`-suffixed variants on input, while `Format.MediaType` emits only the two canonical strings ([plan](docs/plans/archive/2026-09-04-spec-interop-leftovers.md)).
- [ ] **traceability.yaml** — REQ-053: tests `+ openehr/serialize/simplified/mediatype_test.go`, plans `+ this plan (archive path)`; REQ-050: notes rewritten (Task 1), plans `+ this plan`; REQ-095: notes (Task 6), plans `+ this plan`; REQ-112: plans `+ this plan`.
- [ ] **Indexes** — `docs/plans/README.md` (the *Leftover sweeps* section from the sibling plan gains this row; if that section is not on this branch, add it), `docs/plans/archive/README.md` row; `git mv` this file to `archive/` with **Status:** landed, links repointed to `../../specifications/`.
- [ ] **Gates** — `make spec-check`, `make ci`.
- [ ] **Commit + PR** — before pushing, `git fetch origin main` and `gh pr list`; the PR body names the one behaviour addition (the media-type helper), the two strands (13 evidence, 14 opened), and the REQ-095 census; "Follow-ups (not in this PR)" lists STRAND-10, STRAND-12 and the REQ-095 gaps themselves.

## Mapping to specs

- [wire.md § REQ-050](../specifications/wire.md#req-050) — System descriptor binding (Task 1)
- [wire.md § REQ-144](../specifications/wire.md#req-144--definition-metadata-decoding) — owns the unknown-key rule (Task 1)
- [wire.md § REQ-053](../specifications/wire.md#req-053) — media types (Task 2)
- [scope.md](../specifications/scope.md) — examples row (Task 3)
- [research-strands.md § STRAND-13](../specifications/research-strands.md#strand-13--properties-inherited-from-a-primitive-mapped-ancestor-are-dropped) — census evidence (Task 4)
- [research-strands.md § STRAND-14](../specifications/research-strands.md#strand-14--should-template-driven-validation-also-run-the-rm-floor-invariants) — opened (Task 5)
- [clinical-modeling.md § REQ-112](../specifications/clinical-modeling.md#req-112--template-less-reference-model-validation-floor) — composition sentence (Task 5)
- [wire.md § REQ-095](../specifications/wire.md#req-095) — coverage census (Task 6)
