# Plan — Auth / SMART-on-openEHR conformance audit and polish

> **For agentic workers:** REQUIRED SUB-SKILL: use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. This plan touches **normative specs** — follow the spec-driven workflow in [AGENTS.md](../../AGENTS.md) (run `make spec-context REQ=NNN`, edit the canonical topic spec, then the REQ row + `traceability.yaml`). Do **not** resolve [STRAND-05](../specifications/research-strands.md#strand-05--smart-on-openehr-auth-library) silently in code — surface decisions or land an ADR.

**Date:** 2026-06-16
**Status:** Implemented (2026-06-18)
**Owner:** SDK maintainers
**Covers:** REQ-063, REQ-068, REQ-064, REQ-070, REQ-072, REQ-061, REQ-062 (canonical-spec sections only — no duplicate normative prose)
**Probes:** PROBE-001..009, PROBE-041, PROBE-007
**Implementation:** landed — all findings F-A..F-M closed on branch `chore/auth-smart-audit`; ADR 0008 + 0009 landed; STRAND-05 resolved. `make ci` green (note: `test-race` requires CGO and runs in CI).
**Depends on:** landed `auth/`, `auth/smart/`, `auth/clientcreds/`, `auth/jwtbearer/`, `smart/`, `smart/discovery/`, `transport/`
**Defers:** dynamic client registration, MTLS/FAPI/JAR/PAR, token revocation client, UDAP/DPoP refresh-token binding, SMART App State

> **Decision update (2026-06-17):** The owner relaxed the OTel-only dependency rule for this work. `golang.org/x/oauth2` and `github.com/coreos/go-oidc/v3` (transitively `go-jose/v4`) are adopted. Phases 3/3e/4/5 therefore USE these libraries (go-oidc for id-token verification, go-jose for `client_assertion` JWS signing, x/oauth2 for token-source/PKCE/`RetrieveError` patterns) rather than hand-rolling — superseding the "no new deps / hand-roll" framing in the Go-idiom cross-check section below. G-6's x/oauth2 adapter is no longer blocked. Recorded formally in ADR 0009 (Phase 6).

## Goal

Close the gap between the SDK's auth/SMART implementation and the **canonical openEHR "SMART on openEHR" specification** (`specifications.openehr.org/releases/ITS-REST/development/smart_app_launch.html`) and the HL7 **SMART App Launch v2.2** framework it profiles. Fix one wire-shape interop bug (discovery `services`), surface openEHR-specific launch context (`ehrId`/`episodeId`), complete confidential-client and 401→refresh coverage flagged by REQ-063/REQ-068, and record the canonical sources + an ADR so the open SMART strand is closed against evidence.

---

## Audit findings

Severity: **HIGH** = breaks interop with a spec-compliant platform; **MED** = spec coverage/claims gap; **LOW** = docs/ergonomics.

### F-A (HIGH) — Discovery `services` wire shape diverges from the canonical spec

The canonical openEHR SMART spec (§ Service Discovery) defines `services` as a **JSON object/hash map** keyed by reverse-domain id, where each value carries **`baseUrl`** (camelCase), `description`, `documentation`, `openapi`:

```json
"services": {
  "org.openehr.rest": { "baseUrl": "https://platform.example.com/openehr/rest/v1", "description": "…" },
  "org.fhir.rest":    { "baseUrl": "https://platform.example.com/" }
}
```

The SDK decodes `services` as a **JSON array** with snake_case `base_url` plus an invented `spec_version` field:

```28:319:smart/discovery/resolver.go
Services []serviceEntryWire `json:"services"`
// …
type serviceEntryWire struct {
	ID           string   `json:"id"`
	BaseURL      string   `json:"base_url"`
	SpecVersion  string   `json:"spec_version"`
	Capabilities []string `json:"capabilities"`
}
```

Consequences against a real spec-compliant platform:
- `services` decodes to empty → `validate` reports `org.openehr.rest` missing → every deployment fails discovery.
- The SDK's own fixture `testkit/cassettes/its_rest/discovery/smart-configuration.json` uses the **array** shape, so all `resolver_test.go` cases pass against a non-canonical document — the deviation is masked, not caught.
- `spec_version` per service is **not** in the canonical document, so the per-service REQ-072 version gate is unsatisfiable against a compliant platform.

Note the domain model (`ServiceCatalog.Services map[string]ServiceEntry`) is already a map — only the wire decoder and the version-gate source are wrong.

**Decision to record (ADR):** (a) adopt the canonical map shape as the parsed wire form; (b) where to source the openEHR REST spec version now that `services[].spec_version` is not canonical (recommendation: stop gating on a service-level `spec_version`, treat absence as acceptable, and keep the pin check only when a deployment opts to advertise one via a capability/extension — softens REQ-072); (c) whether to remain tolerant of the legacy array shape for one release.

### F-B (MED) — openEHR launch-context claims `ehrId` / `episodeId` not surfaced

The canonical spec conveys EHR-level context via the **`ehrId`** token claim (requested via `launch/patient`) and Episode context via **`episodeId`** (requested via `launch/episode`, experimental). The SDK only parses FHIR-style `patient` / `encounter` / `fhirUser` (`auth/smart/exchange.go`) and `smart.LaunchContext` has no `EHRID` / `EpisodeID` fields — these reach consumers only untyped via `Raw`. REQ-064 names `Patient/Encounter/User` but not the openEHR-native context keys.

### F-C (HIGH) — No `private_key_jwt` (asymmetric) client authentication; SMART Backend Services flow absent

The canonical spec advertises `token_endpoint_auth_methods_supported: ["client_secret_basic", "private_key_jwt"]`, and the HL7 `client-confidential-asymmetric` profile (used by **both** SMART App Launch confidential clients **and** SMART Backend Services) requires `private_key_jwt`: a `client_assertion_type=urn:ietf:params:oauth:client-assertion-type:jwt-bearer` + signed `client_assertion`. The SDK has no path that emits `client_assertion`:

- `auth/smart` confidential exchange only does `client_secret_basic` (`req.SetBasicAuth` in `postToken`).
- `auth/clientcreds` does `client_credentials` but only `client_secret_basic`/`client_secret_post` — **no** `client_assertion`. So the **SMART Backend Services** wire shape (`grant_type=client_credentials` **+** `client_assertion`) cannot be produced.
- `auth/jwtbearer` is the RFC 7523 *authorization grant* (`grant_type=urn:...:jwt-bearer`, form field `assertion`), which is a **different** flow from SMART Backend Services' client-credentials-with-client-assertion. It is, however, the openEHR spec's "JWT Bearer Token Grant (preferred)" for backend — so it stays valid for openEHR, but does **not** satisfy strict SMART-on-FHIR backend conformance.

Net: REQ-068's confidential authorization-code (asymmetric) and the SMART Backend Services profile are **not covered**, despite the roadmap marking `clientcreds`/`jwtbearer` as REQ-068 backend. This is the second interop headline after F-A.

### F-I (HIGH) — Asymmetric signing algorithm baseline (RS384/ES384) unmet

`auth/jwtbearer/assertion.go` `ClaimsSigner` is **RS256-only** (hardcoded `sha256` / `crypto.SHA256`, construction rejects any `alg != "RS256"` and any non-RSA signer). The HL7 `client-confidential-asymmetric` profile mandates that clients **SHALL support both `RS384` and `ES384`**, and servers advertise `token_endpoint_auth_signing_alg_values_supported` with at least one of them. RS256 is **not** in the SMART baseline, so a strict SMART authorization server rejects the SDK's assertions. Any `private_key_jwt` work (F-C) depends on fixing this.

### F-D (MED, flagged) — REQ-063: transport 401 → refresh not wired; couples with PROBE-041

`transport/` maps `401` to `ErrUnauthorized` and never re-drives `auth/smart` refresh or `discovery.Resolver.Refresh`. `auth.md § REQ-063` documents this as out-of-scope for v1; PROBE-007 (token refresh transparent) and PROBE-041 (catalog refresh on 401) are only half-implemented (discovery layer). Proactive refresh on `Source.Token()` works; wire-triggered re-auth does not. **Framing (per G-1):** proactive expiry-based refresh — which the SDK already does and which is exactly what `x/oauth2`'s `reuseTokenSource` does — is the *primary* mechanism; `oauth2.Transport` itself does **no** 401-reactive refresh. So this finding is a complementary **safety net** for clock skew / server-side early revocation, not a missing core feature. It is a single shared mechanism (a transport reauth/refresh hook) that closes REQ-063's open sub-point + REQ-071 bullet 3 + PROBE-007 + PROBE-041 together.

### F-E (MED) — REQ-068 coverage unproven: no auth conformance probes

`testkit/probes/auth/` does not exist; PROBE-001..009 are `planned`. The four flows (PKCE public, confidential code, client-credentials, jwt-bearer) and three launch modes (standalone, embedded, backend) are implemented but not asserted by probes, so REQ-068 stays `partial`.

### F-F (LOW) — No scope/context helpers; `offline_access`/`online_access` undocumented

`auth.BuildScope` covers `<compartment>/<resource>.<permission>` but there are no constants/helpers for launch-context scopes (`launch`, `launch/patient`, `launch/episode`, `openid`, `profile`) or the refresh-token request scopes (`offline_access`, `online_access`). `auth.md` never mentions how a refresh token is requested (these scopes), even though REQ-063 depends on one being granted.

### F-G (LOW / DOC) — Canonical sources not recorded; extra discovery endpoints dropped

`auth.md` / `service-discovery.md` do not cite the canonical openEHR SMART spec URL, and STRAND-05 is still `Open` with no ADR. The resolver ignores `introspection_endpoint`, `revocation_endpoint`, `management_endpoint` and the openEHR capabilities (`context-openehr-ehr`, `context-openehr-episode`, `openehr-permission-v1`, `launch-base64-json`) the spec defines.

### F-H (LOW) — No runnable SMART/auth example

`cmd/examples/` has no auth/SMART program, although the building blocks (`auth/clientcreds`, `auth/basic`, `auth/smart`) are landed and `docs/examples.md` advertises a runnable catalog. The public **SMART Launcher** (`launch.smarthealthit.org`) and **Aidbox**/**Smartbox** make good manual integration targets for such an example.

### F-J (MED) — No token-introspection (RFC 7662) support

The HL7 `token-introspection` profile says SMART EHRs **SHOULD** support RFC 7662 introspection, and the openEHR spec advertises an `introspection_endpoint`. The SDK has no introspection client, so a consumer holding an **opaque** access token cannot validate it or recover its `scope` / `patient` / launch-context / `exp` without a bespoke call. `auth.md § REQ-062` alludes to "opaque access tokens via introspection" but nothing implements it, and the resolver discards `introspection_endpoint`. **Calibration:** the reference *client* SDKs (`smart-on-fhir/client-js`, `client-py`) deliberately omit introspection — it is primarily a resource-server concern — so keep this **opt-in**, aimed at SDK-as-resource-server / MCP-gateway consumers, not the default client path.

### F-K (LOW) — Discovery drops asymmetric-auth + extra endpoint metadata

`smartConfigWire` ignores `token_endpoint_auth_signing_alg_values_supported` (needed to pick RS384 vs ES384 for F-C/F-I) and `introspection_endpoint` / `revocation_endpoint` / `management_endpoint` (F-J and revocation hygiene). These are advertised by the canonical example document but never parsed onto `AuthEndpoints`.

### Best-practice notes (non-blocking, fold into docs)

- **Refresh-token single use / rotation** (best-practices, OAuth 2.1 §6.1): servers may revoke on reuse; the SDK already replaces the stored `refresh_token` with a server-issued new one and keeps the prior only when the server omits one — correct, but should be documented under REQ-063.
- **Short backend token lifetimes** (`expires_in` SHOULD ≤ 300): the proactive 30s refresh threshold is fine; note it.

### F-L (MED) — Refresh failure does not clear the stored refresh token (loop risk)

Cross-checking against `smart-on-fhir/client-js`: its `client.refresh()` **deletes the refresh token from state when refresh fails** ("so that we don't enter into loops trying to re-authorize"), and `client.request()` auto-refreshes then retries. The Go `auth/smart` `Source.Token()` returns `ErrReauthRequired`/`ErrRefreshFailed` on failure but **does not clear `s.refresh`**, so a caller that retries `Token()` will re-attempt the same (already-rejected, e.g. `invalid_grant`) refresh in a loop. The reference clients also expose an explicit `refreshIfNeeded` (10s window) in addition to per-request auto-refresh. This refines F-D. **Correctness gate (per G-5):** the refresh token MUST be cleared only on a **terminal** failure — a 4xx with an RFC 6749 `invalid_grant`/`invalid_client` envelope — and MUST be **retained** on transient 5xx/network errors (otherwise a blip forces a full re-auth). This requires the token-endpoint error to carry the HTTP status, which the current `auth.OAuth2Error` does not.

### Reference client SDK cross-check (what they confirm)

Validated the plan against the SMART-maintained reference clients `smart-on-fhir/client-js` (v2) and `client-py` (and the `cumulus-fhir-support` network client they recommend for "more auth options and built-in retries"):

- **Auto-refresh + retry on 401 is table-stakes** — `client.request()` refreshes transparently and retries; confirms F-D is core behaviour, not optional polish.
- **Confidential auth = `clientPrivateJwk` with `alg` ∈ {`RS384`,`ES384`}** — confirms F-C/F-I exactly (the reference client rejects other algorithms).
- **PKCE is capability-gated** (`pkceMode: ifSupported|required|disabled`, keyed on `code_challenge_methods_supported`). The Go SDK is always-on S256 (stricter; matches openEHR's S256 mandate) — acceptable, but worth a documented `PKCEMode`-style note rather than silent always-on.
- **State across the redirect must be persisted** — the reference clients store `state` + PKCE `code_verifier` in session storage between `authorize()` and `ready()`. The Go SDK pushes this onto the caller (retain `AuthorizationRequest`); the Phase 6 example MUST demonstrate this persistence (the #1 integration gotcha).
- **Multi-EHR launch** — client-js supports an array of configs + `issMatch` to pick config by incoming `iss`. The Go equivalent is per-issuer clients (REQ-065) + per-request `auth.WithTokenSource` + issuer-keyed discovery cache; a small "resolve Source/catalog by incoming `iss`" helper would serve the MCP/multi-EHR use case (consideration, not a required phase).
- **Token introspection is NOT in the reference clients** — reinforces keeping F-J opt-in (resource-server scope).

### F-M (MED) — ID-token verification is RS256-only and ignores discovery alg metadata

`smart.ValidateIDToken` (`smart/idtoken.go`) hard-rejects any header `alg != "RS256"` and only builds RSA keys (`rsaPublicKeyFromJWK`). This is the **verify-side twin of F-I**: SMART/OIDC servers advertise `id_token_signing_alg_values_supported` and may sign with `RS384`/`ES256`/`ES384`. go-oidc's canonical verifier defaults to RS256 **only as a last resort** and otherwise constrains the accepted set to the provider's advertised algorithms. The SDK should derive its allowlist from discovery (at minimum RS256 + RS384 + ES256 + ES384), keep the existing "verify signature **before** reading claims" ordering (already correct, matches go-oidc), and keep the alg-confusion guard (header `alg` must be in the discovery-derived allowlist **and** match the JWK `kty` — never trust the header blindly). Shares the JOSE alg-agility helper with F-I.

### Go OAuth2 / OIDC idiom cross-check (`golang.org/x/oauth2` + `go-oidc`)

**Hard constraint:** `AGENTS.md` pins OpenTelemetry as the **only** third-party runtime dependency, and `idiom.md` mandates no-reflection / ctx-first / injected `*http.Client`. So the deliverable is to mirror the *patterns and correctness* of the canonical libraries in the existing hand-rolled code — **not** to add `x/oauth2`/`go-oidc`/`go-jose` as deps. Cross-checked against `oauth2.go`, `token.go`, `transport.go`, `clientcredentials`, `pkce.go`, and `go-oidc/oidc/verify.go`:

- **G-1 — Proactive-expiry refresh is the canonical pattern; 401-reactive is complementary (reframes F-D).** `x/oauth2`'s `reuseTokenSource.Token()` refreshes on **expiry** under a mutex (callers coalesce), and `oauth2.Transport.RoundTrip` does **not** do 401-reactive refresh. So the SDK's existing proactive `Source.Token()` refresh IS the idiomatic primary mechanism; F-D's transport 401→reauth hook is a *safety net* for clock skew / server-side early revocation, not the missing core. State F-D that way so it isn't over-scoped. Confirms REQ-026 coalescing (ensure single-flight, like the mutex-guarded `reuseTokenSource`).
- **G-2 (LOW→MED) — Make the early-expiry buffer configurable.** `x/oauth2` uses a 10s `defaultExpiryDelta` and exposes `ReuseTokenSourceWithExpiry(earlyExpiry)`. The SDK hardcodes 30s in `smart.Source`. Expose it as a functional option (default 30s) so deployments with short backend token lifetimes (`expires_in` ≤ 300) can tune it; aligns with the JS client's 10s `refreshIfNeeded`.
- **G-3 (MED) — Select client-auth method from discovery, not by trial-and-error.** `x/oauth2` auto-detects `AuthStyleInHeader` vs `AuthStyleInParams` by probing and caching. SMART discovery already advertises `token_endpoint_auth_methods_supported` (`client_secret_basic`/`client_secret_post`/`private_key_jwt`), so the SDK should **pick deterministically from metadata** (no failed probe round-trip). Ties F-C ↔ F-K: the resolver must surface `token_endpoint_auth_methods_supported`, and the confidential-auth code honours it.
- **G-4 (MED) — see F-M:** generalise id-token verification to a discovery-driven alg allowlist, sharing the JOSE helper with F-I.
- **G-5 (LOW→MED) — Token-endpoint error must carry HTTP status for terminal-vs-retryable classification (powers F-L).** `x/oauth2.RetrieveError` retains `Response` + `Body` + RFC 6749 `error`/`error_description`/`error_uri`. The SDK's `auth.OAuth2Error` carries the envelope but not the HTTP status. F-L's "clear the refresh token" decision MUST fire only on a **terminal** OAuth2 error (4xx with `invalid_grant`/`invalid_client`), never on transient 5xx/network — so the exchange path needs the status to classify. Add a bounded-body status-aware error (mirroring `RetrieveError`).
- **G-6 (consideration → STRAND/ADR) — `x/oauth2` interop adapter.** Many Go consumers already hold an `oauth2.TokenSource` (Google/Azure/`clientcredentials`). A thin `auth.FromOAuth2TokenSource` adapter would let them plug in. **But** (a) `oauth2.TokenSource.Token()` has **no `ctx`** (a known wart) so the adapter must bridge ctx via closure/`oauth2.NewClient(ctx, …)`, and (b) it would add `x/oauth2` to `go.mod`, violating the OTel-only rule. Record as a research strand / ADR decision (e.g. isolate in an optional `auth/oauth2adapter` submodule, or decline) — do **not** silently add the dep.
- **G-7 (validate) — PKCE parity with RFC 7636.** `x/oauth2.GenerateVerifier` = 32 random octets → `base64.RawURLEncoding` (43 chars); challenge = `base64url(SHA256(verifier))`, method `S256`. Add a Phase-5 assertion that the SDK's verifier matches (≥32 bytes entropy, raw-url base64, S256) — likely already correct, but unproven.
- **G-8 (cross-cutting acceptance) — ctx-first + injected `*http.Client` on every new auth path.** `JWKS` already takes an injected client; the new confidential-auth, introspection, and alg-agility code MUST do the same (no `http.Client` allocation; ctx threaded through), matching both `idiom.md` and `x/oauth2`'s contextual-client convention.

> Note: `go-oidc`'s `Verify` deliberately leaves **nonce** validation to the caller and uses a 5-min `nbf` leeway; the SDK already validates nonce and uses a tighter 30s skew — keep the SDK's stricter behaviour.

### Findings → REQ / probe map

| Finding | Severity | Touches | REQ / PROBE |
|---|---|---|---|
| F-A services map shape | HIGH | `smart/discovery` | REQ-070, REQ-072 + ADR |
| F-C private_key_jwt + Backend Services | HIGH | `auth/smart`, `auth/clientcreds` | REQ-068 |
| F-I RS384/ES384 signing baseline | HIGH | `auth/jwtbearer` | REQ-068 |
| F-M id_token verify alg agility (RS256-only) | MED | `smart` (idtoken), `smart/discovery` | REQ-062, REQ-064 |
| F-B ehrId/episodeId (+ intent/style/banner) | MED | `auth/smart`, `smart` | REQ-064 |
| F-D 401→reauth (safety net; proactive refresh is primary) | MED | `transport`, `auth/smart`, `smart/discovery` | REQ-063, REQ-071, PROBE-007, PROBE-041 |
| F-L clear refresh on terminal failure + RefreshIfNeeded | MED | `auth/smart` | REQ-063 |
| G-2 configurable early-expiry buffer | LOW | `auth/smart` | REQ-063 |
| G-3 client-auth method from discovery | MED | `smart/discovery`, `auth/smart`, `auth/clientcreds` | REQ-068, REQ-070 |
| G-5 status-aware token-endpoint error | LOW | `auth`, `auth/smart` | REQ-063 |
| G-6 x/oauth2 interop adapter (decision) | — | `auth` (opt) | new STRAND/ADR |
| F-E auth probes | MED | `testkit/probes/auth` | PROBE-001..009 |
| F-J token introspection | MED | `auth` (new `auth/introspect`), `smart/discovery` | REQ-062 (+ new REQ) |
| F-F scope/context helpers | LOW | `auth` | REQ-061, REQ-063 (doc) |
| F-G canonical sources / endpoints | LOW | specs, `smart/discovery` | STRAND-05, REQ-062 |
| F-K discovery alg/endpoint metadata | LOW | `smart/discovery` | REQ-062, REQ-070 |
| F-H SMART example | LOW | `cmd/examples` | — |

**Validated against:** openEHR *SMART on openEHR* (ITS-REST development); HL7 *SMART App Launch v2.2* (`app-launch`, `scopes-and-launch-context`, `client-confidential-asymmetric`, `backend-services`, `token-introspection`, `best-practices`); **Inferno SMART App Launch Test Kit STU2.2** (notably the *STU2.2 Client* suite — the SDK is a client); **SMART Launcher** sandbox; **Aidbox/Smartbox** SMART-on-FHIR docs as a reference deployment.

## Implementation checklist

| Step | Status |
|---|---|
| Spec / registry updated (`traceability.yaml`, REQ.md rows, ADR for STRAND-05) | ✅ ADR 0008 + 0009; STRAND-05 resolved |
| Code | ✅ all findings F-A..F-M |
| Tests with `// REQ-` / `// PROBE-` comments | ✅ incl. PROBE-001..009 |
| Canonical-shaped discovery fixture added | ✅ |
| `make spec-check` | ✅ OK |
| `make ci` | ✅ green (`test-race` runs in CI — needs CGO) |

---

## Phase 1 — Discovery `services` map shape (F-A) [HIGH]

**Outcome:** the resolver parses a spec-compliant openEHR SMART configuration document.

**Files:**
- Test: `smart/discovery/resolver_test.go`
- Fixture: `testkit/cassettes/its_rest/discovery/smart-configuration.json` (Modify) + add `smart-configuration-legacy-array.json` if back-compat kept
- Modify: `smart/discovery/resolver.go` (`smartConfigWire`, `serviceEntryWire`, `parse`, `validate`)
- ADR: `docs/adr/0008-smart-discovery-services-shape.md` (Create) + `docs/adr/README.md` (Modify)
- Specs: `docs/specifications/service-discovery.md`, `docs/specifications/REQ.md` (REQ-070/072 rows), `docs/specifications/traceability.yaml`

- [x] **Step 1: Write the failing test.** Add `TestResolveCanonicalServicesMap` to `resolver_test.go` that serves a document whose `services` is the canonical **object** form (`"services": {"org.openehr.rest": {"baseUrl": "<srv>/openehr/v1"}}`) and asserts `cat.Service("org.openehr.rest")` is present with the parsed `BaseURL`.

```go
func TestResolveCanonicalServicesMap(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{
		  "issuer": %q,
		  "authorization_endpoint": %q, "token_endpoint": %q,
		  "services": {"org.openehr.rest": {"baseUrl": %q}}
		}`, srv.URL, srv.URL+"/authorize", srv.URL+"/token", srv.URL+"/openehr/v1")
	}))
	defer srv.Close()
	res, err := NewResolver(nil, WithHTTPClient(srv.Client()),
		WithAcceptedSpecVersions("1.1.0-development", ""))
	if err != nil { t.Fatal(err) }
	cat, err := res.Resolve(context.Background(), srv.URL)
	if err != nil { t.Fatalf("resolve: %v", err) }
	e, ok := cat.OpenEHRRest()
	if !ok || e.BaseURL == nil { t.Fatalf("org.openehr.rest missing: %#v", cat.Services) }
}
```

- [x] **Step 2: Run it; verify it fails.** `cd .worktrees/auth-smart-audit && go test ./smart/discovery/ -run TestResolveCanonicalServicesMap -v` — expect FAIL (services empty → `ReasonMissingService`).

- [x] **Step 3: Change the wire decoder to a map.** In `smart/discovery/resolver.go`:

```go
type smartConfigWire struct {
	Issuer                            string                      `json:"issuer"`
	AuthorizationEndpoint             string                      `json:"authorization_endpoint"`
	TokenEndpoint                     string                      `json:"token_endpoint"`
	JWKSURI                           string                      `json:"jwks_uri"`
	RegistrationEndpoint              string                      `json:"registration_endpoint"`
	IntrospectionEndpoint             string                      `json:"introspection_endpoint"`
	RevocationEndpoint                string                      `json:"revocation_endpoint"`
	ManagementEndpoint                string                      `json:"management_endpoint"`
	ScopesSupported                   []string                    `json:"scopes_supported"`
	ResponseTypesSupported            []string                    `json:"response_types_supported"`
	CodeChallengeMethodsSupported     []string                    `json:"code_challenge_methods_supported"`
	GrantTypesSupported               []string                    `json:"grant_types_supported"`
	TokenEndpointAuthMethodsSupported []string                    `json:"token_endpoint_auth_methods_supported"`
	Capabilities                      []string                    `json:"capabilities"`
	Services                          map[string]serviceEntryWire `json:"services"`
}

type serviceEntryWire struct {
	BaseURL       string   `json:"baseUrl"`
	SpecVersion   string   `json:"spec_version"` // non-canonical extension; tolerated when present
	Description   string   `json:"description"`
	Documentation string   `json:"documentation"`
	OpenAPI       string   `json:"openapi"`
	Capabilities  []string `json:"capabilities"`
}
```

Update `parse` to range over the map (`for id, s := range wire.Services`), use `s.BaseURL`, set `ServiceEntry.ID = id`. Reject an empty `baseUrl` with `ReasonMalformedURL`.

- [x] **Step 4: Soften the per-service version gate (decision from ADR).** In `validate`, when `e.SpecVersion == ""` and the accepted set was not explicitly narrowed by the caller, do **not** raise `ReasonSpecVersionMismatch`; only enforce when the entry advertises a version. Keep strict behaviour reachable for callers that pass `WithAcceptedSpecVersions(...)`. Document this in `service-discovery.md` REQ-072 section.

- [x] **Step 5: Update the default fixture to canonical shape.** Rewrite `testkit/cassettes/its_rest/discovery/smart-configuration.json` so `services` is the object form with `baseUrl`. If back-compat is kept (ADR decision), add `smart-configuration-legacy-array.json` and a tolerant-decode test; otherwise drop array support.

- [x] **Step 6: Run the suite.** `go test ./smart/discovery/... -v` — expect PASS. Then `go test ./transport/... ./...` for anything that built catalogs from the old fixture.

- [x] **Step 7: Land the ADR + spec/registry updates.** Write `docs/adr/0008-smart-discovery-services-shape.md` (decision, alternatives, back-compat); add it to `docs/adr/README.md`. Cite the canonical spec URL in `service-discovery.md`. Update `traceability.yaml` REQ-070/072 (add fixture + new test files). Run `make spec-check`.

- [x] **Step 8: Commit.** `git add -A && git commit -m "fix(discovery): parse SMART services as canonical map (REQ-070); ADR 0008"`

## Phase 2 — openEHR launch-context claims `ehrId` / `episodeId` (F-B) [MED]

**Files:**
- Modify: `auth/smart/exchange.go` (`TokenResponse`, `tokenResponse`, `ParseTokenResponse`)
- Modify: `smart/principal.go` or `smart/context.go` (`LaunchContext` fields) + `smart/launch.go` (mapping)
- Test: `auth/smart/exchange_test.go`, `smart/launch_test.go`
- Spec: `docs/specifications/auth.md` (REQ-064 section), `REQ.md` row note, `traceability.yaml`

- [x] **Step 1: Failing test (auth/smart).** Add `TestParseTokenResponseOpenEHRClaims` asserting `ParseTokenResponse([]byte(`{"access_token":"a","ehrId":"ehr-1","episodeId":"ep-9"}`))` yields `EHRID == "ehr-1"` and `EpisodeID == "ep-9"`.
- [x] **Step 2: Run; verify fail** — `go test ./auth/smart/ -run TestParseTokenResponseOpenEHRClaims -v` (fields don't exist).
- [x] **Step 3: Add fields.** Add `EHRID string` and `EpisodeID string` to `TokenResponse` and `tokenResponse` (`json:"ehrId"`, `json:"episodeId"`); copy them in `ParseTokenResponse`. Also capture the SMART-compat launch-context extras the reference clients surface — `intent` (`json:"intent"`), `smart_style_url`, `need_patient_banner`, `tenant` — when present.
- [x] **Step 4: Failing test (smart).** In `smart/launch_test.go` assert `LaunchContextFromTokenResponse` copies `EHRID`/`EpisodeID` (and `Intent`/`SMARTStyleURL`/`NeedPatientBanner`/`Tenant`) onto `LaunchContext`.
- [x] **Step 5: Add `LaunchContext.EHRID` / `LaunchContext.EpisodeID`** (plus `Intent`, `SMARTStyleURL`, `NeedPatientBanner`, `Tenant`) and map them in `smart/launch.go` (keep `Raw` carrying them too). Keep `Patient`/`Encounter` for FHIR-compat per the spec's compatibility goal.
- [x] **Step 6: Run** `go test ./auth/smart/... ./smart/... -v` — expect PASS.
- [x] **Step 7: Spec.** Extend REQ-064 in `auth.md` to name `EHRID`/`EpisodeID` (cite the canonical spec's `ehrId`/`episodeId` claims). Update `traceability.yaml`.
- [x] **Step 8: Commit** `feat(smart): surface openEHR ehrId/episodeId launch context (REQ-064)`.

## Phase 3 — Asymmetric client authentication: RS384/ES384 + `private_key_jwt` + SMART Backend Services (F-I, F-C) [HIGH]

**Outcome:** the SDK can authenticate to the token endpoint with `private_key_jwt` using SMART's mandated `RS384`/`ES384` algorithms, on (a) the confidential authorization-code exchange and (b) a `client_credentials`-grant backend-services flow.

**Files:**
- Modify: `auth/jwtbearer/assertion.go` (algorithm support), `auth/jwtbearer/jwtbearer_test.go`
- Modify: `auth/smart/source.go` (Config + `postToken`), `auth/smart/smart_test.go`
- Modify: `auth/clientcreds/clientcreds.go` (assertion-based client auth), `auth/clientcreds/clientcreds_test.go`
- Spec: `auth.md` REQ-068, `service-discovery.md` (alg metadata), `traceability.yaml`

### 3a — RS384/ES384 in the signer (F-I)

- [x] **Step 1: Failing tests.** In `jwtbearer_test.go` add `TestClaimsSignerRS384` and `TestClaimsSignerES384`: build a `ClaimsSigner` with `WithAlgorithm("RS384")` (RSA key) and `WithAlgorithm("ES384")` (ECDSA P-384 key), produce an assertion, decode the JWS header and assert `alg` matches, and verify the signature with the public key.
- [x] **Step 2: Run; verify fail** (`RS384`/`ES384` rejected at construction today).
- [x] **Step 3: Implement.** Replace the RS256-only path: map `alg → (crypto.Hash, signer-type)`. Support `RS384` (RSA + SHA-384), `ES384` (ECDSA P-384 + SHA-384, encode r||s fixed-width per JOSE), keep `RS256` for back-compat. Validate the key type matches `alg`. For ECDSA, sign the hash and serialise `r`/`s` to 48-byte big-endian halves. Make the documented **default `RS384`** (SMART baseline) while still accepting an explicit `RS256`. **Factor the `alg → (hash, key-type)` map and the JWS sign/verify primitives into a small unexported helper (e.g. `internal/jose` or shared funcs in `auth`) so the F-M id-token *verifier* (3e) reuses the exact same alg table — one source of truth for the supported algorithm set, no reflection.**
- [x] **Step 4: Run** `go test ./auth/jwtbearer/... -v` — PASS.

### 3b — `private_key_jwt` on the SMART authorization-code exchange (F-C)

- [x] **Step 5: Failing test.** `TestExchangeWithPrivateKeyJWT` in `smart_test.go`: configure a `Source` with a client-assertion signer (no `client_secret`); assert the token POST body carries `client_assertion_type=urn:ietf:params:oauth:client-assertion-type:jwt-bearer` and a `client_assertion` whose decoded claims have `iss==sub==client_id`, `aud==token endpoint`, a `jti`, and `exp ≤ now+5m`; assert **no** HTTP Basic header.
- [x] **Step 6: Run; verify fail.**
- [x] **Step 7: Implement.** Add `smart.WithClientAssertionKey(signer crypto.Signer, alg, kid string)` (build a `jwtbearer.ClaimsSigner` internally with `aud = TokenEndpoint`, `iss = sub = ClientID`). In `postToken`, when a signer is configured, set the two `client_assertion*` form fields instead of `SetBasicAuth`; `client_secret_basic` stays the fallback when only a secret is set; public clients (neither) send nothing. **(G-3) Drive method selection from discovery:** prefer the auth method advertised in `token_endpoint_auth_methods_supported` (surfaced in Phase 6 / F-K) — pick `private_key_jwt` when a signer is set and the server lists it; do **not** trial-and-error like `x/oauth2`'s `AuthStyleAutoDetect`. If discovery and the configured credential disagree, fail fast with `ErrInvalidConfig`.
- [x] **Step 8: Run** `go test ./auth/smart/... -v` — PASS.

### 3c — SMART Backend Services (`client_credentials` + `client_assertion`) (F-C)

- [x] **Step 9: Failing test.** In `clientcreds_test.go` add `TestClientCredentialsWithClientAssertion`: construct a `Source` via a new `WithClientAssertion(jwtbearer.AssertionSource)` (no secret) and assert the POST body has `grant_type=client_credentials`, `client_assertion_type=urn:ietf:params:oauth:client-assertion-type:jwt-bearer`, a signed `client_assertion`, and the requested `scope`, with **no** Basic header.
- [x] **Step 10: Run; verify fail.**
- [x] **Step 11: Implement.** Add `AuthMethod`-equivalent assertion mode (or a `clientAssertion AssertionSource` field) to `clientcreds`; relax `FromConfig` so a `ClientSecret` is not required when an assertion source is set; in `fetch` emit `client_assertion*` instead of Basic/post credentials. This makes `clientcreds` the SMART-Backend-Services provider while `jwtbearer` remains the openEHR "JWT Bearer Token Grant".
- [x] **Step 12: Run** `go test ./auth/clientcreds/... -race -v` — PASS.

### 3e — ID-token verification alg agility (F-M)

- [x] **Step 12a: Failing tests.** In `smart/idtoken_test.go` add `TestValidateIDTokenRS384` and `TestValidateIDTokenES384`: sign an id_token with RS384 (RSA) and ES384 (ECDSA P-384), publish the matching JWK, and assert `ValidateIDToken` accepts it. Add `TestValidateIDTokenRejectsUnlistedAlg` (alg not in the allowlist → `ErrJWKSValidationFailed`) and `TestValidateIDTokenRejectsAlgNone`.
- [x] **Step 12b: Run; verify fail** (RS256-only today).
- [x] **Step 12c: Implement.** Generalise `ValidateIDToken`/`rsaPublicKeyFromJWK` to the **shared alg table from 3a** (RS256/RS384/ES256/ES384): add EC JWK parsing (`crypto/ecdsa`, P-256/P-384) and ECDSA verification, keep the **verify-signature-before-claims** ordering, and constrain the accepted header `alg` to the allowlist (default RS256/RS384/ES256/ES384, narrowed to the server's `id_token_signing_alg_values_supported` from F-K when present). Keep the `alg`-must-match-`kty` guard; continue rejecting `none` and multi-signature tokens. No new third-party dep.
- [x] **Step 12d: Run** `go test ./smart/... -v` — PASS.

### 3d — Spec + close-out

- [x] **Step 13: Spec.** In `auth.md` REQ-068 document the asymmetric confidential code flow and the Backend Services flow (cite `client-confidential-asymmetric` and `backend-services`), and record the `RS384`/`ES384` baseline (cite the SHALL). Note in `service-discovery.md` that `token_endpoint_auth_signing_alg_values_supported` selects the client-auth algorithm and `id_token_signing_alg_values_supported` selects the id-token verify allowlist (both parsed in F-K / Phase 6); record the discovery-driven client-auth-method selection (G-3) and the id-token alg agility (F-M) under REQ-062/REQ-064. Keep REQ-068 `partial` until Phase 5 probes pass. Update `traceability.yaml`.
- [x] **Step 14: Commit** `feat(auth): RS384/ES384 + private_key_jwt for code exchange, backend services, and id_token verification (REQ-068, REQ-064)`.

## Phase 4 — Transport 401 → refresh / catalog-refresh hook (F-D) [MED]

**Outcome:** an opt-in transport hook that, on a `401`, re-drives token refresh (and optionally `Resolver.Refresh`) exactly once, then retries the request once; on a second `401` it returns `transport.ErrUnauthorized`. Closes REQ-063's open sub-point + PROBE-007 + PROBE-041.

**Files:**
- Modify: `transport/options.go` (new `WithReauthOn401` option + config field), `transport/client.go` (`Do` retry-on-401 path)
- New: `auth/reauth.go` — a small `Reauther` interface (`Reauth(ctx) error`) implemented by `*auth/smart.Source` (force-refresh) and satisfiable by a discovery refresh closure
- Test: `transport/client_test.go`, plus `testkit/probes/auth/probe_007_token_refresh.go`
- Spec: `auth.md` REQ-063 (replace the "out of scope" paragraph with the wired behaviour), `service-discovery.md` REQ-071 bullet 3, `REQ.md` rows, `traceability.yaml`

- [x] **Step 1: Failing test.** In `transport/client_test.go` add `TestDoReauthOn401`: a test server returns `401` on the first call (stale bearer), `200` on the second; the configured `TokenSource` returns a new bearer after `Reauth`. Assert exactly two upstream calls, the second carrying the refreshed bearer, and a final non-error `200`. A second test asserts two consecutive `401`s return `errors.Is(err, transport.ErrUnauthorized)` with exactly one reauth attempt.
- [x] **Step 2: Run; verify fail.**
- [x] **Step 3: Define the hook.** Add to `auth`:

```go
// Reauther forces a fresh credential after a wire 401 (REQ-063).
type Reauther interface { Reauth(ctx context.Context) error }
```

Implement `(*smart.Source).Reauth` = "mark current token stale + run refresh grant"; expose a `discovery`-refresh adapter. Add `transport.WithReauthOn401(r auth.Reauther)`.

- [x] **Step 3a2 (G-5): Status-aware token-endpoint error.** Give the token-exchange/refresh path an error that carries the HTTP status alongside the parsed `auth.OAuth2Error` envelope (mirroring `x/oauth2.RetrieveError{Response, Body, ErrorCode, …}`, but with a bounded body snippet). Add a `Terminal()` predicate: true for a 4xx carrying `invalid_grant`/`invalid_client`, false for 5xx/network. Add `TestTokenErrorTerminalClassification`. This is the gate F-L's clearing logic depends on.
- [x] **Step 3b (F-L): Clear the refresh token on terminal failure only.** In `auth/smart` `Source.Token()`/`refreshGrant`, when the refresh fails and the Step-3a2 error reports `Terminal()`, **clear `s.refresh`** and the cached token before returning `ErrReauthRequired`, mirroring `client-js` ("delete refresh token so we don't loop"); on a non-terminal (5xx/network) failure **retain** `s.refresh` and return `ErrRefreshFailed` so a transient blip doesn't force re-auth. Subsequent `Token()` then deterministically returns `ErrReauthRequired` instead of re-POSTing a dead refresh token. Add `TestRefreshFailureClearsRefreshTokenOnlyWhenTerminal`.
- [x] **Step 3c (F-L): Add explicit `RefreshIfNeeded`.** Expose `(*smart.Source).RefreshIfNeeded(ctx)` (refresh only when within the threshold) alongside the existing proactive path, matching the reference client's API and giving consumers a non-request-bound refresh trigger.
- [x] **Step 3d (G-2): Configurable early-expiry buffer.** Replace the hardcoded 30s proactive-refresh threshold in `smart.Source` with a functional option (e.g. `smart.WithEarlyExpiry(d)`, default 30s), mirroring `x/oauth2.ReuseTokenSourceWithExpiry`. Add `TestEarlyExpiryConfigurable`. Document under REQ-063.
- [x] **Step 4: Wire `Do`.** After `doOnce` returns a `*WireError` with status `401`, if a reauther is configured and this attempt has not yet reauthed, call `Reauth(ctx)` once, rebuild headers (fresh token via `tokenSourceFor`), retry once. Guard against infinite loops with a per-`Do` boolean. Leave behaviour unchanged when no reauther is set (preserves current `ErrUnauthorized` contract).
- [x] **Step 5: Run** `go test ./transport/... ./auth/... -race -v` — PASS.
- [x] **Step 6: Probes.** Implement `testkit/probes/auth/probe_007_token_refresh.go` (and complete the transport half of PROBE-041) per `conformance.md`.
- [x] **Step 7: Spec.** Rewrite the REQ-063 "Out of scope" paragraph in `auth.md` to document the wired, opt-in 401→refresh; update REQ-071 bullet 3 in `service-discovery.md`; flip REQ-063 row to `landed`. Update `traceability.yaml` + `conformance.md` PROBE-007/041 status.
- [x] **Step 8: Commit** `feat(transport): opt-in 401 -> reauth/refresh-and-retry (REQ-063, REQ-071)`.

## Phase 5 — Auth conformance probes (F-E) [MED]

**Files:** `testkit/probes/auth/probe_00{1..9}_*.go`, `conformance.md`, `traceability.yaml`, `docs/roadmap.md`

- [x] **Step 1:** Create `testkit/probes/auth/` and implement PROBE-001 (discovery declares `code`+`S256`), PROBE-004 (PKCE verifier round-trip), PROBE-005 (scope round-trip), PROBE-006 (JWKS rotation), PROBE-007 (token refresh — shared with Phase 4), PROBE-008/009 as scoped in `conformance.md`, each driven by an `httptest` SMART server. Tag each with `// PROBE-00N`. **(G-7) PKCE parity:** within PROBE-004 assert the SDK verifier matches RFC 7636 / `x/oauth2.GenerateVerifier` — ≥32 bytes entropy, `base64.RawURLEncoding` (no padding), and `challenge == base64url(SHA256(verifier))` with method `S256`.
- [x] **Step 2:** Add launch-mode coverage tests proving standalone (`AuthorizeURL` without `launch`), embedded (`launch` param present), and backend (`clientcreds` + `jwtbearer`) so REQ-068 is provably covered.
- [x] **Step 3:** Run `make probe-status` and `go test ./testkit/probes/auth/... -v` — PASS.
- [x] **Step 4:** Cross-check the in-process assertions against the **Inferno SMART App Launch Test Kit STU2.2 *Client* suite** scenarios (Public, Confidential Symmetric, Confidential Asymmetric, Backend Services Asymmetric) — use them as the external requirement checklist; record any scenario the SDK cannot satisfy as a follow-up rather than silently skipping.
- [x] **Step 5:** Update `conformance.md` statuses, `traceability.yaml` (REQ-068 → `landed` once flows+modes are probe-covered), and `roadmap.md`.
- [x] **Step 6: Commit** `test(probes): land auth probes PROBE-001..009 (REQ-061/068)`.

## Phase 5b — Token introspection client (F-J) [MED]

**Outcome:** consumers can validate an opaque access token and recover its `scope` / launch context / `exp` via RFC 7662.

**Files:** new `auth/introspect/` (`introspect.go`, `introspect_test.go`), `smart/discovery` (surface `introspection_endpoint` from Phase 6), `auth.md` (new REQ or REQ-062 extension), `REQ.md`, `traceability.yaml`

- [x] **Step 1: Failing test.** `TestIntrospectActiveToken`: an `httptest` server implements `POST /introspect` returning `{"active":true,"scope":"patient/COMPOSITION.read openid","client_id":"c1","patient":"p1","exp":...}`; assert the client posts `token=<value>` form-encoded with its own bearer in `Authorization`, and parses `Active`, `Scope`, `ClientID`, `Patient`, `Exp` into a typed `Result`.
- [x] **Step 2: Run; verify fail.**
- [x] **Step 3: Implement** `introspect.New(endpoint, *http.Client)` + `Introspect(ctx, token, bearer) (Result, error)` per RFC 7662 (required `active`/`scope`/`client_id`/`exp`; conditional launch-context + `iss`/`sub`/`fhirUser`; `Raw map[string]any`). Inject `*http.Client` (REQ-021); honour ctx (REQ-020).
- [x] **Step 4: Run** `go test ./auth/introspect/... -v` — PASS.
- [x] **Step 5: Spec + commit.** Add a REQ (or extend REQ-062) citing the `token-introspection` profile; update `traceability.yaml`. `feat(auth/introspect): RFC 7662 token introspection client`.

## Phase 6 — Scope/context helpers + canonical sources + example (F-F, F-G, F-H) [LOW]

**Files:** `auth/scope.go`, `auth/scope_test.go`, `auth.md`, `service-discovery.md`, `research-strands.md`, `docs/adr/`, `smart/discovery/catalog.go` (+resolver), `cmd/examples/smart-launch/`, `docs/examples.md`

- [x] **Step 1:** Add launch-context scope constants/helpers to `auth/scope.go` (`ScopeOpenID`, `ScopeLaunch`, `ScopeLaunchPatient`, `ScopeLaunchEpisode`, `ScopeOfflineAccess`, `ScopeOnlineAccess`) with a table test. Purely lexical — no enforcement (consistent with § Scope handling).
- [x] **Step 2:** Document in `auth.md` how a refresh token is requested (`offline_access` / `online_access`) under REQ-063, and cite the canonical openEHR SMART spec + SMART App Launch v2 URLs in `auth.md` and `service-discovery.md`.
- [x] **Step 3 (F-K + G-3 + F-M):** Capture `introspection_endpoint` / `revocation_endpoint` / `management_endpoint`, `token_endpoint_auth_signing_alg_values_supported`, **`token_endpoint_auth_methods_supported` (G-3 — client-auth method selection)**, and **`id_token_signing_alg_values_supported` (F-M — id-token verify allowlist)** on `AuthEndpoints` (parsed in `parseAuthEndpoints`) — surface only (the signing-alg list feeds Phase 3a/3b's RS384/ES384 selection; the methods list feeds Phase 3b's G-3 selection; the id-token alg list feeds Phase 3e; `introspection_endpoint` feeds Phase 5b). Add the openEHR capability strings (`context-openehr-ehr`, `context-openehr-episode`, `openehr-permission-v1`, `launch-base64-json`) to a documented constant block.
- [x] **Step 4:** Resolve STRAND-05: write `docs/adr/0009-smart-auth-library-scope.md` recording the hand-rolled vs `x/oauth2` decision and the probe evidence from Phase 5; flip STRAND-05 to **Resolved** with the ADR backlink; amend REQ-061..064 notes as needed. **(G-6)** In the same ADR, record the decision on an optional `x/oauth2` interop adapter (`auth.FromOAuth2TokenSource`): the OTel-only dependency rule means it cannot live in core — either isolate it in an opt-in `auth/oauth2adapter` submodule (separate `go.mod`) or decline and document why; note the `oauth2.TokenSource.Token()` no-`ctx` bridging caveat. Do **not** add `x/oauth2` to the root module.
- [x] **Step 5:** Add a runnable `cmd/examples/smart-launch/` (standalone PKCE against an in-process SMART stub, no secrets) and register it in `docs/examples.md` per the ai-workflow Examples checklist. The example MUST **demonstrate persisting the `AuthorizationRequest` (`state` + PKCE `code_verifier`) across the redirect** (e.g. keyed by `state` in an in-memory session map) — the reference clients (`client-js`/`client-py`) hide this in session storage and it is the #1 integration gotcha for the Go SDK's caller-managed model.
- [x] **Step 5b (consideration, optional):** If the MCP/multi-EHR use case is in scope, add a small `smart`/`smart/discovery` helper to resolve a `Source`+`ServiceCatalog` by an incoming `iss` (issuer allow-list), the Go analog of client-js `issMatch` — built on the issuer-keyed discovery cache (REQ-065/070) + per-request `auth.WithTokenSource`. Otherwise record it as a follow-up. — **Outcome: deferred** (not built); recorded as a follow-up in ADR 0009.
- [x] **Step 6:** `make spec-check && make ci`.
- [x] **Step 7: Commit** `feat(auth): scope helpers, canonical SMART sources, ADR 0009, smart-launch example`.

## Mapping to specs

- [docs/specifications/auth.md](../specifications/auth.md) — REQ-060..069 (REQ-063, REQ-064, REQ-068 amended)
- [docs/specifications/service-discovery.md](../specifications/service-discovery.md) — REQ-070..072 (services shape, version gate)
- [docs/specifications/conformance.md](../specifications/conformance.md) — PROBE-001..009, PROBE-041
- [docs/specifications/research-strands.md § STRAND-05](../specifications/research-strands.md#strand-05--smart-on-openehr-auth-library) — resolved via ADR 0009
- [docs/specifications/REQ.md](../specifications/REQ.md) + [traceability.yaml](../specifications/traceability.yaml) — registry rows
- Canonical external sources: see the **References** section below.

## References

External sources consulted for this audit (2026-06-16), with what each grounds. Keep these current if the agent revisits the plan.

### Normative specifications

| Source | URL | Grounds |
|---|---|---|
| openEHR *SMART on openEHR* (ITS-REST, development) | https://specifications.openehr.org/releases/ITS-REST/development/smart_app_launch.html | F-A (services map, `baseUrl`), F-B (`ehrId`/`episodeId`), F-G/F-K (capabilities, endpoints), scope syntax |
| openEHR ITS-REST component (development) | https://specifications.openehr.org/releases/ITS-REST/development | spec-version pin context (REQ-050/072) |
| openEHR ITS-REST APIs overview | https://specifications.openehr.org/releases/ITS-REST/latest/overview.html | 401/403 + `WWW-Authenticate` expectations |
| HL7 SMART App Launch v2.2 — IG home | https://hl7.org/fhir/smart-app-launch/ | overall profile the openEHR spec tracks |
| — App Launch & Authorization | https://hl7.org/fhir/smart-app-launch/app-launch.html | `aud` required, launch modes, refresh (REQ-061/063/068) |
| — Scopes & Launch Context | https://hl7.org/fhir/smart-app-launch/scopes-and-launch-context.html | `offline_access`/`online_access`, `launch/*` scopes (F-F) |
| — Confidential Client, Asymmetric | https://hl7.org/fhir/smart-app-launch/client-confidential-asymmetric.html | F-C/F-I (`private_key_jwt`, RS384/ES384, `client_assertion`) |
| — Backend Services | https://hl7.org/fhir/smart-app-launch/backend-services.html | F-C (`client_credentials` + `client_assertion`) |
| — Token Introspection | https://hl7.org/fhir/smart-app-launch/token-introspection.html | F-J (RFC 7662 fields) |
| — Best Practices | https://hl7.org/fhir/smart-app-launch/best-practices.html | refresh-token single-use, confidential vs public trade-offs |
| HL7 SMART App Launch — source pages | https://github.com/HL7/smart-app-launch/tree/master/input/pages | authoritative Markdown for the pages above |

### Tooling, sandboxes, reference deployments

| Source | URL | Use |
|---|---|---|
| Inferno SMART App Launch Test Kit (STU1/STU2/**STU2.2**, incl. *STU2.2 Client* suite) | https://inferno.healthit.gov/test-kits/smart-app-launch/ | external client conformance checklist (Phase 5) |
| SMART App Launcher sandbox | https://launch.smarthealthit.org/ | manual/integration target for the example (Phase 6) |
| Aidbox / Smartbox SMART-on-FHIR docs | https://www.health-samurai.io/docs/aidbox/access-control/authorization/smart-on-fhir | reference deployment (asymmetric/symmetric client auth, scopes, Inferno pass notes) |
| SMART reference client `client-js` (v2) + docs | https://github.com/smart-on-fhir/client-js · http://docs.smarthealthit.org/client-js/ | API reference for client behaviour: auto-refresh+retry on 401, `clientPrivateJwk` RS384/ES384, `pkceMode`, `refreshIfNeeded`, state-across-redirect, `issMatch` (F-C/F-D/F-I/F-L, Phase 6) |
| SMART reference client `client-py` | https://github.com/smart-on-fhir/client-py | Python reference (`prepare`/`ready`/`authorize_url`); points to `cumulus-fhir-support` for "more auth options + built-in retries" (confirms F-D) |

### Go library patterns (cross-checked, NOT added as deps — OTel-only rule)

| Source | URL | Grounds |
|---|---|---|
| `golang.org/x/oauth2` — `oauth2.go` / `token.go` | https://pkg.go.dev/golang.org/x/oauth2 · https://cs.opensource.google/go/x/oauth2 | G-1 (`reuseTokenSource` proactive-expiry refresh, mutex coalescing), G-2 (`defaultExpiryDelta` 10s, `ReuseTokenSourceWithExpiry`), G-5 (`RetrieveError`) |
| `golang.org/x/oauth2` — `transport.go` | https://cs.opensource.google/go/x/oauth2/+/master:transport.go | G-1 (`Transport.RoundTrip` does NOT 401-reactive refresh → reframes F-D as a safety net) |
| `golang.org/x/oauth2/clientcredentials` | https://pkg.go.dev/golang.org/x/oauth2/clientcredentials | G-3 (`AuthStyle` header-vs-params; 2-legged token source shape) |
| `golang.org/x/oauth2` — `pkce.go` | https://cs.opensource.google/go/x/oauth2/+/master:pkce.go | G-7 (`GenerateVerifier` 32 octets → raw-url base64, `S256ChallengeFromVerifier`) |
| `github.com/coreos/go-oidc/v3` — `oidc/verify.go` | https://pkg.go.dev/github.com/coreos/go-oidc/v3/oidc · https://github.com/coreos/go-oidc | F-M (discovery-driven `SupportedSigningAlgs`, RS256 only as last resort, verify-before-claims, reject `none`/multi-sig, nonce is caller's job) |

### Go idiom authorities

- Effective Go — https://go.dev/doc/effective_go · Go Code Review Comments — https://go.dev/wiki/CodeReviewComments (G-8 ctx-first, injected client)
- Repo `idiom.md` (canonical) — `docs/specifications/idiom.md` (no-reflection, `*http.Client` injection, ctx-first, functional options)

### Underlying RFCs

- PKCE — RFC 7636 — https://www.rfc-editor.org/rfc/rfc7636 (REQ-061)
- JWT Bearer assertions — RFC 7523 — https://www.rfc-editor.org/rfc/rfc7523 (F-C, `jwtbearer`)
- Token Introspection — RFC 7662 — https://www.rfc-editor.org/rfc/rfc7662 (F-J)
- JSON Web Key / Algorithms — RFC 7517 / 7518 — https://www.rfc-editor.org/rfc/rfc7517 · https://www.rfc-editor.org/rfc/rfc7518 (F-I, JWKS/alg)
- OAuth 2.0 + Bearer tokens — RFC 6749 / 6750 — https://www.rfc-editor.org/rfc/rfc6749 · https://www.rfc-editor.org/rfc/rfc6750
- Authorization Server Metadata — RFC 8414 — https://www.rfc-editor.org/rfc/rfc8414 (F-K, `*_supported` discovery fields)
- Token Revocation — RFC 7009 — https://www.rfc-editor.org/rfc/rfc7009 (`revocation_endpoint`, deferred)

## Self-review notes

- **Spec coverage:** every finding F-A..F-M maps to a phase (table above). REQ-063 and REQ-068 (the flagged items) are Phases 4 and 3/5; the externally-grounded additions (RS384/ES384, Backend Services, introspection, id-token alg agility) are Phases 3 and 5b.
- **Decisions, not silent code:** F-A and STRAND-05 land ADRs (0008, 0009); the per-service version-gate softening, the `clientcreds`-as-Backend-Services choice, and the G-6 `x/oauth2`-adapter decision are called out explicitly rather than buried.
- **Dependency discipline (Go idiom cross-check):** the `x/oauth2` / `go-oidc` learnings (G-1..G-8, F-M) are applied as *pattern/correctness parity in hand-rolled code* — **no** new runtime dependency is added (AGENTS.md: OpenTelemetry only). The only place a third-party dep is even contemplated (G-6 adapter) is gated behind an ADR and an isolated submodule.
- **Type consistency:** `LaunchContext.EHRID/EpisodeID`, `TokenResponse.EHRID/EpisodeID`, `auth.Reauther`, `transport.WithReauthOn401`, `smart.WithClientAssertionKey`, `smart.WithEarlyExpiry`, `clientcreds.WithClientAssertion`, the shared JOSE alg table (3a), and `jwtbearer` `WithAlgorithm("RS384"|"ES384")` are named once and reused across phases.
- **Ordering:** Phase 3a (signer algorithms + shared JOSE alg table) precedes 3b/3c (`private_key_jwt` depends on RS384/ES384) and 3e (the id-token verifier reuses the 3a alg table); Phase 4 Step 3a2 (status-aware token error) precedes 3b (F-L's terminal-vs-transient clearing depends on it); Phase 6 (F-K) surfaces the `*_endpoint` + `*_supported` metadata that Phases 3/3e/5b consume — implement F-K's parsing early if running 3/5b first.
