# ADR 0019 — Definition metadata timestamps: a closed tolerant layout set on decode, RFC 3339 on encode

- **Status:** Accepted, 2026-08-30.
- **Supersedes:** —
- **Superseded by:** —
- **Strand:** none — direct decision (this plan's Phase 0); no prior open research strand.
- **Introduces:** [REQ-144](../specifications/wire.md#req-144--definition-metadata-decoding) (Definition metadata decoding). **Amends:** [REQ-095](../specifications/wire.md#req-095) (OpenAPI authoritative source) — one keyed compatibility exception, carried by § REQ-095 itself rather than asserted from here.
- **Plan:** [2026-08-30-definition-metadata-decoding.md](../plans/archive/2026-08-30-definition-metadata-decoding.md) Phase 0.
- **Related:** [ADR 0004](0004-numeric-wire-tolerance.md) (strict-encode / permissive-decode for BMM numerics — the asymmetry precedent and its evidence bar); [ADR 0015](0015-flat-metadata-spelling.md) (accept both spellings on input, emit exactly one).

## Context

Two Definition-area catalog descriptors carry a timestamp, and both decode into a Go
`time.Time`: `TemplateMetadata.created_timestamp` (the template list) and
`StoredQueryMetadata.saved` (the stored-query list). `encoding/json` unmarshals a
`time.Time` from RFC 3339 and from nothing else.

**The vendored pin is not symmetric about the two fields.** This matters, because
[REQ-095](../specifications/wire.md#req-095) makes the OpenAPI files authoritative for
per-endpoint detail, so how far tolerance may go is a different question for each:

| Field | Pin declaration | Consequence for an RFC 3339-only decoder |
|---|---|---|
| `created_timestamp` | `required` ([`definition-validation.openapi.yaml:509`](../../resources/its-rest/definition-validation.openapi.yaml)), property is a bare `type: string` with **no `format`** (`:521-522`) | The decoder **over-constrains** the pin. Nothing in the pin says this string is a date-time at all. |
| `saved` | `required` (`:3872`), `type: string` + `format: date-time` (`:3882-3884`), example `2017-07-16T19:20:30.450+01:00` (`:3893`) | The decoder does **exactly** what the pin asks. Tolerating more is a departure that needs granting. |

**Deployments emit forms neither field's decoder accepts.** The recorded failure:
`created_timestamp` arriving as `2019-04-01 10:12:33` — space separator, no zone —
aborted the whole `ListTemplates` decode (`609a104`, "tolerate space-separated
created_timestamp in ListTemplates", on the unmerged `feat/datamap` branch). The blast
radius is the point: `encoding/json` fails the containing item, the item fails the list,
and the consumer sees *no* templates rather than n−1. A template catalog that cannot be
listed is a catalog that cannot be used, and the SDK cannot fix a remote server's
formatting.

**The zone-less forms are not obviously wrong, either.** The pinned REST overview says:

> Timezone SHOULD be only supplied when needed, otherwise the local timezone is assumed.
> — [`resources/its-rest/overview-validation.openapi.yaml:675`](../../resources/its-rest/overview-validation.openapi.yaml), the *Datetime format* note

A server omitting the zone is following that `SHOULD`. The overview then tells the reader
to assume the *local* timezone — which is well defined for a human reading a catalog on
the server's own console, and undefined for an SDK talking to a remote host over HTTP.
The SDK does not know, and cannot discover, what "local" means to the deployment it is
reading from.

The encode side is not in question: nothing here proposes emitting anything but RFC 3339.
This is the same asymmetry ADR 0004 settled for quoted numerics — liberal on decode,
single-valued on encode — applied to a different surface, and ADR 0004's evidence bar
applies with it: tolerance is granted for a failure that was actually observed, not
pre-emptively for every format a server might conceivably emit.

## Decision

**Decode of the two Definition metadata timestamps accepts a closed set of layouts;
encode stays stdlib RFC 3339.** The normative wording is
[wire.md § REQ-144](../specifications/wire.md#req-144--definition-metadata-decoding);
this ADR records the decision behind it and the departures it takes, not the requirement
itself.

The accepted set, in the order a decoder tries it:

| Layout | Shape | Why it is in the set |
|---|---|---|
| `time.RFC3339` / `time.RFC3339Nano` | zoned, ISO 8601 extended | The pin's own `saved` example; the only form `encoding/json` accepted before this decision. |
| `2006-01-02T15:04:05` | zone-less, ISO 8601 extended | A server following the overview's "supply the zone only when needed" `SHOULD`. |
| `2006-01-02T15:04` | zone-less, minute precision, ISO 8601 extended | The same, from a server that records catalog metadata to the minute. |
| `2006-01-02 15:04:05` | zone-less, **space** separator | **Deployment interop.** Not ISO 8601 extended (no `T`), not REST-legal, and not a form the pin exemplifies. It is in the set for one reason: deployments emit it and it broke a real catalog read (`609a104`). |

The set is **closed** — a non-empty value matching no member is an error, not a
best-effort guess — and it is a **one-way door**: removing a layout later breaks every
consumer whose server emits it, so each entry is an addition made deliberately and kept.

Fractional seconds ride the set rather than doubling it. Go's `time.Parse` absorbs a
fractional-second field immediately following a seconds element even when the layout omits
it, so the three seconds-bearing layouts accept fractions without separate entries. The
minute-precision layout carries no seconds element and so accepts neither seconds nor
fractions — an input carrying them simply matches a seconds-bearing layout instead. This
is a property of the standard library the set is built on, not a rule the SDK invents.

### The three departures

**(a) For `saved`, the tolerance exceeds the pin — a keyed exception to REQ-095.**
`saved` is pinned `required` with `format: date-time`, and REQ-095 says the OpenAPI wins
when it and in-repo prose disagree. Accepting a zone-less or space-separated `saved` is
therefore not a reading of the pin; it is a departure from it, and it is granted here as a
**named compatibility exception** on deployment evidence — the same bar ADR 0004 set: a
real observed failure, not pre-emption. The exception is written into
[§ REQ-095](../specifications/wire.md#req-095) itself, so the authoritative-source rule
carries its own single exception and no ADR overrides a requirement from outside it. For
`created_timestamp` no exception is needed: the pin declares an unformatted string, so
tolerance there *closes* an over-constraint rather than opening one.

**(b) An absent, `null`, or empty timestamp yields the zero `time.Time`, with no error.**
Both fields are pinned `required`, so a strict reading would refuse a descriptor missing
one. The decision prioritises catalog readability: a template whose `created_timestamp`
the server did not populate is still a template the consumer needs to see and use, and its
identity, concept, and archetype id are all intact. The zero time is an honest "not
stated" — it is what a `time.Time` field means when nothing set it — and it is
distinguishable by the caller (`IsZero`). This departure is deliberately **narrow**: it
covers absence, not malformation. A non-empty value that matches no accepted layout is a
server saying something the SDK cannot read, and that fails loudly (below).

**(c) Zone-less values decode as UTC, where the overview assumes local time.**
`time.Parse` returns a UTC time when the input carries no zone indicator, so UTC is what
the tolerant layouts produce. That is recorded here as a **decision**, not passed off as a
side effect of the standard library: the overview's stated assumption is the deployment's
*local* zone, which the SDK cannot know for a remote server, cannot discover from the
response, and must not guess from the client host's own `time.Local` — that would make the
same catalog decode to different instants on different machines. UTC is the one
deterministic, reproducible choice, and the departure from the overview's assumption is
stated rather than hidden.

### Out of scope for this ADR

- **RM `DV_DATE_TIME` wire formats.** REQ-052 / REQ-123 territory, untouched. This decision
  reaches exactly two Definition-area catalog metadata fields, not clinical temporal values.
- **Any global zone-less tolerance for `time.Time`.** The tolerance attaches to these two
  fields' decoders; it does not become an SDK-wide time policy.
- **The empty-list shape.** § REQ-144 also fixes the non-nil zero-length slice `ListTemplates`
  and `ListStoredQueries` return on an empty 2xx body. That is a plain surface contract with
  no pin conflict and no departure to grant — it rides the same § because it is the same
  read path, and it needed no decision.

## Consequences

- **A catalog with one unreadable timestamp is still a catalog.** The recorded failure mode
  — one space-separated `created_timestamp` costing the consumer the entire template list —
  is closed for every layout in the set.
- **The layout set is load-bearing and one-way.** Every accepted layout lives in one place
  with the reason it is there, so adding a fifth is a reviewed edit rather than a scattered
  fallback chain, and removing any of the four is a breaking change to be treated as one.
- **Zone-less values re-marshal with a `Z` the wire never carried.** Decoding
  `2019-04-01 10:12:33` yields a UTC time, and encoding that time emits
  `2019-04-01T10:12:33Z`. Round-tripping a descriptor therefore does not reproduce the bytes
  received. Stated openly rather than discovered later: the SDK's output is a *correct*
  RFC 3339 rendering of the instant it decoded under departure (c), not a transcription of
  the server's spelling. Consumers that must echo the original spelling need the raw string,
  which this decision does not provide.
- **An unreadable non-empty timestamp still fails, and names the field.** The tolerance buys
  more accepted inputs, not a silent zero on the ones it still refuses — the failure mode
  that would let a wrong instant reach a consumer as though the server had sent it. Catalog
  timestamps are design-time metadata, not clinical content, so the offending value may
  appear in the error.
- **REQ-095 gains its first named exception, and a precedent for how one is written.** The
  exception is keyed (this tolerance, this REQ, this ADR) and lives in the amended
  requirement, which keeps the authoritative-source rule readable in one place. It is not a
  general licence to depart from the pin.
- **Encode is unchanged; nothing downstream shifts.** This is additive on the input surface
  only, exactly as ADR 0004 and ADR 0015 were on theirs.

## Alternatives considered

- **Keep RFC 3339-only decode (status quo).** Rejected: it is a live adoption blocker — a
  conformant-enough deployment renders its whole template catalog unreadable through the SDK
  — and for `created_timestamp` it enforces a constraint the pin does not state.
- **Accept anything, falling back to the zero time on failure.** Rejected: it converts a
  server defect into a plausible-looking value silently, and the caller cannot tell an
  unpopulated timestamp from an unparsed one. Departure (b) is deliberately confined to
  absence for this reason.
- **A general-purpose permissive time parser (try many layouts, or a date-guessing library).**
  Rejected: an open set has no reviewable boundary, invites ambiguity between day-first and
  month-first spellings, and would silently absorb the next malformed input instead of
  reporting it. A closed set of four is auditable.
- **Decode zone-less values in the client host's local zone (`time.Local`).** Rejected: it
  reads the overview's "local timezone" as the *client's*, which it is not, and makes the
  same response decode to different instants on a developer laptop and a UTC container.
- **Keep the timestamps as raw strings and expose a parse helper.** Rejected: it breaks the
  landed `time.Time` surface of both descriptors for every existing caller, and pushes the
  same layout question onto each consumer to solve inconsistently.
- **Tolerate `created_timestamp` only, leaving `saved` strict.** Tempting, since only
  `created_timestamp` has a recorded failure and only it is unformatted in the pin. Rejected:
  the two fields are the same kind of catalog metadata read by the same kind of consumer, and
  a decoder that is lenient on one list and strict on the other is a trap for anyone who
  learns the behaviour from the first. Departure (a) exists precisely so that this is a
  granted, recorded exception rather than an accident.

## References

- [`docs/specifications/wire.md`](../specifications/wire.md) — § REQ-144 (the normative rule), § REQ-095 (the authoritative-source rule and its keyed exception).
- [`resources/its-rest/definition-validation.openapi.yaml`](../../resources/its-rest/definition-validation.openapi.yaml) — the `TemplateMetadata` (`:509`, `:521-522`) and `StoredQuery` (`:3872`, `:3882-3884`) pin declarations.
- [`resources/its-rest/overview-validation.openapi.yaml`](../../resources/its-rest/overview-validation.openapi.yaml) — the *Datetime format* note (`:675`).
- [`openehr/client/definition/template.go`](../../openehr/client/definition/template.go), [`openehr/client/definition/stored_query.go`](../../openehr/client/definition/stored_query.go) — the two descriptors this decision reaches.
