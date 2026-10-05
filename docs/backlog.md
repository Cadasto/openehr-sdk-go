---
kind: plan
harvested_through: 2026-10-05T10:59:37Z
---
# Backlog

Leftovers of merged branches, by directory: suggestions, and findings the maintainer deferred here. Each line is a lead, not a finding: verify it before acting. A delivery whose `Files` touch a line's path folds it in and deletes the line. `sdd-pr harvest` and `sdd-pr flip --carry` append; edit freely.

## auth/jwtbearer
- auth/jwtbearer/assertion.go:370 · Removing usablePublicKey's `case nil` arm keeps every test green because the algorithm check refuses a nil key anyway; give the "Public reports no key" row a wantInMsg so the arm is pinned · by: sdd-spec-conformance-reviewer · from: feat/smart-token-lifecycle

## auth/smart
- auth/smart/revoke.go:64 · Token called after Revoke waits for an overtaken refresh to finish its network round trip before returning ErrReauthRequired, because Revoke leaves s.inflight set and a zero access token counts as stale; Revoke could set s.inflight = nil under the lock, which is safe because the old refresh checks `s.inflight == ex` before clearing it · by: go-reviewer · from: feat/smart-token-lifecycle
- auth/smart/source.go:948 · The token and revocation POSTs follow a caller client's 307/308 redirects with the token still in the body; discovery guards its redirects, auth/smart does not (predates the range for the token endpoint) · by: go-reviewer · from: feat/smart-token-lifecycle
- auth/smart/source.go:866 · A session restored with SetTokens on a new Source has no last token response, so a refresh response that leaves out scope gives an access token with an empty Scope although the imported token had one; consider falling back to the held token's Scope · by: go-reviewer · from: feat/smart-token-lifecycle
- auth/smart/source.go:95 · The TokenChange.Response doc says "Its Raw map is the hook's own copy", but maps.Clone copies only the top level: nested values such as fhirContext objects stay shared with LastTokenResponse; add the values inside Raw to the read-only list · by: go-reviewer · from: feat/smart-token-lifecycle
- auth/smart/jwks.go:15 · the JWKS cache uses a fixed five-minute TTL; Cache-Control on the JWKS response and stale-if-error are not honoured · from: smart-client-conformance plan

## cmd/examples/validate-composition
- cmd/examples/validate-composition/main.go:122 · The claims that this composition "matches the default fixture of the validate-from-json example" and that the committed fixture is what gen_fixture.go writes have no test; the generator is `//go:build ignore`, so the copies can drift · evidence: generator output byte-identical today; overlay canjson comparison identical; no test refers to either · fix: move the builder into a normal file and add a comparison test, or drop the "matches" sentence · by: go-reviewer · from: docs/examples-validation-path

## cmd/probe-record
- cmd/probe-record/scenarios_writes.go · the ehr-status scenario submits is_modifiable true, so its recording would also pass against a server that behaves differently; changing it needs a recapture · from: pr201

## docs
- docs/examples.md:205 · The failing-run outcomes documented here (-invalid: one `required` issue per pass at /category; -corpus at line 253: RM floor 0, template 12) are not checked by any test; the transcript test runs each program with no arguments and requires exit 0 · evidence: cmd/examples/transcripts_test.go:394-418; both variants match the prose by hand today · fix: add a transcript case with arguments that expects exit 1 and checks the issue lines, or drop the exact count · by: go-reviewer · from: docs/examples-validation-path

## docs/specifications
- docs/specifications/traceability.yaml · test files may still cite a REQ they do not pin and so appear in its generated tests list; a sweep over all test files is still to do (the audit estimated 45 to 50 files, not re-counted) · from: audit-2026-09
- docs/specifications/clinical-modeling.md · nine Building-block independence (REQ-013) sections restate REQ-013 with no RFC-2119 keyword, and two more sections (Public surface scope near line 391, Amends REQ-117 near line 1416) carry none either, so sdd-check warns on each · from: audit-2026-09
- docs/specifications/wire.md · § REQ-052 says decode keeps whatever bound it reads with no keyword, and no sentence says a flag set only on the embedded interval of a Point_interval does not open a side (the outer flags win, pinned only by tests); the client-package split in Functional API areas also has no keyword and no normative home · from: audit-2026-09
- docs/specifications/bmm-conformance.md · the rm.Release bullet in Generator output conventions has no RFC-2119 keyword and points at § REQ-107, while REQ-107's MUST points back at it, so no binding sentence makes the generator emit the constant · from: audit-2026-09
- docs/specifications/bmm-conformance.md · specifications still link into docs/plans/ (bmm-conformance.md § Cardinality and § Functions, rm-modeling.md, research-strands.md, use-cases.md), and Terminology_code and Terminology_term say TBD by generator, see plan · from: audit-2026-09
- docs/specifications/rm-functions.md · § REQ-121's Acceptance paragraph covers the in-context leaf rule but has no clause for the every-node rule · from: audit-2026-09
- docs/specifications/rm-modeling.md · a second (REQ-024) section, Generics for clients, validators, repositories, sits beside the canonical one in idiom.md § Generics policy · from: audit-2026-09
- docs/specifications/module-layout.md · § Versioning says a field added to an exported struct is not a breaking change with no RFC-2119 keyword, so it cannot relax the table row that makes a breaking change to a public type a major bump · from: audit-2026-09
- docs/specifications/conformance.md · § Adding probes still says a backend-facing probe must be runnable in at least Sandbox mode, but nothing checks it and § REQ-082 treats a missing mode as an open gap; the retired REQ-081 and the Launch-mode coverage (REQ-068) sections also carry no keyword · from: audit-2026-09
- docs/specifications/transport.md · the deprecated REQ-097 section carries no RFC-2119 keyword, so sdd-check warns on it; so does the service-discovery.md section Surfaced authorization-server metadata (REQ-070, REQ-062) · from: audit-2026-09
- docs/specifications/conformance.md:603 · Legs (b) and (c) say "the decoded upstream canonical document" but do not say which document a set that carries both canonical JSON and canonical XML uses, and the harness takes the JSON (testkit/conformance/crossformat/harness.go:452), so consult_record's canonical.xml never reaches a FLAT leg and the spec could name that choice · by: sdd-spec-conformance-reviewer · from: test/cross-format-goldens
- docs/specifications/conformance.md:168 · No automated check covers the MUST NOT on vendoring different instances as one set; it rests on the curated table in scripts/ingest-crossformat.sh:15, confirmed by hand for alternative_events, test_all_types and consult_record, so recording each set's pairing evidence in that table would make the rule reviewable · by: sdd-spec-conformance-reviewer · from: test/cross-format-goldens
- docs/specifications/conformance.md:610 · The landing tally (ten sets, 24 legs, 4 agree, 3 refused, 17 differ) repeats the generated CENSUS.md summary in the normative entry and goes stale at the next record change, so pointing at the census alone keeps one home · by: sdd-doc-reviewer · from: test/cross-format-goldens
- docs/specifications/service-discovery.md:169 · The bullet calls token_endpoint one of the members SMART App Launch 2.2.0 makes conditional, but SMART 2.2.0 (conformance.html) lists token_endpoint as REQUIRED and makes issuer required with sso-openid-connect, which the rule does not check; say that the anonymous-only relaxation is the SDK's own choice, and say whether issuer is checked · by: sdd-spec-conformance-reviewer · from: feat/smart-discovery-model
- docs/specifications/service-discovery.md:189 · REQ-073 has no rule for a plaintext Platform base URL, yet insecure_baseurl_test.go (now listed under REQ-073 at traceability.yaml:789) pins its refusal with ReasonInsecureURL (resolver.go:336-337); add that bullet · by: sdd-doc-reviewer · from: feat/smart-discovery-model
- docs/specifications/transport.md:373 · The Params comment says other auth-params are kept verbatim, while challenge.go stores those names in lower case. · by: sdd-doc-reviewer · from: feat/smart-backend-transport
- docs/specifications/auth.md:174 · The parenthetical that the client secret is empty when a client assertion is configured has no RFC-2119 keyword and does not say what NewFromCatalog does when both are set. · by: sdd-doc-reviewer · from: feat/smart-backend-transport
- docs/specifications/service-discovery.md:257 · The methods row still says that list feeds Phase 3b G-3 selection, while the new consumed-fields bullet says NewFromCatalog checks it as well. · by: sdd-doc-reviewer · from: feat/smart-backend-transport
- docs/specifications/auth.md:580 · The error-mapping prose says only token-exchange and refresh failures are wrapped in *auth.ExchangeError, though revocation failures are now wrapped too, and the ErrRevocationFailed godoc (auth/errors.go:34) omits a failure to sign the client assertion · by: sdd-doc-reviewer, sdd-spec-conformance-reviewer · from: feat/smart-token-lifecycle
- docs/specifications/auth.md:9 · The intro's "Covers REQ-060 through REQ-064 and REQ-069" and the coverage matrix leave out REQ-167; the openEHR scope branch rewrites the same intro line, so expect a merge conflict · by: sdd-doc-reviewer · from: feat/smart-token-lifecycle
- docs/specifications/auth.md:257 · The allowlist bullet binds the refusal and "does not choose" with no RFC-2119 keyword; only the cited REQ-068 sentence carries MUST · by: sdd-doc-reviewer · from: feat/smart-token-lifecycle
- docs/specifications/auth.md:368 · § REQ-167 does not define "holds no token" (no refresh token and no access-token value) nor say that Revoke then clears a valueless access token without calling the hook; revoke.go defines both · by: sdd-spec-conformance-reviewer · from: feat/smart-token-lifecycle
- docs/specifications/conformance.md:112 · PROBE-106 starts its own server instead of receiving a configured client, like PROBE-001 to 009, so the REQ-082 Known gaps bullet should name it and a Cassette or Live mode would need the probe rewritten · by: sdd-doc-reviewer, sdd-spec-conformance-reviewer · from: feat/smart-token-lifecycle
- docs/specifications/traceability.yaml:1840 · REQ-166 (the Bearer challenge on 401 and 403, a newly found gap worked under a new REQ like REQ-167) cites no PROBE for its wire behaviour, which development-process.md asks of a gap; it lives on PR 215, outside this branch · by: claude-opus-5-5 · from: feat/smart-token-lifecycle
- docs/specifications/auth.md:22 · § Canonical sources lists the two SMART specifications but not the rule the SMART client work followed: each gap belongs to the standard that owns it (the OAuth 2.0 family, OIDC, JOSE, HL7 SMART 2.1.0 as floor and 2.2.0 as target), and where SMART on openEHR disagrees or is silent the upstream standard decides · from: smart-client-conformance plan
- docs/specifications/auth.md:618 · nothing records the SMART client watch list: the RFC 7523 update (assertion aud as the issuer, typ client-authentication+jwt), URI forms of the openEHR capability names, DPoP, and SMART 2.2.0 associated_endpoints and authorization_details · from: smart-client-conformance plan
- docs/specifications/clinical-modeling.md:786 · the shared simplified-template model is to be "extracted with REQ-053 when a second consumer exists", but REQ-053 was that second consumer and the extraction was left on purpose for a third; say so · from: simplified-formats plan
- docs/specifications/clinical-modeling.md:80 · § Strict parse mode does not record the decision to stop where it is: no check for a missing language with original_language in its place, a top-level archetype id, or empty description fields, and the lenient parse stays silent when it drops the nested subtree · from: generated-instances plan

## internal/bmmtype
- internal/bmmtype/bmmtype.go · Substitute leaves a formal parameter unresolved for a bare generic owner, so the data attribute of an OPT's EVENT, POINT_EVENT or INTERVAL_EVENT compiles with RM type T (the row is pinned in bmmtype_test.go) · from: audit-2026-09

## internal/templateinstance/rmwrite
- internal/templateinstance/rmwrite/write.go · EnsureSingle has no case for TERMINOLOGY_ID.value or a locatable's archetype_node_id, so the generator's writeBMMString cannot write them and drops the refusal; surfacing write errors in the generator (materialiseImplicitSingle, populateBMMRequiredAttrs, fillEntryCode, materialiseImplicitMultiple) needs this first and would move census outcomes · from: audit-2026-09

## openehr/aom
- openehr/aom/aom14 · no standalone ADL 1.4 archetype corpus is vendored, so the aom14 interval corpus tests read their constraint intervals out of the OPTs instead of real archetype files · from: audit-2026-09

## openehr/instance
- openehr/instance/interval_order.go · an interval whose two sides' OPT constraints admit no ordered pair is left inverted with no error, as § REQ-107 prescribes; reporting it needs a spec change first, and ErrConstraintUnsatisfiable is raised only for C_STRING leaves · from: audit-2026-09
- openehr/instance/generate.go · settleIntervalEndpoints never reads a C_BOOLEAN on lower_unbounded or upper_unbounded, and a true-only C_BOOLEAN on lower_included or upper_included is not honoured on an open side (no vendored OPT constrains either flag) · from: audit-2026-09
- openehr/instance/rmtype.go · Generate refuses a generic non-interval rm_type_name such as POINT_EVENT<ITEM_TREE> with ErrUnknownRMType because newGenericRM builds only DV_INTERVAL instantiations, and the template validator reports the matching node as rm_type_mismatch (pinned by nonstorable_generic_test.go) · from: audit-2026-09
- openehr/instance/generate.go · a template root whose OPT node has no archetype id gets archetype_details with an empty archetype_id.value, which ValidateRM reports as required · from: audit-2026-09
- openehr/instance/generate.go · fillPartyRelationship gives every PARTY_RELATIONSHIP the same literal source and target ids (00000000-0000-0000-0000-000000000001 and -0002) instead of drawing from Options.UIDSource, and its fixed-uid closure is dead because stampsUID leaves PARTY_RELATIONSHIP out · from: audit-2026-09
- openehr/instance/generate.go · finishNode gives an empty ITEM_LIST an at0000 placeholder ELEMENT while CLUSTER and ITEM_TREE fill from the OPT child, so an ITEM_LIST whose OPT items are only an ELEMENT slot gets slot_fill at /items[at0000] from the template validator · from: audit-2026-09
- openehr/instance/generate.go · ensureItems skips a child when makeChild, stampSlotFill or walkNode fails, so an unsatisfiable slot yields no ErrSlotFillUnsupported and CLUSTER.items can stay empty (latent, no vendored OPT reaches it) · from: audit-2026-09
- openehr/instance/generate.go · an OPT-pinned context setting, ACTION current_state or INTERVAL_EVENT math_function keeps the DV_CODED_TEXT text example beside its code, as the category and null flavour did before their rubrics; each needs its own terminology group · from: chore/backlog-round3
- openehr/instance/generate.go · when the OPT leaves ACTION.ism_transition silent, the BMM-built ISM_TRANSITION carries current_state local::at0000|example|, which breaks RM Current_state_valid because fillCurrentState runs only on nodes the walk visits; the floor does not evaluate that invariant · from: pr199
- openehr/instance/generate.go · a required COMPOSITION.content with no OPT children still gets a BMM-built OBSERVATION without archetype_details (materialiseImplicitMultiple), which ValidateRM reports as is_archetype_root (degenerate OPT) · from: pr199
- openehr/instance/locatable.go · a generated COMPOSITION carries a bare UUID uid (stampsUID); reported as refused by EHRbase 2.36.0 on commit while the same body without a uid commits (not re-run here) · from: pr199

## openehr/rm
- openehr/rm/foundation_types_interval_gen.go · LowerIncluded and UpperIncluded are plain bools, so a canonical document that omits the RM-mandatory flag decodes it as false (excluded), the opposite of EHRbase's reading, and FLAT encode then writes `false` (PROBE-105 test_all_types canonical-flat) · from: pr-crossformat
- openehr/rm/data_types_encapsulated_xmlmar_gen.go · canonical XML writes and reads the Array<Octet> attributes (DV_MULTIMEDIA.data, integrity_check) as one element per byte, where ITS-XML DataTypes.xsd types them xs:base64Binary, so an openEHR XML multimedia payload does not decode (PROBE-105 consult_record); the generator in internal/bmmgen owns the fix · from: pr-crossformat
- openehr/rm/temporal_iso_test.go · REQ-123 accepts `2019-01-28T21:22:19,979+0000`, an extended date and time with a basic-format zone, a mixed representation; upstream openEHR_SDK replaced exactly that value in 66845f98 "CDR-541 fix mixed date formats", so whether REQ-123 should refuse mixed forms is open · from: pr208
- openehr/rm/temporal_funcs.go · DVTime.ToTime and DVDateTime.ToTime pass the valid leap second 23:59:60 to time.Date as second 60, so it converts to the next minute's instant with a nil error · from: pr199

## openehr/serialize/simplified
- openehr/serialize/simplified/flat_decode.go · decode aliases only `<root>/language|code` and `territory|code`; the indexed spellings StructuredToFlat writes (`<root>/language:0|code`) are neither applied nor refused but overwritten by the ctx values, a silent drop (PROBE-105 structured-flat) · from: pr-crossformat
- openehr/serialize/simplified/flat_decode.go · DV_PROPORTION `|type` (and numerator, denominator) reach canjson unchecked, so EHRbase's `1.0` fails as a canjson error naming no FLAT key, where deviations.md promises a refusal naming the key (PROBE-105 test_all_types flat-canonical) · from: pr-crossformat
- openehr/serialize/simplified/flat_decode.go · the WithTemplate completion of RM-mandatory attributes skips ACTIVITY.action_archetype_id, so a FLAT that omits it decodes to an empty string that breaks Action_archetype_id_valid although the OPT constrains it (PROBE-105 nested) · from: pr-crossformat
- openehr/serialize/simplified · EHRbase writes a renamed composition as `<root>/_name`; encode drops it and decode refuses it, while deviations.md says the formats carry no names (PROBE-105 consult_record, ehrn_abdm) · from: pr-crossformat
- openehr/serialize/simplified · FLAT and STRUCTURED decode refuse the body-form composer keys `<root>/composer|name`, `|id`, `|id_scheme` and `|id_namespace` as an unsupported PARTY_PROXY datatype, and most EHRbase-produced FLAT carries them (PROBE-105 census) · from: pr-crossformat

## openehr/template
- openehr/template/webtemplate/build.go · ACTION gets no in-context `time` or `ism_transition` node, so FLAT encode drops ACTION.time and any transition without a careflow_step (PROBE-105 test_all_types canonical-flat) · from: pr-crossformat
- openehr/template/webtemplate · INTERVAL_EVENT `math_function` and `width` are not Web Template nodes, so FLAT neither emits nor decodes them (PROBE-105 alternative_events) · from: pr-crossformat
- openehr/template/webtemplate · the Web Template builder keeps only the first value alternative of the corpus CLUSTER's labresult ELEMENT and projects one collapsed DV_TEXT leaf, so the reference's labresult/text_value key in ehrbase_conformance_cluster.json is refused on decode (PROBE-086 census) · from: audit-2026-09
- openehr/template/webtemplate · the Web Template builder spells an archetyped ACTION transition as the nodes transition and transition2 where the reference emits one in-context ism_transition, so 10 of the action body's 14 excluded keys are still refused on decode (PROBE-086 census) · from: audit-2026-09
- openehr/template/parse.go:411 · Strict parsing keeps an unrecognised leaf node type (CONSTRAINT_REF, for one) as a bare leaf and drops its constraint without error, so a strictly parsed template can still lose constraints; out of this range, a lead for the parser · evidence: Demonstration.v1.opt (8 CONSTRAINT_REF nodes) parses under ParseFileStrict with a nil error (go-reviewer overlay test) · fix: decide whether strict mode should reject or support CONSTRAINT_REF; backlog · by: go-reviewer · from: docs/examples-validation-path

## openehr/template/constraints
- openehr/template/constraints/temporal.go · CTime, CDateTime and CDuration Validate refuse forms the REQ-123 parse and the RM floor accept (20251024T121033, 10:30:00+0100, -P1D, PT1,5S); align them or record the gap · from: pr199

## openehr/validation
- openehr/validation/walk_composition.go · bmmSubtypes has a row for DATA_VALUE only, so an OPT node declared as another abstract DV class such as DV_ORDERED or DV_QUANTIFIED is refused with a false rm_type_mismatch (no vendored OPT declares one) · from: chore/backlog-round3
- openehr/validation/rmfloor.go · the RM floor does not evaluate Interval Limits_comparable, DV_ORDERED Other_reference_ranges_validity, REFERENCE_RANGE Range_is_simple, EHR_ACCESS Scheme_valid, DV_AMOUNT Accuracy_is_percent_validity and Accuracy_validity, or DV_QUANTIFIED Magnitude_status_valid, and checkDVInterval walks a bound beside its own *_unbounded flag without reporting the contradiction · from: audit-2026-09
- openehr/validation/rmfloor.go · an empty or absent temporal value is reported twice, as required at <path>/value and as rm_invariant (Value_valid) at <path>, and § REQ-112 does not state the pair as it does for TERM_MAPPING; also a typed-nil element inside a container such as COMPOSITION.content is skipped without any report · from: pr199
- openehr/validation/rmread · rmread does not read the optional String fields magnitude_status on DV_COUNT, DV_PROPORTION and the temporal types, units_display_name and units_system on DV_QUANTITY, PARTY_IDENTIFIED.name or ATTESTATION.proof; the generator writes none of them, so no template can constrain them yet · from: pr199

## pages
- pages/examples.md:51 · The heading still says "against a template", and the at-a-glance row at docs/examples.md:29 still says "vs OPT", while the rewritten sections say both the RM floor and the template constraints are required · by: sdd-doc-reviewer · from: #225

## scripts
- scripts/probe-status.sh · the test-file column is a filename heuristic (its header says so), not the runner's per-mode state the runnability work wanted `make probe-status` to show · from: probe-runnability plan

## smart/discovery
- smart/discovery/resolver.go:296 · The shared fetch runs under the starting caller's context, so one caller's cancellation fails every waiter that joined it with a context error those waiters did not cause, and now also drops a fresh cached catalog; consider running the shared fetch under context.WithoutCancel plus the client timeout, or letting a waiter whose own ctx is still live retry · by: go-reviewer · from: feat/smart-discovery-model
- smart/discovery/static.go:44 · cmp.Or(cfg.BaseURL, cfg.Issuer) means an existing StaticConfig whose Issuer names a separate identity provider now quietly sends that provider's URL as aud through NewFromCatalog, and the mistake only shows when the authorization server rejects it; say so in the upgrade notes, or require BaseURL whenever a caller means a separate provider · by: go-reviewer · from: feat/smart-discovery-model
- smart/discovery/errors.go:73 · DiscoveryError.Issuer now holds the Platform base URL while ServiceCatalog.Issuer holds the OpenID Connect issuer, so one field name means two things on neighbouring types; this PR already breaks the Error() text, so adding a BaseURL field (and deprecating Issuer) costs less now than later · by: go-reviewer · from: feat/smart-discovery-model
- smart/discovery/resolver.go:246 · A fresh cache hit returns the cached catalog without the calling Resolver's own checks, so with a Cache shared between Resolvers built with different options a stricter Resolver gets a catalog its checks would refuse until the entry expires; this predates the branch (main returned a fresh hit unchecked too) · by: sdd-implementer · from: feat/smart-discovery-model
- smart/discovery/catalog.go:129 · The RevocationEndpoint field doc does not say that auth/smart's Source.Revoke posts to it, unlike the signing-algorithm list doc updated in this range · by: go-reviewer · from: feat/smart-token-lifecycle
- smart/discovery/catalog.go:207 · only the launch-base64-json capability constant exists; no decoder reads a base64-JSON launch context, and the SMART client plan's watch list also named relative endpoint URLs in the discovery document, not yet checked · from: smart-client-conformance plan

## testkit/conformance/webtemplate
- testkit/conformance/webtemplate/case.go · IsCompositionMeta matches only unindexed spellings, so StructuredToFlat's `language:0|code`, `composer:0|name` and `context:0/start_time:0` reach decode in PROBE-105's structured-flat leg instead of being held out; changing it moves PROBE-086 · from: pr-crossformat

## testkit/corpus
- testkit/corpus/README.md:117 · lists only the rewrites applied to social.opt; the full recipe for normalising Code24 OPT exports (eight steps, a script, and FerroEHR accepting all seven converted exports) is in git history at `f649aae4:docs/plans/2026-09-29-generated-instances-pass-the-rm-floor.md`; move it here if more Code24 exports are vendored · from: generated-instances plan

## testkit/probe
- testkit/probe/livestatus_test.go · the Live snapshots assert less than the cassette witnesses: createEHRProbe (live_test.go) passes on any non-empty EHR id without comparing it to the per-run id, and the PROBE-065 read-back checks only a non-empty archetype_node_id, not the saved node id or template id · from: pr201

## testkit/probes/auth
- testkit/probes/auth/launch_modes.go:271 · backendSigner builds by hand the SMART client assertion that jwtbearer.NewClientAssertion now provides; switching to it would keep the probe on the profile the SDK enforces · by: go-reviewer · from: feat/smart-token-lifecycle
- testkit/probes/auth/probe_106_token_revocation.go:169 · The probe compares only client_id and the Authorization header, so a revocation request that adds client_secret or client_assertion fields the token request lacked still passes; compare the full set of client-authentication form fields at both endpoints · by: sdd-spec-conformance-reviewer · from: feat/smart-token-lifecycle
- testkit/probes/auth/probe_106_token_revocation.go:112 · PROBE-106 logs four REQ-092 "plaintext URL in catalog" WARN lines per run; httptest.NewTLSServer and "https://" + req.Host pass with no warnings · by: go-reviewer · from: feat/smart-token-lifecycle

## testkit/probes/instance
- testkit/probes/instance/corpus_ratchet_test.go · the census runs with Language en and one fixed Now, and its placeholder scan flags only the literal example, so a generator that wrote encoding utf8 or read time.Now() would leave it green · from: audit-2026-09
- testkit/probes/instance/corpus_ratchet_test.go · the hollow_body floor counts every ELEMENT, so a body of null-flavour placeholders passes; counting only ELEMENTs that hold a value adds two rows (clinical_content_validation generate/example/example and generate/example/random) · from: pr199

## testkit/recordings
- testkit/recordings/composition-minimal.har · the captured OPT keeps its authoring tool's Generated By entry with an account name, as the vendored corpus OPTs do; strip it at capture if recordings are to carry no account names · from: pr201
