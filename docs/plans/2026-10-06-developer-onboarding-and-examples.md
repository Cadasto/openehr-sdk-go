---
kind: plan
---

# Plan: developer onboarding and example readability, the parts still open

**Date:** 2026-10-06, refreshed 2026-11-07.
**Status:** Draft. Carried forward from a read-only developer-experience audit of 2026-10-04. The refresh of 2026-11-07 delivered the small tasks and removed them from this plan: the two documented failing runs and the `validate-composition` fixture claim are pinned by tests, the SMART stub checks the PKCE proof, `canonical_json` takes an optional path, `ehr_create`, `contribution-build` and the quick-start REST snippets set deadlines, the README's catalogue claim is true, the learning order is one list, prose uses one flag spelling, the homepage's first action runs an offline example, the package map links examples, and the per-example Definition of Done lives in [ai-workflow.md § Examples](../ai-workflow.md#examples). What follows is still open.
**Covers:** no requirement. This is documentation and example work, on the maintenance lane ([two lanes](../development-process.md#two-lanes)), unless a step changes a public behaviour or a normative statement. A step that does goes through `/sdd-specify` first.
**Defers:** an example for every public function, which was never the aim; anything that needs a new requirement.

This header is for the reader. No tool reads it, and nothing fails when it is missing or out of date. The work itself meets the [Definition of Ready](../development-process.md#definition-of-ready) before it starts and the [Definition of Done](../development-process.md#definition-of-done) in the implementing PR; an example change also meets [ai-workflow.md § Examples](../ai-workflow.md#examples).

## Goal

Make the move from "I found this SDK" to "I built a correct integration" short and predictable for an application developer.

The readers in mind are the people who build on the SDK rather than on openEHR itself: an MCP server that forwards a caller's token, a federative client that talks to several repositories, a load or seeding tool, an application with a SMART sign-in, and a pipeline that validates data in CI. They follow one path: decode and validate offline, then connect, save and query, then authenticate.

What is left is the front door (the public site) and the shape of the heaviest examples. The answer is not an example per function. It is to teach the few interaction patterns developers reuse across packages, and to teach them once.

## Phase 1: the site as the front door

Intent for the reader: a visitor goes from the landing page to a running program without leaving the site and without reading a repository path.

1. **Quick start on the site.** [pages/install.md](../../pages/install.md) still sends readers to `docs/quick-start.md` on GitHub for the longer walkthrough. Either add a Quick start page to the navigation in [mkdocs.yml](../../mkdocs.yml), or turn Install into the complete walkthrough. Keep [docs/quick-start.md](../quick-start.md) the one detailed text: the site page is shorter and held to it by [cmd/examples/mirrors_test.go](../../cmd/examples/mirrors_test.go) where the same output appears.
2. **Examples by task.** Before the exhaustive catalogue in [pages/examples.md](../../pages/examples.md), add an index by what the reader wants to do: decode or convert a Composition; validate incoming data; generate or build data; connect to a CDR; save and update a Composition; execute AQL; authenticate a user or a service; build forms from a template. Label each example Start here, Common workflow or Advanced, and order the catalogue table by the learning order.
3. **Task headings.** The headings in [docs/examples.md](../examples.md) are directory names (`canonical_json`, `canxml_roundtrip`, and so on). Use plain task wording ("Decode canonical JSON") and keep the old names as stable anchors so existing links survive.
4. **An HTTP and error-handling guide.** Application developers need this more than an endpoint inventory. Cover: `errors.Is` for 404, 409, 412, 422 and the 5xx classes; `errors.As` for `transport.WireError` and `transport.DecodeError`; what raw bodies and server messages mean for patient data; ETag and `If-Match` optimistic concurrency; minimal versus representation responses; and context deadlines, HTTP timeouts, retry policy and safe logging. Teach the status answers as the vendored OpenAPI pin gives them, and link it instead of restating it.
5. **An Application patterns page** with small tested snippets, each saying what stays application policy and what is SDK behaviour:
   - per-request token forwarding in an MCP server;
   - independent catalogs and partial-failure handling in a federative client;
   - tuned connection pools, deadlines, retry policy and OpenTelemetry for load tools;
   - compile once, reuse many times, for seeders;
   - a SMART callback, to a token source, to a CDR call.

**Definition of done:** each new page is in the navigation and in `llms.txt` (the docs check derives the page list from the navigation and asserts that `llms.txt` covers it); `make docs-check` passes; every snippet is a Go `Example` or a transcript, so it is compiled or compared; site prose carries no em dashes.

## Phase 2: simplify the advanced examples

Intent for the reader: less plumbing between them and the SDK call being taught, and examples that run where a program may not open a listening socket.

1. **Sandbox instead of a loopback listener in `contribution-build`.** Its optional commit path still opens a local HTTP server and spends much of its code on a fake handler; the REST examples use `sandbox.Backend`. `ehr_create` keeps its listener on purpose, as the quick start's variant without the sandbox, and `smart-launch` keeps one unless an in-memory authentication transport is built.
2. **Approachable Contribution output.** `contribution-build` prints the whole request body before its short summary and has only a `-commit` flag. Print the summary by default, add `-json` for the full body, and move the program from the transcript test's exclusions to its allowlist.
3. **Split the structured AQL parser example.** [aql-parse-structured](../../cmd/examples/aql-parse-structured/main.go) is about 390 lines, most of it an AST printer. Make a short parse, inspect and emit example for the beginner path (inspect a few nodes, and handle expression variants added later safely), and move the full walker to an advanced example or a reusable helper.

**Definition of done:** every changed default transcript is deterministic and compared by a test; the program inventory in [cmd/examples/doc.go](../../cmd/examples/doc.go), [docs/examples.md](../examples.md) and [pages/examples.md](../../pages/examples.md) changes in the same commit.

Delete this plan when both phases have landed, or when the maintainer drops what is left.
