# Plan — Definition metadata decoding

**Date:** 2026-08-30
**Status:** Draft
**Owner:** SDK maintainers
**Covers:** REQ-144 (new — Definition metadata decoding: timestamp tolerance + empty-list shape)
**Probes:** — (PROBE-093 keeps its catalog entry unchanged; one stale probe-harness comment is corrected)
**Implementation:** planned
**Depends on:** nothing hard. Coordinate textually with two unmerged branches that touch the same files: `origin/feat/datamap` commit `609a104` ("fix(definition): tolerate space-separated created_timestamp in ListTemplates") — this plan generalises that fix, so whichever lands second drops or rebases the duplicate — and `fix/aql-deferred-leftovers` commit `373eeb4` (REQ-057 reserved-name guard in `stored_query.go`).
**Defers:** RM `DV_DATE_TIME` wire format (REQ-052 / REQ-123 territory — untouched); any global zone-less tolerance for `time.Time`; recording stored-query cassettes (none exist today; package tests with inline bodies suffice for this plan); `canxml`/`canjson` codec concerns (nothing here touches the serializers).

## Goal

Two decode defects in the Definition client's metadata leaves, both hit by real deployments. First, `TemplateMetadata.CreatedOn` and `StoredQueryMetadata.Saved` are `time.Time` fields whose decode is stdlib RFC 3339-only, so one zone-less timestamp (EHRbase and other deployments emit values like `2022-03-30T07:18:13.591`, and space-separated variants have been observed — see `feat/datamap` commit `609a104`, where a non-RFC 3339 `created_timestamp` "aborted the whole ListTemplates decode") fails the entire catalog list. Notably the SDK is stricter than its own ground truth for templates: the vendored pin declares `created_timestamp` as a bare `type: string` with no `format` (`resources/its-rest/definition-validation.openapi.yaml:521-522`), while `saved` does declare `format: date-time` (`:3882-3884`) — so tolerance there is deliberate leniency in the ADR 0004 shape. Second, an empty 2xx list body returns a nil slice, which renders as JSON `null` when boxed in an interface — the read-side twin of the typed-nil trap REQ-094 documents for writes. Consumers get: lists that survive zone-less catalogs, and `[]`-not-`null` empty lists.

## Definition of Ready

- **Covers:** REQ-144 only; no ADR — no irreversible fork. The decode-tolerance shape (accept liberally, emit RFC 3339 unchanged) cites [ADR 0004](../adr/0004-numeric-wire-tolerance.md) as precedent in the spec prose rather than opening a new decision record.
- Canonical normative prose for REQ-144 is written in Phase 0 (`wire.md`) and lands with the code in the same PR.
- Phases name their verification: `make spec-check`, `make ci` (or the documented no-Docker fallback: `fmt-check`, `vet`, `spec-check`, `build`, `go test ./... -count=1`).

**Home decision, recorded here because the band note must reflect it:** this takes a **new id in the wire-extensions band (144, first free; 141 is retired-not-free)** rather than amending REQ-143 and REQ-057. Neither existing § covers metadata decoding — REQ-143 is titled and scoped to *list filters* (its only payload clause says filters don't change the return type) and REQ-057 never characterises `StoredQueryMetadata` at all — so "amending" them would bolt the same rule onto two §s whose titles don't own it, duplicating normative prose in two homes. One new § owns both fields and both empty-body arms in one place (single-canonical-home rule, `specifications/README.md`).

## Definition of Done

- Code and tests land with `// REQ-144` citations; guard tests named so removing a guard fails a named test.
- `traceability.yaml` gains the REQ-144 entry; REQ.md registry row + `Impl.` column agree.
- The two indexes `make spec-check` cannot see: the `docs/roadmap.md` Definition row (~line 131, currently `REQ-143, PROBE-093`) gains REQ-144; REQ.md band prose records "144 taken 2026-08-30 …, leaving 145–149 free."
- `wire.md` line 5 (Covers sentence) and the Coverage matrix gain REQ-144.
- `make spec-check` and `make ci` pass.
- Plan archived under `docs/plans/archive/`.

## Implementation checklist

| Step | Status |
|---|---|
| Spec/registry updated (wire.md § REQ-144, REQ.md row + band prose) | ☐ |
| Indexes spec-check misses (roadmap.md Definition row, REQ.md band prose) | ☐ |
| Code (`template.go`, `stored_query.go`) | ☐ |
| Tests with `// REQ-144` comments (incl. first-ever `ListStoredQueries` tests) | ☐ |
| `make spec-check` | ☐ |
| `make ci` | ☐ |

## Phases

### Phase 0 — Spec and registry

**Tasks:**

- [ ] Write **`wire.md` § REQ-144** as `### REQ-144 — Definition metadata decoding` under `## REST leaf operations`, directly after the REQ-143 block (~line 364). Contract points the § makes normative (RFC-2119 wording belongs there):
  - Decode of `TemplateMetadata.created_timestamp` and `StoredQueryMetadata.saved` accepts RFC 3339 (zoned) **and** the zone-less layouts enumerated below; fractional seconds are tolerated on all of them (stdlib `time.Parse` absorbs a fractional-second field after the seconds element even when the layout omits it). Ground truth stated in the §: the vendored pin declares `created_timestamp` as an unformatted string — RFC 3339-only decode over-constrains it — while `saved` is pinned `format: date-time`, so its zone-less tolerance is deliberate leniency beyond the pin (asymmetric-tolerance shape, ADR 0004). Accepted layouts, normatively listed: `time.RFC3339` / `time.RFC3339Nano`; `2006-01-02T15:04:05`; `2006-01-02T15:04`; `2006-01-02 15:04:05` (space-separated, observed in deployments). Zone-less values are interpreted as UTC-naive wall time (whatever `time.Parse` yields — no zone is invented).
  - A JSON `null`, absent key, or empty string yields the zero `time.Time` with no error; a non-empty value matching no accepted layout fails the containing item's decode (and therefore the list call) with an error naming the field — never a silent zero time.
  - Encode is unchanged: RFC 3339 via the existing marshal paths (asymmetric tolerance).
  - `ListTemplates` and `ListStoredQueries` return a **non-nil** zero-length slice and nil error when the 2xx body is empty (the empty-*body* arm; a JSON `[]` already yields a non-nil slice via `encoding/json`). Rationale sentence: a nil slice boxed in a non-nil interface marshals as JSON `null` — the read-side twin of the typed-nil trap REQ-094 documents.
- [ ] Amend `wire.md` line 5 Covers sentence and add the Coverage-matrix row (`openehr/client/definition/`).
- [ ] REQ.md: registry row `| REQ-144 | Definition metadata decoding | [wire.md § REQ-144](wire.md#req-144--definition-metadata-decoding) | planned |` (flipped to `landed` at close-out), plus the band-prose sentence ("144 taken 2026-08-30 by REQ-144 … homed in wire.md beside REQ-143, leaving 145–149 free"; 141 stays retired).
- [ ] `traceability.yaml`: new REQ-144 block — `canonical: docs/specifications/wire.md#req-144--definition-metadata-decoding`, `status: draft`, `implementation: planned` (flipped with the code), `packages: [openehr/client/definition]`, `plans:` this file, `tests:` the two test files below, and a `notes: >-` paragraph recording the pin asymmetry (bare-string `created_timestamp` vs `format: date-time` `saved`) so the leniency rationale is machine-adjacent.

**Definition of done:** `make spec-check` passes with the planned entry; anchors resolve (GitHub-style slugs: `#req-144--definition-metadata-decoding`).

### Phase 1 — Tolerant timestamp decode (both metadata types)

Both structs already have custom `UnmarshalJSON` methods using the alias trick (`template.go:94`, `stored_query.go:31`) — the alias pass is where stdlib RFC 3339-only decoding happens, so the fix is to shadow the timestamp key as `json.RawMessage` inside those existing methods and parse it leniently afterwards. This is the same mechanism as `feat/datamap` commit `609a104`, generalised to one shared helper and both types.

**Tasks:**

- [ ] Failing tests first, in `openehr/client/definition/template_test.go` and `stored_query_test.go` (external `_test` package, stdlib `testing`):
  - `TestListTemplatesZoneLessTimestamp` — 200 body `[{"template_id":"t.v1","created_timestamp":"2022-03-30T07:18:13.591"}]`: nil error, one item, `CreatedOn` non-zero.
  - `TestListTemplatesMixedZoneCatalog` — two items, one `2026-05-17T12:00:00Z` and one zone-less: both returned.
  - `TestListTemplatesSpaceSeparatedTimestamp` — `"2026-06-22 14:50:55"` decodes (port of the `609a104` test).
  - `TestListTemplatesUnparseableTimestampFails` — `"created_timestamp":"not-a-time"`: non-nil error naming `created_timestamp`; never a silent zero. Removing the failure guard must fail this named test.
  - `TestListStoredQueriesZoneLessSaved` and a happy-path `TestListStoredQueries` — the leaf currently has zero tests; the happy path pins `saved` decode (`"2017-07-16T19:20:30.450+01:00"`, the pin's own example shape) alongside the zone-less arm.
- [ ] Implement one shared helper in `openehr/client/definition` (unexported):

```go
// definitionTimestampLayouts are the accepted created_timestamp / saved
// layouts (REQ-144). time.Parse absorbs fractional seconds against the
// second-precision layouts, so no explicit fractional variants are needed.
var definitionTimestampLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02T15:04",
	"2006-01-02 15:04:05",
}

// parseDefinitionTimestamp decodes a metadata timestamp leniently.
// JSON null or an empty string yields the zero time with no error.
func parseDefinitionTimestamp(field string, raw json.RawMessage) (time.Time, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return time.Time{}, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return time.Time{}, fmt.Errorf("%s: %w", field, err)
	}
	if s == "" {
		return time.Time{}, nil
	}
	for _, layout := range definitionTimestampLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("%s: cannot parse %q as a known timestamp layout", field, s)
}
```

  (The `%q` in the failure message carries a catalog timestamp, not clinical data — design-time metadata, outside the REQ-093 PHI discipline; say so in a code comment only if a reviewer asks.)
- [ ] Rework `TemplateMetadata.UnmarshalJSON` (`template.go:94`) to shadow `created_timestamp` as `json.RawMessage` over the alias (the `609a104` shape) and assign via the helper; same for `StoredQueryMetadata.UnmarshalJSON` (`stored_query.go:31`) with `saved`. The `Extras`/known-fields second pass in both methods is untouched (`created_timestamp` and `saved` are already in the known-field sets, so nothing leaks into `Extras`).
- [ ] Confirm the existing pins still hold: `TestTemplateMetadataRoundTrip` (zoned cassette value round-trips; `created_timestamp` not in `Extras`) and `TestListTemplates` (cassette `template_list.json`, both items) — no edits expected.
- [ ] `go test ./openehr/client/definition/... -count=1`.

**Definition of done:** all new named tests pass; existing template tests unchanged; encode side untouched (stdlib RFC 3339 marshalling verified by the round-trip test).

### Phase 2 — Non-nil empty lists

**Tasks:**

- [ ] Failing assertions first: extend `TestListTemplatesEmpty` (`template_test.go:198`, currently nil-agnostic — it only checks `len`) to also assert `got != nil`; add `TestListStoredQueriesEmpty` with the same two assertions. Removing either guard must fail the named test.
- [ ] Change the empty-body arms: `template.go:290-292` returns `[]TemplateMetadata{}, resp.Metadata, nil`; `stored_query.go:277-279` returns `[]StoredQueryMetadata{}, resp.Metadata, nil`.
- [ ] Correct the stale comment on `TestProbe093EmptyCatalogPasses` (`testkit/probes/definition/probes_test.go:146-162`), which documents "ListTemplates yields a nil slice for a 204" — the probe's assertions are shape-agnostic, so only the comment changes; the PROBE-093 catalog entry in `conformance.md` needs no edit (its response clause — "still decodes as the existing template-metadata slice" — remains true).
- [ ] `go test ./openehr/client/definition/... ./testkit/... -count=1`.

**Definition of done:** empty-body 200/204 yields `len == 0 && got != nil` on both leaves; probe harness comment matches behaviour.

### Phase 3 — Close-out

**Tasks:**

- [ ] Flip `traceability.yaml` REQ-144 to `implementation: landed` and the REQ.md `Impl.` cell to `landed`; roadmap Definition row gains REQ-144.
- [ ] Full-tree verification: `make spec-check`; `make ci` (or the documented no-Docker fallback plus PR CI).
- [ ] Flip this plan's **Status:** to done and `git mv` it to `docs/plans/archive/` inside the implementing PR (sdd-archive).

**Definition of done:** the Implementation checklist above is all ☑.

## Mapping to specs

- [wire.md § REQ-144](../specifications/wire.md#req-144--definition-metadata-decoding) (new, Phase 0) — canonical home of both rules.
- [wire.md § REQ-143](../specifications/wire.md#req-143--template-list-filters) — untouched neighbour; its "decoded result remains the same slice" clause is unaffected.
- [wire.md § REQ-057](../specifications/wire.md#req-057) — untouched; stored-query list membership stays there, metadata decoding lives in REQ-144.
- [ADR 0004](../adr/0004-numeric-wire-tolerance.md) — cited precedent for the accept-liberally / emit-RFC 3339 asymmetry; no new ADR.
- [REQ.md registry](../specifications/REQ.md) row REQ-144 + band prose.
