// Package auth provides the generic TokenSource abstraction and shared
// OAuth2 primitives (JWKS, discovery, scope builder) used by every
// authenticated SDK call.
//
// auth is intentionally provider-neutral. Concrete providers live in
// sub-packages: auth/smart (SMART-on-openEHR), auth/clientcreds
// (Client Credentials), auth/jwtbearer (JWT Bearer), auth/basic
// (HTTP Basic on openEHR REST). Further providers (plain OIDC,
// session-cookie) MAY be added without disturbing the TokenSource contract.
//
// Implements REQ-060, REQ-066, REQ-068 (partial: clientcreds, jwtbearer),
// and REQ-069 (auth/basic) per docs/specifications/auth.md. SMART PKCE (REQ-061..064)
// lives in auth/smart (planned).
package auth
