# submissions/

Vendored **CONTRIBUTION create** wire JSON from ehrbase Robot integration tests (`contributions/` in the upstream tree).

Each file is a full `CONTRIBUTION` body with `versions[]` holding inline `ORIGINAL_VERSION<T>` payloads (the ITS-REST submission shape). These are **not** persisted `CONTRIBUTION` resources where `versions` are `OBJECT_REF` only.

Resolve paths via [`fixtures.SubmissionJSON`](../../fixtures/paths.go). Decode with [`contribution.Submission`](../../../openehr/client/ehr/contribution/submission.go) when testing the EHR contribution client — not `canjson.Unmarshal` into `rm.Contribution`.

**Commit-audit shape (SPECITS-95 / [ITS-REST PR 131](https://github.com/openEHR/specifications-ITS-REST/pull/131)).** These fixtures use the write-side commit-audit shape: `audit` / `commit_audit` carry `_type:"AUDIT_DETAILS"`, a `DV_CODED_TEXT` `change_type` (nested `defining_code`), and **no** `time_committed` (server-assigned). The SDK emits the same shape via [`contribution.UpdateAudit`](../../../openehr/client/ehr/contribution/update_audit.go); PROBE-072 asserts no fixture or SDK payload regresses to a `time_committed` or flat `TERMINOLOGY_CODE` `change_type`.
