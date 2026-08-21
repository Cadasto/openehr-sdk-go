# Plan — Write-result contract

**Date:** 2026-08-18
**Status:** Done
**Owner:** SDK maintainers
**Covers:** [REQ-094](../../specifications/transport.md#req-094--prefer-response-shape-negotiation) (implementation-aligned amendment — binding words live in that §; this plan is delivery history)
**Probes:** none new — package tests only (PROBE-061/071 stay the representation-decode probes)
**Implementation:** landed (amendment on a landed REQ)
**Depends on:** landed REQ-094 Prefer state machine; `rm.IsTypedNil`
**Defers:** a breaking `WriteOutcome[T]` result type; canjson marshal error typing; changing Prefer defaults; `contribution.Commit` `identifier`-slot population (Commit treats `identifier` as metadata-only today — REQ-094's landed-state paragraph names the gap); typing `ehr.Create`'s committed-but-unusable arm (it decodes through `transport.Decode`, shared with the read paths, which keep the bare `ErrInvalidShape`)

> **Execution (historical):** phases were worked in order; this copy is the archived record.

**Goal:** Callers can tell “no body, write succeeded” from “write committed, body unusable” without inspecting `WireError` or correlating a parallel metadata value, and without treating a typed-nil resource as present.

**Architecture:** Keep the `(T, *VersionMetadata, error)` triple. Document the typed-nil success path and add `HasResource[T]`. On 2xx + `PreferRepresentation` + empty/undecodable body, return `*NoRepresentationError` (Meta + Cause). Align `contribution.Commit`.

**Tech Stack:** Go 1.26; existing `ehr.WriteResult`; `errors.As`.

## Global Constraints

- Implementation-aligned: spec § and code land in the same PR.
- REQ-024: `HasResource` is reflection-free.
- REQ-025: `NoRepresentationError` implements `Unwrap`.
- Do not mention any external consumer or review artefact in commits or prose.

## Definition of Ready

- The REQ-094 amendment text is final; it landed in `transport.md` § REQ-094 in this plan's PR, not before.
- No ADR (the success path stays `err == nil` for identifier/minimal).
- Verification: `go test ./openehr/client/ehr/... ./openehr/client/demographic/ -count=1` (demographic calls the same `WriteResult`, so its observable error type changes with Phase 2 too), `make spec-check`, `make ci`.
- Negative space: a 409 stays `*WireError`; an empty representation after 2xx is `*NoRepresentationError`, never a silent nil success.

## Definition of Done

- `HasResource` and `NoRepresentationError` landed with `// REQ-094` tests.
- `WriteResult` and `contribution.Commit` obey the amended § .
- `traceability.yaml` REQ-094 notes + tests list updated (implementation stays `landed`).
- `make spec-check` and `make ci` pass.
- Plan archived.
- DoR negative space exercised: 409 stays `*WireError`; empty 2xx representation is `*NoRepresentationError`.

## Implementation checklist

| Step | Status |
|---|---|
| Phase 0: REQ-094 § amended (same PR as the code) | ✅ |
| Code | ✅ |
| Tests with `// REQ-094` comments | ✅ |
| `make spec-check` | ✅ |
| `make ci` | host gates ✅ (gofmt/vet/lint/build/`go test ./...`/`codegen-verify`); the `aqlgen-verify`→`antlr-image` step needs Docker and is unaffected by this change |

## Phases

### Phase 0 — Spec delta (same PR as the code)

The RFC-2119 amendment landed in [transport.md § REQ-094](../../specifications/transport.md#req-094--prefer-response-shape-negotiation). This phase is historical; the § is the only binding home.

- [x] **Step 0a:** Landed-state paragraph updated so empty `representation` is a `NoRepresentationError` wrapping `transport.ErrInvalidShape`; `contribution.Commit` no longer treats an empty representation as silent success. The `identifier`-slot gap stays deferred (this plan's Defers).
- [x] **Step 0b:** Absent-resource and committed-but-unusable-representation clauses landed in that § (not restated here).
- [x] **Step 0c:** `traceability.yaml` REQ-094 notes + `docs/roadmap.md` REQ-094 row updated. Registry Impl. stays `landed`.

### Phase 1 — `HasResource` and `NoRepresentationError`

**Files:**

- Create: `openehr/client/ehr/resource.go`, `openehr/client/ehr/resource_test.go`
- Create: `openehr/client/ehr/representation.go` (or add the type next to `WriteResult` in `write.go`)
- Test: `openehr/client/ehr/write_test.go` (new if absent)

**Interfaces:**

- Produces: `func HasResource[T any](v T) bool`; `type NoRepresentationError struct { Meta *VersionMetadata; Cause error }`

- [x] **Step 1: Write failing tests**

```go
// REQ-094
func TestHasResource(t *testing.T) {
	var none *rm.Composition
	if HasResource(none) {
		t.Fatal("typed-nil *Composition")
	}
	if !HasResource(&rm.Composition{}) {
		t.Fatal("non-nil *Composition")
	}
	var party rm.Party
	if HasResource(party) {
		t.Fatal("bare-nil Party")
	}
	var person *rm.Person
	party = person
	if HasResource(party) {
		t.Fatal("Party holding typed-nil *Person")
	}
}

func TestNoRepresentationErrorAs(t *testing.T) {
	meta := &VersionMetadata{VersionUID: "uid::system::1"}
	err := &NoRepresentationError{Meta: meta, Cause: transport.ErrInvalidShape}
	var got *NoRepresentationError
	if !errors.As(err, &got) {
		t.Fatal("errors.As")
	}
	if got.Meta.VersionUID != meta.VersionUID {
		t.Fatalf("uid = %q", got.Meta.VersionUID)
	}
	if !errors.Is(err, transport.ErrInvalidShape) {
		t.Fatal("unwrap Cause")
	}
	var we *transport.WireError
	if errors.As(err, &we) {
		t.Fatal("must not look like WireError")
	}
}
```

- [x] **Step 2: Confirm FAIL**

```
go test ./openehr/client/ehr/ -run 'TestHasResource|TestNoRepresentationErrorAs' -count=1
```

- [x] **Step 3: Implement**

```go
// HasResource reports whether a write returned a usable resource (REQ-094).
// False for a bare-nil interface, a typed-nil REGISTERED RM pointer, and an
// interface holding one; true otherwise. The contract is scoped to the RM
// registry: rm.IsTypedNil is a generated closed type switch whose default is
// false, so a typed nil of a type OUTSIDE the registry reads as present —
// state the scope in the godoc rather than promising "any typed nil", or the
// helper tells the exact lie it exists to prevent. (Every write leaf returns
// a registered RM type, so the planned tests cannot reach the gap; the
// CONTRACT is what must not overclaim.) Comparing any(v) against a boxed
// zero value would panic for an uncomparable T — compare against untyped
// nil only.
func HasResource[T any](v T) bool {
	a := any(v)
	if a == nil {
		return false
	}
	return !rm.IsTypedNil(a)
}

// NoRepresentationError: Meta and the CLASSIFICATION (the type itself, via
// errors.As) are the boundary-safe surface. Cause is internal diagnostics
// and may carry payload-derived text — rm decode errors embed the offending
// value (`parse %q`), the same class WithRawErrorBodies gates for
// OpenEHRErrorDetail.Message — so Error() is value-free per the landed §
// (REQ-093 discipline); Cause is reachable only by unwrapping.
type NoRepresentationError struct {
	Meta  *VersionMetadata
	Cause error
}

func (e *NoRepresentationError) Error() string {
	if e == nil {
		return "ehr: no representation"
	}
	if errors.Is(e.Cause, transport.ErrInvalidShape) {
		return "ehr: committed write has no usable representation (empty body)"
	}
	if e.Cause != nil {
		return "ehr: committed write has no usable representation (decode failed)"
	}
	return "ehr: committed write has no usable representation"
}

func (e *NoRepresentationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}
```

- [x] **Step 4: Tests PASS**, then commit

```
git add openehr/client/ehr/resource.go openehr/client/ehr/resource_test.go openehr/client/ehr/write.go
git commit -m "$(cat <<'EOF'
feat(client/ehr): add HasResource and NoRepresentationError

REQ-094: callers distinguish an absent success body from a
committed write whose representation cannot be decoded.
EOF
)"
```

### Phase 2 — `WriteResult` and `contribution.Commit`

**Files:**

- Modify: `openehr/client/ehr/write.go` (`WriteResult` godoc + representation arm)
- Modify: `openehr/client/ehr/contribution/contribution.go`
- Test: existing composition / directory / ehrstatus write tests; new Commit cases

- [x] **Step 5: Representation empty body returns `*NoRepresentationError`**

Change the empty-body arm from a wrapped `ErrInvalidShape` to:

```go
return zero, meta, &NoRepresentationError{
	Meta:  meta,
	Cause: fmt.Errorf("%s: %w: Prefer=return=representation but response body is empty", label, transport.ErrInvalidShape),
}
```

Decode failure:

```go
return zero, meta, &NoRepresentationError{Meta: meta, Cause: err}
```

Do **not** wrap identifier/minimal success in this type.

- [x] **Step 6: `contribution.Commit`** — when `PreferRepresentation` and the body is empty, return `*NoRepresentationError` (today this is `nil, meta, nil`). Decode failure wraps the same type.

- [x] **Step 7: httptest** — PreferRepresentation + 2xx + empty body: `errors.As` `*NoRepresentationError`, `HasResource` false, `Meta` set. PreferIdentifier success: `err == nil`, `HasResource` false. Wire 409: `*WireError`, not `*NoRepresentationError`.

- [x] **Step 8: Verify and commit**

```
go test ./openehr/client/ehr/... -count=1
make spec-check
```

```
git commit -m "$(cat <<'EOF'
fix(client/ehr): type committed-but-unusable write results

REQ-094: WriteResult and contribution.Commit return
NoRepresentationError after a 2xx with an empty or undecodable
representation body.
EOF
)"
```

## Mapping to specs

- [transport.md § REQ-094](../../specifications/transport.md#req-094--prefer-response-shape-negotiation)
- [REQ.md](../../specifications/REQ.md)
