// Package smart implements the SMART-on-openEHR auth provider: PKCE,
// authorization-code launch flow, token refresh, and JWKS rotation,
// returning a TokenSource compatible with the parent auth package.
//
// Each SMART launch keeps its own [AuthorizationRequest] (state + PKCE
// verifier) from [Source.BeginAuthorization] through
// [Source.ExchangeAuthorizationCode]; the Source does not store
// per-launch handshake state.
//
// The application-level SMART launch context (patient, user, encounter,
// scopes) lives in the top-level smart/ package — this package only
// covers the OAuth2/PKCE wire flow.
//
// See the SDK Specification proposal — SMART-on-openEHR auth library strand.
package smart
