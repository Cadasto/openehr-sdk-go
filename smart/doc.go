// Package smart is the application-level SMART context: AppContext
// (patient, user, encounter, launch parameters) and App Registration
// helpers.
//
// Distinct from auth/smart, which handles the OAuth2/PKCE flow. The
// top-level smart/ consumes a token produced by auth/smart (or any
// compatible provider) and exposes the resulting launch context to
// the application.
//
// Service discovery lives in smart/discovery — every typed client
// resolves its base URL from a ServiceCatalog returned by the
// discovery resolver.
package smart
