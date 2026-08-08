package parse_test

// REQ-119 · PROBE-090
//
// value_position_parity_test.go sweeps the CROSS PRODUCT the corpus tests
// structurally cannot: every value kind × every value position × both
// carriers. Four review rounds proved that a value-kind defect hides in the
// position nobody wrote a row for — `MATCHES {true}` survived them all
// because the value-kind sweep ran only through the comparison RHS, while a
// hand-written accepted/refused list in the aql package (which cannot see the
// grammar) pinned the wrong answer as correct.
//
// The property is deliberately one-directional: this test does NOT decide
// which kinds a position admits — the grammar does, through the round trip.
// For every (kind, position):
//
//   - if the write path ACCEPTS the value, the emitted text MUST re-parse and
//     the position's operand MUST come back equal ([aql.EqualValues]) — which
//     kills both failure classes at once, text the parser rejects AND text
//     that parses as something else;
//   - the POINTER carrier must agree with the value carrier: both accepted
//     with byte-identical text, or both refused.
//
// A guard that is too strict (refusing a kind the grammar admits) is caught
// by the positive rows of the per-position corpus tests, which stay
// authoritative for that direction.

import (
	"fmt"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
)

// pointerTwin returns the same value carried by its pointer shape.
func pointerTwin(t *testing.T, v aql.Value) aql.Value {
	t.Helper()
	switch s := v.(type) {
	case aql.StringValue:
		return &s
	case aql.IntValue:
		return &s
	case aql.RealValue:
		return &s
	case aql.BoolValue:
		return &s
	case aql.NullValue:
		return &s
	case aql.ParamValue:
		return &s
	case aql.PathValue:
		return &s
	case aql.FuncCall:
		return &s
	}
	t.Fatalf("pointerTwin: unhandled shape %T — extend this helper alongside the vocabulary", v)
	return nil
}

// valuePosition is one grammar position that carries an [aql.Value]: emit
// builds a full query around the value through a supported write path, and
// read recovers the operand from the re-parsed document.
type valuePosition struct {
	emit func(v aql.Value) (string, error)
	read func(doc *parse.Query) (aql.Value, error)
}

func wherePosition(mk func(v aql.Value) aql.WhereExpr, read func(w aql.WhereExpr) (aql.Value, error)) valuePosition {
	return valuePosition{
		emit: func(v aql.Value) (string, error) {
			pred, err := aql.FormatWhere(mk(v))
			if err != nil {
				return "", err
			}
			return "SELECT c/uid/value FROM COMPOSITION c WHERE " + pred, nil
		},
		read: func(doc *parse.Query) (aql.Value, error) { return read(doc.Where) },
	}
}

func TestEveryValueKindInEveryPositionRoundTripsOrRefuses(t *testing.T) {
	kinds := map[string]aql.Value{
		"string":      aql.String("O'Brien"),
		"int":         aql.Int(-3),
		"real":        aql.Real(2),
		"bool":        aql.Bool(true),
		"null":        aql.Null(),
		"param":       aql.Param("p"),
		"path":        aql.Path("c/other/value"),
		"func":        aql.Func("LENGTH", aql.Path("c/other/value")),
		"terminology": aql.Terminology("expand", "//fhir", "url=x"),
	}

	positions := map[string]valuePosition{
		"comparison RHS": wherePosition(
			func(v aql.Value) aql.WhereExpr { return aql.Eq("c/n/value", v) },
			func(w aql.WhereExpr) (aql.Value, error) {
				c, ok := w.(aql.Comparison)
				if !ok {
					return nil, fmt.Errorf("WHERE came back %T, want Comparison", w)
				}
				return c.Val, nil
			}),
		"MATCHES member": wherePosition(
			func(v aql.Value) aql.WhereExpr { return aql.Matches("c/n/value", v) },
			func(w aql.WhereExpr) (aql.Value, error) {
				m, ok := w.(aql.MatchesExpr)
				if !ok {
					return nil, fmt.Errorf("WHERE came back %T, want MatchesExpr", w)
				}
				// A bare TERMINOLOGY member may legally come back on the
				// Terminology carrier when it is the whole operand.
				if len(m.Values) == 1 {
					return m.Values[0], nil
				}
				if m.Terminology != nil {
					return m.Terminology, nil
				}
				return nil, fmt.Errorf("MATCHES came back with %d members", len(m.Values))
			}),
		"LIKE pattern": wherePosition(
			func(v aql.Value) aql.WhereExpr { return aql.Like("c/n/value", v) },
			func(w aql.WhereExpr) (aql.Value, error) {
				l, ok := w.(aql.LikeExpr)
				if !ok {
					return nil, fmt.Errorf("WHERE came back %T, want LikeExpr", w)
				}
				return l.Pattern, nil
			}),
		"SELECT item": {
			emit: func(v aql.Value) (string, error) {
				q := &parse.Query{
					Select: parse.SelectClause{Items: []parse.SelectItem{{Expr: parse.LiteralExpr{Value: v}}}},
					From:   parse.FromClause{Root: parse.ClassExpr{RMType: "COMPOSITION", Alias: "c"}},
				}
				return q.Emit()
			},
			read: func(doc *parse.Query) (aql.Value, error) {
				if len(doc.Select.Items) != 1 {
					return nil, fmt.Errorf("SELECT came back with %d items", len(doc.Select.Items))
				}
				switch e := doc.Select.Items[0].Expr.(type) {
				case parse.LiteralExpr:
					return e.Value, nil
				case parse.FunctionCall:
					// A projected call is modelled by FunctionCall on
					// re-read; rebuild the value form for comparison.
					return selectCallAsValue(e)
				case parse.PathExpr:
					return aql.Path(e.Raw), nil
				default:
					return nil, fmt.Errorf("SELECT item came back %T", e)
				}
			},
		},
		"SELECT function argument": {
			emit: func(v aql.Value) (string, error) {
				q := &parse.Query{
					Select: parse.SelectClause{Items: []parse.SelectItem{{Expr: parse.FunctionCall{
						Name: "CONCAT",
						Args: []parse.SelectExpr{
							parse.LiteralExpr{Value: aql.String("a")},
							parse.LiteralExpr{Value: v},
						},
					}}}},
					From: parse.FromClause{Root: parse.ClassExpr{RMType: "COMPOSITION", Alias: "c"}},
				}
				return q.Emit()
			},
			read: func(doc *parse.Query) (aql.Value, error) {
				fc, ok := doc.Select.Items[0].Expr.(parse.FunctionCall)
				if !ok || len(fc.Args) != 2 {
					return nil, fmt.Errorf("SELECT came back %#v", doc.Select.Items[0].Expr)
				}
				switch a := fc.Args[1].(type) {
				case parse.LiteralExpr:
					return a.Value, nil
				case parse.PathExpr:
					return aql.Path(a.Raw), nil
				case parse.FunctionCall:
					return selectCallAsValue(a)
				default:
					return nil, fmt.Errorf("argument came back %T", a)
				}
			},
		},
	}

	for posName, pos := range positions {
		for kindName, sample := range kinds {
			t.Run(posName+"/"+kindName, func(t *testing.T) {
				text, err := pos.emit(sample)
				twin := pointerTwin(t, sample)
				twinText, twinErr := pos.emit(twin)

				// Carrier parity: the pointer twin gets the same answer.
				if (err == nil) != (twinErr == nil) {
					t.Fatalf("carrier asymmetry: value err=%v, pointer err=%v", err, twinErr)
				}
				if err != nil {
					return // refused on both carriers — a legal outcome
				}
				if twinText != text {
					t.Fatalf("carriers emitted differently:\n  value:   %s\n  pointer: %s", text, twinText)
				}

				// Accepted ⇒ the text re-parses AND the operand survives.
				doc, perr := parse.ParseQuery(text)
				if perr != nil {
					t.Fatalf("accepted %T in %s but the emitted text does not re-parse\n  text: %s\n  err: %v",
						sample, posName, text, perr)
				}
				got, rerr := pos.read(doc)
				if rerr != nil {
					t.Fatalf("accepted %T in %s but it came back as a DIFFERENT construct — the "+
						"silent-substitution class\n  text: %s\n  read: %v", sample, posName, text, rerr)
				}
				if !aql.EqualValues(got, sample) {
					t.Fatalf("accepted %T in %s but the value came back changed\n  text: %s\n  in:  %#v\n  out: %#v",
						sample, posName, text, sample, got)
				}
			})
		}
	}
}

// selectCallAsValue rebuilds the aql.Value form of a projected function call
// whose arguments are literals or paths, so the sweep can compare a
// FunctionCall-carried read against the FuncCall it emitted.
func selectCallAsValue(fc parse.FunctionCall) (aql.Value, error) {
	args := make([]aql.Value, 0, len(fc.Args))
	for _, a := range fc.Args {
		switch v := a.(type) {
		case parse.LiteralExpr:
			args = append(args, v.Value)
		case parse.PathExpr:
			args = append(args, aql.Path(v.Raw))
		default:
			return nil, fmt.Errorf("nested argument came back %T", a)
		}
	}
	return aql.Func(fc.Name, args...), nil
}
