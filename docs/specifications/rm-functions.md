---
kind: specification
---

# RM behavioural functions

**Status:** Draft

Normative contract for the **derived / behavioural functions** the openEHR Reference Model defines on its identifier, URI, `PATHABLE`/`LOCATABLE`, version-control, and temporal data-value classes — the operations that `bmmgen` emits as signatures from the pinned BMM but cannot implement from a schema (they carry algorithm, not just shape).

Covers REQ-120 through REQ-123. Structural RM rules (how the types are generated and shaped) are in [rm-modeling.md](rm-modeling.md) and [bmm-conformance.md](bmm-conformance.md); this spec governs the **runtime behaviour** of the functions on those types. Identifier lexical forms are authoritative in the openEHR BASE *Identification* package and the RM *Common* package; agents look them up there ([base_types identification](https://specifications.openehr.org/releases/BASE/development/base_types.html#_identification_package), [RM common](https://specifications.openehr.org/releases/RM/development/common.html)) rather than guess.

Conventions (RFC-2119 keywords, status axes): [README.md](README.md). Surface and fallibility decisions: [ADR 0011](../adr/0011-rm-behavioural-functions-surface.md).

## Surface and fallibility (applies to all REQs here)

[ADR 0011](../adr/0011-rm-behavioural-functions-surface.md) sets the surface. A concrete-typed derivation (identifier components, `is_branch`) is a method on the RM type, written by hand beside the generated code, at the generator's documented "implement in a non-generated file" extension point. A fallible parse (input may be malformed) also offers an error-returning entry point: a package-level `Parse…` function for the identifier forms, and the `ToTime` / `ToDuration` conversions for the temporal forms. The BMM-signature derivation methods are **best-effort**: they return a zero value on malformed input, and the package-level error-returning function is the validity check (there is no per-method `ok` boolean). Where a derivation method needs no validation beyond decomposition (e.g. `VersionTreeID.is_branch`), it stays purely lexical, and only the `Parse…` path speaks to well-formedness.

None of these functions panics on a malformed identifier string, on resolving an absent path, or on a typed-nil node in the object tree: the no-panic rule is [idiom.md § Errors (REQ-025)](idiom.md#errors-req-025)'s. Navigation uses no reflection ([idiom.md § Generics and reflection](idiom.md#generics-and-reflection)).

## REQ-120 — RM identifier parsing and derivation

The SDK **MUST** expose the derived components of the openEHR identifier types, and of `DV_URI`, from their normative lexical forms, on the corresponding `openehr/rm` types:

- `UID_BASED_ID` (lexical form `root '::' extension`): `root`, `extension`, and `has_extension`. `HIER_OBJECT_ID` and `OBJECT_VERSION_ID` inherit these; an `OBJECT_VERSION_ID`'s `object_id` is its `root`.
- `OBJECT_VERSION_ID` (lexical form `object_id '::' creating_system_id '::' version_tree_id`): `object_id`, `creating_system_id`, `version_tree_id`, and `is_branch`.
- `VERSION_TREE_ID` (lexical form `trunk_version [ '.' branch_number '.' branch_version ]`, 1 or 3 dot-separated parts): `trunk_version`, `branch_number`, `branch_version`, `is_branch` (true iff the 3-part branch form is present), and `is_first` (true iff the trunk version is `1`).
- `ARCHETYPE_ID` (lexical form `rm_originator '-' rm_name '-' rm_entity '.' concept { '-' specialisation }* '.v' version_id`): `rm_originator`, `rm_name`, `rm_entity`, `qualified_rm_entity`, `domain_concept`, `specialisation`, and `version_id`.
- `TERMINOLOGY_ID` (lexical form `name [ '(' version ')' ]`): `name` and `version_id`.
- `DV_URI` (an RFC 3986 URI reference, `[ scheme ':' ] hier-part [ '?' query ] [ '#' fragment ]`): `scheme`, `path`, `query` and `fragment_id`, which `DV_EHR_URI` inherits. Each is the RFC 3986 component as written, not percent-decoded, or empty when the reference has none. A relative reference has no scheme, and the path leaves out the authority (the `//system_id` of an `ehr://` URI). The RM admits plain text as a `DV_URI` value, so these functions split any text: the text before the first colon is the scheme only when RFC 3986 § 3.1 admits it as one (a letter, then letters, digits, `+`, `-` or `.`), and the rest splits as RFC 3986 appendix B reads a reference. A nil `DV_URI` or `DV_EHR_URI` answers empty for each, and the malformed-string rule below does not apply to them.

`OBJECT_REF` / `PARTY_REF` and the `PARTY_PROXY` family **SHOULD** expose convenience accessors for their reference and identity components, and `LOCATABLE_REF` **SHOULD** provide `as_uri()` — the `ehr:`-scheme URI built from `namespace` + `id.value` + `path` — because callers otherwise rebuild these from the fields by hand.

Each fallible identifier form has one error-returning parser in `openehr/rm` (`ParseObjectVersionID`, `ParseVersionTreeID`, `ParseArchetypeID`, `ParseTerminologyID`), and client-side helpers such as the version-uid splitter in `openehr/client/ehr` delegate to it rather than re-parse. `HIER_OBJECT_ID` / `UID_BASED_ID` decomposition is best-effort only, with no `Parse…` entry point.

For a malformed identifier string, the `Parse…` function **MUST** return a non-nil error and the best-effort derivation methods **MUST** return a zero value; neither panics. `DV_URI`'s functions are not identifier derivations in this sense: they split any text, as its bullet above says.

**Acceptance:** for canonical and malformed sample strings of each identifier form, `Parse…`, where the form has one, round-trips the input and derived components equal the spec's lexical decomposition; malformed input yields an error from `Parse…` (no panic); best-effort methods never panic; the client version-uid helper produces identical results to the canonical parser. The `DV_URI` functions split sample references with and without each component, a `DV_EHR_URI` decoded from a `LINK` target answers them without a panic, and a nil `DV_URI` or `DV_EHR_URI` answers empty.

**Out of scope:** validity *checking* of the embedded `UID` syntax beyond decomposition; generation of new identifiers (covered for instance synthesis by REQ-107). Of the recommended convenience accessors, only `LOCATABLE_REF.as_uri` exists; the `OBJECT_REF` / `PARTY_REF` / `PARTY_PROXY` ones are deferred, and the underlying fields are already directly accessible on the generated structs.

## REQ-121 — Locatable path read access

The SDK **MUST** provide read navigation of an in-memory RM instance by an openEHR path, implementing the `PATHABLE` read operations over the actual object tree:

- `item_at_path(path)` **MUST** return the single item at a **unique** path, and **MUST** return an error when the path is absent or resolves to more than one item.
- `items_at_path(path)` **MUST** return all items matching a **non-unique** path (empty when none match).
- `path_exists(path)` **MUST** report whether the path resolves to at least one item; `path_unique(path)` **MUST** report whether it resolves to exactly one.

The accepted path grammar **MUST** follow the openEHR path syntax — `/`-separated RM attribute-name segments with optional `[archetype_node_id]` and/or `[name]` predicates — consistent with the archetype/AQL path form already parsed in [clinical-modeling.md § REQ-100](clinical-modeling.md#req-100--adl-14-operational-template-opt-parse-and-paths) and [§ REQ-109](clinical-modeling.md#req-109--aql-static-lint). An `ISM_TRANSITION` is `PATHABLE` but not `LOCATABLE`, so it carries no `archetype_node_id`; a node-id predicate on `ism_transition` **MUST** therefore match the transition whose `careflow_step` is coded with that id in the `local` terminology. That is the ADL 1.4 convention by which an ACTION archetype's `ism_transition[atNNNN]` constrains `careflow_step` to `local::atNNNN`. A careflow step whose terminology is blank counts as `local`, the reading FLAT decode gives a careflow step that names none ([wire.md § REQ-053](wire.md#req-053)). A transition with no careflow step, another code, or the node id in another terminology **MUST NOT** match the predicate.

The read operations live in a building-block package with no `transport/` or `auth/` import ([module-layout.md § REQ-013](module-layout.md#req-013--building-block-independence)). Because that package imports `openehr/rm`, the `LOCATABLE` path methods cannot delegate to it without an import cycle; the generated `LOCATABLE.{item_at_path,items_at_path,path_exists,path_unique}` stubs are therefore **suppressed** (not emitted) and the package functions are the canonical surface (see [ADR 0011](../adr/0011-rm-behavioural-functions-surface.md) decision refinement 2).

**Attribute coverage for Web Template leaves.** Path navigation is the resolution step the FLAT encoder ([wire.md § REQ-053](wire.md#req-053)) runs before leaf mapping, so an attribute this package cannot navigate is data the encoder cannot emit. Two guard tests beside the Web Template builder hold navigation to the leaves a Web Template carries. The first covers every RM attribute leaf the Web Template export synthesizes from the RM rather than from the template (the *in-context* leaves of [REQ-106](clinical-modeling.md#req-106--webtemplate-json-export)): each resolves through these read operations on a populated instance of its container RM type, or is a documented exemption that records why it stays unresolved and what that costs on encode. A newly synthesized in-context leaf with no navigation support fails that test rather than reach the encoder, and an exemption that no longer matches an emitted leaf is reported as stale. The second covers the nodes the Web Template builds from the template itself, over the vendored operational templates whose root is a `COMPOSITION` (the FLAT encoder takes compositions only): each RM attribute on the path of a node resolves on an instance populated along that path, or is a documented exemption with its reason. The exemptions are the composition metadata the `ctx/` short forms carry on encode (`/language`, `/territory`, `/context/start_time` and `/context/setting`), which resolving would spell under two keys, and attributes the RM derives rather than stores (`EVENT.offset`, a function).

The FLAT encoder **MUST NOT** silently drop a leaf that resolves and whose datatype it can serialise. Resolution failure **MAY** be treated as an absent optional (that is what an unpopulated optional attribute looks like); an absent leaf whose datatype the encoder cannot write **MAY** stay unemitted. A populated value at that leaf is covered by [wire.md § REQ-140](wire.md#req-140--underscore-prefixed-rm-attributes). Neither licence extends to a resolvable, serialisable leaf — an omission there is a defect, not an absent optional.

The exemption list is kept by hand because `item_at_path` returns the same `ErrPathNotFound` whether the addressed attribute is *unknown to the navigator* or *known but unpopulated*. Whether to split that sentinel (e.g. `ErrUnknownAttribute`) and retire the exemption list is open as [STRAND-10](research-strands.md#strand-10--rmpath-one-sentinel-for-two-not-found-conditions), and code leaves it open until that strand resolves.

**Acceptance:** against a representative composition (the blood-pressure / vital-signs shape, with real archetype-node-id and name predicates), a known unique leaf path resolves via `item_at_path` to the expected value; a non-unique path returns the expected item count via `items_at_path`; an absent path reports `path_exists = false`; a multi-match path reports `path_unique = false` and `item_at_path` errors. A typed-nil node anywhere in the tree (including the root) yields no match rather than a panic. For the in-context coverage rule: every leaf the WebTemplate synthesizes resolves on a populated instance of its container RM type, or appears in the exemption list with its reason and encode consequence — and a stale exemption fails the guard.

**Out of scope:** `parent` and `path_of_item` (the inverse: object → its path). These require parent back-pointers the concrete RM structs do not carry and identity-based tree search; they remain documented, fail-loud panic stubs until a consumer need is established.

## REQ-122 — Version-control derived helpers

The SDK **MUST** implement the pure, derived version-control functions, computed from the version identifier:

- `VERSION.is_branch` **MUST** be derived from the version's `uid` (true iff its `version_tree_id` is a branch, per REQ-120).

Client-side version *container management* is **out of scope** and **MUST NOT** be implemented as in-memory RM behaviour: `VERSIONED_OBJECT` operations (`version_count`, `all_versions`, `all_version_ids`, `has_version_at_time`, `has_version_id`, `version_at_time`, `version_with_id`, `latest_version`, `latest_trunk_version`, `trunk_lifecycle_state`) and all `commit_*` mutators are server-mediated in this SDK — they are realised over REST in `openehr/client/ehr` and `openehr/client/demographic`, not against a materialised in-memory container. These methods **MUST** remain explicit, documented stubs that fail loudly rather than return a misleading value (e.g. a `version_count` of `0`).

**Acceptance:** `VERSION.is_branch` is true for a branch version uid and false for a trunk uid; the out-of-scope container operations are documented as server-side and do not silently return zero values.

## REQ-123 — Temporal data-value helpers

The SDK **MUST** expose read, inspection, comparison, and conversion helpers for the ISO 8601-backed temporal data values `DV_DATE`, `DV_TIME`, `DV_DATE_TIME`, and `DV_DURATION`, parsing each type's `value` string per ISO 8601 (including openEHR's documented `DV_DURATION` deviations: a leading negative sign and mixing the `W` designator with others), in its extended and basic layouts, with a comma or a dot before fractional seconds and the zone forms `Z`, `±hh`, `±hhmm` and `±hh:mm`, refusing `24:00:00` as BASE states, and accepting two forms the BASE predicates do not list, as SDK deviations: a zone whose style differs from the layout of the time it follows (`10:30:00+0530`, `103000+05:30`), and a `DV_DATE_TIME` without its hour (`2024`, `2024-03`, `2024-03-15`), where BASE `Iso8601_date_time` needs one.

- **Component access** — each type **MUST** expose the components of its parsed form: `DV_DATE` → `year`/`month`/`day`; `DV_TIME` → `hour`/`minute`/`second`/`fractional_second`; `DV_DATE_TIME` → their union; all with `timezone` where present; `DV_DURATION` → `years`/`months`/`weeks`/`days`/`hours`/`minutes`/`seconds`/`fractional_seconds`.
- **Partial-form inspection** — `DV_DATE`/`DV_DATE_TIME`/`DV_TIME` **MUST** report partial forms (`is_partial`, and for dates `month_unknown`/`day_unknown`), since openEHR admits `"2024"` / `"2024-03"` approximate values that Go's `time.Time` cannot represent.
- **Magnitude & comparison** — each type **MUST** expose `magnitude()` (days for `DV_DATE`; seconds for `DV_TIME`/`DV_DATE_TIME`/`DV_DURATION`, a duration's being its length in seconds within the precision of the result, computed without an intermediate integer overflow; a `DV_TIME`'s counting the seconds from the start of its day as written, its time zone not applied; a `DV_DATE_TIME`'s counting the seconds from `0001-01-01T00:00:00Z`, as the RM defines it, a value with a time zone normalised to UTC and a value without one counting its clock time) and a total ordering consistent with the spec's `less_than` / `is_strictly_comparable_to`; the package-level `Compare` helper **MAY** implement the shared ordering logic for idiomatic Go callers.
- **Go bridge** — each type **SHOULD** offer idiomatic conversion (`DV_DATE`/`DV_TIME`/`DV_DATE_TIME` → `time.Time`; `DV_DURATION` → `time.Duration`), because a Go caller computes with those types; a value that cannot map cleanly gets an error, as below.

These helpers **MUST NOT** panic on any value or receiver, and **MUST** answer a value they cannot use as follows:

- the component accessors, the partial-form inspections and `magnitude()` answer zero, the empty string or false for a value they cannot read and for a nil receiver, components parsed before the error included, so a malformed value is never reported as partial;
- `ToTime` and `ToDuration` return a non-nil error for a malformed, partial or calendar-nominal (`Y`/`M` duration components) value, for a definite duration too long for `time.Duration` (never a value wrapped around by overflow), and for a nil receiver;
- for a leap second (second `60`) on a `DV_TIME` or `DV_DATE_TIME`, `ToTime`'s error wraps `rm.ErrTemporalConversion`: a Go `time.Time` cannot hold it, and normalising it into the next minute would change the instant the value names.

The leap second is still a valid ISO 8601 value: the accessors and `is_partial` read it as written. A duration component too large for a Go `int` is unparseable input.

**Acceptance:** for canonical, partial and basic-layout sample strings of each type, with comma fractions, each zone form and the two SDK deviations (`10:30:00+0530`, a `DV_DATE_TIME` of `2024`), the component accessors and `is_partial` match the ISO 8601 decomposition; `magnitude()` and comparison order a known sequence correctly, and the `DV_DATE_TIME` `2026-01-01T10:00:00+05:00`, which is `05:00Z`, compares below `2026-01-01T08:00:00Z`; the Go-bridge conversion succeeds for full values and errors for partial/nominal ones; malformed samples (an illegal day such as `2024-04-31`, a repeated or misordered duration designator such as `P1Y2Y`) return an error from `ToTime`/`ToDuration`; `23:59:60` and `2016-12-31T23:59:60Z` return an error wrapping `rm.ErrTemporalConversion` from `ToTime`; and malformed samples give zero components and no partial flag, and never panic. A nil receiver never panics and answers as an unparseable value does. `ToDuration` errors for `PT9999999999999H` and `P400000D`, and for a length whose intermediate sum or product would wrap back into range, such as `PT18446744074S`; `PT1.9999999999S` converts to 2 s, its fraction rounded to the nearest nanosecond. `magnitude()` of `PT9999999999999999H` is that duration's length in seconds within the precision of the result, and `PT9999999999999999999H`, whose hours do not fit an `int`, answers as an unparseable value does.

**Out of scope:** temporal **arithmetic** — `add`/`subtract`/`diff` against `DV_DURATION`, `DV_DURATION.multiply`/`negative`, and the calendar-aware `add_nominal`/`subtract_nominal` (leap-year / short-month semantics). Deferred to a follow-up REQ; the generated arithmetic methods remain documented stubs.

## Editing rules

[README.md § Editing rules](README.md#editing-rules), [§ Identifier scheme](README.md#identifier-scheme) and [§ Traceability](README.md#traceability).
