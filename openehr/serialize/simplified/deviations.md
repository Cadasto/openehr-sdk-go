# Simplified Formats — documented deviations & deferrals (REQ-053)

Parity with the openEHR *Simplified Formats* (ITS-REST, STABLE) is **structural, not
byte-exact**. This file records where `openehr/serialize/simplified` intentionally
deviates from, or has not yet implemented, part of the spec. Each entry says what the
current behaviour is and where the full behaviour lands.

Status legend: **Deviation** = deliberate, permanent-ish choice; **Deferred** = not yet
implemented; each is recorded in the rows below, and the refusals the upstream corpus
meets are counted in [SKIPPED.md](../../../testkit/conformance/webtemplate/SKIPPED.md).
This package landed in [PR 76](https://github.com/Cadasto/openehr-sdk-go/pull/76).

## Fail-loud posture and unmatched content

The codec never succeeds while silently losing or altering data the Web Template
models (REQ-053 is semantics-preserving), with four registered narrowings on the
**encode** leg and no others: the folded structural wrappers (§ below),
`archetype_details` (§ Deviations below), an ACTION transition's `transition` and
`reason` (the ISM transition note below), and an empty repeat instance (§ below). The
first three are the attribute classes `wire.md` § REQ-140 names as deliberately out of
scope; the fourth is a FLAT structural limit, not an attribute. The composer's
`external_ref` and `identifiers` were a fifth narrowing until
[ADR 0029](../../../docs/adr/0029-composer-real-path-carrier.md) made the composer's real
path their carrier (§ `ctx/` context row below). Instance data the template does not model
at all is outside the claim: encode walks the Web Template, so it never reaches that data.
Decode holds the same line from the other side (REQ-170): a key the codec does not read
is **unmatched content**, not modelled data, so it is ignored by default and reported or
refused on request (`WithUnmatched`, `WithStrict`), while a key it reads but cannot carry,
and malformed content, stay errors in every mode. Nothing the codec reads is dropped or
altered without an error.
Concretely:

- **Encode** — a **clinical** datatype (`DV_*`) is emitted as its FLAT suffix form only
  when that form fully captures the value; a **decorated** value (carrying `normal_range`,
  `other_reference_ranges`, `mappings`, a non-`openehr` `normal_status`, … — anything outside
  the datatype's captured keys) and any datatype outside the core set are embedded as a
  lossless `|raw` canonical
  fragment rather than partially/silently dropped. A `DV_*` leaf the Web Template gives no
  input descriptors for (e.g. `DV_URI`, `DV_MULTIMEDIA`, `DV_PARSABLE`) is still emitted (as
  bare/suffixed or `|raw`), not skipped — including the in-context `ACTIVITY.timing`
  (`DV_PARSABLE`), which rides its **suffix form** since REQ-140 Phase C3 and rode `|raw` before
  that (it became resolvable with REQ-121). A **party** leaf (the ENTRY `subject`) decomposes
  through the REQ-140 party grammar and a **DV_INTERVAL** leaf through its interval grammar; the
  **STRING** leaf (ACTIVITY `action_archetype_id`, the one RM String attribute the Web Template
  gives a leaf) rides its **bare value**, per the REQ-053 *Leaf datatypes* row. A leaf's RM type is
  read through one normaliser (`nodeRMType`), so a Web Template spelling it with white space
  (`" DV_INTERVAL<DV_QUANTITY>"`, `"DV_QUANTITY "`) classifies exactly like the clean spelling. A
  childless node whose RM type the codec cannot spell (a container left empty, an unmapped or
  malformed type name) is skipped only while the composition holds nothing there: a populated
  value is `ErrUnsupportedDatatype` naming the node's FLAT path and RM type, never the value
  (REQ-140). "Nothing there" includes the zero value `rmpath` hands back for an attribute held by
  value that the composition never set (an unset ENTRY `language`, ACTION `ism_transition`, …),
  the same reading an empty `CODE_PHRASE` and an all-zero `ctx/setting` already get. The backstop
  sees only what `rmpath` resolves, so it relies on `rmpath` resolving every node the Web
  Template builder emits; `webtemplate.TestWebTemplatePathsResolveViaRmpath` enforces that for
  every vendored OPT rooted at a COMPOSITION, with the `ctx/`-owned paths and the derived EVENT
  `offset` as its only reasoned exemptions. An ACTION's **ISM transition** round-trips through the
  Web Template's ISM_TRANSITION nodes (one per careflow step, `ism_transition[at0005]`, …): an
  ISM_TRANSITION carries no `archetype_node_id`, so `rmpath` matches the predicate against its
  `careflow_step` code in the `local` terminology, which ADL 1.4 codes with the same node id. A transition whose careflow
  step names none of the template's nodes is instance data the template does not model, and like
  any such data it is not encoded. Only a matched transition's `current_state` and
  `careflow_step` travel: the builder models no `transition` leaf and no `_reason` family on those
  nodes, so a populated `transition` or `reason` is dropped on encode (the third narrowing above),
  and decode refuses the reference's in-context spelling of an ACTION's ISM transition and time in
  every mode (REQ-170, since 2026-10-12), wherever the Web Template does not resolve it: a
  `current_state`, `transition` or `careflow_step` leaf, with any suffix, or a `_reason` family,
  under an ISM transition node — the reference's in-context `ism_transition` node, which this SDK's
  Web Template does not project, or an archetyped `ism_transition[atNNNN]` node, which models
  `current_state` and `careflow_step` (those decode) but never `transition` or `_reason` — and the
  ACTION's own `time` leaf where the ACTION has no node for it. Each names RM data the decode
  would otherwise drop, or for `time` replace with the `ctx/time` default a `WithTemplate` decode
  fills in, so the refusal (`ErrUnknownPath`) names the key, or the `_reason` family instance, and
  carries no value. The refusal of a `transition` leaf or a `_reason` family cites REQ-140, whose
  third out-of-scope class covers them; the refusal of a `current_state` or `careflow_step` leaf
  and of the ACTION's `time` cites REQ-170. On the same ground decode refuses an INTERVAL_EVENT's `math_function`, `width`
  and `sample_count` and an OBSERVATION's `history_origin` in the reference's spelling, a leaf
  under the event or observation node (with any suffix) or a suffix on it, wherever the Web
  Template does not resolve them (the corpus writes `any_event:0/math_function|code`,
  `any_event:0/width`, `any_event:0|sample_count` and `<observation>/history_origin`). A Web
  Template that models one decodes it as before. Any other key under those nodes that the Web
  Template does not resolve is unmatched content. The composition-level metadata leaves (`language`, `territory`, `composer`,
  `context/start_time`, `context/setting`) are held back from the node walk **by name**
  (`ctxOnlyLeafPaths`), because the context emission writes them: `language`, `territory`,
  `context/start_time` and `context/setting` in their `ctx/`-only spelling (ADR 0015), the
  composer in its `ctx/` short form or at its real path ([ADR 0029](../../../docs/adr/0029-composer-real-path-carrier.md)), whether or not the
  Web Template carries a composer node. Writing one again from the walk would spell one value
  twice. A container node that resolves to a
  non-`Locatable` RM object (e.g. `EVENT_CONTEXT`) is recursed via the enclosing Locatable
  ancestor, not dropped. A typed-nil RM pointer is treated as an absent leaf (skipped).
  An unset `DV_CODED_TEXT` (the Go zero value: no value, code, terminology or formatting) writes no
  keys, as an absent leaf does.
  A **composer** one `ctx/` short form carries whole is written in that form:
  `ctx/composer_self` for a `PARTY_SELF` with no `external_ref`, `ctx/composer_name` for a
  `PARTY_IDENTIFIED` with a name and nothing else. Every other composer is written whole at
  its real path `<root>/composer` in the REQ-140 party grammar, with no `ctx/` composer key,
  so a body spells its composer one way ([ADR 0029](../../../docs/adr/0029-composer-real-path-carrier.md)): a `PARTY_IDENTIFIED` with a
  reference or identifiers (its name as `<root>/composer\|name`), a `PARTY_RELATED` (with
  `/relationship`), and a `PARTY_SELF` with an `external_ref` (`<root>/composer_self: true`
  beside the reference's keys). A `PARTY_IDENTIFIED` with an empty `name`, or with no `name`,
  `external_ref` or `identifiers`, is `ErrUnsupportedDatatype` naming the composer, not an
  omission: written as nothing it would read back as `PARTY_SELF`, a silent type
  substitution.
- **Decode** — input the decode does not read is **unmatched content** (REQ-170): a key
  that does not resolve to a Web Template node, an underscore segment that names no RM
  attribute this codec carries at that node (an unknown family, or one the owner's RM class
  does not declare), a suffix outside the suffix set of its datatype or family (e.g. a
  `\|unitt` typo, the derived bare magnitude of a DV_PROPORTION), and a `ctx/` field the codec
  does not read. STRUCTURED applies one rule at every level of the body: a member whose name
  the FLAT grammar does not read at that level is unmatched whatever the JSON shape of its value,
  and is reported as that member, while a name the grammar reads keeps the shape refusals. The
  levels are the top level, the ctx fields (a ctx field reads its suffixes only), the Web
  Template's nodes and leaves, a leaf's sub-paths (an interval's `lower` and `upper`, a party's
  `relationship` and `_identifier`), the underscore families an owner carries and the grammar
  inside each, and everything below those. The composer's party sub-structure (`_identifier` and
  `relationship`) is read whether or not the Web Template carries a composer node, in
  STRUCTURED as in FLAT, and a member below it that the party grammar does not read is
  unmatched ([ADR 0029](../../../docs/adr/0029-composer-real-path-carrier.md)). A name that spells more than one FLAT piece (a `:`,
  `\|` or `/` in it) is judged by its base, the id before the first of them: where the base
  reads nothing the member is unmatched; where it reads something the member is flattened as
  written, the FLAT decode judges the key it spells, and a value that is not an array is
  refused. It is **ignored by
  default**: the decode carries on, and the value holds what the input says without it, exactly as
  if the input had left those keys out (structural checks such as a dense `:index` sequence
  included). It never reaches the value anywhere.
  [`WithUnmatched`](simplified.go) reports each item, in document order, by its path (the key
  as written for FLAT, the slash-joined member path for STRUCTURED), and
  [`WithStrict`](simplified.go) refuses the first with an error that wraps `ErrUnknownPath`,
  names the path and carries no value. Before REQ-170 every such key was refused, and a body
  carrying a producer's extension key could not be read at all.

  A key the decode **reads** is never unmatched, and it stays an error in every mode where it
  cannot be honoured: the keys that resolve but have no carrier (PARTICIPATION `\|time`, FEEDER_AUDIT_DETAILS
  `other_details`, `_instruction_details` at an ACTION and `_wf_definition` at an INSTRUCTION —
  the REQ-140 deliberate exclusions), the keys the decode recognises as the reference's spelling
  of RM data this SDK's Web Template does not model, resolved or not (an ACTION's ISM transition
  and time, an INTERVAL_EVENT's `math_function`, `width` and `sample_count` and an OBSERVATION's
  `history_origin` in the reference's in-context spelling, the ISM transition note above), the
  bare value of a leaf whose
  datatype has neither a suffix set nor a sub-path grammar (an unmapped datatype such as
  `DV_STATE`) and a suffix naming an attribute of its RM class (any other suffix there is
  unmatched, and so is
  every key but `\|raw` at an unparameterised `DV_INTERVAL` leaf, whose interval grammar has no
  bound datatype to read with),
  a suffix of `language`, `territory`, `context/setting` or `context/start_time` that the leaf's
  datatype has but its `ctx/` form cannot carry (`ErrUnsupportedDatatype` naming the key and the
  `ctx/` form), and malformed matched content. That is a misused `\|raw`/`\|other` or a `\|other`
  on a closed value-set (`ErrUnsupportedDatatype`), a missing **required** suffix (an error, not a
  coerced zero value; a `null` one beside a present key of its leaf among them, since a null
  reads as absent, so `\|magnitude: null` beside a unit is a quantity without a magnitude, never
  one of 0; a bare value alone at a leaf whose datatype has no bare spelling, such as a
  DV_CODED_TEXT, a DV_QUANTITY or a DV_IDENTIFIER, and a bare `ctx/setting` are such a value
  missing every required suffix, while a DV_PROPORTION's derived bare magnitude beside its
  numerator is unmatched, and alone where every other key of its leaf is null), a value that
  does not fit its attribute (one the canonical-JSON decode of its RM attribute would refuse,
  [ADR 0004](../../../docs/adr/0004-numeric-wire-tolerance.md) tolerance included: a string
  that is no decimal where the attribute is a number, a number or a Boolean where it is a
  `String`, a number with a non-zero fraction or outside the int32 range where it is an
  `Integer`; refused with `ErrUnsupportedDatatype` naming the FLAT key, required and optional
  suffixes and the bare value alike, so it never surfaces as a canonical-JSON error naming a path
  the caller did not write; the key named is the key as written, except that a bare value
  spelled `x\|` is named `x`, the key it spells, and a STRUCTURED value is named by the FLAT key
  its conversion makes, with an `:index` on every segment below the template root; a `\|code`
  at a DV_TEXT leaf that is present and not `""` selects the DV_CODED_TEXT whatever its JSON
  kind, so one that is not a string is refused this way, naming the `\|code` key; the check
  decodes the value with canonical JSON's own decoder for the attribute's RM type, so it admits
  everything canonical JSON admits, a quoted decimal and a whole number written `1.0` included),
  a bare value at a key that resolves to a
  node with children (`ErrUnsupportedDatatype` naming the node's RM type, while a suffix such a
  node does not read is unmatched), two spellings of one field the decode
  reads that disagree (including a key and the same key ending in a bare `\|`, the empty suffix
  that spells the bare value: refused with `ErrUnknownPath` unless their values are equal, when
  the value is read once; a lone `x\|` reads as `x`; two spellings of a key the decode does not
  read are two unmatched keys, each reported as written, whatever their values; inside a
  `_feeder_audit` the index-less spelling of a sub-path segment and its `:0` name one slot and
  merge, under the same rule for a key spelled under both),
  a key that walks
  through one of several reused siblings, and an out-of-bound, over-budget or sparse `:index`.
  Trailing JSON after the object is rejected too. A key is judged segment by segment from the
  left, so an index past the bound of a segment that resolves makes the key malformed whatever
  follows it. An `:index` past `:0` on the template root's own segment (a key that is that
  segment alone, `<root>:1\|x_note`, included), on a segment of the
  Web Template path that addresses a single-valued RM attribute (`<root>/context:1/…`, an
  ACTION's `ism_transition` node, an ENTRY's `language`, a party leaf, an interval leaf on a
  single-valued attribute), or on a composition-metadata segment, whether or not the Web
  Template carries its node, is an index past its bound (REQ-170): `ErrUnknownPath` naming the
  key, never merged into the one value. STRUCTURED judges an element's position in its member's
  array the same way, before what the element holds: an element past the bound of a
  single-valued node, metadata segment, underscore family, FEEDER_AUDIT sub-path, or sub-path
  inside a family or a composite leaf's grammar is refused even when every member it holds is
  unmatched content. An interval leaf's index belongs to the ELEMENT the Web
  Template folds away, judged on the RM attribute that holds it as any other leaf's, so a
  repeating interval ELEMENT takes `:N` and round-trips.
  The same index on a sub-path inside an underscore family's value or an interval value
  (`…/interval_dv_quantity/lower:1\|magnitude`, `…/dv_count/_normal_range/lower:1`) is refused
  with `ErrUnsupportedDatatype` instead (`rmattrNotAList`), whatever follows it
  (`…/_normal_range/lower:1/x_unknown` too), and an index past the repeat bound on a party's
  `_identifier` list with `ErrUnknownPath`; on a single-valued `_family`
  (`…/_uid:1`) and on a FEEDER_AUDIT sub-path (`…/_feeder_audit/originating_system_audit:1\|system_id`)
  it is `ErrUnknownPath`. Every one of these names the key, or the path up to the offending
  segment, and carries no value (`TestREQ170IndexPastBoundSentinels`).
  An ISM transition rebuilt at an `ism_transition[atNNNN]` node without a
  careflow step gets the one the node names (the node id as a `local` code, the node's Web
  Template name as its value); a careflow step that names no terminology (none, null or blank)
  takes `local`, and one coded otherwise (another code, another terminology, or a
  `terminology_id` of another shape) is `ErrUnsupportedDatatype`.
  Keys that reach two Web Template nodes standing for one single-valued RM attribute (two
  transition nodes of one ACTION) are `ErrUnknownPath`, never merged into one object.

  A JSON `null` held by a key the decode reads is **absent** (REQ-053), never the zero of its
  type: a leaf, an interval bound or a `_` family instance whose keys are all null is absent;
  a null required suffix beside a present key of its leaf is missing; a null `ctx/` field is
  absent, so a null `ctx/language` is `ErrMissingContext`; a null key that resolves but has no
  carrier (PARTICIPATION `\|time`, FEEDER_AUDIT_DETAILS `other_details`, a deferred family, a
  ctx/-owned leaf spelling, the reference's in-context
  spelling of RM data the Web Template does not model) carries nothing to drop and is not
  refused; and a null spelling of a field beside another spelling (`x\|` beside `x`, a
  sub-path with and without `:0`) is no disagreement. A null key at the composer's real path is
  absent like a null key at any other party position (ADR 0029). A key the decode does
  **not** read is unmatched content whatever its value, null included. In STRUCTURED a null
  member or array element is carried into FLAT as a key holding null, read the same way, and
  OPT-free STRUCTURED → FLAT conversion writes it so (`{"leaf":[null]}` becomes
  `leaf:0: null`); a null template root or `ctx` object, which no FLAT key can hold, is absent.
  FLAT → STRUCTURED carries a null as written as well: a lone null bare value as a null array
  element, a null suffix as a null `\|suffix` member, and a null bare value beside its leaf's
  suffixes or children as the null `"\|"` member, as it carries one that is not null. A null
  `ctx/` field beside its suffixes (`ctx/setting: null` beside `ctx/setting\|code`) takes the
  null `"\|"` member of the field's object the same way, and STRUCTURED → FLAT reads that
  member back as the field's own key, as it does at a leaf, a member that holds a value
  included (`{"\|": "x"}` gives `ctx/setting: "x"`, where it gave `ctx/setting\|` before, a
  spelling decode reads alike). A `ctx/` field holding a value beside its suffixes takes the
  `"\|"` member the same way (`ctx/setting: "x"` beside `ctx/setting\|code` gives
  `{"\|": "x", "\|code": …}`), so OPT-free conversion carries it both ways (REQ-140). A
  `ctx/` field whose value is a JSON object, the shape its suffixes take, is refused by
  FLAT → STRUCTURED rather than merged with them.
  Before 2026-10-15 a null suffix decoded to the zero of its type, a null STRUCTURED element
  was refused as a hole in the array, and FLAT → STRUCTURED refused a `ctx/` field beside
  its suffixes as a direct value beside a suffixed leaf, null or not, echoing the value, and
  merged an object-valued one with its suffixes, the later key overwriting the earlier.

Consequence: a payload that spells a not-yet-supported feature (below) in keys the codec does
not read decodes without those keys by default, and the caller learns which they were through
`WithUnmatched`, or has the payload refused through `WithStrict`; a payload that spells one in
keys the codec reads but cannot carry is **rejected** in every mode. Nothing the codec reads is
partially or silently accepted.

## Deferred features

| Feature | Current behaviour | Lands in |
|---|---|---|
| `ctx/` context — **core supported**: `ctx/language`, `ctx/territory` (both mandatory on decode → `ErrMissingContext`), `ctx/composer_name` / `ctx/composer_self`, `ctx/time` (context `start_time`), `ctx/setting\|code` + `ctx/setting\|value` (context `setting`; the `openehr` terminology is implied, not written; a `ctx/setting\|terminology` witness, like the real-path `context/setting\|terminology`, is dropped when it names `openehr` and refused with `ErrUnsupportedDatatype` when it names anything else). | Emitted on encode; rebuilt on decode. The **all-zero** setting writes nothing (non-pointer field — "unset" and "zero" coincide, the CODE_PHRASE precedent); a **populated** setting the pair cannot carry — a non-`openehr` (or empty) terminology, extras beyond code+value (`mappings`, `formatting`, a `preferred_term`, …), or a value without a code — is `ErrUnsupportedDatatype` naming `ctx/setting`, not an omission (omitting it would let a `WithTemplate` decode substitute the `238 other care` default silently, the stance encode takes on a composer it cannot write). On decode one half of the pair without the other is refused naming the missing key. | landed (Task 6; `setting` 2026-08-05, REQ-053 amended) |
| Composition metadata in the reference's **real-path** spelling (`<root>/language\|code`, `<root>/territory\|code`, `<root>/composer\|name`, `<root>/composer_self`, `<root>/context/start_time`) | **Accepted on decode**, normalised onto the `ctx/` short forms ([ADR 0015](../../../docs/adr/0015-flat-metadata-spelling.md)); **never emitted** — `ctx/` is the only output spelling, except for a composer no `ctx/` short form carries whole, which is written whole at its real path (§ Fail-loud posture, [ADR 0029](../../../docs/adr/0029-composer-real-path-carrier.md)). A `\|terminology` witness is checked against the terminology the `ctx/` form implies and then discarded; a mismatch, or two spellings of one field that disagree, is an error rather than a precedence rule — including a composite (object/array) value, which no `ctx/` field can hold and which is refused before it can be compared. `composer_self: true` beside a composer **name** (either spelling) is refused on the same grounds: they are mutually exclusive representations of one RM attribute, and the `PARTY_SELF` branch would silently drop the name (`composer_self: false` beside a name is fine). Respellings only: `context/setting\|code` / `\|value` **are** aliases since 2026-08-05 (the amended REQ-053 closed the `ctx/setting` emission gap ADR 0015 had left open), with `context/setting\|terminology` as an `openehr` witness; the composer's other real-path keys (`\|id`, `\|id_scheme`, `\|id_namespace`, `\|type`, `_identifier:N`, `/relationship`) are no aliases: no `ctx/` field carries them, and they carry the rest of the composer at its real path, merged with the name either spelling gives (ADR 0029). `composer_self: true` beside them reads as a `PARTY_SELF` carrying the `external_ref`, and beside an identifier or a relationship is refused as it is beside a name. The same real path with `:0` on its segments, the template root's own segment included (`<root>/language:0\|code`, `<root>:0/composer:0\|name`, `<root>/context:0/start_time:0`, the spelling OPT-free STRUCTURED → FLAT conversion writes), is read exactly as the unindexed one, under the same disagreement and witness rules and the same refusals; any other index on those single-valued segments (`<root>/language:1\|code`, `<root>:1/…`) is refused naming the key (`ErrUnknownPath`). Until 2026-10-11 the indexed form missed the alias table: `language:0\|code`, `territory:0\|code`, `context:0/start_time:0` and `context:0/setting:0\|code` + `\|value` decoded as ordinary leaves and were then overwritten by a `ctx/` value with no error, and the indexed composer spellings were refused. | landed (ADR 0015; `setting` alias 2026-08-05; indexed spelling 2026-10-11) |
| `ctx/` context — **rest deferred**: the `ctx/` short forms for participations, `health_care_facility`, `work_flow_id`, composer `external_ref` (`composer_id` / `id_namespace` / `id_scheme`), `end_time`, `location`. | Not emitted on encode; any such `ctx/*` key is unmatched content on decode (REQ-170): ignored by default, reported by `WithUnmatched`, refused with `ErrUnknownPath` under `WithStrict`. By default such a key is therefore dropped silently: a `ctx/composer_id`, `ctx/id_namespace` or `ctx/id_scheme` decodes to a composer without its `external_ref`, and only `WithUnmatched` or `WithStrict` tells the caller. All are optional, so nothing is silently substituted on decode. **The `ctx/` *spelling* is what is deferred here, not always the data**: since 2026-08-05 (REQ-140 Phase C0, [ADR 0016](../../../docs/adr/0016-event-context-optionals-underscore-spelling.md)) `end_time`, `location` and ENTRY `work_flow_id` round-trip under the **underscore grammar** at their real paths (`<root>/context/_end_time`, `<root>/context/_location`, `<entry>/_work_flow_id\|*` — § underscore-prefixed RM attributes below), so no source value is dropped for those three; ADR 0016 decision 3 declines the `ctx/` sketches for them deliberately. Since 2026-08-05 (Phase C2) `participations` and `health_care_facility` round-trip under the underscore grammar too (`<root>/context/_participation:N\|*`, `<root>/context/_health_care_facility\|*`), and since 2026-10-15 ([ADR 0029](../../../docs/adr/0029-composer-real-path-carrier.md)) the composer's `external_ref` round-trips at the composer's real path (`<root>/composer\|id`, `\|id_scheme`, `\|id_namespace`, `\|type`), beside its `_identifier:N` and a PARTY_RELATED's `/relationship`, so no source value is dropped in this row: only the `ctx/` spelling of each stays deferred. Until then the composer's `external_ref` and `identifiers` were dropped on encode, which wrote `ctx/composer_name` alone, and decode refused the reference's keys for them. Note the fields that do NOT belong here: `category` is a template-constrained Web Template leaf and round-trips via its own path; a composer **name** round-trips via `ctx/composer_name`, or via `<root>/composer\|name` beside the rest of a composer written at its real path; `setting` left this row on 2026-08-05 (§ core supported above); template-constrained `other_context` content rides its Web Template paths, and does so end-to-end since `rmpath` resolves `EVENT_CONTEXT.other_context` (REQ-121) — before that the paths existed in the Web Template but the encoder dropped the data. | Deferred |
| Datatypes — **first-class** suffix form: `DV_TEXT`, `DV_CODED_TEXT`, `DV_DATE_TIME`, `DV_DATE`, `DV_TIME`, `DV_QUANTITY`, `DV_COUNT`, `DV_BOOLEAN`, `DV_DURATION`, `DV_URI`, `DV_EHR_URI`, `DV_ORDINAL`, `DV_SCALE`, `DV_PROPORTION`, `DV_IDENTIFIER`, `DV_PARSABLE`, `DV_MULTIMEDIA` (§ encapsulated leaves below), and `DV_INTERVAL<T>` (not a suffix set at all — the REQ-140 interval grammar). Any other `DV_*`, a decorated instance of the above, or a **substituted subtype** (the value's dynamic type differs from the leaf type, e.g. `DV_EHR_URI` at a `DV_URI` leaf), rides `\|raw` — with **one carve-out** (REQ-053, amended 2026-08-05): a **fully-captured `DV_CODED_TEXT` at a `DV_TEXT`-typed leaf** rides the DV_CODED_TEXT suffix set (`\|code` + `\|value` + `\|terminology`, `\|formatting` where populated; **no bare key**), matching the reference's `dv_coded_text_as_dv_text` corpus shape; on decode, `\|code` at a `DV_TEXT` leaf re-selects the DV_CODED_TEXT builder under that type's own rules (`\|value` required). A *decorated* coded text whose extras the suffixes cannot carry (`mappings`, a `preferred_term`, …) still rides `\|raw` stamped `DV_CODED_TEXT`, and every **other** substituted subtype keeps riding `\|raw`. This changed emitted bytes for the fully-captured substituted value only (previously a `\|raw` fragment). | Both directions. | landed (Task 6; substitution carve-out 2026-08-05) |
| Optional `DV_ORDERED` / `DV_QUANTIFIED` / `DV_AMOUNT` attributes — **first-class** suffix form: `\|magnitude_status`, `\|normal_status`, `\|accuracy`, `\|accuracy_is_percent`, `\|precision`, `\|units_system`, `\|units_display_name`, and `DV_TEXT`'s `\|formatting`. | Supported both directions. **This changed emitted bytes:** a value carrying one of these previously rode `\|raw` as a whole and now rides the suffix form. An *undecorated* value is byte-identical to before — an absent attribute writes no suffix. `\|normal_status` carries the bare ordinal code and decode rebuilds it in the implied `openehr` terminology, so a status coded elsewhere still rides `\|raw`. On decode each value is checked against its RM attribute (REQ-053 *A value must fit its attribute*: `\|accuracy` a `Real`, `\|accuracy_is_percent` a `Boolean`, `\|precision` an `Integer`, …) so a malformed value is refused naming the offending FLAT key rather than escaping as a canonical-path error from `canjson`; the refusal wraps `ErrUnsupportedDatatype`, as every value that does not fit its attribute does. The check admits everything `canjson` admits and so adds no strictness: a **quoted** number (`\|accuracy: "0.5"`) still decodes, since `canjson` parses a numeric string into `Real` / `Integer`, and so does a whole number written with a fraction where the attribute is an `Integer` (`\|precision: 2.0`), while `\|precision: 1.5` is refused. Before 2026-10-15 the check was by JSON kind alone, carried no sentinel, and let `\|precision: 1.5` through to fail in `canjson` naming no key. The RM's own value constraints (`\|magnitude_status` against its code set, …) are left to the validation package. | landed (PROBE-086 ratchet) |
| Underscore-prefixed RM attributes ([REQ-140](../../../docs/specifications/wire.md#req-140--underscore-prefixed-rm-attributes)) — **landed**: `_uid` (any LOCATABLE the Web Template **models** — the composition root, an ENTRY, a SECTION, a CLUSTER and a collapsed `ELEMENT` leaf; see § folded structural wrappers below for the ones it does not), `_name` (the composition's own name, at the template root only, in the DV_TEXT leaf grammar; § Deviations, `LOCATABLE.name` on decode), `_link:N` (`\|meaning` + `\|type` + `\|target`), `_work_flow_id` / `_guideline_id` (OBJECT_REF: `\|id`, `\|id_scheme`, `\|namespace`, `\|type`), `context/_end_time`, `context/_location`; and the **value-decoration** families on a collapsed `ELEMENT` leaf — `_normal_range` (DV_INTERVAL of the leaf's anchor type: `/lower` + `/upper` in the anchor's own suffix form, `\|lower_included` / `\|upper_included` / `\|lower_unbounded` / `\|upper_unbounded`), `_other_reference_ranges:N` (REFERENCE_RANGE — that interval grammar with the RM's `range` level **elided**, plus `/meaning` as DV_TEXT or DV_CODED_TEXT), `_mapping:N` (TERM_MAPPING — `\|match`, `/target` CODE_PHRASE, optional `/purpose` DV_CODED_TEXT), `_null_flavour` and `_null_reason`. | Both directions, byte-exact, with one exception: a `<root>/_name` holding the plain name a body without it decodes to (the Web Template root's name, or the template id where the root has none) re-encodes with no `_name` key. **This changed emitted bytes**: a composition carrying a populated `uid`, `links`, `workflow_id`, `guideline_id`, `end_time` or `location`, or a name other than the plain Web Template root name, now emits those keys where it previously emitted nothing at all. It is new coverage rather than a `\|raw` narrowing — none of these attributes ever had a FLAT spelling here — but any consumer diffing emitted FLAT will see the new keys (notably `<path>/_uid`, which `instance.Generate` stamps one on the composition root and on each ENTRY — `stampsUID` in `openehr/instance/locatable.go`, not on every archetyped node; and `<root>/_name: "example"`, since `instance.Generate` names every composition `example`). Three subtype policies are load-bearing, and all three refuse rather than retype: (a) `_uid` is one bare string, so the concrete `UID_BASED_ID` is re-derived from the lexical form — three `::` parts with a valid `VERSION_TREE_ID` is `OBJECT_VERSION_ID`, anything else `HIER_OBJECT_ID` — and a value whose form implies the *other* subtype is `ErrUnsupportedDatatype`; (b) an OBJECT_REF's `\|id_scheme` is the `OBJECT_ID` discriminator (present ⇒ `GENERIC_ID`, absent ⇒ `HIER_OBJECT_ID`), every other subtype being scheme-less and therefore indistinguishable on the wire; (c) `\|meaning` / `\|type` on a LINK and the bare `context/_end_time` carry a plain undecorated value only, and these families have **no `\|raw` carrier**, so a coded LINK meaning or a decorated `DV_DATE_TIME` is a typed error. Owner admissibility comes from the RM itself (`rminfo`), not a hand table, which makes `_guideline_id` CARE_ENTRY-only — `ADMIN_ENTRY` carries `_work_flow_id` and no `_guideline_id`, exactly as the corpus writes it. A single-valued family admits `:0` beside the index-less spelling, because the OPT-free FLAT ↔ STRUCTURED interconversion re-spells every segment with an explicit index — as does a *sub-path* segment inside a family (`_normal_range/lower:0`), which the same interconversion produces. **The value-decoration families narrow the `\|raw` boundary and therefore change emitted bytes** (unlike C0's, which were new coverage): a value whose only extras are `normal_range`, `other_reference_ranges` or `mappings` now rides its suffix form **plus** `_` keys where it previously rode one whole `\|raw` fragment. `capturedKeysDecorated` derives that widening from `rminfo`, so the encode boundary and the decode router's admissibility rule are the same rule; a value with an extra still outside the grammar (a non-`openehr` `normal_status`, DV_TEXT's `language` / `encoding`) keeps riding `\|raw` **whole, with no `_` keys beside it** — never both spellings for one value. Which family a leaf admits comes from the **leaf datatype** (`normal_range` on every DV_ORDERED, `mappings` on DV_TEXT and its coded subtype), not from the ELEMENT: one FLAT path addresses both objects, and `_null_flavour` / `_null_reason` are the two that decorate the ELEMENT itself — which is what makes them legal beside an **absent** value (an ELEMENT with a null flavour and no value emits the `_null_flavour` keys and no bare key, and decodes back to exactly that). Nested values reuse the leaf machinery in both directions, so an interval bound, a `/meaning`, a `/target` or a `/purpose` the suffix set cannot capture rides `\|raw` at its own position rather than being refused. Two boundary rules are load-bearing: `\|*_unbounded` is emitted only when **true** and `\|*_included` only when **false** (see § interval boundary flags below), and `\|match` is validated lexically against `=` `<` `>` `?` on both sides because the RM types it as a bare Character. | landed (REQ-140 Phase C0 + C1, 2026-08-05; `_name` 2026-10-15) |
| Underscore-prefixed RM attributes — the **party** families ([REQ-140](../../../docs/specifications/wire.md#req-140--underscore-prefixed-rm-attributes)): `context/_health_care_facility`, `context/_participation:N`, the five ENTRY subtypes' `_other_participation:N`, and the nested `_identifier:N` (DV_IDENTIFIER) every party position carries. | Both directions, byte-exact, with **one implementation for every position the grammar reaches a party** — a family's whole value, a PARTICIPATION performer inlined on the same key base, the ENTRY `subject` leaf, (Phase C3) a FEEDER_AUDIT_DETAILS party, and (ADR 0029) the composition's composer at its real path `<root>/composer`. Three discriminations are read off the keys: `/relationship` selects PARTY_RELATED, any other party key selects PARTY_IDENTIFIED, and **no** party key is PARTY_SELF — which is how the reference writes a record-subject performer (`\|function` + `\|mode` alone) and what keeps the `WithTemplate` PARTY_SELF `subject` default byte-identical on re-encode. `\|id_scheme` is the `external_ref`'s OBJECT_ID discriminator, the same policy the OBJECT_REF families use (present ⇒ GENERIC_ID, absent ⇒ HIER_OBJECT_ID; note the suffix is `\|id_namespace` here where those spell `\|namespace`). A PARTICIPATION spells its performer's identifiers as the reference's **inlined** `\|identifiers_<field>:N` and a standalone party as the nested `_identifier:N`; each position reads only its own, so the nested spelling on a participation is unmatched content there (REQ-170). **This changed emitted bytes**: a composition carrying a `health_care_facility`, `participations`, `other_participations` or an ENTRY `subject` now emits those keys where it previously emitted nothing. The party grammar has no `\|raw` fallback, so every shape the suffixes cannot express is a typed error: a PARTICIPATION `time` (no channel in the reference's suffix set, corpus-unexercised — **deferred**); a coded or decorated `\|function`; and, from the PR #89 review, a PARTY_IDENTIFIED carrying none of `name` / `identifiers` / `external_ref` or carrying an empty `name` (RM invariants `Basic_validity` / `Name_valid` — it would write no key, and no party key is how this grammar spells PARTY_SELF), plus a nil RM-mandatory `PARTICIPATION.performer` for the same reason. Decode refuses all four shapes symmetrically, in every mode: a PARTICIPATION `\|time` key resolves but has no carrier, and the other three are malformed (REQ-170). | landed (REQ-140 Phase C2, 2026-08-05) |
| The party `external_ref`'s `type` — `\|type`, emitted **only** when it differs from the reference's hardcoded `PARTY`. | The reference writes no `\|type` at a party and hardcodes `PARTY`, which is lossy: the vendored `clinical_content_validation` composition carries `PERSON` and `ORGANISATION` references at its composer, health-care facility and every participation performer. So absent decodes as `PARTY` — every corpus body round-trips byte-exactly and never gains the key — and encode spells `\|type` where the value differs, rather than normalising it away (a silent loss) or refusing it (which would have failed a green PROBE-076 leg). The suffix is not invented vocabulary: `\|type` is the reference's own spelling of the same RM attribute in the OBJECT_REF families (`_work_flow_id\|type`), written at one further position. An **empty** `type` is `ErrUnsupportedDatatype`, not a defaulted one. | landed (REQ-140 Phase C2, 2026-08-05) |
| **Vendored vocabulary** — the openEHR Terminology `participation mode` group (codes 193–224, code ↔ rubric), read from `openehr/terminology` (`terminology.ParticipationMode`). | The only terminology this codec reads, and it reads it because the reference gives `\|mode` **no code channel**: it writes the bare rubric (`"face-to-face communication"`) where PARTICIPATION.mode is a DV_CODED_TEXT whose `defining_code` is RM-mandatory, so rebuilding the code is the only alternative to fabricating an empty CODE_PHRASE. **No hand-typed copy remains in the codec** (since 2026-09-07, [§ REQ-034](../../../docs/specifications/rm-modeling.md#openehr-terminology-vocabulary-req-034)): the table is generated from the pinned `resources/terminology/openehr_terminology.xml` (TERM Release-3.0.0), so it is versioned with the openEHR specifications rather than transcribed here — and it is still a compiled-in table, not a runtime terminology lookup. The same package supplies the terminology **identifiers** the codec implies: `openehr` for `\|normal_status` and `ctx/setting`, and the external identifiers of the openEHR code sets for `\|compression_algorithm` and `\|integrity_check_algorithm` (`openehr_compression_algorithms`, `openehr_integrity_check_algorithms`); only `\|mediatype`'s `IANA_media-types` is still typed in the codec. It also supplies the rubrics of the three `ctx/` completion defaults (`238 other care`, `433 event`, `640 actual`), so no rubric is typed beside a code anywhere in the codec. A rubric outside the group is refused loudly; encode is the exact inverse and refuses a mode whose code, terminology (`openehr` is implied) or value that rebuild would not reproduce, plus any DV_CODED_TEXT decoration the bare key cannot hold. `TestParticipationModeRoundTripsEveryGroupMember` pins the round trip over all 32 members, and `TestCtxDefaultsAreGroupMembersWithPinnedRubrics` pins that each `ctx/` default is a group member carrying the pin's own rubric. | landed (REQ-140 Phase C2, 2026-08-05; reads the pin since REQ-034, 2026-09-07) |
| **Implied terminology** — one code-set identifier per `DV_MULTIMEDIA` coded attribute, in `datatypes.go`: `IANA_media-types` for `media_type`, `openehr_compression_algorithms` for `compression_algorithm` and `openehr_integrity_check_algorithms` for `integrity_check_algorithm`. | Not a vocabulary but one code-set **identifier** per attribute, implied because `\|mediatype`, `\|compression_algorithm` and `\|integrity_check_algorithm` each carry the code alone. None is invented: the RM names the openEHR code set for each attribute. The `media types` identifier is the one this SDK already pins in the template-instance writer (`rmwrite`, REQ-107). The two algorithm identifiers are the external ids the pinned openEHR terminology gives the `compression algorithms` and `integrity check algorithms` code sets, read from `openehr/terminology` (`terminology.CompressionAlgorithms`, `terminology.IntegrityCheckAlgorithms`, [§ REQ-034](../../../docs/specifications/rm-modeling.md#openehr-terminology-vocabulary-req-034)), so the codec types neither. Only the identifier is read: decode does not check that the code is a member of the set, as it does not for `\|normal_status`; membership is the validation floor's to check. The implication is safe only because encode refuses to take the suffix form for a value coded in any other terminology, or carrying a `preferred_term` — that value rides `\|raw` whole rather than being re-terminologised — which is the `\|normal_status` rule one attribute up; an **absent** terminology contradicts nothing the implication asserts and survives the suffix form, as it does for `\|normal_status`. **This changed decoded output**: the two algorithms used to decode to a code-only CODE_PHRASE, without the `terminology_id` the RM makes mandatory, and a value in its implied code set now takes the suffix form on encode where it rode `\|raw`. | landed (REQ-140 Phase C3, 2026-08-05; the algorithm identifiers from the pin since 2026-10-14) |
| Underscore-prefixed RM attributes — the **DataValue-member** families ([REQ-140](../../../docs/specifications/wire.md#req-140--underscore-prefixed-rm-attributes)): `_charset`, `_language`, `_encoding` (CODE_PHRASE), `_thumbnail` (a nested DV_MULTIMEDIA) and `_accuracy` (a bare DV_DURATION). | Both directions, byte-exact. All five are ordinary **value** families whose admissibility is read off the RM (`rminfo`): `charset` reaches the two DV_ENCAPSULATED subtypes, `language` those two **and** DV_TEXT with its coded subtype, `encoding` only DV_TEXT, `thumbnail` only DV_MULTIMEDIA. `_accuracy` needs one extra gate — DV_TEMPORAL redefines `accuracy` as a DV_DURATION **object** where DV_AMOUNT declares a `Real`, and the `Real` form already has the scalar `\|accuracy` suffix, so the family carries a declared-type predicate (`rmattrFamily.attrType`) and reaches DV_DATE / DV_DATE_TIME / DV_TIME and nothing else. One attribute therefore never has two channels at one owner. Each member goes out through its own datatype's **leaf** emitter, so the `\|raw` carrier behind that key takes anything the suffixes cannot hold (a decorated `TERMINOLOGY_ID` → `_charset\|raw`; a thumbnail carrying its own charset → `_thumbnail\|raw`, which is why the family needs no recursive sub-path spelling and the corpus never writes one). A member whose `code_string` is **empty** is a typed error rather than a silent skip: these are pointer fields, so "present but blank" is distinguishable from absent, and `codePhraseToFlat` writes nothing for an empty code. **This narrows the `\|raw` boundary**: a DV_TEXT or DV_CODED_TEXT carrying a `language` / `encoding`, and a temporal carrying its DV_DURATION `accuracy`, now ride suffixes plus `_` keys where they rode one whole `\|raw` fragment; `hyperlink` is what still forces it for a text value. | landed (REQ-140 Phase C3, 2026-08-05) |
| The **encapsulated** leaves — `DV_PARSABLE` (bare value + `\|formalism`) and `DV_MULTIMEDIA` (bare **uri** + `\|mediatype` + `\|size` + `\|data` + `\|alternatetext` + `\|integrity_check` + `\|integrity_check_algorithm` + `\|compression_algorithm`). | Both directions, byte-exact, at a Web Template leaf and at every nested position the underscore grammar reaches one (`_thumbnail`, `original_content`, `original_content_multimedia`). Four corpus findings are load-bearing and all four correct an earlier guess: (a) DV_MULTIMEDIA's **bare key is the `uri`**, not the inline data — the octets ride `\|data`, and the corpus `_thumbnail` spells `\|data` with no bare key at all; (b) the reference writes `\|mediatype` and `\|alternatetext` **without** the underscore the RM attribute has; (c) `\|data` and `\|integrity_check` carry the **base64** of the RM's `Byte[]`, which is what the canonical JSON form uses, so the wire value round-trips through `[]byte` unchanged; (d) the three CODE_PHRASE-valued attributes travel as a **bare code**, each in an implied terminology (the *Implied terminology* row above): `media_type` in `IANA_media-types`, the two algorithms in their openEHR code sets, by the identifiers the pinned terminology gives them (`openehr_compression_algorithms`, `openehr_integrity_check_algorithms`, REQ-034); a value that rebuild would re-terminologise, or one carrying a `preferred_term`, rides `\|raw` **whole**. Both types' `charset` / `language` ride the `_charset` / `_language` members above, so a DV_PARSABLE at a leaf is now **always** fully captured and never rides `\|raw`; at a *nested* position (`original_content`) those members have no key, so a charset-carrying value rides `original_content\|raw` — lossless, and the shape the corpus never writes stays unwritten. `media_type` and `size` are RM-mandatory and required on decode, so a half-spelled multimedia is refused rather than zero-filled. **This changed emitted bytes** for `ACTIVITY.timing`, which rode `\|raw` and now rides its two suffixes. | landed (REQ-140 Phase C3, 2026-08-05) |
| The `DV_INTERVAL<T>` leaf — the REQ-140 interval grammar at a template-modelled node: `/lower` + `/upper` in the bound datatype's own suffix form, plus the four boundary Booleans. | Both directions, byte-exact, sharing C1's one implementation (`intervalSuffixes` / `intervalToFlat`) with `_normal_range` and `_other_reference_ranges:N`. The **anchor** — the datatype the bounds are spelled with — comes from the Web Template leaf type, which names it inside the angle brackets (`DV_INTERVAL<DV_QUANTITY>`); a bare, unparameterised `DV_INTERVAL` names none, so it is not treated as an interval leaf and rides `\|raw` rather than being mis-spelled. Decode siphons the leaf's keys before the ordinary leaf loop (`compositeLeafGroups`, generalised from C2's party-leaf siphon), because `/lower` and `/upper` are not Web Template children and would otherwise fail as unresolvable paths. Encode enumerates the generated `DVInterval[T]` instantiations explicitly (no reflection, REQ-024) — including the abstract `DVInterval[DVOrdered]` a canonical decode produces. | landed (REQ-140 Phase C3, 2026-08-05) |
| Underscore-prefixed RM attributes — the **`_feeder_audit`** family ([REQ-140](../../../docs/specifications/wire.md#req-140--underscore-prefixed-rm-attributes)), on any LOCATABLE the Web Template **models** (the corpus writes it at nine positions across 14 of its 34 bodies; on a folded structural wrapper it is dropped — § folded structural wrappers below): `originating_system_item_id:N` / `feeder_system_item_id:N` (DV_IDENTIFIER lists — the FLAT segment is **singular** where the RM attribute is plural), `originating_system_audit` (RM-mandatory) / `feeder_system_audit` (FEEDER_AUDIT_DETAILS: `\|system_id` mandatory, `\|version_id` and `\|time` optional; `/location`, `/subject`, `/provider`), and `original_content` **or** `original_content_multimedia`. | Both directions, byte-exact. It is the deepest family in the grammar and the only one whose tails nest **three** levels (`…/originating_system_audit/provider/_identifier:0\|id`), which the C1 tail machinery cannot express — it flattens a multi-segment sub-path and admits a `:index` only on the leading segment. Rather than widen that, `rmattrChildGroups` re-roots each sub-path segment into its own group, so every level is decoded by the machinery that already owns it: a party by C2's `partyLeafSuffixes` (nested `_identifier:N` and PARTY_RELATED `/relationship` included), an item id and an `original_content` by the datatype leaf builder. The **DV_ENCAPSULATED choice is by key name** in both directions — `original_content` is the DV_PARSABLE, `original_content_multimedia` the DV_MULTIMEDIA — and a body carrying both spellings of the one RM attribute is a typed error, not a silent pick. FEEDER_AUDIT has no scalar attribute at all, so a bare value or a `\|suffix` on the family itself is unmatched content (REQ-170), as is a sub-path no FEEDER_AUDIT or FEEDER_AUDIT_DETAILS attribute spells; an absent `originating_system_audit` is refused rather than defaulted (fabricating one would put an empty `system_id` on the wire), and so is an empty `system_id` on encode. **This changed emitted bytes**: a composition carrying a `feeder_audit` anywhere now emits those keys where it previously emitted nothing. | landed (REQ-140 Phase C3, 2026-08-05) |
| PARTY_SELF at an **RM-optional** PARTY_PROXY position — `\|_type: "PARTY_SELF"` at FEEDER_AUDIT_DETAILS' `subject` and ENTRY's `_provider`. | Both directions. PARTY_SELF is spelled by the **absence** of every party key wherever that absence is unambiguous — a PARTICIPATION performer and an ENTRY `subject`, both RM-mandatory. At an RM-**optional** PARTY_PROXY absence already means *absent*, so the reference writes the discriminator explicitly (`ehrbase_conformance_party_self` carries `…/originating_system_audit/subject\|_type: "PARTY_SELF"` and no other key there) and this codec accepts and emits it at exactly those two positions. The suffix is exclusive: any other `\|_type` value is refused (every other subtype is implied by the party keys themselves, and re-emitting a redundant discriminator would put a key on the wire the reference never writes), as is a `\|_type` standing beside party keys that contradict it, and a PARTY_SELF carrying an `external_ref` — which the suffix set cannot spell beside the discriminator. | landed (REQ-140 Phase C3, 2026-08-05) |
| Underscore-prefixed RM attributes — **rest deferred**: `_instruction_details` (ACTION) and `_wf_definition` (INSTRUCTION), plus FEEDER_AUDIT_DETAILS' `other_details` (an ITEM_STRUCTURE) — all three named by the spec and **unexercised by the pinned corpus** as far as a *shape* goes (the first two appear as keys; no fixture supplies a decodable value grammar for any of them). | Refused loudly on both sides, in every mode, and therefore visible in the PROBE-086 census: each key resolves but has no carrier (REQ-170). The two families are refused by the router (`ErrUnknownPath`) at the owner that declares them, an ACTION and an INSTRUCTION; at any other owner they name nothing and are unmatched content. `other_details` is refused through a typed refusal naming the key and the deferral. `ism_transition/_reason:N` is **not** in this row — the reference's in-context `ism_transition` node is absent from this SDK's WebTemplate projection, and REQ-140's third out-of-scope class covers an ISM transition's `transition` and `reason`, and decode refuses the reference's in-context spelling of an ACTION's ISM transition in every mode (REQ-170, the ISM transition note in § Fail-loud posture): its `_reason` family, like its in-context leaves, is refused, and closing it is a builder question, not a grammar one (see `webtemplate/deviations.md` § inContext coverage). **`_identifier:N` is deliberately not a router family**: every position that reaches a party reaches its identifier list through the party grammar, so `<entry>/_identifier:N` names no family and is unmatched content (`ErrUnknownPath` under `WithStrict`). Since Phase C3 every other underscore family the corpus writes is carried. | Deferred |
| `\|raw` escape hatch (canonical fragment for exotic/decorated datatypes) | Supported both directions: encode emits `\|raw` for non-core or decorated `DV_*`; decode accepts a `\|raw` fragment that carries a string `_type` and is not combined with any other suffix; encode stamps the fragment with the value's **dynamic** type when it can classify it. On decode, `\|raw` is **not** checked against the leaf's Web Template constraint (an explicit bypass) — a documented relaxation. It **is** checked against the RM attribute it fills (REQ-053, a value must fit its attribute): its `_type` must name that attribute's declared type or an RM subtype of it, so a collapsed ELEMENT's value admits any `DATA_VALUE` and an interval bound the RM holds as an interface any `DV_ORDERED`, while `_name\|raw` admits a `DV_TEXT` or a `DV_CODED_TEXT` only; and canonical JSON must decode the fragment as that `_type`, so a member of the wrong kind (`"magnitude": "abc"` in a `DV_QUANTITY`), or a class no value can be (an abstract one, or one spelled with parameters), does not fit either. A fragment that does not fit is refused, wrapping `ErrUnsupportedDatatype` and naming its `\|raw` key, never a member's value or its path inside the fragment. Before 2026-10-15 a fragment whose `_type` fitted but whose members did not failed in canonical JSON, naming a canonical path and quoting the value. A `\|raw` key that resolves to a node with children (the composition root, the `EVENT_CONTEXT`, a SECTION, an ENTRY, an EVENT, a CLUSTER, an ACTIVITY, an ISM_TRANSITION) is refused the same way in every mode, whatever its `_type`, since `\|raw` carries a datatype; a null one is absent. Before 2026-10-15 a fragment there that fitted was placed at the node and then dropped without a word once the node's own keys rebuilt it, and failed in canonical JSON naming no FLAT key where the node had none. | landed (Task 6); attribute check 2026-10-15 |
| `\|other` open-value-set free text for `DV_CODED_TEXT` | Supported: an **undecorated** `DV_TEXT` at a `DV_CODED_TEXT` leaf whose Web Template input is `listOpen` encodes to `\|other`; decode maps `\|other` back to `DV_TEXT`, requiring `listOpen` and rejecting `\|other` combined with **any** other suffix. `\|other` carries the value alone, so a decorated `DV_TEXT` (a `formatting`, a `hyperlink`, …) is not expressible in this form and rides `\|raw` instead. | landed (Task 6) |
| `.schema`-suffixed media types on input | **Landed 2026-09-05** (REQ-053 SHOULD): [`ParseMediaType`](mediatype.go) classifies one media-type token under the § REQ-053 contract (the two canonical strings and the deprecated `.schema`-suffixed variants — retired from the specification, still served by EHRbase — accepted; anything else refused with `ErrUnknownMediaType`); `Format.MediaType` emits the canonical strings only, never `.schema`. The codecs still take bytes — negotiation is the one call before them. | Landed |
| `CODE_PHRASE` leaves (ENTRY `language` / `encoding`, and REQ-140's `_charset` / `_language` / `_encoding` members) — the reference emits these as leaves in their own right, under the `\|code` + `\|terminology` pair a `DV_CODED_TEXT`'s `defining_code` uses, **plus** a `\|preferred_term` that nested spelling has no channel for. | Supported both directions. The `\|preferred_term` suffix landed with REQ-140 Phase C3, because the corpus writes it at `dv_text/_language\|preferred_term`; **this narrows the `\|raw` boundary** for a standalone CODE_PHRASE, and the asymmetry with the nested form is deliberate — a `defining_code` carrying a `preferred_term` still rides `\|raw`, since `\|code`+`\|value`+`\|terminology` has nowhere to put it. The **all-zero** value writes nothing (the field is non-pointer at the ENTRY leaves, so "unset" and "zero" coincide and an unconditional emit would put blank leaves on every `ctx/`-decoded composition); a decorated `TERMINOLOGY_ID` rides `\|raw`, and it is the only shape that still does. A **partly-populated** value — an empty `code_string` beside a non-empty `TERMINOLOGY_ID` or a `preferred_term` — also rides `\|raw`: the empty-code skip would otherwise drop it silently, so it is deliberately not treated as captured (PR #86 review round 3). At the REQ-140 *member* positions the field is a **pointer**, so an empty code there is a typed error instead (§ DataValue-member families). | landed (PROBE-086 ratchet; `\|preferred_term` REQ-140 Phase C3) |
| ENTRY `subject` (PARTY_PROXY) | **Landed 2026-08-05** (REQ-053 amendment, on REQ-140's party grammar). Both directions, byte-exact: PARTY_SELF is spelled by the **absence** of every party key — which is what keeps the `WithTemplate` PARTY_SELF completion byte-identical on re-encode — PARTY_IDENTIFIED emits `\|id` / `\|id_scheme` / `\|id_namespace` / `\|name` plus the nested `_identifier:N`, and PARTY_RELATED adds `/relationship`. Decode siphons the whole leaf as one party group before the `_` router, because those three key shapes address one RM value and the concrete subtype is only known once all three are in hand. `rmpath` resolves `subject` on the five ENTRY subtypes (REQ-121) and the `TestInContextLeavesResolveViaRmpath` exemptions are gone. | landed (REQ-140 Phase C2, 2026-08-05) |
| `STRING` leaf (ACTIVITY `action_archetype_id`) | Supported both directions: a bare JSON string at the leaf's key, the RM `String` as written (the corpus writes `…/current_activity/action_archetype_id: "/openEHR-EHR-CLUSTER.conformance_action.v0/"`). Any suffix there, `\|raw` included, is outside the leaf's suffix set: unmatched content (REQ-170), ignored by default and refused with `ErrUnknownPath` under `WithStrict`. A value that is not a JSON string does not fit the RM `String` and is refused in every mode with `ErrUnsupportedDatatype` naming the key (REQ-053); a null one is absent. An empty string is refused in every mode, with a plain wrapped error and no gap sentinel: it fits the `String`, but the payload breaks ACTIVITY's `Action_archetype_id_valid` invariant, which is a defect in the body rather than a datatype the codec declines to model, and encode writes nothing for one. `rmpath` resolves the attribute to the String itself. **Fail-closed elsewhere:** the Web Template builder makes this one STRING leaf, so a STRING leaf on an attribute `rmpath` does not resolve to an RM String (from a Web Template built elsewhere) is `ErrUnsupportedDatatype` whenever its owner is in the composition, because whether it holds a value cannot be told; with no owner it is skipped. It was the last non-`DV_` leaf type encode skipped; with it gone, a populated value at a leaf type the codec does not map is refused, not skipped. | landed (REQ-053 *Leaf datatypes* STRING row) |

## Deviations

- **`LOCATABLE.name` on decode** — the FLAT/STRUCTURED formats carry no node's name but the
  composition's own, and the Web Template collapses the HISTORY / ITEM_STRUCTURE wrappers, so
  decode cannot name every node from the WT alone. Passing
  [`WithTemplate(compiled)`](simplified.go) repopulates the mandatory `name` on every
  reconstructed node below the root from the archetype terminology (keyed by the compiled
  aqlPath); without it, those nodes are unnamed and the round-trip is merely
  **format-idempotent**. Their names never leak into FLAT, so idempotence is preserved either
  way. **The composition's own name** travels as `<root>/_name` (REQ-140, since 2026-10-15),
  in both modes: decode sets the COMPOSITION's `name` from it, in the DV_TEXT leaf grammar
  (`\|formatting` beside a bare name, a coded name in the `\|code` + `\|value` +
  `\|terminology` substitution form, a name the suffixes cannot carry as `_name\|raw`), and a
  body without it names the composition after the Web Template root, or after the template id
  where the root has no name. `WithTemplate` never replaces a decoded `_name`. Encode writes
  `<root>/_name` unless the name is a plain DV_TEXT, one that is no coded text and sets
  nothing but its value, whose value is that root name, so a composition named after its
  template writes no extra key and every other name round-trips. `_name` at any other node is
  unmatched content (REQ-170). Until then decode reported a renamed composition's
  `<root>/_name` as unmatched and named the composition after the template, and encode wrote
  no name at all (the PROBE-105 census recorded it for `consult_record` and `ehrn_abdm`).

- **`archetype_details` is rebuilt on decode, not carried.** FLAT has no key for it, because
  the Web Template already holds the archetype id of every archetype root and the template id.
  Decode gives every node the Web Template identifies by an archetype id an `ARCHETYPED` whose
  `archetype_id` is that id and whose `rm_version` is `rm.Release`, and adds the
  Web Template's `templateId` as `template_id` on the COMPOSITION root only. The Web Template
  identifies a node by its node id or, for a structural wrapper it folds away (an archetyped
  `ITEM_TREE` under an `ACTIVITY` description, for instance), by the predicate its children's
  paths carry for the wrapper. Neither value
  needs `WithTemplate`, so both decode modes do it. The placement matches the reference
  (observed on EHRbase 2.36.0 with openEHR SDK 2.35.0, 2026-10-01). The `rm_version` value
  does not: the reference writes a fixed `1.0.4`, and the SDK writes the RM release it is
  generated from. Encode writes nothing for `archetype_details`, so a round trip rebuilds it:
  an `archetype_id` that differs from its node id takes the node id, any other `rm_version`
  becomes `rm.Release`, and a `template_id` below the root is dropped (`wire.md` § REQ-140,
  the second out-of-scope class).

- **RM-mandatory attributes not carried by FLAT — completed on `WithTemplate` decode.** The
  formats omit several RM-mandatory attributes that are neither clinical-data leaves nor names
  (`HISTORY.origin`, `EVENT.time`, `ENTRY.language`/`.encoding`/`.subject`,
  `EVENT_CONTEXT.setting`, `COMPOSITION.category`/`.composer`, `INTERVAL_EVENT.math_function`/
  `.width`). With `WithTemplate`, decode now completes them from `ctx/` defaults + RM
  conventions (`rminfo.RequiredAttributes` drives the walk), so the decoded composition
  **validates against the OPT** (verified over the vendored corpus by PROBE-076's conformance
  leg and `names_test.go`). These are **synthesised defaults**, not recovered data — the
  formats never carried them, so e.g. every `EVENT.time`/`HISTORY.origin` takes the context
  `start_time` and `subject` becomes `PARTY_SELF`. **Qualifier:** `EVENT.time` and
  `HISTORY.origin` have no source other than `ctx/time` — a payload without `ctx/time`
  decodes successfully but does not validate when the template carries HISTORY/EVENT nodes
  (pinned by `names_test.go`). Without `WithTemplate`, decode omits names and defaults
  entirely (format-idempotent only). **One completion is visible on re-encode** (since
  2026-08-05, the amended REQ-053): the synthesised `EVENT_CONTEXT.setting` default
  (`238 other care`) re-encodes as `ctx/setting\|code` + `\|value` like any populated
  setting, so a `WithTemplate` decode of a body that carried no setting re-encodes with
  exactly those two keys gained — a *faithful* encoding of the completed composition, not
  drift; an OPT-free decode synthesises nothing and stays byte-identical (both pinned by
  `TestSettingWithTemplateDefaultRoundTrip`). The other completions stay invisible on
  re-encode: the names it gives the nodes below the root never leak into FLAT, and
  origin/encoding have no emitted
  surface. `subject` acquired one on 2026-08-05 (Phase C2) and stays invisible anyway,
  because the completion's default is a `PARTY_SELF` and the party grammar spells
  PARTY_SELF by the **absence** of every party key — the symmetry
  `TestSubjectLeafPartySelfEmitsNothing` pins. (The former deviation here —
  "`EVENT_CONTEXT.setting` is dropped on encode",
  once the one in-context attribute this codec lost outright, and the PROBE-086 census's
  one documented waiver — was closed by that same amendment: encode emits the pair, decode
  accepts either spelling, and the real path `…/context/setting` stays deliberately
  unresolvable in `rmpath` so the value cannot double-spell.)

- **Empty repeat instances are not representable in FLAT** — an instance of a repeatable
  node whose subtree contributes no FLAT keys (all leaves below it absent) is omitted on
  encode and does **not** consume an `:index`; later instances close ranks. Stamping the
  index by RM list position instead would emit a sparse sequence (`:0`,`:2`) that the
  decoder rejects as phantom gap-fill, breaking `MarshalFlat → UnmarshalFlat` on a valid
  composition. Consequence: a composition with such an empty instance round-trips
  **minus that instance**, because the format simply has no spelling for "an instance
  with no values". The fourth of the four narrowings listed at the top of this file;
  the other three are the folded structural wrappers, `archetype_details` and an ACTION
  transition's `transition` and `reason`.

- **`ITEM_TREE` vs `ITEM_LIST` on decode** — the Web Template collapses `ITEM_STRUCTURE`
  nodes, so the concrete subtype is inferred from the child aqlPath attribute:
  `item` → `ITEM_SINGLE`, `rows` → `ITEM_TABLE`, `items` → `ITEM_TREE`. `ITEM_TREE` and
  `ITEM_LIST` both use `items` and are indistinguishable from the path alone, so `items`
  defaults to `ITEM_TREE`. This is round-trip-preserving (the leaf values and their
  paths are identical); it can differ from an upstream canonical that used `ITEM_LIST`.

## Implementation notes (not deviations, but worth recording)

- **Integer precision** — FLAT/STRUCTURED JSON is decoded with `json.Number`
  (`UseNumber`), so a `DV_COUNT` magnitude above 2^53 is preserved exactly through
  decode and through OPT-free interconversion rather than being rounded via `float64`.
- **Zero-valued mandatory in-context attributes emit empty leaves** — an in-context RM
  attribute that is RM-**mandatory** and value-typed (a non-pointer struct, e.g. `EVENT.time`)
  always resolves, so a composition that left it at its zero value emits the leaf with an empty
  value rather than omitting it: the encoder cannot tell "unset" from "set to the zero value"
  for a non-pointer field. Only reachable with RM-invalid input — a valid composition populates
  it, and a `WithTemplate` decode synthesises it (§ Deviations, RM-mandatory attributes).

- **`:index` strictness** — a FLAT `:index` must be canonically spelled (`0`, `1`, …;
  negative, `+`, or zero-padded spellings are rejected — they would collide with other
  keys), must not be sparse (a gap would fabricate a phantom empty instance), and is capped
  at `maxRepeatIndex` (see `flat_decode.go`) with a total decoded-node budget on top, so a
  hostile key cannot force an unbounded allocation. Violations are `ErrUnknownPath`.

- **OPT-free `FlatToStructured` → `StructuredToFlat` normalises `:index`** — STRUCTURED is
  arrays-always (spec), and interconversion has no OPT, so the back-conversion cannot tell
  a single-cardinality leaf (no `:index` in FLAT) from a one-element repeatable (`:0`); it
  emits `:0` on both. The result is valid-but-verbose FLAT that decodes to the same
  composition (the redundant `:0` on a max=1 node is ignored on decode), so interconversion
  is **semantics-preserving, not byte-identical**. PROBE-076 asserts the semantic form
  (decode + re-encode equals the original FLAT). Since REQ-140 the normalisation reaches a
  *sub-path* segment inside an underscore family too (`_normal_range/lower` → `…/lower:0`),
  which decode folds back the same way; `:1` or higher there is refused, since no attribute
  of a DataValue is a list at that position.

- **Interval boundary flags are asymmetric** (REQ-140) — each of the four DV_INTERVAL
  Booleans is written only where it differs from its bounded-side default, and decode reads
  an absent one back by side.
  BASE `Interval` declares all four mandatory (`1..1`), and the reference omits each one's
  bounded-side default. `\|lower_unbounded` / `\|upper_unbounded` are omitted when `false`, and
  only `true` is emitted. Absent, they follow the side's bound (wire.md § REQ-169, amended
  2026-10-13): `false` beside a bound, `true` where the side's bound keys are absent or all
  null, so an interval that spells only an upper bound is open below. A side marked
  `\|*_unbounded: false` with no bound names a closed end with no value: decode refuses it with
  `ErrUnsupportedDatatype` where the RM holds the bound by value (the `_normal_range` of a
  DV_COUNT, DV_QUANTITY or DV_PROPORTION), and reads it as written, with no bound, where the RM
  holds it as an interface (every other `_normal_range`, every `_other_reference_ranges` range,
  a DV_INTERVAL leaf). `\|lower_included` / `\|upper_included`
  default by side. On a bounded side they are omitted when `true`, the **closed** endpoint:
  absent decodes as `true`, and only `false` is emitted. On an open side absent decodes as
  `false`, the only value BASE `Lower_included_valid` / `Upper_included_valid` permit there.
  An included open side, an explicit `\|*_included: true` beside `\|*_unbounded: true`,
  breaks that invariant, but a typed value can still be built from it, so decode reads it
  as written (wire.md § REQ-169) and RM validation reports it (clinical-modeling.md
  § REQ-112). Encode refuses, with a typed error, to write an interval side both unbounded
  and included into the interval keys (a `\|raw` fragment carries it as canonical JSON
  writes it), so the pair decodes but does not re-encode. An open end is spelled by its
  `\|*_unbounded: true` flag alone. Encode refuses a side not marked unbounded that has no
  bound to spell (written without either, it would read back as open), and a missing bound is
  never emitted as a zero-valued one. Consequences, deliberate: a redundant
  `\|lower_included: true` on a bounded side is **normalised away** on re-encode, and an
  absent `\|*_included` on an open side re-encodes as an explicit `false`. Each denotes the
  same RM value as the other spelling, so these are canonical-spelling normalisations like
  the `:0` one above, not a loss.
  The bounded-side defaults are corpus-derived: `dv_count` omits both flags on a
  bounded interval, and `dv_quantity` spells both `false`. Every open end in the
  corpus (`dv_ordinal`, `dv_quantity`, `interval_dv_quantity`) spells
  `\|*_included: false`, so BASE `Lower_included_valid` / `Upper_included_valid`
  fix the open-side default, not the corpus (wire.md § REQ-140).

## Conformance

**PROBE-076** (landed) exercises the codec over the vendored EHRbase `Test_dv_*` corpus
(OPT + canonical composition) — 24 pass, 1 skip. It asserts **round-trip idempotence**
(FLAT/STRUCTURED/interconversion) **and OPT-conformance**: when the source composition is
itself OPT-valid, a `WithTemplate` decode must also validate against the OPT. The conformance
leg catches dropped/mistyped leaves that idempotence alone (a symmetric omission) would miss.
It does **not** compare emitted FLAT/STRUCTURED against vendored upstream simplified
output — that comparison is **PROBE-086** (Implemented, Sandbox): its harness at
`testkit/conformance/webtemplate/` round-trips the corpus vendored at
`testkit/corpus/flat-conformance/` over the modelled subset, with the refusal
inventory in that package's `SKIPPED.md`.

**PROBE-089** (Implemented, Sandbox — `testkit/probes/serialize/probe_089_underscore_round_trip.go`)
is the per-family view of the REQ-140 grammar, where PROBE-086 is the aggregate one: fourteen
SDK-authored fixtures, one per grammar-table row, each asserting a byte-exact whole-body
round-trip, an encode leg that takes the decoded composition out through **canonical JSON and
back** before re-encoding (so an underscore value the canonical form does not model cannot
survive), the deliberate refusals with the sentinel each boundary declares, and the STRUCTURED
vocabulary as array-valued members.

**A bare leaf value beside the same leaf's members: the `"|"` STRUCTURED member — closed
2026-08-05 (REQ-140 Phase C3).** STRUCTURED gives one array element per FLAT segment, and
that element is *either* the leaf's bare value or an object holding the segment's members —
so a leaf spelled bare that also carries a `|suffix` (`…/dv_text` + `|formatting`), an
underscore family (`…/dv_count/_normal_range/lower`, C0's `…/dv_count/_uid`) or a sub-path
needs both at once, and `FlatToStructured` used to **refuse**, loudly. The bare value now
takes the `"|"` member on that object: the `"|"+suffix` convention with the empty suffix the
FLAT key itself spells, which makes the inverse mapping unambiguous **without** an OPT —
`"|value"` would not be, since DV_ORDINAL and DV_CODED_TEXT spell a real `|value`. A leaf
carrying nothing but a bare value stays a bare scalar, so every STRUCTURED body this codec
emitted before C3 is byte-identical. Phase C3 forced the closure rather than choosing it: a
DV_MULTIMEDIA leaf always carries a bare uri *and* RM-mandatory `|mediatype` / `|size`, so
without a spelling it could not interconvert at all and two green PROBE-076 legs would have
turned red. Round-tripped both ways in `structured_test.go`
(`TestBareLeafBesideMembersThroughStructured`, `TestBareStructuredMemberIsTheEmptySuffix`) for
every shape that used to collide. **Not corpus-pinned:** no vendored STRUCTURED fixture
exercises the shape, so `"|"` is this codec's choice within the format's own conventions, and
wire.md § REQ-053 states it normatively as such — the object form, the empty-suffix reading in
both interconversion directions, and the bare-scalar rule for a leaf that cannot collide.

**DV_SCALE takes the DV_ORDINAL suffix set — CLOSED 2026-08-06 (PR #89 review).** DV_SCALE
is DV_ORDINAL with a Real `value`, and was the one `DV_*` with no `capturedKeys` and no
`allowedSuffixes` entry: it rode `|raw` on encode and its suffix form was *refused* on decode,
an unregistered gap rather than a decision. It now takes `|code` + `|value` + `|ordinal`,
`|ordinal` carrying the Real. **Not corpus-pinned:** no vendored body and no reference sample
spells a DV_SCALE, so — like the `"|"` member above — this is this codec's choice within the
format's conventions; reusing the sibling's spelling for the sibling's attribute is the same
move the party `|type` suffix makes. Should a reference sample later spell it otherwise,
[ADR 0014](../../../docs/adr/0014-webtemplate-reference-implementation-lock.md) means the
reference wins. No census movement: DV_SCALE appears in no fixture.

**Interval bound vs `|*_unbounded`.** wire.md § REQ-140 now states the rule for both
directions: a bound standing beside its own `|*_unbounded: true` is a typed error on decode
and on encode. Encode can only report it where the instantiation distinguishes the bound from
absence — see `boundContradictsUnbounded`; a concrete `Interval[DVCount]` cannot, and that
limit is stated there.

**A null-flavoured instance of a *repeating* collapsed ELEMENT is not emitted — CLOSED
2026-08-06 (PR #89 review).** Encode resolved a leaf by its `…/value` path, so an ELEMENT
whose `value` is Void was invisible to that resolution: for a `Max == 1` leaf the owner walk
ran anyway and wrote the `_null_flavour` / `_null_reason` keys (REQ-140 Phase C1), but a
**repeating** leaf took its `:index` from the value-list enumeration, and an instance
contributing no value contributed no slot — so its null flavour had nowhere to go. The fix is
the one this entry predicted: `repeatingLeafOwners` / `emitRepeatingLeafOwners` enumerate the
**ELEMENT** list rather than the value list, so each instance keeps its own `:index` and its
own underscore attributes, and a mid-sequence null-flavour-only instance round-trips. The
same change closed a hard encode failure — the unindexed owner path matched every instance at
once (`ErrPathAmbiguous`) and failed the whole document. Pinned by
`TestMarshalFlatRepeatingElementNullFlavourOnlyInstance` and
`TestRepeatingNullFlavourOnlyInstanceRoundTrips`.

**Folded structural wrappers carry no underscore RM attributes.** The Web Template builder
drops the pure structural LOCATABLEs — `ITEM_TREE`, `ITEM_LIST`, `ITEM_TABLE`, `ITEM_SINGLE`,
`HISTORY`, and a `max == 1` EVENT it lifts — so none of them is a Web Template node and none
has a FLAT key. § REQ-140 gives `_uid`, `_link:N` and `_feeder_audit` to any LOCATABLE the
template *models*, and these are the ones it does not: a `uid` or a gateway-stamped
`feeder_audit` sitting on one of them is **dropped on encode**, with no error.

This is a drop rather than the typed refusal the grammar uses elsewhere, and the reason is
the reference's own spelling: FLAT has no key for a folded wrapper, so there is nothing to
emit and nothing for decode to read back. Refusing instead was measured — EHRbase mints a
`uid` on the `ITEM_TREE` of essentially every composition it writes, and **24 of the vendored
PROBE-076 bodies** carry one, so a refusal would make the reference implementation's own
output unencodable. The composer's `external_ref` and `identifiers` were dropped on the same
trade, for the same reason, until ADR 0029 gave them the composer's real path as a carrier.

Closing it needs a FLAT spelling for a folded wrapper — a reference-side change (ADR 0014:
the reference spelling wins), not a codec one. PROBE-086 cannot see the gap at all, because
its input is FLAT and a folded wrapper never appears there.

**Archetype-reuse siblings (REQ-116 residual).** REQ-116 made templates that reuse one
archetype among siblings *exportable* (distinct WebTemplate `id`s via the pinned
template-level name; name-predicated `aqlPath`s), and this codec handles the pinned
predicate on the paths it consumes: decode strips it when rebuilding lookup keys
(`bareAQLPath`, keyed consistently across `resolveLeaf` / `placeLeaf` / the name index),
and encode strips it before rmpath resolution. A **sole** pinned claimant of its bare
path — every pinned node in `GECCO_Diagnose`, and pinned ELEMENTs generally — round-trips,
with `WithTemplate` decode repopulating the **pinned** name (`flat_pinned_test.go`). What
does **not** round-trip yet is *reused siblings*: several pinned siblings sharing one bare
path (corona's four `SECTION.adhoc.v1`).

- **Encode** — the bare relPath answers to *every* reused sibling: with several
  instances present a `Max==1` node fails on the rmpath ambiguity, but with **one**
  instance present (the common partially-filled composition) each sibling's Web Template
  node would resolve that same instance and emit its data under **every** sibling's FLAT
  id — silent misattribution, no error. Encode therefore **refuses** whenever data
  resolves at a reused sibling (`refuseReusedSibling`, wrapping
  `rmpath.ErrPathAmbiguous`; pinned in `flat_pinned_test.go`, which also pins that a
  composition with no data in the reused region still encodes). Disambiguating needs
  the runtime `name/value` carried into resolution — exactly what the stripped
  predicate expressed — restricted to genuinely ambiguous nodes.
- **Decode** — sibling FLAT ids map to one bare path, so their instances would collapse
  onto one list slot (both default to index 0), landing different siblings' leaves in
  **one** RM instance. Decode therefore **refuses** any key that walks through a reused
  sibling (`ErrUnknownPath`, "reused siblings"; `ambiguousBarePaths` precomputes the
  affected paths, `flat_pinned_test.go` pins the refusal on the corona oracle) — a loud
  no rather than a silent merge. The name index likewise falls back to the shared
  archetype rubric for the bare spelling. Fixing both needs the FLAT segment identity
  (which sibling's id the key walked through) carried into placement — the information
  the stripped predicate expressed.

Reused-sibling FLAT is therefore **fail-loud in both directions** — nothing silently
misattributes data — and closing it is owned by the PROBE-086 adapter work (the upstream
corpus template pins no names, so its parity does not depend on this residual).
