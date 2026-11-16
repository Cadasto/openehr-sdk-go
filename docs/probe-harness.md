---
kind: guide
---

# Conformance probes

The probe catalogue is [conformance.md](specifications/conformance.md): one entry per `PROBE-NNN`, saying what the probe checks and which requirement it satisfies. This guide is how probes are written, run and kept. What an SDK consumer, or the operator of a deployment a probe writes to, relies on stays in [conformance.md § REQ-082](specifications/conformance.md#req-082--runnability): the sandbox as a published backend, what a recording keeps off disk, what the replayer refuses, and the opt-in before any write to a live deployment.

## Writing a probe

- A probe asserts at the wire: the HTTP request (method, path, headers, body), the response status and the shape of the response body. It never asserts on source-level idioms such as function names. A probe that tests a refusal can also check the error the SDK documents for it, a sentinel or a typed error, because that error is part of the SDK's public contract.
- A JSON round trip is compared by value, never by bytes, as [wire.md § REQ-052](specifications/wire.md#req-052) says: the decoded values are equal, the recovered value passes the reference-model validation floor, and a check on the encoded form is [wire-equivalence](specifications/conformance.md#terms). A canonical XML round trip keeps its byte-stable check ([wire.md § REQ-056](specifications/wire.md#req-056), PROBE-033).
- A new probe takes the next free `PROBE-NNN`, whatever its topic, and a new topic adds its own section to the catalogue. The catalogue entry is written before the implementation lands. It says what the probe checks and which requirement it satisfies, and leaves the rule itself to that requirement's section.
- A backend-facing probe is written for Sandbox mode first. Cassette mode follows once a recording is captured, and Live mode once a reference deployment is reachable. A probe that reaches no backend declares `In-repo` instead.
- A probe's `Status:` changes in the catalogue when its state does: Draft to Implemented, then Ratified for a backend-facing probe; Draft to Deferred while the probe is unwritten; Deprecated before removal. The PR that makes the change names it in its body, and the CHANGELOG follows [releases.md § CHANGELOG](releases.md#changelog).
- A probe is never removed silently. It is marked `Status: Deprecated` with a reason and a removal target version, stays runnable for at least one minor version, and goes in the next major version. Before 1.0 a probe can go in the release that deprecates it, when its requirement is retired or another probe checks every input it checked at least as strictly; its `Status:` then names the probe that supersedes it, or the retired requirement, and the release that removed it. Its `PROBE-NNN` follows [REQ.md § Numbering policy](specifications/REQ.md#numbering-policy).

## Modes

A backend-facing probe runs in up to three modes. The runner, `testkit/probe`, picks the backend when it is invoked, and the probe definition stays the single source.

| Mode | Backend | Artefact | Use |
|---|---|---|---|
| **Sandbox** | the `sandbox/` in-memory transport | none | Fast unit tests; the CI default |
| **Cassette** | a replayed **recording** | `testkit/recordings/` | Deterministic CI against traffic captured from a real deployment |
| **Live** | a reference openEHR deployment | none | Verification against a real backend before a release |

An **in-repo** probe asserts a property over vendored inputs or over the SDK's own output, such as the AQL round-trip and catalogue properties, the upstream FLAT parity harness, and the codec and validation multiset probes, and reaches no server in any mode. Its **Modes** line says `In-repo`, and the three modes do not apply to it: there is no backend for a recording to capture or a deployment to confirm. That is a declared class, not a shortfall; [conformance.md § REQ-082](specifications/conformance.md#req-082--runnability) counts the entries in it.

For a backend-facing probe the **Modes** line is the record of which modes it supports today, and [roadmap.md](roadmap.md) tracks the modes still open.

The class check holds each Modes line to what its probe reaches. It refuses a dot import of a package it would follow, because it cannot tell which names that import declares.

A **recording** is a captured HTTP exchange: method, URL, request and response headers, status and both bodies. It is a different artefact from the vendored **fixture documents** under `testkit/corpus/` (§ Vendored fixtures), which are bodies only and carry no exchange. The two never share a directory: a fixture is hand-curated input, a recording is captured evidence, and only the second can go stale against a deployment.

## Mode selection

The runner selects the mode, never the probe. A backend-facing probe receives an already-configured client and does not know which mode it runs in: no mode parameter, no environment read, no type assertion on the transport. An in-repo probe receives no client, since there is nothing to configure. A probe that cannot be written without knowing its backend is asserting something other than wire behaviour, and is reformulated rather than given an escape hatch. Twelve probes do not work this way yet (§ Cross-mode agreement, *Known gaps*).

The runner has a single entry point that takes the mode and that mode's backend configuration, and runs the whole catalogue, a named subset or one probe. Selecting a mode the invocation cannot satisfy, Cassette with no recording for a probe or Live with no endpoint, fails loudly and never falls back to another mode.

## The probe result

Every probe reports through one shared result type, `probe.Result` in `testkit/probe`, carrying the `PROBE-NNN` id, the mode, the status and a detail string. The per-package `Result` types under `testkit/probes/` are aliases of it, not copies: a result compared across modes is the same type in every package, or the runner could not aggregate it.

Status is the closed set `pass`, `fail` and `skip`. A `skip` names the unmet precondition in its detail and is never reported or counted as a pass. The runner's summary tells "every probe passed" from "some probes skipped", and a run whose probes all skipped does not read as green: vacuous success is the failure this guards against, for the reason [PROBE-086](specifications/conformance.md#probe-086--upstream-flat-serialisation-parity) fails an empty compared set.

## Effect

Every backend-facing probe declares its effect, **read-only** or **mutating**, in an **Effect** field of its catalogue entry: part of the probe's definition, not a runner-side annotation. An entry without one counts as mutating ([conformance.md § Live mode](specifications/conformance.md#live-mode)). An in-repo probe needs no declaration, having no backend to affect.

## Cassette mode

Recordings are checked-in evidence, held to the standard of any vendored corpus. Their encoding is HTTP Archive 1.2 ([ADR 0020](adr/0020-cassette-recording-har.md)); what a recording keeps off disk, and what the replayer refuses, is [conformance.md § Cassette mode](specifications/conformance.md#cassette-mode).

- A recording's provenance (the deployment, the date and the capturing SDK commit) and the refusal of a recording without it are part of the replayer's contract in [conformance.md § Cassette mode](specifications/conformance.md#cassette-mode).
- Replay matches a normalised request key, not byte-exact equality: the method, the path with the deployment's REST base stripped, and the headers and body fields the probe's assertion depends on. A capture carries timestamps, generated UUIDs and `ETag` values that differ on every run, so byte-exact matching would make every recording single-use. The browser-oriented HAR fields (`timings`, `cache`, `pageref`) are outside the key, and the replayer ignores them. While no recording's assertion depends on a header or a body field, the key is the method and the stripped path alone, so two exchanges that differ only in headers or body match the same entry, in recorded order.
- Where one probe issues the same normalised request more than once and expects different responses (a write followed by a read of what it wrote), the recording keeps the exchange order and the replayer consumes matches in that order.

## Live mode

- A read-only probe can run against any reachable deployment.
- A mutating probe is self-scoping: it creates the resources it needs, derived from a per-run identifier, and depends on no server state it did not create. A probe that needs a pre-seeded EHR skips when that precondition is absent rather than failing.
- A probe whose precondition is a capability the openEHR REST specification defines and recommends that services expose, without requiring it, skips when the deployment does not expose it, naming the absent capability, rather than failing: the omission is an accepted deployment deviation, not an SDK defect. A required capability that is absent is a failure. The System capabilities operation (`OPTIONS /`, ITS-REST *Options and Conformance*) is the current instance: the Live core snapshot's System-version probe skips against a deployment that answers it `404`, and passes against one that serves it.
- The runner refuses a mutating probe in Live mode without the per-invocation opt-in ([conformance.md § Live mode](specifications/conformance.md#live-mode)). The repository's Live tests take their target and the mutating opt-in from environment variables, and CI sets neither.
- Cleanup is best effort, since a deployment can forbid deletion. The per-run identifier appears in every resource a mutating probe creates, so anything it leaves behind is attributable and removable by hand.

## The CDR cross-check tool

`cmd/cdrcheck`, run by `make cdr-crosscheck`, measures live CDRs and the SDK against each other. Over a directory of OPTs, it records each CDR's verdict on every document `instance.Generate` writes ([clinical-modeling.md § REQ-107](specifications/clinical-modeling.md#req-107--template-driven-rm-instance-example-generator)), and the SDK validator's verdict ([§ REQ-102](specifications/clinical-modeling.md#req-102--composition-validation)) on each CDR's own template example, side by side with the SDK's verdict on its own documents. It records these numbers and asserts nothing, so it is a tool and not a probe: it has no `PROBE-NNN` id, no pass/fail outcome and no catalogue entry, and it reads its own environment and builds its own clients, which § Mode selection rules out for a probe.

The tool writes to a CDR only under the opt-in [conformance.md § The CDR cross-check tool](specifications/conformance.md#the-cdr-cross-check-tool) requires, and otherwise follows § Live mode as a mutating probe does, except for templates. The id of every EHR it creates carries its per-run identifier, and every composition it creates goes into one of those EHRs (*Self-scoping* below), so whatever it leaves on a CDR is attributable to the run. Templates are the exception: the tool uploads each template under the template's own id, and uses a copy a CDR already holds under that id when the copy has the same signature (*Upload* below), which a probe's self-scoping rules out. The exception exists because the verdict the tool measures is each CDR's on that template id, and the openEHR REST definition API defines no template delete. Where a CDR holds other content under the id, the tool uploads the OPT again, under `<id>.xc1`, to every CDR. The tool records every template it uploads or finds, with the CDR's response, in its output.

- **Inputs.** A directory of OPTs named by `OPENEHR_XC_TEMPLATES`. One or two reachable CDRs: `OPENEHR_LIVE_EHRBASE` and `OPENEHR_LIVE_FERROEHR` name their REST bases, and `OPENEHR_LIVE_EHRBASE_BASIC` and `OPENEHR_LIVE_FERROEHR_BASIC`, when set, carry their Basic credentials as `user:pass`. `OPENEHR_LIVE_ALLOW_MUTATING` lets the run write to them. The `-only REGEXP` flag limits the run to the templates whose id matches, and the summary of a run so filtered says so and names the expression.
- **Upload.** Each OPT is sent to each CDR (`POST /definition/template/adl1.4`) with any UTF-8 byte-order mark removed. Where a CDR answers that it holds the id already, the copy it holds (`GET /definition/template/adl1.4/{template_id}`) is compared with the OPT by a signature of the parsed XML, not by bytes, so a re-serialised copy of the same template counts as the same. Where the content differs, the OPT is uploaded again under its id plus `.xc1`, to every CDR, and generated from under that id, so that the SDK and every CDR read the same content. The upload records of the original id are kept, marked as superseded by the new id, and left out of the counts. A template is never replaced or deleted, and it is renamed once at most: where a CDR holds a copy under `<id>.xc1` too, that copy is compared by the same signature, and a copy with other content leaves the template absent from that CDR, which then gets none of its documents and is asked for none of its examples, while the SDK and the other CDRs go on under `<id>.xc1`. Where a CDR refuses an upload as already present and its copy cannot be read back, the template counts as present on that CDR when the refusal was a `409`, and as absent otherwise.
- **Generate.** Per template, the run generates under each policy (`Required`, `Medium`, `Large`) with `ExampleFill` and with `RandomFill` on two fixed seeds, on a fixed clock. It records each generation error, `validation.Validate` and `validation.ValidateRM` on each output.
- **Post.** Each generated COMPOSITION goes to each CDR that holds its template (`composition.Save`), each to an EHR of its own (see *Self-scoping*). The status and the CDR's refusal text are recorded.
- **Examples.** Each CDR's own example of each template it holds is fetched (`GET /definition/template/adl1.4/{template_id}/example`), once with no `detail_level` from EHRbase and at `required`, `medium` and `complete` from FerroEHR, and validated by `validation.Validate`, with the members the decoder did not read. Each example that is a COMPOSITION is posted, its top-level `uid` removed and its other members as the CDR sent them, to every CDR that holds its template, again each to an EHR of its own.
- **Summary.** Per CDR: templates present after upload, EHRs created, SDK documents accepted, examples fetched, examples the SDK validator passes, own and other CDRs' examples accepted, and the refusals grouped by status and detail line, EHR creates included. Across CDRs: of the SDK documents the SDK itself accepts, how many every CDR refused and how many one CDR alone refused while every other accepted them, and the count of documents per combination of verdicts. The groups are read from the CDR's whole refusal text; a record holds the first 8000 bytes of it.
- **Self-scoping.** The run draws a per-run identifier, twelve hex digits written to `run.json` before its first request. It creates each EHR with `PUT /ehr/{ehr_id}` (`ehr.Create` with `WithEHRID`), with no body and no `Content-Type`, under an id that begins with that identifier: a version 8 UUID (RFC 9562) whose first 48 bits are the per-run identifier and whose last 48 bits number the EHRs the run has asked for, `xxxxxxxx-xxxx-8000-8000-nnnnnnnnnnnn`. Each composition goes into one of those EHRs. Each EHR create is recorded, with its id and the CDR's response, as it returns. A create the CDR refuses is recorded for its document and counted in the summary, and the run creates no EHR under an id the CDR chooses instead. Templates are not self-scoped (see above).
- **Credentials** never reach the output, a progress line or an error: a base URL that carries one is refused, and the credential's wire forms (`user:pass` and its base64 encoding, in plain text or JSON-escaped in whole or in part) are removed from a CDR's answer before it is kept or cut, and from everything written.
- **Effect.** The run uploads templates and creates EHRs and compositions; it deletes nothing.
- **Running it.** `make cdr-crosscheck` runs the tool. It is opt-in, and `make ci` does not run it. With no template directory it prints that it skipped and exits 0; with templates but no CDR, or without the write opt-in, it generates and validates offline and skips the CDR stages. The output is a directory outside the repository that no other run has claimed, by default a new one under the system temp directory: one JSON Lines file per stage, each line written as its record completes, the documents, `run.json`, `summary.json` and `summary.md`. A run that stops early, interrupted or on an error, keeps every record made so far and writes a summary marked partial; a second interrupt while it winds down ends it at once, before the summary is written.
- **Tests.** The tool's tests stand `httptest` servers in for the CDRs and reach no deployment. They pin the skip, the write opt-in, the byte-order-mark removal, the signature comparison, the `.xc1` upload and the superseded records, one EHR per document under a per-run id, the bodiless EHR create and a refused one reported, the records kept on an interrupted run, the `uid` removal, credential redaction before a cut and in its JSON-escaped forms, refusals over 8000 bytes grouped whole, distinct file names for ids that differ only in spacing, the output directory's claim and its place outside the repository through a symbolic link, the `-only` filter and the summary counts on a canned run.

## Cross-mode agreement

The same probe reaches the same verdict in every mode it runs in, with the recording captured once against the live backend. A probe that passes in Sandbox and fails on replay shows that the sandbox models the deployment wrongly: the recording is authoritative, and the sandbox is corrected, never the reverse. A `skip` for an absent precondition is not a disagreement, and an in-repo probe, reaching no server in any mode, has nothing to disagree about.

**Known gaps.**

- The auth probes (PROBE-001 to PROBE-009), the discovery probes (PROBE-040, PROBE-041) and PROBE-106 start their own `httptest` server from a fixture instead of receiving an already-configured client, so they run in Sandbox mode only: a Cassette or Live mode needs each rewritten to take a configured client.
- The class check, `TestREQ082ProbeClassMatchesModes`, classifies only the probes that have a `ProbeNNN` function under `testkit/probes`. The Modes lines of in-repo probes implemented as unit tests elsewhere are checked by review alone.

## Vendored fixtures

`testkit/corpus/` holds **fixture documents**: bodies, not exchanges. The Cassette-mode recordings are a separate artefact under `testkit/recordings/`.

A serialization or clinical-modeling probe that needs reference RM bytes or OPT bodies reads them from `testkit/corpus/` through the `testkit/fixtures` accessors (`TemplateOpt`, `CompositionJSON`, `CompositionXML`, `RMJSON`, `RMXML`, `SubmissionJSON`), never through a hard-coded directory name.
The layout, each corpus's provenance pin, its ingest and its integrity check are in [`testkit/corpus/README.md`](../testkit/corpus/README.md), among them the AQL suite corpus [PROBE-107](specifications/conformance.md#probe-107--upstream-aql-suite-admissibility-corpus) reads and the cross-format sets [PROBE-105](specifications/conformance.md#probe-105--upstream-cross-format-parity) reads.

| Kind | Role | Typical probes |
|---|---|---|
| `templates/` + `compositions/` | Operational template + canonical instance for a `template_id` | PROBE-022–027, PROBE-030 (JSON), PROBE-033 (XML when paired) |
| `rm/` | RM root samples without a paired OPT (ehrbase COMPOSITION/EHR_STATUS/FOLDER, leaf `DV_QUANTITY`, …) | PROBE-030, PROBE-033 |
| `submissions/` | CONTRIBUTION create payloads for the EHR contribution client (not `rm.Contribution` decode) | contribution client tests (REQ-059) |
| `its_rest/` | ITS-REST request and response **bodies**: no method, URL, headers or status (§ Modes) | PROBE-010+, discovery probes (REQ-050) |

PROBE-030 and PROBE-033 read the top-level files of `compositions/` and `rm/` through `fixtures.ListCompositionJSON` and `fixtures.ListRMXML`. Neither walk enters a subdirectory, so `rm/polymorphic/` is outside both (PROBE-038 names that directory itself). Composition XML those walks skip can be listed in `compositionXMLExcluded`, and a top-level `rm/` JSON sample that is an alternate wire shape or a deliberately invalid API-validation payload in `rmJSONExcluded` or `rmJSONExcludedPrefixes` respectively, all in that package, so the probes stay green while the files remain available for template and validation work.
