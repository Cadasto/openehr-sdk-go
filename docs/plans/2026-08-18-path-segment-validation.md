# Plan — Path-parameter segment validation

**Date:** 2026-08-18
**Status:** Draft
**Owner:** SDK maintainers
**Covers:** [REQ-150](../specifications/transport.md#req-150--path-parameter-segment-validation); encoding half remains [REQ-095](../specifications/wire.md#req-095)
**Probes:** [PROBE-091](../specifications/conformance.md#probe-091--path-parameter-traversal-is-refused) (Draft)
**Implementation:** planned
**Depends on:** landed REQ-095 single-encode contract (`transport` is the only path encoder)
**Defers:** a breaking `PathSegment` named type on every leaf; validating the service base URL

> **Execution:** work the phases in order and the steps within a phase sequentially. Run each step's verification command before moving on; a failing step blocks the next. Commit exactly where a step says commit. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The transport refuses a decoded path segment that would change the request URI (`.`, `..`, empty, `\`, control characters) and issues no HTTP request.

**Architecture:** `ValidateRequestPath` walks `Request.Path` segments; the transport calls it once at the request-preparation point before building the URL. The same point compares the segment count of `Request.Path` against `Request.Route` when set — a parameter that *contains* `/` yields well-formed segments, so only the arity mismatch betrays it. `ValidatePathSegment` is the per-parameter rule for callers constructing raw requests. New sentinel `ErrInvalidPathSegment`; a violation returns one error chain wrapping BOTH it and `ErrInvalidConfig` (the sentinels themselves are independent — `errors.Is(ErrInvalidPathSegment, ErrInvalidConfig)` is false).

**Tech Stack:** Go 1.26; stdlib `net/http/httptest` + `testing`; no new dependencies.

## Global Constraints

- `context.Context` first on every I/O method (REQ-020).
- No reflection (REQ-024). Wrap errors with `fmt.Errorf("…: %w", err)` (REQ-025).
- Leaf clients MUST NOT call `url.PathEscape` on path parameters (REQ-095).
- Cite `REQ-150` / `PROBE-091` in tests and `doc.go`.
- Do not mention any external consumer or review artefact in commits, plans, or spec prose.

## Definition of Ready

Implementation may start when:

- **`Covers:`** lists REQ-150 (and REQ-095 as the encoding sibling).
- Canonical normative prose exists ([transport.md § REQ-150](../specifications/transport.md#req-150--path-parameter-segment-validation) + registry row).
- No ADR is required (non-breaking enforcement at the existing encoder).
- Phases name verification: `go test ./transport/ ./openehr/client/...`, `make spec-check`, `make ci`.
- Negative space is REQ-150: traversal / empty / `\` / control-character segments, and a `Route`-arity mismatch (separator smuggled inside a parameter), fail closed with no request.

## Definition of Done

- Code and tests land with `// REQ-150` / `// PROBE-091` citations.
- `traceability.yaml` and the REQ.md **Impl.** column for REQ-150 move `planned → landed`.
- PROBE-091 **Status:** Draft → Implemented (Sandbox).
- `make spec-check` and `make ci` pass.
- Plan archived under [`archive/`](archive/).

## Implementation checklist

| Step | Status |
|---|---|
| Spec / registry (REQ-150, PROBE-091 Draft) | done in this change |
| Code | |
| Tests with `// REQ-150` / `// PROBE-091` comments | |
| `make spec-check` | |
| `make ci` | |

## Phases

### Phase 1 — Helpers and sentinel

**Files:**

- Create: `transport/path.go`
- Modify: `transport/errors.go`
- Test: `transport/path_test.go`

**Interfaces:**

- Produces: `var ErrInvalidPathSegment error`; `func ValidatePathSegment(s string) error`; `func ValidateRequestPath(path string) error`

- [ ] **Step 1: Write the failing tests**

```go
// REQ-150
func TestValidatePathSegment(t *testing.T) {
	cases := []struct {
		in      string
		wantErr bool
	}{
		{"Referral Request.v1", false},
		// the service root: no segments to validate — the System API sends it
		// (ValidateRequestPath case; a bare "/" is not a valid single SEGMENT)
		{"openEHR-EHR-COMPOSITION.t_vital_signs.v1", false},
		{"..", true},
		{".", true},
		{"", true},
		{"a/b", true},
		{`a\b`, true},
		{"a\x00b", true},
		{"%2e%2e", false}, // not decoded — ordinary segment
	}
	for _, tc := range cases {
		err := ValidatePathSegment(tc.in)
		if tc.wantErr && err == nil {
			t.Fatalf("%q: want error", tc.in)
		}
		if !tc.wantErr && err != nil {
			t.Fatalf("%q: %v", tc.in, err)
		}
		if tc.wantErr && !errors.Is(err, ErrInvalidPathSegment) {
			t.Fatalf("%q: %v is not ErrInvalidPathSegment", tc.in, err)
		}
	}
}

func TestValidateRequestPathAcceptsTheServiceRoot(t *testing.T) {
	if err := ValidateRequestPath("/"); err != nil {
		t.Fatalf("service root refused: %v (the landed System API sends OPTIONS /)", err)
	}
}

func TestValidateRequestPathIgnoresLeadingSlash(t *testing.T) {
	if err := ValidateRequestPath("/ehr/abc/composition"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRequestPath("/ehr/a/../../definition/query/evil/composition"); err == nil {
		t.Fatal("expected traversal to fail")
	}
	if err := ValidateRequestPath("/ehr/abc/composition/"); err == nil {
		t.Fatal("trailing empty segment must fail")
	}
}
```

- [ ] **Step 2: Run tests and confirm they fail**

```
go test ./transport/ -run 'TestValidatePathSegment|TestValidateRequestPath' -count=1
```

Expected: FAIL — `ValidatePathSegment` / `ErrInvalidPathSegment` undefined.

- [ ] **Step 3: Implement the helpers**

Add to `transport/errors.go`:

```go
// ErrInvalidPathSegment indicates a decoded Request.Path segment is empty,
// `.` / `..`, contains `\` or a control character, or (for ValidatePathSegment)
// contains `/`. Returned errors also wrap ErrInvalidConfig — the chain is
// built where the violation is detected, inside the validators (REQ-150).
ErrInvalidPathSegment = errors.New("transport: invalid path segment")
```

`transport/path.go`:

```go
package transport

import (
	"fmt"
	"strings"
)

// ValidatePathSegment reports whether s is a single decoded path parameter (REQ-150).
func ValidatePathSegment(s string) error {
	if s == "" || s == "." || s == ".." {
		return fmt.Errorf("%w: %w", ErrInvalidConfig, ErrInvalidPathSegment)
	}
	for _, r := range s {
		if r == '/' || r == '\\' || r < 0x20 || r == 0x7F {
			return fmt.Errorf("%w: %w", ErrInvalidConfig, ErrInvalidPathSegment)
		}
	}
	return nil
}

// ValidateRequestPath validates a decoded Request.Path (REQ-150). The leading
// empty segment of an absolute path is ignored.
func ValidateRequestPath(path string) error {
	for i, seg := range strings.Split(path, "/") {
		if i == 0 && seg == "" {
			continue
		}
		if err := ValidatePathSegment(seg); err != nil {
			return err
		}
	}
	return nil
}
```

Wrapping rule: `ValidatePathSegment` builds the one error chain carrying both sentinels; `ValidateRequestPath` and `joinTarget` return that error **unchanged** — the chain wraps `ErrInvalidConfig` exactly once.

- [ ] **Step 4: Re-run the helper tests**

```
go test ./transport/ -run 'TestValidatePathSegment|TestValidateRequestPath' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```
git add transport/path.go transport/path_test.go transport/errors.go
git commit -m "$(cat <<'EOF'
feat(transport): add path-segment validation helpers

REQ-150: ValidatePathSegment / ValidateRequestPath refuse traversal
segments before any request is built.
EOF
)"
```

### Phase 2 — Enforce at the request-preparation point

**Files:**

- Modify: `transport/client.go` — `joinTarget` sees only `(base, path, query)`; the route-arity half needs `Request.Route`, so enforce both checks at `joinTarget`'s caller where the `*Request` is in scope. (Widening `joinTarget`'s signature instead is NOT sanctioned as-is: `Do`'s call site wraps every `joinTarget` error with `ErrInvalidConfig` already, so validators returning the double-sentinel chain from inside `joinTarget` would wrap `ErrInvalidConfig` twice, violating the wrapping rule below — taking that branch requires unwrapping at the call site in the same change.)
- Modify: `transport/request.go` (godoc — drop the “ids contain no `/`” assumption; point at REQ-150)
- Test: `transport/path_test.go` (Do / joinTarget cases)

- [ ] **Step 6: Write the failing Do test**

```go
func TestDoRejectsTraversalPath(t *testing.T) {
	var hits atomic.Int32 // Store/Load cross handler and test goroutines; -race aborts on a plain int
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		hits.Add(1)
	}))
	t.Cleanup(srv.Close)
	c, err := New(newCatalog(t, srv), WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Do(t.Context(), &Request{
		Method: http.MethodGet,
		Path:   "/ehr/a/../../definition/query/evil/composition",
	})
	if !errors.Is(err, ErrInvalidPathSegment) || !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("err = %v", err)
	}
	if hits.Load() != 0 {
		t.Fatalf("issued %d requests, want 0", hits)
	}
}

func TestDoRejectsSmuggledSeparator(t *testing.T) {
	// ehr_id="foo/bar" interpolated into the path: every segment is legal,
	// only the count betrays it — 5 path segments vs 4 in the route template.
	var hits atomic.Int32 // Store/Load cross handler and test goroutines; -race aborts on a plain int
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		hits.Add(1)
	}))
	t.Cleanup(srv.Close)
	c, err := New(newCatalog(t, srv), WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Do(t.Context(), &Request{
		Method: http.MethodGet,
		Path:   "/ehr/foo/bar/contribution/x",
		Route:  "/ehr/{ehr_id}/contribution/{contribution_uid}",
	})
	if !errors.Is(err, ErrInvalidPathSegment) || !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("err = %v", err)
	}
	if hits.Load() != 0 {
		t.Fatalf("issued %d requests, want 0", hits)
	}
}

```

`transport/path_encoding_test.go` already pins the REQ-095 single-encode behaviour (spaced template id → one `%20` on the wire). Do **not** duplicate it here — re-run it in Step 9 to prove the new check does not break it. Add a case there only if it lacks a joined `Do`-level assertion that a spaced id still issues exactly one request.

- [ ] **Step 7: Confirm it fails** (currently `joinTarget` issues the request)

```
go test ./transport/ -run 'TestDoRejectsTraversalPath|TestDoRejectsSmuggledSeparator' -count=1
```

Expected: FAIL for both — `hits != 0` or missing sentinel.

- [ ] **Step 8: Enforce both checks before the URL is built**

At the single request-preparation site (where the `*Request` is in scope):

```go
if err := ValidateRequestPath(req.Path); err != nil {
	return nil, err
}
if req.Route != "" && segmentCount(req.Path) != segmentCount(req.Route) {
	return nil, fmt.Errorf("%w: %w: path does not match route template arity", ErrInvalidConfig, ErrInvalidPathSegment)
}
```

`segmentCount` counts `/`-separated segments ignoring the leading empty one — an unexported helper beside `ValidateRequestPath`. A `Route`-less request skips the arity half (`Route` is optional on raw `transport.Do` use); every path-interpolating leaf sets it, which the REQ-150 spec text now requires — key the check on `req.Route != ""` directly, never on `effectiveRoute()` (it falls back to `Path`, and comparing a path's arity against itself never fires). Add the tripwire the spec mandates: a source-level test asserting every `transport.Request` literal under `openehr/client/` sets `Route`, so a new leaf cannot silently opt out of the smuggling defence.

- [ ] **Step 9: Re-run transport tests including REQ-095 encoding and the smuggling case**

```
go test ./transport/ -count=1
```

Expected: PASS.

- [ ] **Step 10: Commit**

```
git add transport/client.go transport/request.go transport/path_test.go
git commit -m "$(cat <<'EOF'
feat(transport): refuse illegal path segments before the URL is built

REQ-150: a traversal or control-character segment, or a path whose
segment count contradicts the route template, returns
ErrInvalidPathSegment and never hits the wire.
EOF
)"
```

### Phase 3 — Cross-leaf probe

**Files:**

- Create: `testkit/probes/versioned/probe_091_path_segment_validation.go`
- Modify: the package's `*_test.go` harness (follow PROBE-072)
- Test: one leaf httptest in `openehr/client/ehr/composition/` (`ehr_id = "a/../../definition/query/evil"`)

- [ ] **Step 11: Leaf httptest** — `composition.Get` with a traversal `ehrID` returns the sentinel and the server sees zero requests. Cite `// REQ-150`.

- [ ] **Step 12: Implement PROBE-091** as a Sandbox function that drives one path-interpolating call per `openehr/client` leaf package that builds its own `transport.Request` (ehr, composition, directory, ehrstatus, contribution, query, definition, demographic, admin — enumerate the driven calls in the probe file's doc comment; `itemtags` builds none, delegating wholly to composition / ehrstatus / directory, and `system`'s only path is the fixed service root, which is PROBE-091's positive `"/"` case rather than an interpolation site) with **two** hostile inputs each: a traversal id (`a/../../…`) and a separator-smuggling id (`foo/bar` — legal segments, wrong arity against the leaf's `Route`). Per REQ-080 the probe asserts fail-closed behaviour only: non-nil error and zero captured requests — **no** sentinel-identity assertions (those live in `transport/path_test.go`). Status in `conformance.md`: Draft → Implemented (Sandbox).

- [ ] **Step 13: Flip REQ-150 to landed** in `REQ.md` + `traceability.yaml` (packages, tests, probes); move the `roadmap.md` REQ-150 row from **Planned** to **Landed**. Archive this plan.

- [ ] **Step 14: Verify**

```
go test ./transport/ ./openehr/client/... ./testkit/probes/versioned/ -count=1
make spec-check
make ci
```

Expected: PASS.

## Mapping to specs

- [transport.md § REQ-150](../specifications/transport.md#req-150--path-parameter-segment-validation)
- [wire.md § REQ-095](../specifications/wire.md#req-095) — encode-once sibling
- [REQ.md](../specifications/REQ.md) — registry row
