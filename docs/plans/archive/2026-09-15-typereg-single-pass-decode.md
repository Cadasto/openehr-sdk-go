# Plan: single-pass concrete decode in typereg (drop the buffer-and-peek on the concrete path)

**Date:** 2026-09-15
**Status:** landed (2026-09-15, archived in the implementing PR). Scope: the concrete decode path only (ruling F9). The precedence question is ruled below (F8): documented and pinned by a positive test. The registry path's regression is ruled below (F12): resolved, not deferred.
**Owner:** SDK maintainers
**Worktree:** `<worktree>`, branch `perf/typereg-single-pass-decode`, from `7a11918e` (stacked on follow-up A at planning time). The branch was later rebased onto `main` at `f1c88e8d` after PR 173 merged, and PR 174 targets `main`. The main checkout is never touched.
**Covers:** [REQ-052](../../specifications/wire.md#req-052) (Canonical JSON, Impl. `landed`, no status change; two descriptive clauses amended in place, implementation-aligned). [REQ-108](../../specifications/clinical-modeling.md#req-108--untrusted-document-bounds) (Untrusted document bounds, no status change; the polymorphic-decode depth bullet amended in place, implementation-aligned) and [REQ-025](../../specifications/idiom.md#errors-req-025) (Error wrapping, no status change; the nil-argument guard) through the post-review amendments. No REQ id is allocated and no probe id is allocated. Exercised through [PROBE-030](../../specifications/conformance.md#probe-030--canonical-json-round-trip) and [PROBE-031](../../specifications/conformance.md#probe-031----type-discriminator-decoded-via-registry).
**Probes:** none rewritten. PROBE-030 (round trip) and PROBE-038 (polymorphic decode) exercise the changed path and stay green; PROBE-031 (unknown `_type` via `Registry.Decode`) is untouched because the registry keeps its peek.
**Implementation:** one runtime helper signature, one generator template (both branches), a regeneration (`make codegen`), three godoc rewrites, two spec clause amendments, one ADR consequences line.
**Depends on:** the landed json/v2 codec on this branch (ADR 0022, the archived plan `docs/plans/archive/2026-09-14-json-v2-migration.md`), and ruling R21/R26 there, which named this follow-up (`docs/plans/archive/2026-09-14-json-v2-migration.md:683,706`).
**Defers:** the polymorphic and registry decode paths keep the buffer-and-peek. `DecodePolymorphic` (`openehr/rm/typereg/streaming.go:247`) and `Registry.Decode` (`openehr/rm/typereg/registry.go:140`) must read `_type` before choosing a constructor, and their benchmarks are within the R26 threshold today, so a token-scanning peek for them is a later question, not this plan.

## Goal

The concrete-type decode does three passes per nesting level: `dec.ReadValue()` buffers the subtree (`streaming.go:187`), `peekType` unmarshals that buffer into a one-field struct to read `_type` (`streaming.go:193`, `:310`), then `json.Unmarshal` decodes the buffer into `out` (`streaming.go:201`). Task 9 of the migration measured `BenchmarkDecodeDVQuantity` at +37.88 percent time/op against the v1 baseline (`docs/plans/archive/2026-09-14-json-v2-migration.md:635`), the only benchmark of ten that crossed the R26 line, with allocations up from 3 to 5.

The generated decode targets already declare `_type`: the zero-copy alias wrapper as `Type string ` + "`json:\"_type\"`" + ` and the flat wire struct as `Class string ` + "`json:\"_type\"`" + `. So the discriminator can be read from the wrapper field after one decode, and the buffer and the peek deleted. `DecodeInto` decodes straight from the decoder with `json.UnmarshalDecode`, then compares the declared field to the expected type. The scratch benchmark below measures the concrete path alone at **-52.75 percent time/op** and back to 3 allocations, which recovers the regression and lands faster than the v1 baseline.

## How the evidence was produced

Read-only. Two scratch artifacts outside the worktree, under a local scratch directory `<scratch>/peek/`:

- `peek/`: a standalone module confirming the `encoding/json/v2` semantics the design rests on (discriminator population, single-value consumption and decoder positioning, the error types for malformed / shape / wrong-type, `RejectUnknownMembers`, and the `SemanticError.JSONPointer` root). Eight tests, all pass.
- `peek/benchmod/`: a module with a `replace` directive into the worktree, so both benchmark arms decode into the real `rm.DVQuantity` and differ only in the `DecodeInto` shape. `benchstat` over `-count=12`.

The worktree stayed clean throughout (`git status --porcelain` empty, no commits). The v2 symbols are cited from `go doc` on the host (`go1.27.1`).

A benchmark-harness caveat worth recording so the implementer does not repeat it: a first cut drove both arms through `jsontext.NewDecoder(bytes.NewReader(body))` per iteration and reported the single-pass arm as *slower* and 5.5 KB heavier. That was an artifact of the unpooled read buffer a decoder over an `io.Reader` allocates (present in both arms, but masking the real delta). The real `canjson.Unmarshal([]byte)` uses a pooled, byte-slice-backed decoder; re-running both arms through `json.Unmarshal([]byte, ...)` (the same pooled entry) revealed the true 2x speedup below. Benchmark the concrete path through a byte-slice entry, never a fresh `io.Reader` decoder.

## Findings

### Item 1: the concrete path today

Control flow of `DecodeInto` (`streaming.go:186-206`) and the helpers it uses:

| Symbol | `file:line` | Role today |
|---|---|---|
| `DecodeInto` | `streaming.go:186` | `ReadValue` (187) → `peekType` (193) → `_type` mismatch guard (194-199) → `json.Unmarshal(raw, out, decodeOptions(dec))` (201) → `classifyDecode` on failure (202) |
| `peekType` | `streaming.go:310` | `json.Unmarshal(raw, &typeDiscriminator{})`; the second pass, reads `_type` from the buffered bytes |
| `classifyDecode` | `streaming.go:215` | a `*DecodeError` with an empty `Path` gets the slot pointer filled from `slotPointer`; every other failure goes through `WrapShapeError` |
| `slotPointer` | `streaming.go:228` | the outermost `*json.SemanticError`'s `JSONPointer`, the slot position |
| `decodeOptions` | `streaming.go:48` | joins the caller options, the interface hooks and v2 error semantics; **unchanged** by this plan |

Error order today, and after single-pass, for the six inputs the brief names (confirmed in the `peek/` scratch module):

| Input into a concrete target `T` | Today | After single-pass | Same result? |
|---|---|---|---|
| Well-formed, matching `_type` | success | success | yes |
| Well-shaped, **wrong** `_type` (e.g. `DV_TEXT` bytes into `DVQuantity`) | `peekType` reads the wrong name, the guard returns `*DecodeError{Path:"/_type"}` wrapping `ErrTypeMismatch`; the body is never decoded | decode succeeds (the extra members are ignored by default), then the same `*DecodeError{Path:"/_type"}` + `ErrTypeMismatch` | yes (outcome), the receiver is now mutated before the mismatch is reported |
| **Missing** `_type` on a concrete target | `peekType` returns `""`, the guard passes, decode proceeds | the declared field stays `""`, the guard passes, decode proceeds | yes |
| **Malformed** JSON (syntactic) | `ReadValue` returns a `*jsontext.SyntacticError` unwrapped (`streaming.go:187-191`); no sentinel | `json.UnmarshalDecode` returns the same `*jsontext.SyntacticError`; the new classify passes it through unwrapped; no sentinel | yes, **only if** the syntactic passthrough is added (see item 5) |
| Malformed JSON **and** wrong `_type` | `ReadValue` fails first; the wrong `_type` is never seen; syntactic error, no sentinel | decode fails on the syntactic error; the wrong `_type` is never seen; syntactic error, no sentinel | yes |
| **Well-shaped-but-wrong-shape body** and wrong `_type` (e.g. `{"_type":"DV_TEXT","magnitude":"x"}` into `DVQuantity`) | `peekType` fires the `_type` guard first: `ErrTypeMismatch` on `/_type`, no shape sentinel | decode fails on `magnitude` first: `*json.SemanticError` wrapped by `WrapShapeError` into `ErrInvalidShape`; the `_type` guard never runs | **no** (the precedence flip, item 3) |
| Duplicate member name | `ReadValue` refuses it (`*jsontext.SyntacticError` wrapping `jsontext.ErrDuplicateName`), returned unwrapped; the canjson entry's `ClassifyDuplicate` tags `ErrInvalidShape` with no prefix | `json.UnmarshalDecode` refuses it identically; the passthrough returns it unwrapped; `ClassifyDuplicate` tags it the same way | yes, **only if** the syntactic passthrough is added. Dated note (2026-09-24): on the rebased branch `classifyDecode` itself attaches `ErrInvalidShape` to a duplicate-name refusal (`main`'s `2b1be45b`, kept on the single pass), so a bare `encoding/json/v2` caller gets the sentinel too |
| Nested polymorphic slot failing beneath | the slot hook returns `*DecodeError{Path:""}`, the json package attaches the `JSONPointer`, `classifyDecode` fills `Path` from `slotPointer`, `WrapShapeError` keeps it outside the shape sentinel for a dispatch failure | identical: the polymorphic hook is unchanged, and for a top-level value the `JSONPointer` roots at the same object | yes for every failure directly under the top-level value |

### Item 2: the single-pass design

`DecodeInto` gains a discriminator pointer and decodes in one pass:

```go
// DecodeInto is the shared decode body for a concrete type's UnmarshalJSONFrom.
// It decodes the next value straight into out, then refuses a _type that names a
// different concrete type: gotType points at the wrapper's declared _type field,
// populated by the same single decode, so the discriminator is read once. A
// whole-value shape failure goes through WrapShapeError (canjson: <RM_TYPE>:,
// ErrInvalidShape); a syntactic or IO error passes through unwrapped, because it
// is malformed input, not a shape failure of this type (REQ-052, ADR 0022).
func DecodeInto(dec *jsontext.Decoder, rmType string, out any, gotType *string) error {
	if err := json.UnmarshalDecode(dec, out, decodeOptions(dec)); err != nil {
		return classifyDecode(rmType, err)
	}
	if *gotType != "" && *gotType != rmType {
		return &DecodeError{
			Path:  "/_type",
			Inner: fmt.Errorf("canjson: expected %q, got %q: %w", rmType, *gotType, ErrTypeMismatch),
		}
	}
	return nil
}
```

Both generated shapes can pass the pointer. The alias shape needs a named local so `&w.Type` is addressable (today it is an inline literal); the flat shape already has a named `wire`:

- Alias (e.g. `DVQuantity`, `data_types_quantity_jsonunmar_gen.go:90`): `w := struct{ Type string; *rawDVQuantity }{...}; return typereg.DecodeInto(dec, "DV_QUANTITY", &w, &w.Type)`.
- Flat (e.g. `DVEHRURI`, `data_types_uri_jsonunmar_gen.go:22`, wire struct `data_types_uri_jsonmar_gen.go:19` with `Class string ` + "`json:\"_type\"`" + ` at `:20`): `var wire jsonWireDVEHRURI; if err := typereg.DecodeInto(dec, "DV_EHR_URI", &wire, &wire.Class); err != nil {...}`. The copy-back loop copies only the `effectiveFields`, never `Class`, so the discriminator is read for the guard and discarded.

Confirmed in the scratch module against real and modelled generated shapes:

| Claim | Evidence (`peek/peek_test.go`) |
|---|---|
| The discriminator field is populated regardless of member order, and unknown members are ignored by default, so a well-shaped wrong-`_type` body decodes then mismatches | `TestDiscriminatorOrderAndUnknown` PASS |
| `json.UnmarshalDecode` on a `*jsontext.Decoder` consumes exactly one value and leaves the decoder positioned for the next member (a parent object continues to its next field after a child's single-pass decode) | `TestNestedPositioningAndPointer` PASS (parent reads `units` after the child); `go doc encoding/json/v2.UnmarshalerFrom`: "must read only one JSON value"; `go doc encoding/json/v2.UnmarshalDecode`: "unmarshals only the next JSON value in the stream" |
| Malformed input surfaces as `*jsontext.SyntacticError` (wrapping `io.ErrUnexpectedEOF` for a truncated value), not a `SemanticError` | `TestMalformedIsSyntactic` PASS |
| A shape failure surfaces as `*json.SemanticError` with a `JSONPointer` | `TestShapeFailureIsSemantic` PASS (`/magnitude`) |
| `RejectUnknownMembers` is unchanged: `_type` is a declared member and is accepted, a genuinely unknown member is refused | `TestRejectUnknownMembers` PASS |
| A failure inside a value decoded in place roots its `JSONPointer` at the enclosing decode, not the slot | `TestNestedPositioningAndPointer` PASS (`/child`) |

`DecodeError.Path` for a failure inside the single decode: `go doc encoding/json/v2.SemanticError` states the `JSONPointer` is "automatically populated by the calling context if they are the zero value," and `go doc encoding/json/v2.UnmarshalerFrom` states that a returned error is wrapped in a `SemanticError` "except for `jsontext.SyntacticError`s and IO errors." So a shape or dispatch failure returned from the single decode carries a `JSONPointer` rooted at the decoder in force. For a value decoded **in place** (the concrete path), that root is the enclosing object, which yields fuller paths than the old buffer-and-peek (which re-rooted at each `ReadValue`). The polymorphic hook keeps `ReadValue`, so a failure reached **through a hook** still re-roots at the slot, exactly the caveat ADR 0022 already records in its Consequences ("a nested decode inside a hook restarts the pointer at that slot's own root"). No test pins a deep concrete-nested path, so the fuller paths are invisible to the suite (item 3).

### Item 3: error-precedence change and the spec

The only observable behaviour change is the precedence flip in the last-but-one row of item 1: when a body carries **both** a wrong whole-value `_type` **and** a shape failure, the shape failure now wins (`ErrInvalidShape`) where the `_type` mismatch won before (`ErrTypeMismatch` on `/_type`). No normative sentence and no test pins that precedence, so nothing turns red on it; it is recorded in ADR 0022's Consequences. The clause and prose census:

| Clause / prose / test | `file:line` | Verdict |
|---|---|---|
| "The decoder MUST accept members in any order, `_type` included." | `wire.md:105` | **unaffected**. The declared field is populated wherever `_type` appears; confirmed `TestDiscriminatorOrderAndUnknown` |
| Decode-side shape sentinel MUST (a failure inside `UnmarshalJSONFrom` wraps `ErrInvalidShape`) | `wire.md:123` | **unaffected**. `WrapShapeError` still runs on a semantic failure |
| Duplicate member name MUST be refused wrapping `ErrInvalidShape` | `wire.md:125` | **unaffected**, but now **depends on** the syntactic passthrough (item 5): the refusal is a `*jsontext.SyntacticError` that must reach `ClassifyDuplicate` unwrapped |
| Malformed JSON stays outside the sentinel, "which the codec reports **before it dispatches to any `UnmarshalJSONFrom` method**" | `wire.md:126` | **amend**. The MUST-NOT (no sentinel) holds; the descriptive "before it dispatches" clause becomes inaccurate, since a nested malformed value is now refused inside that value's own `UnmarshalJSONFrom` decode. Proposed wording below |
| `TERM_MAPPING.match`: invalid UTF-8 "refused by the tokenizer, which the codec reports before it dispatches to any `UnmarshalJSONFrom` method" | `wire.md:114` | **amend**, same reason and same minimal reword |
| Polymorphic dispatch failure stays outside the sentinel, `DecodeError` carries the path | `wire.md:127` | **unaffected**; the polymorphic hook and `classifyDecode` slot-pointer path are unchanged |
| Package doc: "Malformed JSON reaches the caller before any generated decode method runs, **because the codec validates the whole input first**." | `openehr/serialize/canjson/doc.go:115-116` | **amend** (godoc). The codec no longer buffers each value before decoding it; the reason clause is now false |
| `ErrInvalidShape` doc: malformed / duplicate "before any generated decode runs" | `decode.go:30,38,93` | **amend** (godoc), mechanism-neutral reword; outcomes unchanged |
| `DecodeInto` doc: "It reads the next value, refuses a `_type` ..., then decodes the raw bytes into out" | `streaming.go:173-185` | **amend** (godoc), rewrite for single-pass and the added pointer |
| Generator doc: "hands the decoder to the shared `DecodeInto` helper, which reads the value, enforces the `_type` discipline" | `internal/bmmgen/render_jsonunmar.go:20` | **amend** (godoc), rewrite for single-pass |
| `DecodePolymorphic` doc: "reads the next value, resolves the concrete type from its `_type`" | `streaming.go:236` | **unaffected**; the polymorphic path keeps buffer-and-peek |
| PROBE-030 round trip; PROBE-031 unknown `_type`; PROBE-038 polymorphic decode | `conformance.md` | **unaffected**; exercised and green |

Proposed amendment (no em dashes), `wire.md:126`, replacing the "before it dispatches to any `UnmarshalJSONFrom` method" clause:

> **Malformed JSON**, which the codec reports as an underlying syntax or truncated-input error reachable through unwrapping, carrying no SDK sentinel. The tokenizer refuses it as the value it malforms is decoded, not through a separate whole-input validation pass.

Proposed amendment, `wire.md:114`, the UTF-8 sentence, replacing "which the codec reports before it dispatches to any `UnmarshalJSONFrom` method":

> Invalid UTF-8 and a lone surrogate escape are refused by the tokenizer as a syntactic error (the **Malformed JSON** exclusion below), so they carry no SDK sentinel, exactly as that exclusion states.

Proposed ADR 0022 Consequences line (appended to the existing "diagnostic path" consequence, which already records the hook re-root caveat):

> The concrete decode reads `_type` from the declared wire field after one `json.UnmarshalDecode`, rather than buffering and peeking. Two edges follow. A body that is both a wrong whole-value `_type` and a shape failure now reports the shape failure (`ErrInvalidShape`), where the buffer-and-peek reported the `_type` mismatch (`ErrTypeMismatch` on `/_type`) first, because the discriminator is checked only after a successful decode. And a shape failure inside a value reached without a polymorphic hook now carries a `JSONPointer` rooted at the enclosing object rather than at that value, a fuller path. Neither is pinned by a normative sentence or a test.

### Item 4: the polymorphic and registry paths, and the benchmark

`DecodePolymorphic` (`streaming.go:247`) and `Registry.Decode` (`registry.go:140`) both need `_type` before they can pick a constructor, so they keep the buffer-and-peek (`ReadValue` + `peekType`). Their migration benchmarks are within the R26 threshold already (`BenchmarkRegistryDecodeDVQuantity` +18.80 percent, `BenchmarkRegistryDecodeElement` +13.84 percent; `docs/plans/archive/2026-09-14-json-v2-migration.md:637-638`), so a token-scanning peek for them buys nothing this plan needs and adds real complexity. The concrete path carries the whole regression.

The scratch benchmark decodes the exact `BenchmarkDecodeDVQuantity` body (`{"_type":"DV_QUANTITY","magnitude":80.5,"units":"kg"}`) into the real `rm.DVQuantity`, both arms entering through the pooled `json.Unmarshal([]byte, ...)`. The current arm drives the shipped three-pass `DecodeInto`; the single-pass arm decodes the alias wrapper in one pass and reads `w.Type`. `benchstat`, `-count=12`, on a 13th Gen i7-13700H, `go1.27.1`, WSL2:

```
                    │   cur.txt    │               sp.txt                │
                    │    sec/op    │   sec/op     vs base                │
DecodeDVQuantity-12   1209.0n ± 2%   571.2n ± 1%  -52.75% (p=0.000 n=12)

B/op:      272.0 → 144.0   -47.06% (p=0.000 n=12)
allocs/op:   5.000 → 3.000  -40.00% (p=0.000 n=12)
```

The current arm (1209 ns/op, 272 B, 5 allocs) reproduces the shipped `BenchmarkDecodeDVQuantity` at this head (measured 1177-1217 ns/op, 272 B, 5 allocs), which validates the harness. Against the migration's recorded v1 baseline (`90473f9f`: 863.4 ns/op, 3 allocs) the single-pass arm is faster **and** back to 3 allocations. The +37.88 percent regression is not merely recovered; the single-pass concrete decode lands about 34 percent under the v1 baseline, well inside R26's 20 percent bar.

The composition cassette cannot regress. The single-pass change only removes work from every concrete `DecodeInto` in the tree (each concrete leaf saves the two allocations and roughly halves its per-node decode cost, above), and adds nothing to the polymorphic path. `BenchmarkDecodeCompositionCassette` at this head is 3.18-3.25 ms, 5222 allocs (already -38 percent versus v1); `BenchmarkDecodeComposition_400` is 6.5-6.7 ms, 16 860 allocs. Both are dominated by concrete decodes, so both move down or hold. A full-tree measurement needs the regenerated tree, so the acceptance gate below runs `benchstat` on the real benchmarks during implementation.

**Recommended scope: the concrete path only.** The DV_QUANTITY regression lives entirely on `DecodeInto`, and the single-pass concrete decode recovers it with margin while `DecodePolymorphic` and `Registry.Decode` keep their peek. Widening to the polymorphic path is a separate, more intricate change (a token-scanning peek) that no current benchmark justifies.

### Item 5: guards

| Guard | Turns red / can-fail control | `file:line` |
|---|---|---|
| Malformed JSON keeps no shape sentinel | Deleting the syntactic passthrough turns the truncated `{` case red: it acquires `ErrInvalidShape` against `wantShapeSentinel:false`. This is the **guaranteed red that forces the passthrough** | `decode_test.go:185` in `TestUnmarshalWrapsErrInvalidShape` (`:249`) and its `Decode` twin `TestDecoderDecodeWrapsErrInvalidShape` (`:271`) |
| A duplicate member name is refused, wraps `ErrInvalidShape`, keeps no `canjson: <RM_TYPE>:` prefix | Existing test asserts both sentinels; **add** a discriminating assertion that the message carries no `canjson:` prefix, since the current test does not check the message and would pass even if the passthrough were missing (the double-wrap only changes the string) | `decode_test.go:596` (Unmarshal), `:613` (Decode); new prefix assertion |
| Wrong whole-value `_type` still refused with `ErrTypeMismatch` on `/_type`, not shape-tagged | Well-shaped `DV_TEXT` into `DVQuantity`; unaffected by the flip (no shape failure), stays green as the operation-specific control | `TestUnmarshalWholeValueTypeMismatchIsNotShapeTagged` `decode_test.go:455` |
| A correct-`_type` shape failure still wraps `ErrInvalidShape` with the cause reachable | `magnitude` is a string; green (decode fails, `WrapShapeError` runs) | `decode_test.go:194` |
| A nested slot dispatch failure keeps its `DecodeError` and stays outside the shape sentinel | green; the polymorphic hook is unchanged | `TestUnmarshalNestedDecodeErrorIsNotShapeTagged` `decode_test.go:314` |
| A slot-nested shape failure carries both the path and `ErrInvalidShape` | green; `Path` still `/value` because the top-level value decodes at root | `decode_test.go:347` |
| The narrow-slot missing-`_type` fallback keeps both classifications | green; `Path` still `/name` | `decode_test.go:415` |
| `DecodeError.Path` names the failing slot | green; `Contains("content")` and the exact `/value`, `/name`, `/_type` all hold at the top level | `decode_test.go:120,368,419,474` |
| `Unmarshal` and `Decoder.Decode` still diverge on empty and trailing input | green; entry points unchanged | `TestDecoderDecodeStreamDivergesFromUnmarshal` `decode_test.go:490` |
| The two-shape census still holds after regeneration | `TestRegisteredTypeCensus` marshals and decodes back every registered type; green | `openehr/serialize/canjson/type_census_test.go` |
| A bare v2 caller still resolves a polymorphic slot | `DecodeInto` still threads `decodeOptions(dec)` into `json.UnmarshalDecode`; green | `openehr/serialize/canjson/bare_v2_decode_test.go` |
| `decodeOptions` memoisation unchanged | `decodeOptions` is untouched; the three memo tests stay green | `openehr/rm/typereg/streaming_test.go` |
| The generator still renders `typereg.DecodeInto(dec, "DV_INTERVAL"` and no pre-streaming construct | The new call `typereg.DecodeInto(dec, "DV_INTERVAL", &w, &w.Type)` still satisfies the `Contains` prefix; green | `internal/bmmgen/render_jsonunmar_polymorphic_test.go:83` |
| Benchmark acceptance | `BenchmarkDecodeDVQuantity` within 20 percent of the v1 baseline at `90473f9f` (p < 0.05), no large-payload benchmark (`BenchmarkDecodeCompositionCassette`, `BenchmarkDecodeComposition_400`, `BenchmarkEncodeComposition_400`) regressing, measured with `benchstat -count=10` | `openehr/serialize/canjson/bench_test.go:101,132,70` |

New can-fail controls the plan adds:

1. A syntactic-passthrough control on `Registry.Decode`-free ground: assert that a truncated concrete body decoded through `canjson.Unmarshal` returns a `*jsontext.SyntacticError` and `errors.Is(err, canjson.ErrInvalidShape)` is false (the existing `{` case already does this; keep it and name the mutation in the commit body).
2. A duplicate-name **message** control: assert `!strings.Contains(err.Error(), "canjson: DV_QUANTITY:")` for the duplicate-`units` body, so the passthrough is proven to keep the prefix off (mutation: route the syntactic error through `WrapShapeError`; the assertion goes red).

## Global constraints

Binding constraints (maintainer decisions, the amended REQ-052, the ADR 0022 design, the process rules), and its "Follow-ups after PR 171" section (worktree, the maintainer decision for B, the gates). The specification wins over this plan; the maintainer's decisions win over both. No reflection (AGENTS.md). Prose is plain English, no em dashes, no second person, no bare `#N` ordinals. Commits are Conventional with an `Assisted-by: Claude Code (claude-opus-4-8[1m])` trailer, staged with explicit pathspecs, no `git add -A`, no branch switching, no other worktree.

## Rulings where the analysis had to choose

| Point | Options | Ruling and reason |
|---|---|---|
| Scope | concrete path only; or concrete plus a token-scanning peek for the polymorphic and registry paths | Concrete path only. The regression is entirely on `DecodeInto`, the single-pass arm recovers it with margin, and the registry/polymorphic benchmarks are within R26 today. A token-scanning peek is a later plan if those ever cross the line |
| Malformed classification | let `classifyDecode` `WrapShapeError` everything; or pass syntactic and IO errors through unwrapped | Pass them through. `go doc encoding/json/v2.UnmarshalerFrom` says the json package does not wrap a `SyntacticError` or IO error returned from the method, and `wire.md:126` requires malformed input to keep no sentinel. The `{` case at `decode_test.go:185` makes this a guaranteed red, not a judgment call |
| Alias-wrapper form | keep the inline anonymous literal; or a named local | Named local. `&w.Type` must be addressable to pass the discriminator pointer; the flat shape already has a named `wire` |
| Precedence flip | leave it silent; or record it | Record it in ADR 0022 Consequences and amend the one inaccurate descriptive clause in REQ-052. No normative sentence pins the precedence, so no re-pin is owed, but the mechanism prose must stay true |
| **F8** (controller). Pin the precedence flip | leave it as documented behaviour only; or add a positive test | Both. Documented in ADR 0022 Consequences, and pinned by `TestUnmarshalConcreteTypeMismatchPrecedence` (`decode_test.go:496`): a body both mislabelled and malformed for the target reports `canjson.ErrInvalidShape`; a mislabelled but well-formed body reports `typereg.ErrTypeMismatch` on `/_type`; a body with no `_type` decodes as the concrete target. Cost if wrong: one test to change if the maintainer later prefers the old precedence |
| **F9** (controller). Scope, reaffirmed | concrete path only; or also widen `DecodePolymorphic` / `Registry.Decode` to a token-scanning peek | Concrete path only, confirming the Scope ruling above. `DecodePolymorphic` and `Registry.Decode` keep their buffer-and-peek in this plan; a token-scanning peek for them is a later plan, only if their benchmarks cross the R26 line |
| **F12** (controller, fix round 1). `RegistryDecodeDVQuantity` crossed R26 as a side effect | leave `Registry.Decode` unchanged (accept the +33.71 percent regression as a named follow-up); or make it decode from the pooled byte slice while keeping its peek | `Registry.Decode` keeps its `peekType` and depth guard but decodes the value from the byte slice through the pooled `json.Unmarshal` entry, using the same `hookOptions` helper `decodeOptions` builds from, because single-pass `DecodeInto` had driven the old io.Reader-backed decoder's unpooled read buffer onto the registry path. Acceptance met per the fix-round benchstat: `RegistryDecodeDVQuantity` -30.04 percent and `RegistryDecodeElement` -30.80 percent against the v1 baseline |

### Facts recorded in the fix round

- **The positive pass-through rule.** Round 0's `classifyDecode` exempted only a `*jsontext.SyntacticError` and the `io.EOF` family; a failing reader on the streaming entry surfaces as jsontext's own unexported IO error type, neither of those, so it fell through to `WrapShapeError` and wrongly gained `ErrInvalidShape`. `classifyDecode` now passes an error through unwrapped unless its chain carries a `*json.SemanticError` or a `*DecodeError`, the only two shapes `encoding/json/v2` leaves unwrapped from a decode (`go doc encoding/json/v2.UnmarshalerFrom`), so every other malformed-input shape, a syntactic error, a duplicate name, or a failing reader's IO error, keeps no sentinel. Pinned by `TestDecoderDecodeFailingReaderIsNotShapeTagged` (`decode_test.go:816`). Dated note (2026-09-24): the duplicate name no longer falls in that list; `classifyDecode` attaches `ErrInvalidShape` to a duplicate-name refusal (`main`'s `2b1be45b`, kept on the single pass; see As built below).
- **The mutated-receiver consequence**, recorded in ADR 0022 Consequences: after the discriminator guard refuses a whole-value `_type` mismatch, the receiver already holds the foreign body the decode wrote, for the alias shape only (its decode writes into the receiver directly through the type-converted embed). The flat wire-struct shape decodes into a local `wire` value and returns before its field-by-field copy back, so its receiver stays untouched on that failure.
- **The "during tokenisation" doc sweep.** Six stale "before any generated decode runs" / "validates the whole document before any decode runs" sites were reworded to the mechanism-neutral "during tokenisation": `doc.go:125`, `errors.go:184`, `character_test.go:393,495`, `decode_test.go:152-153,245`. Three remaining hits were checked and left in place, each for a stated reason: `marshal_sentinel_test.go:82` decodes into `map[string]any`, which invokes no generated `UnmarshalJSON`, so the old wording is vacuously true there; this plan's own Findings census (item 3) quotes the old wording as the "before" state it directs the implementer to amend, a historical planning record; and `docs/plans/archive/2026-09-01-ehr-create-empty-2xx-typing.md:16` describes unrelated transport-level pre-decode classification (REQ-094).

## Definition of Ready

- The REQ-052 clause amendments and the ADR 0022 Consequences line are drafted (item 3), and the maintainer accepts the amendment wording (the one open question).
- The scratch benchmark headline is in hand (item 4).

## Definition of Done

- The runtime helper, the generator (both branches) and the regenerated tree land with `// REQ-052` / `// ADR 0022` citations where the repository convention has them.
- REQ-052 stays `landed`; the amended clauses merge with the code (`scripts/spec-check.sh` enforces agreement). `traceability.yaml` and the REQ.md **Impl.** column do not move (no id allocated, no package/probe row changes).
- ADR 0022 gains the Consequences line; the three godoc sites and the two spec sentences are corrected in the same PR.
- `BenchmarkDecodeDVQuantity` is within 20 percent of the v1 baseline at `90473f9f` (p < 0.05) with no large-payload benchmark regressing, shown by a `benchstat -count=10` run recorded in the PR body.
- `make ci` (which runs `codegen-verify` first) and `make spec-check` pass. Test output is pristine.
- Plan archived under `docs/plans/archive/` per `sdd-archive`, in the implementing PR.

## Tasks

Each task ends green on its named gate. `make test` runs `codegen-verify`, so the generator change and `make codegen` land together.

### Task 1: the runtime helper (red first)

- [x] Add the two new can-fail controls in `openehr/rm/typereg` (or extend the canjson decode tests). The duplicate-name **message** control (`!strings.Contains(err.Error(), "canjson: DV_QUANTITY:")`) is new; the truncated-`{` no-sentinel control already exists at `decode_test.go:185`. Run them against the current three-pass helper: the message control passes today (no regression yet), the suite is green.
- [x] Change `DecodeInto` (`openehr/rm/typereg/streaming.go:186`) to the single-pass body above: add the `gotType *string` parameter, replace `ReadValue`/`peekType`/`json.Unmarshal(raw,...)` with one `json.UnmarshalDecode(dec, out, decodeOptions(dec))`, then the `_type` guard on `*gotType`.
- [x] Add the syntactic and IO passthrough to `classifyDecode` (`streaming.go:215`), before the `*DecodeError` arm:

```go
func classifyDecode(rmType string, err error) error {
	// A syntactic or IO error is malformed input, not a shape failure of this
	// type: leave it unwrapped so it keeps no SDK sentinel and no
	// canjson: <RM_TYPE>: prefix (REQ-052). The json package already declines to
	// wrap a SyntacticError or IO error returned from UnmarshalJSONFrom
	// (go doc encoding/json/v2.UnmarshalerFrom); passing it through preserves the
	// contract dec.ReadValue() used to give for free.
	if _, ok := errors.AsType[*jsontext.SyntacticError](err); ok {
		return err
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return err
	}
	if de, ok := errors.AsType[*DecodeError](err); ok && de != nil && de.Path == "" {
		if p := slotPointer(err); p != "" {
			return WrapShapeError(rmType, &DecodeError{Path: p, Type: de.Type, Inner: de.Inner})
		}
	}
	return WrapShapeError(rmType, err)
}
```

- [x] Add `"io"` to the `streaming.go` imports (`jsontext`, `errors`, `json` are already there). `peekType` and `typeDiscriminator` stay: `DecodePolymorphic` and `Registry.Decode` still use them.
- [x] Rewrite the `DecodeInto` godoc (`streaming.go:173-185`) for single-pass and the new pointer.
- [x] The typereg package will not build until the generated callers pass the fourth argument, so this task compiles together with Task 2's regeneration. Sequence the commit accordingly (helper + generator + `make codegen` in one green step), or land the helper with a temporary shim only if the tree must stay buildable between commits; prefer the single green step.
- **Gate:** `go test ./openehr/rm/typereg/ ./openehr/serialize/canjson/`, then the mutation checks by hand-reverting each guard, then `golangci-lint run` on the two packages.

### Task 2: the generator (both branches) and regeneration

- [x] In `renderUnmarshalJSON` (`internal/bmmgen/render_jsonunmar.go`), flat branch (`:105-114`): pass the discriminator pointer.

```go
fmt.Fprintf(&b, "\tif err := typereg.DecodeInto(dec, %q, &wire, &wire.Class); err != nil {\n", pc.BMMName)
```

- [x] Alias branch (`:116-121`): emit a named local so `&w.Type` is addressable.

```go
alias := aliasTypeName(pc.GoName)
b.WriteString("\tw := struct {\n")
b.WriteString("\t\tType string `json:\"_type\"`\n")
fmt.Fprintf(&b, "\t\t*%s%s\n", alias, typeArgs)
fmt.Fprintf(&b, "\t}{%s: (*%s%s)(%s)}\n", alias, alias, typeArgs, recv)
fmt.Fprintf(&b, "\treturn typereg.DecodeInto(dec, %q, &w, &w.Type)\n", pc.BMMName)
```

The rendered alias method then reads:

```go
func (d *DVQuantity) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	if d == nil {
		return fmt.Errorf("canjson: DV_QUANTITY: %w", typereg.ErrNilReceiver)
	}
	w := struct {
		Type string `json:"_type"`
		*rawDVQuantity
	}{rawDVQuantity: (*rawDVQuantity)(d)}
	return typereg.DecodeInto(dec, "DV_QUANTITY", &w, &w.Type)
}
```

and the flat method:

```go
func (d *DVEHRURI) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	if d == nil {
		return fmt.Errorf("canjson: DV_EHR_URI: %w", typereg.ErrNilReceiver)
	}
	var wire jsonWireDVEHRURI
	if err := typereg.DecodeInto(dec, "DV_EHR_URI", &wire, &wire.Class); err != nil {
		return err
	}
	d.Value = wire.Value
	return nil
}
```

- [x] Rewrite the generator doc (`render_jsonunmar.go:20`) for single-pass and the discriminator pointer.
- [x] `make codegen`. This regenerates the 110 `openehr/rm/*_jsonunmar_gen.go` and 29 `openehr/aom/aom14/*_jsonunmar_gen.go` methods. The base-struct goldens (`internal/bmmgen/testdata/data_types_quantity_gen.go.golden`, `aom14_archetype_gen.go.golden`) carry no `UnmarshalJSONFrom`, so no golden is re-copied. `TestPolymorphicPropertyRendersTyperegDispatch` (`render_jsonunmar_polymorphic_test.go:83`) still matches the `typereg.DecodeInto(dec, "DV_INTERVAL"` prefix.
- **Gate:** `make codegen-verify` (clean), then `go test ./internal/bmmgen/ ./openehr/rm/... ./openehr/aom/...`.

### Task 3: the prose and the spec

- [x] Amend `wire.md:126` and `wire.md:114` with the two rewordings in item 3. REQ-052 stays `landed`.
- [x] Add the ADR 0022 Consequences line (item 3).
- [x] Correct the godoc at `doc.go:115-116`, `decode.go:30,38,93`. Keep them mechanism-neutral.
- **Gate:** `make spec-check`, `make docs-check`.

### Task 4: verify and measure

- [x] `go test ./...` for the touched trees; confirm the full decode-test suite, the census, the bare-v2 test and PROBE-030/031/038 are green with pristine output.
- [x] `benchstat -count=10` for `BenchmarkDecodeDVQuantity`, `BenchmarkDecodeCompositionCassette`, `BenchmarkDecodeComposition_400`, `BenchmarkEncodeComposition_400`, `BenchmarkRegistryDecodeDVQuantity`, `BenchmarkRegistryDecodeElement` against `90473f9f`. Record the table in the PR body. Acceptance: DVQuantity within 20 percent of the v1 baseline (p < 0.05), no large-payload benchmark regressing.
- [x] `make ci`.

#### Benchmark results (`benchstat -count=10`, `go1.27.1`, linux/amd64)

The DecodeDVQuantity and large-payload rows are the first measurement (round 0, head `69b8a627`); the two registry rows are the fix-round-1 measurement (head after F12, `18a6416f`), because F12 changed `Registry.Decode` itself. Source: `task-B-report.md`, first section and `## Fix round 1` section.

| Benchmark | v1 `90473f9f` | v2 | sec/op delta | R26 verdict |
|---|---|---|---|---|
| `BenchmarkDecodeDVQuantity` | 898.0n / 248 B / 3 allocs | 716.7n / 256 B / 4 allocs | -20.19% (p=0.000, n=10) | NO REGRESSION (the hard gate, within the 20 percent threshold) |
| `BenchmarkDecodeCompositionCassette` | 5.320m / 3762.8Ki / 6267 allocs | 2.072m / 446.1Ki / 4809 allocs | -61.05% (p=0.000, n=10) | NO REGRESSION (large-payload) |
| `BenchmarkDecodeComposition_400` | 6.112m / 2703.0Ki / 21.67k allocs | 3.690m / 960.0Ki / 13.25k allocs | -39.62% (p=0.000, n=10) | NO REGRESSION (large-payload) |
| `BenchmarkEncodeComposition_400` | 2.905m / 3.083Mi / 9280 allocs | 1.513m / 1.323Mi / 16.44k allocs | -47.91% (p=0.000, n=10) | NO REGRESSION (large-payload) |
| `BenchmarkRegistryDecodeDVQuantity` | 1.656u / 753 B / 9 allocs | 1.159u / 272 B / 5 allocs | -30.04% (p=0.000, n=10) | NO REGRESSION (was +33.71 percent before F12; resolved) |
| `BenchmarkRegistryDecodeElement` | 5.517u / 2755 B / 29 allocs | 3.818u / 1009 B / 15 allocs | -30.80% (p=0.000, n=10) | NO REGRESSION (was -1.65 percent before F12, unaffected either way) |

**Hard acceptance gate MET.** `BenchmarkDecodeDVQuantity` is 20.19 percent faster than the v1 baseline, and no large-payload benchmark regresses. The registry benchmarks, not part of the hard gate, both land within R26 too once F12 lands.

### Task 5: close-out

- [x] Archive this plan under `docs/plans/archive/` per `sdd-archive`, in the implementing PR.

Commit sequence (Conventional, `Assisted-by: Claude Code (claude-opus-4-8[1m])`):

1. `perf(typereg): decode a concrete value in one pass, read _type from the wire field` (helper + generator + `make codegen` + the guards, one green step).
2. `docs(wire): the malformed-JSON exclusion no longer says "before dispatch"` (spec + ADR + godoc).

## Question for the controller, ruled

**Ruling F8 (controller, 2026-09-15):** the precedence flip is accepted as documented behaviour and also pinned by a positive test: a body that is both mislabelled (`_type` names another class) and malformed for the target reports `canjson.ErrInvalidShape`; a mislabelled but well-formed body still reports `typereg.ErrTypeMismatch` on `/_type`; a body with no `_type` behaves as REQ-052 states for a concrete target. The `classifyDecode` pass-through for syntactic and IO errors is part of the same task. Cost if wrong: one test to change if the maintainer later prefers the old precedence. **Ruling F9:** `DecodePolymorphic` and `Registry.Decode` keep their buffer-and-peek in this plan; a token-scanning peek is a later plan only if their benchmarks cross the R26 line.

The question as it stood at planning time: the precedence flip (a body that is both a wrong whole-value `_type` and a shape failure now reports `ErrInvalidShape`, not `ErrTypeMismatch` on `/_type`) was then pinned by no normative sentence or test and recorded only in ADR 0022 Consequences plus the two REQ-052 wording fixes in item 3, so the plan asked whether to accept that wording or to pin the flip with a positive test. Ruling F8 above answered it: the flip is both documented and pinned, by `TestUnmarshalConcreteTypeMismatchPrecedence`.

## Follow-ups

- A token-scanning peek for `DecodePolymorphic` (and `Registry.Decode`, though F12 already moved it off the io.Reader-backed decoder), only if either one's benchmarks ever cross the R26 line. Neither does today.
- A test pinning `DecodeError.Path` for a concrete value nested beneath another concrete value with no polymorphic hook between them, so the fuller `JSONPointer` the single-pass change produces there (item 2; the ADR 0022 Consequences second edge) is exercised by the suite rather than only observed in the scratch module.
- A parity test pinning that `decodeOptions` and `Registry.Decode` build the same option set: both now call the shared `hookOptions` helper (`streaming.go:65`), so a drift between the two call sites would currently pass unnoticed until a decode outcome actually differed.
- The `_`-discarding `errors.AsType` guards at `errors.go:156` (`WrapShapeError`) and `errors.go:177` (`ClassifyShape`): both check only `ok`, not `ok && v != nil`, unlike the guarded pattern this package uses elsewhere (REQ-025). Pre-existing and unreachable today (`errors.AsType[*DecodeError]` finding a match but a nil pointer would require a caller to box a typed-nil `*DecodeError`, which no producer in this package does), so no test can currently turn either one red; recorded so a future producer of a possibly-nil `*DecodeError` does not reintroduce the axis silently.

## As built (2026-09-24)

This branch was rebased onto `main` after PR #173 merged. Meanwhile `main` had hardened the shared decode body (`2b1be45b`): a REQ-040 depth bound and a REQ-052 duplicate-name classification in `DecodeInto`, both at the buffered `ReadValue` this plan removes. The single-pass body keeps both:

- **Depth.** `DecodeInto` checks `dec.StackDepth()` on entry, so each nested RM value is measured where it opens and an over-deep chain is refused at the first value past the cap. This drops the per-level `jsonNestingDepth` re-scan that `main`'s comment named as a redundancy for this follow-up to fold away. `classifyDecode` passes the nested refusal through unchanged. Re-wrapped at every enclosing level, the error text for a 300-FOLDER chain grew to 46 KB; passed through, it stays one path long. `TestUnmarshalMaxDepthExceeded` pins the refusal, the absence of `ErrInvalidShape` and the bounded text.
- **Duplicates.** `classifyDecode` routes a `jsontext.ErrDuplicateName` through `ClassifyDuplicate` before the syntactic pass-through, so a caller driving a generated method through bare `encoding/json/v2` still gets `ErrInvalidShape` (`TestBareV2DecodeDuplicateMemberWrapsErrInvalidShape`).

`BenchmarkDecodeDVQuantity` is unchanged by the two guards (about 729 ns/op, 256 B/op, 4 allocs/op, before and after).

## Post-review amendments (2026-09-24)

Two code reviews of PR 174 led to these fixes, landed in the same PR. The `file:line` citations in the body of this plan predate the rebase onto `main` and may no longer match the tree.

- **First failure in document order (rulings P1, P12).** The first failure in document order wins for a value decoded outside a buffered value; inside `Registry.Decode`'s document or a polymorphic slot, and for a v1 caller, malformed bytes win. The rule's home is [REQ-052](../../specifications/wire.md#req-052) § Decode-side shape sentinel, malformed-JSON exclusion. Pinned by `TestUnmarshalFirstFailureInDocumentOrderWins`, which includes the slot case.
- **v1 decode options (ruling P2).** A bare v1 `encoding/json` caller inherits v1's decode options through the decoder, so a case-variant `_TYPE` member now reports `ErrTypeMismatch` where the buffer-and-peek accepted it (ADR 0022 Consequences, dated line). The SDK's two v1 call sites, `openehr/validation/rmfloor_bytes.go` and `openehr/client/demographic/versioned.go`, moved to `encoding/json/v2`.
- **Depth across polymorphic slots (rulings P3, P9).** `DecodePolymorphic` now measures a buffered slot as the enclosing decoder depth plus the slot's own nesting, where the slot had reset the count. The refusal is `ErrMaxDepthExceeded` inside a `typereg.DecodeError` on the `canjson` and generated-method routes, and a plain wrapped error from `Registry.Decode` (ruling P11). Undeclared nesting is counted only inside a buffered value. The bound's home is [REQ-108](../../specifications/clinical-modeling.md#req-108--untrusted-document-bounds). Pinned by `TestUnmarshalMaxDepthAcrossPolymorphicSlots`, `TestUnmarshalMaxDepthBoundary` and `TestUnmarshalUndeclaredNestingDependsOnBuffering`.
- **Receiver after a failed decode.** The behaviour ADR 0022 describes for the two shapes after a `_type` mismatch is pinned by `TestUnmarshalTypeMismatchLeavesReceiverAsDocumented`.
- **Mismatch census and generator pin.** `TestUnmarshalTypeMismatchCensus` and `TestRenderUnmarshalPassesDeclaredTypeField` cover the registered types and the generated decode's use of the declared `_type` field.
- **Nil `gotType`.** `DecodeInto` refuses a nil `gotType` with an error (REQ-025), pinned by `TestDecodeIntoNilArguments` (renamed from `TestDecodeIntoNilGotType` in the second round).
- **Slot peek options (2026-09-24, ruling P17).** `DecodePolymorphic` peeks a slot's `_type` under the caller's decoder options, so a bare v1 caller's options govern the whole decode, slots included; `Registry.Decode` keeps v2 defaults. Pinned by `TestBareV1CallerOptionsGovernPolymorphicSlots`.
- **Exact depth counting (2026-09-24, ruling P19).** `DecodeInto` checks the decoder's stack depth only where an RM value opens; `Registry.Decode` and a polymorphic slot count every bracket of the buffered value, stated descriptively in REQ-108. The depth and order tests gain `Registry.Decode` and bare-v1 arms and a declared-array case.
- **v1 decode guard (2026-09-24, ruling P20).** `TestNoV1DecodeIntoRMValues` (`internal/v1_rm_decode_guard_test.go`) scans non-test, non-generated Go files for a v1 `encoding/json` decode into an RM value, the guard REQ-052 names.
- **Nil-argument sentinel (2026-09-24, ruling P20).** `typereg.ErrNilArgument` classifies a nil decoder, target or `gotType` (REQ-025, ADR 0022 point (g)). Pinned by `TestDecodeIntoNilArguments` and `TestDecodePolymorphicNilArguments`.
- **Call-site consequences (2026-09-24, ruling P18).** The v2 move at the two call sites also makes case-variant member names unknown members, refuses a repeated member, and has the validator report invalid UTF-8 inside an unknown member (ADR 0022 point (e)). Pinned by `TestValidateRMEHRStatusBytes_V2DecodeSemantics` and `TestGetVersionCaseVariantMemberIsUnknown`.
- **Wording (ruling P8).** Specification and ADR text written in this round says a body "fails the target's shape" rather than "malformed for the target"; earlier text, this plan included, keeps its original wording.
