// Package demographic is the openEHR REST client for the Demographic API:
// the PARTY hierarchy (PERSON, ORGANISATION, GROUP, AGENT, ROLE) with
// versioned CRUD. Each concrete PARTY type is its own resource
// (`/demographic/{type}`); there is no generic `/demographic/party` endpoint.
// PARTY_IDENTITY and PARTY_RELATIONSHIP have no endpoints of their own; they
// live inside the PARTY body.
//
// The surface mirrors the EHR versioned-resource leaves (e.g. the composition
// client): package-level functions over a [*transport.Client], a
// [Repository] interface for dependency injection, context.Context as the
// first argument, functional options, typed transport errors, and If-Match /
// ETag optimistic concurrency on writes. Reads decode the bare PARTY body
// polymorphically by its `_type` discriminator via the type registry,
// returning the concrete type behind the [rm.Party] interface.
//
// The read-only `versioned_party` family ([GetVersionedParty],
// [GetRevisionHistory], and [GetVersion] / [GetVersionAtTime] /
// [GetVersionByID]) exposes the VERSIONED_PARTY container, its revision
// history, and the ORIGINAL_VERSION<PARTY> envelope decoded into a
// [PartyVersion] (envelope fields + the polymorphically-decoded payload).
//
// Maturity: draft. The upstream ITS-REST Demographic API is `x-status:
// DEVELOPMENT` (the development/unstable companion to the EHR API), so this
// package's surface may change between SDK minor versions until upstream
// stabilises.
package demographic
