package canxml_test

// typed_nil_bound_test.go: an interval bound typed by an interface whose
// receiver already holds a typed-nil pointer. REQ-052 reads such a bound as
// absent, so the decode fills the slot through `xsi:type` dispatch as it
// fills a nil one; library code never panics on it (AGENTS.md).

import (
	"errors"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// typedNilDecodes runs decode once per way a canonical-XML decode can run:
// Unmarshal in each mode, and a Decoder with and without options. decode gets
// the name of the run and the function that decodes a document into v.
func typedNilDecodes(t *testing.T, decode func(t *testing.T, run func(in string, v any) error)) {
	t.Helper()
	for mode, opts := range xmlDecodeModes() {
		t.Run("Unmarshal "+mode, func(t *testing.T) {
			decode(t, func(in string, v any) error { return canxml.Unmarshal([]byte(in), v, opts...) })
		})
		t.Run("Decoder "+mode, func(t *testing.T) {
			decode(t, func(in string, v any) error { return canxml.NewDecoder(strings.NewReader(in), opts...).Decode(v) })
		})
	}
}

// TestREQ052_XMLTypedNilBoundDispatches pins a receiver whose interval bound,
// typed by an interface, holds a typed-nil pointer: the decode reads the
// bound as empty and dispatches on `xsi:type`, so a DV_QUANTITY lower bound
// decodes into a fresh quantity and a bound with no `xsi:type` fails as a
// missing discriminator, as for a nil bound, in every mode.
//
// Can-fail control: hand the typed-nil to encoding/xml as a held value, as
// before, and the DV_QUANTITY case panics inside DVQuantity.UnmarshalXML on
// its nil receiver, while the Character case fails with
// typereg.ErrNilReceiver classified as a primitive failure.
func TestREQ052_XMLTypedNilBoundDispatches(t *testing.T) {
	t.Run("DV_QUANTITY into a typed-nil DV_ORDERED bound", func(t *testing.T) {
		const in = `<iv ` + ucNS + `><lower xsi:type="DV_QUANTITY"><magnitude>1.5</magnitude><units>mm</units></lower></iv>`
		typedNilDecodes(t, func(t *testing.T, run func(in string, v any) error) {
			iv := &rm.Interval[rm.DVOrdered]{Lower: (*rm.DVQuantity)(nil)}
			if err := run(in, iv); err != nil {
				t.Fatalf("decode = %v; want nil", err)
			}
			q, ok := iv.Lower.(*rm.DVQuantity)
			if !ok || q == nil || q.Magnitude != 1.5 || q.Units != "mm" {
				t.Errorf("Lower = %#v; want a *rm.DVQuantity of 1.5 mm", iv.Lower)
			}
		})
	})

	t.Run("no xsi:type into a typed-nil Character bound", func(t *testing.T) {
		const in = `<iv ` + ucNS + `><lower>J</lower></iv>`
		typedNilDecodes(t, func(t *testing.T, run func(in string, v any) error) {
			err := run(in, &rm.Interval[any]{Lower: (*rm.Character)(nil)})
			if !errors.Is(err, typereg.ErrMissingType) {
				t.Errorf("decode err = %v; want errors.Is(_, typereg.ErrMissingType), as for a nil bound", err)
			}
			if errors.Is(err, typereg.ErrNilReceiver) || errors.Is(err, canxml.ErrInvalidShape) {
				t.Errorf("decode err = %v; want neither typereg.ErrNilReceiver nor canxml.ErrInvalidShape: the typed nil is no value to decode into", err)
			}
		})
	})
}
