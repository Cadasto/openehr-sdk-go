---
kind: plan
---
# Backlog

Leftovers of merged branches, by directory: suggestions, and findings the maintainer deferred here. Each line is a lead, not a finding: verify it before acting. A delivery whose `Files` touch a line's path folds it in and deletes the line. `sdd-pr flip --carry` appends; edit freely.

## cmd/probe-record
- cmd/probe-record/scenarios_writes.go · the ehr-status scenario submits is_modifiable true, so its recording would also pass against a server that behaves differently; changing it needs a recapture · from: pr201
- cmd/probe-record/main_test.go · only the composition-minimal scenario has an offline sandbox capture test; ehr-status and stored-query have none, unlike ehr-lifecycle · from: pr201

## docs
- docs/examples.md:619 · "holds none" can be read as an empty ETag, while REQ-054 falls back to Location whenever the ETag is not a well-formed object_version_id · by: sdd-doc-reviewer · from: fix/version-uid-from-etag

## docs/specifications
- docs/specifications/traceability.yaml · test files may still cite a REQ they do not pin and so appear in its generated tests list; a sweep over all test files is still to do (the audit estimated 45 to 50 files, not re-counted) · from: audit-2026-09
- docs/specifications/clinical-modeling.md · § REQ-107 has no spec text for what Options.Now sets (HISTORY.origin, ACTION.time and every DV_DATE_TIME default) or for the generator's placeholder values (the at0000 node id, the local::at0000 code, ehr://example) · from: audit-2026-09
- docs/specifications/clinical-modeling.md · § REQ-107 says Minimal materialises attributes with existence lower >= 1 plus BMM-mandatory ones, but shouldVisit in openehr/instance/generate.go also visits attributes whose cardinality lower is >= 1 and those with OPT-pinned children · from: audit-2026-09
- docs/specifications/clinical-modeling.md · nine Building-block independence (REQ-013) sections restate REQ-013 with no RFC-2119 keyword, and two more sections (Public surface scope near line 391, Amends REQ-117 near line 1416) carry none either, so sdd-check warns on each · from: audit-2026-09
- docs/specifications/wire.md · § REQ-052 says decode keeps whatever bound it reads with no keyword, and no sentence says a flag set only on the embedded interval of a Point_interval does not open a side (the outer flags win, pinned only by tests); the client-package split in Functional API areas also has no keyword and no normative home · from: audit-2026-09
- docs/specifications/bmm-conformance.md · the rm.Release bullet in Generator output conventions has no RFC-2119 keyword and points at § REQ-107, while REQ-107's MUST points back at it, so no binding sentence makes the generator emit the constant · from: audit-2026-09
- docs/specifications/bmm-conformance.md · the mapping tables never say an optional Any property becomes *any (bmmgen emits five), and the Hash rows give map[K]V with keys typically String while bmmgen always emits map[string]V · from: audit-2026-09
- docs/specifications/bmm-conformance.md · specifications still link into docs/plans/ (bmm-conformance.md § Cardinality and § Functions, rm-modeling.md, research-strands.md, use-cases.md), and Terminology_code and Terminology_term say TBD by generator, see plan · from: audit-2026-09
- docs/specifications/rm-functions.md · § REQ-121's Acceptance paragraph covers the in-context leaf rule but has no clause for the every-node rule · from: audit-2026-09
- docs/specifications/rm-modeling.md · a second (REQ-024) section, Generics for clients, validators, repositories, sits beside the canonical one in idiom.md § Generics policy · from: audit-2026-09
- docs/specifications/module-layout.md · § Versioning says a field added to an exported struct is not a breaking change with no RFC-2119 keyword, so it cannot relax the table row that makes a breaking change to a public type a major bump · from: audit-2026-09
- docs/specifications/conformance.md · § Adding probes still says a backend-facing probe must be runnable in at least Sandbox mode, but nothing checks it and § REQ-082 treats a missing mode as an open gap; the retired REQ-081 and the Launch-mode coverage (REQ-068) sections also carry no keyword · from: audit-2026-09
- docs/specifications/transport.md · the deprecated REQ-097 section carries no RFC-2119 keyword, so sdd-check warns on it; so does the service-discovery.md section Surfaced authorization-server metadata (REQ-070, REQ-062) · from: audit-2026-09
- docs/specifications/conformance.md:724 · The status recovers the version id "from the ETag or Location", which drops REQ-054's well-formedness order; cite that rule instead of restating it. · by: sdd-doc-reviewer · from: fix/version-uid-from-etag
- docs/specifications/wire.md:388 · The previous sentence still says the exposed ETag is what the caller uses for the next PUT, while this paragraph's VersionUID may be the Location tail. · by: sdd-doc-reviewer · from: fix/version-uid-from-etag
- docs/specifications/transport.md:118 · "the ETag and Location headers staying canonical" can be read as equal rank, while the cited REQ-054 orders ETag before Location and both ahead of the identifier body. · by: sdd-doc-reviewer · from: fix/version-uid-from-etag
- docs/specifications/wire.md:388 · "its `VersionUID` MUST be the `ehr_id` from `Location`" is unconditional, yet ITS-REST sends no `Location` on `GET /ehr/{ehr_id}` (overview-validation.openapi.yaml:303) and `ehr.Get` and `ehr.GetBySubject` use the same `newEHRMetadata`, leaving `VersionUID` empty; say "when the response carries a `Location`" · by: sdd-doc-reviewer · from: fix/version-uid-from-etag
- docs/specifications/conformance.md:725 · PROBE-065 now pins REQ-054's version-id rule but its Satisfies line lists only REQ-094, and the REQ-054 row in traceability.yaml lists only PROBE-010 to PROBE-013, so the new MUST has no catalogued probe · by: sdd-doc-reviewer · from: fix/version-uid-from-etag
- docs/specifications/wire.md:388 · the evidence sentence is firmer than the vendored sources: only `ETag_VERSION` says "the VERSION identifier", while `ETag_COMPOSITION` and `ETag_FOLDER` say "an identifier (e.g. a `version_uid` ...)" (ehr-validation.openapi.yaml:4383, 4402); soften "names the ETag as the version identifier" · by: sdd-doc-reviewer · from: fix/version-uid-from-etag
- docs/specifications/wire.md:388 · "a server may put the EHR_STATUS version there instead" has no recorded evidence: all five `POST /ehr` responses in testkit/recordings/*.har carry the bare ehr_id in the ETag; cite the server or soften it · by: sdd-spec-conformance-reviewer · from: fix/version-uid-from-etag
- docs/specifications/conformance.md:696 · PROBE-062's Wire assertion still says the Contribution `versions` list names the version uid "the write's `Location` returned", while the probe now binds on the ETag-first `VersionUID` (probe_062_audit_details_header.go:103); say "the version uid the write returned (REQ-054)" · by: sdd-doc-reviewer, sdd-spec-conformance-reviewer, go-reviewer · from: fix/version-uid-from-etag

## internal/bmmtype
- internal/bmmtype/bmmtype.go · Substitute leaves a formal parameter unresolved for a bare generic owner, so the data attribute of an OPT's EVENT, POINT_EVENT or INTERVAL_EVENT compiles with RM type T (the row is pinned in bmmtype_test.go) · from: audit-2026-09

## internal/templateinstance/rmwrite
- internal/templateinstance/rmwrite/write.go · EnsureSingle has no case for TERMINOLOGY_ID.value or a locatable's archetype_node_id, so the generator's writeBMMString cannot write them and drops the refusal; surfacing write errors in the generator (materialiseImplicitSingle, populateBMMRequiredAttrs, fillEntryCode, materialiseImplicitMultiple) needs this first and would move census outcomes · from: audit-2026-09

## openehr/aom
- openehr/aom/aom14 · no standalone ADL 1.4 archetype corpus is vendored, so the aom14 interval corpus tests read their constraint intervals out of the OPTs instead of real archetype files · from: audit-2026-09

## openehr/client/ehr
- openehr/client/ehr/metadata.go:37 · on a 409 or 412 the OAS puts the server's current version_uid in the `ETag`, so every versioned leaf's error-path metadata now reports that version as `VersionUID` where it was empty before (scratch run: `composition.Update` with a 412 and ETag "...::7" returns the error plus VersionUID "...::7"); neither the PR's consumer-visible note nor the `VersionMetadata` doc says it is the conflicting current version, not one the caller wrote · by: go-reviewer · from: fix/version-uid-from-etag
- openehr/client/ehr/ids_test.go:82 · the "bare id in the ETag" case passes with the well-formedness guard in `versionUIDFromETag` removed, because its `Location` tail equals the ETag; give it a `Location` tail that differs so it pins the MUST as well · by: sdd-spec-conformance-reviewer · from: fix/version-uid-from-etag

## openehr/instance
- openehr/instance/interval_order.go · an interval whose two sides' OPT constraints admit no ordered pair is left inverted with no error, as § REQ-107 prescribes; reporting it needs a spec change first, and ErrConstraintUnsatisfiable is raised only for C_STRING leaves · from: audit-2026-09
- openehr/instance/generate.go · settleIntervalEndpoints never reads a C_BOOLEAN on lower_unbounded or upper_unbounded, and a true-only C_BOOLEAN on lower_included or upper_included is not honoured on an open side (no vendored OPT constrains either flag) · from: audit-2026-09
- openehr/instance/rmtype.go · Generate refuses a generic non-interval rm_type_name such as POINT_EVENT<ITEM_TREE> with ErrUnknownRMType because newGenericRM builds only DV_INTERVAL instantiations, and the template validator reports the matching node as rm_type_mismatch (pinned by nonstorable_generic_test.go) · from: audit-2026-09
- openehr/instance/generate.go · a template root whose OPT node has no archetype id gets archetype_details with an empty archetype_id.value, which ValidateRM reports as required · from: audit-2026-09
- openehr/instance/generate.go · fillPartyRelationship gives every PARTY_RELATIONSHIP the same literal source and target ids (00000000-0000-0000-0000-000000000001 and -0002) instead of drawing from Options.UIDSource, and its fixed-uid closure is dead because stampsUID leaves PARTY_RELATIONSHIP out · from: audit-2026-09
- openehr/instance/generate.go · the fallback node id at0000, the reserved ADL root code, is written at seven sites here and in the C_CODE_PHRASE example of openehr/template/constraints/code.go; pick a non-reserved placeholder or document the choice · from: audit-2026-09
- openehr/instance/generate.go · finishNode gives an empty ITEM_LIST an at0000 placeholder ELEMENT while CLUSTER and ITEM_TREE fill from the OPT child, so an ITEM_LIST whose OPT items are only an ELEMENT slot gets slot_fill at /items[at0000] from the template validator · from: audit-2026-09
- openehr/instance/generate.go · ensureItems skips a child when makeChild, stampSlotFill or walkNode fails, so an unsatisfiable slot yields no ErrSlotFillUnsupported and CLUSTER.items can stay empty (latent, no vendored OPT reaches it) · from: audit-2026-09
- openehr/instance/generate.go · an OPT-filled null_flavour is kept with the DV_CODED_TEXT text example rather than its code's rubric (an OPT allowing only openehr::253 yields example, not unknown), and the rubric check in element_rule_test.go never meets one · from: pr199
- openehr/instance/generate.go · when the OPT leaves ACTION.ism_transition silent, the BMM-built ISM_TRANSITION carries current_state local::at0000|example|, which breaks RM Current_state_valid because fillCurrentState runs only on nodes the walk visits; the floor does not evaluate that invariant · from: pr199
- openehr/instance/generate.go · a required COMPOSITION.content with no OPT children still gets a BMM-built OBSERVATION without archetype_details (materialiseImplicitMultiple), which ValidateRM reports as is_archetype_root (degenerate OPT) · from: pr199
- openehr/instance/generate.go · a generated COMPOSITION keeps the category text example beside the OPT-pinned code openehr::433 because applyCompositionDefaults sets the rubric only when the code is empty; reported as refused by EHRbase 2.36.0 on commit (not re-run here) · from: pr199
- openehr/instance/locatable.go · a generated COMPOSITION carries a bare UUID uid (stampsUID); reported as refused by EHRbase 2.36.0 on commit while the same body without a uid commits (not re-run here) · from: pr199

## openehr/rm
- openehr/rm/temporal_funcs.go · DVTime.ToTime and DVDateTime.ToTime pass the valid leap second 23:59:60 to time.Date as second 60, so it converts to the next minute's instant with a nil error · from: pr199

## openehr/template
- openehr/template/parse_primitives.go · buildBoolean reads an empty true_valid or false_valid element as false even under ParseOPTStrict, so two empty elements yield the forbidden false/false C_BOOLEAN without an error (no vendored OPT has one) · from: audit-2026-09
- openehr/template/webtemplate · the Web Template builder keeps only the first value alternative of the corpus CLUSTER's labresult ELEMENT and projects one collapsed DV_TEXT leaf, so the reference's labresult/text_value key in ehrbase_conformance_cluster.json is refused on decode (PROBE-086 census) · from: audit-2026-09
- openehr/template/webtemplate · the Web Template builder spells an archetyped ACTION transition as the nodes transition and transition2 where the reference emits one in-context ism_transition, so 10 of the action body's 14 excluded keys are still refused on decode (PROBE-086 census) · from: audit-2026-09

## openehr/template/constraints
- openehr/template/constraints/temporal.go · CTime, CDateTime and CDuration Validate refuse forms the REQ-123 parse and the RM floor accept (20251024T121033, 10:30:00+0100, -P1D, PT1,5S); align them or record the gap · from: pr199

## openehr/validation
- openehr/validation/rmfloor.go · the RM floor does not evaluate Interval Limits_comparable, DV_ORDERED Other_reference_ranges_validity, REFERENCE_RANGE Range_is_simple, EHR_ACCESS Scheme_valid, DV_AMOUNT Accuracy_is_percent_validity and Accuracy_validity, or DV_QUANTIFIED Magnitude_status_valid, and checkDVInterval walks a bound beside its own *_unbounded flag without reporting the contradiction · from: audit-2026-09
- openehr/validation/rmfloor.go · an empty or absent temporal value is reported twice, as required at <path>/value and as rm_invariant (Value_valid) at <path>, and § REQ-112 does not state the pair as it does for TERM_MAPPING; also a typed-nil element inside a container such as COMPOSITION.content is skipped without any report · from: pr199
- openehr/validation/walk_composition.go · bmmSubtypes[DATA_VALUE] omits DV_INTERVAL, DV_PROPORTION, DV_MULTIMEDIA, DV_PARSABLE, DV_SCALE and DV_STATE, so an OPT slot declared as DATA_VALUE would refuse them with a false rm_type_mismatch (no vendored OPT declares one) · from: audit-2026-09
- openehr/validation/constraint_fixtures_test.go · the skip-listed fixtures Test_dv_parsable_open_constraint.v0 and clinical_content_validation are excluded from the no-violation test, but no test pins their violations, unlike the multimedia, boolean and count fixtures · from: audit-2026-09
- openehr/validation/rmfloor_temporal_element_test.go · no test pins that the Value_valid and Inv_null_flavour_indicated details stay value-free (REQ-093), or sends a JSON-null temporal value through the floor · from: pr199
- openehr/validation/rmread · rmread does not read the optional String fields magnitude_status on DV_COUNT, DV_PROPORTION and the temporal types, units_display_name and units_system on DV_QUANTITY, PARTY_IDENTIFIED.name or ATTESTATION.proof; the generator writes none of them, so no template can constrain them yet · from: pr199

## testkit/probe
- testkit/probe/livestatus_test.go · the Live snapshots assert less than the cassette witnesses: createEHRProbe (live_test.go) passes on any non-empty EHR id without comparing it to the per-run id, and the PROBE-065 read-back checks only a non-empty archetype_node_id, not the saved node id or template id · from: pr201

## testkit/probes/instance
- testkit/probes/instance/corpus_ratchet_test.go · the census runs with Language en and one fixed Now, and its placeholder scan flags only the literal example, so a generator that wrote encoding utf8 or read time.Now() would leave it green · from: audit-2026-09
- testkit/probes/instance/corpus_ratchet_test.go · the hollow_body floor counts every ELEMENT, so a body of null-flavour placeholders passes; counting only ELEMENTs that hold a value adds two rows (clinical_content_validation generate/example/example and generate/example/random) · from: pr199

## testkit/probes/versioned
- testkit/probes/versioned/probe_065_test.go:34 · the harness sets `Location` to the same full version id as the ETag, so PROBE-065's sandbox run cannot tell the ETag from `Location` (ignoring the ETag leaves ./testkit/probes/versioned green) and does not exercise the EHRbase shape this PR fixes; the `withLocation` knob now also controls the ETag · by: go-reviewer · from: fix/version-uid-from-etag
- testkit/probes/versioned/probe_012_etag_round_trip.go:19 · the doc comment ("the Location-derived VersionUID") and the failure detail at line 41 ("Location header missing or unparseable") describe the old rule now that `VersionUID` is ETag-first · by: sdd-spec-conformance-reviewer, go-reviewer · from: fix/version-uid-from-etag

## testkit/recordings
- testkit/recordings/composition-minimal.har · the captured OPT keeps its authoring tool's Generated By entry with an account name, as the vendored corpus OPTs do; strip it at capture if recordings are to carry no account names · from: pr201

## transport
- transport/response.go:61 · only the first `ETag` header is read, but overview-validation.openapi.yaml:352 lets servers add further opaque `ETag` headers, so an opaque one sent first sends `VersionUID` to the `Location` fallback; outside this range · by: sdd-spec-conformance-reviewer · from: fix/version-uid-from-etag
