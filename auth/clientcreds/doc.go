// Package clientcreds implements the OAuth2 Client Credentials grant
// (RFC 6749 §4.4) as an auth.TokenSource for service-to-service
// callers (batch jobs, data loaders, MCP server backends, federating
// gateways) that do not run an interactive user flow.
//
// The provider caches the issued access token until it nears expiry,
// then re-requests. Client Credentials does not produce a refresh
// token; "refresh" here means "request a new access token via the same
// grant". Concurrent Token() calls coalesce around a single in-flight
// request.
//
// Callers must inject the *http.Client whose timeouts and TLS roots they
// want to apply to the token endpoint. A nil http.Client is rejected at
// construction.
package clientcreds
