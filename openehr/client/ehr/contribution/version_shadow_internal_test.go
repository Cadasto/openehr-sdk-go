package contribution

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// TestShadowMarshallersCoverTheGeneratedKeySet turns the BMM-BUMP comment on
// originalVersionJSON / importedVersionJSON into a check.
//
// Those two structs hand-copy the generated rm marshallers so the write path
// can replace `commit_audit` with the [UpdateAudit] DTO and omit the
// server-assigned fields (REQ-130). Hand-copies rot: if `bmmgen` adds a
// member to ORIGINAL_VERSION or IMPORTED_VERSION, the shadow silently drops
// it from every submission body and nothing else notices — the generated
// marshaller is not on the write path, so its own tests stay green.
//
// The comparison is over JSON **key names**, not struct tags or Go types,
// because the shadows differ from the generated form by design: `contribution`
// and `uid` carry `omitempty`, `uid` is a pointer so an unset one is omitted,
// and `commit_audit` is a different type. A key on either side alone is drift.
//
// Reflection is confined to this test. REQ-024 bars it from the library
// surface, not from a guard that reads two struct definitions at build time.
func TestShadowMarshallersCoverTheGeneratedKeySet(t *testing.T) {
	cases := []struct {
		name      string
		generated any
		shadow    any
	}{
		{
			name:      "ORIGINAL_VERSION",
			generated: rm.OriginalVersionJSONMarshaller[rm.Composition]{},
			shadow:    originalVersionJSON[rm.Composition]{},
		},
		{
			name:      "IMPORTED_VERSION",
			generated: rm.ImportedVersionJSONMarshaller[rm.Composition]{},
			shadow:    importedVersionJSON[rm.Composition]{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := jsonKeys(reflect.TypeOf(tc.generated))
			got := jsonKeys(reflect.TypeOf(tc.shadow))
			for _, k := range want {
				if !slices.Contains(got, k) {
					t.Errorf("shadow is missing %q — bmmgen changed %s and the write-side copy did not follow (see the BMM-BUMP note in version.go)", k, tc.name)
				}
			}
			for _, k := range got {
				if !slices.Contains(want, k) {
					t.Errorf("shadow emits %q, which the generated %s marshaller does not declare", k, tc.name)
				}
			}
		})
	}
}

// jsonKeys returns the JSON key of every field of a struct type, ignoring the
// tag options (`omitempty` and friends) that the shadows deliberately differ
// on. A field tagged "-" is skipped.
func jsonKeys(t reflect.Type) []string {
	keys := make([]string, 0, t.NumField())
	for f := range t.Fields() {
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		switch name {
		case "", "-":
			continue
		}
		keys = append(keys, name)
	}
	slices.Sort(keys)
	return keys
}
