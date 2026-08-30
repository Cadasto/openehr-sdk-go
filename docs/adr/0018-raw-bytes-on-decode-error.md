# ADR 0018 — Raw response bytes on the typed 2xx decode error

- **Status:** Accepted, 2026-08-30.
- **Supersedes:** —
- **Superseded by:** —
- **Strand:** none — direct decision (this plan's Phase 0); no prior open research strand.
- **Introduces:** [REQ-151](../specifications/transport.md#req-151--typed-2xx-decode-failure) (the typed 2xx decode failure and its `Body` field). **Amends:** —
- **Plan:** [2026-08-30-read-path-decode-taxonomy.md](../plans/archive/2026-08-30-read-path-decode-taxonomy.md).
- **Related:** [ADR 0004](0004-numeric-wire-tolerance.md) (the "no strict-mode knob in v1" posture this decision follows); [REQ-093](../specifications/transport.md#req-093--openehr-error-envelope-mapping) (the PHI-safe error-surface discipline, the `WithRawErrorBodies` opt-in this decision deliberately does *not* extend, and the `WithMaxResponseBody` cap it relies on); [REQ-052](../specifications/wire.md#req-052) (the encode-only refusal sentinel — a rider delivered by the same plan, whose decision of record is the plan and § REQ-052 itself, not this ADR); [REQ-094](../specifications/transport.md#req-094--prefer-response-shape-negotiation) (cross-reference only — the write-result contract keeps every arm it owns, including the `ehr.Create` empty-body keyed exception).

## Context

When a 2xx response body cannot be decoded as the requested representation, the SDK today wraps
the decoder's error and **drops the bytes**. `transport.Decode` returns the wrapped error and the
response metadata; metadata is headers-only, so nothing on the returned triple carries the
payload. The bytes the server already delivered — already read into the process, already counted
against the response-size cap — become unrecoverable at the exact moment a caller most needs
them: a body that did not parse is a body someone has to look at.

The archived [write-result plan](../plans/archive/2026-08-18-write-result-contract.md) recorded
this as a blocker rather than solving it. It typed the write side (`NoRepresentationError`,
REQ-094) but deferred `ehr.Create`'s committed-but-unusable arm *because* EHR creation decodes
through this same shared path: typing one arm of a shared decode while the shared decode itself
stays untyped would have put the taxonomy in the wrong layer.

REQ-151 supplies the missing primitive — a `*transport.DecodeError` carrying the raw bytes. That
raises one genuinely irreversible question, which is what this ADR settles:

> Are the raw bytes attached to the error **always**, or only behind an opt-in?

The question is a one-way door in both directions. Attaching by default and gating later breaks
every consumer that came to rely on the field. Gating first and un-gating later is safe but ships
a default that still loses the payload — which is the defect this work exists to close.

It is also not obviously the same question REQ-093 already answered. REQ-093 deliberately gates
non-2xx response bodies behind `transport.WithRawErrorBodies(true)`, on the grounds that openEHR
error envelopes routinely carry patient identifiers and clinical narrative. A rule that says
"raw bodies are opt-in" would, read mechanically, cover this case too.

## Decision

**The `Body` field on `transport.DecodeError` is always populated on a 2xx decode failure. There
is no opt-in knob, and `WithRawErrorBodies` does not gate it.**

- **Always, not by default.** "Default-on" would imply a switch exists. None does: REQ-151 states
  the field as an unconditional part of the error's contract, so a caller that recovers the typed
  error can rely on the bytes being there without inspecting client configuration.
- **The 2xx body is the caller's own requested representation.** This is the whole reason the
  case differs from REQ-093's, and it is stated in Consequences below rather than restated here.
- **Always-on is affordable because the string surface is not.** The error-string discipline —
  a value-free `Error()`, the PHI warning carried by the field rather than the message — is
  REQ-151's, stated there and not decided here. This decision relies on it: because nothing the
  SDK emits by default (log lines, REQ-098 observations, REQ-090 span statuses) interpolates the
  bytes, attaching them unconditionally costs no exposure a caller did not choose.
- **Bounded by an existing control, to the extent the caller left it in place.**
  `transport.WithMaxResponseBody` (default `DefaultMaxResponseBody`, 64 MiB — REQ-093) caps how
  many bytes `Client.Do` reads, and the error can never carry more than the transport was already
  willing to read. On the default, or on any explicit positive limit, `Body` therefore introduces
  no ceiling the caller has not already chosen, and a consumer who wants a smaller one tightens
  that existing option rather than reaching for a new knob. REQ-093 also documents a **negative**
  value as an escape hatch that disables the cap outright for trusted backends; for such a client
  there is no ceiling to inherit, and the retention consequence of that combination is stated in
  Consequences rather than glossed over here.

## Consequences

- **Why 2xx decode-failure bytes are not REQ-093's non-2xx bodies.** A 2xx body is the very
  representation the caller asked for. Had it parsed, the caller would have received all of it,
  fully decoded, as the return value — so the caller is *already entitled* to those bytes, and
  handing them back on the failure path returns nothing the success path would have withheld. A
  non-2xx error body is the opposite: diagnostic content the caller never requested, generated by
  the server about a request that failed, and routinely carrying server-side clinical narrative.
  Gating that content behind an explicit opt-in is a real privacy decision; gating a caller's own
  requested representation behind one is not — it withholds from the caller only what the caller
  asked for, and only when something went wrong.
- **The bytes have already crossed the wire — but they now live longer.** They are read into the
  process either way, so the decision is about whether the caller may *see* them, not about
  whether the SDK reads them. It does change how long they survive: previously the transport read
  the body, failed to decode it, and discarded it; now the error holds it until the caller drops
  the error. Under the default cap or any explicit positive limit that retained buffer is bounded
  by the ceiling the caller chose. Under REQ-093's documented escape hatch — a **negative**
  `WithMaxResponseBody`, which disables the cap for trusted backends — it is not: such a client
  now retains an **unbounded, PHI-bearing buffer per 2xx decode failure**, where before those
  bytes were read and thrown away. This ADR accepts that cost rather than denying it. Disabling
  the cap is already an explicit statement that the caller trusts the backend's response sizes,
  the exposure is proportional to a failure that should be rare, and the remedy is the option
  that is already there — set a positive ceiling — not a second knob on the error. A consumer
  that both disables the cap and retains errors long-term should set one.
- **Retention becomes the caller's responsibility.** A `*transport.DecodeError` kept alive keeps
  its `Body` alive. Callers that log or persist errors wholesale, rather than logging `Error()`,
  now have a PHI surface they did not have before. REQ-151 documents the field so that this is a
  visible, deliberate choice; the SDK's own surfaces (`Error()`, observers, spans) do not make it.
- **The opt-in vocabulary stays at one knob.** The SDK ships `WithRawErrorBodies` and no sibling.
  A consumer reasoning about PHI exposure reads one option and one documented field, rather than
  a matrix of two knobs whose interaction has to be specified.
- **Reversing this later is a breaking change**, and is meant to be. Once consumers recover
  payloads from `Body`, gating it would silently empty a field they read. That is the same
  one-way door REQ-093 walked through in the opposite direction, walked deliberately here in the
  direction the evidence supports.
- **`transport.DecodeError` becomes the taxonomy's shared primitive.** With it in place,
  REQ-094's deferred `ehr.Create` decode-failure arm is typed by REQ-151 without routing EHR
  creation through the write-result contract; the empty-body keyed exception in REQ-094 is
  untouched and remains a separate, still-deferred amendment.

## Alternatives considered

- **Reuse `transport.WithRawErrorBodies` to gate `Body`.** One knob, one mental model, and it
  reads consistent with REQ-093 at a glance. **Rejected:** the option is off by default, so the
  default behaviour still loses the payload — the defect this work exists to close would survive
  the change, and every consumer would have to discover an option named for *error* bodies in
  order to recover a *success* body. It also conflates two different privacy questions (see
  Consequences) under one switch, so a consumer who wants raw decode payloads is forced to also
  turn on raw server error narrative, and vice versa.
- **A new opt-in `transport.WithRawDecodeBodies`.** Keeps the two questions separate and leaves
  the choice with the consumer. **Rejected:** off by default it has the same defect as the
  option above; on by default it is a knob nobody needs to touch — a no-op switch whose only
  effect is to make the contract conditional and the documentation longer. ADR 0004 set the
  precedent directly: the SDK declined a strict-decode mode in v1 rather than ship a second
  wire-strictness knob, on the grounds that the default should simply be right. The same
  reasoning applies here, and the default that is right is the one that returns the caller's own
  bytes.
- **Return the bytes as a second return value instead of a field on the error.** No new PHI
  surface on the error type. **Rejected:** it changes the `(*T, *Metadata, error)` triple every
  leaf already returns (REQ-094 depends on that shape), and it would hand bytes back on the
  success path too, where they are redundant. The failure is the only occasion the bytes are
  needed, so the error is where they belong.
