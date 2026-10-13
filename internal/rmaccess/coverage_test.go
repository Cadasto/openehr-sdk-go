package rmaccess

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// TestAccess_REQ171_CoversEveryRegisteredClass pins REQ-171 Structural
// access, Acceptance: a test fails when the registry knows a class the
// generated access does not cover. Every class the type registry decodes is
// covered, and every attribute rminfo declares for it, own and inherited, is
// answered by the reader of its kind: a container by multiple, any other
// attribute by single, never by both. A BMM bump that adds a class or an
// attribute fails here until the generator is run.
func TestAccess_REQ171_CoversEveryRegisteredClass(t *testing.T) {
	names := typereg.Default.Names()
	if len(names) < 100 {
		t.Fatalf("typereg.Default.Names() lists %d classes, want the full RM registry (110 at RM 1.2.0)", len(names))
	}
	lister, ok := rminfo.Default.(rminfo.AttributeLister)
	if !ok {
		t.Fatal("rminfo.Default does not list attribute names")
	}
	attrs := 0
	for _, name := range names {
		ctor, _ := typereg.Default.Lookup(name)
		v := ctor()
		if !Covers(v) {
			t.Errorf("Covers(%T) = false for registered %s", v, name)
			continue
		}
		for _, attr := range lister.AttributeNames(name) {
			if rminfo.IsNonStorableAttr(name, attr) {
				continue
			}
			attrs++
			container, ok := rminfo.Default.IsContainer(name, attr)
			if !ok {
				t.Fatalf("rminfo.IsContainer(%s, %q) not found for an attribute AttributeNames lists", name, attr)
			}
			_, _, bySingle := single(v, attr)
			_, _, byMultiple := multiple(v, attr)
			switch {
			case container && !byMultiple:
				t.Errorf("multiple(%T, %q) does not serve the container %s.%s", v, attr, name, attr)
			case !container && !bySingle:
				t.Errorf("single(%T, %q) does not serve the attribute %s.%s", v, attr, name, attr)
			case bySingle && byMultiple:
				t.Errorf("%s.%s is served by both single and multiple", name, attr)
			}
		}
		if _, _, served := single(v, "no_such_attribute"); served {
			t.Errorf("single(%T, %q) serves an attribute the class does not declare", v, "no_such_attribute")
		}
		if _, _, served := multiple(v, "no_such_attribute"); served {
			t.Errorf("multiple(%T, %q) serves an attribute the class does not declare", v, "no_such_attribute")
		}
	}
	if attrs < 500 {
		t.Fatalf("checked %d attributes, want the full RM attribute set; rminfo has gone blind", attrs)
	}
}

// TestAccess_REQ171_NilAndForeignValues pins the answers for a value the
// access does not read: nil, a typed-nil pointer to a registered class, and a
// Go value of no RM class.
func TestAccess_REQ171_NilAndForeignValues(t *testing.T) {
	var typedNil any = (*rm.Composition)(nil)
	cases := []struct {
		name       string
		v          any
		wantCovers bool
	}{
		{name: "nil", v: nil, wantCovers: false},
		{name: "typed-nil pointer to a registered class", v: typedNil, wantCovers: true},
		{name: "a Go string", v: "COMPOSITION", wantCovers: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Covers(tc.v); got != tc.wantCovers {
				t.Errorf("Covers(%#v) = %t, want %t", tc.v, got, tc.wantCovers)
			}
			if v, present := Single(tc.v, "name"); v != nil || present {
				t.Errorf("Single(%#v, %q) = (%v, %t), want (nil, false)", tc.v, "name", v, present)
			}
			if items, ok := Multiple(tc.v, "content"); items != nil || ok {
				t.Errorf("Multiple(%#v, %q) = (%v, %t), want (nil, false)", tc.v, "content", items, ok)
			}
		})
	}
	if !IsTypedNilPointer(typedNil) || IsTypedNilPointer(nil) {
		t.Errorf("IsTypedNilPointer: typed nil = %t, nil = %t; want true, false", IsTypedNilPointer(typedNil), IsTypedNilPointer(nil))
	}
}
