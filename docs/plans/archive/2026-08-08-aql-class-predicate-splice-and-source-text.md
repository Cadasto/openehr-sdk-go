# Plan — AQL class-predicate splice guard and predicate source text

**Date:** 2026-08-08
**Status:** Landed
**Owner:** SDK maintainers
**Covers:** **REQ-119** ([clinical-modeling.md § REQ-119](../../specifications/clinical-modeling.md#req-119--re-parseable-canonical-aql-emission)) — closes the § Out of scope deferral recorded for [issue #99](https://github.com/Cadasto/openehr-sdk-go/issues/99); extends [§ REQ-118](../../specifications/clinical-modeling.md#req-118--deprecated-select-top-clause-and-literal-source-text)'s source-text rule from literals to predicates
**Probes:** [PROBE-090](../../specifications/conformance.md#probe-090--aql-emission-round-trip-closure) (extended)
**Implementation:** landed
**Depends on:** REQ-113 (structured AST), REQ-117 (catalogue completion), REQ-118 (`sourceText` helper), REQ-055 (canonical write form) — all landed
**Defers:** a full `nodePredicate` recursive-descent validator (see § Deferred); guarding path positions against splice text, which stays out of scope by REQ-055 rule 3

> **Amended in review, before merge.** Parts 2 and 3 below record what was PLANNED; two rules were tightened while the PR was open and the spec is the current contract. The VERSION position is no longer admitted on the necessary condition this plan describes (§ Part 2, Phase 2) but held to its whole `versionPredicate` production — exactly one top-level comparison operator with an operand on each side, and no top-level junction — because with three non-recursive alternatives its shape is decidable and the closure clause governs. And skipped trivia means every rule the lexer skips, `UNICODE_BOM` included. Both are stated normatively in [clinical-modeling.md § The class predicate positions](../../specifications/clinical-modeling.md#req-119--re-parseable-canonical-aql-emission); this plan is delivery history and is not the place to read the rule.

## Goal

Close the last verbatim-splice position in `(*parse.Query).Emit` — `ClassExpr.Predicate`, the bracket text that is neither an archetype HRID nor a `$param` — and, because the two are causally coupled, repair the extraction defect that makes any guard there unlandable: ANTLR's `GetText()` drops hidden-channel whitespace, so every predicate whose grammar needs a separator (`AND`, `OR`, `MATCHES`) is already emitted as text this SDK's own parser rejects. After this plan a class predicate round-trips as **identity**, a spliced one is refused, and the `VERSION` position is held to its own sub-grammar rather than the class one.

## What is actually broken

Three defects at one field, found by confronting the vendored grammar with the real parser rather than by reading the issue. Only the first is what #99 describes.

### 1. The splice (issue #99)

`emitClassExpr` splices `ClassExpr.Predicate` verbatim, so text that closes the bracket early re-parses as a *different* query with `err == nil`:

```
ClassExpr{RMType: "COMPOSITION", Alias: "c", Predicate: "a/b='c'] CONTAINS OBSERVATION o[d/e='f'"}
  -> SELECT c/x FROM COMPOSITION c[a/b='c'] CONTAINS OBSERVATION o[d/e='f']
```

REQ-119's silent-substitution class: invisible to every round-trip, golden and parser check downstream.

### 2. One field, two sub-grammars

[`emitClassExpr`](../../../openehr/aql/parse/query.go) writes `Predicate` under **two** branches whose grammars differ:

| Branch | Grammar | Admits |
|---|---|---|
| class | `pathPredicate` | `standardPredicate \| archetypePredicate \| nodePredicate` |
| `VERSION` | `versionPredicate` | `LATEST_VERSION \| ALL_VERSIONS \| standardPredicate` |

`versionPredicate` has **no `nodePredicate`**. Two consequences, both live on `main`:

- `VERSION v[at0001]` emits with `err == nil` and the parser rejects it — a closure break at that position.
- The uniform `standardPredicate | nodePredicate` validator #99 specifies would **refuse `VERSION v[LATEST_VERSION]`** — legal AQL the extractor itself produces and round-trips today. That is the tightening failure #99 names as the risk, reached by implementing #99 literally.

### 3. The extractor destroys whitespace-separated predicates

`trimBrackets(pp.GetText())` — ANTLR's `GetText()` concatenates default-channel tokens and drops hidden-channel whitespace:

```
SELECT c/x FROM COMPOSITION c[a/b='c' AND d/e='f']
  -> emit   SELECT c/x FROM COMPOSITION c[a/b='c'ANDd/e='f']
  -> parse  aql: syntax error at 1:38: no viable alternative at input 'a/b='c'ANDd'
```

Same for `OR` and `MATCHES {/re/}`. String-internal spaces survive (`'c v d'` is fine), which is exactly why a corpus written in canonical form never caught it — the structural blind spot REQ-119's traceability entry already describes.

**This blocks defect 1.** Any guard worth having refuses `a/b='c'ANDd/e='f'`, which is the extractor's own output — so the guard cannot land until the extractor stops producing it.

REQ-118 already diagnosed this exact trap for literals and added [`sourceText`](../../../openehr/aql/parse/extract_query.go) to fix it, then left `GetText()` in place at every predicate site. Fixing the instance and not the axis is what this plan corrects.

## Design

### Part 1 — source text at every re-emitted position

Swap `GetText()` for the existing REQ-118 `sourceText` helper at each site whose text is later **re-emitted verbatim**, in both extractors (`ast.go` serves `Parse`/`Document`, `extract_query.go` serves `ParseQuery`/`Query`):

- `ClassExpr.Predicate` — class and `VERSION` branches
- `IdentifiedPath.Predicate`, `PathSegment.Predicate`
- `IdentifiedPath.Raw` — Emit renders paths from this field, so its documented "whitespace-collapsed" contract is what breaks WHERE / ORDER BY / SELECT path predicates
- `aql.Comparison.Path`

The set is defined by a property — *is this text re-emitted verbatim?* — not by a list, and Phase 1 pins it with a test that walks the corpus rather than a hand enumeration.

`versionPredicate` excludes its brackets in the grammar while `pathPredicate` includes them, so `trimBrackets` stays on the latter only.

**This does not reopen the path decision.** Guarding path positions against splice text remains out of scope by REQ-055 rule 3. Not corrupting a path's text is extraction fidelity, not a guard — the opposite direction.

### Part 2 — the position split

`checkClassOperands` must dispatch on `c.Version`, because the field feeds the two grammars above:

- **`Version == true`** → the text is `LATEST_VERSION` or `ALL_VERSIONS` (case-insensitive: the lexer builds both from case-insensitive letter fragments), **or** carries a top-level `COMPARISON_OPERATOR` outside literals, brackets and regexes — the necessary condition for `standardPredicate`. Refuses `VERSION v[at0001]`; accepts every `standardPredicate` and both keywords.
- **`Version == false`** → the bracket-escape scan below.

### Part 3 — the bracket-escape scan

Emission writes `"[" + Predicate + "]"`, so the **only** way the text alters the query's structure is by terminating that bracket early. Text that stays inside its brackets can at worst be a malformed predicate, which the parser rejects loudly — and REQ-119 explicitly reserves refusal for the silent mode. The condition is therefore exact, not approximate:

- bracket depth never goes negative and ends at zero, counting `[` / `]` **outside** string literals and regexes — `objectPath` legally nests (`a[at0001]/b='c'`);
- every `'…'` / `"…"` literal is terminated — an unterminated one swallows the emitter's own `]` and runs into the following clause, which is a silent substitution rather than a loud one;
- every `{/…/}` regex is terminated — `SLASH_REGEX_CHAR : ~[/\n\r] | ESCAPE_SEQ | '\\/'` admits `[` and `]` freely, so a character class must not be counted.

A backslash skips one byte (`ESCAPE_SEQ`, `UTF8CHAR` and `OCTAL_ESC` all begin `\`). `TERM_CODE`'s trailing `|…|` section needs no special case: its content is `~[|[\]]+`, which excludes both brackets.

Lives in `openehr/aql` with no lexer import, so REQ-013 holds.

**Tightening risk is nil by construction:** text the extractor produced came from a balanced, terminated bracket, so the guard accepts everything `ParseQuery` emits.

### There is no builder intake to guard

#99's checklist asks for the guard to be applied "to the equivalent builder intake". There is none: [`aql.Containment`](../../../openehr/aql/containment.go) carries `rmType` / `alias` / `archetypeID` only, and `Builder.From(rmType, alias)` takes no predicate. This position exists solely on `parse.ClassExpr`. Build/Emit parity is vacuous here and is recorded as such in the spec rather than closed by inventing a field.

## Definition of Ready

- **`Covers:`** names REQ-119 and the REQ-118 rule it extends. ✅
- Canonical normative prose exists for REQ-119; this plan **moves** its § Out of scope deferral into normative text and states the two-position rule. Landing in the same PR as the code (implementation-aligned, per [AGENTS.md § Source of truth](../../../AGENTS.md)).
- No irreversible fork needs an ADR: both guards are read off the vendored profile, and the accept-set trade (escape scan vs full validator) is recorded in § Deferred with its reasoning.
- Ground truth quoted, not guessed: `resources/aql/grammar/active/AqlParser.g4` (`pathPredicate`, `versionPredicate`, `nodePredicate`, `objectPath`) and `AqlLexer.g4` (`STRING`, `CONTAINED_REGEX`, `SLASH_REGEX`, `TERM_CODE`, `LATEST_VERSION`, `ALL_VERSIONS`). ✅

## Definition of Done

- Code and tests carry `// REQ-119` / `// PROBE-090` citations.
- REQ-119 § Out of scope loses the `ClassExpr.Predicate` bullet; the normative rules land in the section body.
- `traceability.yaml` tests list and REQ.md **Impl.** column reflect the implementation.
- CHANGELOG records the canonical-form change (predicate spacing is now preserved).
- `make spec-check` and `make ci` pass.
- Plan archived under [`docs/plans/archive/`](.) in the implementing PR. ✅

## Implementation checklist

| Step | Status |
|---|---|
| Phase 1 — source text at every re-emitted position | done |
| Phase 2 — position split (`VERSION` vs class) | done |
| Phase 3 — bracket-escape scan | done |
| Phase 4 — grammar confrontation, positive controls, mutation checks | done — 2954 generated cases, 699 accepted / 2255 refused |
| Spec / registry updated (REQ-119 §, `traceability.yaml`, CHANGELOG) | done |
| `make spec-check` | done — OK |
| `make ci` | done — 0 lint issues, all tests pass |

## Phases

### Phase 1 — source text at every re-emitted position

**Tasks:**

- Replace `GetText()` with `sourceText` at the predicate and path sites listed in § Design Part 1, in both `ast.go` and `extract_query.go`.
- Update `IdentifiedPath.Raw`'s doc comment: it is the source text, no longer whitespace-collapsed.
- Re-pin any golden or count that moves because emitted spacing now matches the source.

**Verification:** a round-trip **identity** test over a corpus extended with `[a/b='c' AND d/e='f']`, `[a/b='c' OR d/e='f']`, `[a/b MATCHES {/re/}]`, `[at0001, 'name']`, `[ehr_id/value = $id]` and the same forms in SELECT / WHERE / ORDER BY path positions — each currently fails to re-parse.

**Definition of done:** every predicate form in the corpus emits text that re-parses, and re-parses to the same text (identity, not merely parseability). No test asserts the collapsed spelling.

### Phase 2 — the position split

**Tasks:**

- Split the `Predicate` validation in `checkClassOperands` on `c.Version`.
- Add `validateVersionPredicate` — the two keywords or a top-level comparison operator.

**Verification:** `VERSION v[LATEST_VERSION]` / `[ALL_VERSIONS]` / `[commit_audit/time_committed > '2020']` accepted (the positive control that fails if the class-position rule is applied here); `VERSION v[at0001]` refused with an error wrapping `aql.ErrInvalidQuery`.

**Definition of done:** the two positions have visibly different accept sets, and a test names the reason.

### Phase 3 — the bracket-escape scan

**Tasks:**

- Add the scanner to `openehr/aql` beside the REQ-119 identifier guards, exported for a consumer holding the same line outside the package.
- Call it from `checkClassOperands`' non-`VERSION` branch, after the existing blank-predicate check (which stays: emptiness is a separate rule with its own test).

**Verification:** the #99 splice refused; `a[at0001]/b='c'` (nested brackets) accepted; `a/b='c` (unterminated string) refused; `a/b MATCHES {/[0-9]+/}` (brackets inside a regex) accepted; `a/b='c\']'` (escaped quote) accepted.

**Definition of done:** no text carrying a structural escape emits, and nothing `ParseQuery` produces is refused.

### Phase 4 — mechanical honesty

**Tasks:**

- **Generated** confrontation corpus: a cross product of bracket / quote / regex states rather than a hand list — REQ-119 § Acceptance forbids a hand corpus where the position has no token names to walk. Oracle is round-trip identity, since a spliced predicate parses perfectly well as something else.
- **Positive controls** that fail when either guard is tightened: every predicate form in the round-trip corpus must still emit.
- **Mutation checks**: removing the scan, removing the version rule, or reverting `sourceText` each fails a *named* test.

**Definition of done:** each guard is mutation-detectable, and the confrontation derives its corpus rather than enumerating it.

## Settled while implementing

- **Part 1 needed no new machinery.** [`sourceText`](../../../openehr/aql/parse/extract_query.go) already existed from REQ-118, and its own comment already said `GetText()` drops interior whitespace. The fix was applying the existing helper to the rest of the axis, which is why the diff is a swap rather than a new reader.
- **Emitted text now carries the caller's formatting, including comments.** The lexer `skip`s whitespace *and* comments rather than channelling them, so both survive in the character stream. A `--` comment's terminating newline rides through with it, which is what keeps it from swallowing the remainder of the emitted query. Pinned as corpus cases (`fmt_newline`, `fmt_comment`) rather than left to be discovered.
- **The confrontation property is two-sided.** A one-sided sweep would have passed with a guard that refuses everything (no substitution possible) or accepts everything (no tightening possible). The generated test asserts both directions and additionally fails if the corpus collapses or goes one-sided, so a generator that stops generating cannot pass quietly.
- **Mutation-detectability was verified by mutating, not asserted.** Each of the three guards was removed in turn and the failing test names recorded in the implementing commits.
- **No builder intake exists, as predicted.** Confirmed against [`aql.Containment`](../../../openehr/aql/containment.go): the write side models a class as RM type, alias and archetype id. Recorded in the REQ rather than closed by adding a field.

## Deferred

**A full `nodePredicate` recursive-descent validator.** The scan closes REQ-119's silent-substitution class exactly and refuses nothing the parser accepts. A complete validator would additionally catch *loud* malformations contained inside the brackets (`[a b c]`), but at the cost of several hundred hand-derived lines covering `nodePredicate`'s left recursion, nested `objectPath` predicates, `TERM_CODE` and `CONTAINED_REGEX` — the largest divergence surface in the package, in a package that may not import the lexer to check itself. Recorded here with its reasoning rather than approximated, the same way #99 itself was deferred out of #96.

## Mapping to specs

- [docs/specifications/clinical-modeling.md § REQ-119](../../specifications/clinical-modeling.md#req-119--re-parseable-canonical-aql-emission) — normative contract (the two predicate positions, the escape condition, the source-text rule)
- [docs/specifications/conformance.md § PROBE-090](../../specifications/conformance.md#probe-090--aql-emission-round-trip-closure) — acceptance
- [docs/specifications/REQ.md](../../specifications/REQ.md) — registry row
- [resources/aql/grammar/DIVERGENCES.md](../../../resources/aql/grammar/DIVERGENCES.md) — unchanged; this plan adds no grammar delta
