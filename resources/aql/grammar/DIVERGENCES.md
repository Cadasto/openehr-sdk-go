# AQL grammar divergences (SDK profile)

Every delta between `baseline/` (official openEHR) and `active/` (what
`make aqlgen` consumes) has a row here. IDs are stable (`SDK-AQL-NNN`); never
reuse. Two classes:

- **relaxation** — `active/` accepts *more* than official AQL (lint is a
  pre-flight aid, not a conformance gate; "lint-clean ≠ spec-conformant").
- **correction** — `active/` fixes a weak spot in the official grammar that is
  expected to be corrected upstream in time.

Regression fixtures live under
[`../../../openehr/aql/parse/testdata/grammar/`](../../../openehr/aql/parse/testdata/grammar/);
`.aql` must parse, `.reject` must not (asserted by the `parse` package tests).

---

### SDK-AQL-001 — `CONTAINS` string function shadowed by the containment operator

- **Class:** correction
- **Upstream:** QUERY Release-1.1.0 `AqlLexer.g4` — `STRING_FUNCTION_ID`
- **Symptom:** `STRING_FUNCTION_ID` lists `CONTAINS`, but the `CONTAINS`
  containment-operator token is declared earlier and shadows it (equal length →
  first rule wins). The string `CONTAINS(a,b)` function is therefore unreachable;
  it lexes as the containment operator and mis-parses.
- **Fix:** spell the string function `CONTAINS_STR`; inline its pattern in
  `STRING_FUNCTION_ID` (`… | (C O N T A I N S '_' S T R) | …`) so it lexes as
  `STRING_FUNCTION_ID` while `CONTAINS` stays the containment operator. A
  separate `CONTAINS_STR` token would either shadow it or be unreachable, so the
  pattern is inlined. Spec spelling `CONTAINS(a,b)` is intentionally rejected.
- **Regression:** `contains_containment.aql`, `contains_str_fn.aql`,
  `contains_spec_fn.reject`
- **Upstream status:** open (weak spot; expected to be corrected upstream)

### SDK-AQL-002 — `SELECT *`

- **Class:** relaxation
- **Upstream:** QUERY Release-1.1.0 `AqlParser.g4` — `selectExpr`
- **Symptom:** official AQL has no bare `SELECT *` (projection columns must be
  explicit); the SDK admits it as a convenience some CDRs honour.
- **Fix:** add `| SYM_ASTERISK` to `selectExpr` (the `SYM_ASTERISK` token already
  exists, used by `COUNT(*)`).
- **Regression:** `select_star.aql`
- **Upstream status:** intentional-relaxation
- **Notes:** REQ-109 prose states that lint-clean does not imply spec-conformance.

### SDK-AQL-003 — `LIMIT` / `OFFSET` parameter operands

- **Class:** correction
- **Upstream:** QUERY Release-1.1.0 `AqlParser.g4` — `limitClause`
- **Symptom:** `limitClause` accepts only `INTEGER`, so a parameterised page
  size/offset (`LIMIT $count OFFSET $start`) cannot be expressed, though `$param`
  is allowed everywhere else a value appears.
- **Fix:** introduce `limitValue : INTEGER | PARAMETER ;` and use it for both
  `LIMIT` and `OFFSET`. `OFFSET` still only follows `LIMIT` (no reversed order).
- **Regression:** `limit_offset_param.aql`, `offset_before_limit.reject`
- **Upstream status:** open (weak spot; expected to be corrected upstream)

### SDK-AQL-004 — string escape `\*`

- **Class:** correction
- **Upstream:** QUERY Release-1.1.0 `AqlLexer.g4` — `ESCAPE_SEQ`
- **Symptom:** `ESCAPE_SEQ` omits `\*`, so an escaped asterisk in a string
  literal fails to lex. (`\"` is already present in Release-1.1.0.)
- **Fix:** add `*` to the `ESCAPE_SEQ` character class.
- **Regression:** `escape_quote_star.aql`
- **Upstream status:** open (weak spot; expected to be corrected upstream)

### SDK-AQL-005 — comments skipped, not channelled

- **Class:** correction
- **Upstream:** QUERY Release-1.1.0 `AqlLexer.g4` — `COMMENT`
- **Symptom:** `COMMENT` is routed to `COMMENT_CHANNEL`; the SDK lint has no use
  for comment tokens and the channel adds machinery for no benefit.
- **Fix:** `COMMENT -> skip`; drop the now-unused `channels { COMMENT_CHANNEL }`
  block.
- **Regression:** `comment.aql` (parses with a trailing `--` comment skipped)
- **Upstream status:** open (cosmetic; harmless either way)
