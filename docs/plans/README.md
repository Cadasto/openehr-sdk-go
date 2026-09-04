# Implementation plans

Active and archived implementation plans for `openehr-sdk-go`. Plans derive from [`../../docs/specifications/`](../../docs/specifications/) — they translate normative REQs into sequenced delivery.

**Landed vs planned checklist:** [`../roadmap.md`](../roadmap.md). **Completed or superseded plans:** [`archive/README.md`](archive/README.md).

## Active plans

### Leftover sweeps (2026-09-04)

Two small plans draining the verified-open follow-ups the 2026-08-30 → 2026-09-03 review rounds left in PR bodies and session notes. Each amends landed requirements in place — no new id — and each lands in its own PR.

| Plan | Scope | Covers | Notes |
|---|---|---|---|
| [archive/2026-09-04-spec-interop-leftovers.md](archive/2026-09-04-spec-interop-leftovers.md) | **Landed 2026-09-05 and archived**: § REQ-050 binds the System descriptor's unknown keys to the § REQ-144 rule; `simplified.ParseMediaType` / `Format.MediaType` accept EHRbase's `.schema`-suffixed media types on input and emit only the canonical strings; scope.md's examples row describes the tree; a `bmmgen` census pins the single property inherited from a primitive-mapped ancestor across every pinned schema root (STRAND-13 evidence); STRAND-14 is opened on chaining the RM floor into template-driven validation and § REQ-112 states the composition; REQ-095's `partial` has a named gap list in the cassette README | REQ-050 / 053 / 095 / 112 (no new id); STRAND-13 evidence; STRAND-14 opened | STRAND-13/14 stay open (each needs an ADR); REQ-095 stays `partial` with its gaps named |

### Go 1.27 floor (2026-09-03)

| Plan | Scope | Covers | Notes |
|---|---|---|---|
| [archive/2026-09-03-go-1.27.md](archive/2026-09-03-go-1.27.md) | **Landed 2026-09-03 and archived**: raised the module floor to Go 1.27.0 (REQ-002) and adopted 1.27 embedlit, CutLast, stdlib uuid generation, Registry.DecodeAs, URL.Clone, rand.N, and synctest on four sleep tests | REQ-002; STRAND-04 timeline note only | json/v2 canjson migration stays deferred |

### Deferred-work backlog (2026-09-01 / 02)

Five plans from a consumer→SDK gap survey plus the review-round follow-ups that were living only in session notes — four already landed and archived (below), the remaining one is **not approved for implementation yet** — it exists so the work is ready. All amend shipped REQs in place (normative deltas ride in the implementing PR) **except** the RM-function stubs, which propose new ids. Landing the decode-error-surface plan settled the group's named cross-plan seam: it wired `canjson.ErrInvalidShape` (option A) rather than retiring it, so the fidelity plan's `Real`-precision task reuses that sentinel instead of minting its own — and the fidelity plan itself landed 2026-09-03, closing that reuse.

| Plan | Scope | Covers | Notes |
|---|---|---|---|
| [archive/2026-09-01-rm-canonical-json-fidelity.md](archive/2026-09-01-rm-canonical-json-fidelity.md) | **Landed 2026-09-03 and archived**: `TERM_MAPPING.match` is a canonical single-character `Character` named primitive, `DV_TEXT.mappings`'s re-encode collapse is documented and pinned (decode preserves `[]` versus `null`/absent), the RM floor enforces `match`'s value set and `Mappings_valid`, a `DV_MULTIMEDIA` base64 round-trip is pinned, and `Real` now refuses a literal past the 17-significant-digit budget instead of rounding silently | REQ-046 / 052 / 112 (no new id; REQ-052 `partial` at landing, `landed` since 2026-09-03) | Reported by the consuming CDR project |
| [2026-09-01-rm-function-deferred-stubs.md](2026-09-01-rm-function-deferred-stubs.md) | Realise the two `rm-functions` panic-stub clusters — `DV_AMOUNT` arithmetic, and reference accessors + inverse navigation | proposed REQ-124 / 125 | **Parked** behind a YAGNI/DoR gate — no consumer need yet; `parent`/`path_of_item` needs an ADR |
| [archive/2026-09-02-decode-error-surface-typing.md](archive/2026-09-02-decode-error-surface-typing.md) | **Landed 2026-09-02 and archived**: the decode-only `canjson.ErrInvalidShape` gained a producer — every generated RM/AOM `UnmarshalJSON` now classifies its JSON-shape failures with it, text and cause unchanged — and `transport`'s error strings stop falling back to `Request.Path`, rendering the stable `(unrouted)` placeholder when the caller set no route template | REQ-052 (`partial` then, `landed` since 2026-09-03) / REQ-093 (no new id) | The Extras re-encode candidate was verified already fixed and dropped |

The AQL + REQ-151 error leftovers plan (the fifth of this batch) **landed 2026-09-02 and was archived** ([archive/2026-09-02-aql-req151-error-leftovers.md](archive/2026-09-02-aql-req151-error-leftovers.md)): REQ-057's stored-query `version` path parameter is now trimmed once and reused, matching `name`; REQ-025's nil-receiver guard idiom (`ok && x != nil` after `errors.AsType`) is applied to the `parse.SyntaxError` / `typereg.DecodeError` inspection sites in `openehr/aql/lint`, `openehr/aql/parse` and `openehr/rm/typereg`, and `SyntaxError.Error()` stops printing a fabricated `at 0:0:` for a zero position. PROBE-099 gained a discriminating supplied-relation row (**REQ-164**) proving containment-relation threading is mutation-detectable at probe level.

The `ehr.Create` empty-2xx typing plan **landed 2026-09-02 and was archived** ([archive/2026-09-01-ehr-create-empty-2xx-typing.md](archive/2026-09-01-ehr-create-empty-2xx-typing.md)): REQ-094 stays `landed` (implementation-aligned amendment). `ehr.Create`'s empty/null-body 2xx arm now returns a typed `*ehr.NoRepresentationError`, matching the versioned-write family, instead of the bare `transport.ErrInvalidShape` that was REQ-094's one keyed exception; the decode-failure arm stays `*transport.DecodeError` per REQ-151, and `ehr.Get`/`ehr.Exists`/`ehr.GetBySubject` are untouched.

### Read-path decode taxonomy and Definition metadata decoding (2026-08-30)

Two independent plans, each authoring its spec deltas in a Phase 0 (via `sdd-specify`) before
implementation; both landed 2026-08-30 and are archived.

The read-path decode taxonomy plan **landed 2026-08-30 and was archived**
([archive/2026-08-30-read-path-decode-taxonomy.md](archive/2026-08-30-read-path-decode-taxonomy.md)):
[REQ-151](../specifications/transport.md#req-151--typed-2xx-decode-failure) is `landed` and
PROBE-101 is Implemented (Sandbox). A 2xx body that will not decode as the requested
representation now fails with `*transport.DecodeError`, carrying the raw bytes the server already
delivered — with the request's method, its route template and the codec's own error beside them —
instead of dropping them; `Error()` stays value-free and the bytes are attached by default
([ADR 0018](../adr/0018-raw-bytes-on-decode-error.md)). `transport.Decode` and every hand-rolled
leaf decode return the same type, so which implementation route a leaf took is invisible to the
caller. The disjoint arms are pinned negatively: a non-2xx stays `*transport.WireError`, an empty
representation body stays `transport.ErrInvalidShape`, and the REQ-094 write-result funnel and
`Prefer=identifier` arm keep their own contracts. The rider closed the second read-path deferral
from the archived [write-result plan](archive/2026-08-18-write-result-contract.md): a
`canjson.ErrInvalidValue` encode sentinel under REQ-052, `errors.Is`-distinct from the decode-side
shape sentinel and from the transport one. **Deferred:** lifting REQ-094's empty-body keyed
exception for `ehr.Create` (closed — see the `ehr.Create` empty-2xx typing row above), the
`auth` / `smart` JSON decodes, and aligning `canxml`'s single both-directions sentinel.

The Definition metadata decoding plan **landed 2026-08-30 and was archived**
([archive/2026-08-30-definition-metadata-decoding.md](archive/2026-08-30-definition-metadata-decoding.md)):
[REQ-144](../specifications/wire.md#req-144--definition-metadata-decoding) is `landed`, and
PROBE-093 keeps its catalog entry unchanged. The two Definition catalog timestamps
(`TemplateMetadata.created_timestamp`, `StoredQueryMetadata.saved`) now decode against a closed
five-layout set ([ADR 0019](../adr/0019-definition-timestamp-tolerance.md)) instead of RFC 3339
only, so a single zone-less or space-separated value no longer fails a whole catalog: absent,
`null` or empty yields the zero time, an unparseable value fails naming the field, and encode
stays RFC 3339. Zone-less values decode as UTC and therefore re-marshal with a `Z` the wire never
carried — the recorded round-trip asymmetry. `ListTemplates` and `ListStoredQueries` also return a
non-nil zero-length slice for an empty 2xx body, the read-side twin of REQ-094's typed-nil trap.
The `saved` half of the tolerance is the one keyed exception § REQ-095 now names, granted on
deployment evidence rather than pre-emption.

### AQL alignment audit follow-ups (2026-08-26)

Four plans from the 2026-08-26 AQL alignment audit (maintainer's knowledge base, ecosystem
fit-gap report Part 2, findings AQL-FIT-01..10), one per audit group. Each plan authors its
spec deltas in a Phase 0 (via `sdd-specify`) before implementation; the two new requirement ids
continue the AQL semantics band (160–169). The plans are mutually independent; the corpus plan
noted a soft sequencing preference (land it before further relation extensions). All four have
now landed and are archived — the corpus plan last, so the path-shape lint's REQ-160 extension
went in ahead of the ratchet rather than behind it; the preference was soft and the extension is
covered retroactively, the corpus holding the widened relation exactly as it holds the original.

| Plan | Scope | Covers | Probe |
|---|---|---|---|
| [archive/2026-08-26-aql-write-side-parity.md](archive/2026-08-26-aql-write-side-parity.md) | **Landed 2026-08-27 and archived**: the builder spells the class- and projection-position vocabularies the read side already modelled — a sealed `VERSION` predicate carrier reached through `aql.Version`, one standing comparison in class position through `Containment.Predicated`, and a typed projection (`DISTINCT` / `AS` / aggregates / function calls / literals / star) — and `Build()` now verifies the `SELECT` it emitted, closing the last unvalidated write path REQ-119 left open and bounding `Col`'s verbatim splicing | REQ-163 (`landed`) | extends PROBE-088 (+15 goldens, +4 refusals) / PROBE-097 arm (c) (+4 parity rows) |
| [archive/2026-08-26-aql-path-shape-lint.md](archive/2026-08-26-aql-path-shape-lint.md) | **Landed 2026-08-29 and archived**: a third additive REQ-109 Layer-2 check group, five Warning codes over the query text plus the pinned BMM — the repeating-segment anchor check (`rminfo` multi-cardinality, no OPT, over `SELECT` / `WHERE` / `ORDER BY` alike), paging-without-`ORDER BY` on both channels, the missing projection alias, the fan-out advisory's path axis beside its recorded junction-source scope limit, and the provably-redundant containment step, which was specified as cuttable and **kept** | REQ-164 (`landed`); one REQ-161 scope sentence | PROBE-099 Implemented (inline); PROBE-028 deliberately re-baselined |
| [archive/2026-08-26-aql-small-corrections.md](archive/2026-08-26-aql-small-corrections.md) | **Landed 2026-08-26 and archived**: `aql.ErrEngineCapability` maps HTTP 501 as a capability gap (valid AQL this deployment does not implement) rather than bad AQL, the reserved `aql` stored-name guard refuses the SDK's own routing collision client-side alongside the corrected wire.md § REQ-055 route and EHR-scoping prose, `aql_top_with_fetch` gives the linter the envelope arm of the `TOP` pairing `Build()` already refused, and the `PredicateComparison` flat-view caveat moved to the exported godoc | amends REQ-055 / 057 / 109 / 113 / 118 — no new id | extends PROBE-021 |
| [archive/2026-08-26-aql-conformance-corpus.md](archive/2026-08-26-aql-conformance-corpus.md) | **Landed 2026-08-29 and archived**: the upstream FROM-family combination data is vendored under `testkit/cassettes/aql/conformance/` with its own provenance pin and an ingest-generated exclusion list, and PROBE-100 holds the REQ-160 relation to it — every one of the 67 engine-accepted rows, reconstructed with its consuming Robot suite's own query template, must parse and draw no Error-severity containment code, so the compatibility guard's evidence is mechanical rather than a maintainer-kept row list. The ratchet has a refresh arm too: an unlearned CSV, family or header shape is a read error rather than a partial corpus | REQ-160 evidence (no new id) | PROBE-100 Implemented (inline) |

### RM class-universe absence reasons (2026-08-26)

This plan **landed 2026-08-26 and was archived** ([archive/2026-08-26-rminfo-absence-reason.md](archive/2026-08-26-rminfo-absence-reason.md)): [REQ-049](../specifications/bmm-conformance.md#req-049--rm-class-universe-absence-reasons) is `landed` and PROBE-098 is Implemented (inline). `rminfo` now answers why a name is *not* in the class universe — undeclared, excluded package, excluded class, primitive, enumeration — through a closed `AbsenceReason` enum on an optional `AbsenceReporter` that `Default` implements, backed by a generated table emitted beside `lookup_gen.go`. *Undeclared* and *none* are computed rather than stored, precedence is fixed so what a name IS outranks why it was skipped, and a declared-but-omitted name no rule accounts for fails generation. `Lookup`, `Hierarchy`, `AttributeLister` and `New` are unchanged; `NewWithAbsence` gives tests a synthetic seam.

### AQL semantic layer (2026-08-21)

This plan **landed 2026-08-24 and was archived** ([archive/2026-08-21-aql-semantic-layer.md](archive/2026-08-21-aql-semantic-layer.md)): REQ-160, REQ-161 and REQ-162 are all `landed`, and PROBE-097 is Implemented (inline). `openehr/aql/contain` derives a containment admissibility relation at runtime from the pinned BMM plus a cited overlay table ([ADR-0017](../adr/0017-aql-semantic-layer.md)); `openehr/aql/lint` gains the REQ-161 containment checks and three portability advisories; `(*Builder).VerifyContainment` gives the write side the same judgement, opt-in, held to read/write parity with the linter. `Build()` and the parser are unchanged.

### Client path safety, write-result contract, and missing leaves (2026-08-18)

Four independent plans, specified first (REQ-150 / REQ-094 amendment / REQ-142 / REQ-143) — all four have now landed. The REQ-094 amendment was carried inside its plan and landed with the code (implementation-aligned), not ahead of it.

The write-result contract plan **landed 2026-08-20 and was archived** ([archive/2026-08-18-write-result-contract.md](archive/2026-08-18-write-result-contract.md)): REQ-094 stays `landed` (implementation-aligned amendment), PROBE-061/071 unchanged. `ehr.HasResource` is a reflection-free typed-nil guard for the absent-resource success path, and a 2xx `representation` with an empty or undecodable body — in both `WriteResult` (composition / directory / ehr_status / demographic) and `contribution.Commit` — is now a typed `ehr.NoRepresentationError` that carries the commit metadata and is distinguishable via `errors.As`, never a silent nil success. **Deferred:** a breaking `WriteOutcome[T]`, canjson marshal-error typing, and `contribution.Commit` `identifier`-slot population.

The contribution-read plan **landed 2026-08-19 and was archived** ([archive/2026-08-18-contribution-get.md](archive/2026-08-18-contribution-get.md)): REQ-142 is `landed` and PROBE-092 is Implemented (Sandbox). `contribution.Get` issues `GET /ehr/{ehr_id}/contribution/{contribution_uid}` and decodes the persisted `CONTRIBUTION`; `Repository` grew the same read — source-compatible for callers, a compile-time break for out-of-tree implementers. Empty ids fail with `ErrInvalidConfig` before any request and a 404 maps to `ErrNotFound`. Version metadata is returned for shape consistency but never required: the pin defines only `Content-Type` on `200_CONTRIBUTION`. This closes the EHR contribution surface — the pin declares no other contribution operation.

The path-segment-validation plan **landed 2026-08-19 and was archived** ([archive/2026-08-18-path-segment-validation.md](archive/2026-08-18-path-segment-validation.md)): REQ-150 is `landed` and PROBE-091 is Implemented (Sandbox). `transport.ValidatePathSegment` / `ValidateRequestPath` refuse a traversal, empty, backslash-bearing or control-character segment, and a `Route`-arity rule catches a separator smuggled inside one parameter; both run at `Do` before the URL is built, so a violation issues no request. The service root (`OPTIONS /`) is exempt. Because the arity rule is gated on `Route`, `openehr/client` carries an AST tripwire holding every `transport.Request` literal to setting it.

The template list-filters plan **landed 2026-08-19 and was archived** ([archive/2026-08-18-template-list-filters.md](archive/2026-08-18-template-list-filters.md)): REQ-143 is `landed` and PROBE-093 is Implemented (Sandbox). `ListTemplates` and `definition.Repository` carry a trailing `...ListOption` for the five ITS-REST list parameters; unset options omit their key, an explicit zero offset/fetch reaches the wire, and a negative one fails closed with no request. It was sequenced last in this set and taken out of order — it depends on nothing else here. ADL 2 stays deferred: `FormatADL14` is the only registered `TemplateFormat`, and the pin's ADL 2 list operation references the same five parameter components, so the option set carries over unchanged.

### Probe runnability — the sandbox transport and the three-mode runner (2026-08-18)

| Plan | Scope | Covers | Probe |
|---|---|---|---|
| [2026-08-18-probe-runnability.md](2026-08-18-probe-runnability.md) | Give the existing probe catalog (66 entries) the execution modes REQ-082 mandates: one shared result type + a runner (phase 1), a real `sandbox/` in-memory backend (phase 2), then recording/replay and Live (phases 3–4, blocked on access to a live CDR) | REQ-082 (**`partial`**) | none new — promotes PROBE-077 / 078 / 079 out of `Status: Deferred` and unblocks PROBE-065 (`Status: Draft`) |

Phase 0 (the REQ-082 normative prose — mode selection, the probe result contract, per-mode rules, cross-mode precedence) landed 2026-08-18 alongside [STRAND-11](../specifications/research-strands.md#strand-11--probe-recording-format-har-or-a-purpose-built-yaml), which holds the recording-format fork open until there is a capture to judge it against. Eight probes and [STRAND-09](../specifications/research-strands.md#strand-09--its-rest-conformance-follow-ups) item 1 are gated on this plan, and REQ-082 is one of the three [`v1.0.0` gate](../releases.md#v100-gate) conditions.

### Model & diagnostics asks (2026-08-18)

All three plans in this group **landed 2026-08-19 and were archived**. The two AQL plans ([archive/2026-08-18-aql-structured-node-predicates.md](archive/2026-08-18-aql-structured-node-predicates.md), [archive/2026-08-18-aql-value-free-diagnostics.md](archive/2026-08-18-aql-value-free-diagnostics.md)) allocated no new requirement ids: each extended the landed requirement that already owned its surface ([REQ-113](../specifications/clinical-modeling.md#req-113--execution-oriented-parsed-aql-ast), [REQ-109](../specifications/clinical-modeling.md#req-109--aql-static-lint)), and PROBE-095 / PROBE-096 are implemented inline. The grounding story — four plan errors caught by checking the vendored grammar and the extractor before writing code, a fifth caught by a test, and the review round that closed the operand-level gap — is recorded in the archived plans and the [archive README](archive/README.md). The `rminfo` class-hierarchy plan landed as **REQ-048** (PROBE-094, STRAND-12/13) and is [archived](archive/2026-08-18-rminfo-class-hierarchy.md).

### Ecosystem fit-gap delivery (2026-07-16)

Prioritised from a peer-SDK ecosystem fit-gap review. Each plan authors its REQ spec prose (Phase 0, via `sdd-specify`) before implementation starts. The proposed REQ IDs follow the [numbering policy](../specifications/REQ.md#numbering-policy) topic bands — the two authoring validators land in the clinical-modeling headroom (110–119) next to REQ-109/110; the contribution builder opened the "SDK authoring & client tooling" band (130–139) because the wire band (050–059) is exhausted.

| Plan | Scope | Covers (proposed) | Probe |
|---|---|---|---|
| [2026-07-16-flat-author-linter.md](2026-07-16-flat-author-linter.md) | Pre-submit FLAT path linter + CLI | REQ-115 (builds on REQ-053/106/111) | PROBE-083 |
| [2026-07-16-opt-author-validator.md](2026-07-16-opt-author-validator.md) | OPT author validator + CLI | REQ-114 (builds on REQ-100/104/106/108) | PROBE-085 |

The contribution-builder plan **landed 2026-08-21 and was archived** ([archive/2026-07-16-contribution-builder.md](archive/2026-07-16-contribution-builder.md)): REQ-130 is `landed` — the first allocation in the SDK authoring & client-tooling band — and PROBE-084 is Implemented (Sandbox). `contribution.Builder` plus the `Creation` / `Amendment` / `Modification` / `Deletion` constructors assemble a `Contribution_create` body; the type parameter sits on those package-level constructors rather than on `Add*` methods (REQ-023; Go 1.27 allows generic methods, but four generic `Add*` methods would be a worse fluent surface). The plan's open question is settled from the vendored pin — `UpdateVersion` marks `lifecycle_state` required, so it is body-carried and REQ-059's per-request header was never a dependency — and the corpus refuted a derived batch `change_type` (10 of 47 records) and a change-type-derived lifecycle state (6 of 10 deletions carry `complete`). It also closed a wire defect the builder could not route around: the write-side wrappers no longer emit the server-assigned `contribution` / empty `uid` that the pin's `UpdateVersion` does not declare. **Deferred:** `IMPORTED_VERSION` authoring beyond the landed pass-through, multi-EHR batching, checkpoint/resume.

The fourth plan in this set — the upstream FLAT parity harness — **landed 2026-08-01 and was archived** ([archive/2026-07-16-web-template-tests-conformance.md](archive/2026-07-16-web-template-tests-conformance.md)): PROBE-086 is Implemented (inline), and REQ-080's roadmap row moved `planned → partial`.

The AQL honesty-residuals plan **landed 2026-08-18 and was archived** ([archive/2026-08-18-aql-honesty-residuals.md](archive/2026-08-18-aql-honesty-residuals.md)): implementation-aligned hardening of landed REQ-109 / 119 / 117 / 055 — lint `$param` HRID skip, MATCHES URI diagnostic redaction, `Bind` `$` strip, REAL overflow residual test, optional `aql_select_star` Warning + grammar-profile godoc. No new REQ; PROBE-028 / 087 / 090 unchanged in contract.

The AQL class-predicate splice plan **landed 2026-08-08 and was archived** ([archive/2026-08-08-aql-class-predicate-splice-and-source-text.md](archive/2026-08-08-aql-class-predicate-splice-and-source-text.md)): REQ-119 closes the § Out of scope deferral recorded for issue #99 — the class and VERSION bracket positions are guarded (the class one on bracket ESCAPE, the VERSION one on its whole `versionPredicate` production), and predicate and path text is read from the character stream so the round trip at those positions is IDENTITY rather than mere parseability. A full `nodePredicate` sub-grammar validator stays deferred for the class position; the regex whose token boundary depends on text after the predicate is CLOSED at the query level by REQ-119 § Emission verified after emission (issue #103).

The AQL read-side fidelity plan **landed 2026-08-05 and was archived** ([archive/2026-08-05-aql-top-carrier-literal-source-text.md](archive/2026-08-05-aql-top-carrier-literal-source-text.md)): REQ-118 is `landed`, the deprecated `SELECT TOP` clause parses/emits/builds instead of being refused, a projected literal carries its source text, and two new lint codes report the spec-forbidden `TOP` + `LIMIT` pairing. `TOP $n` stays unmodelled — the grammar admits no parameter there.

The AQL expressivity plan **landed 2026-08-04 and was archived** ([archive/2026-08-04-aql-expressivity-completion.md](archive/2026-08-04-aql-expressivity-completion.md)): REQ-117 is `landed`, PROBE-087 (structured-AST catalogue completeness) and PROBE-088 (builder containment + in-text paging stability) are Implemented, and the only surviving `aql.ErrIncompleteAST` class is an integer literal the AST cannot represent. A builder entry point for a FROM-root containment junction stays deferred.

### Simplified formats — WebTemplate + FLAT/STRUCTURED (planned umbrella)

| Plan | Scope | Covers REQs / probes |
|---|---|---|
| [2026-06-23-simplified-formats.md](2026-06-23-simplified-formats.md) | Umbrella — FLAT/STRUCTURED codecs (WebTemplate export Phase 2 **landed**; Phase 3 codecs **landed**); carries the shared simplified-template model and what is left of the REQ-053 edge deferrals. The datatype-coverage residue went to the [coverage ratchet](archive/2026-08-03-flat-coverage-ratchet.md) (landed) and the three residuals it named to the [closure plan](archive/2026-08-05-flat-rm-attributes.md) (landed) — the umbrella's § Residual scope is the ledger of the five items still open (`.schema` media types, reused-sibling FLAT, the deferred underscore families, the ITS `ctx/` sketches, the WebTemplate-projection gaps) | REQ-053; PROBE-076 |

The FLAT residual closure **landed 2026-08-05 and was archived** ([archive/2026-08-05-flat-rm-attributes.md](archive/2026-08-05-flat-rm-attributes.md)): REQ-140 is `landed` and opened the wire-extension band 140–149, PROBE-089 is Implemented (inline), and PROBE-086 coverage moved 19.5% → **80.4%** of the upstream corpus. The DV_TEXT substitution carve-out + `ctx/setting` emission went in PR #88, the underscore-prefixed RM attribute grammar and the DV_MULTIMEDIA / DV_PARSABLE / DV_INTERVAL / ENTRY-`subject` leaf closures in PR #89 ([ADR 0016](../adr/0016-event-context-optionals-underscore-spelling.md)). Its 52-key residue is the umbrella's § Residual scope above.

WebTemplate JSON export (**REQ-106**) landed as a direct slice and was archived ([archive/2026-05-22-webtemplate-export.md](archive/2026-05-22-webtemplate-export.md)); the umbrella's shared simplified-template model is deferred until REQ-053 (FLAT/STRUCTURED) gives it a second consumer.

Phase 3 — FLAT / STRUCTURED composition codecs (`openehr/serialize/simplified`, REQ-053) — **landed and was archived** ([archive/2026-07-14-flat-structured-codecs.md](archive/2026-07-14-flat-structured-codecs.md)): bidirectional codecs, `ctx/`, `|raw`/`|other`, full datatype set, PROBE-076, and `WithTemplate` name + RM-mandatory completion so a decoded composition validates against the OPT. Residual edge deferrals are tracked by the umbrella above + the package [`deviations.md`](../../openehr/serialize/simplified/deviations.md) — the EVENT_CONTEXT optionals left that list on 2026-08-05 by riding REQ-140's underscore grammar at their real paths ([ADR 0016](../adr/0016-event-context-optionals-underscore-spelling.md)) rather than gaining `ctx/` short forms, leaving `.schema` media types on input as the oldest survivor; the upstream byte-conformance probe landed as PROBE-086, and its coverage ratchet is archived at [2026-08-03-flat-coverage-ratchet.md](archive/2026-08-03-flat-coverage-ratchet.md).

The Phase 2 clinical-building-blocks umbrella **landed and was archived** ([archive/2026-05-21-phase-2-clinical-building-blocks.md](archive/2026-05-21-phase-2-clinical-building-blocks.md)); its remaining deferred scope (simplified formats) is now sequenced by the umbrella above. The two AQL plans also landed and moved to `archive/` — AQL builders ([REQ-055](archive/2026-05-21-aql-builders.md)) and AQL parse + lint ([REQ-109](archive/2026-06-15-aql-lint.md)).

**Landed (archived):** OPT parser, REQ-100 follow-ups (Phases 1–8), composition validation (REQ-102), composition builder (REQ-101), template-driven instance generator (REQ-107), REQ-104 slot assertions + REQ-105 terminology bindings (PR #43), C_PRIMITIVE_OBJECT wire parser + REQ-107 UID emission, BMM codegen, canonical JSON/XML, AQL builders (REQ-055), AQL static lint (REQ-109), validation beyond COMPOSITION (REQ-110 — demographic PARTY hierarchy + FOLDER / EHR_STATUS), the public compiled-template bridge (REQ-111, ADR 0010), the SMART-on-openEHR auth conformance audit (REQ-061..064/068, REQ-070..072; ADR 0008/0009; STRAND-05), polymorphic `_type` round-trip stability (REQ-052/040/102/107), seeded synthetic value generation (value-fill + seed; REQ-103/107/101), template-less RM validation floor (REQ-112), stored-query / query REST conformance (REQ-055/057), the execution-oriented parsed AQL AST (REQ-113), the modernize & simplify sweep (client write-plumbing dedup REQ-094, generated LOCATABLE identity surface — ADR 0013, REQ-031/040/043), the path-parameter single-encoding fix + class-predicate `ParsedPath` (REQ-095/113, PR #74), the WebTemplate JSON export (REQ-106, ADR-0014; EHRbase v2.3 structural parity via PROBE-075), template-level node naming with name-predicated paths (REQ-116; archetype-reuse templates now export, PROBE-075 parity extended to both vendored oracles, PROBE-086 unblocked), the upstream FLAT parity harness (REQ-080/PROBE-086 — the pinned EHRbase corpus round-trips through the REQ-053 codec, exact on the modelled subset and counting the rest; it caught a silent encode-side data loss, fixed in `rmpath` under REQ-121), the PROBE-086 coverage ratchet (REQ-053/121 — CODE_PHRASE leaves, the optional datatype suffixes, and the metadata-spelling decision [ADR 0015](../adr/0015-flat-metadata-spelling.md); 10.5% → 19.5% of the upstream corpus compared exactly), and the AQL expressivity completion (REQ-117/PROBE-087/088 — structured-AST catalogue closed to the whole grammar profile, two lint over-rejections accepted, builder containment algebra + opt-in in-text paging) — see [archive/](archive/README.md). The umbrella validation scope first sketched in the archived [umbrella validation plan](archive/2026-05-21-validation.md) is now complete.

## Header convention (load-bearing)

Every plan MUST start with the fields in [`_template.md`](_template.md):

- **`Covers:`** — REQ-NNN / PROBE-NNN / STRAND-NN identifiers
- **`Status:`** / **`Implementation:`** — mirror [`../specifications/README.md`](../specifications/README.md#status-header)
- **`Depends on:`** / **`Defers:`** — explicit scope boundaries

Naming: `YYYY-MM-DD-<short-title>.md`. New plans: copy [`_template.md`](_template.md). When a plan lands, move it to [`archive/`](archive/README.md) and update [`../specifications/traceability.yaml`](../specifications/traceability.yaml).
