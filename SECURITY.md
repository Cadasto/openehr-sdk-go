# Security policy

## Reporting a vulnerability

`openehr-sdk-go` ships under [MIT](LICENSE) with no commercial support contract, but security reports are taken seriously.

**Do not open a public issue or PR for a security bug.**

Report privately through GitHub (private vulnerability reporting is enabled on this repo): **Security → Report a vulnerability**, or go straight to <https://github.com/Cadasto/openehr-sdk-go/security/advisories/new>. Anyone with a GitHub account can use it, and the report stays private to maintainers. No GitHub account? Email the repository owner via the contact on their GitHub profile.

Helpful to include: SDK version (git tag, or `go list -m github.com/cadasto/openehr-sdk-go`), Go version and OS, a minimal reproduction (snippet / OPT / composition body), impact (data exposure / auth bypass / DoS / …), and a suggested fix if you have one.

**What happens next:** we'll acknowledge the report, work with you to confirm and assess the issue, and coordinate a fix and disclosure before any advisory is published. As a pre-1.0, volunteer-maintained SDK we can't commit to fixed response times, but security issues take priority. We don't issue CVEs ourselves, though we may request one through GitHub's CNA process for a high-severity issue.

## Scope

In scope:

- Incorrect or unauthorised behaviour in the SDK itself.
- Wire-codec bugs (canonical JSON / XML) causing data confusion or injection.
- AuthN/Z in `auth/` and `auth/smart/` — token leak, audience confusion, scope escalation, JWKS handling.
- Supply-chain issues reaching consumers through `go.mod`.

Out of scope — report upstream, or not an SDK vulnerability:

- The openEHR specification itself → [openEHR editors](https://specifications.openehr.org/).
- Third-party CDR implementations → the CDR vendor.
- Misconfiguration in consumer applications (happy to advise).
- Hardening suggestions without a concrete vulnerability → open a public issue or discussion.

## Supported versions

Pre-1.0: **only the latest tagged release** receives security fixes. `main` carries unreleased work — pinning to it is at your own risk. A multi-version support window arrives with `v1.0.0`.

## Hardening guidance

SDK defaults consumers should be aware of:

- **TLS** ([REQ-092](docs/specifications/transport.md#req-092--tls-posture)): the SDK uses the caller's `*http.Client` and never overrides its `tls.Config`. Inject a strict-mode client in production.
- **Tokens** ([REQ-060..069](docs/specifications/auth.md)): held in `auth.TokenSource`; the SDK never logs them — don't add logging that does.
- **Spec-version pin** ([REQ-050](docs/specifications/wire.md#req-050)): a mismatched discovery `spec_version` fails fast at construction (PROBE-003). Don't disable it.

Reporters who want credit in the advisory are credited; those who prefer anonymity are respected.
