package ehr_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// REQ-094
func TestHasResource(t *testing.T) {
	var none *rm.Composition
	if openehrclient.HasResource(none) {
		t.Fatal("typed-nil *Composition")
	}
	if !openehrclient.HasResource(&rm.Composition{}) {
		t.Fatal("non-nil *Composition")
	}
	var party rm.Party
	if openehrclient.HasResource(party) {
		t.Fatal("bare-nil Party")
	}
	var person *rm.Person
	party = person
	if openehrclient.HasResource(party) {
		t.Fatal("Party holding typed-nil *Person")
	}
	party = &rm.Person{}
	if !openehrclient.HasResource(party) {
		t.Fatal("Party holding live *Person")
	}
}

// REQ-094: the registry scope is the contract — a typed nil of a type
// OUTSIDE the RM registry reads as present (rm.IsTypedNil's closed
// switch defaults to false), and a non-pointer RM value is present.
// These pin the documented gap so a regeneration cannot silently
// invert it.
func TestHasResourceRegistryScope(t *testing.T) {
	type outside struct{ X int }
	if !openehrclient.HasResource((*outside)(nil)) {
		t.Fatal("out-of-registry typed nil must read as present (documented scope)")
	}
	if !openehrclient.HasResource(rm.Composition{}) {
		t.Fatal("value-typed RM struct is present")
	}
}

// REQ-094: HasResource compares only against untyped nil, so an
// uncomparable T must not panic (the godoc's own constraint).
func TestHasResourceUncomparable(t *testing.T) {
	if !openehrclient.HasResource([]int{1}) {
		t.Fatal("non-nil slice is present")
	}
	if !openehrclient.HasResource(map[string]int{}) {
		t.Fatal("non-nil map is present")
	}
}

// REQ-094 / REQ-093: Error() is value-free — the classification only,
// never the Cause text — and the nil-receiver arms of Error/Unwrap are
// load-bearing (a typed-nil *NoRepresentationError must not panic under
// %v or an errors.Is/As chain walk).
func TestNoRepresentationErrorStrings(t *testing.T) {
	var nilErr *openehrclient.NoRepresentationError
	if got := nilErr.Error(); got != "ehr: no representation" {
		t.Errorf("nil receiver Error() = %q", got)
	}
	if nilErr.Unwrap() != nil {
		t.Error("nil receiver Unwrap() must be nil")
	}
	if got := (&openehrclient.NoRepresentationError{}).Error(); got != "ehr: committed write has no usable representation" {
		t.Errorf("nil-Cause Error() = %q", got)
	}
	empty := &openehrclient.NoRepresentationError{
		Cause: fmt.Errorf("composition: %w: Prefer=return=representation but response body is empty", transport.ErrInvalidShape),
	}
	if got := empty.Error(); got != "ehr: committed write has no usable representation (empty body)" {
		t.Errorf("empty-body Error() = %q", got)
	}
	decode := &openehrclient.NoRepresentationError{Cause: errors.New(`parse "secret-payload-value"`)}
	if got := decode.Error(); strings.Contains(got, "secret-payload-value") {
		t.Errorf("Error() leaked Cause text: %q", got)
	} else if got != "ehr: committed write has no usable representation (decode failed)" {
		t.Errorf("decode-failure Error() = %q", got)
	}
}

// REQ-094
func TestNoRepresentationErrorAs(t *testing.T) {
	meta := &openehrclient.VersionMetadata{VersionUID: "uid::system::1"}
	err := &openehrclient.NoRepresentationError{Meta: meta, Cause: transport.ErrInvalidShape}
	var got *openehrclient.NoRepresentationError
	if !errors.As(err, &got) {
		t.Fatal("errors.As")
	}
	if got.Meta.VersionUID != meta.VersionUID {
		t.Fatalf("uid = %q", got.Meta.VersionUID)
	}
	if !errors.Is(err, transport.ErrInvalidShape) {
		t.Fatal("unwrap Cause")
	}
	if _, ok := errors.AsType[*transport.WireError](err); ok {
		t.Fatal("must not look like WireError")
	}
}
