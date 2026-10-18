package composition_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// TestBuilderNotMadeByNewBuilder checks that a Builder NewBuilder did not
// make, the zero value or a nil pointer, returns an error from Set, from
// each typed setter and from Build, and does not panic; Build returns no
// composition with it. A nil pointer keeps its own error text.
func TestBuilderNotMadeByNewBuilder(t *testing.T) {
	t.Parallel()
	const path = "/context/other_context/items[at0002]/value"
	cases := []struct {
		name    string
		b       *composition.Builder
		wantErr string
	}{
		{name: "zero value", b: &composition.Builder{}, wantErr: "composition: Builder not made by NewBuilder"},
		{name: "nil pointer", b: nil, wantErr: "composition: nil Builder"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			setters := map[string]func() error{
				"Set":           func() error { return tc.b.Set(path, &rm.DVText{Value: "x"}) },
				"SetText":       func() error { return tc.b.SetText(path, "x") },
				"SetQuantity":   func() error { return tc.b.SetQuantity(path, 1, "mm") },
				"SetCodedText":  func() error { return tc.b.SetCodedText(path, "local", "at0001", "x") },
				"Set nil value": func() error { return tc.b.Set(path, nil) },
			}
			for name, set := range setters {
				if err := set(); err == nil || err.Error() != tc.wantErr {
					t.Errorf("%s on a %s Builder = %v, want %q", name, tc.name, err, tc.wantErr)
				}
			}
			comp, err := tc.b.Build()
			if err == nil || err.Error() != tc.wantErr {
				t.Errorf("Build on a %s Builder = %v, want %q", tc.name, err, tc.wantErr)
			}
			if comp != nil {
				t.Errorf("Build on a %s Builder returned a composition with the error, want nil", tc.name)
			}
			if got := tc.b.TemplateID(); got != "" {
				t.Errorf("TemplateID on a %s Builder = %q, want empty", tc.name, got)
			}
		})
	}
}
