// Package introspect provides an opt-in RFC 7662 token introspection
// client for SDK consumers acting as resource servers or MCP gateways.
//
// # Overview
//
// Token introspection is a resource-server concern, not a default client
// concern. Reference SMART client SDKs deliberately omit introspection;
// this package is a standalone, opt-in building block for consumers that
// need to validate opaque access tokens against an authorization server.
//
// # Standards
//
// The implementation follows:
//
//   - RFC 7662 — OAuth 2.0 Token Introspection
//     (https://www.rfc-editor.org/rfc/rfc7662)
//   - HL7 SMART App Launch — token-introspection profile
//     (https://www.hl7.org/fhir/smart-app-launch/token-introspection.html)
//
// # Usage
//
// Construct a [Client] by calling [New] with the introspection endpoint
// URL and an injected *http.Client (REQ-021). The endpoint URL is
// typically surfaced as introspection_endpoint in the authorization
// server's SMART / OpenID Connect discovery document — the
// smart/discovery resolver exposes it on AuthEndpoints, and a consumer
// can pass it directly to New.
//
// Call [Client.Introspect] with the opaque token to validate and the
// resource server's own bearer credential used to authenticate to the
// introspection endpoint. An {"active":false} response is a
// successful introspection (Result.Active == false, nil error); non-2xx
// responses are returned as wrapped errors.
//
// This package is part of REQ-062 (JWKS / token-validation surface).
package introspect
