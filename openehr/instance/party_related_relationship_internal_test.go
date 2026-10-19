package instance

import (
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// relationshipCodedText is a DV_CODED_TEXT whose defining code a
// C_CODE_PHRASE pins to codes of terminologyID. With no codes it names
// the terminology and no code.
func relationshipCodedText(terminologyID string, codes ...string) string {
	var list strings.Builder
	for _, code := range codes {
		list.WriteString(`<code_list>` + code + `</code_list>`)
	}
	return `<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>DV_CODED_TEXT</rm_type_name><node_id></node_id>` +
		visitSingle("defining_code", `<children xsi:type="C_CODE_PHRASE"><rm_type_name>CODE_PHRASE</rm_type_name>`+
			`<node_id></node_id><terminology_id><value>`+terminologyID+`</value></terminology_id>`+list.String()+`</children>`) +
		`</children>`
}

// TestREQ107_RelationshipDefaultYieldsToTheOPT is the REQ-107 check of
// Precedence 1 on a PARTY_RELATED's relationship: the RM default openehr
// 253 "unknown" replaces only a relationship with no code (empty, or the
// at0000 placeholder), and only where the OPT's constraint on relationship
// admits it. A code the OPT gave is kept, also where the OPT admits 253
// too, and the placeholder stays where the OPT names another terminology.
// It calls fillRelationship directly, so each relationship state it starts
// from is set by hand. Through Generate,
// TestREQ107_PartyRelatedGetsARelationship checks the 253 default where the
// OPT gives no code, and TestREQ107_RelationshipKeepsTheOPTCode checks that
// a code the OPT pins is kept: the walk's 10 under ExampleFill, also where
// the OPT admits 253 too.
func TestREQ107_RelationshipDefaultYieldsToTheOPT(t *testing.T) {
	coded := func(terminologyID, code, text string) rm.DVCodedText {
		return rm.DVCodedText{
			Value:        text,
			DefiningCode: rm.CodePhrase{CodeString: code, TerminologyID: rm.TerminologyID{Value: terminologyID}},
		}
	}
	unknown := coded("openehr", "253", "unknown")
	cases := []struct {
		name string
		opt  string // the relationship's OPT child
		have rm.DVCodedText
		want rm.DVCodedText
	}{
		{
			name: "OPT code kept where the OPT admits 253 too",
			opt:  relationshipCodedText("openehr", "10", "253"),
			have: coded("openehr", "10", "mother"),
			want: coded("openehr", "10", "mother"),
		},
		{
			name: "placeholder kept where the OPT names terminology local",
			opt:  relationshipCodedText("local"),
			have: coded("local", "at0000", "example"),
			want: coded("local", "at0000", "example"),
		},
		{
			name: "placeholder replaced where the OPT names openehr with no code",
			opt:  relationshipCodedText("openehr"),
			have: coded("openehr", "at0000", "example"),
			want: unknown,
		},
		{
			name: "empty relationship replaced where the OPT names openehr with no code",
			opt:  relationshipCodedText("openehr"),
			have: rm.DVCodedText{},
			want: unknown,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opt, _ := visitAttribute(t, visitRootOPT("PARTY_RELATED", visitSingle("relationship", tc.opt)), "relationship")
			got := tc.have
			fillRelationship(opt, &got)
			if got.DefiningCode != tc.want.DefiningCode || got.Value != tc.want.Value {
				t.Errorf("fillRelationship(%+v) = %q %+v, want %q %+v",
					tc.have.DefiningCode, got.Value, got.DefiningCode, tc.want.Value, tc.want.DefiningCode)
			}
		})
	}
}
