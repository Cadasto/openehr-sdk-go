# Plan — Retire SDK-GAP as a durable identifier

> **For agentic workers:** REQUIRED SUB-SKILL: use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Date:** 2026-07-02
**Status:** Landed
**Owner:** SDK maintainers
**Covers:** No new REQ. **Promotes content into** [REQ-055 / REQ-057](../specifications/wire.md#req-055--wire-boundary) (wire), [REQ-107](../specifications/clinical-modeling.md#req-107--template-driven-rm-instance-example-generator) (instance value-fill), [REQ-083](../specifications/conformance.md#req-083--cadasto-platform-api-conformance) (cadasto health probes). Governed by **[ADR 0012](../adr/0012-retire-sdk-gap-identifier.md)**.
**Probes:** none built here. PROBE-077 / 078 / 079 stay `Deferred` catalog entries.
**Implementation:** landed.
**Depends on:** ADR 0012 committed (`7cb4436`, this branch); the GAP→REQ/PROBE crosswalk in that ADR is the authoritative mapping for every replacement below.
**Defers:** building PROBE-077/078/079; implementing the GAP-14 `medium`/`detail_level` synthesis level (recorded as `Planned`, not built).

## Goal

Remove `SDK-GAP-NN` as a live identifier from the entire mutable tree (docs, `CHANGELOG.md`, and code), after first **promoting the three content-carrying gaps** (16, 14, 07) into the normative REQ layer so nothing is lost. Git history is never rewritten; ADR 0012's crosswalk is the permanent decoder. Delivered as two PRs on branch `chore/retire-sdk-gap-identifier` in worktree `/src/cadasto/openehr-sdk-go-retire-gap`.

## Architecture

Two PRs. **PR1** (small, judgment-heavy) promotes the three durable facts into `wire.md` / `clinical-modeling.md` / `conformance.md`, wires their traceability, and codifies the going-forward rule in the process docs; on merge it flips ADR 0012 to Accepted. **PR2** (large, mechanical, greppable) strips every remaining `SDK-GAP` token — renaming 6 archived plans and 5 test files, replacing code-comment tokens with their governing `REQ`/`PROBE`, and purging doc + CHANGELOG prose — and ends with a whole-tree grep proving the token survives only where it *describes its own retirement*.

## Global Constraints

- **Worktree only.** All edits happen in `/src/cadasto/openehr-sdk-go-retire-gap` on `chore/retire-sdk-gap-identifier`. Never touch the shared tree at `/src/cadasto/openehr-sdk-go`.
- **Never rewrite git history.** Commit messages retain their tokens by design.
- **Authoritative mapping.** Every GAP→REQ/PROBE replacement uses the crosswalk table in [ADR 0012](../adr/0012-retire-sdk-gap-identifier.md#crosswalk--sdk-gap-nn--req--probe-permanent-decoder). Do not invent mappings.
- **Replace, don't delete, in code.** A stripped `(SDK-GAP-NN)` in a code comment becomes its governing `(REQ-NNN)` / `(PROBE-NNN)` — traceability is preserved.
- **Legitimate residual (the ONLY files that keep the token).** The token survives *only* where the subject is the retirement itself: `docs/adr/0012-retire-sdk-gap-identifier.md`, this plan, the ADR index row in `docs/adr/README.md`, and the one-line going-forward rule in `AGENTS.md` + `docs/development-process.md`. Everywhere else → zero.
- **No normative prose duplication.** New RFC-2119 text lands in the canonical topic spec; `REQ.md` gets a registry row/status only, never duplicated prose (AGENTS.md rule).
- **Verification per task:** `make spec-check` for doc/traceability changes; `go build ./... && go test ./... && go vet ./...` for code/test changes; `make ci` (includes spec-check) as the final gate on each PR. `make` is the single entry point.
- **Commits:** Conventional Commits; each commit ends with the `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>` trailer.

## Definition of Ready

- [x] `Covers:` / promotion targets identified (REQ-055/057/107/083).
- [x] ADR 0012 committed with the authoritative crosswalk.
- [x] Source-of-truth for the 3 promoted facts read: `openehr/instance/options.go` (ValueFill/ValueSource), `openehr/client/query/execute.go` + `openehr/client/definition/stored_query.go` (GAP-16), `cadasto/admin/doc.go` (GAP-07).
- [x] Phases name their verification command.

## Definition of Done

- [ ] The three facts are normative in `wire.md` / `clinical-modeling.md` / `conformance.md`; `traceability.yaml` + `REQ.md` reflect them.
- [ ] Going-forward rule codified in `AGENTS.md` + `development-process.md`.
- [ ] ADR 0012 Status = Accepted (PR1 merge).
- [ ] Whole-tree grep (Task 11) returns only the legitimate residual set.
- [ ] `make ci` passes on both PRs.
- [ ] This plan archived under `docs/plans/archive/` after PR2 lands.

## Implementation checklist

| Step | Status |
|---|---|
| PR1 — 3 promotions + traceability + process docs + ADR→Accepted | |
| `make spec-check` (PR1) | |
| PR2 — archived-plan renames + link fixes | |
| PR2 — test-file + ident renames | |
| PR2 — code-comment token→REQ/PROBE replacement | |
| PR2 — doc + CHANGELOG prose purge + anchor churn | |
| Whole-tree grep clean (Task 11) | |
| `make ci` (PR2) | |
| Plan archived | |

---

# PR1 — Promotions + process docs

### Task 1: Promote the GAP-16 wire facts into wire.md (REQ-055 + REQ-057)

**Files:**
- Modify: `docs/specifications/wire.md` (REQ-055 "AQL executor" §, after line ~218; REQ-057 §, after line ~229)
- Modify: `docs/specifications/traceability.yaml` (REQ-055, REQ-057 `notes:`)

**Interfaces produced:** two new normative anchors of behaviour that Task 8/9 will point code comments and roadmap notes at, replacing GAP-16.

- [ ] **Step 1: Add the verb-aware EHR-scoping paragraph to the REQ-055 "AQL executor" subsection** (immediately after the "AQL injection" paragraph, currently ending line 218):

```markdown
**EHR scoping (verb-aware).** When execution is scoped to a single EHR, the SDK **MUST** apply the scope by the verb-appropriate mechanism the ITS-REST OAS declares: `GET /query/aql/{qualified_query_name}` carries the `ehr_id` **query parameter**; `POST /query/aql` carries the **`openehr-ehr-id` request header** — the POST operations declare no `ehr_id` query parameter and the request body carries no `ehr_id` field, so the header is the only channel. The SDK **MUST NOT** scope POST via the query parameter: a strict-spec server that honours only the header would otherwise run the query population-wide.
```

- [ ] **Step 2: Add the store-response version-recovery paragraph to REQ-057** (after the qualified-name paragraph, currently ending line 229):

```markdown
**Store-response version recovery.** The Definition store operation (`PUT /definition/query/{qualified_query_name}[/{version}]`) returns the server-assigned `{name, version}` in a **`Location` response header** with an empty body — the canonical `200_StoredQuery_stored` OAS shape, and what a `text/plain` store returns. The SDK **MUST** recover the assigned identifier in order: (1) parse the `Location` header (canonical); (2) decode a JSON body if present (lenient — some deployments return one); (3) fall back to the caller's input `{name, version}` (graceful degradation). A malformed `Location` **MUST NOT** fail the call — it falls through to (2)/(3).
```

- [ ] **Step 3: Add trace notes** — in `traceability.yaml`, on the REQ-055 and REQ-057 entries add a `notes:` line each: `notes: "POST exec scopes via openehr-ehr-id header (GET uses ehr_id query param); PutStoredQuery recovers {name,version} from Location header. Verified by query/execute.go, definition/stored_query.go; PROBE-078/079 deferred."`

- [ ] **Step 4: Verify**

Run: `cd /src/cadasto/openehr-sdk-go-retire-gap && make spec-check`
Expected: PASS (no new orphan REQ/PROBE; REQ-055/057 still resolve).

- [ ] **Step 5: Commit**

```bash
git add docs/specifications/wire.md docs/specifications/traceability.yaml
git commit -m "docs(wire): promote verb-aware EHR scoping + stored-query Location recovery into REQ-055/057

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

### Task 2: Promote the GAP-14 ValueFill seam into clinical-modeling.md (REQ-107) + record the deferred `medium` level

**Files:**
- Modify: `docs/specifications/clinical-modeling.md` (new subsection inside REQ-107, after the "### Contract" block ends at line ~450, before "### Trust model" line 446 — insert as a new `###` subsection after Contract)
- Modify: `docs/roadmap.md` (add a `Planned` row for the `medium`/`detail_level` follow-up)
- Modify: `docs/specifications/traceability.yaml` (REQ-107 `notes:`)

- [ ] **Step 1: Insert the value-fill subsection** into REQ-107 (place after the `### Contract` section, before `### Trust model`):

```markdown
### Primitive-leaf value fill

`Policy` selects *which* nodes are materialised; an orthogonal **`ValueFill`** selects *how* primitive leaves are valued. The SDK **MUST** offer two fills: `ExampleFill` (default) populates each leaf with its REQ-103 `PrimitiveConstraint.ExampleValue` — a single representative value, byte-identical across calls for one OPT; `RandomFill` draws each leaf from within its constraint (in-range magnitudes, value-set-member codes, enumeration entries), valid by construction and varying between calls. A `ValueFill` other than `RandomFill` **MUST** degrade to `ExampleFill` rather than error.

`RandomFill` reproducibility is caller-controlled via **`Options.ValueSource`** (a `math/rand/v2.Source`): a fixed source makes leaf values byte-reproducible; `nil` draws from the auto-seeded package global so successive calls differ — mirroring the `UIDSource` determinism seam. A `Source` is not safe for concurrent use: each concurrent `Generate` **MUST** own its source (or leave it `nil` for the concurrency-safe global). The composition builder surfaces the seam as `composition.WithValueFill` / `composition.WithValueSource`.

**Deferred.** A third `medium` / `detail_level` structural level — a representative optional-subset fill between `Minimal` and full population — is planned but not delivered; it is **not** part of the v1 `ValueFill` contract. Tracked in the roadmap.
```

- [ ] **Step 2: Add the roadmap Planned row.** In `docs/roadmap.md`, in the instance-synthesis area, add a row:

```markdown
| Synthesis `medium`/`detail_level` level | **Planned** | `openehr/instance/` REQ-107 | Representative optional-subset fill between `Minimal` and full population; deferred follow-up |
```

- [ ] **Step 3: Trace note.** On REQ-107 in `traceability.yaml` add: `notes: "ValueFill{ExampleFill,RandomFill} + Options.ValueSource seam (composition.WithValueFill/WithValueSource). medium/detail_level level deferred."`

- [ ] **Step 4: Verify**

Run: `make spec-check`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add docs/specifications/clinical-modeling.md docs/roadmap.md docs/specifications/traceability.yaml
git commit -m "docs(clinical-modeling): promote ValueFill/ValueSource seam into REQ-107; record deferred medium level

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

### Task 3: Promote the GAP-07 health-probe contract into conformance.md (REQ-083)

**Files:**
- Modify: `docs/specifications/conformance.md` (new `####` subsection under REQ-083, after "**Status: planned.**" line 52, before "### Vendored cassettes" line 54)
- Modify: `docs/specifications/traceability.yaml` (REQ-083: add `cadasto/admin` packages + tests; `notes:`)
- Modify: `docs/specifications/module-layout.md` (the `cadasto/admin/` line: point "healthcheck" at REQ-083 instead of leaving it uncontracted)

- [ ] **Step 1: Insert the health-probe subsection** (lift from `cadasto/admin/doc.go:4-23`, made normative):

```markdown
#### Health probes (`cadasto/admin`)

`cadasto/admin` exposes deployment liveness/readiness probes (`Live`, `Ready`) with a wire contract independent of the (planned) Cadasto platform API surface:

- **Default paths** `DefaultLivePath = "/health/live"`, `DefaultReadyPath = "/health/ready"`, each overridable per call via `WithLivePath` / `WithReadyPath` (e.g. `/healthz`).
- **URL derivation.** The probe URL derives from the **origin (scheme + host) of the openEHR REST service entry** — the openEHR REST API path prefix is **NOT** inherited.
- **Public / no auth.** Health endpoints are public: the SDK **MUST NOT** send an `Authorization` header on a probe.
- **Status mapping** (`errors.Is`-compatible): `2xx → nil`; `401 → transport.ErrUnauthorized`; `403 → transport.ErrForbidden`; `404 → transport.ErrNotFound`; `5xx → transport.ErrServerError`. Other non-2xx codes (400, 405, 408, 429, …) surface as a plain formatted error with no sentinel.
- Probes borrow `transport`'s sentinel taxonomy (REQ-093) but **bypass** `transport.Client.Do` — no openEHR error-envelope decoding, no OTel spans, no retries. Use `openehr/client/admin` when those concerns matter.

**Status:** landed (`cadasto/admin`). Distinct from the ITS-REST Admin client (`openehr/client/admin`). Platform-API conformance fixtures remain Phase 4 per above.
```

- [ ] **Step 2: Trace + module-layout.** In `traceability.yaml` REQ-083, add `packages: [cadasto/admin]`, `tests: [cadasto/admin/probes_test.go]`, and `notes: "Health probes (Live/Ready) landed: default paths, WithLivePath/WithReadyPath, origin-derived URL, public/no-auth, status→sentinel map. Platform-API fixtures Phase 4."`. In `module-layout.md`, change the `cadasto/admin/` description to reference REQ-083 for the healthcheck contract.

- [ ] **Step 3: Verify**

Run: `make spec-check`
Expected: PASS (REQ-083 now has packages/tests).

- [ ] **Step 4: Commit**

```bash
git add docs/specifications/conformance.md docs/specifications/traceability.yaml docs/specifications/module-layout.md
git commit -m "docs(conformance): promote cadasto/admin health-probe contract into REQ-083

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

### Task 4: Codify the going-forward rule + accept ADR 0012

**Files:**
- Modify: `AGENTS.md` (§ "Spec-driven workflow (agents)")
- Modify: `docs/development-process.md` (identifier conventions section)
- Modify: `docs/adr/0012-retire-sdk-gap-identifier.md` (Status → Accepted)
- Modify: `docs/adr/README.md` (0012 row → Accepted)

- [ ] **Step 1: Add the rule to AGENTS.md** — one line in the spec-driven-workflow section:

```markdown
- **`REQ`/`PROBE` is the feature register; there is no `SDK-GAP` identifier.** A discovered gap is worked under a REQ (extend or create via `sdd-specify`) with a `PROBE` for wire conformance. A GAP-style label may appear only as an ephemeral in-flight plan filename — never in `traceability.yaml`, test names, `doc.go`, or normative prose. See [ADR 0012](docs/adr/0012-retire-sdk-gap-identifier.md).
```

- [ ] **Step 2: Add the same rule (fuller)** to `docs/development-process.md` wherever identifier conventions / DoR-DoD live, linking ADR 0012 as the rationale + crosswalk.

- [ ] **Step 3: Accept the ADR.** In `docs/adr/0012-…md` set `**Status:** Accepted, 2026-07-02.` (drop the "Proposed / becomes Accepted" wording). In `docs/adr/README.md` change the 0012 row Status to `Accepted (2026-07-02)`.

- [ ] **Step 4: Verify**

Run: `make spec-check` and confirm links resolve: `grep -n "0012-retire-sdk-gap-identifier" AGENTS.md docs/development-process.md docs/adr/README.md`
Expected: spec-check PASS; each grep prints the new link.

- [ ] **Step 5: Commit + open PR1**

```bash
git add AGENTS.md docs/development-process.md docs/adr/0012-retire-sdk-gap-identifier.md docs/adr/README.md
git commit -m "docs: codify REQ/PROBE-is-the-register rule; accept ADR 0012

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
make ci    # full PR1 gate
```

Open PR1 (`gh pr create`) titled `docs: retire SDK-GAP — promote gap-16/14/07 into REQ, accept ADR 0012`. **PR1 stops here.** PR2 tasks below run after PR1 is reviewed (they may proceed on the same branch stacked, or after merge — maintainer's choice).

---

# PR2 — Mechanical purge + renames

### Task 5: Rename the 6 gap-named archived plans + fix inbound links

**Files (git mv, exact):**
- `docs/plans/archive/2026-06-19-sdk-gap-12-newskeleton.md` → `…/2026-06-19-realworld-opt-synthesis.md`
- `docs/plans/archive/2026-06-23-sdk-gap-13-polymorphic-encode-decode.md` → `…/2026-06-23-polymorphic-encode-decode.md`
- `docs/plans/archive/2026-06-23-sdk-gap-14-seeded-synthetic-generation.md` → `…/2026-06-23-seeded-synthetic-generation.md`
- `docs/plans/archive/2026-06-29-sdk-gap-15-rm-floor-validation.md` → `…/2026-06-29-rm-floor-validation.md`
- `docs/plans/archive/2026-06-29-sdk-gap-16-stored-query-rest-conformance.md` → `…/2026-06-29-stored-query-rest-conformance.md`
- `docs/plans/archive/2026-06-29-sdk-gap-17-aql-execution-ast.md` → `…/2026-06-29-aql-execution-ast.md`

- [ ] **Step 1: git mv all six.**

```bash
cd /src/cadasto/openehr-sdk-go-retire-gap/docs/plans/archive
git mv 2026-06-19-sdk-gap-12-newskeleton.md 2026-06-19-realworld-opt-synthesis.md
git mv 2026-06-23-sdk-gap-13-polymorphic-encode-decode.md 2026-06-23-polymorphic-encode-decode.md
git mv 2026-06-23-sdk-gap-14-seeded-synthetic-generation.md 2026-06-23-seeded-synthetic-generation.md
git mv 2026-06-29-sdk-gap-15-rm-floor-validation.md 2026-06-29-rm-floor-validation.md
git mv 2026-06-29-sdk-gap-16-stored-query-rest-conformance.md 2026-06-29-stored-query-rest-conformance.md
git mv 2026-06-29-sdk-gap-17-aql-execution-ast.md 2026-06-29-aql-execution-ast.md
```

- [ ] **Step 2: Find every inbound link to the old names.**

Run: `cd /src/cadasto/openehr-sdk-go-retire-gap && grep -rIn 'sdk-gap-1[234567]' docs/ --include='*.md' --include='*.yaml'`
This lists the ~15 links in `docs/roadmap.md`, `docs/specifications/traceability.yaml`, `docs/plans/README.md`, `docs/plans/archive/README.md`, and any spec cross-links. Update each path to its new name (drop the `sdk-gap-NN-` infix per the map above).

- [ ] **Step 3: Strip GAP tokens from ALL archived plan bodies** (`docs/plans/archive/*.md`) — not only the six renamed ones. The non-gap-named archived plans also cite SDK-GAP in prose: `2026-05-26-rm-polymorphic-decode-coverage.md`, `2026-05-26-contribution-submission-shape.md`, `2026-05-25-req094-prefer-followups.md`, `2026-05-27-rm-like-interface-ergonomics.md`, `2026-05-15-rest-api-client.md`, `2026-06-11-contribution-update-audit-dv-coded-text.md` (and the archived-plan `README.md`). Replace each token with its governing REQ/PROBE per the ADR crosswalk. For the renamed six, that includes the title line (`# Plan — SDK-GAP-NN: …` → `# Plan — REQ-NNN: …`; `Covers:` already carries the REQ). Read each body in context so narrative sentences stay grammatical (e.g. "SDK-GAP-16 finding A" → "the verb-aware-scoping fix (REQ-055)"). These are frozen delivery records: change only the identifier tokens, not the substance.

- [ ] **Step 4: Verify** no dangling old paths, links resolve, and no token survives in the plans tree.

Run: `grep -rInE 'SDK-GAP|sdk-gap' docs/plans/ | grep -vE '2026-07-02-retire-sdk-gap'`
Expected: empty (no old filenames, no tokens in any archived body; this plan file is the only permitted residual and is excluded).
Run: `make spec-check` → PASS.

- [ ] **Step 5: Commit**

```bash
git add -A docs/
git commit -m "docs(plans): rename gap-named archived plans to descriptive names; fix inbound links

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

### Task 6: Rename the 5 gap-tokened test files + in-file identifiers

**Files (git mv, exact):**
- `openehr/instance/gap12_test.go` → `openehr/instance/realworld_opt_synthesis_test.go`
- `openehr/instance/gap13_test.go` → `openehr/instance/polymorphic_roundtrip_test.go`
- `openehr/instance/gap14_test.go` → `openehr/instance/valuefill_test.go`
- `openehr/validation/rmread/read_gap12_test.go` → `openehr/validation/rmread/read_datavalues_test.go`
- `internal/templateinstance/rmwrite/write_gap12_test.go` → `internal/templateinstance/rmwrite/write_datavalue_test.go`

**Identifiers (rename across all usages — they cross files):**
- `compileGAP12Fixture` → `compileRealWorldFixture` (def in the renamed `realworld_opt_synthesis_test.go`; used in `polymorphic_roundtrip_test.go`, `testkit/probes/instance/probes_test.go`)
- `gap14CounterUID` → `counterUID` (def in `valuefill_test.go`; used in `polymorphic_roundtrip_test.go`)
- `TestGAP13_CorpusRoundTripValidates` → `TestCorpusRoundTripValidates`
- `TestReadSingle_DataValuesGAP12` / `…GAP12_unknownAttr` → drop the `GAP12` suffix
- `TestProbe027_GAP12Corpus` → `TestProbe027_RealWorldCorpus` (`testkit/probes/instance/probes_test.go`)
- `gap19Query` const → `standingPredicateQuery` (`openehr/aql/parse/structured_test.go`)

- [ ] **Step 1: git mv the 5 files** (commands mirror Task 5 pattern with the paths above).

- [ ] **Step 2: Rename identifiers.** For each identifier above run a scoped replace and confirm no stragglers, e.g.:

```bash
cd /src/cadasto/openehr-sdk-go-retire-gap
grep -rln 'compileGAP12Fixture' --include='*.go' | xargs sed -i 's/compileGAP12Fixture/compileRealWorldFixture/g'
grep -rln 'gap14CounterUID'      --include='*.go' | xargs sed -i 's/gap14CounterUID/counterUID/g'
# …repeat per identifier; then verify none remain:
grep -rInE 'GAP1[0-9]|gap1[0-9]|gap19Query' --include='*.go' .
```
Expected final grep: empty.

- [ ] **Step 3: Verify build + tests green** (the renamed tests must still run and pass).

Run: `go build ./... && go vet ./... && go test ./openehr/instance/... ./openehr/validation/... ./internal/templateinstance/... ./openehr/aql/... ./testkit/probes/instance/...`
Expected: PASS; the renamed test functions appear in `-v` output under their new names.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "test: rename gap-tokened test files and identifiers to REQ/behaviour names

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

### Task 7: Replace SDK-GAP tokens in code comments with their governing REQ/PROBE

Use the ADR crosswalk. Replacement map by package (comment token → replacement):

| Files | GAP(s) | Replace token with |
|---|---|---|
| `internal/bmmgen/{render_jsonunmar,render_xmlunmar,render_jsonmar,render,plan}.go` (+ `render_jsonunmar_polymorphic_test.go`) | 11, 13 | `REQ-052` |
| `openehr/rm/{like_interfaces,like_accessors,doc}.go` (+ `like_interfaces_test.go`) | 11, 13 | `REQ-052` (doc.go headings: `# … (SDK-GAP-11)` → `# … (REQ-052)`) |
| `openehr/serialize/canxml/decode.go`, `openehr/serialize/canjson/polymorphic_{encode,decode}_test.go` | 11, 13 | `REQ-052` / `PROBE-038` |
| `openehr/internal/jsonpoly/jsonpoly.go` (+ test) | 13 | `REQ-052` |
| `openehr/aql/parse/{parse,extract_query,ast,query,query_test,parse_test,ast_test,roundtrip_test}.go`, `openehr/aql/{where,value,path,errors,introspect_test}.go` | 17, 19 | `REQ-113` (round-trip: `PROBE-080`; standing predicate: `PROBE-082`) |
| `openehr/validation/{rmfloor,rmfloor_bytes,rmread/read}.go` + those `_test.go` | 12, 15, 18 | `REQ-112` (leaf-read: `REQ-110`; bytes: `PROBE-081`) |
| `openehr/validation/walk_composition.go:600` (combined `SDK-GAP-11/12`) | 11, 12 | `REQ-052 / REQ-110` (both) |
| `openehr/instance/{options,generate,sample}.go` (+ `sample_test.go`) | 12, 14 | `REQ-107` |
| `openehr/composition/{options,skeleton}.go` | 14 | `REQ-107` |
| `openehr/client/query/execute.go` | 16 | `REQ-055` |
| `openehr/client/definition/stored_query.go` (+ test) | 16 | `REQ-057` |
| `openehr/client/ehr/composition/composition.go` (+ test), `.../directory/directory.go` (+ test) | 09 | `REQ-094` |
| `openehr/client/ehr/contribution/{contribution,submission,doc}.go` (+ test) | 10 | `REQ-050 / REQ-095` (doc.go: "closes SDK-GAP-10 … SDK-GAP-09 fix" → "REQ-050/095 … REQ-094") |
| `transport/client.go:94` (stray) | 07 | `REQ-083` |
| `testkit/probes/versioned/probe_071_*.go`, `probe_072_*.go`, `probes_test.go`; `testkit/probes/serialize/probe_038_*.go`, `probes_test.go`; `testkit/probes/instance/probes_test.go`; `testkit/fixtures/discover.go`; `cadasto/admin/{doc,probes}.go`, `probes_test.go`; `cmd/examples/aql-parse-structured/main.go` | per crosswalk | the file's own `PROBE-NNN`/`REQ` (071→REQ-094, 072→REQ-050/095, 038→REQ-052; admin→REQ-083; aql example→REQ-113) |

- [ ] **Step 1: Apply replacements** package-group by package-group (edit, don't blind-sed — several are prose sentences that must read naturally, e.g. the `contribution/doc.go` line and `cadasto/admin/doc.go` heading). Prefer `Edit` on each occurrence found by:

Run: `grep -rInE 'SDK-GAP' --include='*.go' .`

- [ ] **Step 2: Verify no code token remains + build/tests/vet green.**

```bash
grep -rInE 'SDK-GAP|GAP1[0-9]|gap1[0-9]' --include='*.go' .   # expect: empty
go build ./... && go vet ./... && go test ./...               # expect: PASS
```
(`go test ./...` reconfirms the `// REQ-`/`// PROBE-` citation swaps didn't disturb behaviour.)

- [ ] **Step 3: Commit**

```bash
git add -A
git commit -m "refactor(comments): replace SDK-GAP citations with governing REQ/PROBE tokens

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

### Task 8: Purge SDK-GAP prose from living docs + fix the conformance/wire anchor churn

**Files (from census):** `docs/roadmap.md` (14), `docs/specifications/conformance.md` (15), `docs/specifications/traceability.yaml` (8), `docs/specifications/clinical-modeling.md` (8 — incl. the REQ-107 lines 442/454 and REQ-112 `SDK-GAP-18` heading, REQ-113 `SDK-GAP-19` heading), `docs/specifications/wire.md` (4), `docs/specifications/transport.md` (1), `docs/specifications/module-layout.md` (1), `docs/specifications/research-strands.md` (2 — de-GAP the STRAND-13 prose), `docs/examples.md` (1), `docs/plans/README.md` (1), `docs/plans/archive/README.md` (9), `testkit/cassettes/README.md` (1), `docs/adr/0001-bmm-version-bump-runbook.md` (1). Replace each token with its governing REQ/PROBE (crosswalk); rewrite the roadmap/README mega-lines (which bundle up to 5 GAPs) into prose that names each REQ inline.

- [ ] **Step 1: Anchor churn (do heading + inbound links together).** In `conformance.md`, rename the three PROBE headings that embed a token:
  - `#### PROBE-038 … (SDK-GAP-11)` → drop the ` (SDK-GAP-11)` suffix
  - `#### PROBE-071 … (SDK-GAP-09)` → drop suffix
  - `#### PROBE-072 … (SDK-GAP-10)` → drop suffix

  Then fix the inbound links whose anchors change: `grep -n 'probe-038\|probe-071\|probe-072\|sdk-gap' docs/specifications/wire.md` (lines ~123/176/177) and update each `#…-sdk-gap-NN` anchor to the new token-free anchor. Re-grep the whole `docs/` tree for any other link to those old anchors.

- [ ] **Step 2: Replace remaining prose tokens** file by file (Edit, reading each in context so mega-lines stay grammatical). The `research-strands.md` STRAND-13 entry: reword so the strand no longer frames itself as "SDK-GAP-13" — reference REQ-052 and the archived plan by its new name.

- [ ] **Step 3: Keep the deferred probes honest (do not delete them with the GAP prose).** Ensure `conformance.md` carries an explicit catalog entry for each of PROBE-077, PROBE-078, PROBE-079 with `**Status:** Deferred` and a `**Satisfies:**` line (077 → REQ-112; 078/079 → REQ-055/057). If an entry exists only as an inline GAP mention, convert it to a proper deferred catalog stub; if it is missing, add the stub. They must remain discoverable as planned-but-unbuilt after the purge.

- [ ] **Step 4: Verify.**

```bash
grep -rInE 'SDK-GAP|sdk-gap' docs/ testkit/ --include='*.md' --include='*.yaml' \
  | grep -vE 'docs/adr/0012-retire-sdk-gap|docs/plans/2026-07-02-retire-sdk-gap|docs/adr/README\.md|docs/development-process\.md'
# expect: empty
make spec-check   # expect: PASS
```

- [ ] **Step 5: Commit**

```bash
git add -A docs/ testkit/
git commit -m "docs: purge SDK-GAP tokens from living specs/roadmap/traceability; fix anchor churn

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

### Task 9: Purge SDK-GAP from CHANGELOG.md

**Files:** `CHANGELOG.md` (17 tokens across released-minor summaries).

- [ ] **Step 1: Rewrite each released entry** so the token is replaced by its governing REQ/PROBE inline (crosswalk). Do **not** restructure or re-date releases — only swap the identifier within the existing sentence (e.g. "template-less RM validation floor (SDK-GAP-15)" → "template-less RM validation floor (REQ-112)").

- [ ] **Step 2: Verify.**

```bash
grep -nE 'SDK-GAP|sdk-gap' CHANGELOG.md   # expect: empty
```

- [ ] **Step 3: Commit**

```bash
git add CHANGELOG.md
git commit -m "docs(changelog): replace SDK-GAP references with governing REQ/PROBE

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

### Task 10: Whole-tree verification + PR2 gate

- [ ] **Step 1: The one grep that must come back clean** (only the retirement-describing residual survives):

```bash
cd /src/cadasto/openehr-sdk-go-retire-gap
grep -rInE 'SDK-GAP|sdk-gap|GAP[0-9]|gap[0-9]' . \
  --include='*.go' --include='*.md' --include='*.yaml' --include='*.yml' \
  | grep -vE '/\.git/'
```
Expected surviving hits — confirm each is one of: `docs/adr/0012-retire-sdk-gap-identifier.md` (the decoder), `docs/plans/2026-07-02-retire-sdk-gap-identifier.md` (this plan), the ADR row in `docs/adr/README.md`, and the going-forward rule lines in `AGENTS.md` + `docs/development-process.md`. **Any other hit is a miss — go fix it.**

- [ ] **Step 2: No gap-named files remain.**

```bash
find . -path ./.git -prune -o -name '*gap*' -print | grep -vE 'retire-sdk-gap'
```
Expected: empty.

- [ ] **Step 3: Full CI gate.**

Run: `make ci`
Expected: PASS (build, vet, tests, `make spec-check`).

- [ ] **Step 4: Archive this plan + open PR2.**

```bash
git mv docs/plans/2026-07-02-retire-sdk-gap-identifier.md docs/plans/archive/2026-07-02-retire-sdk-gap-identifier.md
# update docs/plans/README.md + archive/README.md to list it under "Landed (archived)"
git add -A docs/plans/
git commit -m "docs(plans): archive the SDK-GAP retirement plan

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```
Open PR2 titled `chore: purge SDK-GAP tokens from the mutable tree (rename plans/tests, comments→REQ)`.

## Mapping to specs

- [ADR 0012](../adr/0012-retire-sdk-gap-identifier.md) — decision + authoritative crosswalk.
- [wire.md § REQ-055 / § REQ-057](../specifications/wire.md) — GAP-16 promotion target.
- [clinical-modeling.md § REQ-107](../specifications/clinical-modeling.md#req-107--template-driven-rm-instance-example-generator) — GAP-14 promotion target.
- [conformance.md § REQ-083](../specifications/conformance.md#req-083--cadasto-platform-api-conformance) — GAP-07 promotion target.
- [traceability.yaml](../specifications/traceability.yaml) + [REQ.md](../specifications/REQ.md) — registry/trace updates.
