---
kind: plan
---

# Plan: developer onboarding and example readability, the parts still open

**Date:** 2026-10-06
**Status:** Draft. Carried forward from a read-only developer-experience audit of 2026-10-04 and checked against `main` on 2026-10-06.
**Covers:** no requirement. This is documentation and example work, on the maintenance lane ([two lanes](../development-process.md#two-lanes)), unless a step changes a public behaviour or a normative statement. A step that does goes through `/sdd-specify` first.
**Depends on:** the example and docs work already landed in [PR 225](https://github.com/Cadasto/openehr-sdk-go/pull/225), [PR 230](https://github.com/Cadasto/openehr-sdk-go/pull/230), [PR 232](https://github.com/Cadasto/openehr-sdk-go/pull/232) and [PR 233](https://github.com/Cadasto/openehr-sdk-go/pull/233).
**Defers:** an example for every public function, which was never the aim; anything that needs a new requirement.

This header is for the reader. No tool reads it, and nothing fails when it is missing or out of date. The work itself meets the [Definition of Ready](../development-process.md#definition-of-ready) before it starts and the [Definition of Done](../development-process.md#definition-of-done) in the implementing PR.

## Goal

Make the move from "I found this SDK" to "I built a correct integration" short and predictable for an application developer.

The readers in mind are the people who build on the SDK rather than on openEHR itself: an MCP server that forwards a caller's token, a federative client that talks to several repositories, a load or seeding tool, an application with a SMART sign-in, and a pipeline that validates data in CI. They follow one path: decode and validate offline, then connect, save and query, then authenticate.

The audit found the documentation strong on the offline clinical-modeling building blocks and thin where those blocks meet a live REST API. That gap is now closed in the examples and on pkg.go.dev. What is left is the front door (the public site), the shape of the three heaviest examples, and keeping every page that repeats an example true as the pages grow.

## What to keep

The audit named qualities worth protecting. New work keeps them.

- `canonical_json` is the model first example: compact, concrete, offline, and explicit about the fields it decodes.
- [docs/examples.md](../examples.md) explains each program the same way: purpose, packages, commands, output, and what application code should copy.
- Examples use public packages and checked-in fixtures, with no hidden setup.
- Example output in the catalogue is compared with real program output by a test, and every program left out of that test has a recorded reason ([cmd/examples/transcripts_test.go](../../cmd/examples/transcripts_test.go)).
- The documentation keeps the network-free building blocks apart from the HTTP, discovery and authentication layers.
- The roadmap says plainly what is partial or deferred.

The answer is not an example per function. It is to teach the few interaction patterns developers reuse across packages, and to teach them once.

## Already done

| What the audit asked for | Landed in |
|---|---|
| Report the RM floor and the template constraints as two named passes, fail on either, and explain that they compose but do not chain. Replace the mismatched `EHR_STATUS` demonstration. | [PR 225](https://github.com/Cadasto/openehr-sdk-go/pull/225) |
| One parse policy for operational templates: strict wherever a program validates, builds, generates, exports or lints; lenient only in `template-explore`, with the reason stated. | [PR 225](https://github.com/Cadasto/openehr-sdk-go/pull/225) |
| Runnable REST and auth examples that use the in-memory sandbox: `composition-crud`, `query-execute`, `definition-lifecycle` and `service-auth`. | [PR 230](https://github.com/Cadasto/openehr-sdk-go/pull/230) |
| Compiled Go `Example` functions on pkg.go.dev: 13 of them in 8 files, covering canonical JSON decode, both validation layers, `composition.Save` and `Update`, `query.Execute` with parameters, `WithTokenSource`, transport error classification and terminology lookup. | [PR 232](https://github.com/Cadasto/openehr-sdk-go/pull/232) |
| An exact release tag in the README and on the site, and a pkg.go.dev link on every package row. | [PR 233](https://github.com/Cadasto/openehr-sdk-go/pull/233) |

One deliberate difference from the audit: `composition-crud` shows a stale update as a 412 handled with `ErrPreconditionFailed`, where the audit named `ErrVersionConflict`. The pinned specification answers 412 for a stale update, and a 409 is what a delete of a version that is no longer the latest gets ([REQ-054](../specifications/wire.md#req-054)). The examples follow the specification.

## Phases

### Phase 1: the site as the front door

Intent for the reader: a visitor goes from the landing page to a running program without leaving the site and without reading a repository path.

**Tasks:**

1. **Quick start on the site.** [pages/install.md](../../pages/install.md) still sends readers to `docs/quick-start.md` on GitHub for the longer walkthrough. Either add a Quick start page to the navigation in [mkdocs.yml](../../mkdocs.yml), or turn Install into the complete walkthrough. Keep one detailed quick start (see Phase 3).
2. **Homepage action.** [pages/index.md](../../pages/index.md) offers Install and View on GitHub. Make the primary action "Run an offline example", with Install as its companion. The page already says that decoding, validation and the AQL tools run offline, so the action matches the promise.
3. **Examples by task.** Before the exhaustive catalogue in [pages/examples.md](../../pages/examples.md), add an index by what the reader wants to do: decode or convert a Composition; validate incoming data; generate or build data; connect to a CDR; save and update a Composition; execute AQL; authenticate a user or a service; build forms from a template. Label each example Start here, Common workflow or Advanced, and order the catalogue by learning path. Today the page offers a "try them in this order" list, but the catalogue table follows directory order and the page details four programs.
4. **Task headings.** The headings in [docs/examples.md](../examples.md) are directory names (`canonical_json`, `canxml_roundtrip`, and so on). Use plain task wording ("Decode canonical JSON") and keep the old names as stable anchors so existing links survive.
5. **Package navigation.** Every package row on [pages/packages.md](../../pages/packages.md) links to pkg.go.dev. Where an example exists, add a second link to the most relevant one. Keep the partial and planned maturity labels.
6. **An HTTP and error-handling guide.** Application developers need this more than an endpoint inventory. Cover: `errors.Is` for 404, 409, 412, 422 and the 5xx classes; `errors.As` for `transport.WireError` and `transport.DecodeError`; what raw bodies and server messages mean for patient data; ETag and `If-Match` optimistic concurrency; minimal versus representation responses; and context deadlines, HTTP timeouts, retry policy and safe logging. Teach the status answers as the pinned specification gives them, and link the specification section instead of restating it.
7. **An Application patterns page** with small tested snippets, each saying what stays application policy and what is SDK behaviour:
   - per-request token forwarding in an MCP server;
   - independent catalogs and partial-failure handling in a federative client;
   - tuned connection pools, deadlines, retry policy and OpenTelemetry for load tools;
   - compile once, reuse many times, for seeders;
   - a SMART callback, to a token source, to a CDR call.

**Definition of done:** each new page is in the navigation and in `llms.txt` (the docs check derives the page list from the navigation and asserts that `llms.txt` covers it); `make docs-check` passes; every snippet is a Go `Example` or a transcript, so it is compiled or compared; site prose carries no em dashes, as the existing pages do.

### Phase 2: simplify the advanced examples

Intent for the reader: less plumbing between them and the SDK call being taught, and examples that run in restricted environments.

**Tasks:**

1. **Sandbox instead of loopback listeners.** [ehr_create](../../cmd/examples/ehr_create/main.go) and the optional commit path of [contribution-build](../../cmd/examples/contribution-build/main.go) still open a local HTTP server and spend much of their code on fake handlers. The new REST examples already use `sandbox.Backend`. Moving these two over means less non-SDK code, makes the sandbox visible in real usage, and lets "offline" mean that no network capability is needed at all: the audit could not run `ehr_create` or `smart-launch` in an environment that forbids listening sockets. [smart-launch](../../cmd/examples/smart-launch/main.go) can keep an HTTP server unless an in-memory authentication transport is built.
2. **Approachable Contribution output.** `contribution-build` prints the whole request body before its short summary and has only a `-commit` flag. Print the summary by default, add `-json` for the full body, and make the default transcript deterministic and tested.
3. **Split the structured AQL parser example.** [aql-parse-structured](../../cmd/examples/aql-parse-structured/main.go) is about 385 lines, most of it an AST printer. Make a short parse, inspect and emit example for the beginner path (inspect a few nodes, and handle expression variants added later safely), and move the full walker to an advanced example or a reusable helper.
4. **Make the SMART stub enforce PKCE.** The stub in [smart-launch](../../cmd/examples/smart-launch/main.go) says a real authorization server checks the code verifier and then skips the check. Store the challenge when the authorization request arrives and verify the verifier at token exchange. Keep the program about state and verifier persistence, and put "attach the resulting token source to a CDR client" in a short documentation snippet instead of growing the program. Never print full tokens or verifiers.
5. **Smaller touches, without bloating the small examples:**
   - let [canonical_json](../../cmd/examples/canonical_json/main.go) take an optional Composition path, with its vendored fixture still the zero-argument default;
   - add context deadlines to `ehr_create`, `contribution-build` and the production-oriented REST snippets in the docs;
   - pick one spelling for flags in prose. Go's `flag` package accepts both, and the docs use `-corpus` style for most programs and `--opt` style for `generate-example`;
   - keep the deprecated `-cassette` spelling out of the beginner path. It stays in the flags table as a documented alias.

**Definition of done:** every changed default transcript is deterministic and compared by a test; the program inventory in [cmd/examples/doc.go](../../cmd/examples/doc.go), [docs/examples.md](../examples.md) and [pages/examples.md](../../pages/examples.md) changes in the same commit.

### Phase 3: claims, drift control and one home for each page

Intent for the reader: what the pages say stays true, and a fact lives in one place.

**Tasks:**

1. **The completeness claim.** [README.md](../../README.md) says the catalogue "has a program of this size for each SDK surface". The audit proposed this replacement for the time before the REST and auth examples existed: "The catalogue covers the major offline clinical-modeling workflows plus selected REST and SMART flows." Those examples now exist, so first check which client packages still have no program, then choose the wording that is true. Update every mirrored description in the same change.
2. **One detailed quick start and one catalogue.** [docs/quick-start.md](../quick-start.md) and [docs/examples.md](../examples.md) stay the canonical, detailed texts. The site shows shorter presentations that are generated, included or mechanically compared where that is practical. Today [cmd/examples/mirrors_test.go](../../cmd/examples/mirrors_test.go) holds only the `canonical_json` output in the README and the site page to the real program. Extend it whenever the same output appears in another prominent place, such as a new site quick start. A snippet that cannot be shared mechanically gets an owner and source comment, a Go `Example` or transcript test, deterministic output, and no copy of a whole program into several documents.
3. **Two documented claims that no test pins.** Both were backlog leads from the PR 225 review and moved here from `docs/backlog.md`:
   - [validate-composition](../../cmd/examples/validate-composition/main.go) says its composition matches the default fixture of `validate-from-json`, and the docs say the committed fixture is what [gen_fixture.go](../../cmd/examples/validate-from-json/gen_fixture.go) writes. No test checks either, and the generator is excluded from the build, so the copies can drift. They were identical when checked by hand, and a decode comparison agreed. Move the builder into a normal file and add a comparison test, or drop the "matches" sentence.
   - [docs/examples.md](../examples.md) describes two failing runs. `validate-composition -invalid` reports one `required` issue per pass at `/category`. `validate-from-json -corpus` reports no RM floor issues and 12 template issues. No test checks either, because the transcript test runs every program with no arguments and expects exit 0. Add a transcript case that passes arguments, expects exit 1 and checks the issue lines, or drop the exact counts.

**Definition of done:** the claim in the README is true and mirrored consistently; any extended mirror check passes; both unpinned claims are pinned or removed.

## Guardrails

These carry over from the audit.

- The specifications under `docs/specifications/` win over this plan. A research strand is never resolved silently while an example changes.
- Read the Go skills and the focused skills named in [AGENTS.md](../../AGENTS.md) before writing or reviewing Go.
- REST paths, bodies, headers and status codes come from the vendored OpenAPI pin under `resources/its-rest/`, never from memory.
- Do not hand-edit vendored fixtures. Do not import an internal package into a consumer-facing example. Do not add a runtime dependency only to simplify example code.
- Contexts come first on I/O calls, HTTP clients are injected, and the package-level function style stays.
- Never put full tokens, PKCE verifiers, patient data or raw error bodies into logs or transcripts.
- Godoc comments and package examples are for SDK users: no process ids, internal architecture notes or RFC-2119 language in them.
- The CHANGELOG is left alone unless a release is being cut or a maintainer asks.

## Definition of done for an example change

An example change is complete when:

- the program builds without relying on internal visibility;
- its default path is safe and offline unless the docs say otherwise;
- the source starts with its purpose and the exact run commands;
- errors are wrapped and classified through the SDK's public contracts;
- nondeterministic or secret values are not copied into docs;
- [docs/examples.md](../examples.md) explains purpose, packages, inputs, output, and what to copy;
- [cmd/examples/doc.go](../../cmd/examples/doc.go) and the site catalogue are in step;
- transcript or mirror coverage is added, or an exact exclusion reason is recorded;
- the package-level examples it touches compile;
- formatting, tests, docs checks and spec checks pass; and
- the diff holds no generated, vendored or unrelated change.

## Where this came from

The audit that this plan carries forward was a read-only review of the SDK's advertised examples, repository guides and public site, kept as a local working note. When it was written the repository had 17 example directories (it has 21 now), the non-network example, mirror and census tests passed, and two programs, `ehr_create` and `smart-launch`, could not run because the environment forbade loopback listeners. That is not a product defect, and it is the reason Phase 2 prefers the listener-free sandbox wherever it can stand in for the endpoint.
