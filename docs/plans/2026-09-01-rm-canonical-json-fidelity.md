# RM canonical-JSON fidelity (TERM_MAPPING.match + DV_TEXT.mappings + DV_MULTIMEDIA data) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Date:** 2026-09-01
**Status:** Draft
**Owner:** SDK maintainers
**Covers:** [REQ-046](../specifications/bmm-conformance.md#req-046--primitive-type-mapping) (primitive type mapping), [REQ-052](../specifications/wire.md#req-052) (canonical JSON round-trip), [REQ-112](../specifications/clinical-modeling.md#req-112--composition-validation) (template-less RM floor). No new REQ id — every delta amends a shipped REQ in place (implementation-aligned; the normative table edit rides in the same PR).
**Probes:** PROBE-030 (canonical-JSON round-trip corpus, extended); PROBE-081 pattern reused for presence (no new probe minted).
**Implementation:** planned
**Depends on:** landed named-primitive precedent (`openehr/rm/integer.go`, `openehr/rm/real.go`); the template-less RM floor `openehr/validation/rmfloor.go` + `rmfloor_bytes.go` (REQ-112); the generator `internal/bmmgen`. All shipped; no new prerequisites.
**Defers:** a general per-node decode-time presence map for `mappings` at arbitrary depth (Phase 2 delivers the boundary case + the documented model limitation, not a whole-tree presence API); any change to `omitempty` on `mappings` or `data`; terminology validation of `TERM_MAPPING.target`; the consuming CDR project's own skip-on-decode-failure policy (theirs to re-examine).

**Goal:** Make `TERM_MAPPING.match` decode and encode as the canonical single-character JSON string the openEHR RM defines (not a number), enforce its value set at the RM floor, give callers a way to tell an empty `mappings` from an absent one on decode, and pin the `DV_MULTIMEDIA` inline-byte round-trip that is currently unexercised. Reported by the consuming CDR project (openehr-cdr), where a valid composition carrying a term mapping cannot be decoded and the failure is silent.

**Architecture:** `Character` becomes a hand-written named string primitive with custom JSON, exactly like the existing `Integer`/`Real` types; the BMM primitive map points `Character` at it, and one regenerated field (`TERM_MAPPING.match`, the *only* `Character` in the RM) picks it up with no other generator change. The value-set and non-empty-list invariants land in the existing RM-floor walker. The byte-level presence signal follows the `ValidateRMEHRStatusBytes` (REQ-112 / SDK-GAP-18) precedent.

**Tech Stack:** Go (`encoding/json` custom marshalers), `internal/bmmgen` code generator, `openehr/serialize/canjson`, `openehr/validation`.

**Spec:** canonical homes updated by this plan —
[bmm-conformance.md § Primitive type mapping](../specifications/bmm-conformance.md#primitive-type-mapping) (REQ-046),
[wire.md § REQ-052](../specifications/wire.md#req-052) (canonical JSON),
[clinical-modeling.md § REQ-112](../specifications/clinical-modeling.md#req-112--composition-validation) (RM floor).

## Global Constraints

- **`go.mod` `go` directive stays a `1.N.0` minor floor** — never a specific patch (air-gapped tool images).
- **Generated files are never hand-edited.** `*_gen.go` change only by re-running `make codegen`; `make codegen-verify` must stay green.
- **Encode-side refusal travels through `canjson.ErrInvalidValue`** (REQ-052); a value that cannot be encoded is an error, never silent corruption.
- **Value-free diagnostics** (REQ-093): error strings name the attribute and classification, never the payload value.
- **One home per fact.** Commit/PR/CHANGELOG cite `REQ-046` / `REQ-052` / `REQ-112` and this plan; they do not restate the normative prose.

---

## Definition of Ready

- **Covers** lists every REQ this plan touches — REQ-046, REQ-052, REQ-112. ✅
- Canonical normative prose exists for each (the primitive-mapping table, the REQ-052 canonical-JSON section, the REQ-112 floor). This plan *amends* those sections; it allocates no new id, so no ADR and no numbering-band edit are required. The one design fork (Phase 1, `Character` representation) is settled below, not left open.
- Phases list concrete tasks and name the verification command (`make ci`, `make spec-check`, `make codegen-verify`).

## Definition of Done

- Code and tests land with `// REQ-046` / `// REQ-052` / `// REQ-112` citations.
- [`traceability.yaml`](../specifications/traceability.yaml) and the REQ.md **Impl.** column reflect the change (REQ-052 gains these clauses; it **remains `partial`** — its FLAT/STRUCTURED and other open clauses are untouched).
- The two indexes `spec-check` cannot see are updated: a [`roadmap.md`](../roadmap.md) row for what landed; no numbering band moved (no new id).
- Canonical spec prose updated in the **same PR** (the `Character` row in the primitive table; the REQ-052 `match` round-trip clause; the REQ-112 invariant clause).
- `make codegen-verify`, `make spec-check`, and `make ci` pass.
- Plan archived under [`docs/plans/archive/`](archive/) in the implementing PR (`sdd-archive`).

## Implementation checklist

| Step | Status |
|---|---|
| Spec / registry updated (`bmm-conformance.md` table, `wire.md` REQ-052 clause, `clinical-modeling.md` REQ-112 clause, `traceability.yaml`, REQ.md row) | |
| Indexes `spec-check` misses (`roadmap.md` row) | |
| Code (Phases 1–4) | |
| Tests with `// REQ-` comments | |
| `make codegen-verify` | |
| `make spec-check` | |
| `make ci` | |

---

## File Structure

- **Create** `openehr/rm/character.go` — the `Character` named primitive + custom JSON (Phase 1).
- **Create** `openehr/rm/character_test.go` — round-trip + refusal tests (Phase 1).
- **Modify** `internal/bmmgen/primitives.go:20` — map `Character` → `Character` (Phase 1).
- **Modify** `docs/specifications/bmm-conformance.md` — primitive-mapping table row + a justifying sentence (Phase 1).
- **Modify** `docs/specifications/wire.md` — REQ-052 `match` round-trip clause (Phase 1); `mappings` decode-collapse note (Phase 2).
- **Regenerated** by `make codegen`: `openehr/rm/data_types_text_gen.go` + its `*_jsonmar_gen.go` / `*_jsonunmar_gen.go` (field becomes `Match Character`).
- **Modify** `openehr/validation/rmfloor.go` — value-set + `Mappings_valid` invariants (Phase 3).
- **Modify** `openehr/validation/rmfloor_test.go` — invariant tests (Phase 3).
- **Modify** `docs/specifications/clinical-modeling.md` — REQ-112 invariant clause (Phase 3).
- **Create/Modify** `openehr/serialize/canjson/*_test.go` — DV_MULTIMEDIA base64 round-trip pin + mappings-collapse documentation test (Phases 2, 4).

---

## Phases

### Task 1: `Character` named primitive with canonical single-character JSON (Item A)

**Files:**
- Create: `openehr/rm/character.go`
- Test: `openehr/rm/character_test.go`
- Modify: `internal/bmmgen/primitives.go:20`
- Modify: `docs/specifications/bmm-conformance.md` (primitive-mapping table), `docs/specifications/wire.md` (REQ-052 clause)

**Interfaces:**
- Produces: `type Character string` with `func (Character) MarshalJSON() ([]byte, error)` and `func (*Character) UnmarshalJSON([]byte) error`. After regen, `rm.TermMapping.Match` has type `Character` (was `rune`).

- [ ] **Step 1: Write the failing test** (`openehr/rm/character_test.go`)

```go
package rm_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// REQ-046 / REQ-052: TERM_MAPPING.match is a single-character canonical JSON string.
func TestTermMappingMatchRoundTrip(t *testing.T) {
	for _, want := range []string{">", "=", "<", "?"} {
		in := []byte(`{"_type":"TERM_MAPPING","match":"` + want + `",` +
			`"target":{"_type":"CODE_PHRASE","terminology_id":` +
			`{"_type":"TERMINOLOGY_ID","value":"SNOMED-CT"},"code_string":"371532007"}}`)
		var tm rm.TermMapping
		if err := canjson.Unmarshal(in, &tm); err != nil {
			t.Fatalf("decode %q: %v", want, err)
		}
		if string(tm.Match) != want {
			t.Fatalf("match = %q, want %q", string(tm.Match), want)
		}
		out, err := canjson.Marshal(&tm)
		if err != nil {
			t.Fatalf("encode %q: %v", want, err)
		}
		if !contains(out, `"match":"`+want+`"`) {
			t.Fatalf("encoded form = %s, want match as one-char string %q", out, want)
		}
	}
}

func TestTermMappingMatchRejectsBadLength(t *testing.T) {
	for _, bad := range []string{`""`, `"=="`} {
		var c rm.Character
		if err := c.UnmarshalJSON([]byte(bad)); err == nil {
			t.Fatalf("UnmarshalJSON(%s) = nil error, want a length error", bad)
		}
	}
}

func contains(b []byte, sub string) bool { return len(b) >= len(sub) && (string(b) == sub || indexOf(string(b), sub) >= 0) }
func indexOf(s, sub string) int { for i := 0; i+len(sub) <= len(s); i++ { if s[i:i+len(sub)] == sub { return i } }; return -1 }
```

- [ ] **Step 2: Run it and confirm it fails**

Run: `go test ./openehr/rm/ -run TestTermMappingMatch -v`
Expected: FAIL — today `tm.Match` is a `rune`, so the decode errors with *"cannot unmarshal string into Go struct field … of type int32"*.

- [ ] **Step 3: Write `openehr/rm/character.go`**

```go
package rm

import (
	"encoding/json"
	"errors"
	"fmt"
	"unicode/utf8"
)

// Character is the BMM Character primitive. In canonical openEHR JSON it is
// written as a single-character string (e.g. TERM_MAPPING.match "="), never a
// number. Its zero value ("") is not a legal Character, so an omitted attribute
// is distinguishable from a supplied one. Mirrors the named-primitive shape of
// Integer/Real. REQ-046 / REQ-052.
type Character string

// UnmarshalJSON accepts a canonical one-character JSON string. For backward
// compatibility with the pre-fix encoder — which wrote match as a number — a
// JSON number is also accepted on decode, but it is never the encoded form
// (REQ-052 point 4).
func (c *Character) UnmarshalJSON(b []byte) error {
	if len(b) == 0 {
		return errors.New("rm.Character: empty input")
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return fmt.Errorf("rm.Character: %w", err)
		}
		if utf8.RuneCountInString(s) != 1 {
			return fmt.Errorf("rm.Character: must be exactly one character, got %d", utf8.RuneCountInString(s))
		}
		*c = Character(s)
		return nil
	}
	var n int32
	if err := json.Unmarshal(b, &n); err != nil {
		return fmt.Errorf("rm.Character: %w", err)
	}
	*c = Character(rune(n))
	return nil
}

// MarshalJSON emits a one-character JSON string. An empty or multi-rune value is
// an encode error (never silently coerced), which canjson wraps as
// ErrInvalidValue (REQ-052 point 3). Diagnostics are value-free (REQ-093).
func (c Character) MarshalJSON() ([]byte, error) {
	if utf8.RuneCountInString(string(c)) != 1 {
		return nil, fmt.Errorf("rm.Character: must be exactly one character, got %d", utf8.RuneCountInString(string(c)))
	}
	return json.Marshal(string(c))
}
```

- [ ] **Step 4: Point the generator at it** — `internal/bmmgen/primitives.go:20`

```go
	"Character": "Character", // was "rune": canonical JSON is a single-char string — REQ-046
```

- [ ] **Step 5: Regenerate and verify no unintended drift**

Run: `make codegen && make codegen-verify`
Expected: the only functional change is `TERM_MAPPING`'s field, now `Match Character \`json:"match"\`` in `data_types_text_gen.go` and both marshaller aux structs. `codegen-verify` passes (generated tree matches the generator).

- [ ] **Step 6: Run the Task-1 tests — expect PASS**

Run: `go test ./openehr/rm/ -run TestTermMappingMatch -v`
Expected: PASS. The generated `TermMapping` marshaller aux struct now carries `Character`, so `encoding/json` calls its custom methods (the same mechanism that already works for `Integer` fields, e.g. `DVQuantity` — no generator change needed).
Note: the generated `TermMapping` unmarshaller already wraps a child field error as `typereg.DecodeError{Path: "/match", Inner: err}`, so the attribute is named by the wrapper (REQ-052 point 3) — the custom error only has to be descriptive.

- [ ] **Step 7: Update the normative spec (same PR)**

In `docs/specifications/bmm-conformance.md`, change the primitive-mapping table row from `| \`Character\` | \`rune\` | |` to:

```
| `Character` | `Character` | canonical JSON single-character **string**, not a number; hand-written named type (`openehr/rm/character.go`), like `Integer`/`Real` |
```

Add one sentence under REQ-046 recording *why* this row is not a bare builtin: `Character` is the one primitive whose canonical JSON spelling (a one-character string) disagrees with a numeric Go `rune`, so it maps to a named type carrying its own codec — the mapping is still fixed, this is the fixed target.

In `docs/specifications/wire.md` § REQ-052, add a bullet: `TERM_MAPPING.match` **MUST** encode as a one-character JSON string and **MUST** decode from one; a string that is empty or longer than one character **MUST** be a decode error; a bare number **MAY** decode (back-compat) but **MUST NOT** be the encoded form.

- [ ] **Step 8: Commit**

```bash
git add openehr/rm/character.go openehr/rm/character_test.go internal/bmmgen/primitives.go \
        openehr/rm/data_types_text_gen.go openehr/rm/data_types_text_jsonmar_gen.go \
        openehr/rm/data_types_text_jsonunmar_gen.go \
        docs/specifications/bmm-conformance.md docs/specifications/wire.md
git commit -m "fix(rm): TERM_MAPPING.match is a canonical single-character string (REQ-046, REQ-052)"
```

---

### Task 2: Distinguish an empty `mappings` from an absent one on decode (Item B)

**Files:**
- Modify: `docs/specifications/wire.md` (REQ-052 decode-collapse note)
- Test: `openehr/serialize/canjson/mappings_presence_test.go` (create)
- Modify (if a boundary helper is added): `openehr/validation/rmfloor_bytes.go`

**Interfaces:**
- Consumes: the `Character` change is unrelated; this task is independent and may land in parallel.
- Produces: a documented, tested statement of the model-level collapse, and — where the consuming floor decodes bytes — a presence signal following `ValidateRMEHRStatusBytes`.

**Scope note (honest):** `[]TermMapping` with `omitempty` cannot carry the `[]` / `null` / absent distinction after decode, and the proposal explicitly does **not** want `omitempty` changed. The general fix (a decode-time presence map for every `mappings` at any depth) is larger than this bundle and is **deferred** (see Defers). Task 2 delivers (a) a test that *documents* the collapse so it cannot regress silently, and (b) the byte-level presence signal at the RM-floor boundary the consumer actually crosses, mirroring the REQ-112 precedent.

- [ ] **Step 1: Write the documentation test** (`openehr/serialize/canjson/mappings_presence_test.go`)

```go
package canjson_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// REQ-052: records the known model-level collapse — []/null/absent mappings all
// decode to an empty slice and re-encode to an absent key. Locks the behaviour
// so a future change is a conscious one, not an accident.
func TestDVTextMappingsCollapse(t *testing.T) {
	for _, in := range []string{
		`{"_type":"DV_TEXT","value":"x"}`,
		`{"_type":"DV_TEXT","value":"x","mappings":[]}`,
		`{"_type":"DV_TEXT","value":"x","mappings":null}`,
	} {
		var d rm.DVText
		if err := canjson.Unmarshal([]byte(in), &d); err != nil {
			t.Fatalf("decode %s: %v", in, err)
		}
		out, err := canjson.Marshal(&d)
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		if got := string(out); got != `{"_type":"DV_TEXT","value":"x"}` {
			t.Fatalf("re-encoded %s -> %s, want the absent-key form (collapse documented in wire.md REQ-052)", in, got)
		}
	}
}
```

- [ ] **Step 2: Run it — expect PASS** (it pins today's behaviour)

Run: `go test ./openehr/serialize/canjson/ -run TestDVTextMappingsCollapse -v`
Expected: PASS. If it ever fails, the codec changed and the wire.md note must be revisited.

- [ ] **Step 3: Record the limitation in the spec** — `docs/specifications/wire.md` § REQ-052

Add a note: the canonical decoder cannot distinguish `"mappings": []`, `"mappings": null`, and an absent `mappings` (all yield an empty list; `omitempty` re-drops the key). A caller needing the distinction reads JSON-key presence at the decode boundary, the shape REQ-112 uses for `EHR_STATUS.subject` (`ValidateRMEHRStatusBytes`). `omitempty` is intentionally unchanged: emitting `[]` would be RM-invalid (`Mappings_valid`), emitting `null` a spelling nothing needs.

- [ ] **Step 4: (Optional, if a consumer boundary needs it now) presence helper**

If the consuming floor decodes composition bytes and must flag a present-but-empty `mappings`, add a byte-level read mirroring `openehr/validation/rmfloor_bytes.go:44` (`ValidateRMEHRStatusBytes`) — a targeted key-presence check, not a whole-tree map. Otherwise leave this as the recorded follow-up in Defers.

- [ ] **Step 5: Commit**

```bash
git add openehr/serialize/canjson/mappings_presence_test.go docs/specifications/wire.md
git commit -m "docs(wire): record DV_TEXT.mappings decode collapse; pin it in canjson (REQ-052)"
```

---

### Task 3: RM-floor invariants — `match` value set + `Mappings_valid` (Item C)

**Files:**
- Modify: `openehr/validation/rmfloor.go`
- Test: `openehr/validation/rmfloor_test.go`
- Modify: `docs/specifications/clinical-modeling.md` (REQ-112 clause)

**Interfaces:**
- Consumes: `rm.TermMapping.Match` (a `Character` after Task 1) and `rm.DVText.Mappings`. Reuses the existing `rmFloorWalker.emit(Issue)` and `Issue`/`Result` types (`openehr/validation/issue.go`).
- Produces: two new Error-severity issues from `ValidateRM` / `ValidateComposition` when a term mapping is malformed.

- [ ] **Step 1: Write the failing tests** (`openehr/validation/rmfloor_test.go`)

```go
// REQ-112: the RM floor enforces the TERM_MAPPING.match value set and Mappings_valid.
func TestRMFloorTermMappingMatchValueSet(t *testing.T) {
	txt := &rm.DVText{Value: "x", Mappings: []rm.TermMapping{{
		Match:  rm.Character("x"), // not in {> = < ?}
		Target: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"}, CodeString: "1"},
	}}}
	res := validation.ValidateRM(txt)
	if res.OK() {
		t.Fatalf("expected an invariant issue for match=%q", "x")
	}
}

func TestRMFloorMappingsNotEmptyIfPresent(t *testing.T) {
	txt := &rm.DVText{Value: "x", Mappings: []rm.TermMapping{}} // present, empty -> invalid
	res := validation.ValidateRM(txt)
	if res.OK() {
		t.Fatalf("expected a Mappings_valid issue for a present-but-empty mappings")
	}
}
```

(Adjust `res.OK()` to the real `Result` accessor — confirm against `openehr/validation/issue.go`.)

- [ ] **Step 2: Run — expect FAIL** (`no such issue emitted`)

Run: `go test ./openehr/validation/ -run 'TestRMFloor(TermMappingMatchValueSet|MappingsNotEmpty)' -v`
Expected: FAIL.

- [ ] **Step 3: Add the invariants in `openehr/validation/rmfloor.go`**

Where the walker visits a `*rm.DVText` (add the hook next to the existing per-type invariant checks, e.g. the EHR_STATUS.subject one), emit:

```go
// REQ-112: TERM_MAPPING.match value set (openEHR RM Data Types).
func checkTermMappings(w *rmFloorWalker, path string, ms []rm.TermMapping) {
	if ms != nil && len(ms) == 0 {
		w.emit(Issue{ /* Severity: Error, Code: "mappings_valid",
			Message: "DV_TEXT.mappings present but empty", Path: path */ })
		return
	}
	for i, m := range ms {
		switch m.Match {
		case ">", "=", "<", "?":
		default:
			w.emit(Issue{ /* Severity: Error, Code: "term_mapping_match",
				Message: "TERM_MAPPING.match outside {> = < ?}",
				Path: fmt.Sprintf("%s/mappings[%d]/match", path, i) */ })
		}
	}
}
```

Fill the `Issue` fields from the real struct in `issue.go` (severity/code/message/path). Keep the message value-free (REQ-093) — name the attribute and the allowed set, never echo the bad value.

- [ ] **Step 4: Run — expect PASS**

Run: `go test ./openehr/validation/ -run 'TestRMFloor(TermMappingMatchValueSet|MappingsNotEmpty)' -v`
Expected: PASS.

- [ ] **Step 5: Update the REQ-112 clause** in `docs/specifications/clinical-modeling.md` — the floor **MUST** report a `TERM_MAPPING.match` outside `{'>','=','<','?'}` and a present-but-empty `DV_TEXT.mappings` (`Mappings_valid`) as invariant violations.

- [ ] **Step 6: Commit**

```bash
git add openehr/validation/rmfloor.go openehr/validation/rmfloor_test.go docs/specifications/clinical-modeling.md
git commit -m "feat(validation): RM floor enforces TERM_MAPPING.match value set and Mappings_valid (REQ-112)"
```

---

### Task 4: Pin the `DV_MULTIMEDIA` inline-byte round-trip; record the presence deviation (Item D)

**Files:**
- Test: `openehr/serialize/canjson/multimedia_bytes_test.go` (create)
- Modify: `docs/specifications/wire.md` (or the canjson `doc.go`) — deviation note

**Interfaces:**
- Consumes: nothing from Tasks 1–3; independent.
- Produces: a regression pin proving `DV_MULTIMEDIA.data` / `integrity_check` (`[]byte`) round-trip losslessly as base64, and a one-line deviation record for the `[]byte` presence-collapse.

**Finding that scopes this task:** the FLAT conformance fixture spells inline data as base64 (`…|data": "Z2hnZ2pnamdnag=="`), which is exactly what Go's `[]byte` emits — so **base64 is the correct canonical spelling and no representation change is needed.** What is missing is a *pin*: the canonical composition corpus carries only `uri`/`media_type`/`size`, never inline `data`, so the base64 round-trip is currently unexercised. `data`/`integrity_check` also carry the same `omitempty` presence-collapse as `mappings` (Task 2), recorded here, not fixed (base64 is right; `size` already carries the byte count, so a present-but-empty `data` is low-value).

- [ ] **Step 1: Write the round-trip pin** (`openehr/serialize/canjson/multimedia_bytes_test.go`)

```go
package canjson_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// REQ-052: DV_MULTIMEDIA.data / integrity_check ([]byte) round-trip as base64.
func TestDVMultimediaBytesRoundTrip(t *testing.T) {
	// "ghggjgjggj" -> base64 "Z2hnZ2pnamdnag==" (matches the FLAT conformance fixture).
	in := []byte(`{"_type":"DV_MULTIMEDIA",` +
		`"media_type":{"_type":"CODE_PHRASE","terminology_id":` +
		`{"_type":"TERMINOLOGY_ID","value":"IANA_media-types"},"code_string":"text/plain"},` +
		`"size":10,"data":"Z2hnZ2pnamdnag=="}`)
	var m rm.DVMultimedia
	if err := canjson.Unmarshal(in, &m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(m.Data) != "ghggjgjggj" {
		t.Fatalf("data = %q, want decoded bytes", string(m.Data))
	}
	out, err := canjson.Marshal(&m)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if !bytesContains(out, `"data":"Z2hnZ2pnamdnag=="`) {
		t.Fatalf("encoded form = %s, want base64 data", out)
	}
}
```

(Reuse or add a small `bytesContains` helper, or assert via `strings.Contains(string(out), …)`.)

- [ ] **Step 2: Run — expect PASS** (proves base64 is lossless today)

Run: `go test ./openehr/serialize/canjson/ -run TestDVMultimediaBytesRoundTrip -v`
Expected: PASS. If it fails, the `[]byte` handling changed and REQ-052 must be revisited.

- [ ] **Step 3: Record the deviation** — one line in `docs/specifications/wire.md` § REQ-052 (or canjson `doc.go`): `DV_MULTIMEDIA.data` / `integrity_check` (`[]byte`) encode as base64 (correct canonical spelling) but carry the same `[]`/`null`/absent collapse as `mappings`; not fixed — `size` carries the unencoded byte count, so present-but-empty inline data is not a distinction the SDK needs to preserve.

- [ ] **Step 4: Commit**

```bash
git add openehr/serialize/canjson/multimedia_bytes_test.go docs/specifications/wire.md
git commit -m "test(canjson): pin DV_MULTIMEDIA inline base64 round-trip; record []byte presence deviation (REQ-052)"
```

---

### Task 5: Report `Real` mantissa precision loss on decode (Item E — folded from the memory backlog)

**Files:**
- Modify: `openehr/rm/real.go` (`Real.UnmarshalJSON`)
- Test: `openehr/rm/real_test.go`
- Modify: `openehr/serialize/canjson/doc.go` (drop the "open half … still silent" note), `docs/specifications/wire.md` (§ REQ-052 Floating-point precision)

**Interfaces:**
- Produces: `Real.UnmarshalJSON` returns a typed error when the decimal literal carries more significant digits than `float64` can represent, instead of silently rounding.

**Context:** wire.md § REQ-052 requires a wire value exceeding JSON number precision to be a typed decode error "rather than silently rounding." `canjson/doc.go` records this half as **still open** — overflow (`1e400`) is already typed by the generated wrapper; only mantissa precision loss stays silent. Both `Real.UnmarshalJSON` branches (string `strconv.ParseFloat(s,64)` at `real.go:24`; number `json.Unmarshal(b,&f)` at `real.go:31`) round silently.

- [ ] **Step 1: Failing test.** `Real` decode of `0.12345678901234567890` (20 significant digits) returns an error; `0.5`, `80.5`, and a 17-digit value decode cleanly; `"0.5"` (quoted) too.
- [ ] **Step 2:** Run → today it rounds, `err == nil` → FAIL.
- [ ] **Step 3: Implement.** Add `significantDigits(s string) int` (strip sign / exponent / decimal point / leading zeros; ignore trailing zeros). In both branches, if `significantDigits(...) > 17` return a value-free typed error (REQ-093 — names no magnitude): `fmt.Errorf("rm.Real: %w: value carries more precision than float64 can represent", <sentinel>)`.
- [ ] **Step 4:** Run → PASS. Confirm `DV_QUANTITY` / `DV_PROPORTION` magnitudes (which use `Real`) inherit the check.
- [ ] **Step 5: Spec.** wire.md § REQ-052 clause is now **met**, not deferred; drop the "open half … still silent" note in `canjson/doc.go`.
- [ ] **Step 6: Commit** `fix(rm): Real reports mantissa precision loss on decode (REQ-052)`.

**Decision (named):** the trigger is **>17 significant decimal digits** (float64's shortest-round-trip guarantee) — simple and non-over-reporting: it does **not** reject `0.1` (inexact but round-trips). A `big.Float` exactness check would wrongly reject `0.1` and is not the criterion. Second choice: which sentinel to wrap — a fresh `rm`/`canjson` precision sentinel, or **reuse `canjson.ErrInvalidShape`**. Settled: Plan B ([`archive/2026-09-02-decode-error-surface-typing.md`](archive/2026-09-02-decode-error-surface-typing.md)) landed option A, so the sentinel already has a producer — reuse it rather than minting a second.

---

## Verification (whole bundle)

- [ ] `make codegen-verify` — generated tree matches the generator (Task 1 is the only regen).
- [ ] `make spec-check` — traceability intact; the REQ rows and citations resolve.
- [ ] `make ci` — gofmt/vet/golangci-lint/tests green across `openehr/rm/…`, `openehr/serialize/canjson/…`, `openehr/validation/…`, `internal/bmmgen/…`.
- [ ] Negative space exercised (DoD): empty / multi-char `match` refused on decode **and** encode; malformed `match` and empty-present `mappings` caught by the floor; base64 round-trip lossless.

## Mapping to specs

- [bmm-conformance.md § Primitive type mapping](../specifications/bmm-conformance.md#primitive-type-mapping) — REQ-046, the `Character` row (Task 1).
- [wire.md § REQ-052](../specifications/wire.md#req-052) — canonical `match` round-trip (Task 1), `mappings` collapse note (Task 2), `DV_MULTIMEDIA` base64 (Task 4).
- [clinical-modeling.md § REQ-112](../specifications/clinical-modeling.md#req-112--composition-validation) — RM-floor invariants (Task 3).
- [REQ.md](../specifications/REQ.md) — registry rows for REQ-046 / REQ-052 / REQ-112 (REQ-052 stays `partial`).

## Self-review notes

- **Coverage:** A→Task 1, B→Task 2, C→Task 3, D→Task 4, E (REQ-052 mantissa precision, folded from the backlog)→Task 5. All items mapped.
- **Type consistency:** `Character` is the string type throughout; tests compare `string(tm.Match)`. Task 3's `switch m.Match` compares against string literals — valid because `Character` is a string kind.
- **Decision recorded (Task 1):** representation is `type Character string` (zero value `""` is illegal-and-detectable, per proposal point 5); an incomplete `match` errors on encode (wrapped as `canjson.ErrInvalidValue`) rather than emitting legal-looking JSON. A maintainer preferring "emit and let the floor catch it" should flip Step 3's `MarshalJSON` and lean on Task 3 — but that reintroduces a silent-zero encode, which is the defect being closed, so erroring is the recommended path.
