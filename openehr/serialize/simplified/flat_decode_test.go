package simplified

// REQ-053 — FLAT decode: parsing the FLAT key grammar (inverse of the path
// build). Segment ids, zero-based :index, and the trailing |suffix.
import (
	"reflect"
	"testing"
)

func TestDvFromSuffixes(t *testing.T) {
	tests := []struct {
		rmType string
		sfx    map[string]any
		want   map[string]any
	}{
		{
			rmType: "DV_TEXT",
			sfx:    map[string]any{"": "hello"},
			want:   map[string]any{"_type": "DV_TEXT", "value": "hello"},
		},
		{
			rmType: "DV_QUANTITY",
			sfx:    map[string]any{"magnitude": float64(120), "unit": "mm[Hg]"},
			want:   map[string]any{"_type": "DV_QUANTITY", "magnitude": float64(120), "units": "mm[Hg]"},
		},
		{
			rmType: "DV_COUNT",
			sfx:    map[string]any{"magnitude": float64(5)},
			want:   map[string]any{"_type": "DV_COUNT", "magnitude": float64(5)},
		},
		{
			rmType: "DV_BOOLEAN",
			sfx:    map[string]any{"value": true},
			want:   map[string]any{"_type": "DV_BOOLEAN", "value": true},
		},
	}
	for _, tc := range tests {
		t.Run(tc.rmType, func(t *testing.T) {
			got := dvFromSuffixes(tc.rmType, tc.sfx)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("dvFromSuffixes(%s) = %#v, want %#v", tc.rmType, got, tc.want)
			}
		})
	}
}

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
