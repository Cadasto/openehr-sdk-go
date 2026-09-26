// Package jwtbearer implements the OAuth2 JWT Bearer (RFC 7523) grant
// as an auth.TokenSource. The provider exchanges a signed JWT
// assertion for an access token at the deployment's token endpoint.
//
// Two signing modes are offered:
//
//   - ClaimsSigner: the SDK signs claims with a held crypto.Signer
//     (default RS384, the SMART client-confidential-asymmetric baseline;
//     RS256, ES256 and ES384 are also supported). Use when the consumer
//     owns the private key, including opaque KMS/HSM adapters.
//   - AssertionFunc / StaticAssertion: the consumer supplies a
//     pre-signed assertion. Use when the assertion is minted by an
//     upstream identity broker.
//
// The Source caches the issued access token until it nears expiry,
// then signs a fresh assertion and re-exchanges. Concurrent Token()
// calls coalesce around a single in-flight exchange.
//
// The caller injects the *http.Client used for token-endpoint calls; there
// is no default.
package jwtbearer
