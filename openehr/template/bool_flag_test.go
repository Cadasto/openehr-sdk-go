package template

// In package template (internal) to decode into the unexported
// xmlBoolFlag directly.

import (
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

// refBoolFlag is xmlBoolFlag as it stood when it decoded through
// UnmarshalXML: a copy kept as the reference the current decode must
// match on every input below.
type refBoolFlag struct {
	set   bool
	value bool
}

func (f *refBoolFlag) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var text string
	if err := d.DecodeElement(&text, &start); err != nil {
		return err
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	v, err := strconv.ParseBool(text)
	if err != nil {
		return err
	}
	*f = refBoolFlag{set: true, value: v}
	return nil
}

// boolFlagDoc holds one flag element <f> and a following sibling
// <after>, so a decode that stops in the wrong place shows in After.
type boolFlagDoc[F any] struct {
	F     F      `xml:"f"`
	After string `xml:"after"`
}

// boolFlagOutcome is what one decode produced, error text included.
type boolFlagOutcome struct {
	set, value bool
	after      string
	err        string
}

func (o boolFlagOutcome) String() string {
	return fmt.Sprintf("set=%v value=%v after=%q err=%q", o.set, o.value, o.after, o.err)
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// The flag decodes every input below exactly as the reference
// UnmarshalXML did: the same set and value, the same following sibling,
// and the same error text.
func TestXMLBoolFlag_DecodesLikeReference(t *testing.T) {
	cases := []struct {
		name string
		doc  string
	}{
		{"omitted", `<w></w>`},
		{"self-closing", `<w><f/></w>`},
		{"empty", `<w><f></f></w>`},
		{"whitespace only", `<w><f>   </f></w>`},
		{"padded true", "<w><f>\n\t true \n</f></w>"},
		{"no-break spaces around true", "<w><f> true </f></w>"},
		{"false", `<w><f>false</f></w>`},
		{"upper case TRUE", `<w><f>TRUE</f></w>`},
		{"1", `<w><f>1</f></w>`},
		{"0", `<w><f>0</f></w>`},
		{"t", `<w><f>t</f></w>`},
		{"not a boolean", `<w><f>yes</f></w>`},
		{"nested child only", `<w><f><x>true</x></f></w>`},
		{"text around a child", `<w><f> true <x>false</x> </f></w>`},
		{"text split by a child", `<w><f>tr<x/>ue</f></w>`},
		{"text split by a comment", `<w><f>tr<!-- c -->ue</f></w>`},
		{"CDATA", `<w><f><![CDATA[true]]></f></w>`},
		{"character reference", `<w><f>&#116;rue</f></w>`},
		{"repeated", `<w><f>true</f><f>false</f></w>`},
		{"repeated, second empty", `<w><f>true</f><f></f></w>`},
		{"repeated, second not a boolean", `<w><f>true</f><f>bad</f></w>`},
		{"following sibling", `<w><f>true</f><after>x</after></w>`},
		{"nested child, following sibling", `<w><f><x>true</x></f><after>x</after></w>`},
		{"unclosed element", `<w><f>true</w>`},
		{"attribute", `<w><f a="1">true</f></w>`},
		{"namespace", `<w><f xmlns="urn:x">true</f></w>`},
	}
	var set, unset, failed int
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotDoc boolFlagDoc[xmlBoolFlag]
			gotErr := xml.Unmarshal([]byte(tc.doc), &gotDoc)
			got := boolFlagOutcome{set: gotDoc.F.set, value: gotDoc.F.value, after: gotDoc.After, err: errText(gotErr)}

			var refDoc boolFlagDoc[refBoolFlag]
			refErr := xml.Unmarshal([]byte(tc.doc), &refDoc)
			want := boolFlagOutcome{set: refDoc.F.set, value: refDoc.F.value, after: refDoc.After, err: errText(refErr)}

			if got != want {
				t.Errorf("Unmarshal(%q):\n got  %v\n want %v", tc.doc, got, want)
			}
			switch {
			case want.err != "":
				failed++
			case want.set:
				set++
			default:
				unset++
			}
		})
	}
	// Each outcome must occur, or the comparison proves nothing for it.
	if set == 0 || unset == 0 || failed == 0 {
		t.Errorf("reference outcomes: %d set, %d unset, %d failed; want each above zero", set, unset, failed)
	}
}
