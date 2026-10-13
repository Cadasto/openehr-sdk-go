package rmjsonwalk_test

import (
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/rmjsonwalk"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
)

// TestResolve pins how an object's RM type is read: its own "_type" wins,
// else the declared type applies, and only a concrete RM class can apply. An
// error names why, and the "_type" it read, so testkit/expectedform can
// refuse an input it cannot read.
func TestResolve(t *testing.T) {
	m, err := rmjsonwalk.Default()
	if err != nil {
		t.Fatalf("Default(): %v", err)
	}
	cases := []struct {
		name     string
		typ      string
		hasType  bool
		declared string
		want     string
		wantErr  string // a substring of the error message
	}{
		{name: "own _type", typ: "DV_CODED_TEXT", hasType: true, declared: "DV_TEXT", want: "DV_CODED_TEXT"},
		{name: "own _type where nothing is declared", typ: "DV_TEXT", hasType: true, want: "DV_TEXT"},
		{name: "declared concrete class", declared: "DV_TEXT", want: "DV_TEXT"},
		{name: "_type names no RM class", typ: "C_COMPLEX_OBJECT", hasType: true, wantErr: `_type "C_COMPLEX_OBJECT" names no RM class`},
		{name: "empty _type", typ: "", hasType: true, declared: "DV_TEXT", wantErr: `_type "" names no RM class`},
		{name: "elided behind an unbound parameter", wantErr: "type parameter with no binding"},
		{name: "elided where the declared type is abstract", declared: "DATA_VALUE", wantErr: "declared type DATA_VALUE is abstract"},
		{name: "elided where the declared type is no class", declared: "String", wantErr: `declared type "String" is not an RM class`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := m.Resolve(tc.typ, tc.hasType, tc.declared)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("Resolve(%q, %v, %q) = %q, %v, want an error containing %q", tc.typ, tc.hasType, tc.declared, got, err, tc.wantErr)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Errorf("Resolve(%q, %v, %q) = %q, %v, want %q, nil", tc.typ, tc.hasType, tc.declared, got, err, tc.want)
			}
		})
	}
}

// TestAttribute pins what an attribute lookup returns: the declared type and
// its kind, the binding a DV_INTERVAL slot passes to its bounds, a type
// parameter taking the slot's binding, and a generic payload.
func TestAttribute(t *testing.T) {
	m, err := rmjsonwalk.Default()
	if err != nil {
		t.Fatalf("Default(): %v", err)
	}
	cases := []struct {
		name, rmType, attr, bound string
		want                      rmjsonwalk.Attribute
	}{
		{
			name: "class", rmType: "CODE_PHRASE", attr: "terminology_id",
			want: rmjsonwalk.Attribute{Type: "TERMINOLOGY_ID", Kind: rmjsonwalk.KindClass, DeclaredOn: "CODE_PHRASE"},
		},
		{
			name: "primitive", rmType: "DV_TEXT", attr: "value",
			want: rmjsonwalk.Attribute{Type: "String", Kind: rmjsonwalk.KindPrimitive, DeclaredOn: "DV_TEXT"},
		},
		{
			name: "container", rmType: "DV_TEXT", attr: "mappings",
			want: rmjsonwalk.Attribute{Type: "TERM_MAPPING", Kind: rmjsonwalk.KindClass, Container: true, DeclaredOn: "DV_TEXT"},
		},
		{
			name: "interval slot binding its bounds", rmType: "DV_QUANTITY", attr: "normal_range",
			want: rmjsonwalk.Attribute{Type: "DV_INTERVAL", Kind: rmjsonwalk.KindClass, DeclaredOn: "DV_QUANTITY", Bound: "DV_QUANTITY"},
		},
		{
			name: "bound type parameter", rmType: "DV_INTERVAL", attr: "lower", bound: "DV_QUANTITY",
			want: rmjsonwalk.Attribute{Type: "DV_QUANTITY", Kind: rmjsonwalk.KindClass, DeclaredOn: "Interval"},
		},
		{
			name: "unbound type parameter", rmType: "DV_INTERVAL", attr: "lower",
			want: rmjsonwalk.Attribute{Kind: rmjsonwalk.KindParameter, DeclaredOn: "Interval"},
		},
		{
			name: "generic payload", rmType: "ORIGINAL_VERSION", attr: "data",
			want: rmjsonwalk.Attribute{Kind: rmjsonwalk.KindParameter, DeclaredOn: "ORIGINAL_VERSION", Payload: true},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := m.Attribute(tc.rmType, tc.attr, tc.bound)
			if !ok || got != tc.want {
				t.Errorf("Attribute(%q, %q, %q) = %+v, %v, want %+v, true", tc.rmType, tc.attr, tc.bound, got, ok, tc.want)
			}
		})
	}
	if got, ok := m.Attribute("DV_TEXT", "x-a", ""); ok {
		t.Errorf("Attribute(DV_TEXT, x-a) = %+v, true, want false: DV_TEXT has no such attribute", got)
	}
	if got, ok := m.Attribute("C_COMPLEX_OBJECT", "rm_type_name", ""); ok {
		t.Errorf("Attribute(C_COMPLEX_OBJECT, rm_type_name) = %+v, true, want false: the class is outside the RM", got)
	}
}

// lookupOnly is an rminfo.Lookup with none of the optional capabilities.
type lookupOnly struct{ rminfo.Lookup }

// TestNewNeedsTheRMCapabilities pins that New refuses a Lookup that cannot
// answer the class-graph or absence questions, rather than misread a type.
func TestNewNeedsTheRMCapabilities(t *testing.T) {
	if _, err := rmjsonwalk.New(lookupOnly{rminfo.Default}); err == nil || !strings.Contains(err.Error(), "rminfo.Hierarchy") {
		t.Errorf("New(Lookup without Hierarchy) error = %v, want one naming rminfo.Hierarchy", err)
	}
	if _, err := rmjsonwalk.New(rminfo.New(nil)); err != nil {
		t.Errorf("New(rminfo.New(nil)) error = %v, want nil: it implements both capabilities", err)
	}
}
