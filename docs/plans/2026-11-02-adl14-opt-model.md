---
kind: plan
---

# Plan: ADL 1.4 operational template model (REQ-176)

**Date:** 2026-11-02
**Status:** Draft, ready for `/sdd-deliver REQ-176` (one pull request, branch `feat/opt-model-req-176`)
**Covers:** [REQ-176](../specifications/clinical-modeling.md#req-176--adl-14-operational-template-model); [REQ-041 § SDK-authored root schemas](../specifications/bmm-conformance.md#sdk-authored-root-schemas); [REQ-045](../specifications/bmm-conformance.md#req-045--bmm-loader-is-a-building-block) (the include merge); [REQ-056](../specifications/wire.md#req-056), [REQ-052](../specifications/wire.md#req-052), [REQ-170](../specifications/wire.md#req-170--unmatched-content-on-decode)
**Probes:** [PROBE-111](../specifications/conformance.md#probe-111--operational-template-corpus-round-trip)
**Depends on:** PR 360 (Template.xsd pinned, ADR 0034, REQ-176 written), merged as `acf9a9ca`
**Defers:** how `template.ParseOPT` relates to the model ([STRAND-17](../specifications/research-strands.md#strand-17--the-opt-parser-and-the-generated-operational-template-model)); a depth or size bound on canonical-XML decode of an untrusted OPT (REQ-108 bounds only `template.ParseOPT`); ADL 2 and AOM 2

This header is for the reader. No tool reads it, and nothing fails when it is missing or out of date. The work itself meets the [Definition of Ready](../development-process.md#definition-of-ready) before it starts and the [Definition of Done](../development-process.md#definition-of-done) in the implementing PR.

## Goal

A consumer decodes any ADL 1.4 OPT the corpus holds into generated `aom14` types, with no unmatched content, and encodes it back to a document that validates against the pinned AM Release-1.4 schemas. The types come from SDK-authored root schemas over the pinned openEHR BMM files ([ADR 0034](../adr/0034-sdk-authored-root-schemas.md)); the generator stays generic over a BMM and the ITS-XML schemas. PROBE-111 proves it across the whole corpus and adds an XML schema check to `make ci`.

## Decisions taken (maintainer, 2026-11-02)

- **One pull request** for all of REQ-176. The pass budget of `/sdd-review` applies to the whole branch.
- **Round-trip comparison is normalised**: a `Hash` group's entries compared as a set, numbers by value, an absent interval flag as its [REQ-169](../specifications/wire.md#req-169--interval-boundary-flag-defaults) default, the root's `xsi:type` set aside. The encoder writes map entries sorted by key. No ordered-map type.
- **`timezone_validity`** decodes the words, `disallowed` and the codes, and encodes the integer code 1001 to 1003: a live upload test found FerroEHR 5.5.0 refuses the words and EHRbase 2.36.0 accepts both.
- **What P_BMM cannot say goes into SDK extension keys in the root JSON**, each entry with its reason and upstream reference, read by the generator: classes left out of a target, type aliases, dropped elements, names for anonymous schema types, the schema forms P_BMM has no construct for. No Go table names a class.
- **RM resource classes in canonical XML take the AM Release-1.4 shape everywhere**: no `id` attribute, keyed by language.
- **The loader's include merge resolves a diamond**: when two includes meet again and one form comes from a schema that descends from the other's source (RM includes BASE), the descendant's form wins; `ErrSchemaConflict` stays for a real sibling conflict. This changes `openehr/bmm` behaviour and amends ADR 0002 D2.
- **Members typed `C_COMPLEX_OBJECT` become an interface** once `C_ARCHETYPE_ROOT` and `T_COMPLEX_OBJECT` subclass it. Accepted under the pre-1.0 policy; the CHANGELOG names it under Breaking.

## What the research found

Read before dispatch; every task's `Code map` starts from here.

- **Targets and loading.** `Target` (`internal/bmmgen/target.go:13-93`) carries a fixed `RootID` and a hand-written `SourceLabel`; the `// Source:` header comes from `render.go:20-25` and five other renderers, in 189 checked-in files. Includes resolve from one directory (`openehr/bmm/resolver.go:32-45`); `mergeAncestor` (`openehr/bmm/loadall.go:128-192`) lets a descendant's class shadow an ancestor's and fails two differing siblings with `ErrSchemaConflict` (`:164-172`). Nothing leaves an included class out (`internal/bmmgen/primitives.go:99-161` are name tables). About 70 test resolvers in `internal/bmmgen` use `bmm.FSResolver{Root: testResources}` (`plan_test.go:11`); PROBE-094 loads the pinned RM (`openehr/rm/rminfo/probe_094_test.go:129`, `nonempty_test.go:22`). Verify mode never reports an orphaned `*_gen.go` (`generate.go:222-449`).
- **No BMM checker in the tree.** `make bmm-check` is a sha256 check of the pinned files (`scripts/sync-bmm.sh:225-288`); the openEHR checker script lives outside the repository.
- **XSD reading** (`internal/bmmgen/itsxml.go`) keeps only the base and the particle of named top-level types and drops every `xs:attribute`, `simpleContent` and `fixed` value (`:186-216`). The attribute rule today is a name check, `isXMLAttributeProperty` (`render_xmlmar.go:226-228`, used at `:146`, `render_xmlunmar.go:188` and `:766`), and the generated attribute code assumes a plain string field (`render_xmlmar.go:234-238`, `render_xmlunmar.go:724-728`). Template.xsd's `T_VIEW` uses anonymous types (`Template.xsd:70-88`).
- **Hash, Any, polymorphism in canonical XML.** A `Hash` member goes to `encoding/xml`, which refuses a map, so encoding any value holding one fails; decode skips it (`render_xmlmar.go:314-320`, `render_xmlunmar.go:392-393,428-429,907-911`); the comment at `render_xmlmar.go:48-57` is stale. An `Any` member encodes with no `xsi:type` and decodes as skipped (`render_xmlunmar.go:301-305,898-902`). `xsi:type` dispatch strips only a literal `xsd:` (`openehr/serialize/canxml/decode.go:292-333`) and looks up the shared `typereg.Default`, which panics on a duplicate (`openehr/rm/typereg/registry.go:113-127`) and whose census test needs `_type` to equal the registered name (`canjson/type_census_test.go:24-38`). The encoder names the root after `BMMName()` (`canxml/marshal.go:130-149`); the schema's global element is `template` (`Template.xsd:10`).
- **Schema against BMM, AM Release-1.4.** Template.xsd types, all absent from the BMM: `OPERATIONAL_TEMPLATE` (extends nothing; `language`, not `original_language`; no `translations`; `uid`, `template_id`, `concept`, `definition: C_ARCHETYPE_ROOT`, `ontology`, `component_ontologies`, `annotations`, `constraints`, `view`), `C_ARCHETYPE_ROOT`, `FLAT_ARCHETYPE_ONTOLOGY` (`@archetype_id`), `ANNOTATION` (`@path`, `items`), `T_VIEW` (anonymous `constraints` and `items`), `T_CONSTRAINT`, `T_ATTRIBUTE` (not a `C_ATTRIBUTE`), `T_COMPLEX_OBJECT`, `C_CODE_REFERENCE` (`referenceSetUri`). AOM 1.4 classes the schema shapes otherwise: the three names `C_CODE_PHRASE`, `C_DV_ORDINAL`, `C_DV_QUANTITY`; `C_CODE_PHRASE` (`terminology_id`, no `reference`, typed `assumed_value`); `C_DV_QUANTITY.property: CODE_PHRASE`; `C_QUANTITY_ITEM` (`precision`, `magnitude` optional, `units` required); `ORDINAL` (the `DvOrderedElements` group, `symbol` as a choice of two shapes); `C_ATTRIBUTE.match_negated` at two positions; `C_DEFINED_OBJECT.assumed_value` typed per subclass; `EXPR_OPERATOR.precedence_overridden` required; `OPERATOR_KIND` an integer enumeration (the corpus writes `2007`); `C_STRING.list_open` optional; `C_DATE`, `C_TIME`, `C_DATE_TIME`, `C_DURATION` (`pattern`, `timezone_validity`) against the BMM's validity flags; `ARCHETYPE_ONTOLOGY` (`CodeDefinitionSet@language`, `TermBindingSet@terminology`, `TERM_BINDING_ITEM@code`, the constraint binding sets); `ARCHETYPE_TERM` (`@code`, `items: StringDictionaryItem+`); `C_DV_SCALE` and `C_DV_STATE` in the schema only; `C_REFERENCE_OBJECT` in the BMM only. XML attributes in use: `@code`, `@language`, `@terminology`, `@id`, `@archetype_id`, `@path`, `archetype_node_id`.
- **RM resource classes.** `rm.ResourceDescription`, `ResourceDescriptionItem` and `TranslationDetails` hold `map[string]…` fields (`openehr/rm/common_resource_gen.go:14-60`); `aom14.Archetype` carries the BASE form (`archetype_gen.go:15,31,35`). BASE Release-1.2.0 keys two of them by `@id` (`resources/its-xml/components/BASE/Release-1.2.0/Resource.xsd:44,76`), AM Release-1.4 by language with no `id`.
- **Corpus.** 74 `.opt` files under `testkit/corpus/` (62 templates, 7 crossformat, 3 webtemplate, 1 flat-conformance, 1 its_rest); `fixtures.ListAllOPTs` (`testkit/fixtures/paths.go:156-205`) skips the 7 crossformat ones. `match_negated` in 43 files, all `false`; `timezone_validity` only in `sdk_crosscheck_values.v1.opt`, as codes; no `T_ARCHETYPE_ROOT` (`testkit/probes/instance/t_archetype_root_test.go:40` writes one); `EXPR_LEAF.item` as `C_STRING` (219), `xs:string` with a local `xmlns:xs` (90), `xsd:string` (128), untyped (1, `TestPerson.v2.opt`); `assumed_value` 7, all `C_CODE_PHRASE`; `precision` 24 in 5 files; `C_CODE_REFERENCE` 28 in 10 files; `view` and `constraints` in 11 and 10 files; one root `xsi:type="OPERATIONAL_TEMPLATE"` (`social.opt`); `ORDINAL.symbol` in the `DV_CODED_TEXT` shape 57 times in 6 files; no `translations`.
- **Schema validation.** No `xmllint` on the host or in the dev image (`Dockerfile:55`), and no validation target or CI step. The pinned XSDs name `v2` only in `xmlns` and `targetNamespace` and use relative includes, so a temporary copy with `v2` rewritten to `v1` validates the documents as written.

## Tasks

Sequential, on the branch, in the main checkout, one `sdd-implementer` per task, briefs begun with `sdd-pr brief --task`. Workers load `go-coding:go-coding` and the focused skills for their diff. Each task's verification is its own package tests plus `make codegen-check` where it regenerates; `make ci` runs after the last task.

### Task 1: specification amendments for the decisions

Edit in `/sdd-specify`'s form, adding no keyword for an internal, and cite rather than restate:

- `clinical-modeling.md` § REQ-176: the intro's `OPERATIONAL_TEMPLATE` sentence (it declares its own members as Template.xsd does, not the `AUTHORED_RESOURCE` ones); § Round trip, the normalised comparison and the sorted map entries; the resource classes keyed by language with no `id`; the root element named after the schema's global element; § Shapes, `timezone_validity` written as the integer code, `EXPR_LEAF.item`'s prefix wording made consistent with the `wire.md` change below.
- `conformance.md` § PROBE-111: every `.opt` under `testkit/corpus/`, not the fixtures list; the normalised comparison; the rewrites the shapes section now names; the name of the schema validation target.
- `wire.md` § REQ-056, the `xsi:type` bullet (`:229`): a value whose prefix is bound to the XML Schema namespace names an XML Schema built-in and reads as its BMM primitive; any other prefixed value is still refused.
- `bmm-conformance.md` § SDK-authored root schemas: what a root declares gains the SDK extension keys and their entries' reason and reference; the RM root's id; the *Gate* bullet names the Go test of Task 3 as the root check. § REQ-045, or wherever the merge rule is stated: the diamond rule.
- ADR 0034: a dated amendment for the extension keys, the diamond rule, the normalised comparison and the integer code. ADR 0002: D2 amended for the diamond. `docs/adr/README.md` if the index shows amendments.
- REQ-176's map row: `implementation: in_progress`.

Verify: `make spec-check`.

### Task 2: the loader resolves an include diamond

`openehr/bmm/loadall.go`: track each merged class's source schema; when two includes disagree and one source descends from the other, keep the descendant's form. Tests first, in the external package: a diamond resolves to the descendant form; two unrelated siblings with differing classes still fail with `ErrSchemaConflict`; the existing merge tests stay green. Cite REQ-045.

Verify: `go test ./openehr/bmm/... -count=1`.

### Task 3: the generator loads SDK-authored roots; the RM root

- A resolver over `resources/bmm/sdk/` first and `resources/bmm/` second that refuses an id present in both; `Target.RootID` names the root; the `// Source:` header and `SourceLabel` come from the include chain (`render_inventory.go:124-152` already walks it).
- The RM root under `resources/bmm/sdk/`: includes `openehr_rm_1.2.0`, keeps `rm_release` 1.2.0 (`rm.Release` reads it, `render_release.go:23-38`), redeclares `TRANSLATION_DETAILS` with `accreditation` and its reason and upstream reference ([specifications-RM PR 43](https://github.com/openEHR/specifications-RM/pull/43)). Its id differs from every pinned id.
- A test helper for the ~70 test resolvers; PROBE-094 and `nonempty_test.go` load the RM root.
- The root check, a Go test in `internal/bmmgen`: every type name resolves after the merge; every class and property a root declares carries its reason and upstream reference; each shadow still differs from the class it shadows (so a re-pin that lands the fix fails until the shadow goes); no root id collides with a pinned one.
- `scripts/sync-bmm.sh`, `resources/bmm/README.md` and the [ADR 0001 runbook](../adr/0001-bmm-version-bump-runbook.md): a re-pin step for the roots.

Regenerate (`make codegen`); `rm.TranslationDetails.Accreditation` replaces `Accreditaton`. Verify: `make codegen-check`, `go test ./internal/bmmgen/... ./openehr/rm/... -count=1`.

### Task 4: the generator reads the SDK extension keys

Parsed from the root JSON beside the public loader, so `openehr/bmm` stays a P_BMM reader. One key per kind the decisions list: classes a target leaves out (the three BMM-named AOM 1.4 classes leave `typereg`, `jsonhooks` and the generated files); type aliases a decoder accepts (`T_ARCHETYPE_ROOT`), consumed by canonical-XML dispatch and not registered in `typereg`, so the census holds; dropped elements (`match_negated`: text `false` dropped in every mode, other text refused with `canxml.ErrInvalidShape`, never written); names for anonymous schema types; the schema forms P_BMM cannot express that Task 6 needs (the `ORDINAL_SYMBOL` choice). Key names are the implementer's to propose; each entry carries a reason and an upstream reference, and the root check of Task 3 holds them to that. A test per key, each failing without its rule.

### Task 5: XML attributes read from the schemas (ADR 0002 D10)

`itsxml.go` reads `xs:attribute` on a type, in `complexContent/extension` and `simpleContent/extension`, inherited through the base chain; the generator takes the attribute set from there and `isXMLAttributeProperty` goes. Attribute members that are not a plain string get a rule or a refusal at generation time. An attribute that names no BMM member is the key of the `Hash` holding the class (Task 7). Verify that RM output is unchanged apart from what the schemas say (`archetype_node_id` stays an attribute): `make codegen-check` after `make codegen`, `go test ./internal/bmmgen/... -count=1`.

### Task 6: the AOM 1.4 root, `openehr_opt_1.4.0`

Under `resources/bmm/sdk/`, including `openehr_am_1.4.0` and the RM root (the diamond of Task 2 gives the RM `common.resource` forms). Declares the Template.xsd classes and each AOM 1.4 class the schema shapes otherwise, listed under *What the research found*, in the schema type's form, each with its reason and upstream reference (the schema file and line, or the openEHR ticket or pull request). `OPERATOR_KIND` and the `timezone_validity` type as `P_BMM_ENUMERATION_INTEGER` (the loader reads it, `openehr/bmm/class.go:110-120`); the generator gains one generic rule for an integer enumeration: decode from its value or its item name, encode the value; `disallowed` reads as 1003. `C_COMPLEX_OBJECT` members become an interface. Regenerate; rename `C_CODED_TEXT` and the other two in the tests that name them (`render_xmlunmar_unmatched_test.go:139,295`, `aom14/coded_text_code_list_xml_test.go`, `aom14/typereg_smoke_test.go:24`). This task verifies and closes the two [backlog](../backlog.md) leads under *AOM 1.4 canonical XML*.

Verify: `make codegen-check`, `go test ./openehr/aom/... ./internal/bmmgen/... -count=1`.

### Task 7: `Hash` members in canonical XML

In the generated marshal and unmarshal and `openehr/internal/xmlunmatched`: a `Hash<String,String>` as one `StringDictionaryItem` per entry, key in `id`, value as text; a `Hash` of class values as one element per value, keyed by the value's `id` attribute where the schema declares one, else by its `language` code, with no `id` written for the resource classes; entries written sorted by key; decode takes the key attribute off before decoding the value, so it is not reported. The stale comments (`render_xmlmar.go:48-57`, `canxml/doc.go:263-281`) and the `wire.md` sentences that say the codec skips `Hash` members (§ REQ-052's absent-member bullet, § REQ-170) are brought up to date. Tests: `ARCHETYPE_TERM.items`, `RESOURCE_DESCRIPTION` maps, `other_details`, round trip and decode report.

### Task 8: `Any` members and the remaining shapes

- `EXPR_LEAF.item`: dispatch on `xsi:type` to a registered class, an XML Schema built-in under any prefix bound to that namespace (`String`), or, with none, its text as `String`; the value remembers the form it was read in, and the encoder writes that form, declaring the prefix it uses. The `discriminator` and `stripXSDPrefix` change in `canxml/decode.go` follows the `wire.md` § REQ-056 amendment.
- `assumed_value` takes its type from Task 6's restated classes.
- Canonical-XML dispatch applies the alias keys; the root element is named after the schema's global element for its class.
- Canonical JSON for the new types: `typereg` and `jsonhooks` from the generator; a `Hash` as a JSON object; the JSON form of an integer enumeration settled in Task 1's text before code.

Tests per shape, each failing without its rule.

### Task 9: PROBE-111 and the schema validation target

- `testkit/probes/<area>/probe_111_*.go` and its test: walk every `.opt` under `testkit/corpus/`; the round-trip leg (empty report, strict refuses nothing, second decode equal, the normalised comparison with the named rewrites); a can-fail control modelled on `probe_030_guard_internal_test.go`, a member withheld (for example `C_CODE_REFERENCE.referenceSetUri`) failing on the files that carry it.
- A Makefile target that writes the re-encoded OPTs to a temporary folder, copies `resources/its-xml/components` with `v2` rewritten to `v1`, and runs `xmllint --noout --schema` against `AM/Release-1.4/Template.xsd`; host `xmllint` when present, else the dev image with `libxml2-utils` added to `Dockerfile:55`; `make ci` runs it, and the CI Verify job installs `libxml2-utils`. A ratchet lists any OPT the pinned schemas refuse as written, with its reason; it starts empty.
- `conformance.md` PROBE-111 status: Implemented.

Verify: `go test ./testkit/probes/... -run 111 -count=1`, the new target, `make probe-status`.

### Task 10: reconcile and close out

- `bmm-conformance.md`: the *Known gaps* paragraph under § SDK-authored root schemas goes; `module-layout.md`, `scope.md`, `architecture.md` and `canxml` package docs describe the roots and the model as present.
- REQ-176's map row: `implementation: landed`, its packages, probes; `make spec-generate`.
- The backlog theme *AOM 1.4 canonical XML* deleted once Task 6 closes both lines.
- `CHANGELOG.md` `[Unreleased]`, on the maintainer's request: Added (the operational template model, canonical-XML `Hash` shapes); Changed with Breaking (`C_COMPLEX_OBJECT` members an interface, the three class renames, `Accreditation`, the canonical-XML `xsi:type` prefix rule).

Then `make ci`, `/sdd-review --lane full`, `/sdd-triage`, and the pull request.

## Open points for the delivery

Each is the implementer's or the orchestrator's to settle in Task 1's text or the task's code, and to put to the maintainer only if it reopens a decision above:

- The RM root's id and file name, and the extension keys' names.
- How `EXPR_LEAF.item` remembers the `xsi:type` form it was read in.
- The names given to `T_VIEW`'s anonymous types, and the Go names of `CodeDefinitionSet` and `referenceSetUri`.
- Whether `C_REFERENCE_OBJECT`, which only the BMM has, stays in the target.
- The canonical-JSON form of an integer enumeration, for which no ITS-JSON schema of the ADL 1.4 OPT exists.

## Verification

`make ci` (codegen-check, bmm-check, spec-check and the new schema validation target included), `make probe-status`, `make spec-check`.
