package aql

// builder_internal_test.go reaches the unexported builder ast to pin guards on
// fields the public API cannot put into an out-of-catalogue state. The public
// surface is covered from aql_test (see builder_test.go / paging_test.go); only
// checks that need the internal seam live here.
// REQ-117 · PROBE-088

import (
	"errors"
	"strings"
	"testing"
)

// TestValidateLimitValueRefusesUnknownShape pins the fail-closed default on the
// in-text paging operand: the grammar's `limitValue : INTEGER | PARAMETER`, so
// any other [Value] must be refused by name rather than emitted as
// ungrammatical AQL. The public setters ([Builder.LimitInline] etc.) only ever
// store an IntValue / ParamValue, so the exotic shape is reachable only through
// this internal seam — the guard exists so a future setter cannot regress it.
func TestValidateLimitValueRefusesUnknownShape(t *testing.T) {
	for _, tc := range []struct {
		name string
		v    Value
	}{
		{name: "string", v: StringValue{S: "10"}},
		{name: "real", v: RealValue{F: 1.5}},
		{name: "bool", v: BoolValue{B: true}},
		{name: "null", v: NullValue{}},
		{name: "path", v: PathValue{IdentifiedPath: IdentifiedPath{Raw: "o/x"}}},
		{name: "func", v: FuncCall{Name: "LENGTH", Args: []Value{StringValue{S: "x"}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateLimitValue("LIMIT", tc.v)
			if !errors.Is(err, ErrInvalidQuery) {
				t.Fatalf("validateLimitValue(%#v) = %v, want ErrInvalidQuery", tc.v, err)
			}
			// The error must name the offending type so the refusal is
			// diagnosable.
			if want := "LIMIT"; !strings.Contains(err.Error(), want) {
				t.Errorf("error does not name the clause %q: %v", want, err)
			}

			// …and the refusal must reach Build(), not just the helper.
			b := NewBuilder().Select(Col("o/x")).From("OBSERVATION", "o")
			b.ast.limitInline = tc.v
			if _, berr := b.Build(); !errors.Is(berr, ErrInvalidQuery) {
				t.Errorf("Build with an exotic in-text LIMIT = %v, want ErrInvalidQuery", berr)
			}
		})
	}

	// A nil operand means the clause is absent — not an error.
	if err := validateLimitValue("OFFSET", nil); err != nil {
		t.Errorf("validateLimitValue(nil) = %v, want nil (absent clause)", err)
	}
}
