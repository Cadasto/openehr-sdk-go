// Package auth provides the generic TokenSource abstraction and shared
// OAuth2 primitives (JWKS, discovery, scope builder) used by every
// authenticated SDK call.
//
// auth is intentionally provider-neutral. Concrete providers live in
// sub-packages: auth/smart (SMART-on-openEHR), auth/clientcreds
// (Client Credentials), auth/jwtbearer (JWT Bearer). Non-SMART providers
// (Basic, plain OIDC, session-cookie) are addressable later by adding
// further sub-packages without disturbing the TokenSource contract.
//
// Implements REQ-060, REQ-066, and REQ-068 (partial: clientcreds, jwtbearer)
// per specs/auth.md. SMART PKCE (REQ-061..064) lives in auth/smart (planned).
package auth
