# Plan — Write-result contract

**Date:** 2026-08-18
**Status:** Draft
**Owner:** SDK maintainers
**Covers:** [REQ-094](../specifications/transport.md#req-094--prefer-response-shape-negotiation) (implementation-aligned amendment — the normative delta is carried in this plan under [Spec delta](#phase-0--spec-delta-same-pr-as-the-code) and lands in `transport.md` in the same PR as the code, never ahead of it)
**Probes:** none new — package tests only (PROBE-061/071 stay the representation-decode probes)
**Implementation:** planned (amendment on a landed REQ)
**Depends on:** landed REQ-094 Prefer state machine; `rm.IsTypedNil`
**Defers:** a breaking `WriteOutcome[T]` result type; canjson marshal error typing; changing Prefer defaults

> **Execution:** work the phases in order and the steps within a phase sequentially. Run each step's verification command before moving on; a failing step blocks the next. Commit exactly where a step says commit.

**Goal:** Callers can tell “no body, write succeeded” from “write committed, body unusable” without inspecting `WireError` or correlating a parallel metadata value, and without treating a typed-nil resource as present.

**Architecture:** Keep the `(T, *VersionMetadata, error)` triple. Document the typed-nil success path and add `HasResource[T]`. On 2xx + `PreferRepresentation` + empty/undecodable body, return `*NoRepresentationError` (Meta + Cause). Align `contribution.Commit`.

**Tech Stack:** Go 1.26; existing `ehr.WriteResult`; `errors.As`.

## Global Constraints

- Implementation-aligned: spec § and code land in the same PR.
- REQ-024: `HasResource` is reflection-free.
- REQ-025: `NoRepresentationError` implements `Unwrap`.
- Do not mention any external consumer or review artefact in commits or prose.

## Definition of Ready

- The REQ-094 amendment text is final (Phase 0 below carries it verbatim); `transport.md` is edited in this plan's PR, not before.
- No ADR (the success path stays `err == nil` for identifier/minimal).
- Verification: `go test ./openehr/client/ehr/... ./openehr/client/ehr/contribution/ -count=1`, `make spec-check`, `make ci`.
- Negative space: a 409 stays `*WireError`; an empty representation after 2xx is `*NoRepresentationError`, never a silent nil success.

## Definition of Done

- `HasResource` and `NoRepresentationError` landed with `// REQ-094` tests.
- `WriteResult` and `contribution.Commit` obey the amended § .
- `traceability.yaml` REQ-094 notes + tests list updated (implementation stays `landed`).
- `make spec-check` and `make ci` pass.
- Plan archived.

## Implementation checklist

| Step | Status |
|---|---|
| Phase 0: REQ-094 § amended (same PR as the code) | |
| Code | |
| Tests with `// REQ-094` comments | |
| `make spec-check` | |
| `make ci` | |

## Phases

### Phase 0 — Spec delta (same PR as the code)

`transport.md` § REQ-094 is **implementation-aligned**: the spec edit and the code below ride one PR. Apply both edits to [transport.md § REQ-094](../specifications/transport.md#req-094--prefer-response-shape-negotiation) in this plan's first commit; `make spec-check` gates the pair.

- [ ] **Step 0a:** In the landed-state paragraph ("All three write-path modes are landed…"), replace the clause "returns [`transport.ErrInvalidShape`](../../transport/errors.go) on an empty body" with "reports an empty body as `NoRepresentationError` wrapping `transport.ErrInvalidShape`" — one contract per path, no contradiction with the new MUSTs.

- [ ] **Step 0b:** Append the amendment verbatim after that paragraph (proposed SPEC text — it lands in `transport.md` § REQ-094 together with the code, and only then):

> **Absent resource on a successful write.** `minimal` and `identifier` **MUST** return a nil error and a zero resource. For a pointer resource type the zero value is a typed nil: `== nil` is the wrong test. The SDK **MUST** expose a reflection-free `HasResource` helper that reports false for a typed-nil pointer, a bare-nil interface, and an interface holding a typed-nil pointer, and true for any populated resource. Write-path documentation **MUST** name the typed-nil trap and point at that helper (and at `rm.IsTypedNil` for callers already on the RM type).
>
> **Committed write, unusable representation.** After a successful HTTP response (2xx), when `Prefer: return=representation` was sent and the body is empty or does not decode as the expected resource, the write **MUST** be reported as a typed `NoRepresentationError` that:
>
> - carries the version metadata that proves the commit (including `VersionUID` when the server supplied it);
> - wraps the cause (`ErrInvalidShape` for an empty body; the decoder's error otherwise);
> - is distinguishable with `errors.As` alone, with no reference to `WireError` and no correlation against a separately-returned metadata value.
>
> A wire failure **MUST** remain a `*transport.WireError`. The SDK **MUST NOT** return `NoRepresentationError` for a non-2xx response. The existing `(resource, metadata, error)` triple **MUST** still populate metadata on this path so current callers do not break.
>
> The same empty-body and decode-failure rules **MUST** apply to `contribution.Commit` when `Prefer: return=representation` was sent. An empty representation body **MUST NOT** be a silent success.

- [ ] **Step 0c:** Same commit: `traceability.yaml` REQ-094 — replace the "Amendment planned" note with the landed facts (tests list grows in Phase 2); `docs/roadmap.md` REQ-094 row — drop the "Amendment planned" clause. Registry Impl. stays `landed` throughout.

### Phase 1 — `HasResource` and `NoRepresentationError`

**Files:**

- Create: `openehr/client/ehr/resource.go`, `openehr/client/ehr/resource_test.go`
- Create: `openehr/client/ehr/representation.go` (or add the type next to `WriteResult` in `write.go`)
- Test: `openehr/client/ehr/write_test.go` (new if absent)

**Interfaces:**

- Produces: `func HasResource[T any](v T) bool`; `type NoRepresentationError struct { Meta *VersionMetadata; Cause error }`

- [ ] **Step 1: Write failing tests**

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

- [ ] **Step 2: Confirm FAIL**

```
go test ./openehr/client/ehr/ -run 'TestHasResource|TestNoRepresentationErrorAs' -count=1
```

- [ ] **Step 3: Implement**

```go
// HasResource reports whether a write returned a usable resource (REQ-094).
// False for a bare-nil interface, a typed-nil RM pointer, and an interface
// holding one; true otherwise. Comparing any(v) against a boxed zero value
// would panic for an uncomparable T — compare against untyped nil only.
func HasResource[T any](v T) bool {
	a := any(v)
	if a == nil {
		return false
	}
	return !rm.IsTypedNil(a)
}

type NoRepresentationError struct {
	Meta  *VersionMetadata
	Cause error
}

func (e *NoRepresentationError) Error() string {
	if e == nil {
		return "ehr: no representation"
	}
	if e.Cause != nil {
		return fmt.Sprintf("ehr: committed write has no usable representation: %v", e.Cause)
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

- [ ] **Step 4: Tests PASS**, then commit

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

- [ ] **Step 5: Representation empty body returns `*NoRepresentationError`**

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

- [ ] **Step 6: `contribution.Commit`** — when `PreferRepresentation` and the body is empty, return `*NoRepresentationError` (today this is `nil, meta, nil`). Decode failure wraps the same type.

- [ ] **Step 7: httptest** — PreferRepresentation + 2xx + empty body: `errors.As` `*NoRepresentationError`, `HasResource` false, `Meta` set. PreferIdentifier success: `err == nil`, `HasResource` false. Wire 409: `*WireError`, not `*NoRepresentationError`.

- [ ] **Step 8: Verify and commit**

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

- [transport.md § REQ-094](../specifications/transport.md#req-094--prefer-response-shape-negotiation)
- [REQ.md](../specifications/REQ.md)
