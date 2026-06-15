package aql

import (
	"fmt"
	"strings"
)

// WhereExpr is a boolean expression in a WHERE clause. The interface is sealed;
// construct expressions with the comparison helpers ([Eq], [Ne], [Gt], [Ge],
// [Lt], [Le]) and combine them with [And] / [Or].
type WhereExpr interface {
	// expr is the canonical wire form of the predicate.
	expr() string
	// validate reports a malformed predicate (empty path, nil value) so
	// [Builder.Build] can surface it as ErrInvalidQuery instead of panicking
	// or emitting invalid AQL.
	validate() error
}

type comparison struct {
	path string
	op   string
	val  Value
}

func (c comparison) expr() string { return c.path + " " + c.op + " " + c.val.token() }

func (c comparison) validate() error {
	if strings.TrimSpace(c.path) == "" {
		return fmt.Errorf("%w: empty path in %s comparison", ErrInvalidQuery, c.op)
	}
	if c.val == nil {
		return fmt.Errorf("%w: nil value in comparison on %q", ErrInvalidQuery, c.path)
	}
	return nil
}

// Eq is `path = value`.
func Eq(path string, v Value) WhereExpr { return comparison{path: path, op: "=", val: v} }

// Ne is `path != value`.
func Ne(path string, v Value) WhereExpr { return comparison{path: path, op: "!=", val: v} }

// Gt is `path > value`.
func Gt(path string, v Value) WhereExpr { return comparison{path: path, op: ">", val: v} }

// Ge is `path >= value`.
func Ge(path string, v Value) WhereExpr { return comparison{path: path, op: ">=", val: v} }

// Lt is `path < value`.
func Lt(path string, v Value) WhereExpr { return comparison{path: path, op: "<", val: v} }

// Le is `path <= value`.
func Le(path string, v Value) WhereExpr { return comparison{path: path, op: "<=", val: v} }

type junction struct {
	op    string // "AND" or "OR"
	terms []WhereExpr
}

func (j junction) expr() string {
	parts := make([]string, len(j.terms))
	for i, t := range j.terms {
		// Parenthesise a nested OR inside an AND to preserve precedence;
		// a bare comparison or same-operator junction needs no grouping.
		if inner, ok := t.(junction); ok && inner.op == "OR" && j.op == "AND" {
			parts[i] = "(" + t.expr() + ")"
			continue
		}
		parts[i] = t.expr()
	}
	return strings.Join(parts, " "+j.op+" ")
}

func (j junction) validate() error {
	for _, t := range j.terms {
		if err := t.validate(); err != nil {
			return err
		}
	}
	return nil
}

// And joins predicates with AND. nil terms are dropped; a single surviving term
// is returned unchanged; no terms yields nil (a vacuously-true conjunction —
// the builder emits no WHERE rather than invalid AQL).
func And(terms ...WhereExpr) WhereExpr { return junctionOf("AND", terms) }

// Or joins predicates with OR, with the same nil/empty handling as [And].
func Or(terms ...WhereExpr) WhereExpr { return junctionOf("OR", terms) }

func junctionOf(op string, terms []WhereExpr) WhereExpr {
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
		return junction{op: op, terms: kept}
	}
}
