package rm_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// TestDVTextLikeGetValue pins the SDK-GAP-11 Phase 1 ergonomic method:
// `.GetValue()` on a DVTextLike returns the rendered text of the
// underlying concrete type — DVText OR DVCodedText. Equivalence with
// the legacy compat helper `rm.DVTextValueOf` is the migration
// guarantee.
func TestDVTextLikeGetValue(t *testing.T) {
	tests := []struct {
		name string
		v    rm.DVTextLike
		want string
	}{
		{
			name: "DVText value",
			v:    rm.DVText{Value: "blood pressure"},
			want: "blood pressure",
		},
		{
			name: "DVText pointer",
			v:    &rm.DVText{Value: "blood pressure"},
			want: "blood pressure",
		},
		{
			name: "DVCodedText value",
			v:    rm.DVCodedText{DVText: rm.DVText{Value: "moderate"}, DefiningCode: rm.CodePhrase{CodeString: "at0001"}},
			want: "moderate",
		},
		{
			name: "DVCodedText pointer",
			v:    &rm.DVCodedText{DVText: rm.DVText{Value: "moderate"}, DefiningCode: rm.CodePhrase{CodeString: "at0001"}},
			want: "moderate",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.v.GetValue(); got != tc.want {
				t.Errorf("GetValue() = %q, want %q", got, tc.want)
			}
			// Equivalent compat-helper output — the migration guarantee.
			if got := rm.DVTextValueOf(tc.v); got != tc.want {
				t.Errorf("DVTextValueOf = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestDVTextLikeGetValueNilInterface guards against the v0.x.y pattern
// where a LOCATABLE-descended struct may have a nil Name field (rare
// but legal pre-construction). `DVTextValueOf(nil)` must return ""
// without panicking; calling `.GetValue()` directly on a nil interface
// is a runtime panic by Go's normal interface rules — exercised here
// only via the helper.
func TestDVTextLikeGetValueNilInterface(t *testing.T) {
	if got := rm.DVTextValueOf(nil); got != "" {
		t.Errorf("DVTextValueOf(nil) = %q, want \"\"", got)
	}
}

// TestDVTextLikeGetDefiningCode pins the second Phase 1 accessor:
// (CodePhrase, true) when the concrete type is DVCodedText; (zero,
// false) when it is bare DVText.
func TestDVTextLikeGetDefiningCode(t *testing.T) {
	tests := []struct {
		name        string
		v           rm.DVTextLike
		wantCode    rm.CodePhrase
		wantPresent bool
	}{
		{
			name:        "DVText — no defining code",
			v:           rm.DVText{Value: "free text"},
			wantPresent: false,
		},
		{
			name:        "DVText pointer — no defining code",
			v:           &rm.DVText{Value: "free text"},
			wantPresent: false,
		},
		{
			name:        "DVCodedText — defining code present",
			v:           rm.DVCodedText{DVText: rm.DVText{Value: "moderate"}, DefiningCode: rm.CodePhrase{CodeString: "at0001"}},
			wantCode:    rm.CodePhrase{CodeString: "at0001"},
			wantPresent: true,
		},
		{
			name:        "DVCodedText pointer — defining code present",
			v:           &rm.DVCodedText{DVText: rm.DVText{Value: "moderate"}, DefiningCode: rm.CodePhrase{CodeString: "at0001"}},
			wantCode:    rm.CodePhrase{CodeString: "at0001"},
			wantPresent: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code, present := tc.v.GetDefiningCode()
			if present != tc.wantPresent {
				t.Errorf("present = %v, want %v", present, tc.wantPresent)
			}
			if code.CodeString != tc.wantCode.CodeString {
				t.Errorf("code.CodeString = %q, want %q", code.CodeString, tc.wantCode.CodeString)
			}
		})
	}
}
