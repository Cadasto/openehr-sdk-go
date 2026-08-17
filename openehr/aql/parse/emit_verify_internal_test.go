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
