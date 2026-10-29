package simplified

// REQ-052 / REQ-053 / REQ-140 — the text of a FLAT refusal is read by the
// caller and lands in logs: it names the key and what was refused, never a
// requirement id, and never a value the document carries.
import (
	"errors"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// refusalTextSecret is the value a case plants where the document's own value
// could reach the error text.
const refusalTextSecret = "secret-value-0451"

// checkRefusalText fails when err is not want, does not name key, names a
// requirement id, or carries refusalTextSecret.
func checkRefusalText(t *testing.T, err, want error, key string) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
	msg := err.Error()
	if !strings.Contains(msg, key) {
		t.Errorf("err = %q, want it to name %q", msg, key)
	}
	if strings.Contains(msg, "REQ-") {
		t.Errorf("err = %q names a requirement id", msg)
	}
	if strings.Contains(msg, refusalTextSecret) {
		t.Errorf("err = %q carries the document's value", msg)
	}
}

// TestREQ140DecodeRefusalTextNamesNoRequirementOrValue — REQ-052, REQ-053,
// REQ-140. A representative set of the underscore grammar's decode refusals,
// each spelled with a planted value where the document's value could reach the
// text.
func TestREQ140DecodeRefusalTextNamesNoRequirementOrValue(t *testing.T) {
	wt, _ := conformanceWT(t)
	audit := rmattrObs + "/_feeder_audit/originating_system_audit"
	participation := rmattrContext + "/_participation:0"
	for _, tc := range []struct {
		name, key string
		want      error
		keys      map[string]any
	}{
		{
			name: "TERM_MAPPING match outside its set", key: rmattrText + "/_mapping:0|match", want: ErrUnsupportedDatatype,
			keys: map[string]any{
				rmattrText:                             "DV_TEXT value",
				rmattrText + "/_mapping:0|match":       refusalTextSecret,
				rmattrText + "/_mapping:0/target|code": "21794005",
			},
		},
		{
			name: "participation mode outside its group", key: participation + "|mode", want: ErrUnsupportedDatatype,
			keys: map[string]any{participation + "|function": "requester", participation + "|mode": refusalTextSecret},
		},
		{
			name: "PARTY_PROXY subtype other than PARTY_SELF", key: audit + "/subject|_type", want: ErrUnsupportedDatatype,
			keys: map[string]any{audit + "|system_id": "orig", audit + "/subject|_type": refusalTextSecret},
		},
		{
			name: "PARTICIPATION time", key: participation + "|time", want: ErrUnsupportedDatatype,
			keys: map[string]any{participation + "|function": "requester", participation + "|time": refusalTextSecret},
		},
		{
			name: "FEEDER_AUDIT_DETAILS other_details", key: audit + "/other_details|x", want: ErrUnsupportedDatatype,
			keys: map[string]any{audit + "|system_id": "orig", audit + "/other_details|x": refusalTextSecret},
		},
		{
			name: "deferred family", key: rmattrAction + "/_instruction_details", want: ErrUnknownPath,
			keys: map[string]any{rmattrAction + "/_instruction_details|activity_id": refusalTextSecret},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checkRefusalText(t, decodeRMAttrErr(t, wt, rmattrBody(tc.keys)), tc.want, tc.key)
		})
	}
}

// TestREQ140EncodeRefusalTextNamesNoRequirementOrValue — REQ-052, REQ-053,
// REQ-140. A representative set of the underscore grammar's encode refusals,
// each built with a planted value where the composition's value could reach the
// text.
func TestREQ140EncodeRefusalTextNamesNoRequirementOrValue(t *testing.T) {
	wt, _ := conformanceWT(t)
	participation := rmattrContext + "/_participation:0"
	base := rmattrBody(map[string]any{
		rmattrText:                                    "DV_TEXT value",
		rmattrText + "/_mapping:0|match":              "=",
		rmattrText + "/_mapping:0/target|code":        "21794005",
		rmattrText + "/_mapping:0/target|terminology": "SNOMED-CT",
	})
	for _, tc := range []struct {
		name, key string
		mutate    func(t *testing.T, c *rm.Composition)
	}{
		{
			name: "participation mode code outside its group", key: participation + "|mode",
			mutate: func(_ *testing.T, c *rm.Composition) {
				mode := rm.DVCodedText{
					Value:        "by carrier pigeon",
					DefiningCode: rm.CodePhrase{CodeString: refusalTextSecret, TerminologyID: rm.TerminologyID{Value: "openehr"}},
				}
				c.Context.Participations = []rm.Participation{{
					Function: rm.DVText{Value: "requester"}, Mode: &mode, Performer: rm.PartySelf{},
				}}
			},
		},
		{
			name: "PARTICIPATION time", key: participation + "|time",
			mutate: func(_ *testing.T, c *rm.Composition) {
				c.Context.Participations = []rm.Participation{{
					Function: rm.DVText{Value: "requester"}, Performer: rm.PartySelf{},
					Time: &rm.DVInterval[rm.DVDateTime]{Lower: rm.DVDateTime{Value: refusalTextSecret}},
				}}
			},
		},
		{
			name: "TERM_MAPPING match outside its set", key: rmattrText + "/_mapping:0|match",
			mutate: func(t *testing.T, c *rm.Composition) {
				text, ok := elementText(t, c).(*rm.DVText)
				if !ok {
					t.Fatalf("dv_text leaf decoded as %T", elementText(t, c))
				}
				text.Mappings[0].Match = "~"
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			comp := decodeRMAttr(t, wt, base)
			tc.mutate(t, comp)
			_, err := MarshalFlat(comp, wt)
			checkRefusalText(t, err, ErrUnsupportedDatatype, tc.key)
		})
	}
}
