# Plan — openEHR terminology vocabulary (REQ-034): pinned source, generated accessor, one home per code

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (or superpowers:executing-plans inline) to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax. Before any Go edit load `go-coding:go-coding`, then `go-layout` (Task 2 — a new package and exported API), `go-testing` (every `_test.go`), `go-errors` (Tasks 3–5), `go-idioms` (iterators, `slices`, `maps`).

**Date:** 2026-09-07
**Status:** landed (2026-09-07, archived in the implementing PR)
**Owner:** SDK maintainers
**Covers:** [REQ-034](../../specifications/rm-modeling.md#openehr-terminology-vocabulary-req-034) (new — `planned` → `landed` in this plan's PR); implementation-aligned amendments to landed [REQ-130](../../specifications/wire.md#req-130--contribution-builder) (change-type and lifecycle-state sentences) and [REQ-013](../../specifications/module-layout.md#req-013--building-block-independence) (the package set gains `openehr/terminology`)
**Probes:** none — nothing crosses the wire; the guard tests carry the bar
**Implementation:** landed
**Depends on:** `resources/` pin discipline (REQ-041, [ADR 0001](../../adr/0001-bmm-version-bump-runbook.md) pattern), `cmd/bmmgen` / `internal/bmmgen` (the `-verify` drift pattern to mirror), the landed consumers in `openehr/client/ehr`, `openehr/client/ehr/contribution`, `openehr/serialize/simplified`, `openehr/instance`
**Defers:** wiring the template-less RM floor's coded invariants (REQ-112: `Setting_valid`, `Category_validity`, `Change_type_valid`, `Mode_valid`, `Normal_status_validity`, …) to the accessor — a follow-up plan; membership validation of a bare `|normal_status` code on FLAT decode (today lenient, stays lenient); the two `openehr_term_3.1.0.bmm.json` service *interfaces* in `resources/bmm/` (unrelated — that BMM models the terminology *service* API, not the vocabulary)

**Goal:** Vendor the openEHR Terminology (`openehr_terminology.xml`, TERM Release-3.0.0) as a pinned, sha256-recorded asset; generate a stdlib-only `openehr/terminology` accessor from it, drift-detected in `make test`; and re-point every hand-typed `openehr` code table in the SDK at it, so a code and its rubric have exactly one home.

**Architecture:** `resources/terminology/` follows `resources/its-rest/` (byte-identical file + `MANIFEST.txt` with ref / commit / sha256 + sync script + offline `-verify` target in `make ci`). `internal/termgen` + `cmd/termgen` follow `internal/bmmgen` + `cmd/bmmgen`: parse the pin, render one `go/format`-clean `openehr_gen.go`, `-verify` diffs against disk and exits 1 on drift, `make test` depends on `termgen-verify`. The hand-written half of `openehr/terminology` is the `Group` / `CodeSet` types, their nil-safe lookup methods and the registry accessors; the generated half is the 17 group and 3 code-set variables plus `Version` / `SourceSHA256`. Consumers keep their public typed enums (`ehr.LifecycleState`, `contribution.ChangeType`) but validity becomes group membership and rubrics come from the pin; the `simplified` codec's 32-row participation-mode table and its `ctx/` defaults, and the instance generator's setting / category defaults, read the same tables.

**Why vendor + generate rather than centralise constants by hand:** the pin is 13.9 KB and the generator is a couple of hundred lines; in exchange the 249 concepts are mechanically derived rather than transcribed, and the one hand-typed pair that *was* wrong — `flat_decode.go`'s `math_function` default spelled `146|actual` where the terminology says `146` is *mean* and *actual* is `640` — becomes the class of defect a test over the tables catches (Task 5 fixes it).

**Tech Stack:** Go 1.27.0 (module floor, REQ-002), `encoding/xml` (generator only — a tool, not library code), `go/format`, `iter` / `slices` / `maps`, stdlib `testing`. Lint: `golangci-lint` v2.13.2 (host binary is 1.27-built, `make lint` also routes through the pinned image), `gofmt` from `$(go env GOROOT)/bin`.

> **Reading note.** Where a task quotes prose destined for another file, its cross-references are rendered as code (`§ REQ-034`) rather than links: the real links are relative to the destination file.

## Global Constraints

- **REQ-034:** `openehr/terminology` is stdlib-only — it imports **no** `github.com/cadasto/openehr-sdk-go/...` package (below `openehr/rm`); enforced by `TestTerminologyForbiddenImports`.
- **REQ-013:** no `openehr/*` building block imports `transport/`, `auth/` or `openehr/client/*`; `openehr/instance` and `openehr/serialize/simplified` MAY import `openehr/terminology` (it is below them).
- **REQ-025:** library code MUST NOT panic on caller input — every `*Group` / `*CodeSet` method is nil-receiver safe; no `init`-time panic; lookups return `(zero, false)` on a miss.
- **REQ-024:** no reflection in library code (the generator MAY use `encoding/xml`; generated tables are literals).
- **AGENTS.md § Vendored fixtures:** never hand-edit `resources/terminology/openehr_terminology.xml`; re-sync instead. Never hand-edit `openehr_gen.go`; run `make termgen`.
- **Generated-file header:** first line exactly `// Code generated by termgen; DO NOT EDIT.` so `.golangci.yml`'s `generated: lax` and the format hook's `*_gen.go` skip both apply.
- **Behaviour change (deliberate, ruled in this plan):** `ehr.LifecycleState.IsValid` and `contribution.ChangeType.IsValid` become *group membership* — 5 and 9 codes instead of 3 and 4. A code outside the group is still refused (guard tests keep that bar with a non-member such as `"999"`).
- **Tests:** stdlib `testing` only; behaviour tests of a public surface in the external `_test` package; every guard asserts the operation-specific facet (removing the guard MUST fail a named test).
- **Formatting / gates:** `$(go env GOROOT)/bin/gofmt -l <files>`; `go vet ./...`; `go test ./... -count=1`; `make termgen-verify`; `make terminology-verify`; `make lint`; `make spec-check`; `make ci`.
- **Commits:** Conventional Commits, one per task, scope = touched area (`build`, `terminology`, `client/ehr`, `simplified`, `instance`, `docs`); every commit ends with the trailer `Assisted-by: Claude Code (<model id>)` (CONTRIBUTING § AI-assisted contributions), never `Co-Authored-By`. CHANGELOG only in Task 6.
- **Git hygiene:** work on the main tree, branch `feat/terminology-vocabulary`; stage files by name (never `git add -A`); never `git stash`.

## Definition of Ready

Implementation may start when:

- REQ-034 has canonical prose (rm-modeling.md), a registry row (REQ.md, `planned`) and a traceability entry (`implementation: planned`) — done 2026-09-07.
- The upstream artefact is identified: `openEHR/specifications-TERM` tag `Release-3.0.0` = commit `d45ef3e21a05d3759101ae7bdb260e8193a3d0da` (2023-06-26), file `computable/XML/en/openehr_terminology.xml`, 13,860 bytes, sha256 `a1a64cc8665afff3992b4c511997e8a8acd1a3706efc32b775894aaf601f0bcf`, `<terminology name="openehr" language="en" version="3.0.0" date="2023-03-05">`, 17 groups (249 concepts) + 3 code sets (19 codes), every group's codes and rubrics unique within the group — verified 2026-09-07.
- Each task below names its files, its tests and its verification command.

## Definition of Done

- `resources/terminology/openehr_terminology.xml` is byte-identical to upstream at the pinned commit; `MANIFEST.txt` records ref, commit, fetch time and sha256; `make terminology-verify` (offline) passes and is part of `make ci`; `make terminology-sync` / `terminology-check` exist and are documented in `resources/terminology/README.md` and `docs/ci.md`.
- `openehr/terminology/openehr_gen.go` is regenerated byte-for-byte by `make termgen`; `make termgen-verify` is a dependency of `make test` and fails on a hand edit (proven by a test in `internal/termgen`).
- `grep -rn --include=*.go -E '"(193|2[0-2][0-9]|238|249|250|251|433|523|532|553)"' openehr internal cadasto | grep -v _test.go | grep -v _gen.go` finds only the named constants in `openehr/client/ehr/version_header.go` and `openehr/client/ehr/contribution/builder.go` and the three `ctx/` / generator default codes (`238`, `433`, `640`) — no rubric is typed beside any of them. *(Verified at close-out: the grep also shows one godoc example in `openehr/client/ehr/audit.go` spelling out the audit-header format — an illustration, not a table.)*
- `contribution.ChangeType` and `ehr.LifecycleState` validity is group membership; every member renders with the pinned rubric; a non-member is still refused (named tests).
- `openehr/serialize/simplified` carries no participation-mode table; `|mode` round-trips every one of the 32 group members; the three `ctx/` defaults are group members whose `value` equals the pinned rubric (the `math_function` default now reads `640|actual`).
- `openehr/instance` derives the setting / category default rubrics from the pin and replaces an `openehr`-coded setting outside the *setting* group with the default.
- wire.md § REQ-130 carries the amended change-type and lifecycle sentences; module-layout.md § REQ-013 and its package table, AGENTS.md's REQ-013 line, bmm-conformance.md § REQ-041 (one pointer sentence), `resources/README.md`, `docs/ci.md`, `docs/roadmap.md`, `deviations.md` § vendored vocabularies are updated; REQ.md row and traceability `landed` with tests enumerated; CHANGELOG has its bullet.
- `make spec-check` and `make ci` pass; this plan is archived under `archive/` in the implementing PR.

## Implementation checklist

| Step | Status |
|---|---|
| Task 1 — vendor the pin: `resources/terminology/` + MANIFEST + sync script + `terminology-verify` in `make ci` | done |
| Task 2 — `openehr/terminology` hand-written surface (`Group`, `CodeSet`, registry accessors, REQ-013 guard) | done |
| Task 3 — `internal/termgen` + `cmd/termgen`, generated tables, `termgen-verify` in `make test`, table pins | done |
| Task 4 — `openehr/client/ehr` + `contribution` re-pointed; group-membership validity; § REQ-130 sentences | done |
| Task 5 — `openehr/serialize/simplified` + `openehr/instance` re-pointed; `ctx/` defaults from the pin (`math_function` fix); deviations.md | done |
| Task 6 — close-out: module-layout / AGENTS / bmm-conformance pointer / roadmap / ci.md / CHANGELOG / traceability / REQ.md; `make ci`; archive | done |

---

## Task 1: Vendor the openEHR Terminology pin

**Files:**
- Create: `resources/terminology/openehr_terminology.xml` (byte-identical download — never typed)
- Create: `resources/terminology/MANIFEST.txt`
- Create: `resources/terminology/README.md`
- Create: `scripts/sync-terminology.sh` (executable)
- Modify: `Makefile` (three targets under `##@ Codegen`'s sibling vendoring block, next to `flat-conformance-*`; `ci` gains `terminology-verify`)
- Modify: `resources/README.md` (table row)
- Modify: `docs/ci.md` (the Verify row at line ~20, the target table rows at ~51–59)

**Interfaces:**
- Produces: `resources/terminology/openehr_terminology.xml`; `MANIFEST.txt` with the keys `source_repo`, `source_path`, `file`, `ref`, `commit`, `fetched_utc`, `source_tree` and one `sha256  filename` line — Task 3's generator reads `ref:` and recomputes the sha256 from the file.

- [x] **Step 1: Write the sync script**

Model it on `scripts/sync-its-rest-specs.sh` (same helper names: `die`, `api`, `sha256_of`; same `set -euo pipefail`; `curl -fsSL --retry 3 --retry-all-errors`; honour `GITHUB_TOKEN`). Subcommands:

```bash
#!/usr/bin/env bash
#
# sync-terminology.sh — vendor / verify the openEHR Terminology
# (openehr_terminology.xml) into resources/terminology/ (REQ-034).
#
# Source: https://github.com/openEHR/specifications-TERM
#         computable/XML/en/openehr_terminology.xml
#
# Subcommands:
#   sync     Resolve TERMINOLOGY_REF (default: the ref pinned in MANIFEST.txt,
#            else the latest GitHub release tag) to a commit sha, download the
#            file at that sha, write it byte-identical, regenerate MANIFEST.txt.
#            Then run `make termgen` so the generated tables follow the pin.
#   verify   Offline: recompute the sha256 and compare to MANIFEST.txt.
#            No network, no curl/jq — run by `make ci`.
#   check    verify, then (best-effort, network) compare the pinned commit
#            with the latest release tag's commit and say if a sync is due.
#
# Environment:
#   TERMINOLOGY_REF   upstream git ref to pin (tag / branch / sha).
#   GITHUB_TOKEN      optional; raises the unauthenticated API rate limit.
set -euo pipefail

readonly REPO="openEHR/specifications-TERM"
readonly SRC_PATH="computable/XML/en"
readonly FILE="openehr_terminology.xml"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
readonly DEST="$ROOT/resources/terminology"
readonly MANIFEST="$DEST/MANIFEST.txt"
```

`sync` writes the manifest in this exact shape (the generator parses `ref:` and the sha line):

```
# openEHR Terminology — sync manifest
# Generated by scripts/sync-terminology.sh — do not edit by hand.
source_repo: openEHR/specifications-TERM
source_path: computable/XML/en
file: openehr_terminology.xml
ref: Release-3.0.0
commit: d45ef3e21a05d3759101ae7bdb260e8193a3d0da
fetched_utc: 2026-09-07T00:00:00Z
source_tree: https://github.com/openEHR/specifications-TERM/tree/d45ef3e21a05d3759101ae7bdb260e8193a3d0da/computable/XML/en
#
# sha256  filename
a1a64cc8665afff3992b4c511997e8a8acd1a3706efc32b775894aaf601f0bcf  openehr_terminology.xml
```

`verify` fails with a non-zero exit and `FAILED: openehr_terminology.xml (sha256 mismatch)` on a hand edit, `MISSING` when the file is absent, and prints `terminology-verify: OK` otherwise. `sync` must not call `make termgen` when `TERMINOLOGY_SKIP_GEN=1` (Task 3 lands the generator; until then this task's `sync` run uses that switch).

- [x] **Step 2: Run the sync at the pinned release**

```bash
chmod +x scripts/sync-terminology.sh
TERMINOLOGY_REF=Release-3.0.0 TERMINOLOGY_SKIP_GEN=1 ./scripts/sync-terminology.sh sync
sha256sum resources/terminology/openehr_terminology.xml
# expect a1a64cc8665afff3992b4c511997e8a8acd1a3706efc32b775894aaf601f0bcf
grep -E '^(ref|commit):' resources/terminology/MANIFEST.txt
# expect ref: Release-3.0.0 / commit: d45ef3e21a05d3759101ae7bdb260e8193a3d0da
```

- [x] **Step 3: Makefile targets**

Next to `flat-conformance-verify`:

```make
terminology-sync: ## Vendor the openEHR Terminology (openehr_terminology.xml) into resources/terminology/ and regenerate the accessor (needs network; TERMINOLOGY_REF to pin)
	@./scripts/sync-terminology.sh sync

terminology-check: ## Verify the vendored terminology matches MANIFEST + report a newer upstream release (offline integrity; network for drift)
	@./scripts/sync-terminology.sh check

terminology-verify: ## Offline sha256 integrity of the vendored openEHR Terminology (no network, no curl/jq) — run by `make ci`
	@./scripts/sync-terminology.sh verify
```

Add `terminology-verify` to the `ci:` prerequisite list right after `flat-conformance-verify`, and to the `.PHONY` list.

- [x] **Step 4: Prove the verify target discriminates**

```bash
make terminology-verify                                  # OK
printf '\n' >> resources/terminology/openehr_terminology.xml
make terminology-verify; echo "exit=$?"                  # FAILED … exit=1
git checkout -- resources/terminology/openehr_terminology.xml
make terminology-verify                                  # OK again
```

- [x] **Step 5: README + inventory rows**

`resources/terminology/README.md` — same sections as `resources/its-rest/README.md`: what the file is (the openEHR Terminology, `openehr` groups + code sets, versioned with the specifications), the pin table (file, TERM release, commit, sha256, group / code-set / concept counts: 17 / 3 / 249) *(Ruled at review: the commit and sha256 cells were dropped — `MANIFEST.txt` is their one home; the table keeps the release tag and the counts.)*, *Provenance* (byte-identical copy; why in-tree — same three reasons as `resources/bmm/README.md`), *Consumers* (`openehr/terminology` via `cmd/termgen`, `make termgen` / `termgen-verify`), *Updating* (`TERMINOLOGY_REF=Release-X.Y.Z make terminology-sync`, then review the generated diff, CHANGELOG bullet, one commit), *Integrity* (`make terminology-verify`, in `make ci`). Cite `§ REQ-034`.

`resources/README.md`: add the row `| [`terminology/`](terminology/README.md) | The openEHR Terminology (`openehr_terminology.xml`, TERM Release-3.0.0) — source of truth for the generated `openehr/terminology` accessor; synced via `make terminology-sync` |`.

`docs/ci.md`: add `terminology-verify` to the Verify row's list and to the `make ci` description, and a `| Fixtures | `make terminology-verify` | Offline `sha256` integrity of the pinned openEHR Terminology against its `MANIFEST.txt` |` row beside the flat-conformance one.

- [x] **Step 6: Commit**

```bash
git add resources/terminology scripts/sync-terminology.sh Makefile resources/README.md docs/ci.md
git commit -m "build(terminology): vendor the openEHR Terminology at TERM Release-3.0.0 with a sha256 manifest and an offline verify gate (REQ-034)" -m "Assisted-by: Claude Code (<model id>)"
```

## Task 2: `openehr/terminology` — the hand-written surface

**Files:**
- Create: `openehr/terminology/doc.go`
- Create: `openehr/terminology/terminology.go`
- Create: `openehr/terminology/terminology_test.go` (internal package `terminology` — it constructs groups via `newGroup`)
- Create: `openehr/terminology/imports_test.go` (external package)

**Interfaces:**
- Produces (Task 3 generates calls to these; Tasks 4–5 consume the exported ones):

```go
const ID = "openehr"                       // TERMINOLOGY_ID.value every table is defined in

type Concept struct{ Code, Rubric string }

type Group struct { /* unexported */ }
func newGroup(id, name string, concepts []Concept) *Group
func (g *Group) ID() string                 // openehr_id, e.g. "audit_change_type"
func (g *Group) Name() string               // "audit change type"
func (g *Group) Len() int
func (g *Group) All() iter.Seq[Concept]     // source order
func (g *Group) Has(code string) bool
func (g *Group) Rubric(code string) (string, bool)
func (g *Group) Code(rubric string) (string, bool)

type CodeSet struct { /* unexported */ }
func newCodeSet(id, name string, codes []string) *CodeSet
func (s *CodeSet) ID() string
func (s *CodeSet) Name() string
func (s *CodeSet) Len() int
func (s *CodeSet) All() iter.Seq[string]
func (s *CodeSet) Has(code string) bool

func Groups() iter.Seq[*Group]              // source order, over the generated `groups` slice
func CodeSets() iter.Seq[*CodeSet]
func GroupByID(id string) (*Group, bool)    // "setting" → Setting
func CodeSetByID(id string) (*CodeSet, bool)
```

Until Task 3 lands, `terminology.go` declares `var groups []*Group` and `var codeSets []*CodeSet` **in a temporary file `tables_stub.go`** that Task 3 deletes when `openehr_gen.go` defines them.

- [x] **Step 1: Write the failing tests** (`terminology_test.go`, package `terminology`)

```go
func TestGroupLookups(t *testing.T) {
	t.Parallel()
	g := newGroup("demo", "demo group", []Concept{{"1", "one"}, {"2", "two"}})
	if got := g.ID(); got != "demo" { t.Errorf("ID = %q", got) }
	if got := g.Name(); got != "demo group" { t.Errorf("Name = %q", got) }
	if got := g.Len(); got != 2 { t.Errorf("Len = %d", got) }
	if r, ok := g.Rubric("2"); !ok || r != "two" { t.Errorf("Rubric(2) = %q,%v", r, ok) }
	if c, ok := g.Code("one"); !ok || c != "1" { t.Errorf("Code(one) = %q,%v", c, ok) }
	if !g.Has("1") || g.Has("3") || g.Has("") { t.Error("Has misreports membership") }
	if _, ok := g.Rubric("3"); ok { t.Error("Rubric on an unknown code must report absence") }
	if _, ok := g.Code("three"); ok { t.Error("Code on an unknown rubric must report absence") }
	got := slices.Collect(g.All())
	if !slices.Equal(got, []Concept{{"1", "one"}, {"2", "two"}}) { t.Errorf("All = %v (source order required)", got) }
}

func TestNilGroupAndCodeSetAreInert(t *testing.T) { // REQ-025
	t.Parallel()
	var g *Group
	if g.Has("1") || g.Len() != 0 || g.ID() != "" || g.Name() != "" { t.Error("nil *Group must be inert") }
	if _, ok := g.Rubric("1"); ok { t.Error("nil Rubric") }
	if _, ok := g.Code("x"); ok { t.Error("nil Code") }
	if n := len(slices.Collect(g.All())); n != 0 { t.Errorf("nil All yields %d", n) }
	var s *CodeSet
	if s.Has("N") || s.Len() != 0 || s.ID() != "" || s.Name() != "" { t.Error("nil *CodeSet must be inert") }
	if n := len(slices.Collect(s.All())); n != 0 { t.Errorf("nil All yields %d", n) }
}

func TestCodeSetLookups(t *testing.T) {
	t.Parallel()
	s := newCodeSet("demo_set", "demo set", []string{"N", "H"})
	if !s.Has("N") || s.Has("X") || s.Len() != 2 || s.ID() != "demo_set" || s.Name() != "demo set" { t.Error("code-set surface") }
	if got := slices.Collect(s.All()); !slices.Equal(got, []string{"N", "H"}) { t.Errorf("All = %v", got) }
}

func TestRegistryAccessorsUseTheTables(t *testing.T) {
	// swap the package tables for the test's own, restore after
	saveG, saveS := groups, codeSets
	t.Cleanup(func() { groups, codeSets = saveG, saveS })
	a := newGroup("a", "a", []Concept{{"1", "one"}})
	b := newGroup("b", "b", []Concept{{"2", "two"}})
	groups = []*Group{a, b}
	codeSets = []*CodeSet{newCodeSet("s", "s", []string{"N"})}
	if got := slices.Collect(Groups()); len(got) != 2 || got[0] != a || got[1] != b { t.Error("Groups order") }
	if g, ok := GroupByID("b"); !ok || g != b { t.Error("GroupByID") }
	if _, ok := GroupByID("zzz"); ok { t.Error("GroupByID unknown") }
	if s, ok := CodeSetByID("s"); !ok || s.ID() != "s" { t.Error("CodeSetByID") }
	if _, ok := CodeSetByID("zzz"); ok { t.Error("CodeSetByID unknown") }
}
```

(`TestRegistryAccessorsUseTheTables` is not `t.Parallel()` — it mutates package state.)

`imports_test.go` (package `terminology_test`) mirrors `openehr/validation/imports_test.go` but the forbidden set is **every** in-module import: iterate `pkg.Imports` and fail on any import containing `github.com/cadasto/openehr-sdk-go/` — `TestTerminologyForbiddenImports` — with the REQ-034 / REQ-013 rationale in the comment (stdlib-only so `openehr/rm` can import it later).

- [x] **Step 2: Run, expect compile failure** — `go test ./openehr/terminology/`

- [x] **Step 3: Implement** (`terminology.go`)

```go
package terminology

import (
	"iter"
	"slices"
)

// ID is the TERMINOLOGY_ID.value every group and code set in this package is
// defined in — the openEHR Terminology's own identifier.
const ID = "openehr"

// Concept is one coded entry of a [Group]: the code and its English rubric.
type Concept struct {
	Code   string
	Rubric string
}

// Group is one closed, source-ordered openEHR terminology group. Every group is a
// package-level variable generated from the pin (see openehr_gen.go); the zero
// value and a nil pointer are inert — every method reports absence (REQ-025).
type Group struct {
	id, name string
	concepts []Concept
	byCode   map[string]int
	byRubric map[string]int
}

func newGroup(id, name string, concepts []Concept) *Group {
	g := &Group{id: id, name: name, concepts: concepts,
		byCode: make(map[string]int, len(concepts)), byRubric: make(map[string]int, len(concepts))}
	for i, c := range concepts {
		g.byCode[c.Code] = i
		g.byRubric[c.Rubric] = i
	}
	return g
}

// ID returns the group's openehr_id, e.g. "audit_change_type".
func (g *Group) ID() string { if g == nil { return "" }; return g.id }
// Name returns the group's display name, e.g. "audit change type".
func (g *Group) Name() string { if g == nil { return "" }; return g.name }
// Len returns the number of concepts.
func (g *Group) Len() int { if g == nil { return 0 }; return len(g.concepts) }
// All yields the concepts in source order.
func (g *Group) All() iter.Seq[Concept] { if g == nil { return func(func(Concept) bool) {} }; return slices.Values(g.concepts) }
// Has reports whether code is a member.
func (g *Group) Has(code string) bool { if g == nil { return false }; _, ok := g.byCode[code]; return ok }
// Rubric returns the rubric for code; false when code is not a member.
func (g *Group) Rubric(code string) (string, bool) { if g == nil { return "", false }; i, ok := g.byCode[code]; if !ok { return "", false }; return g.concepts[i].Rubric, true }
// Code returns the code whose rubric is rubric; false when no member carries it.
func (g *Group) Code(rubric string) (string, bool) { /* symmetric via byRubric */ }
```

(Write the bodies on separate lines — the one-liners above are for compactness in the plan; gofmt will spread them.) `CodeSet` is the same shape with `codes []string` and `index map[string]struct{}`. Registry accessors:

```go
// Groups yields every group of the pinned terminology in source order.
func Groups() iter.Seq[*Group] { return slices.Values(groups) }
// CodeSets yields every code set of the pinned terminology in source order.
func CodeSets() iter.Seq[*CodeSet] { return slices.Values(codeSets) }
// GroupByID returns the group whose openehr_id is id — the identifier the RM's
// has_code_for_group_id invariants name — or false.
func GroupByID(id string) (*Group, bool) { for _, g := range groups { if g.id == id { return g, true } }; return nil, false }
func CodeSetByID(id string) (*CodeSet, bool) { /* same */ }
```

Linear scan is fine: 17 groups, called at validation time, not in a hot loop; no init-time map to keep the package's init cost at "literals only" (the `rminfo` posture).

`doc.go`: package comment modelled on `openehr/rm/rminfo/doc.go` — what it answers (is this code a member of that group, what is its rubric, which code has this rubric), where the data comes from (generated from `resources/terminology/openehr_terminology.xml`, TERM Release-3.0.0, via `cmd/termgen`; `make termgen` / `make termgen-verify`), what is hand-written vs generated, the building-block weight (stdlib-only, literals only, no init work), who consumes it (`openehr/client/ehr`, `openehr/client/ehr/contribution`, `openehr/serialize/simplified`, `openehr/instance`), and the REQ-034 citation. Mention the one upstream quirk verbatim from the pin's own comment: code `532` is *complete* in *version lifecycle state* and *completed* in *instruction states* (SPECPR-51) — the rubric is per group, which is why lookups are per group.

- [x] **Step 4: Run** — `go test ./openehr/terminology/ -count=1` → PASS; `go vet ./openehr/terminology/`; `$(go env GOROOT)/bin/gofmt -l openehr/terminology`.

- [x] **Step 5: Commit**

```bash
git add openehr/terminology
git commit -m "feat(terminology): openehr/terminology building block — Group and CodeSet with nil-safe code/rubric/membership lookups and registry accessors (REQ-034)" -m "Assisted-by: Claude Code (<model id>)"
```

## Task 3: `termgen` — generate the tables from the pin, drift-detected

**Files:**
- Create: `internal/termgen/parse.go`, `internal/termgen/render.go`, `internal/termgen/run.go`
- Create: `internal/termgen/parse_test.go`, `internal/termgen/render_test.go`, `internal/termgen/run_test.go`
- Create: `cmd/termgen/main.go`
- Create (generated): `openehr/terminology/openehr_gen.go`
- Delete: `openehr/terminology/tables_stub.go`
- Create: `openehr/terminology/tables_test.go` (external package — pins over the real tables)
- Modify: `Makefile` (`termgen`, `termgen-verify`; `test:` depends on `termgen-verify`; `.PHONY`), `docs/ci.md` (Codegen rows), `scripts/sync-terminology.sh` (`sync` now runs `make termgen` unless `TERMINOLOGY_SKIP_GEN=1`)

**Interfaces:**
- Consumes: Task 2's `newGroup` / `newCodeSet` / `Concept`, and the `groups` / `codeSets` slice names.
- Produces: `openehr/terminology.Version` (`"3.0.0"`), `terminology.SourceSHA256`, and the exported group / code-set variables named by UpperCamel-casing the `openehr_id` (`audit_change_type` → `AuditChangeType`; full list: `AttestationReason`, `AuditChangeType`, `CompositionCategory`, `Property`, `VersionLifecycleState`, `ParticipationFunction`, `NullFlavours`, `ParticipationMode`, `InstructionStates`, `InstructionTransitions`, `SubjectRelationship`, `TermMappingPurpose`, `EventMathFunction`, `Setting`, `ExtractContentType`, `ExtractActionType`, `ExtractUpdateTriggerEventType`; code sets `CompressionAlgorithms`, `IntegrityCheckAlgorithms`, `NormalStatuses`). Tasks 4–5 use these names.

- [x] **Step 1: Failing parser tests** (`parse_test.go`) over an inline fixture:

```go
const fixture = `<terminology name="openehr" language="en" version="9.9.9" date="2026-01-01">
	<codeset issuer="openehr" openehr_id="normal_statuses" name="normal statuses" external_id="openehr_normal_statuses">
		<code value="N"/><code value="H"/>
	</codeset>
	<group openehr_id="audit_change_type" name="audit change type">
		<concept id="249" rubric="creation"/>
		<concept id="523" rubric="deleted"/><!-- a comment -->
	</group>
</terminology>`
```

Cases: happy path (name/version/date, 1 code set of 2, 1 group of 2 in source order); `name != "openehr"` refused; empty `version` refused; group with a duplicate `id` refused naming the group and code; group with a duplicate `rubric` refused; group with zero concepts refused; two groups sharing an `openehr_id` refused; an `openehr_id` that is not `^[a-z][a-z0-9_]*$` refused; code set with a duplicate `value` refused; a `<concept>` missing `rubric` refused. Every refusal is an `error` (never a panic) whose text names the offending id.

- [x] **Step 2: Parser** (`parse.go`)

```go
type Terminology struct { Name, Language, Version, Date string; CodeSets []CodeSetDef; Groups []GroupDef }
type CodeSetDef struct { ID, Name string; Codes []string }
type GroupDef struct { ID, Name string; Concepts []ConceptDef }
type ConceptDef struct { Code, Rubric string }

// Parse decodes openehr_terminology.xml and validates the invariants the
// generated tables rely on: unique ids, unique codes and rubrics per group,
// non-empty groups, Go-mangle-safe ids.
func Parse(r io.Reader) (*Terminology, error)
```

Use `encoding/xml` struct tags (`xml:"codeset"`, `xml:"group"`, `xml:"concept"`, attributes `openehr_id,attr`, `name,attr`, `id,attr`, `rubric,attr`, `value,attr`). `GoName(id string) string` mangles `snake_case` → `UpperCamel` (`strings.Split` on `_`, upper-case each part's first byte, join) — exported so `render_test` can pin it.

- [x] **Step 3: Failing render test** (`render_test.go`): render the fixture with `SourceInfo{Path: "resources/terminology/openehr_terminology.xml", Ref: "Release-9.9.9", SHA256: "abc"}` and assert: first line is exactly `// Code generated by termgen; DO NOT EDIT.`; a `// Source:` line naming the path, the ref and the sha; `package terminology`; `const Version = "9.9.9"`; `const SourceSHA256 = "abc"`; `var AuditChangeType = newGroup("audit_change_type", "audit change type", []Concept{` followed by `{Code: "249", Rubric: "creation"},` then `{Code: "523", Rubric: "deleted"},` (source order); `var NormalStatuses = newCodeSet("normal_statuses", "normal statuses", []string{"N", "H"})`; `var groups = []*Group{AuditChangeType}`; `var codeSets = []*CodeSet{NormalStatuses}`; the output equals `format.Source(output)` (already gofmt-clean); rendering twice yields identical bytes.

- [x] **Step 4: Renderer** (`render.go`)

```go
type SourceInfo struct{ Path, Ref, SHA256 string }
// Render emits openehr_gen.go: header, Version / SourceSHA256, one var per group
// (source order) and per code set, then the `groups` / `codeSets` registry slices.
func Render(t *Terminology, src SourceInfo) ([]byte, error)
```

Header shape:

```go
// Code generated by termgen; DO NOT EDIT.
// Source: resources/terminology/openehr_terminology.xml — openEHR TERM Release-3.0.0
// (terminology version 3.0.0, sha256 a1a64cc8…).
// Regenerate: make termgen. Verify: make termgen-verify.

package terminology

// Version is the openEHR Terminology release the tables below are generated from.
const Version = "3.0.0"

// SourceSHA256 is the sha256 of the pinned openehr_terminology.xml the tables were generated from.
const SourceSHA256 = "a1a64cc8665afff3992b4c511997e8a8acd1a3706efc32b775894aaf601f0bcf"

// AuditChangeType is the openEHR terminology group "audit change type" (openehr_id audit_change_type), 9 concepts.
var AuditChangeType = newGroup("audit_change_type", "audit change type", []Concept{
	{Code: "249", Rubric: "creation"},
	…
})
```

Rubrics go through `strconv.Quote` (one carries a `/`, several carry `;`). Run the buffer through `format.Source` and return its error.

- [x] **Step 5: Run + verify** (`run.go`, `run_test.go`)

```go
type Options struct { ResourcesDir, OutDir string; Verify bool; Stderr io.Writer }
type Result struct { Path string; Drift, Missing bool }
// Run reads <ResourcesDir>/openehr_terminology.xml and <ResourcesDir>/MANIFEST.txt
// (the `ref:` line), renders openehr_gen.go under <OutDir>/openehr/terminology/,
// and either writes it atomically or — with Verify — compares it with the file on
// disk, reporting Drift / Missing without writing.
func Run(opts Options) (Result, error)
```

`run_test.go` uses `t.TempDir()`: write the fixture + a two-line manifest; `Run` (write) then `Run` (verify) → no drift; append a comment to the generated file → verify reports `Drift`; remove it → `Missing`; a manifest without `ref:` → error. Mirror `internal/bmmgen`'s `compareFile` / `writeAtomic` semantics (read those two helpers first — copy their behaviour, not their code path).

`cmd/termgen/main.go`: flags `-resources ./resources/terminology`, `-out .`, `-verify`; on drift print `termgen: drift detected in openehr/terminology/openehr_gen.go — run 'make termgen'` and exit 1; other errors exit 2. Doc comment in the `cmd/bmmgen/main.go` style.

- [x] **Step 6: Makefile + generate for real**

```make
termgen: ## Regenerate openehr/terminology from the pinned resources/terminology/openehr_terminology.xml
	@$(GO) run ./cmd/termgen -resources ./resources/terminology -out .

termgen-verify: ## Fail if openehr/terminology drifts from resources/terminology
	@$(GO) run ./cmd/termgen -resources ./resources/terminology -out . -verify
```

`test: codegen-verify aqlgen-verify termgen-verify`. Then `make termgen`, delete `openehr/terminology/tables_stub.go`, `go build ./...`, `make termgen-verify` → OK; append a blank line to `openehr_gen.go`, `make termgen-verify` → exit 1; `make termgen` restores it. Update `scripts/sync-terminology.sh` so `sync` runs `make termgen` (unless `TERMINOLOGY_SKIP_GEN=1`) and re-run `make terminology-verify`. `docs/ci.md`: Codegen rows for `make termgen-verify` (beside `codegen-verify`), and the Test row's dependency list.

- [x] **Step 7: Pin the real tables** (`openehr/terminology/tables_test.go`, package `terminology_test`)

```go
func TestPinnedRelease(t *testing.T) {
	if terminology.Version != "3.0.0" { t.Errorf("Version = %q", terminology.Version) }
	if terminology.ID != "openehr" { t.Errorf("ID = %q", terminology.ID) }
	sum, _ := os.ReadFile("../../resources/terminology/openehr_terminology.xml")  // hash it with crypto/sha256; compare hex to terminology.SourceSHA256
}
func TestTablesMatchTheOpenEHRTerminology(t *testing.T) {
	// counts pinned from the release: 17 groups / 3 code sets / 249 concepts / 19 codes
	// spot pins: AuditChangeType 523→"deleted", 253→"unknown", 252→"synthesis", Len 9;
	// VersionLifecycleState 532→"complete", 800→"inactive", Len 5;
	// InstructionStates 532→"completed" (the SPECPR-51 quirk: same code, different rubric, different group);
	// ParticipationMode Len 32, 193→"not specified", 224→"interpreted video communication", Code("face-to-face communication")=="216";
	// Setting 238→"other care", Has("227"), !Has("226"); CompositionCategory 433→"event";
	// EventMathFunction 146→"mean", 640→"actual"; NormalStatuses Has N,H,HH,HHH,L,LL,LLL, Len 7, !Has("X");
	// GroupByID("setting")==Setting; CodeSetByID("normal_statuses")==NormalStatuses; GroupByID("nope") false.
}
func TestEveryGroupIsInvertible(t *testing.T) {
	// for every g in Groups(): for every c in g.All(): Rubric(c.Code)==c.Rubric && Code(c.Rubric)==c.Code; Len()==count(All())
}
func TestGroupsAreInSourceOrder(t *testing.T) {
	// first is AttestationReason, last is ExtractUpdateTriggerEventType (the pin's document order)
}
```

- [x] **Step 8: Gates + commit**

`go test ./internal/termgen/ ./openehr/terminology/ ./cmd/termgen/ -count=1`; `go vet ./...`; `make termgen-verify`; `make terminology-verify`; `golangci-lint run ./internal/termgen/... ./openehr/terminology/... ./cmd/termgen/...`; gofmt check on hand-written files.

```bash
git add internal/termgen cmd/termgen openehr/terminology Makefile docs/ci.md scripts/sync-terminology.sh
git commit -m "feat(terminology): termgen generates the openehr/terminology tables from the pinned TERM Release-3.0.0 XML; make test gates drift via termgen-verify (REQ-034)" -m "Assisted-by: Claude Code (<model id>)"
```

## Task 4: The client tree reads the pin — lifecycle state and audit change type

**Files:**
- Modify: `openehr/client/ehr/version_header.go`
- Modify: `openehr/client/ehr/version_header_test.go`
- Modify: `openehr/client/ehr/contribution/builder.go` (the `ChangeType` block, `changeTypeTerms`, `lifecycleTerms`, `codedText`, `newChange`'s error text, `WithChangeType`)
- Modify: `openehr/client/ehr/contribution/builder_test.go`
- Modify: `docs/specifications/wire.md` § REQ-130 (the *Change types* and *Lifecycle state* paragraphs)

**Interfaces:**
- Consumes: `terminology.VersionLifecycleState`, `terminology.AuditChangeType`, `terminology.ID`.
- Produces: new constants `ehr.LifecycleStateInactive = "800"`, `ehr.LifecycleStateAbandoned = "801"`; `func (s LifecycleState) Rubric() (string, bool)`; new constants `contribution.ChangeTypeSynthesis = "252"`, `ChangeTypeUnknown = "253"`, `ChangeTypeAttestation = "666"`, `ChangeTypeRestoration = "816"`, `ChangeTypeFormatConversion = "817"`; `func (c ChangeType) Rubric() (string, bool)`.

- [x] **Step 1: Failing tests — lifecycle** (`version_header_test.go`)

Replace the *accepts all known codes* sub-test body with a loop over `terminology.VersionLifecycleState.All()` (`IsValid()` true, header formats without error, and `Rubric()` equals the concept's rubric); keep *rejects unknown code* with `"999"`. Add:

```go
func TestLifecycleStateConstantsCoverTheGroup(t *testing.T) { // REQ-034: the promoted constants MUST be exactly the group's members
	want := map[LifecycleState]bool{LifecycleStateComplete: true, LifecycleStateIncomplete: true, LifecycleStateDeleted: true, LifecycleStateInactive: true, LifecycleStateAbandoned: true}
	for c := range terminology.VersionLifecycleState.All() {
		if !want[LifecycleState(c.Code)] { t.Errorf("group member %s (%s) has no LifecycleState constant", c.Code, c.Rubric) }
	}
	if len(want) != terminology.VersionLifecycleState.Len() { t.Errorf("%d constants, group has %d members", len(want), terminology.VersionLifecycleState.Len()) }
}
```

- [x] **Step 2: Failing tests — change type** (`builder_test.go`)

- *unknown batch change_type code* case: `ChangeType("253")` → `ChangeType("999")` (253 is *unknown*, a real member).
- `TestBuilderBuildIsIdempotentOnTheErrorPath`: `ChangeType("253")` → `ChangeType("999")`.
- `TestBuilderWithAuditCarriesAnyChangeType`: make it discriminating against the widened set — code `"999"`, value `"custom"`; rewrite its comment: *WithChangeType admits the openEHR group; a code outside it (a deployment-local extension, say) still reaches the wire only by supplying the whole audit.*
- New `TestWithChangeTypeAdmitsEveryGroupMemberWithItsRubric`: for each `c` in `terminology.AuditChangeType.All()`, build with `WithChangeType(ChangeType(c.Code))`, marshal, assert `audit.change_type.defining_code.code_string == c.Code`, `.terminology_id.value == "openehr"` and `.value == c.Rubric`.
- New `TestChangeTypeConstantsCoverTheGroup` (same shape as the lifecycle one, nine constants).
- New `TestChangeTypeCodedTextOutsideTheGroupIsZero`: `ChangeType("999").CodedText()` is the zero `rm.DVCodedText`; `Rubric()` false.

- [x] **Step 3: Implement**

`version_header.go`: import `github.com/cadasto/openehr-sdk-go/openehr/terminology`; add the two constants; `IsValid` → `terminology.VersionLifecycleState.Has(string(s))`; add `Rubric`; doc: "The value set is the openEHR *version lifecycle state* group of the pinned terminology (REQ-034); the constants name every member."

`builder.go`: constants for all nine members (keep the `523`-not-`253` remark on `ChangeTypeDeleted`, and on `ChangeTypeUnknown` say it *is* a member — `unknown` — not a deletion); `IsValid` → `terminology.AuditChangeType.Has`; `Rubric`; `CodedText` uses `Rubric` and `codedText`; delete `changeTypeTerms` and `lifecycleTerms`; `codedText` uses `terminology.ID`; the `newChange` error at the old line 191 uses `ct.Rubric()` (fall back to the raw code if the lookup fails — it cannot after `IsValid`, but never index a map that no longer exists); the `OriginalVersion.LifecycleState` at the old line 205 uses `cfg.lifecycle.Rubric()`. Rewrite the `ChangeType` type comment: the set is the group; `WithChangeType` refuses a non-member; a non-member reaches the wire only through `WithAudit`.

- [x] **Step 4: Spec sentences** (wire.md § REQ-130)

After the four-row table, replace *"This table is the code set's single home in these specs — `523` is the deletion code; `253` is *unknown*, not *deleted*."* with: *"The codes and rubrics are the pinned openEHR terminology's (`§ REQ-034`); this table maps the builder's four operations onto them — `523` is the deletion code; `253` is *unknown*, not *deleted*. The batch-level `audit.change_type` MAY be any member of the *audit change type* group: the builder **MUST** refuse a code outside the group and **MUST** render a member with the group's rubric."* Keep the rest of that paragraph. In the *Lifecycle state* paragraph replace *"— `complete` (`532`), `incomplete` (`553`), `deleted` (`523`) —"* with *"— any member of the group (`§ REQ-034`); the SDK names `complete` (`532`), `incomplete` (`553`), `deleted` (`523`), `inactive` (`800`) and `abandoned` (`801`) —"*.

- [x] **Step 5: Gates + commit**

`go test ./openehr/client/... -count=1`; `go vet ./openehr/client/...`; `golangci-lint run ./openehr/client/...`; gofmt.

```bash
git add openehr/client/ehr/version_header.go openehr/client/ehr/version_header_test.go openehr/client/ehr/contribution/builder.go openehr/client/ehr/contribution/builder_test.go docs/specifications/wire.md
git commit -m "feat(client/ehr): lifecycle state and audit change type read the pinned terminology — validity is group membership, rubrics come from the pin (REQ-034, REQ-130)" -m "Assisted-by: Claude Code (<model id>)"
```

## Task 5: The simplified codec and the instance generator read the pin

**Files:**
- Modify: `openehr/serialize/simplified/rmattr_party.go` (delete `participationModes`, `participationModeCodes`, `participationModeTerminology`; `participationModeJSON`)
- Modify: `openehr/serialize/simplified/rmattr_party_encode.go` (`participationModeRubric`)
- Modify: `openehr/serialize/simplified/rmattr_party_test.go` (`TestParticipationModeVocabularyIsInvertible` → `TestParticipationModeRoundTripsEveryGroupMember`)
- Modify: `openehr/serialize/simplified/datatypes.go` (`normalStatusTerminology` → `terminology.ID`; delete the const, keep its comment's *why* at the use site)
- Modify: `openehr/serialize/simplified/flat_encode.go` (`emitContextSetting`'s `"openehr"`)
- Modify: `openehr/serialize/simplified/flat_decode.go` (`metadataAliasTerminology`, the `ctxObj["setting"]` build, the three `ctx/` defaults)
- Modify: `openehr/serialize/simplified/context_test.go` (add `TestCtxDefaultsAreGroupMembersWithPinnedRubrics`) *(landed in `flat_decode_test.go` — `context_test.go` is an external package and cannot reach `defaultAttr`)*
- Modify: `openehr/serialize/simplified/deviations.md` (§ vendored vocabularies row)
- Modify: `openehr/instance/generate.go` (`applyCompositionDefaults`)
- Modify: `openehr/instance/instance_test.go` (setting-outside-the-group case)

**Interfaces:**
- Consumes: `terminology.ParticipationMode`, `terminology.Setting`, `terminology.CompositionCategory`, `terminology.EventMathFunction`, `terminology.ID`.

- [x] **Step 1: Failing tests**

`rmattr_party_test.go` (internal package): replace the invertibility test with

```go
// TestParticipationModeRoundTripsEveryGroupMember — REQ-140 / REQ-034. `|mode`
// carries the bare rubric; decode rebuilds code + terminology from the pinned
// `participation mode` group and encode is the exact inverse, for all 32 members.
func TestParticipationModeRoundTripsEveryGroupMember(t *testing.T) {
	n := 0
	for c := range terminology.ParticipationMode.All() {
		n++
		got, err := participationModeJSON("x|mode", c.Rubric)
		// assert got["value"]==c.Rubric, defining_code.code_string==c.Code, terminology_id.value=="openehr"
		back, err := participationModeRubric("x|mode", rm.DVCodedText{DVText: rm.DVText{Value: c.Rubric}, DefiningCode: rm.CodePhrase{CodeString: c.Code, TerminologyID: rm.TerminologyID{Value: terminology.ID}}})
		// assert back==c.Rubric
	}
	if n != 32 { t.Fatalf("participation mode group has %d members, want 32", n) }
	if _, err := participationModeJSON("x|mode", "telepathy"); !errors.Is(err, ErrUnsupportedDatatype) { t.Error("unknown rubric must be refused") }
}
```

`context_test.go`:

```go
// TestCtxDefaultsAreGroupMembersWithPinnedRubrics — REQ-034: every openehr-coded
// default the ctx/ completion synthesises is a member of its group and carries the
// pin's rubric as its value. Would have caught the 146|actual pair (146 is `mean`).
func TestCtxDefaultsAreGroupMembersWithPinnedRubrics(t *testing.T) {
	for _, tc := range []struct{ attr string; group *terminology.Group; code string }{
		{"setting", terminology.Setting, "238"},
		{"category", terminology.CompositionCategory, "433"},
		{"math_function", terminology.EventMathFunction, "640"},
	} {
		v := <the ctx-default function>(<a ctxInfo with no time>, tc.attr).(map[string]any)   // read flat_decode.go to name it; it is the switch returning codePhraseJSON(...) defaults
		dc := v["defining_code"].(map[string]any)
		rubric, ok := tc.group.Rubric(tc.code)
		// assert ok, dc["code_string"]==tc.code, terminology_id value == "openehr", v["value"]==rubric
	}
}
```

`instance_test.go`: a new case where the compiled template leaves setting unconstrained and the walk hands the generator an `openehr`-coded setting `"999"` (or drive it through whatever seam the existing `"433"` category test at line ~91 uses) → assert the emitted setting is `238|other care`; and a setting `"227|emergency care"` (a member) is kept.

- [x] **Step 2: Implement — simplified**

- `rmattr_party.go`: keep the `// --- the vendored participation mode vocabulary` section as a short comment explaining *why* `|mode` needs the group (no code channel), pointing at `terminology.ParticipationMode` (REQ-034); `participationModeJSON` → `code, known := terminology.ParticipationMode.Code(rubric)`; `codePhraseJSON(code, terminology.ID)`.
- `rmattr_party_encode.go`: `term != terminology.ID`; `terminology.ParticipationMode.Rubric(code)`; error texts unchanged except the identifier names.
- `datatypes.go`: every `normalStatusTerminology` use → `terminology.ID`; move the const's explanatory comment to the first use.
- `flat_encode.go`: `!= terminology.ID`.
- `flat_decode.go`: `metadataAliasTerminology["context/setting|terminology"] = terminology.ID`; `codePhraseJSON(ci.settingCode, terminology.ID)`; the defaults:

```go
case "setting":
	return ctxCodedDefault(terminology.Setting, "238")
case "category":
	return ctxCodedDefault(terminology.CompositionCategory, "433")
case "math_function":
	// 640 is `actual` — the earlier 146|actual pair was inconsistent (146 is `mean`).
	return ctxCodedDefault(terminology.EventMathFunction, "640")
```

```go
// ctxCodedDefault builds the DV_CODED_TEXT for a ctx/ default from the pinned
// group, so the value is the code's own rubric and never a string typed beside
// the code (REQ-034). The codes are constants of this file and members of their
// groups — TestCtxDefaultsAreGroupMembersWithPinnedRubrics pins that.
func ctxCodedDefault(g *terminology.Group, code string) map[string]any {
	rubric, _ := g.Rubric(code)
	return map[string]any{"_type": "DV_CODED_TEXT", "value": rubric, "defining_code": codePhraseJSON(code, terminology.ID)}
}
```

- `deviations.md` § vendored vocabularies: the *participation mode* row now reads that the table lives in `openehr/terminology` (generated from the pinned `resources/terminology/openehr_terminology.xml`, `§ REQ-034`) and no hand-typed copy remains in the codec; keep the *why* (`|mode` has no code channel).

- [x] **Step 3: Implement — instance**

`applyCompositionDefaults`: category default → `value, _ := terminology.CompositionCategory.Rubric("433")`; setting: replace the two-arm condition with

```go
if c.Context.Setting.DefiningCode.CodeString == "" ||
	c.Context.Setting.DefiningCode.TerminologyID.Value != terminology.ID ||
	!terminology.Setting.Has(c.Context.Setting.DefiningCode.CodeString) {
	rubric, _ := terminology.Setting.Rubric("238")
	c.Context.Setting = rm.DVCodedText{Value: rubric, DefiningCode: rm.CodePhrase{CodeString: "238", TerminologyID: rm.TerminologyID{Value: terminology.ID}}}
}
```

and rewrite the comment: the "residual case … needs the terminology tables … left to the RM-floor validator" sentence is gone — the tables exist and the generator pins the default for a non-member too (the RM floor's own `Setting_valid` check stays a REQ-112 follow-up). `openehr/instance/imports_test.go` may need `openehr/terminology` allowed — read it first; it forbids wire layers, not building blocks.

- [x] **Step 4: Gates + commit**

`go test ./openehr/serialize/... ./openehr/instance/... ./testkit/... -count=1` (the conformance census and PROBE-089 round trips must stay green — if a pin somewhere carried the old `146|actual` bytes, update that pin and say so in the commit body); `go vet`; `golangci-lint run ./openehr/serialize/... ./openehr/instance/...`; gofmt.

```bash
git add openehr/serialize/simplified openehr/instance
git commit -m "feat(simplified,instance): participation mode, ctx/ defaults and generator defaults read the pinned terminology; math_function default is 640|actual, not 146|actual (REQ-034, REQ-140, REQ-107)" -m "Assisted-by: Claude Code (<model id>)"
```

## Task 6: Close-out — spec status, indexes, CHANGELOG, archive

**Files:**
- Modify: `docs/specifications/module-layout.md` (package table row after `openehr/bmm/`; § REQ-013 sentence adds `openehr/terminology` and `TestTerminologyForbiddenImports`)
- Modify: `AGENTS.md` (the REQ-013 bullet's package list gains `terminology`; one clause)
- Modify: `docs/specifications/bmm-conformance.md` § REQ-041 (one pointer sentence: the openEHR Terminology is pinned the same way under `resources/terminology/` — `§ REQ-034`)
- Modify: `docs/roadmap.md` (a **Landed** row for *openEHR terminology vocabulary* beside the `rminfo` rows)
- Modify: `docs/specifications/REQ.md` (REQ-034 row `planned` → `landed`)
- Modify: `docs/specifications/traceability.yaml` (REQ-034 `implementation: landed`, `tests:` enumerated — `openehr/terminology/terminology_test.go`, `openehr/terminology/tables_test.go`, `openehr/terminology/imports_test.go`, `internal/termgen/parse_test.go`, `internal/termgen/render_test.go`, `internal/termgen/run_test.go`, `openehr/client/ehr/version_header_test.go`, `openehr/client/ehr/contribution/builder_test.go`, `openehr/serialize/simplified/rmattr_party_test.go`, `openehr/serialize/simplified/context_test.go`, `openehr/instance/instance_test.go`; `plans:` → the archive path; REQ-013's `packages:` gains `openehr/terminology`; REQ-130 `notes` one clause)
- Modify: `CHANGELOG.md` (one `### Added` bullet under `## [Unreleased]`)
- Modify: this plan (Status → `landed (2026-09-07, archived in the implementing PR)`, checklist rows → done), `git mv` to `docs/plans/archive/`, rows in `docs/plans/README.md` and `docs/plans/archive/README.md`

- [x] **Step 1: CHANGELOG bullet** (one sentence, ≤ 35 words, artefact class = building block):

`- \`openehr/terminology\`: the openEHR Terminology (TERM Release-3.0.0) vendored and generated into a stdlib-only accessor; lifecycle-state, audit-change-type, participation-mode and ctx/ default codes now read it (REQ-034).`

- [x] **Step 2: Indexes and statuses** — as listed under Files; the plans README row goes in a new `### openEHR terminology vocabulary (2026-09-07)` section at the top of *Active plans* (the archived-row style the Go 1.27 section uses).

- [x] **Step 3: Gates**

```bash
make spec-check
make ci
```

- [x] **Step 4: Archive + commit**

```bash
git mv docs/plans/2026-09-07-terminology-vocabulary.md docs/plans/archive/
git add -u docs AGENTS.md CHANGELOG.md
git commit -m "docs(terminology): REQ-034 landed — module layout, REQ-013 set, roadmap, CHANGELOG, traceability; plan archived" -m "Assisted-by: Claude Code (<model id>)"
```

## Verification commands (all tasks)

```
go test ./openehr/terminology/ ./internal/termgen/ ./cmd/termgen/ -count=1
go test ./openehr/client/... ./openehr/serialize/... ./openehr/instance/... ./testkit/... -count=1
make termgen-verify && make terminology-verify
make lint && make spec-check
make ci
```
