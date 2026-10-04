// Package smart is the application-level SMART launch surface: a typed
// [LaunchContext], ID-token claim parsing, and platform principal claims.
//
// Distinct from auth/smart, which handles the OAuth2/PKCE flow and
// returns [authsmart.TokenResponse]. After
// [authsmart.Source.CompleteAuthorization] (or
// [authsmart.Source.ExchangeAuthorizationCode]), call
// [LaunchContextFromTokenResponse] and attach the result with
// [WithLaunchContext] for handlers that need patient / encounter /
// user context. Pass the TokenResponse on unchanged: its IDTokenClaims are
// the claims the Source verified, and LaunchContextFromTokenResponse trusts
// them without verifying the ID token again.
//
// Service discovery lives in smart/discovery: every typed client
// resolves its base URL from a ServiceCatalog returned by the
// discovery resolver.
package smart
