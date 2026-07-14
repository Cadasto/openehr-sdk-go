package simplified

// REQ-053 — FLAT decode: parsing the FLAT key grammar (inverse of the path
// build). Segment ids, zero-based :index, and the trailing |suffix.
import (
	"reflect"
	"testing"
)

func TestParseFlatKey(t *testing.T) {
	tests := []struct {
		key        string
		wantSegs   []flatSeg
		wantSuffix string
	}{
		{
			key:      "minimal/minimal:0/cualquier_evento/text",
			wantSegs: []flatSeg{{"minimal", -1}, {"minimal", 0}, {"cualquier_evento", -1}, {"text", -1}},
		},
		{
			key:        "vs/blood_pressure:1/systolic|magnitude",
			wantSegs:   []flatSeg{{"vs", -1}, {"blood_pressure", 1}, {"systolic", -1}},
			wantSuffix: "magnitude",
		},
		{
			key:        "root/category|terminology",
			wantSegs:   []flatSeg{{"root", -1}, {"category", -1}},
			wantSuffix: "terminology",
		},
	}
	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			got := parseFlatKey(tc.key)
			if !reflect.DeepEqual(got.segs, tc.wantSegs) {
				t.Errorf("segs = %+v, want %+v", got.segs, tc.wantSegs)
			}
			if got.suffix != tc.wantSuffix {
				t.Errorf("suffix = %q, want %q", got.suffix, tc.wantSuffix)
			}
		})
	}
}
