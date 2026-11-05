package templatecompile_test

// parity_test.go: REQ-111, the exception fixtures: how
// TestREQ111_ParityExceptionsRefused tells the refusal of each reading the
// new path does not keep from another refusal, by its error alone.

import (
	"errors"
	"fmt"
	"strconv"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// TestREQ111_ParityRefusalsByError pins how the parity-exceptions check tells
// a refusal the rule allows from one it does not, by its error: an integer
// past the 32-bit range is the reader's ParseInt range refusal, a
// match_negated of true is the reader's refusal of that element, an infinite
// integer bound that admits no integer is the reader's ParseInt syntax
// refusal of a lower bound of positive infinity or an upper one of negative
// infinity, a definition read as a slot or a primitive constraint is the
// reader's type mismatch refusal of the definition, a shared AQL path is the
// compile step's duplicate-path refusal, and a definition that names no RM
// type is the compile step's refusal of it. A refusal that shares part of one
// of these, such as a real out of range, another element's refusal, an
// infinity on the side it admits every integer or a type mismatch below the
// definition, is another refusal.
func TestREQ111_ParityRefusalsByError(t *testing.T) {
	mismatch := func(typ string) error {
		return &canxml.DecodeError{Path: "/definition", Type: typ, Inner: typereg.ErrTypeMismatch}
	}
	shape := func(path string, inner error) error {
		return &canxml.DecodeError{Path: "/attributes", Type: "C_SINGLE_ATTRIBUTE", Inner: &canxml.DecodeError{Path: path, Inner: inner}}
	}
	rangeErr := func(fn string) error {
		return fmt.Errorf("%w: %w", canxml.ErrInvalidShape, &strconv.NumError{Func: fn, Num: "5000000000", Err: strconv.ErrRange})
	}
	syntaxErr := func(num string) error {
		return fmt.Errorf("%w: %w", canxml.ErrInvalidShape, &strconv.NumError{Func: "ParseInt", Num: num, Err: strconv.ErrSyntax})
	}
	readerCases := []struct {
		name string
		err  error
		want exceptionRefusal
	}{
		{name: "an integer past the 32-bit range", err: shape("/upper", rangeErr("ParseInt")), want: refusedLargeInteger},
		{name: "a match_negated of true", err: shape("/match_negated", fmt.Errorf("%w: the element holds a value other than the one it may hold", canxml.ErrInvalidShape)), want: refusedMatchNegated},
		{name: "a real out of range", err: shape("/upper", rangeErr("ParseFloat")), want: refusedOther},
		{name: "an integer that is no number", err: shape("/upper", syntaxErr("x")), want: refusedOther},
		{name: "a range refusal that is no shape refusal", err: &strconv.NumError{Func: "ParseInt", Num: "5000000000", Err: strconv.ErrRange}, want: refusedOther},
		{name: "another element's refusal", err: shape("/is_ordered", canxml.ErrInvalidShape), want: refusedOther},
		{name: "a refusal inside a match_negated's parent", err: &canxml.DecodeError{Path: "/match_negated", Inner: &canxml.DecodeError{Path: "/value", Inner: canxml.ErrInvalidShape}}, want: refusedOther},
		{name: "a match_negated that is no shape refusal", err: &canxml.DecodeError{Path: "/match_negated", Inner: errors.New("read failed")}, want: refusedOther},
		{name: "a lower bound of positive infinity", err: shape("/lower", syntaxErr("+Inf")), want: refusedInfiniteBound},
		{name: "an upper bound of negative infinity", err: shape("/upper", syntaxErr("-Infinity")), want: refusedInfiniteBound},
		{name: "a lower bound of negative infinity, which admits every integer", err: shape("/lower", syntaxErr("-Inf")), want: refusedOther},
		{name: "an upper bound of positive infinity, which admits every integer", err: shape("/upper", syntaxErr("INF")), want: refusedOther},
		{name: "a list entry of positive infinity", err: shape("/list", syntaxErr("+Inf")), want: refusedOther},
		{name: "a lower bound past the range of a float", err: shape("/lower", syntaxErr("1e400")), want: refusedOther},
		{name: "a real lower bound of positive infinity", err: shape("/lower", fmt.Errorf("%w: %w", canxml.ErrInvalidShape, &strconv.NumError{Func: "ParseFloat", Num: "+Inf", Err: strconv.ErrSyntax})), want: refusedOther},
		{name: "an infinite bound that is no shape refusal", err: &canxml.DecodeError{Path: "/lower", Inner: &strconv.NumError{Func: "ParseInt", Num: "+Inf", Err: strconv.ErrSyntax}}, want: refusedOther},
		{name: "a definition read as a slot", err: mismatch("ARCHETYPE_SLOT"), want: refusedDefinitionNotKept},
		{name: "a definition read as a string constraint", err: mismatch("C_STRING"), want: refusedDefinitionNotKept},
		{name: "a definition of an unknown type", err: &canxml.DecodeError{Path: "/definition", Type: "C_FUTURE", Inner: typereg.ErrUnknownType}, want: refusedOther},
		{name: "a type mismatch below the definition", err: &canxml.DecodeError{Path: "/definition", Inner: &canxml.DecodeError{Path: "/attributes", Type: "C_STRING", Inner: typereg.ErrTypeMismatch}}, want: refusedOther},
		{name: "a type mismatch of another element", err: &canxml.DecodeError{Path: "/children", Type: "C_STRING", Inner: typereg.ErrTypeMismatch}, want: refusedOther},
		{name: "a type mismatch that is no decode error", err: fmt.Errorf("definition: %w", typereg.ErrTypeMismatch), want: refusedOther},
	}
	for _, tc := range readerCases {
		if got := readRefusalOf(tc.err); got != tc.want {
			t.Errorf("readRefusalOf(%s: %v) = %q; want %q", tc.name, tc.err, got, tc.want)
		}
	}

	noRMType := fmt.Errorf("%w: template definition names no RM type", templatecompile.ErrInvalidInput)
	compileCases := []struct {
		name string
		err  error
		want exceptionRefusal
	}{
		{name: "a shared AQL path", err: fmt.Errorf(`%w: duplicate AQL path "/category" (existing DV_CODED_TEXT, new DV_CODED_TEXT)`, templatecompile.ErrInvalidInput), want: refusedSharedPath},
		{name: "another invalid input", err: fmt.Errorf("%w: template has no template_id", templatecompile.ErrInvalidInput), want: refusedOther},
		{name: "a shared AQL path that is no invalid input", err: errors.New(`templatecompile: duplicate AQL path "/category"`), want: refusedOther},
		{name: "a definition that names no RM type", err: noRMType, want: refusedNoRMType},
		{name: "a definition that names no RM type that is no invalid input", err: errors.New("templatecompile: invalid input: template definition names no RM type"), want: refusedOther},
		{name: "a node below the definition that names no RM type", err: fmt.Errorf("%w: at /content: template definition names no RM type", templatecompile.ErrInvalidInput), want: refusedOther},
		{name: "a definition refusal wrapped in another", err: fmt.Errorf("definition: %w", noRMType), want: refusedOther},
	}
	for _, tc := range compileCases {
		if got := compileRefusalOf(tc.err); got != tc.want {
			t.Errorf("compileRefusalOf(%s: %v) = %q; want %q", tc.name, tc.err, got, tc.want)
		}
	}
}
