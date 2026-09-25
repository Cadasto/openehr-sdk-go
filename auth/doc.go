// Package auth provides the generic TokenSource abstraction and shared
// OAuth2 primitives (JWKS, discovery, scope builder) used by every
// authenticated SDK call.
//
// auth is intentionally provider-neutral. Concrete providers live in
// sub-packages: auth/smart (SMART-on-openEHR), auth/clientcreds
// (Client Credentials), auth/jwtbearer (JWT Bearer), auth/basic
// (HTTP Basic on openEHR REST). Further providers (plain OIDC,
// session cookies) can be added without changing the TokenSource contract.
package auth
