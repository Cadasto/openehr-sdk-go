package validation

// rmfloor_primitive_list_internal_test.go: REQ-171 Structural access and
// REQ-112 Driver on a list of a primitive type that the RM makes mandatory:
// the walk reports it absent or empty without reading its items.

import (
	"runtime"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// TestEngine_REQ112_REQ171_MandatoryPrimitiveListCounted pins REQ-112 Driver
// on an RM-mandatory list of a primitive type. No list of a primitive type is
// RM-mandatory in the pinned BMMs, so the attribute is built: DV_MULTIMEDIA
// data, an Array of Octet, read as the walk reads it and marked mandatory.
// Absent (a nil list) it draws `required`, rule mandatory, and present and
// empty it draws `cardinality`, rule cardinality, both at the attribute's
// path. Present with a mebibyte of items it draws nothing at that path, and
// the visit allocates less than the payload's size, so the walk learns
// whether the list is there and empty without reading its items (REQ-171
// Structural access).
func TestEngine_REQ112_REQ171_MandatoryPrimitiveListCounted(t *testing.T) {
	walked := walkedNamed(classAttributes["DV_MULTIMEDIA"], "data")
	if walked == nil {
		t.Fatal("the class table walks no DV_MULTIMEDIA data")
	}
	data := walked.inRole(roleNone)
	if data.shape != shapeCounted || data.mandatory {
		t.Fatalf("precondition: DV_MULTIMEDIA data reads as shape %d, mandatory %t; want a counted list the pinned RM leaves optional", data.shape, data.mandatory)
	}
	data.mandatory = true
	visit := func(payload []byte) []string {
		e := newEngine(Limits{})
		e.visitAttribute(&engineNode{
			value: &rm.DVMultimedia{
				MediaType: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "IANA_media-types"}, CodeString: "image/png"},
				Size:      rm.Integer(len(payload)),
				Data:      payload,
			},
			rmType: "DV_MULTIMEDIA",
		}, "DV_MULTIMEDIA", &data)
		var got []string
		for _, i := range e.found.issues {
			got = append(got, i.Code+" "+i.Path+" "+i.Layer.String()+" "+i.Rule)
		}
		return got
	}
	for _, tc := range []struct {
		name string
		data []byte
		want []string
	}{
		{name: "absent data", data: nil, want: []string{"required /data rm mandatory"}},
		{name: "present and empty data", data: []byte{}, want: []string{"cardinality /data rm cardinality"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := visit(tc.data); !slices.Equal(got, tc.want) {
				t.Errorf("visit of DV_MULTIMEDIA with %s, data RM-mandatory: issues = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
	const size = 1 << 20
	payload := make([]byte, size)
	var got []string
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	got = visit(payload)
	runtime.ReadMemStats(&after)
	if len(got) != 0 {
		t.Errorf("visit of DV_MULTIMEDIA with %d bytes of data, data RM-mandatory: issues = %q, want none", size, got)
	}
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated >= size {
		t.Errorf("visit of DV_MULTIMEDIA with %d bytes of data, data RM-mandatory: allocated %d bytes, want fewer than %d: the bytes are read one item at a time", size, allocated, size)
	}
}
