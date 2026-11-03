package bmmgen

// REQ-176 § Shapes the pinned BMM lacks, the integer-enumeration bullet, and
// REQ-041 § Mapping rules: the generator gives every integer enumeration it
// emits, and no string enumeration, the canonical codecs that write the code
// and read the code or the item name. The behaviour is pinned on the AOM 1.4
// types in openehr/aom/aom14/integer_enumeration_test.go; these tests pin
// which enumerations get the codecs, in which file.

import (
	"regexp"
	"strings"
	"testing"
)

// TestRenderIntegerEnumFile_REQ176_REQ041_EveryIntegerEnumeration pins that
// integer_enumeration_gen.go holds the item table and the four codec methods
// of each integer enumeration a target emits, the RM one included, and names no
// string enumeration; and that a target with no integer enumeration gets no
// file.
func TestRenderIntegerEnumFile_REQ176_REQ041_EveryIntegerEnumeration(t *testing.T) {
	for _, tc := range []struct {
		target   Target
		integers []string
		strings  []string
	}{
		{TargetRM, []string{"ProportionKind"}, []string{"ValidityKind", "VersionStatus"}},
		{TargetAOM14, []string{"OperatorKind", "TimezoneValidityKind"}, nil},
	} {
		t.Run(tc.target.GoPackage, func(t *testing.T) {
			plan, err := BuildPlanForTarget(t.Context(), tc.target, testResolver())
			if err != nil {
				t.Fatalf("BuildPlanForTarget(%s) = %v", tc.target.RootID, err)
			}
			body, err := RenderIntegerEnumFile(plan)
			if err != nil {
				t.Fatalf("RenderIntegerEnumFile(%s) = %v", tc.target.GoPackage, err)
			}
			src := string(body)
			for _, name := range tc.integers {
				for _, want := range []string{
					"var itemsOf" + name + " = []integerEnumItem[" + name + "]{",
					"func (k " + name + ") MarshalText() ([]byte, error) {",
					"func (k *" + name + ") UnmarshalText(text []byte) error {",
					"func (k " + name + ") MarshalJSONTo(enc *jsontext.Encoder) error {",
					"func (k *" + name + ") UnmarshalJSONFrom(dec *jsontext.Decoder) error {",
				} {
					if !strings.Contains(src, want) {
						t.Errorf("%s %s does not hold %q", tc.target.GoPackage, integerEnumFile, want)
					}
				}
			}
			methods := regexp.MustCompile(`func \(k \*?(\w+)\)`)
			for _, m := range methods.FindAllStringSubmatch(src, -1) {
				if !strings.Contains(strings.Join(tc.integers, " "), m[1]) {
					t.Errorf("%s %s gives %s a codec method; it is no integer enumeration the target emits", tc.target.GoPackage, integerEnumFile, m[1])
				}
			}
			for _, name := range tc.strings {
				if strings.Contains(src, name) {
					t.Errorf("%s %s names the string enumeration %s", tc.target.GoPackage, integerEnumFile, name)
				}
			}
		})
	}

	body, err := RenderIntegerEnumFile(extensionPlan(t, extensionRootID))
	if err != nil || body != nil {
		t.Errorf("RenderIntegerEnumFile(a target with no integer enumeration) = %d bytes, %v; want nil, nil", len(body), err)
	}
}

// TestRenderIntegerEnumFile_REQ176_ItemsShareACode pins that the item table
// lists every item, two that share a code included, so a decode reads both
// names: TIMEZONE_VALIDITY_KIND's prohibited and disallowed are both 1003.
func TestRenderIntegerEnumFile_REQ176_ItemsShareACode(t *testing.T) {
	plan, err := BuildPlanForTarget(t.Context(), TargetAOM14, testResolver())
	if err != nil {
		t.Fatalf("BuildPlanForTarget(AOM14) = %v", err)
	}
	body, err := RenderIntegerEnumFile(plan)
	if err != nil {
		t.Fatalf("RenderIntegerEnumFile(AOM14) = %v", err)
	}
	for _, want := range []string{
		`{"prohibited", TimezoneValidityKindProhibited},`,
		`{"disallowed", TimezoneValidityKindDisallowed},`,
	} {
		if !strings.Contains(string(body), want) {
			t.Errorf("AOM 1.4 %s does not list %s", integerEnumFile, want)
		}
	}
}
