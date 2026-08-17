package parse

// White-box unit for the fail-closed halves an EXTERNAL test cannot reach:
// [aql.WhereExpr] and [LimitExpr] are sealed vocabularies, so a shape that
// dereferences cleanly yet has no case in the skeleton's switches cannot be
// constructed from outside — and the external unmodelled-shape test is
// therefore caught by `where.presence` instead (its doc says so). These units
// hold the arm itself, so deleting [checkModelled] or a switch's fail-closed
// default fails a named test (REQ-119 § Emission verified after emission).

import (
	"errors"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
)

// fakeLimit is an in-package shape outside [DerefLimitExpr]'s learned set.
type fakeLimit struct{}

func (fakeLimit) isLimitExpr()  {}
func (fakeLimit) token() string { return "" }

func TestFailClosedHelpersRefuseUnmodelledShapes(t *testing.T) {
	// An UNDEREF-able limit reduces to "" — the same token as an absent one —
	// but the aliasing is unreachable through Emit: emitLimitValue refuses the
	// shape LOUDLY before verification runs. Pin both halves of that layering,
	// so weakening either surfaces here.
	if tok := limitToken(fakeLimit{}); tok != "" {
		t.Errorf("limitToken(fakeLimit) = %q, want \"\" (underef-able reduces to absent)", tok)
	}
	q := &Query{
		Select: SelectClause{Items: []SelectItem{{Expr: PathExpr{IdentifiedPath: IdentifiedPath{
			IdentifiedPath: aql.IdentifiedPath{Raw: "c/x"},
		}}}}},
		From:  FromClause{Root: ClassExpr{RMType: "COMPOSITION", Alias: "c"}},
		Limit: fakeLimit{},
	}
	if _, err := q.Emit(); err == nil || !strings.Contains(err.Error(), "carries no value") {
		t.Errorf("Emit(fakeLimit) = %v, want emitLimitValue's loud refusal — without it the "+
			"shape aliases an ABSENT limit in the skeleton", err)
	}

	// The unmodelled arm proper: a slot [limitToken]'s or [whereSlots]'s switch
	// could not model. Unreachable end-to-end today by construction (a shape
	// must first be taught to the Deref layer), so held at the unit level.
	err := checkModelled([]slot{{at: "limit", token: unmodelledPrefix + "parse.fakeLimit"}})
	if err == nil {
		t.Fatal("checkModelled accepted an unmodelled slot — the oracle is blind at that coordinate")
	}
	if !errors.Is(err, aql.ErrInvalidQuery) {
		t.Errorf("err = %v, want ErrInvalidQuery", err)
	}
	if !strings.Contains(err.Error(), "does not model") {
		t.Errorf("err = %v, want the fail-closed arm's own wording", err)
	}

	if err := checkModelled([]slot{{at: "limit", token: "10"}}); err != nil {
		t.Errorf("checkModelled refused a modelled slot: %v", err)
	}
}

// TestSkeletonDistinguishesEveryCoordinate is the unit half of the per-slot
// soundness table (§ Emission verified after emission): one row per slot NAME
// the skeleton can emit, each confronting two parsed queries that differ in
// exactly that slot and pinning the coordinate [diffSlots] reports. Deleting a
// slot from [skeletonOf] makes its row's skeletons compare equal, so the row
// fails — the REQ's mutation rule held for the coordinates no splice vector
// reaches end-to-end (`select.top`, and the flag-off direction of the others;
// the reachable ones are ALSO held end-to-end by
// TestEmitVerificationPinsEachCarriedCoordinate).
func TestSkeletonDistinguishesEveryCoordinate(t *testing.T) {
	const base = "SELECT c/x FROM COMPOSITION c"
	for _, tc := range []struct{ name, a, b, at string }{
		{"select.distinct", base, "SELECT DISTINCT c/x FROM COMPOSITION c", "select.distinct"},
		{"select.star", base, "SELECT * FROM COMPOSITION c", "select.star"},
		{
			"select.top",
			"SELECT TOP 5 c/x FROM COMPOSITION c",
			"SELECT TOP 6 c/x FROM COMPOSITION c",
			"select.top",
		},
		{"select item count", base, "SELECT c/x, c/y FROM COMPOSITION c", "select item count"},
		{
			"select item alias",
			"SELECT c/x AS a FROM COMPOSITION c",
			"SELECT c/x AS b FROM COMPOSITION c",
			"select.items[0].alias",
		},
		{"from.root", base, "SELECT c/x FROM EHR c", "from.root"},
		{
			"from.contains.class",
			base + " CONTAINS OBSERVATION o",
			base + " CONTAINS EVALUATION o",
			"from.contains.class",
		},
		{
			"from.contains.junction",
			base + " CONTAINS (OBSERVATION o AND EVALUATION ev)",
			base + " CONTAINS (OBSERVATION o OR EVALUATION ev)",
			"from.contains.junction",
		},
		{
			"junction operand class",
			base + " CONTAINS (OBSERVATION o AND EVALUATION ev)",
			base + " CONTAINS (OBSERVATION o AND CLUSTER ev)",
			"from.contains.op[1].class",
		},
		{
			"nested junction operator",
			base + " CONTAINS (OBSERVATION o AND (EVALUATION ev OR INSTRUCTION i))",
			base + " CONTAINS (OBSERVATION o AND (EVALUATION ev AND INSTRUCTION i))",
			"from.contains.op[1].junction",
		},
		{
			"chain element class",
			base + " CONTAINS OBSERVATION o CONTAINS ELEMENT e",
			base + " CONTAINS OBSERVATION o CONTAINS CLUSTER e",
			"from.contains.chain[0].class",
		},
		{
			"chain edge negation",
			base + " CONTAINS OBSERVATION o CONTAINS ELEMENT e",
			base + " CONTAINS OBSERVATION o NOT CONTAINS ELEMENT e",
			"from.contains.chain[0].class",
		},
		{
			"root junction operator",
			"SELECT a/x FROM COMPOSITION a OR OBSERVATION b",
			"SELECT a/x FROM COMPOSITION a AND OBSERVATION b",
			"from.junction",
		},
		{
			"root junction operand",
			"SELECT a/x FROM COMPOSITION a OR OBSERVATION b",
			"SELECT a/x FROM COMPOSITION a OR EVALUATION b",
			"from.op[1].class",
		},
		{"where.presence", base, base + " WHERE c/a = 1", "where.presence"},
		{"where leaf", base + " WHERE c/a = 1", base + " WHERE c/a >= 1", "where"},
		{
			"where.junction",
			base + " WHERE c/a = 1 AND c/b = 2",
			base + " WHERE c/a = 1 OR c/b = 2",
			"where.junction",
		},
		{
			"where junction term",
			base + " WHERE c/a = 1 AND c/b = 2",
			base + " WHERE c/a = 1 AND c/b = 3",
			"where.term[1]",
		},
		{"where.not", base + " WHERE NOT c/a = 1", base + " WHERE c/a = 1", "where.not"},
		{
			"where.not operand",
			base + " WHERE NOT c/a = 1",
			base + " WHERE NOT c/a = 2",
			"where.operand",
		},
		{
			"order by term count",
			base + " ORDER BY c/x",
			base + " ORDER BY c/x, c/y",
			"order by term count",
		},
		{
			"order by direction",
			base + " ORDER BY c/x ASC",
			base + " ORDER BY c/x DESC",
			"orderBy[0].direction",
		},
		{
			"order by path text",
			base + " ORDER BY c/x",
			base + " ORDER BY c/y",
			"orderBy[0].path text",
		},
		{"limit", base + " LIMIT 10", base + " LIMIT 20", "limit"},
		{"offset", base + " LIMIT 10 OFFSET 5", base + " LIMIT 10 OFFSET 6", "offset"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			qa, err := ParseQuery(tc.a)
			if err != nil {
				t.Fatalf("ParseQuery(%q): %v", tc.a, err)
			}
			qb, err := ParseQuery(tc.b)
			if err != nil {
				t.Fatalf("ParseQuery(%q): %v", tc.b, err)
			}
			at, ok := diffSlots(skeletonOf(qa), skeletonOf(qb))
			if ok {
				t.Fatalf("skeletons compare EQUAL; the slot distinguishing\n  %q\n  %q\nis gone", tc.a, tc.b)
			}
			if at != tc.at {
				t.Errorf("difference reported at %q, want %q", at, tc.at)
			}
		})
	}
}
