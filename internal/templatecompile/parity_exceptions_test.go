package templatecompile_test

// parity_exceptions_test.go: REQ-111 § Recorded compiled trees, the exception
// fixtures. Each operational template of testdata/parity-exceptions holds
// one of the readings the model's reader refuses, or one of the compile
// step's own grounds, and the reader or the compile step, each in its default
// mode, refuses it with the error of that reading, told apart by the error
// alone.

import (
	"bytes"
	"errors"
	"maps"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// parityExceptionsDir holds one operational template for each reading the
// reader refuses and each ground the compile step refuses on its own.
const parityExceptionsDir = "testdata/parity-exceptions"

// exceptionRefusal is what the reader and the compile step make of an
// operational template: one of the refusals the exception fixtures name,
// another refusal, or none.
type exceptionRefusal string

const (
	refusedLargeInteger      exceptionRefusal = "an integer the model cannot hold"
	refusedMatchNegated      exceptionRefusal = "a match_negated of true"
	refusedInfiniteBound     exceptionRefusal = "an infinite integer bound that admits no integer"
	refusedDefinitionNotKept exceptionRefusal = "a definition read as a slot or a primitive constraint"
	refusedSharedPath        exceptionRefusal = "two nodes under different attributes share an AQL path"
	refusedNoRMType          exceptionRefusal = "a definition that names no RM type"
	refusedOther             exceptionRefusal = "a refusal the exception fixtures do not name"
	notRefused               exceptionRefusal = "no refusal: the reader and the compile step read and compile it"
)

// allowedRefusals are the refusals the exception fixtures name.
var allowedRefusals = []exceptionRefusal{
	refusedLargeInteger, refusedMatchNegated, refusedInfiniteBound,
	refusedDefinitionNotKept, refusedSharedPath, refusedNoRMType,
}

// TestREQ111_ParityExceptionsRefused holds each operational template of
// testdata/parity-exceptions to the refusal of the reading it holds: the
// model's reader and the compile step, each in its default mode, refuse it
// with that reading's error. The table lists every file of the folder, and
// each refusal the exception fixtures name has a file.
func TestREQ111_ParityExceptionsRefused(t *testing.T) {
	want := map[string]exceptionRefusal{
		"definition_archetype_slot.opt":       refusedDefinitionNotKept,
		"definition_no_rm_type.opt":           refusedNoRMType,
		"definition_primitive_constraint.opt": refusedDefinitionNotKept,
		"infinite_integer_bound.opt":          refusedInfiniteBound,
		"integer_past_32_bits.opt":            refusedLargeInteger,
		"match_negated_true.opt":              refusedMatchNegated,
		"shared_aql_path.opt":                 refusedSharedPath,
	}
	entries, err := os.ReadDir(parityExceptionsDir)
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() {
			files = append(files, e.Name())
		}
	}
	if listed := slices.Sorted(maps.Keys(want)); !slices.Equal(files, listed) {
		t.Fatalf("%s holds %q; the table lists %q", parityExceptionsDir, files, listed)
	}
	for _, r := range allowedRefusals {
		if !slices.Contains(slices.Collect(maps.Values(want)), r) {
			t.Errorf("no file of %s is refused as %q; want one for each refusal the exception fixtures name", parityExceptionsDir, r)
		}
	}

	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(parityExceptionsDir, file))
			if err != nil {
				t.Fatal(err)
			}
			if got, err := refusalOf(raw); got != want[file] {
				t.Errorf("refusalOf(%s) = %q (%v); want %q", file, got, err, want[file])
			}
		})
	}
}

// refusalOf reads raw with the model's reader and compiles it, each in its
// default mode, and returns which refusal stops it, told by its error, with
// that error.
func refusalOf(raw []byte) (exceptionRefusal, error) {
	model, err := aom14.ReadOperationalTemplate(bytes.NewReader(raw))
	if err != nil {
		return readRefusalOf(err), err
	}
	if _, err := templatecompile.Compile(model); err != nil {
		return compileRefusalOf(err), err
	}
	return notRefused, nil
}

// readRefusalOf tells which reader refusal err is: a definition read as a
// slot or a primitive constraint, the type mismatch refusal of the
// definition itself; and, among the shape refusals, an integer the model's
// 32-bit Integer cannot hold, the ParseInt range refusal; an infinite integer
// bound that admits no integer, the ParseInt syntax refusal of a lower bound
// of positive infinity or an upper one of negative infinity; a match_negated
// of true, the refusal of that element; or another.
func readRefusalOf(err error) exceptionRefusal {
	de := innermostDecode(err)
	if de != nil && de.Path == "/definition" && errors.Is(de.Inner, typereg.ErrTypeMismatch) {
		return refusedDefinitionNotKept
	}
	if !errors.Is(err, canxml.ErrInvalidShape) {
		return refusedOther
	}
	if ne, ok := errors.AsType[*strconv.NumError](err); ok && ne != nil && ne.Func == "ParseInt" {
		switch {
		case errors.Is(ne, strconv.ErrRange):
			return refusedLargeInteger
		case errors.Is(ne, strconv.ErrSyntax) && de != nil && boundAdmitsNoInteger(de.Path, ne.Num):
			return refusedInfiniteBound
		}
	}
	if de != nil && de.Path == "/match_negated" {
		return refusedMatchNegated
	}
	return refusedOther
}

// boundAdmitsNoInteger reports whether text, the integer bound at path, is
// an infinity no integer lies on the inner side of: positive infinity at
// "/lower" or negative infinity at "/upper", as strconv.ParseFloat reads it.
func boundAdmitsNoInteger(path, text string) bool {
	f, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	if err != nil {
		return false
	}
	switch path {
	case "/lower":
		return math.IsInf(f, 1)
	case "/upper":
		return math.IsInf(f, -1)
	}
	return false
}

// innermostDecode returns the innermost decode error in err's chain, the
// element the decode failed on, or nil when err holds none.
func innermostDecode(err error) *canxml.DecodeError {
	var inner *canxml.DecodeError
	for {
		de, ok := errors.AsType[*canxml.DecodeError](err)
		if !ok || de == nil {
			return inner
		}
		inner, err = de, de.Inner
	}
}

// compileRefusalOf tells which compile refusal err is: two nodes under
// different attributes that share an AQL path; a definition that names no
// RM type; or another.
func compileRefusalOf(err error) exceptionRefusal {
	if !errors.Is(err, templatecompile.ErrInvalidInput) {
		return refusedOther
	}
	if strings.Contains(err.Error(), ": duplicate AQL path ") {
		return refusedSharedPath
	}
	if msg, ok := strings.CutPrefix(err.Error(), templatecompile.ErrInvalidInput.Error()+": "); ok && msg == "template definition names no RM type" {
		return refusedNoRMType
	}
	return refusedOther
}
