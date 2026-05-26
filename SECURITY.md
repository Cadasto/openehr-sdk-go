# Security policy

## Reporting a vulnerability

`openehr-sdk-go` ships under [MIT](LICENSE) and currently has **no commercial support contract**. Vulnerability reports are still taken seriously — please follow this flow.

### How to report

Use GitHub's **private vulnerability reporting**:

1. Go to <https://github.com/Cadasto/openehr-sdk-go/security/advisories/new>
2. Fill in the form. Include:
   - The Go version, OS, and SDK version (`go list -m github.com/cadasto/openehr-sdk-go` or the git tag) you reproduce against.
   - A minimal reproduction — the smallest snippet, OPT, or composition body that exposes the issue.
   - Impact assessment (data exposure / auth bypass / DoS / etc.).
   - Suggested fix, if you have one.
3. Maintainers triage in private; do **NOT** open a public issue or PR until coordinated.

If GitHub's private reporting is unavailable to you, email the repository owner via the email on their GitHub profile.

### What to expect

- **Acknowledgment**: within 5 working days.
- **Initial triage**: within 10 working days — severity, affected versions, whether we can reproduce.
- **Fix + disclosure**: timing depends on severity and complexity. We coordinate disclosure with the reporter before publishing the advisory.

We are an early-stage SDK (pre-1.0). We do not currently issue CVEs ourselves; we may request one through GitHub's CNA process for any high-severity issue.

## Scope

In scope:

- Bugs that allow incorrect or unauthorised behaviour in the SDK itself.
- Wire-protocol bugs (canonical JSON / XML codec mis-encoding) that lead to data confusion or injection.
- AuthN/Z handling in `auth/` and `auth/smart/` — token leak, audience confusion, scope escalation, JWKS handling.
- Build / supply-chain issues affecting consumers (e.g. a malicious dependency surfacing through `go.mod`).

Out of scope:

- Issues in the upstream openEHR specification itself — report those to the [openEHR specification editors](https://specifications.openehr.org/).
- Issues in third-party openEHR CDR implementations — report to the CDR vendor.
- Misconfiguration in consumer applications using the SDK (we are happy to advise but the SDK code is the scope of this policy).
- Best-practice or hardening suggestions without a concrete vulnerability — open a public issue or discussion instead.

## Supported versions

While pre-1.0, **only the latest tagged release** receives security fixes. `main` carries unreleased work; consumer pinning to an unreleased commit is at their own risk.

Once we reach `v1.0.0`, this section will be updated with a multi-line support window.

## Hardening guidance

Defaults the SDK ships with that consumers should be aware of:

- **TLS** ([REQ-092](docs/specifications/transport.md#req-092--tls-posture)): the SDK respects the caller's `*http.Client`. If the caller provides a permissive `tls.Config`, the SDK does not override it. Production consumers MUST inject a strict-mode client.
- **Token handling** ([REQ-060..069](docs/specifications/auth.md)): tokens live in `auth.TokenSource`. The SDK does not log tokens; consumers SHOULD NOT add their own logging that does.
- **Spec-version pin** ([REQ-050](docs/specifications/wire.md#req-050)): discovery responses with a mismatched `spec_version` fail fast at construction (PROBE-003). Do not disable this.

## Acknowledgments

We thank the security community in advance for any responsible disclosure. Reporters who wish credit in the advisory will be credited; those who prefer anonymity are respected.
