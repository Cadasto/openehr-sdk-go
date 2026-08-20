package ehr_test

import (
	"errors"
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
