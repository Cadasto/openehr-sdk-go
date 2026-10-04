// Package smart implements the SMART-on-openEHR auth provider: PKCE,
// authorization-code launch flow, token refresh, JWKS rotation and ID-token
// verification, returning a TokenSource compatible with the parent auth
// package.
//
// Each SMART launch keeps its own [AuthorizationRequest] (state + PKCE
// verifier) from [Source.BeginAuthorization] through
// [Source.ExchangeAuthorizationCode] (returns [TokenResponse] for
// smart/); the Source does not store per-launch handshake state.
// [Source.LastTokenResponse] holds the latest token-endpoint SMART
// fields, including after a [Source.Token] refresh; re-derive
// smart.LaunchContext when launch context may have changed.
//
// [ValidateIDToken] verifies an OpenID Connect ID token against the
// deployment's key set ([JWKS]) and returns its claims as [IDTokenClaims].
//
// The application-level SMART launch context (patient, user, encounter,
// scopes) lives in the top-level smart/ package, which re-exports the
// ID-token verification. This package covers the OAuth2/PKCE wire flow and
// the verification of the ID token it receives.
package smart
