// Package smart is the application-level SMART launch surface
// (REQ-064, REQ-067): typed [LaunchContext], ID-token claim parsing,
// and platform principal claims.
//
// Distinct from auth/smart, which handles the OAuth2/PKCE flow and
// returns [authsmart.TokenResponse]. After
// [authsmart.Source.ExchangeAuthorizationCode], call
// [LaunchContextFromTokenResponse] and attach the result with
// [WithLaunchContext] for handlers that need patient / encounter /
// user context.
//
// Service discovery lives in smart/discovery — every typed client
// resolves its base URL from a ServiceCatalog returned by the
// discovery resolver.
package smart
