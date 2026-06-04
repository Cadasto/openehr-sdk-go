# submissions/

Vendored **CONTRIBUTION create** wire JSON from ehrbase Robot integration tests (`contributions/` in the upstream tree).

Each file is a full `CONTRIBUTION` body with `versions[]` holding inline `ORIGINAL_VERSION<T>` payloads (the ITS-REST submission shape). These are **not** persisted `CONTRIBUTION` resources where `versions` are `OBJECT_REF` only.

Resolve paths via [`fixtures.SubmissionJSON`](../../fixtures/paths.go). Decode with [`contribution.Submission`](../../../openehr/client/ehr/contribution/submission.go) when testing the EHR contribution client — not `canjson.Unmarshal` into `rm.Contribution`.
