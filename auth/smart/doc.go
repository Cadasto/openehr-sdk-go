// Package smart implements the SMART-on-openEHR auth provider: PKCE,
// authorization-code launch flow, token refresh, JWKS rotation and ID-token
// verification, returning a TokenSource compatible with the parent auth
// package.
//
// A launch from an EHR starts with [ParseEHRLaunch], which reads the
// issuer and launch value from the app's launch URL and checks the issuer
// against the app's allowlist. Each launch then keeps its own
// [AuthorizationRequest] from [Source.BeginAuthorization] through
// [Source.AuthorizeURL] to [Source.CompleteAuthorization], which checks the
// redirect back to the app and exchanges its code (returning the
// [TokenResponse] for smart/); the Source does not store per-launch
// handshake state.
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
