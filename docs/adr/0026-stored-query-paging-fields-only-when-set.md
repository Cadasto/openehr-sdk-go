---
kind: adr
id: ADR-0026
title: "Stored-query execution sends offset and fetch only when the caller sets them, departing from the pinned Query schema"
status: accepted
date: 2026-10-13
---

# ADR 0026 — Stored-query execution sends offset and fetch only when the caller sets them, departing from the pinned Query schema

- **Status:** Accepted, 2026-10-13.
- **Supersedes:** —
- **Superseded by:** —
- **Strand:** resolves item 2 of [STRAND-09](../specifications/research-strands.md#strand-09--its-rest-conformance-follow-ups), recorded from the maintainer's issue [#261](https://github.com/Cadasto/openehr-sdk-go/issues/261) (its *Expected behaviour*).
- **Amends:** [REQ-057](../specifications/wire.md#req-057) (Stored AQL queries), which states the paging rule, and [REQ-095](../specifications/wire.md#req-095) (OpenAPI authoritative source), which carries the third keyed exception.
- **Related:** [ADR 0024](0024-contribution-deletion-without-data.md) (the keyed-exception form this one follows).

## Context

**What the pin says.** [`resources/its-rest/query-validation.openapi.yaml`](../../resources/its-rest/query-validation.openapi.yaml) declares stored-query execution on two verbs, with different rules for the same two fields:

| Verb | Where the fields sit | Required | Defaults |
|---|---|---|---|
| GET `query_execute_stored_query`, `…_version` | query parameters `offset` and `fetch` (`:588-608`) | neither | `offset` defaults to `0`; `fetch` "depends on the implementation" |
| POST `query_execute_stored_query_body`, `…_version_body` | the `Query` request body (`:552-565`) | `offset`, `fetch` and `query_parameters` | the `Offset` schema defaults to `0` (`:523-530`); the `Fetch` schema has none (`:531-537`) |

So a POST body that follows the pin to the letter carries a `fetch` value, and the pin gives the client no value to send. `fetch: 0` asks for zero rows, and any other number is a row cap the caller never chose.

**What the SDK did.** Stored-query execution always sent `offset` (`0` when unset) and sent `fetch` only when the caller set it, on both verbs. The POST body therefore already departed from the `required` list, and § REQ-095 named no such departure.

**What is observed.** EHRbase refuses `offset` without `fetch` on both verbs with `422` ("Query parameter for offset provided, but no fetch parameter"): first seen on 2.35.1 for POST (STRAND-09, 2026-09-09), and on 2.36.0 for POST and GET (issue #261). The same server accepts neither field, `fetch` alone, and both. So every stored-query call made without `WithFetch` failed against EHRbase. Ad hoc execution was not affected, because it already sends each paging field only when the caller sets one.

**Why this needs a decision.** STRAND-09 item 2 named three ways out: always send `fetch`, send a sentinel, or send a default the server advertises. The pin defines no sentinel, ITS-REST gives a server no way to advertise its default, and any number the SDK sends is a row cap the caller did not choose. A fourth way, sending neither field unless the caller sets one, follows the GET contract exactly and departs from the POST `required` list.

## Decision

**Stored-query execution sends `offset` and `fetch` only when the caller sets them, on both verbs, and honours an explicit `0`.** A caller who sets `offset` without `fetch` gets that request as asked; the server decides whether to accept it. The POST body keeps `query_parameters`, as an empty object when the caller passes none. [§ REQ-057](../specifications/wire.md#req-057) *Paging* states the rule, and [§ REQ-095](../specifications/wire.md#req-095) lists the POST omission as its third keyed exception.

The exception is tied to the pin's current text. It lapses if the pin makes `offset` and `fetch` optional on the `Query` schema, as it already is on the GET operations and on `AdhocQueryExecute`.

## Consequences

- **Easier:** a stored-query call with no paging option runs against EHRbase and against any server that applies its own defaults, and stored and ad hoc execution follow one paging rule.
- **Harder:** a server that validates POST bodies against the pinned schema refuses a body without `offset` and `fetch`. A caller facing one sets both, which also satisfies EHRbase.
- **Consumer-visible:** a stored query with no paging option no longer sends `offset=0`. A server that applies the pin's default reads the same request. A caller who relied on the SDK's `0` sets `WithOffset(0)`.
- **Permanently constrained:** the SDK does not invent a `fetch` value, and it does not refuse `offset` without `fetch` on behalf of one server. A server's own pairing rule stays the server's.
- **Cheap to undo:** sending `offset` again is one line per verb, a CHANGELOG entry and a minor release, which is why this records a deliberate choice and not an irreversible fork.

## Traceability

- Resolves: STRAND-09 (item 2)
- Amends: REQ-057, REQ-095
