# Cadasto platform API fixtures

Recorded request/response wire records for the Cadasto-platform extras
(`cadasto/extra`, `cadasto/datamap`, `cadasto/mpi`, `cadasto/care`, and the
Cadasto admin surface).

These surfaces have no openEHR spec. Their conformance authority is the
Cadasto platform API: its OpenAPI document where one exists, otherwise the
behaviour of a reference Cadasto deployment. `cadasto/*` conformance probes
assert the SDK's wire shape against these fixtures, not against any other SDK.
See [`docs/specifications/module-layout.md` § Cadasto extras](../../../docs/specifications/module-layout.md#cadasto-extras).

**Status: placeholder.** Fixtures land with the `cadasto/*` packages in Phase 4.
When adding one, record its provenance here (deployment URL, commit/date, and
the API operation it captures).
