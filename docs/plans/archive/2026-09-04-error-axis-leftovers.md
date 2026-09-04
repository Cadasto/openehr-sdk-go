# Plan — Error-axis leftovers (null bodies, nil receivers, decode-failure shapes, enum switches)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans (inline) or superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax. Before any Go edit load `go-coding:go-coding`, then `go-errors`, `go-testing`, and (Task 6) `go-lint-setup`.

**Date:** 2026-09-04
**Status:** landed (2026-09-05, archived in the implementing PR)
**Owner:** SDK maintainers
**Covers:** implementation-aligned amendments to landed [REQ-151](../../specifications/transport.md#req-151--typed-2xx-decode-failure), [REQ-094](../../specifications/transport.md#req-094--prefer-response-shape-negotiation), [REQ-144](../../specifications/wire.md#req-144--definition-metadata-decoding), [REQ-025](../../specifications/idiom.md#errors-req-025), [REQ-052](../../specifications/wire.md#req-052) — **no new requirement id**; every normative delta rides in this plan's PR and amends the owning section in place
**Probes:** none new; PROBE-101 unchanged in contract
**Implementation:** landed
**Depends on:** landed `transport/` (REQ-090–094, REQ-150, REQ-151), `openehr/rm/typereg`, `internal/bmmgen`, the hand-written primitive codecs `rm.Real` / `rm.Integer` / `rm.Character` (REQ-046 / REQ-052, PR #145)
**Defers:** STRAND-10 (`rmpath` not-found sentinel split — needs an ADR; its encode-side consequence is recorded in the maintainer's notes); STRAND-04 `encoding/json/v2` migration; any change to which errors carry `canjson.ErrInvalidShape` (Real's parse/range arm stays outside it by the pinned rule)

**Goal:** Drain the small, verified-open follow-ups that the REQ-151 / REQ-094 / REQ-025 / REQ-052 review rounds (PRs #140–#145) left in PR bodies and session notes, grouped because they all sit on one axis — what a decode failure or a misuse looks like to the caller — and each is under a day.

**Architecture:** One predicate (`transport.IsNoRepresentationBody`) becomes the single implementation of "this 2xx body carries no representation", used by `transport.Decode`, by every hand-rolled leaf decode, and by the REQ-094 write funnel, so a JSON `null` body can no longer present as a populated all-zero struct on any read path. The generator gains a nil-receiver guard on every `UnmarshalJSON` (a new `typereg.ErrNilReceiver` sentinel, shared with the hand-written primitives) and a census test proves it over the whole registry. Documentation gaps are closed where the fact lives (canjson `doc.go`, wire.md § REQ-052, idiom.md § REQ-025), `typereg`'s error surface moves to its own file, and the `exhaustive` linter joins the gate with its findings fixed rather than suppressed.

**Tech Stack:** Go 1.27.0 (module floor, REQ-002), `encoding/json`, `internal/bmmgen` (regenerate with `make codegen`), golangci-lint v2.13.2 via the Makefile's pinned Docker image (the host binary is 1.26-built and refused by `HOST_GLCI_OK`).

## Global Constraints

- **REQ-025:** library code MUST NOT panic on wire or caller input; a nil receiver is caller-constructible input.
- **REQ-024:** no reflection in library code; tests MAY use `reflect` (precedent: `canjson/field_order_test.go`).
- **REQ-093 / REQ-151:** boundary error strings (`WireError.Error()`, `DecodeError.Error()`) stay value-free; codec causes may name a literal.
- **REQ-013:** `openehr/*` packages import no `transport/` or `auth/` — the predicate lives in `transport` and is used by `openehr/client/*`, which already import it.
- **Formatting:** run `$(go env GOROOT)/bin/gofmt -l` from this checkout (host gofmt is 1.26-built and rejects generic methods); `make fmt-check` / `make lint` route through Docker.
- **Commits:** Conventional Commits, one per task; CHANGELOG bullets only at close-out (Task 7), one sentence per artefact class.
- **Verification commands:** `go build ./...`, `go test ./... -count=1`, `make codegen-verify`, `make lint`, `make spec-check`, `make ci`.

## Definition of Ready

Implementation may start when:

- **`Covers:`** lists every REQ this plan amends — done above; each has canonical prose and a registry row already.
- No new REQ id, probe id or ADR is needed — verified: every item amends a landed requirement in place, and the one design fork on this axis (STRAND-10) is explicitly deferred.
- Each task below names its files, its tests and its verification command.

## Definition of Done

- `transport.IsNoRepresentationBody` is the only response-body no-representation predicate in the client and transport trees (`grep -rn 'byte("null")' openehr/client transport` finds one hit, in `transport/body.go`; the two JSON-presence helpers in `openehr/validation` and `openehr/bmm` test key presence, not response bodies, and may not import `transport` under REQ-013).
- Every generated `UnmarshalJSON` and every hand-written primitive codec returns an error carrying `typereg.ErrNilReceiver` on a nil receiver, proven by a census over `typereg.Default.Names()`.
- `exhaustive` is enabled in `.golangci.yml` and `make lint` is clean with zero `//nolint:exhaustive` directives.
- transport.md § REQ-151, wire.md § REQ-052 and § REQ-144, idiom.md § REQ-025 carry the amended sentences; `traceability.yaml` lists the new tests; CHANGELOG has its bullets.
- `make spec-check` and `make ci` pass; plan archived under `archive/` in this PR.

## Implementation checklist

| Step | Status |
|---|---|
| Task 1 — `transport.IsNoRepresentationBody` + `Decode` null arm + § REQ-151 sentence | done |
| Task 2 — every hand-rolled leaf and the write funnel use the predicate | done |
| Task 3 — `typereg.ErrNilReceiver`, generator guard, regen, census test, § REQ-025 | done |
| Task 4 — duplicate `%q` echo dropped; canjson fourth decode-failure shape; § REQ-052 sentence | done |
| Task 5 — `typereg/errors.go` split | done |
| Task 6 — `exhaustive` linter enabled, findings fixed | done |
| Task 7 — CHANGELOG, traceability, plan indexes, `make spec-check`, `make ci`, archive | done |

---

## Task 1: `transport.IsNoRepresentationBody` and the `Decode` null arm

**Files:**
- Create: `transport/body.go`
- Modify: `transport/client.go` (`Decode`, the `len(resp.Body) == 0` arm and its doc comment)
- Modify: `docs/specifications/transport.md` § REQ-151, paragraph **An empty 2xx body keeps its existing per-surface contract**
- Test: `transport/decode_error_test.go`

**Interfaces:**
- Produces: `func IsNoRepresentationBody(b []byte) bool` — true for zero bytes, whitespace only, or the JSON `null` literal (surrounding whitespace ignored). Tasks 2 and 3 rely on this exact name.

- [ ] **Step 1: Write the failing test**

```go
// transport/decode_error_test.go
func TestDecodeNullBodyStaysInvalidShape(t *testing.T) { // REQ-151
	t.Parallel()
	for _, body := range []string{"null", " \n null \t", "   ", "\n"} {
		t.Run(fmt.Sprintf("%q", body), func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, body)
			}))
			defer srv.Close()
			c := newTestClient(t, srv) // whatever helper decode_error_test.go already uses
			out, _, err := transport.Decode[struct{ Name string }](t.Context(), c, &transport.Request{Method: http.MethodGet, Path: "/x", Route: "/x"})
			if out != nil {
				t.Fatalf("Decode(%q) returned a value %+v; a null body must not present as a populated struct", body, *out)
			}
			if !errors.Is(err, transport.ErrInvalidShape) {
				t.Fatalf("Decode(%q) err = %v, want errors.Is ErrInvalidShape", body, err)
			}
			if _, ok := errors.AsType[*transport.DecodeError](err); ok {
				t.Fatalf("Decode(%q) err = %T; the no-representation arm must not be a DecodeError", body, err)
			}
		})
	}
}

func TestIsNoRepresentationBody(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want bool
	}{
		{"", true}, {"   ", true}, {"null", true}, {"\tnull\n", true},
		{"{}", false}, {"[]", false}, {"nul", false}, {"\"null\"", false}, {"0", false},
	}
	for _, tc := range cases {
		if got := transport.IsNoRepresentationBody([]byte(tc.in)); got != tc.want {
			t.Errorf("IsNoRepresentationBody(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `go test ./transport/ -run 'TestDecodeNullBody|TestIsNoRepresentationBody'`. Expected: compile error (undefined `IsNoRepresentationBody`); after stubbing the func to `return len(b) == 0`, the `null` cases fail because `Decode` returns a zero struct and nil error.

- [ ] **Step 3: Implement**

```go
// transport/body.go
package transport

import "bytes"

var jsonNull = []byte("null")

// IsNoRepresentationBody reports whether a 2xx response body carries no
// representation: zero bytes, whitespace only, or the JSON `null` literal.
//
// It is the single implementation of the "empty body" both REQ-094 (the
// write-result funnel) and REQ-151 (every other 2xx decode) classify.
// `encoding/json` unmarshals `null` into a struct as a nil-error no-op, so a
// decoder that only checks len(body) == 0 would hand back a populated-looking
// all-zero value; classifying against the raw bytes before decode is what
// keeps that arm honest.
func IsNoRepresentationBody(b []byte) bool {
	body := bytes.TrimSpace(b)
	return len(body) == 0 || bytes.Equal(body, jsonNull)
}
```

In `Decode`, replace `if len(resp.Body) == 0 {` with `if IsNoRepresentationBody(resp.Body) {` and the message with `"%w: response body is empty or null (Prefer mismatch?)"`. Update the doc comment's "an empty body fails with [ErrInvalidShape]" to "an empty, whitespace-only or JSON-null body fails with [ErrInvalidShape] (see [IsNoRepresentationBody])".

- [ ] **Step 4: Spec** — in transport.md § REQ-151, append to the paragraph **An empty 2xx body keeps its existing per-surface contract**:

> *Empty*, throughout this §, has the meaning [§ REQ-094](#req-094--prefer-response-shape-negotiation) gives it: zero bytes, whitespace only, or the JSON `null` literal. A `null` body unmarshals into a struct as a nil-error no-op, so every arm below **MUST** classify against the raw bytes ahead of decode — `transport.IsNoRepresentationBody` is the single implementation of that predicate, and a hand-rolled leaf **MUST** call it rather than test `len(body) == 0`.

- [ ] **Step 5: Verify** — `go test ./transport/ -count=1`; `$(go env GOROOT)/bin/gofmt -l transport/`. Expected: PASS, no files listed.

- [ ] **Step 6: Commit** — `git commit -m "fix(transport): classify a JSON-null 2xx body as no representation (REQ-151)"`

## Task 2: Every hand-rolled leaf and the write funnel use the predicate

**Files:**
- Modify: `openehr/client/ehr/composition/composition.go` (the `len(resp.Body) == 0` arm after the 204 carve-out)
- Modify: `openehr/client/system/system.go` (`Capabilities`)
- Modify: `openehr/client/demographic/party.go` (`getParty`) and `openehr/client/demographic/versioned.go` (the version read) — keep the `204 → nil` carve-out inside the predicate branch exactly as today
- Modify: `openehr/client/definition/template.go` (`UploadTemplate` synthesized arm; `ListTemplates`) and `openehr/client/definition/stored_query.go` (`putStoredQuery`, `GetStoredQuery`, `ListStoredQueries`)
- Modify: `openehr/client/ehr/write.go` (delete `isNoRepresentationBody`, call `transport.IsNoRepresentationBody`), `openehr/client/ehr/ehr.go`, `openehr/client/ehr/contribution/contribution.go` (replace the inline `bytes.TrimSpace` + `bytes.Equal` pair)
- Modify: `docs/specifications/wire.md` § REQ-144 **Empty list bodies** — add "(empty as [§ REQ-151](transport.md#req-151--typed-2xx-decode-failure) defines it — zero bytes, whitespace, or JSON `null`)"
- Tests: `composition/composition_test.go`, `system/system_test.go`, `demographic/party_test.go`, `definition/template_test.go`, `definition/stored_query_test.go`

**Interfaces:**
- Consumes: `transport.IsNoRepresentationBody` from Task 1.

- [ ] **Step 1: Write the failing tests** — one `null`-body case per surface, each asserting the surface-specific facet (not merely "an error"):

```go
// composition_test.go — REQ-151 refusal arm
func TestGetNullBodyIsInvalidShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, "null")
	}))
	defer srv.Close()
	out, _, err := composition.Get(t.Context(), newClient(t, srv), ehrID, ref)
	if out != nil || !errors.Is(err, transport.ErrInvalidShape) {
		t.Fatalf("Get(null body) = %v, %v; want nil and ErrInvalidShape", out, err)
	}
	if _, ok := errors.AsType[*transport.DecodeError](err); ok {
		t.Fatalf("Get(null body) err = %T; a null body is the no-representation arm, not a decode failure", err)
	}
}
// system_test.go — same shape for Capabilities.
// party_test.go — getParty: 200 + "null" → ErrInvalidShape; 204 + "null" → nil, nil (carve-out on status alone).
// template_test.go — ListTemplates: 200 + "null" → non-nil zero-length slice, nil error (REQ-144).
//                    UploadTemplate: 201 + "null" + Location → synthesized TemplateMetadata, nil error.
// stored_query_test.go — ListStoredQueries: "null" → non-nil empty slice; GetStoredQuery: "null" → synthesized {Name, Version}.
```

- [ ] **Step 2: Run to verify they fail** — the list tests fail with `list = nil` (a `null` unmarshals to a nil slice); the refusal-arm tests fail with `err = <nil>` and an all-zero value.

- [ ] **Step 3: Implement** — at each site replace `len(resp.Body) == 0` with `transport.IsNoRepresentationBody(resp.Body)`; in `ehr/write.go` remove the local helper and its `bytes` import if now unused; in `contribution.go` replace the inline check. Keep every existing message and error type; only the predicate widens.

- [ ] **Step 4: Verify** — `go test ./openehr/client/... -count=1`; `gofmt -l openehr/client`; `grep -rn 'byte("null")' --include=*.go . | grep -v _test` must list only `transport/body.go`.

- [ ] **Step 5: Commit** — `git commit -m "fix(client): route every 2xx no-representation check through transport.IsNoRepresentationBody (REQ-151, REQ-094, REQ-144)"`

## Task 3: Nil-receiver guard on every generated `UnmarshalJSON` (REQ-025)

**Files:**
- Modify: `openehr/rm/typereg/registry.go` (add `ErrNilReceiver` to the sentinel block; Task 5 moves it)
- Modify: `internal/bmmgen/render_jsonunmar.go` line ~335 (emit the guard as the method's first statement)
- Regenerate: `make codegen` (29 `openehr/rm/*_jsonunmar_gen.go` + 6 `openehr/aom/aom14/*_jsonunmar_gen.go`)
- Modify: `openehr/rm/real.go`, `openehr/rm/integer.go`, `openehr/rm/character.go` (three `UnmarshalJSON` guards + `Character.UnmarshalText`) — wrap the sentinel instead of a bare `errors.New`
- Modify: `docs/specifications/idiom.md` § REQ-025 **No panics**
- Test: `openehr/rm/nilreceiver_census_test.go` (package `rm_test`), plus `internal/bmmgen/render_jsonunmar_polymorphic_test.go` (assert the rendered source contains the guard)

**Interfaces:**
- Produces: `var typereg.ErrNilReceiver = errors.New("typereg: nil receiver")`.

- [ ] **Step 1: Write the failing census test**

```go
// openehr/rm/nilreceiver_census_test.go
package rm_test

// REQ-025: a nil receiver on any UnmarshalJSON is caller-constructible input
// and MUST fail with an error, never a nil dereference. The census runs over
// the whole registry (RM + AOM 1.4 register into typereg.Default) plus the
// three hand-written primitives the registry does not hold. Tests may use
// reflect (REQ-024 binds library code; canjson/field_order_test.go is the
// precedent) — it is the only way to manufacture a typed-nil of a type known
// only by its constructor.
func TestNilReceiverUnmarshalJSONCensus(t *testing.T) {
	t.Parallel()
	names := typereg.Default.Names()
	if len(names) < 100 {
		t.Fatalf("registry holds %d types; the census expects the full RM + AOM inventory", len(names))
	}
	for _, name := range names {
		ctor, _ := typereg.Default.Lookup(name)
		typedNil := reflect.Zero(reflect.TypeOf(ctor())).Interface()
		u, ok := typedNil.(json.Unmarshaler)
		if !ok {
			continue // a registered type without its own UnmarshalJSON is decoded by encoding/json itself
		}
		t.Run(name, func(t *testing.T) {
			err := callWithoutPanicking(t, func() error { return u.UnmarshalJSON([]byte(`{}`)) })
			if !errors.Is(err, typereg.ErrNilReceiver) {
				t.Fatalf("%s: (nil).UnmarshalJSON = %v, want errors.Is typereg.ErrNilReceiver", name, err)
			}
		})
	}
	for name, u := range map[string]json.Unmarshaler{
		"rm.Real": (*rm.Real)(nil), "rm.Integer": (*rm.Integer)(nil), "rm.Character": (*rm.Character)(nil),
	} {
		if err := u.UnmarshalJSON([]byte(`1`)); !errors.Is(err, typereg.ErrNilReceiver) {
			t.Errorf("%s: (nil).UnmarshalJSON = %v, want errors.Is typereg.ErrNilReceiver", name, err)
		}
	}
	if err := (*rm.Character)(nil).UnmarshalText([]byte("x")); !errors.Is(err, typereg.ErrNilReceiver) {
		t.Errorf("rm.Character: (nil).UnmarshalText = %v, want errors.Is typereg.ErrNilReceiver", err)
	}
}

func callWithoutPanicking(t *testing.T, f func() error) (err error) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panicked: %v", r)
		}
	}()
	return f()
}
```

- [ ] **Step 2: Run to verify it fails** — `go test ./openehr/rm/ -run TestNilReceiverUnmarshalJSONCensus`. Expected: FAIL — today every generated type panics (nil dereference on `x.Field = ...`) and the census reports it via `callWithoutPanicking`; the primitives return an error that is not the sentinel.

- [ ] **Step 3: Implement**

In `typereg/registry.go`, inside the existing `var (...)` sentinel block:

```go
	// ErrNilReceiver classifies an UnmarshalJSON / UnmarshalText call on a
	// nil receiver — caller-constructible misuse the no-panic rule (REQ-025)
	// turns into an error instead of a dereference. Every generated
	// UnmarshalJSON and the hand-written primitive codecs wrap it.
	ErrNilReceiver = errors.New("typereg: nil receiver")
```

In `render_jsonunmar.go`, immediately after the `func (%s *%s%s) UnmarshalJSON(data []byte) error {` line:

```go
	fmt.Fprintf(&b, "\tif %s == nil {\n", recv)
	fmt.Fprintf(&b, "\t\treturn fmt.Errorf(\"canjson: %s: %%w\", typereg.ErrNilReceiver)\n", pc.BMMName)
	b.WriteString("\t}\n")
```

(`fmt` and `typereg` are already imported by every generated `*_jsonunmar_gen.go`; no import-list change.) Then `make codegen`, and in each hand-written codec replace `errors.New("rm.Real: nil receiver")` with `fmt.Errorf("rm.Real: %w", typereg.ErrNilReceiver)` (likewise Integer, Character ×2). Drop the now-unused `errors` import only where nothing else uses it.

- [ ] **Step 4: Spec** — in idiom.md § REQ-025 **No panics**, add a bullet after "Consumer input":

> - A nil receiver on a decode method — every generated `UnmarshalJSON`, and the hand-written primitive codecs' `UnmarshalJSON` / `UnmarshalText` — return an error carrying `typereg.ErrNilReceiver`. A census over the type registry pins it.

- [ ] **Step 5: Verify** — `make codegen-verify` (no drift), `go test ./openehr/rm/... ./internal/bmmgen/... -count=1`, `go vet ./...`. Expected: PASS.

- [ ] **Step 6: Commit** — `git commit -m "fix(rm): refuse a nil receiver in every generated UnmarshalJSON with typereg.ErrNilReceiver (REQ-025)"`

## Task 4: Duplicate value echo dropped; the fourth decode-failure shape documented (REQ-052)

**Files:**
- Modify: `openehr/rm/real.go` line ~133, `openehr/rm/integer.go` line ~35 — `fmt.Errorf("rm.Real: parse %q: %w", s, err)` → `fmt.Errorf("rm.Real: parse quoted literal: %w", err)` (the wrapped `*strconv.NumError` already quotes the literal once)
- Modify: `openehr/serialize/canjson/doc.go` — add a fourth bullet to *What a decode failure looks like*
- Modify: `docs/specifications/wire.md` § REQ-052 **Decode-side shape sentinel** — one sentence
- Modify: `openehr/client/ehr/write.go` `NoRepresentationError` doc and `traceability.yaml` REQ-151 notes — both say rm decode errors embed values "in `parse %q` form"; reword to "the wrapped strconv / encoding/json cause quotes the literal"
- Test: `openehr/rm/real_test.go` — assert the message contains the literal exactly once (`strings.Count(err.Error(), lit) == 1`) for a quoted malformed literal; same for Integer

- [ ] **Step 1: Write the failing test**

```go
func TestQuotedLiteralParseErrorNamesTheLiteralOnce(t *testing.T) { // REQ-052
	t.Parallel()
	var r rm.Real
	err := r.UnmarshalJSON([]byte(`"12x"`))
	if err == nil {
		t.Fatal("UnmarshalJSON(\"12x\") = nil, want a parse error")
	}
	if n := strings.Count(err.Error(), "12x"); n != 1 {
		t.Errorf("error %q names the literal %d times, want exactly once (the strconv cause already quotes it)", err, n)
	}
	var i rm.Integer
	err = i.UnmarshalJSON([]byte(`"7y"`))
	if n := strings.Count(err.Error(), "7y"); err == nil || n != 1 {
		t.Errorf("Integer: err = %v names the literal %d times, want exactly once", err, n)
	}
}
```

- [ ] **Step 2: Run to verify it fails** — count is 2 today.

- [ ] **Step 3: Implement** the two one-line message changes.

- [ ] **Step 4: Docs** — canjson `doc.go`, fourth bullet:

```
//   - A hand-written primitive decoded at the top level — rm.Real, rm.Integer
//     or rm.Character handed to [Unmarshal] directly rather than reached
//     through a generated type — carries its own `rm.<Type>:` prefix, not the
//     `canjson: <RM_TYPE>:` funnel. Real's precision refusal and every
//     Character refusal wrap [ErrInvalidShape]; a strconv or encoding/json
//     parse or range failure beneath any of the three carries no sentinel,
//     by the precedence rule wire.md § REQ-052 states, and stays reachable
//     with errors.AsType.
```

wire.md § REQ-052, appended to the **Decode-side shape sentinel** paragraph:

> A cause beneath the sentinel **MAY** name the offending literal — `*strconv.NumError` and `*json.UnmarshalTypeError` both do — because the value-free discipline binds the boundary strings (`WireError.Error()`, `DecodeError.Error()`; [§ REQ-093](transport.md#req-093--openehr-error-envelope-mapping), [§ REQ-151](transport.md#req-151--typed-2xx-decode-failure)), not codec causes. A codec's own prefix **MUST NOT** repeat a value the wrapped cause already carries.

- [ ] **Step 5: Verify** — `go test ./openehr/rm/ ./openehr/serialize/canjson/ -count=1`; `go doc ./openehr/serialize/canjson | head -80` shows the bullet.

- [ ] **Step 6: Commit** — `git commit -m "docs(rm,canjson): name a quoted literal once and document the top-level primitive decode-failure shape (REQ-052)"`

## Task 5: `typereg/errors.go` split

**Files:**
- Create: `openehr/rm/typereg/errors.go`
- Modify: `openehr/rm/typereg/registry.go` (remove what moved), `openehr/rm/shape_classified.go` comment (`registry.go` → `errors.go`)

- [ ] **Step 1: Move** the sentinel `var (...)` block (`ErrMissingType` … `ErrInvalidShape`, `ErrNilReceiver`), `DecodeError` with its methods, `WrapShapeError`, `shapeError` with its methods, verbatim, into `errors.go` with the package doc line `// Error surface of the type registry: sentinels, DecodeError, and the shape-classification wrapper the generated funnel uses.` Keep `jsonNestingDepth`, `Registry`, `Decode`, `DecodeAs` in `registry.go`.

- [ ] **Step 2: Verify** — `go build ./... && go test ./openehr/rm/typereg/ -count=1`, then compare the exported surface: `go doc ./openehr/rm/typereg | grep -E '^(func|type|var|const)' | sort` on this branch must equal the same command's output on the base commit (run it in the main checkout). Expected: identical list, tests PASS.

- [ ] **Step 3: Commit** — `git commit -m "refactor(typereg): move the error surface into errors.go"`

## Task 6: `exhaustive` linter enabled, findings fixed

**Files:**
- Modify: `.golangci.yml` — add `- exhaustive` to `linters.enable` (after `errorlint`, comment `# every enum member spelled in a switch; default never signifies exhaustive`), and under `linters.settings`:
  ```yaml
    exhaustive:
      default-signifies-exhaustive: false
      ignore-enum-types: '^reflect\.Kind$'   # canxml.isNil lists the nil-able kinds only; enumerating 21 others is noise
  ```
- Modify the seven remaining sites:
  1. `internal/bmmgen/render_jsonmar.go:377` — add `case polyNone:` (comment: a non-polymorphic property has no kind to report) before the trailing `return polyNone`.
  2. `internal/bmmgen/render_jsonunmar.go:311` — `default:` → `case polyNone:` for the `renderField` body, and a new `default:` returning `fmt.Errorf("render wire field %s.%s: unknown polymorphic kind %v", pc.BMMName, propName, kind)` so a future kind fails loud.
  3. `openehr/aql/top.go:29` — add `case TopDirUnspecified: return ""` before the trailing `return ""` (which stays for out-of-range values).
  4. `openehr/client/ehr/write.go:121` — replace `default:` with `case transport.PreferDefault, transport.PreferMinimal:` returning `zero, meta, nil`, and keep a `default:` doing the same with the comment `// an unknown Prefer decodes nothing: metadata-only is the fail-safe arm (REQ-094)`.
  5. `openehr/rm/rminfo/probe_094_test.go:257` — add `case unaccountedFor:` returning the existing `"no declared exclusion matches", false` text; keep the trailing return.
  6. `openehr/serialize/simplified/flat_decode.go:1764` — add `case kindString: return "string"`; `default:` returns `fmt.Sprintf("suffixKind(%d)", int(k))`.
  7. `smart/discovery/errors.go:81` — add `case ReasonFetchFailed, ReasonParseError, ReasonMalformedURL, ReasonAuthEndpointsMissing, ReasonInsecureURL, ReasonIssuerMismatch: // no reason-specific detail`.

- [ ] **Step 1: Run the linter before changing code** — `docker run --rm -v "$PWD":/app -w /app -e GOFLAGS=-buildvcs=false golangci/golangci-lint:v2.13.2-alpine golangci-lint run --enable-only=exhaustive ./...`. Expected: 8 findings (the census above).

- [ ] **Step 2: Edit config and the seven sites.**

- [ ] **Step 3: Verify** — rerun the Step 1 command: 0 findings; `make lint` clean; `go test ./internal/bmmgen/ ./openehr/aql/ ./openehr/client/ehr/ ./openehr/rm/rminfo/ ./openehr/serialize/... ./smart/discovery/ -count=1` PASS; `grep -rn "nolint:exhaustive" --include=*.go .` empty.

- [ ] **Step 4: Commit** — `git commit -m "build(lint): enable exhaustive and spell every enum member in the eight flagged switches"`

## Task 7: Close-out

- [ ] **CHANGELOG** `## [Unreleased] / ### Added` — three one-sentence bullets, artefact-class grain:
  - **Null 2xx bodies classify as no representation (REQ-151, REQ-094, REQ-144).** `transport.IsNoRepresentationBody` is the one predicate behind `transport.Decode`, every hand-rolled leaf read, the Definition list and synthesized-metadata arms, and the write funnel, so a JSON `null` body no longer decodes as an all-zero resource.
  - **Nil-receiver guard on every generated decoder (REQ-025).** Every generated `UnmarshalJSON` and the hand-written primitive codecs return an error carrying `typereg.ErrNilReceiver` instead of dereferencing a nil receiver, pinned by a registry-wide census.
  - **Quoted-literal parse errors name the literal once (REQ-052).** `rm.Real` / `rm.Integer` stop repeating a value their wrapped `strconv` cause already quotes; the top-level primitive decode-failure shape is documented.
- [ ] **traceability.yaml** — REQ-151: add `transport/body.go` intent to notes, tests `+ composition/system/demographic/definition null cases` (file paths only); REQ-025: packages `+ openehr/rm, openehr/aom/aom14, internal/bmmgen`, tests `+ openehr/rm/nilreceiver_census_test.go`; REQ-052: tests `+ openehr/rm/real_test.go` (already listed? check) and notes on the echo rule; REQ-144: tests already cover `template_test.go` / `stored_query_test.go`.
- [ ] **Indexes** — `docs/plans/README.md`: this plan's row (landed + archived); `docs/plans/archive/README.md`: row; `git mv` this file to `archive/`, set **Status:** landed / **Implementation:** landed.
- [ ] **Gates** — `make spec-check`, `make ci`. Expected: green.
- [ ] **Commit + PR** — `git commit -m "docs(plan): archive the error-axis leftovers plan in its implementing PR"`; before pushing, `gh pr list` and `git fetch origin main` to confirm no collision with a parallel session; push; open the PR with the summary, the one-home-per-fact pointers (this plan, the amended §§), and a "Follow-ups (not in this PR)" section naming STRAND-10.

## Mapping to specs

- [transport.md § REQ-151](../../specifications/transport.md#req-151--typed-2xx-decode-failure) — the *empty* definition and the predicate (Tasks 1–2)
- [transport.md § REQ-094](../../specifications/transport.md#req-094--prefer-response-shape-negotiation) — owns the definition the § REQ-151 sentence cites (Task 1)
- [wire.md § REQ-144](../../specifications/wire.md#req-144--definition-metadata-decoding) — empty list bodies (Task 2)
- [idiom.md § REQ-025](../../specifications/idiom.md#errors-req-025) — nil receiver bullet (Task 3)
- [wire.md § REQ-052](../../specifications/wire.md#req-052) — decode-side shape sentinel, echo sentence (Task 4)
- [REQ.md](../../specifications/REQ.md) — no row changes (all covered REQs stay `landed`)
