package ehr

import (
	"errors"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// REQ-094
func TestHasResource(t *testing.T) {
	var none *rm.Composition
	if HasResource(none) {
		t.Fatal("typed-nil *Composition")
	}
	if !HasResource(&rm.Composition{}) {
		t.Fatal("non-nil *Composition")
	}
	var party rm.Party
	if HasResource(party) {
		t.Fatal("bare-nil Party")
	}
	var person *rm.Person
	party = person
	if HasResource(party) {
		t.Fatal("Party holding typed-nil *Person")
	}
}

// REQ-094
func TestNoRepresentationErrorAs(t *testing.T) {
	meta := &VersionMetadata{VersionUID: "uid::system::1"}
	err := &NoRepresentationError{Meta: meta, Cause: transport.ErrInvalidShape}
	var got *NoRepresentationError
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
