# Simplified Formats — documented deviations & deferrals (REQ-053)

Parity with the openEHR *Simplified Formats* (ITS-REST, STABLE) is **structural, not
byte-exact**. This file records where `openehr/serialize/simplified` intentionally
deviates from, or has not yet implemented, part of the spec. Each entry says what the
current behaviour is and where the full behaviour lands.

Status legend: **Deviation** = deliberate, permanent-ish choice; **Deferred** = not yet
implemented — residual scope tracked by the
[simplified-formats umbrella plan](../../../docs/plans/2026-06-23-simplified-formats.md)
(the [Phase 3 plan](../../../docs/plans/archive/2026-07-14-flat-structured-codecs.md) that
built this package is done and archived).

## Strict, fail-loud posture

The codec never succeeds while silently losing or altering data (REQ-053 is
semantics-preserving). Concretely:

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
  remaining non-`DV_`, non-party leaf types (`STRING` — ACTIVITY `action_archetype_id`) are a
  documented skip. The composition-level metadata leaves (`language`, `territory`, `composer`,
  `context/start_time`, `context/setting`) are held back **by name** (`ctxOnlyLeafPaths`),
  because ADR 0015 gives them a `ctx/`-only output spelling and emitting them at their real
  path would spell one value twice. A container node that resolves to a
  non-`Locatable` RM object (e.g. `EVENT_CONTEXT`) is recursed via the enclosing Locatable
  ancestor, not dropped. A typed-nil RM pointer is treated as an absent leaf (skipped).
  A **composer** the `ctx/` short forms cannot carry — `PARTY_RELATED`, or a
  `PARTY_IDENTIFIED` without a `name` — is `ErrUnsupportedDatatype`, not an omission
  (omitting it would let a `WithTemplate` decode default the composer to `PARTY_SELF`,
  a silent type substitution).
- **Decode** — a key that does not resolve to a Web Template node returns
  [`ErrUnknownPath`](simplified.go); an unmapped datatype, a suffix outside the datatype's
  allowlist (e.g. a `\|unitt` typo), a misused `\|raw`/`\|other`, or a `\|other` on a closed
  value-set return `ErrUnsupportedDatatype`; a missing **required** suffix is an error, not
  a coerced zero value; trailing JSON after the object and an out-of-bound/over-budget
  `:index` are rejected.

Consequence: a payload that uses a not-yet-supported feature (below) is **rejected**,
not partially/silently accepted.

## Deferred features

| Feature | Current behaviour | Lands in |
|---|---|---|
| `ctx/` context — **core supported**: `ctx/language`, `ctx/territory` (both mandatory on decode → `ErrMissingContext`), `ctx/composer_name` / `ctx/composer_self`, `ctx/time` (context `start_time`), `ctx/setting\|code` + `ctx/setting\|value` (context `setting`; the `openehr` terminology is implied, not written). | Emitted on encode; rebuilt on decode. The **all-zero** setting writes nothing (non-pointer field — "unset" and "zero" coincide, the CODE_PHRASE precedent); a **populated** setting the pair cannot carry — a non-`openehr` (or empty) terminology, extras beyond code+value (`mappings`, `formatting`, a `preferred_term`, …), or a value without a code — is `ErrUnsupportedDatatype` naming `ctx/setting`, not an omission (omitting it would let a `WithTemplate` decode substitute the `238 other care` default silently, the composer PARTY_RELATED stance). On decode one half of the pair without the other is refused naming the missing key. | landed (Task 6; `setting` 2026-08-05, REQ-053 amended) |
| Composition metadata in the reference's **real-path** spelling (`<root>/language\|code`, `<root>/territory\|code`, `<root>/composer\|name`, `<root>/composer_self`, `<root>/context/start_time`) | **Accepted on decode**, normalised onto the `ctx/` short forms ([ADR 0015](../../../docs/adr/0015-flat-metadata-spelling.md)); **never emitted** — `ctx/` is the only output spelling. A `\|terminology` witness is checked against the terminology the `ctx/` form implies and then discarded; a mismatch, or two spellings of one field that disagree, is an error rather than a precedence rule — including a composite (object/array) value, which no `ctx/` field can hold and which is refused before it can be compared. `composer_self: true` beside a composer **name** (either spelling) is refused on the same grounds: they are mutually exclusive representations of one RM attribute, and the `PARTY_SELF` branch would silently drop the name (`composer_self: false` beside a name is fine). Respellings only: `context/setting\|code` / `\|value` **are** aliases since 2026-08-05 (the amended REQ-053 closed the `ctx/setting` emission gap ADR 0015 had left open), with `context/setting\|terminology` as an `openehr` witness; the composer's `external_ref` remains the one non-alias (see § `ctx/` context). | landed (ADR 0015; `setting` alias 2026-08-05) |
| `ctx/` context — **rest deferred**: the `ctx/` short forms for participations, `health_care_facility`, `work_flow_id`, composer `external_ref` (`composer_id` / `id_namespace` / `id_scheme`), `end_time`, `location`. | Not emitted on encode; any such `ctx/*` key is rejected on decode (`ErrUnknownPath`). All are optional, so nothing is silently substituted on decode. **The `ctx/` *spelling* is what is deferred here, not always the data**: since 2026-08-05 (REQ-140 Phase C0, [ADR 0016](../../../docs/adr/0016-event-context-optionals-underscore-spelling.md)) `end_time`, `location` and ENTRY `work_flow_id` round-trip under the **underscore grammar** at their real paths (`<root>/context/_end_time`, `<root>/context/_location`, `<entry>/_work_flow_id\|*` — § underscore-prefixed RM attributes below), so no source value is dropped for those three; ADR 0016 decision 3 declines the `ctx/` sketches for them deliberately. Since 2026-08-05 (Phase C2) `participations` and `health_care_facility` round-trip under the underscore grammar too (`<root>/context/_participation:N\|*`, `<root>/context/_health_care_facility\|*`), so the composer `external_ref` is the only genuine loss left in this row — the ADR 0015 boundary, which also refuses the composer's `_identifier:N` and `/relationship` by name. **On encode that loss is a silent drop, not a refusal**, and deliberately so: a composer carrying an `external_ref` or `identifiers` is written as `ctx/composer_name` alone. Refusing would make every composition whose composer is properly referenced unencodable — the vendored `clinical_content_validation` body is one, and it would fail a green PROBE-076 leg — which is a worse answer than a registered projection loss in a format that is a lossy projection by design. This is the one exception carved out of wire.md's "encode MUST NOT silently lose an in-scope attribute"; closing it needs a `ctx/` or real-path channel, not a codec change. Note the fields that do NOT belong here: `category` is a template-constrained Web Template leaf and round-trips via its own path; a composer **name** round-trips via `ctx/composer_name`; `setting` left this row on 2026-08-05 (§ core supported above); template-constrained `other_context` content rides its Web Template paths, and does so end-to-end since `rmpath` resolves `EVENT_CONTEXT.other_context` (REQ-121) — before that the paths existed in the Web Template but the encoder dropped the data. | Deferred |
| Datatypes — **first-class** suffix form: `DV_TEXT`, `DV_CODED_TEXT`, `DV_DATE_TIME`, `DV_DATE`, `DV_TIME`, `DV_QUANTITY`, `DV_COUNT`, `DV_BOOLEAN`, `DV_DURATION`, `DV_URI`, `DV_EHR_URI`, `DV_ORDINAL`, `DV_PROPORTION`, `DV_IDENTIFIER`, `DV_PARSABLE`, `DV_MULTIMEDIA` (§ encapsulated leaves below), and `DV_INTERVAL<T>` (not a suffix set at all — the REQ-140 interval grammar). Any other `DV_*`, a decorated instance of the above, or a **substituted subtype** (the value's dynamic type differs from the leaf type, e.g. `DV_EHR_URI` at a `DV_URI` leaf), rides `\|raw` — with **one carve-out** (REQ-053, amended 2026-08-05): a **fully-captured `DV_CODED_TEXT` at a `DV_TEXT`-typed leaf** rides the DV_CODED_TEXT suffix set (`\|code` + `\|value` + `\|terminology`, `\|formatting` where populated; **no bare key**), matching the reference's `dv_coded_text_as_dv_text` corpus shape; on decode, `\|code` at a `DV_TEXT` leaf re-selects the DV_CODED_TEXT builder under that type's own rules (`\|value` required). A *decorated* coded text whose extras the suffixes cannot carry (`mappings`, a `preferred_term`, …) still rides `\|raw` stamped `DV_CODED_TEXT`, and every **other** substituted subtype keeps riding `\|raw`. This changed emitted bytes for the fully-captured substituted value only (previously a `\|raw` fragment). | Both directions. | landed (Task 6; substitution carve-out 2026-08-05) |
| Optional `DV_ORDERED` / `DV_QUANTIFIED` / `DV_AMOUNT` attributes — **first-class** suffix form: `\|magnitude_status`, `\|normal_status`, `\|accuracy`, `\|accuracy_is_percent`, `\|precision`, `\|units_system`, `\|units_display_name`, and `DV_TEXT`'s `\|formatting`. | Supported both directions. **This changed emitted bytes:** a value carrying one of these previously rode `\|raw` as a whole and now rides the suffix form. An *undecorated* value is byte-identical to before — an absent attribute writes no suffix. `\|normal_status` carries the bare ordinal code and decode rebuilds it in the implied `openehr` terminology, so a status coded elsewhere still rides `\|raw`. On decode the pass-through suffixes are checked for **JSON kind** (`\|accuracy` a number, `\|accuracy_is_percent` a boolean, …) so a malformed value is refused naming the offending FLAT key rather than escaping as a canonical-path error from `canjson`; that refusal deliberately carries **no** sentinel — it is a payload defect, not a modelled gap. The check admits everything `canjson` admits and so adds no strictness: a **quoted** number (`\|accuracy: "0.5"`) still decodes, since `canjson` parses a numeric string into `Real` / `Integer`. The RM's own value constraints (`\|magnitude_status` against its code set, …) are left to the validation package. | landed (PROBE-086 ratchet) |
| Underscore-prefixed RM attributes ([REQ-140](../../../docs/specifications/wire.md#req-140--underscore-prefixed-rm-attributes)) — **landed**: `_uid` (any LOCATABLE the Web Template **models** — the composition root, an ENTRY, a SECTION, a CLUSTER and a collapsed `ELEMENT` leaf; see § folded structural wrappers below for the ones it does not), `_link:N` (`\|meaning` + `\|type` + `\|target`), `_work_flow_id` / `_guideline_id` (OBJECT_REF: `\|id`, `\|id_scheme`, `\|namespace`, `\|type`), `context/_end_time`, `context/_location`; and the **value-decoration** families on a collapsed `ELEMENT` leaf — `_normal_range` (DV_INTERVAL of the leaf's anchor type: `/lower` + `/upper` in the anchor's own suffix form, `\|lower_included` / `\|upper_included` / `\|lower_unbounded` / `\|upper_unbounded`), `_other_reference_ranges:N` (REFERENCE_RANGE — that interval grammar with the RM's `range` level **elided**, plus `/meaning` as DV_TEXT or DV_CODED_TEXT), `_mapping:N` (TERM_MAPPING — `\|match`, `/target` CODE_PHRASE, optional `/purpose` DV_CODED_TEXT), `_null_flavour` and `_null_reason`. | Both directions, byte-exact. **This changed emitted bytes**: a composition carrying a populated `uid`, `links`, `workflow_id`, `guideline_id`, `end_time` or `location` now emits those keys where it previously emitted nothing at all. It is new coverage rather than a `\|raw` narrowing — none of these attributes ever had a FLAT spelling here — but any consumer diffing emitted FLAT will see the new keys (notably `<path>/_uid`, which `instance.Generate` stamps one on the composition root and on each ENTRY — `stampsUID` in `openehr/instance/locatable.go`, not on every archetyped node). Three subtype policies are load-bearing, and all three refuse rather than retype: (a) `_uid` is one bare string, so the concrete `UID_BASED_ID` is re-derived from the lexical form — three `::` parts with a valid `VERSION_TREE_ID` is `OBJECT_VERSION_ID`, anything else `HIER_OBJECT_ID` — and a value whose form implies the *other* subtype is `ErrUnsupportedDatatype`; (b) an OBJECT_REF's `\|id_scheme` is the `OBJECT_ID` discriminator (present ⇒ `GENERIC_ID`, absent ⇒ `HIER_OBJECT_ID`), every other subtype being scheme-less and therefore indistinguishable on the wire; (c) `\|meaning` / `\|type` on a LINK and the bare `context/_end_time` carry a plain undecorated value only, and these families have **no `\|raw` carrier**, so a coded LINK meaning or a decorated `DV_DATE_TIME` is a typed error. Owner admissibility comes from the RM itself (`rminfo`), not a hand table, which makes `_guideline_id` CARE_ENTRY-only — `ADMIN_ENTRY` carries `_work_flow_id` and no `_guideline_id`, exactly as the corpus writes it. A single-valued family admits `:0` beside the index-less spelling, because the OPT-free FLAT ↔ STRUCTURED interconversion re-spells every segment with an explicit index — as does a *sub-path* segment inside a family (`_normal_range/lower:0`), which the same interconversion produces. **The value-decoration families narrow the `\|raw` boundary and therefore change emitted bytes** (unlike C0's, which were new coverage): a value whose only extras are `normal_range`, `other_reference_ranges` or `mappings` now rides its suffix form **plus** `_` keys where it previously rode one whole `\|raw` fragment. `capturedKeysDecorated` derives that widening from `rminfo`, so the encode boundary and the decode router's admissibility rule are the same rule; a value with an extra still outside the grammar (a non-`openehr` `normal_status`, DV_TEXT's `language` / `encoding`) keeps riding `\|raw` **whole, with no `_` keys beside it** — never both spellings for one value. Which family a leaf admits comes from the **leaf datatype** (`normal_range` on every DV_ORDERED, `mappings` on DV_TEXT and its coded subtype), not from the ELEMENT: one FLAT path addresses both objects, and `_null_flavour` / `_null_reason` are the two that decorate the ELEMENT itself — which is what makes them legal beside an **absent** value (an ELEMENT with a null flavour and no value emits the `_null_flavour` keys and no bare key, and decodes back to exactly that). Nested values reuse the leaf machinery in both directions, so an interval bound, a `/meaning`, a `/target` or a `/purpose` the suffix set cannot capture rides `\|raw` at its own position rather than being refused. Two boundary rules are load-bearing: `\|*_unbounded` is emitted only when **true** and `\|*_included` only when **false** (see § interval boundary flags below), and `\|match` is validated lexically against `=` `<` `>` `?` on both sides because the RM types it as a bare Character. | landed (REQ-140 Phase C0 + C1, 2026-08-05) |
| Underscore-prefixed RM attributes — the **party** families ([REQ-140](../../../docs/specifications/wire.md#req-140--underscore-prefixed-rm-attributes)): `context/_health_care_facility`, `context/_participation:N`, the five ENTRY subtypes' `_other_participation:N`, and the nested `_identifier:N` (DV_IDENTIFIER) every party position carries. | Both directions, byte-exact, with **one implementation for every position the grammar reaches a party** — a family's whole value, a PARTICIPATION performer inlined on the same key base, the ENTRY `subject` leaf, and (Phase C3) a FEEDER_AUDIT_DETAILS party. Three discriminations are read off the keys: `/relationship` selects PARTY_RELATED, any other party key selects PARTY_IDENTIFIED, and **no** party key is PARTY_SELF — which is how the reference writes a record-subject performer (`\|function` + `\|mode` alone) and what keeps the `WithTemplate` PARTY_SELF `subject` default byte-identical on re-encode. `\|id_scheme` is the `external_ref`'s OBJECT_ID discriminator, the same policy the OBJECT_REF families use (present ⇒ GENERIC_ID, absent ⇒ HIER_OBJECT_ID; note the suffix is `\|id_namespace` here where those spell `\|namespace`). A PARTICIPATION spells its performer's identifiers as the reference's **inlined** `\|identifiers_<field>:N` and a standalone party as the nested `_identifier:N`; each position accepts only its own, so the nested spelling on a participation is refused naming the key. **This changed emitted bytes**: a composition carrying a `health_care_facility`, `participations`, `other_participations` or an ENTRY `subject` now emits those keys where it previously emitted nothing. The party grammar has no `\|raw` fallback, so every shape the suffixes cannot express is a typed error: a PARTICIPATION `time` (no channel in the reference's suffix set, corpus-unexercised — **deferred**); a coded or decorated `\|function`; and, from the PR #89 review, a PARTY_IDENTIFIED carrying none of `name` / `identifiers` / `external_ref` or carrying an empty `name` (RM invariants `Basic_validity` / `Name_valid` — it would write no key, and no party key is how this grammar spells PARTY_SELF), plus a nil RM-mandatory `PARTICIPATION.performer` for the same reason. Decode refuses all four shapes symmetrically. | landed (REQ-140 Phase C2, 2026-08-05) |
| The party `external_ref`'s `type` — `\|type`, emitted **only** when it differs from the reference's hardcoded `PARTY`. | The reference writes no `\|type` at a party and hardcodes `PARTY`, which is lossy: the vendored `clinical_content_validation` composition carries `PERSON` and `ORGANISATION` references at its composer, health-care facility and every participation performer. So absent decodes as `PARTY` — every corpus body round-trips byte-exactly and never gains the key — and encode spells `\|type` where the value differs, rather than normalising it away (a silent loss) or refusing it (which would have failed a green PROBE-076 leg). The suffix is not invented vocabulary: `\|type` is the reference's own spelling of the same RM attribute in the OBJECT_REF families (`_work_flow_id\|type`), written at one further position. An **empty** `type` is `ErrUnsupportedDatatype`, not a defaulted one. | landed (REQ-140 Phase C2, 2026-08-05) |
| **Vendored vocabulary** — the openEHR Terminology `participation mode` group (codes 193–224, code ↔ rubric), in `rmattr_party.go`. | The only terminology this codec carries, and it is carried because the reference gives `\|mode` **no code channel**: it writes the bare rubric (`"face-to-face communication"`) where PARTICIPATION.mode is a DV_CODED_TEXT whose `defining_code` is RM-mandatory, so rebuilding the code is the only alternative to fabricating an empty CODE_PHRASE. The group is small, closed and versioned with the openEHR specifications — not a runtime terminology lookup. A rubric outside it is refused loudly; encode is the exact inverse and refuses a mode whose code, terminology (`openehr` is implied) or value that rebuild would not reproduce, plus any DV_CODED_TEXT decoration the bare key cannot hold. `TestParticipationModeVocabularyIsInvertible` pins that the rubrics stay distinct, since decode reads the table backwards. | landed (REQ-140 Phase C2, 2026-08-05) |
| **Implied terminology** — `IANA_media-types` for `DV_MULTIMEDIA.media_type`, in `datatypes.go`. | Not a vocabulary but a single code-set **identifier**, implied because `\|mediatype` carries the code alone. It is not invented: the RM names the openEHR `media types` code set for the attribute, and this SDK already pins that identifier in the template-instance writer (`rmwrite`, REQ-107). The implication is safe only because encode refuses to take the suffix form for a `media_type` coded anywhere else — that value rides `\|raw` whole rather than being re-terminologised — which is the `\|normal_status` rule one attribute up; an **absent** terminology contradicts nothing the implication asserts and survives the suffix form, as it does for `\|normal_status`. `\|integrity_check_algorithm` and `\|compression_algorithm` deliberately get **no** implication: their openEHR code sets have no identifier this codec can source, so decode writes a code-only CODE_PHRASE and a value carrying a terminology rides `\|raw`. Fabricating two identifiers to make the three attributes look uniform would have put terminologies on the RM that no source states. | landed (REQ-140 Phase C3, 2026-08-05) |
| Underscore-prefixed RM attributes — the **DataValue-member** families ([REQ-140](../../../docs/specifications/wire.md#req-140--underscore-prefixed-rm-attributes)): `_charset`, `_language`, `_encoding` (CODE_PHRASE), `_thumbnail` (a nested DV_MULTIMEDIA) and `_accuracy` (a bare DV_DURATION). | Both directions, byte-exact. All five are ordinary **value** families whose admissibility is read off the RM (`rminfo`): `charset` reaches the two DV_ENCAPSULATED subtypes, `language` those two **and** DV_TEXT with its coded subtype, `encoding` only DV_TEXT, `thumbnail` only DV_MULTIMEDIA. `_accuracy` needs one extra gate — DV_TEMPORAL redefines `accuracy` as a DV_DURATION **object** where DV_AMOUNT declares a `Real`, and the `Real` form already has the scalar `\|accuracy` suffix, so the family carries a declared-type predicate (`rmattrFamily.attrType`) and reaches DV_DATE / DV_DATE_TIME / DV_TIME and nothing else. One attribute therefore never has two channels at one owner. Each member goes out through its own datatype's **leaf** emitter, so the `\|raw` carrier behind that key takes anything the suffixes cannot hold (a decorated `TERMINOLOGY_ID` → `_charset\|raw`; a thumbnail carrying its own charset → `_thumbnail\|raw`, which is why the family needs no recursive sub-path spelling and the corpus never writes one). A member whose `code_string` is **empty** is a typed error rather than a silent skip: these are pointer fields, so "present but blank" is distinguishable from absent, and `codePhraseToFlat` writes nothing for an empty code. **This narrows the `\|raw` boundary**: a DV_TEXT or DV_CODED_TEXT carrying a `language` / `encoding`, and a temporal carrying its DV_DURATION `accuracy`, now ride suffixes plus `_` keys where they rode one whole `\|raw` fragment; `hyperlink` is what still forces it for a text value. | landed (REQ-140 Phase C3, 2026-08-05) |
| The **encapsulated** leaves — `DV_PARSABLE` (bare value + `\|formalism`) and `DV_MULTIMEDIA` (bare **uri** + `\|mediatype` + `\|size` + `\|data` + `\|alternatetext` + `\|integrity_check` + `\|integrity_check_algorithm` + `\|compression_algorithm`). | Both directions, byte-exact, at a Web Template leaf and at every nested position the underscore grammar reaches one (`_thumbnail`, `original_content`, `original_content_multimedia`). Four corpus findings are load-bearing and all four correct an earlier guess: (a) DV_MULTIMEDIA's **bare key is the `uri`**, not the inline data — the octets ride `\|data`, and the corpus `_thumbnail` spells `\|data` with no bare key at all; (b) the reference writes `\|mediatype` and `\|alternatetext` **without** the underscore the RM attribute has; (c) `\|data` and `\|integrity_check` carry the **base64** of the RM's `Byte[]`, which is what the canonical JSON form uses, so the wire value round-trips through `[]byte` unchanged; (d) the three CODE_PHRASE-valued attributes travel as a **bare code** — `media_type` in the implied `IANA_media-types` (§ vendored vocabularies), the two algorithms with **no** implied terminology at all — and a value that rebuild would re-terminologise, or one carrying a `preferred_term`, rides `\|raw` **whole**. Both types' `charset` / `language` ride the `_charset` / `_language` members above, so a DV_PARSABLE at a leaf is now **always** fully captured and never rides `\|raw`; at a *nested* position (`original_content`) those members have no key, so a charset-carrying value rides `original_content\|raw` — lossless, and the shape the corpus never writes stays unwritten. `media_type` and `size` are RM-mandatory and required on decode, so a half-spelled multimedia is refused rather than zero-filled. **This changed emitted bytes** for `ACTIVITY.timing`, which rode `\|raw` and now rides its two suffixes. | landed (REQ-140 Phase C3, 2026-08-05) |
| The `DV_INTERVAL<T>` leaf — the REQ-140 interval grammar at a template-modelled node: `/lower` + `/upper` in the bound datatype's own suffix form, plus the four boundary Booleans. | Both directions, byte-exact, sharing C1's one implementation (`intervalSuffixes` / `intervalToFlat`) with `_normal_range` and `_other_reference_ranges:N`. The **anchor** — the datatype the bounds are spelled with — comes from the Web Template leaf type, which names it inside the angle brackets (`DV_INTERVAL<DV_QUANTITY>`); a bare, unparameterised `DV_INTERVAL` names none, so it is not treated as an interval leaf and rides `\|raw` rather than being mis-spelled. Decode siphons the leaf's keys before the ordinary leaf loop (`compositeLeafGroups`, generalised from C2's party-leaf siphon), because `/lower` and `/upper` are not Web Template children and would otherwise fail as unresolvable paths. Encode enumerates the generated `DVInterval[T]` instantiations explicitly (no reflection, REQ-024) — including the abstract `DVInterval[DVOrdered]` a canonical decode produces. | landed (REQ-140 Phase C3, 2026-08-05) |
| Underscore-prefixed RM attributes — the **`_feeder_audit`** family ([REQ-140](../../../docs/specifications/wire.md#req-140--underscore-prefixed-rm-attributes)), on any LOCATABLE (the corpus writes it at nine positions across 14 of its 34 bodies): `originating_system_item_id:N` / `feeder_system_item_id:N` (DV_IDENTIFIER lists — the FLAT segment is **singular** where the RM attribute is plural), `originating_system_audit` (RM-mandatory) / `feeder_system_audit` (FEEDER_AUDIT_DETAILS: `\|system_id` mandatory, `\|version_id` and `\|time` optional; `/location`, `/subject`, `/provider`), and `original_content` **or** `original_content_multimedia`. | Both directions, byte-exact. It is the deepest family in the grammar and the only one whose tails nest **three** levels (`…/originating_system_audit/provider/_identifier:0\|id`), which the C1 tail machinery cannot express — it flattens a multi-segment sub-path and admits a `:index` only on the leading segment. Rather than widen that, `rmattrChildGroups` re-roots each sub-path segment into its own group, so every level is decoded by the machinery that already owns it: a party by C2's `partyLeafSuffixes` (nested `_identifier:N` and PARTY_RELATED `/relationship` included), an item id and an `original_content` by the datatype leaf builder. The **DV_ENCAPSULATED choice is by key name** in both directions — `original_content` is the DV_PARSABLE, `original_content_multimedia` the DV_MULTIMEDIA — and a body carrying both spellings of the one RM attribute is a typed error, not a silent pick. FEEDER_AUDIT has no scalar attribute at all, so a bare value or a `\|suffix` on the family itself is refused; an absent `originating_system_audit` is refused rather than defaulted (fabricating one would put an empty `system_id` on the wire), and so is an empty `system_id` on encode. **This changed emitted bytes**: a composition carrying a `feeder_audit` anywhere now emits those keys where it previously emitted nothing. | landed (REQ-140 Phase C3, 2026-08-05) |
| PARTY_SELF at an **RM-optional** PARTY_PROXY position — `\|_type: "PARTY_SELF"` at FEEDER_AUDIT_DETAILS' `subject` and ENTRY's `_provider`. | Both directions. PARTY_SELF is spelled by the **absence** of every party key wherever that absence is unambiguous — a PARTICIPATION performer and an ENTRY `subject`, both RM-mandatory. At an RM-**optional** PARTY_PROXY absence already means *absent*, so the reference writes the discriminator explicitly (`ehrbase_conformance_party_self` carries `…/originating_system_audit/subject\|_type: "PARTY_SELF"` and no other key there) and this codec accepts and emits it at exactly those two positions. The suffix is exclusive: any other `\|_type` value is refused (every other subtype is implied by the party keys themselves, and re-emitting a redundant discriminator would put a key on the wire the reference never writes), as is a `\|_type` standing beside party keys that contradict it, and a PARTY_SELF carrying an `external_ref` — which the suffix set cannot spell beside the discriminator. | landed (REQ-140 Phase C3, 2026-08-05) |
| Underscore-prefixed RM attributes — **rest deferred**: `_instruction_details` (ACTION) and `_wf_definition` (INSTRUCTION), plus FEEDER_AUDIT_DETAILS' `other_details` (an ITEM_STRUCTURE) — all three named by the spec and **unexercised by the pinned corpus** as far as a *shape* goes (the first two appear as keys; no fixture supplies a decodable value grammar for any of them). | Refused loudly on both sides and therefore visible in the PROBE-086 census: the two families through the router's unknown-family arm, `other_details` through a typed refusal naming the key and the deferral. `ism_transition/_reason:N` is **not** in this row — the whole `ism_transition` node is absent from this SDK's WebTemplate projection, so nothing under it resolves and closing it is a builder question, not a grammar one (see `webtemplate/deviations.md` § inContext coverage). **`_identifier:N` is deliberately not a router family**: every position that reaches a party reaches its identifier list through the party grammar, so `<entry>/_identifier:N` names no family and stays `ErrUnknownPath`. Since Phase C3 every other underscore family the corpus writes is carried. | Deferred |
| `\|raw` escape hatch (canonical fragment for exotic/decorated datatypes) | Supported both directions: encode emits `\|raw` for non-core or decorated `DV_*`; decode accepts a `\|raw` fragment that carries a string `_type` and is not combined with any other suffix; encode stamps the fragment with the value's **dynamic** type when it can classify it. On decode, `\|raw` is **not** checked for RM-type compatibility with the leaf constraint (an explicit bypass) — a documented relaxation. | landed (Task 6) |
| `\|other` open-value-set free text for `DV_CODED_TEXT` | Supported: an **undecorated** `DV_TEXT` at a `DV_CODED_TEXT` leaf whose Web Template input is `listOpen` encodes to `\|other`; decode maps `\|other` back to `DV_TEXT`, requiring `listOpen` and rejecting `\|other` combined with **any** other suffix. `\|other` carries the value alone, so a decorated `DV_TEXT` (a `formatting`, a `hyperlink`, …) is not expressible in this form and rides `\|raw` instead. | landed (Task 6) |
| `.schema`-suffixed media types on input | Not accepted. (Canonical types only; see [simplified.go](simplified.go).) | Deferred |
| `CODE_PHRASE` leaves (ENTRY `language` / `encoding`, and REQ-140's `_charset` / `_language` / `_encoding` members) — the reference emits these as leaves in their own right, under the `\|code` + `\|terminology` pair a `DV_CODED_TEXT`'s `defining_code` uses, **plus** a `\|preferred_term` that nested spelling has no channel for. | Supported both directions. The `\|preferred_term` suffix landed with REQ-140 Phase C3, because the corpus writes it at `dv_text/_language\|preferred_term`; **this narrows the `\|raw` boundary** for a standalone CODE_PHRASE, and the asymmetry with the nested form is deliberate — a `defining_code` carrying a `preferred_term` still rides `\|raw`, since `\|code`+`\|value`+`\|terminology` has nowhere to put it. The **all-zero** value writes nothing (the field is non-pointer at the ENTRY leaves, so "unset" and "zero" coincide and an unconditional emit would put blank leaves on every `ctx/`-decoded composition); a decorated `TERMINOLOGY_ID` rides `\|raw`, and it is the only shape that still does. A **partly-populated** value — an empty `code_string` beside a non-empty `TERMINOLOGY_ID` or a `preferred_term` — also rides `\|raw`: the empty-code skip would otherwise drop it silently, so it is deliberately not treated as captured (PR #86 review round 3). At the REQ-140 *member* positions the field is a **pointer**, so an empty code there is a typed error instead (§ DataValue-member families). | landed (PROBE-086 ratchet; `\|preferred_term` REQ-140 Phase C3) |
| ENTRY `subject` (PARTY_PROXY) | **Landed 2026-08-05** (REQ-053 amendment, on REQ-140's party grammar). Both directions, byte-exact: PARTY_SELF is spelled by the **absence** of every party key — which is what keeps the `WithTemplate` PARTY_SELF completion byte-identical on re-encode — PARTY_IDENTIFIED emits `\|id` / `\|id_scheme` / `\|id_namespace` / `\|name` plus the nested `_identifier:N`, and PARTY_RELATED adds `/relationship`. Decode siphons the whole leaf as one party group before the `_` router, because those three key shapes address one RM value and the concrete subtype is only known once all three are in hand. `rmpath` resolves `subject` on the five ENTRY subtypes (REQ-121) and the `TestInContextLeavesResolveViaRmpath` exemptions are gone. | landed (REQ-140 Phase C2, 2026-08-05) |
| Other non-`DV_` leaves — `STRING` (ACTIVITY `action_archetype_id`) on encode | Skipped on encode (source value dropped). The last member of this class; `CODE_PHRASE` left it with the PROBE-086 ratchet and `PARTY_PROXY` with REQ-140 Phase C2. | Deferred |

## Deviations

- **`LOCATABLE.name` on decode** — the FLAT/STRUCTURED formats do not carry names, and the
  Web Template collapses the HISTORY / ITEM_STRUCTURE wrappers, so decode cannot name every
  node from the WT alone. Passing [`WithTemplate(compiled)`](simplified.go) repopulates the
  mandatory `name` on every reconstructed node from the archetype terminology (keyed by the
  compiled aqlPath); without it, nodes are unnamed and the round-trip is merely
  **format-idempotent**. Names never leak into FLAT, so idempotence is preserved either way.

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
  re-encode: names never leak into FLAT, and origin/encoding have no emitted
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
  **minus that instance** — the one place encode narrows the composition rather than
  erroring, because the format simply has no spelling for "an instance with no values".

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

- **Interval boundary flags are asymmetric** (REQ-140) — the four DV_INTERVAL Booleans are
  written only where they carry information their default does not, in both directions.
  `\|lower_unbounded` / `\|upper_unbounded` are RM-mandatory Booleans whose `false` the
  reference omits: absent decodes as `false`, and only `true` is emitted. `\|lower_included`
  / `\|upper_included` are RM-**optional** (`Interval` declares them `0..1`) against the
  SDK's generated mandatory Boolean, so the codec has to fix a mapping for "absent": it is
  the **closed** endpoint, `true`, and only `false` is emitted. An absent bound is the
  unbounded end and is never emitted as a zero-valued one. Consequence, deliberate: a
  redundant `\|lower_included: true` on input is **normalised away** on re-encode — it
  denotes the same RM value as its absence, so this is a canonical-spelling normalisation
  like the `:0` one above, not a loss. The rule is corpus-derived: `dv_count` omits both
  flags on a bounded interval, `dv_quantity` spells both `false`, and `dv_ordinal`'s
  unbounded end pairs `\|upper_unbounded: true` with `\|upper_included: false` — this is the
  only mapping under which all three round-trip byte-exactly (wire.md § REQ-140).

## Conformance

**PROBE-076** (landed) exercises the codec over the vendored EHRbase `Test_dv_*` corpus
(OPT + canonical composition) — 24 pass, 1 skip. It asserts **round-trip idempotence**
(FLAT/STRUCTURED/interconversion) **and OPT-conformance**: when the source composition is
itself OPT-valid, a `WithTemplate` decode must also validate against the OPT. The conformance
leg catches dropped/mistyped leaves that idempotence alone (a symmetric omission) would miss.
It does **not** compare emitted FLAT/STRUCTURED against vendored upstream simplified
output — that comparison is **PROBE-086** (Implemented, Sandbox): its harness at
`testkit/conformance/webtemplate/` round-trips the corpus vendored at
`testkit/cassettes/flat-conformance/` over the modelled subset, with the refusal
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
wire.md § REQ-053 records it as such.

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
output unencodable. The same trade as the § composer boundary, for the same reason.

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
