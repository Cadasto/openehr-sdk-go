package simplified

// REQ-053 — FLAT decode: parsing the FLAT key grammar (inverse of the path
// build). Segment ids, zero-based :index, and the trailing |suffix.
import (
	"errors"
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
			// STABLE RM mappings: DV_COUNT magnitude is the bare value.
			rmType: "DV_COUNT",
			sfx:    map[string]any{"": float64(5)},
			want:   map[string]any{"_type": "DV_COUNT", "magnitude": float64(5)},
		},
		{
			// STABLE RM mappings: DV_BOOLEAN value is the bare value.
			rmType: "DV_BOOLEAN",
			sfx:    map[string]any{"": true},
			want:   map[string]any{"_type": "DV_BOOLEAN", "value": true},
		},
	}
	for _, tc := range tests {
		t.Run(tc.rmType, func(t *testing.T) {
			got, err := dvFromSuffixes(tc.rmType, false, tc.sfx)
			if err != nil {
				t.Fatalf("dvFromSuffixes(%s): %v", tc.rmType, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("dvFromSuffixes(%s) = %#v, want %#v", tc.rmType, got, tc.want)
			}
		})
	}
}

// TestDvFromSuffixesErrors covers the strict-decode guarantees: an unmapped
// datatype and a missing required suffix are errors, not silent zero values.
func TestDvFromSuffixesErrors(t *testing.T) {
	if _, err := dvFromSuffixes("DV_MULTIMEDIA", false, map[string]any{"": "x"}); !errors.Is(err, ErrUnsupportedDatatype) {
		t.Errorf("unmapped datatype err = %v, want ErrUnsupportedDatatype", err)
	}
	if _, err := dvFromSuffixes("DV_COUNT", false, map[string]any{}); err == nil {
		t.Error("DV_COUNT with no bare value = nil error, want missing-suffix error")
	}
	if _, err := dvFromSuffixes("DV_QUANTITY", false, map[string]any{"magnitude": float64(1)}); err == nil {
		t.Error("DV_QUANTITY without |unit = nil error, want missing-suffix error")
	}
}

// TestDvFromSuffixesStrict covers the suffix allowlist, |raw strictness, and the
// |other listOpen precondition.
func TestDvFromSuffixesStrict(t *testing.T) {
	// D4 — unknown suffix (typo) is rejected, not silently dropped.
	if _, err := dvFromSuffixes("DV_QUANTITY", false, map[string]any{"magnitude": float64(1), "unit": "mm", "unitt": "x"}); !errors.Is(err, ErrUnsupportedDatatype) {
		t.Error("unknown |unitt suffix should be rejected")
	}
	// D3 — |raw is mutually exclusive with other suffixes and needs a string _type.
	if _, err := dvFromSuffixes("DV_TEXT", false, map[string]any{"raw": map[string]any{"_type": "DV_TEXT", "value": "x"}, "": "y"}); err == nil {
		t.Error("|raw combined with another suffix should be rejected")
	}
	if _, err := dvFromSuffixes("DV_TEXT", false, map[string]any{"raw": map[string]any{"value": "x"}}); err == nil {
		t.Error("|raw fragment without string _type should be rejected")
	}
	// D5 — |other requires an open value-set.
	if _, err := dvFromSuffixes("DV_CODED_TEXT", false, map[string]any{"other": "free"}); !errors.Is(err, ErrUnsupportedDatatype) {
		t.Error("|other on a closed list should be rejected")
	}
	if _, err := dvFromSuffixes("DV_CODED_TEXT", true, map[string]any{"other": "free"}); err != nil {
		t.Errorf("|other on an open list should be accepted: %v", err)
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
