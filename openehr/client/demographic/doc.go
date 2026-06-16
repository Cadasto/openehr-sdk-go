// Package demographic is the openEHR REST client for the Demographic API —
// the PARTY hierarchy (PERSON, ORGANISATION, GROUP, AGENT, ROLE) with
// versioned CRUD. Each concrete PARTY type is its own resource
// (`/demographic/{type}`); there is no generic `/demographic/party` endpoint.
// PARTY_IDENTITY and PARTY_RELATIONSHIP carry no endpoints of their own — they
// live inside the PARTY body.
//
// The surface mirrors the EHR versioned-resource leaves (e.g. the composition
// client): package-level functions over a [*transport.Client] with a
// [Repository] DI seam (REQ-023), ctx-first (REQ-020), functional options
// (REQ-022), typed transport errors (REQ-025), and If-Match / ETag optimistic
// concurrency on writes (REQ-054). Reads decode the bare PARTY body
// polymorphically by its `_type` discriminator (REQ-040) via the type
// registry, returning the concrete type behind the [rm.Party] interface.
//
// Maturity: Draft. The upstream ITS-REST Demographic API is `x-status:
// DEVELOPMENT` (the development/unstable companion to the EHR API), so this
// package's surface MAY change between SDK minor versions until upstream
// stabilises.
package demographic
