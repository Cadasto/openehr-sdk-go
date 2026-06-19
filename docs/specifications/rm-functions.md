# RM behavioural functions

**Status:** Draft

Normative contract for the **derived / behavioural functions** the openEHR Reference Model defines on its identifier, `PATHABLE`/`LOCATABLE`, and version-control classes — the operations that `bmmgen` emits as signatures from the pinned BMM but cannot implement from a schema (they carry algorithm, not just shape). Covers REQ-120 through REQ-122.

Covers REQ-120 through REQ-123. Structural RM rules (how the types are generated and shaped) are in [rm-modeling.md](rm-modeling.md) and [bmm-conformance.md](bmm-conformance.md); this spec governs the **runtime behaviour** of the functions on those types. Identifier lexical forms are authoritative in the openEHR BASE *Identification* package and the RM *Common* package; agents **MUST** look them up there ([base_types identification](https://specifications.openehr.org/releases/BASE/development/base_types.html#_identification_package), [RM common](https://specifications.openehr.org/releases/RM/development/common.html)) rather than guess.

Conventions (RFC-2119 keywords, status axes): [README.md](README.md). Surface and fallibility decisions: [ADR 0011](../adr/0011-rm-behavioural-functions-surface.md).

## Surface and fallibility (applies to all REQs here)

Per [ADR 0011](../adr/0011-rm-behavioural-functions-surface.md):

- Concrete-typed derivations (identifier components, `is_branch`) **MUST** be exposed as methods on the RM type, implemented in hand-written `*_funcs.go` files alongside the generated `*_gen.go` (the generator's documented "implement in a non-generated file" extension point).
- A fallible parse (input may be malformed) **MUST** additionally offer an error-returning entry point (e.g. a package-level `Parse…` function); the BMM-signature method returns a best-effort value and an `ok`/error companion.
- Library code **MUST NOT** panic on malformed identifier strings or on resolving an absent path — failures surface as a returned error, `ok` boolean, or typed nil, per [idiom.md § Errors (REQ-025)](idiom.md#errors-req-025) and [§ Concurrency](idiom.md).
- Navigation **MUST** remain reflection-free (typed dispatch) per [idiom.md § Generics policy (REQ-024)](idiom.md#generics-policy-req-024).

## REQ-120 — RM identifier parsing and derivation

The SDK **MUST** expose the derived components of the openEHR identifier types from their normative lexical forms, on the corresponding `openehr/rm` types:

- `UID_BASED_ID` (lexical form `root '::' extension`) **MUST** derive `root`, `extension`, and `has_extension`. `HIER_OBJECT_ID` and `OBJECT_VERSION_ID` inherit these; an `OBJECT_VERSION_ID`'s `object_id` is its `root`.
- `OBJECT_VERSION_ID` (lexical form `object_id '::' creating_system_id '::' version_tree_id`) **MUST** derive `object_id`, `creating_system_id`, `version_tree_id`, and `is_branch`.
- `VERSION_TREE_ID` (lexical form `trunk_version [ '.' branch_number '.' branch_version ]`, 1 or 3 dot-separated parts) **MUST** derive `trunk_version`, `branch_number`, `branch_version`, and `is_branch` (true iff the 3-part branch form is present).
- `ARCHETYPE_ID` (lexical form `rm_originator '-' rm_name '-' rm_entity '.' concept { '-' specialisation }* '.v' version_id`) **MUST** derive `rm_originator`, `rm_name`, `rm_entity`, `qualified_rm_entity`, `domain_concept`, `specialisation`, and `version_id`.
- `TERMINOLOGY_ID` (lexical form `name [ '(' version ')' ]`) **MUST** derive `name` and `version_id`.
- `OBJECT_REF` / `PARTY_REF` and the `PARTY_PROXY` family **SHOULD** expose convenience accessors for their reference and identity components; `LOCATABLE_REF` **SHOULD** provide `as_uri()` — the `ehr:`-scheme URI built from `namespace` + `id.value` + `path`.

There **MUST** be a **single canonical parser** for each identifier form, owned in `openehr/rm`; existing client-side helpers (e.g. the version-uid splitter in `openehr/client/ehr`) **MUST** delegate to it rather than re-parse — one canonical home, no duplicate lexical logic.

A malformed identifier string **MUST NOT** panic: the error-returning parser **MUST** return a non-nil error, and the best-effort methods **MUST** return a zero value with a companion `ok`/error rather than crash.

**Acceptance:** for canonical and malformed sample strings of each form, the derived components equal the spec's lexical decomposition; malformed input yields an error (no panic); the client version-uid helper produces identical results to the canonical parser.

**Out of scope:** validity *checking* of the embedded `UID` syntax beyond decomposition; generation of new identifiers (covered for instance synthesis by REQ-107).

## REQ-121 — Locatable path read access

The SDK **MUST** provide read navigation of an in-memory RM instance by an openEHR path, implementing the `PATHABLE` read operations over the actual object tree:

- `item_at_path(path)` **MUST** return the single item at a **unique** path, and **MUST** return an error when the path is absent or resolves to more than one item.
- `items_at_path(path)` **MUST** return all items matching a **non-unique** path (empty when none match).
- `path_exists(path)` **MUST** report whether the path resolves to at least one item; `path_unique(path)` **MUST** report whether it resolves to exactly one.

The accepted path grammar **MUST** follow the openEHR path syntax — `/`-separated RM attribute-name segments with optional `[archetype_node_id]` and/or `[name]` predicates — consistent with the archetype/AQL path form already parsed in [clinical-modeling.md § REQ-100](clinical-modeling.md#req-100--adl-14-operational-template-opt-parse-and-paths) and [§ REQ-109](clinical-modeling.md#req-109--aql-static-lint). Resolution against the instance **MUST** be reflection-free.

The read operations **MUST** be available as a building-block package (no `transport/` or `auth/` import, per [module-layout.md § REQ-013](module-layout.md#req-013--building-block-independence)). Because that package imports `openehr/rm`, the `LOCATABLE` path methods cannot delegate to it without an import cycle; the generated `LOCATABLE.{item_at_path,items_at_path,path_exists,path_unique}` stubs are therefore **suppressed** (not emitted) and the package functions are the canonical surface (see [ADR 0011](../adr/0011-rm-behavioural-functions-surface.md) decision refinement 2).

**Acceptance:** against bundled example templates (e.g. `vital_signs.opt`, `clinical_note.opt`), a known unique leaf path resolves via `item_at_path` to the expected value; a non-unique path returns the expected item count via `items_at_path`; an absent path reports `path_exists = false`; a multi-match path reports `path_unique = false` and `item_at_path` errors.

**Out of scope:** `parent` and `path_of_item` (the inverse: object → its path). These require parent back-pointers the concrete RM structs do not carry and identity-based tree search; they remain documented, non-panicking stubs until a consumer need is established.

## REQ-122 — Version-control derived helpers

The SDK **MUST** implement the pure, derived version-control functions, computed from the version identifier:

- `VERSION.is_branch` **MUST** be derived from the version's `uid` (true iff its `version_tree_id` is a branch, per REQ-120).

Client-side version *container management* is **out of scope** and **MUST NOT** be implemented as in-memory RM behaviour: `VERSIONED_OBJECT` operations (`version_count`, `all_versions`, `all_version_ids`, `has_version_at_time`, `has_version_id`, `version_at_time`, `version_with_id`, `latest_version`, `latest_trunk_version`, `trunk_lifecycle_state`) and all `commit_*` mutators are server-mediated in this SDK — they are realised over REST in `openehr/client/ehr` and `openehr/client/demographic`, not against a materialised in-memory container. These methods **MUST** remain explicit, documented stubs that fail loudly rather than return a misleading value (e.g. a `version_count` of `0`).

**Acceptance:** `VERSION.is_branch` is true for a branch version uid and false for a trunk uid; the out-of-scope container operations are documented as server-side and do not silently return zero values.

## REQ-123 — Temporal data-value helpers

The SDK **MUST** expose read, inspection, comparison, and conversion helpers for the ISO 8601-backed temporal data values `DV_DATE`, `DV_TIME`, `DV_DATE_TIME`, and `DV_DURATION`, parsing each type's `value` string per ISO 8601 (including openEHR's documented `DV_DURATION` deviations: a leading negative sign and mixing the `W` designator with others).

- **Component access** — each type **MUST** expose the components of its parsed form: `DV_DATE` → `year`/`month`/`day`; `DV_TIME` → `hour`/`minute`/`second`/`fractional_second`; `DV_DATE_TIME` → their union; all with `timezone` where present; `DV_DURATION` → `years`/`months`/`weeks`/`days`/`hours`/`minutes`/`seconds`/`fractional_seconds`.
- **Partial-form inspection** — `DV_DATE`/`DV_DATE_TIME`/`DV_TIME` **MUST** report partial forms (`is_partial`, and for dates `month_unknown`/`day_unknown`), since openEHR admits `"2024"` / `"2024-03"` approximate values that Go's `time.Time` cannot represent.
- **Magnitude & comparison** — each type **MUST** expose `magnitude()` (days for `DV_DATE`; seconds for `DV_TIME`/`DV_DATE_TIME`/`DV_DURATION`) and a total ordering consistent with the spec's `less_than` / `is_strictly_comparable_to`.
- **Go bridge** — each type **SHOULD** offer idiomatic conversion (`DV_DATE`/`DV_TIME`/`DV_DATE_TIME` → `time.Time`; `DV_DURATION` → `time.Duration`), returning an error when the value is partial — or, for a duration, carries calendar-nominal `Y`/`M` components — and so cannot map cleanly.

A malformed `value` **MUST** surface as an error per the surface/fallibility policy above; resolution **MUST NOT** panic.

**Acceptance:** for canonical and partial sample strings of each type, the component accessors and `is_partial` match the ISO 8601 decomposition; `magnitude()` and comparison order a known sequence correctly; the Go-bridge conversion succeeds for full values and errors for partial/nominal ones.

**Out of scope:** temporal **arithmetic** — `add`/`subtract`/`diff` against `DV_DURATION`, `DV_DURATION.multiply`/`negative`, and the calendar-aware `add_nominal`/`subtract_nominal` (leap-year / short-month semantics). Deferred to a follow-up REQ; the generated arithmetic methods remain documented stubs.

## Editing rules

New behavioural functions get a new REQ in the 120–129 band; identifiers are stable once published. When code lands, set the registry `Impl.` column in [REQ.md](REQ.md) and the `implementation:` field in [traceability.yaml](traceability.yaml), and link each REQ section to its implementing package(s).
