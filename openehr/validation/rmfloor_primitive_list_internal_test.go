package validation

// rmfloor_primitive_list_internal_test.go: REQ-171 Structural access and
// REQ-112 Driver on a list of a primitive type that the RM makes mandatory:
// the floor reports it absent or empty without reading its items.

import (
	"runtime"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
)

// mandatoryDataLookup answers as rminfo.Default does, except that it makes
// DV_MULTIMEDIA data, an Array of Octet, RM-mandatory. No list of a primitive
// type is RM-mandatory in the pinned BMMs, so the case can only be built.
type mandatoryDataLookup struct {
	rminfo.Lookup
}

func (l mandatoryDataLookup) RequiredAttributes(rmType string) []string {
	req := l.Lookup.RequiredAttributes(rmType)
	if rmType == "DV_MULTIMEDIA" {
		return append(slices.Clone(req), "data")
	}
	return req
}

func (l mandatoryDataLookup) AttributeNames(rmType string) []string {
	return l.Lookup.(rminfo.AttributeLister).AttributeNames(rmType)
}

func (l mandatoryDataLookup) AbsenceReason(rmType string) rminfo.AbsenceReason {
	return l.Lookup.(rminfo.AbsenceReporter).AbsenceReason(rmType)
}

// TestFloor_REQ112_REQ171_MandatoryPrimitiveListCounted pins REQ-112 Driver
// on an RM-mandatory list of a primitive type: absent (a nil list) it draws
// `required`, rule mandatory, and present and empty it draws `cardinality`,
// rule cardinality, both at the attribute's path. Present with a mebibyte of
// items it draws nothing at that path, and the walk allocates less than the
// payload's size, so the floor learns whether the list is there and empty
// without reading its items (REQ-171 Structural access).
func TestFloor_REQ112_REQ171_MandatoryPrimitiveListCounted(t *testing.T) {
	walk := func(data []byte) []string {
		w := newFloorWalker(nil, Limits{})
		w.info = mandatoryDataLookup{rminfo.Default}
		w.walk(&rm.DVMultimedia{
			MediaType: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "IANA_media-types"}, CodeString: "image/png"},
			Size:      rm.Integer(len(data)),
			Data:      data,
		}, "DV_MULTIMEDIA", "/", roleNone, 0)
		var got []string
		for _, i := range w.issues {
			if i.Path == "/data" {
				got = append(got, i.Code+" "+i.Path+" "+i.Layer.String()+" "+i.Rule)
			}
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
			if got := walk(tc.data); !slices.Equal(got, tc.want) {
				t.Errorf("floor walk of DV_MULTIMEDIA with %s, data RM-mandatory: issues at /data = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
	const size = 1 << 20
	data := make([]byte, size)
	var got []string
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	got = walk(data)
	runtime.ReadMemStats(&after)
	if len(got) != 0 {
		t.Errorf("floor walk of DV_MULTIMEDIA with %d bytes of data, data RM-mandatory: issues at /data = %q, want none", size, got)
	}
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated >= size {
		t.Errorf("floor walk of DV_MULTIMEDIA with %d bytes of data, data RM-mandatory: allocated %d bytes, want fewer than %d: the bytes are read one item at a time", size, allocated, size)
	}
}
