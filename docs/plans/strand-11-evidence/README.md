# STRAND-11 capture evidence

One live `POST /ehr` against local EHRbase (2.35.1), serialised both ways so
the HAR-vs-YAML auditability question can be answered by reading the files,
not by reasoning about the formats.

| File | Format |
|---|---|
| [ehr-create.har](ehr-create.har) | HTTP Archive 1.2 (provenance and redaction stuffed into `log.comment`) |
| [ehr-create.yaml](ehr-create.yaml) | Purpose-built YAML (native `provenance` / `redaction` keys) |

- Deployment: `ehrbase/ehrbase:latest` 2.35.1 at `http://127.0.0.1:8080/ehrbase/rest/openehr/v1`
- Profile: `SPRING_PROFILES_ACTIVE=local` (no auth on this capture)
- SDK commit: `c673a67e`
- Captured: `2026-09-07T18:53:48Z`
- Redaction ran at capture time (`Authorization`, `Cookie`, `Set-Cookie`, `Proxy-Authorization`). None of those headers were present on this exchange.

**Decision:** [ADR 0020](../../adr/0020-cassette-recording-har.md) — Cassette
recordings are HAR 1.2. The YAML file stays here as the rejected alternative.

`log._req082` on the HAR is the REQ-082 provenance / redaction slot the ADR
requires. This directory is evidence, not the Cassette corpus
(`testkit/recordings/` stays empty until the recorder lands).
