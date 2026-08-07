package aql

import (
	"errors"
	"fmt"
	"strings"
)

// WhereExpr is a boolean expression in a WHERE clause. The interface is sealed;
// construct expressions with the comparison helpers ([Eq], [Ne], [Gt], [Ge],
// [Lt], [Le]) and combine them with [And] / [Or]. Parsed queries populate the
// same concrete types ([Comparison] / [Junction]) — the read AST and the
// write AST share one vocabulary (REQ-113). Concrete-type
// fields are intended for read access; mutating an expression already
// passed to [FormatWhere] / [Builder.Build] is undefined (the emitter
// caches a `validate()` outcome the mutation would invalidate).
//
// Use [FormatWhere] to render a [WhereExpr] to canonical AQL text (e.g.
// when emitting a parsed [parse.Query] back to a string).
//
// The set grows ADDITIVELY as the structured-AST catalogue closes further
// grammar positions (REQ-117), so a consumer type-switching over it MUST
// treat an unrecognised case as out-of-catalogue — refuse, skip, or
// report — and MUST NOT panic on it.
type WhereExpr interface {
	// expr is the canonical wire form of the predicate.
	expr() string
	// validate reports a malformed predicate (empty path, nil value) so
	// [Builder.Build] can surface it as ErrInvalidQuery instead of panicking
	// or emitting invalid AQL.
	validate() error
}

// FormatWhere renders a [WhereExpr] to canonical AQL text. It validates
// the expression first; a malformed predicate (empty path, nil value,
// …) returns an error wrapping [ErrInvalidQuery]. A nil expression
// returns "" with no error (a vacuously-true WHERE — the builder skips
// the clause in that case).
//
// This is the public read-side mirror of the internal expr() method:
// consumers of a parsed [parse.Query] use FormatWhere to round-trip
// the WHERE predicate back to AQL without depending on package-local
// internals.
func FormatWhere(w WhereExpr) (string, error) {
	if w == nil {
		return "", nil
	}
	if err := w.validate(); err != nil {
		return "", err
	}
	return w.expr(), nil
}

// FormatValue renders an [aql.Value] to canonical AQL text (the same
// emission the Builder uses internally). Mirrors [FormatWhere] for the value
// side of the vocabulary.
//
// UNLIKE [FormatWhere], it does NOT validate: it has no error to return, so it
// cannot refuse, and a value the grammar has no spelling for renders as its Go
// text (`+Inf`, `NaN`) or as a call the parser rejects. It is therefore the
// deliberate escape hatch for a value the caller has already checked, and is
// excluded from REQ-119's round-trip closure guarantee for that reason. Call
// [ValidateValue] first if the value did not come from a validated source.
//
// It does not PANIC, though: a value with no wire form at all — a nil, or a
// typed-nil pointer shape such as the zero [MatchesExpr.Terminology] — returns
// "". Refusing one is [ValidateValue]'s job, and an unvalidated formatter that
// panics is a worse escape hatch than one that renders nothing.
func FormatValue(v Value) string {
	inner, ok := derefValue(v)
	if !ok {
		return ""
	}
	return inner.token()
}

// Operator is a comparison operator on a [Comparison]. The wire string is
// the typed string itself; values are not interpolated, so consumers can
// safely match on `c.Op == aql.OpEq` etc.
type Operator string

const (
	// OpEq is `=`.
	OpEq Operator = "="
	// OpNe is `!=`.
	OpNe Operator = "!="
	// OpGt is `>`.
	OpGt Operator = ">"
	// OpGe is `>=`.
	OpGe Operator = ">="
	// OpLt is `<`.
	OpLt Operator = "<"
	// OpLe is `<=`.
	OpLe Operator = "<="
)

// known reports whether o is one of the six operators the grammar's
// COMPARISON_OPERATOR admits. Operator is a named string so a parsed query
// keeps its wire spelling, which leaves the type open — [Compare] is the
// first constructor to take one from the caller, so the value is checked at
// validate time rather than emitted verbatim (REQ-117).
func (o Operator) known() bool {
	switch o {
	case OpEq, OpNe, OpGt, OpGe, OpLt, OpLe:
		return true
	default:
		return false
	}
}

// Comparison is a `path <op> value` predicate. It is the concrete type both
// the construction helpers ([Eq] / [Ne] / [Gt] / [Ge] / [Lt] / [Le]) and the
// parser populate; consumers reading a parsed query type-assert
// `w.(aql.Comparison)` and read the fields directly.
//
// Path is the alias-qualified RM path as it appears in the AQL text (e.g.
// `e/ehr_status/subject/external_ref/id/value`); Op is one of the [Operator]
// constants; Val is the [Value] on the right-hand side ([ParamValue] for a
// placeholder, [StringValue] / [IntValue] / [RealValue] / [BoolValue] for a
// literal).
//
// ParsedPath is the structured form of Path (alias + segments), populated
// by the parser on the read side so a consumer reads alias/segments without
// re-splitting the raw string (REQ-113); it is nil on the write side
// (the construction helpers set only Path) and MAY be nil on the read side
// for a path shape the parser does not structure. When non-nil,
// ParsedPath.Raw equals Path (both derive from the same source path).
// Emission uses Path, not ParsedPath, so round-trip is unaffected by its
// presence or absence.
//
// Left carries the left operand when it is NOT a plain path — the
// grammar's `functionCall COMPARISON_OPERATOR terminal` alternative
// (`LENGTH(o/name/value) > 5`, `TERMINOLOGY('a','b','c') = 'x'`), modelled
// as a [FuncCall] (REQ-117). It is nil for the ordinary path form, where
// Path carries the left operand; when Left is non-nil it is authoritative
// for emission and Path is empty (the parser leaves it so). Construct this
// form with [Compare].
type Comparison struct {
	Path       string
	Op         Operator
	Val        Value
	ParsedPath *IdentifiedPath
	Left       Value
}

func (c Comparison) expr() string {
	return c.leftToken() + " " + string(c.Op) + " " + c.Val.token()
}

// leftToken renders the left operand: the structured [Comparison.Left]
// value when present, the raw path otherwise (REQ-117).
func (c Comparison) leftToken() string {
	if c.Left != nil {
		return c.Left.token()
	}
	return c.Path
}

func (c Comparison) validate() error {
	if !c.Op.known() {
		return fmt.Errorf("%w: unknown comparison operator %q on %q",
			ErrInvalidQuery, string(c.Op), c.leftToken())
	}
	if c.Left == nil && strings.TrimSpace(c.Path) == "" {
		return fmt.Errorf("%w: empty path in %s comparison", ErrInvalidQuery, string(c.Op))
	}
	// Path and Left are the two spellings of ONE left operand, so setting
	// both is a caller error, not a precedence question: leftToken would
	// silently discard Path (REQ-117). The sibling MatchesExpr counts its
	// operand forms the same way.
	if c.Left != nil && strings.TrimSpace(c.Path) != "" {
		return fmt.Errorf("%w: comparison sets both Path %q and a structured Left operand",
			ErrInvalidQuery, c.Path)
	}
	if c.Left != nil {
		if err := validateValue(c.Left); err != nil {
			return err
		}
	}
	if c.Val == nil {
		return fmt.Errorf("%w: nil value in comparison on %q", ErrInvalidQuery, c.leftToken())
	}
	return validateValue(c.Val)
}

// Eq is `path = value`.
func Eq(path string, v Value) WhereExpr { return Comparison{Path: path, Op: OpEq, Val: v} }

// Ne is `path != value`.
func Ne(path string, v Value) WhereExpr { return Comparison{Path: path, Op: OpNe, Val: v} }

// Gt is `path > value`.
func Gt(path string, v Value) WhereExpr { return Comparison{Path: path, Op: OpGt, Val: v} }

// Ge is `path >= value`.
func Ge(path string, v Value) WhereExpr { return Comparison{Path: path, Op: OpGe, Val: v} }

// Lt is `path < value`.
func Lt(path string, v Value) WhereExpr { return Comparison{Path: path, Op: OpLt, Val: v} }

// Le is `path <= value`.
func Le(path string, v Value) WhereExpr { return Comparison{Path: path, Op: OpLe, Val: v} }

// Compare is `<left> <op> <right>` where the LEFT operand is a structured
// [Value] rather than a path — the write-side mirror of the parser's
// function-call comparison LHS (REQ-117), e.g.
// Compare(Func("LENGTH", Path("o/name/value")), OpGt, Int(5)) emits
// `LENGTH(o/name/value) > 5`. Use [Eq] / [Ne] / [Gt] / [Ge] / [Lt] / [Le]
// for the ordinary path form.
func Compare(left Value, op Operator, right Value) WhereExpr {
	return Comparison{Op: op, Val: right, Left: left}
}

// BoolOp is a boolean junction operator (AND or OR) joining terms in a
// [Junction]. NOT is a single-operand prefix; see [Not] (when introduced
// by the parser-side AST extension).
type BoolOp string

const (
	// OpAnd is the AND junction.
	OpAnd BoolOp = "AND"
	// OpOr is the OR junction.
	OpOr BoolOp = "OR"
)

// Junction is a multi-term boolean junction (`a AND b`, `a OR b OR c`,
// …). Op is one of the [BoolOp] constants; Terms is the ordered list of
// operands. Parsed queries flatten same-operator chains: a literal
// `a OR b OR c` populates a single [Junction] with three Terms; a
// mixed-operator expression `a AND (b OR c)` populates an outer AND
// [Junction] whose second term is itself a [Junction]. The emitter
// re-parenthesises a nested OR inside an AND to preserve precedence
// (and vice-versa is unnecessary because OR has lower precedence).
type Junction struct {
	Op    BoolOp
	Terms []WhereExpr
}

func (j Junction) expr() string {
	parts := make([]string, len(j.Terms))
	for i, t := range j.Terms {
		// Parenthesise a nested OR inside an AND to preserve precedence;
		// a bare comparison or same-operator junction needs no grouping.
		if inner, ok := t.(Junction); ok && inner.Op == OpOr && j.Op == OpAnd {
			parts[i] = "(" + t.expr() + ")"
			continue
		}
		parts[i] = t.expr()
	}
	return strings.Join(parts, " "+string(j.Op)+" ")
}

func (j Junction) validate() error {
	for _, t := range j.Terms {
		if err := t.validate(); err != nil {
			return err
		}
	}
	return nil
}

// And joins predicates with AND. nil terms are dropped; a single surviving term
// is returned unchanged; no terms yields nil (a vacuously-true conjunction —
// the builder emits no WHERE rather than invalid AQL).
func And(terms ...WhereExpr) WhereExpr { return junctionOf(OpAnd, terms) }

// Or joins predicates with OR, with the same nil/empty handling as [And].
func Or(terms ...WhereExpr) WhereExpr { return junctionOf(OpOr, terms) }

func junctionOf(op BoolOp, terms []WhereExpr) WhereExpr {
	kept := make([]WhereExpr, 0, len(terms))
	for _, t := range terms {
		if t != nil {
			kept = append(kept, t)
		}
	}
	switch len(kept) {
	case 0:
		return nil
	case 1:
		return kept[0]
	default:
		return Junction{Op: op, Terms: kept}
	}
}

// NotExpr is a single-operand boolean negation (`NOT <operand>`). Parsed
// queries populate this when the source carries an explicit NOT prefix.
// The Builder composes NOT predicates via the package-level [Not] helper
// passed into [Builder.Where] — mirroring how [And] / [Or] / [Eq] are
// also package-level helpers rather than Builder methods. No dedicated
// Builder.Not method exists by design; predicate composition is intended
// to flow through the helper functions.
type NotExpr struct {
	Operand WhereExpr
}

func (n NotExpr) expr() string {
	if n.Operand == nil {
		return "NOT"
	}
	// Parenthesise any junction operand so the precedence reads
	// unambiguously regardless of which junctions surround the NOT.
	if _, ok := n.Operand.(Junction); ok {
		return "NOT (" + n.Operand.expr() + ")"
	}
	return "NOT " + n.Operand.expr()
}

func (n NotExpr) validate() error {
	if n.Operand == nil {
		return fmt.Errorf("%w: NOT with nil operand", ErrInvalidQuery)
	}
	return n.Operand.validate()
}

// Not constructs a [NotExpr]. A nil operand yields nil (the emitter has
// nothing to express and the builder skips the clause).
func Not(operand WhereExpr) WhereExpr {
	if operand == nil {
		return nil
	}
	return NotExpr{Operand: operand}
}

// ExistsExpr is the `EXISTS <path>` AQL predicate (presence of a node
// at the path). The parser populates this from the EXISTS form.
type ExistsExpr struct {
	Path string
}

func (e ExistsExpr) expr() string { return "EXISTS " + e.Path }

func (e ExistsExpr) validate() error {
	if strings.TrimSpace(e.Path) == "" {
		return fmt.Errorf("%w: empty path in EXISTS", ErrInvalidQuery)
	}
	return nil
}

// Exists constructs an [ExistsExpr]. An empty path is rejected at
// build time, not at construction.
func Exists(path string) WhereExpr { return ExistsExpr{Path: path} }

// MatchesExpr is the `<path> MATCHES <operand>` AQL predicate. The
// grammar admits three operand forms and exactly ONE of the fields below
// carries the operand:
//
//   - Values — the braced value list (`{'active', 'archived'}`); each
//     member is any [Value], including a [FuncCall] for the grammar's
//     `valueListItem : terminologyFunction` alternative.
//   - Terminology — a BARE `TERMINOLOGY('op','api','params')` operand,
//     with no braces (REQ-117); construct with [MatchesTerminology].
//   - URI — a braced URI operand (`{uri://…}`), carried verbatim
//     (REQ-117); construct with [MatchesURI]. A whitespace-only URI counts as
//     ABSENT — for both validation and emission — so it never shadows a
//     populated Values list.
type MatchesExpr struct {
	Path        string
	Values      []Value
	Terminology *FuncCall
	URI         string
}

func (m MatchesExpr) expr() string {
	// The URI-operand test MUST use the same emptiness rule as validate() —
	// TrimSpace, not `!= ""`. A whitespace-only URI is not an operand there, so
	// treating it as one here would emit `MATCHES {   }` and silently drop a
	// validated value list.
	uri := strings.TrimSpace(m.URI)
	switch {
	case m.Terminology != nil:
		// Bare terminology operand — the grammar's `matchesOperand :
		// terminologyFunction` alternative takes no braces.
		return m.Path + " MATCHES " + m.Terminology.token()
	case uri != "":
		// Emitted trimmed, consistent with the [MatchesURI] constructor.
		return m.Path + " MATCHES {" + uri + "}"
	}
	parts := make([]string, len(m.Values))
	for i, v := range m.Values {
		if v == nil {
			parts[i] = ""
			continue
		}
		parts[i] = v.token()
	}
	return m.Path + " MATCHES {" + strings.Join(parts, ", ") + "}"
}

func (m MatchesExpr) validate() error {
	if strings.TrimSpace(m.Path) == "" {
		return fmt.Errorf("%w: empty path in MATCHES", ErrInvalidQuery)
	}
	// The three operand forms are mutually exclusive: emitting more than
	// one would silently drop an operand (REQ-117).
	forms := 0
	if len(m.Values) > 0 {
		forms++
	}
	if m.Terminology != nil {
		forms++
	}
	if strings.TrimSpace(m.URI) != "" {
		forms++
	}
	switch {
	case forms == 0:
		return fmt.Errorf("%w: empty value list in MATCHES on %q", ErrInvalidQuery, m.Path)
	case forms > 1:
		return fmt.Errorf("%w: MATCHES on %q sets more than one operand form", ErrInvalidQuery, m.Path)
	}
	if uri := strings.TrimSpace(m.URI); uri != "" {
		return validateURIOperand(uri, m.Path)
	}
	if m.Terminology != nil {
		return validateValue(*m.Terminology)
	}
	for i, v := range m.Values {
		if v == nil {
			return fmt.Errorf("%w: nil value at index %d in MATCHES on %q", ErrInvalidQuery, i, m.Path)
		}
		if err := validateValue(v); err != nil {
			return err
		}
	}
	return nil
}

// validateURIOperand refuses a `MATCHES {uri}` operand the grammar's URI token
// cannot carry.
//
// The operand is emitted verbatim between braces because the token is
// unquoted, which makes it the one value position with no escaping to hide
// behind: a `}` closes the operand early, so a URI carrying one turns a single
// predicate into a different, still-VALID query — `MATCHES {uri://a} OR c/y
// MATCHES {uri://b}` — that no round-trip, golden, or parser check can catch.
// Refusing at validation is the only place the substitution is visible.
//
// The check is POSITIONAL, following the token's own decomposition (`URI :
// URI_SCHEME ':' URI_HIER_PART ( '?' URI_QUERY )? ( '#' URI_FRAGMENT )?`,
// resources/aql/grammar/active/AqlLexer.g4), because a flat union of the
// delimiter sets is not what the token admits: `%` leads `URI_PCT_ENCODED` and
// so requires two hex digits, `[`/`]` occur only as an `URI_IP_LITERAL` host,
// and `#` separates the fragment once and appears nowhere inside it. A flat
// alphabet accepted all three and emitted AQL this SDK's own parser rejects
// (REQ-119).
//
// It is still not an RFC-3986 parser: `URI_REG_NAME`, `URI_PORT` and
// `URI_PATH_*` are character classes in this grammar, so a structurally odd but
// spellable URI (an empty host, a dotted-quad out of range) reaches the wire,
// where the backend stays the authority — the same division of labour the SDK
// keeps everywhere else (PROBE-021). The property this guard owes is narrower
// and testable: whatever it accepts MUST come back from ParseQuery as one
// MATCHES predicate on the same URI.
func validateURIOperand(uri, path string) error {
	bad := func(what string, detail any) error {
		return fmt.Errorf("%w: MATCHES on %q carries a URI %s (%v): %q",
			ErrInvalidQuery, path, what, detail, uri)
	}
	// URI_SCHEME : ALPHA_CHAR ( ALPHA_CHAR | DIGIT | '+' | '-' | '.' )* — an
	// absent scheme is the commonest way a caller passes something that is not
	// a URI at all.
	scheme, rest, ok := strings.Cut(uri, ":")
	if !ok || scheme == "" {
		return bad("operand", "no scheme")
	}
	if !asciiLetter(scheme[0]) {
		return bad("whose scheme does not start with a letter", scheme)
	}
	for i := range len(scheme) {
		if c := scheme[i]; !schemeChar(c) {
			return bad("with an invalid scheme character", string([]byte{c}))
		}
	}

	// The fragment comes off first and the query second, so a '?' inside the
	// fragment stays part of it (URI_FRAGMENT admits '?', URI_QUERY does not
	// admit '#').
	hier, frag, hasFrag := strings.Cut(rest, "#")
	hier, query, _ := strings.Cut(hier, "?")
	if hasFrag {
		if strings.Contains(frag, "#") {
			return bad("with a second", "'#' — URI_FRAGMENT admits none")
		}
		if err := checkURIRun(frag, uriQueryByte); err != nil {
			return bad("whose fragment is unspellable", err)
		}
	}
	if err := checkURIRun(query, uriQueryByte); err != nil {
		return bad("whose query is unspellable", err)
	}

	// URI_HIER_PART : '//' URI_AUTHORITY URI_PATH_ABEMPTY | URI_PATH_ABSOLUTE
	// | URI_PATH_ROOTLESS | URI_PATH_EMPTY. Only the authority form admits a
	// bracketed host, so brackets are refused everywhere else by URI_PCHAR.
	if authority, ok := strings.CutPrefix(hier, "//"); ok {
		// URI_PATH_ABEMPTY starts at the first '/', so everything before it is
		// the authority and the rest stays with the path check below.
		hier = ""
		if slash := strings.IndexByte(authority, '/'); slash >= 0 {
			authority, hier = authority[:slash], authority[slash:]
		}
		if err := checkURIAuthority(authority); err != nil {
			return bad("whose authority is unspellable", err)
		}
	}
	if err := checkURIRun(hier, uriPathByte); err != nil {
		return bad("whose path is unspellable", err)
	}
	// Spellable as a URI is necessary but not sufficient: the operand is lexed
	// on its own between the braces, and TERM_CODE is declared BEFORE URI
	// (AqlLexer.g4:174 vs :179), so on an equal-length match it wins the tie and
	// `matchesOperand` — which admits URI and not TERM_CODE — rejects the token.
	if termCodeShadows(uri) {
		return bad("that lexes as a TERM_CODE rather than a URI", "the operand is <term>::<code>, "+
			"which MATCHES admits only as an archetype-predicate operand")
	}
	return nil
}

// termCodeShadows reports whether the whole text matches `TERM_CODE :
// TERM_CODE_CHAR+ ( '(' TERM_CODE_CHAR+ ')' )? '::' TERM_CODE_CHAR+ ( '|'
// ~[|[\]]+ '|' )?`, in which case the lexer prefers TERM_CODE over URI.
//
// `TERM_CODE_CHAR : NAME_CHAR | '.'` admits no `:` and no `/`, which is why
// `a::/b` and `a:::b` stay URIs while `SNOMED-CT::73211009` does not.
func termCodeShadows(s string) bool {
	if display, ok := strings.CutSuffix(s, "|"); ok {
		// The optional trailing `'|' ~[|[\]]+ '|'` display name.
		head, inner, found := strings.Cut(display, "|")
		if !found || inner == "" || strings.ContainsAny(inner, "|[]") {
			return false
		}
		s = head
	}
	head, code, ok := strings.Cut(s, "::")
	if !ok {
		return false
	}
	if open := strings.IndexByte(head, '('); open >= 0 {
		// The optional parenthesised `( '(' TERM_CODE_CHAR+ ')' )` qualifier.
		closed, ok := strings.CutSuffix(head[open+1:], ")")
		if !ok || !allTermCodeChars(closed) {
			return false
		}
		head = head[:open]
	}
	return allTermCodeChars(head) && allTermCodeChars(code)
}

func allTermCodeChars(s string) bool {
	if s == "" {
		return false
	}
	for i := range len(s) {
		if c := s[i]; !wordChar(c) && c != '-' && c != '.' {
			return false
		}
	}
	return true
}

// checkURIAuthority checks `( URI_USERINFO '@' )? URI_HOST ( ':' URI_PORT )?`.
func checkURIAuthority(authority string) error {
	if userinfo, host, ok := strings.Cut(authority, "@"); ok {
		if err := checkURIRun(userinfo, uriUserinfoByte); err != nil {
			return fmt.Errorf("userinfo: %w", err)
		}
		authority = host
	}
	// URI_IP_LITERAL : '[' URI_IPV6_LITERAL ']' is the only bracketed form, and
	// it is the whole host, so anything after ']' can only be `:` port.
	if rest, ok := strings.CutPrefix(authority, "["); ok {
		lit, tail, closed := strings.Cut(rest, "]")
		if !closed {
			return errors.New("'[' opens an IP literal that is never closed")
		}
		if err := checkURIIPv6(lit); err != nil {
			return err
		}
		if tail == "" {
			return nil
		}
		port, isPort := strings.CutPrefix(tail, ":")
		if !isPort {
			return fmt.Errorf("%q follows the IP literal; only \":\" port may", tail)
		}
		return checkURIPort(port)
	}
	// URI_HOST : URI_IPV4_ADDRESS | URI_REG_NAME — both are drawn from the
	// reg-name alphabet, so one check covers them; a trailing `:digits*` is
	// URI_PORT (which the grammar lets be empty).
	if host, port, ok := strings.Cut(authority, ":"); ok {
		if err := checkURIPort(port); err != nil {
			return err
		}
		authority = host
	}
	if err := checkURIRun(authority, uriRegNameByte); err != nil {
		return fmt.Errorf("host: %w", err)
	}
	return nil
}

// checkURIIPv6 checks URI_IPV6_LITERAL : HEX_QUAD (':' HEX_QUAD)* '::' HEX_QUAD
// (':' HEX_QUAD)* — note this grammar REQUIRES the `::` and a full quad on each
// side, so the common abbreviations (`::1`, `2001:db8::1`) are not spellable.
func checkURIIPv6(lit string) error {
	left, right, ok := strings.Cut(lit, "::")
	if !ok {
		return fmt.Errorf("IPv6 literal %q carries no \"::\", which URI_IPV6_LITERAL requires", lit)
	}
	if strings.Contains(right, "::") {
		return fmt.Errorf("IPv6 literal %q carries more than one \"::\"", lit)
	}
	for _, half := range []string{left, right} {
		if half == "" {
			return fmt.Errorf("IPv6 literal %q omits a group; URI_IPV6_LITERAL requires a quad either side of \"::\"", lit)
		}
		for quad := range strings.SplitSeq(half, ":") {
			if len(quad) != 4 {
				return fmt.Errorf("IPv6 literal %q carries %q; URI_IPV6_LITERAL admits only 4-digit hex quads", lit, quad)
			}
			for i := range len(quad) {
				if !hexDigit(quad[i]) {
					return fmt.Errorf("IPv6 literal %q carries a non-hex digit in %q", lit, quad)
				}
			}
		}
	}
	return nil
}

// checkURIPort checks URI_PORT : DIGIT* (empty is admitted by the grammar).
func checkURIPort(port string) error {
	for i := range len(port) {
		if c := port[i]; c < '0' || c > '9' {
			return fmt.Errorf("port %q is not URI_PORT : DIGIT*", port)
		}
	}
	return nil
}

// checkURIRun walks one URI component, admitting a byte that passes admits or a
// `%HH` triple (URI_PCT_ENCODED). `%` is handled here rather than in the byte
// predicates because it is the one place the token is not a character class.
func checkURIRun(s string, admits func(byte) bool) error {
	for i := 0; i < len(s); {
		if c := s[i]; c == '%' {
			if i+2 >= len(s) || !hexDigit(s[i+1]) || !hexDigit(s[i+2]) {
				return fmt.Errorf("%q is not URI_PCT_ENCODED : '%%' HEX_DIGIT HEX_DIGIT", s[i:min(i+3, len(s))])
			}
			i += 3
			continue
		} else if !admits(c) {
			return fmt.Errorf("%q is outside the alphabet of this position", string([]byte{c}))
		}
		i++
	}
	return nil
}

// The byte predicates below spell the grammar's URI fragments. `%` is never
// admitted by one — [checkURIRun] owns URI_PCT_ENCODED.
func schemeChar(c byte) bool {
	return asciiLetter(c) || (c >= '0' && c <= '9') || strings.IndexByte("+-.", c) >= 0
}

func hexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// uriUnreservedByte spells URI_UNRESERVED; uriSubDelimByte spells URI_SUB_DELIMS.
func uriUnreservedByte(c byte) bool {
	return asciiLetter(c) || (c >= '0' && c <= '9') || strings.IndexByte("-._~", c) >= 0
}

func uriSubDelimByte(c byte) bool { return strings.IndexByte("!$&'()*+,;=", c) >= 0 }

// uriRegNameByte spells URI_REG_NAME; uriUserinfoByte adds ':'; uriPcharByte
// adds ':' and '@'; uriPathByte adds '/'; uriQueryByte adds '/' and '?' (which
// is also URI_FRAGMENT's set).
func uriRegNameByte(c byte) bool { return uriUnreservedByte(c) || uriSubDelimByte(c) }

func uriUserinfoByte(c byte) bool { return uriRegNameByte(c) || c == ':' }

func uriPcharByte(c byte) bool { return uriUserinfoByte(c) || c == '@' }

func uriPathByte(c byte) bool { return uriPcharByte(c) || c == '/' }

func uriQueryByte(c byte) bool { return uriPathByte(c) || c == '?' }

// Matches constructs a [MatchesExpr] over a braced value list.
func Matches(path string, values ...Value) WhereExpr {
	return MatchesExpr{Path: path, Values: values}
}

// MatchesTerminology constructs the `<path> MATCHES
// TERMINOLOGY(operation, api, params)` predicate — the bare
// terminology-function operand form (REQ-117).
func MatchesTerminology(path, operation, api, params string) WhereExpr {
	fc := Terminology(operation, api, params).(FuncCall)
	return MatchesExpr{Path: path, Terminology: &fc}
}

// MatchesURI constructs the `<path> MATCHES {uri}` predicate — the URI
// operand form (REQ-117). The URI is emitted verbatim inside the braces
// (the grammar's URI token is unquoted).
func MatchesURI(path, uri string) WhereExpr {
	return MatchesExpr{Path: path, URI: strings.TrimSpace(uri)}
}

// LikeExpr is the `<path> LIKE <pattern>` AQL predicate. Pattern is a
// string literal carrying AQL wildcards (`_` single char, `%` any
// sequence). Pattern is a [Value] so the same shape covers both a
// literal pattern and a parameter-bound pattern.
type LikeExpr struct {
	Path    string
	Pattern Value
}

func (l LikeExpr) expr() string {
	if l.Pattern == nil {
		return l.Path + " LIKE "
	}
	return l.Path + " LIKE " + l.Pattern.token()
}

func (l LikeExpr) validate() error {
	if strings.TrimSpace(l.Path) == "" {
		return fmt.Errorf("%w: empty path in LIKE", ErrInvalidQuery)
	}
	if l.Pattern == nil {
		return fmt.Errorf("%w: nil pattern in LIKE on %q", ErrInvalidQuery, l.Path)
	}
	return validateValue(l.Pattern)
}

// Like constructs a [LikeExpr].
func Like(path string, pattern Value) WhereExpr {
	return LikeExpr{Path: path, Pattern: pattern}
}
