package xmlunmatched

import (
	"encoding/xml"
	"errors"
	"slices"
	"strings"
	"testing"
)

// quantityClass is a cut-down C_QUANTITY: it requires its property, a string
// the AM 1.4 schema writes as a CODE_PHRASE.
var quantityClass = &Class{Name: "C_QUANTITY", Required: []string{"property"}}

// quantityTarget reads as a generated C_QUANTITY decoder reads: its property
// through DecodeCodePhraseString, anything else unmatched.
type quantityTarget struct {
	property string
}

func (q *quantityTarget) UnmarshalXML(dec *xml.Decoder, start xml.StartElement) error {
	if err := Enter(dec, start, quantityClass); err != nil {
		return err
	}
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch tk := tok.(type) {
		case xml.StartElement:
			if LocalName(tk.Name) == "property" {
				err = DecodeCodePhraseString(dec, &tk, &q.property)
			} else {
				err = SkipUnmatched(dec, tk)
			}
			if err != nil {
				return err
			}
		case xml.CharData:
			if err := CheckUnmatchedText(dec, tk); err != nil {
				return err
			}
		case xml.EndElement:
			return nil
		}
	}
}

// quantityDoc returns a quantity document whose property element is property.
func quantityDoc(property string) string {
	return `<quantity ` + xmlnsDecl + `>` + property + `</quantity>`
}

// TestREQ056_DecodeCodePhraseStringValue pins the string DecodeCodePhraseString
// reads from each shape (REQ-056), untracked and tracked: a CODE_PHRASE's
// terminology_id value and code_string joined with `::`, the empty string when
// neither carries a value, and an older SDK's text as it is.
func TestREQ056_DecodeCodePhraseStringValue(t *testing.T) {
	for _, tc := range []struct {
		name, property, want string
	}{
		{"CODE_PHRASE", `<property><terminology_id><value>openehr</value></terminology_id><code_string>125</code_string></property>`, "openehr::125"},
		{"code_string first", `<property><code_string>125</code_string><terminology_id><value>openehr</value></terminology_id></property>`, "openehr::125"},
		{"indented CODE_PHRASE", "<property>\n <terminology_id>\n  <value>openehr</value>\n </terminology_id>\n <code_string>125</code_string>\n</property>", "openehr::125"},
		{"code_string alone", `<property><code_string>125</code_string></property>`, "::125"},
		{"terminology_id alone", `<property><terminology_id><value>openehr</value></terminology_id></property>`, "openehr::"},
		{"empty parts", `<property><terminology_id><value/></terminology_id><code_string/></property>`, ""},
		{"whitespace parts", `<property><terminology_id><value> </value></terminology_id><code_string>  </code_string></property>`, ""},
		{"text beside a CODE_PHRASE child", `<property>junk<code_string>125</code_string></property>`, "::125"},
		{"older text", `<property>openehr::125</property>`, "openehr::125"},
		{"text with no ::", `<property>mass</property>`, "mass"},
		{"text around an unmatched child", `<property>openehr::<x>9</x>125</property>`, "openehr::125"},
		{"empty", `<property/>`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := quantityDoc(tc.property)
			var untracked quantityTarget
			if err := xml.NewDecoder(strings.NewReader(doc)).Decode(&untracked); err != nil {
				t.Fatalf("untracked Decode(%s) = %v; want nil", doc, err)
			}
			if untracked.property != tc.want {
				t.Errorf("untracked Decode(%s): property = %q; want %q", doc, untracked.property, tc.want)
			}
			var tracked quantityTarget
			tr := NewTracker(func(string) {}, false)
			if err := tr.Decode(tr.NewDecoder(strings.NewReader(doc)), &tracked); err != nil {
				t.Fatalf("tracked Decode(%s) = %v; want nil", doc, err)
			}
			if tracked.property != tc.want {
				t.Errorf("tracked Decode(%s): property = %q; want %q", doc, tracked.property, tc.want)
			}
		})
	}
}

// TestREQ170_DecodeCodePhraseStringUnmatched pins the content of a CODE_PHRASE
// string DecodeCodePhraseString does not read (REQ-170), reported in document
// order and refused under strict: an attribute, a child other than
// terminology_id and code_string (preferred_term among them: the string has
// nowhere to hold it), text beside a CODE_PHRASE child, before it or after,
// and anything in terminology_id but its value. Beside text alone, a child
// is unmatched and the text is the value; once a CODE_PHRASE child shows
// the text is unmatched, the text comes before a child skipped after it.
func TestREQ170_DecodeCodePhraseStringUnmatched(t *testing.T) {
	const property = "/quantity[1]/property[1]"
	for _, tc := range []struct {
		name, property string
		want           []string
	}{
		{"CODE_PHRASE", `<property><terminology_id><value>openehr</value></terminology_id><code_string>125</code_string></property>`, nil},
		{"indented CODE_PHRASE", "<property>\n <terminology_id>\n  <value>openehr</value>\n </terminology_id>\n <code_string>125</code_string>\n</property>", nil},
		{"text", `<property>openehr::125</property>`, nil},
		{"attribute", `<property foo="1"><code_string>125</code_string></property>`, []string{property + "/@foo"}},
		{"text before a CODE_PHRASE child", `<property>junk<code_string>125</code_string></property>`, []string{property + "/text()"}},
		{"text after a CODE_PHRASE child", `<property><code_string>125</code_string>junk</property>`, []string{property + "/text()"}},
		{"text, then a child, then a CODE_PHRASE child", `<property>junk<x/><code_string>125</code_string></property>`, []string{property + "/text()", property + "/x[1]"}},
		{"text, then a child", `<property>openehr::125<x/></property>`, []string{property + "/x[1]"}},
		{"preferred_term", `<property><code_string>125</code_string><preferred_term>mass</preferred_term></property>`, []string{property + "/preferred_term[1]"}},
		{
			"inside terminology_id", `<property><terminology_id a="1">t<value>openehr</value><y/></terminology_id></property>`,
			[]string{property + "/terminology_id[1]/@a", property + "/terminology_id[1]/text()", property + "/terminology_id[1]/y[1]"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := quantityDoc(tc.property)
			var got []string
			tr := NewTracker(func(p string) { got = append(got, p) }, false)
			if err := tr.Decode(tr.NewDecoder(strings.NewReader(doc)), new(quantityTarget)); err != nil {
				t.Fatalf("Decode(%s) = %v; want nil", doc, err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("Decode(%s): unmatched = %q; want %q", doc, got, tc.want)
			}
			strict := NewTracker(nil, true)
			err := strict.Decode(strict.NewDecoder(strings.NewReader(doc)), new(quantityTarget))
			if len(tc.want) == 0 && err != nil {
				t.Errorf("strict Decode(%s) = %v; want nil", doc, err)
			}
			if len(tc.want) > 0 && (!errors.Is(err, ErrUnknownElement) || !strings.Contains(err.Error(), tc.want[0])) {
				t.Errorf("strict Decode(%s) = %v; want a refusal of %q", doc, err, tc.want[0])
			}
		})
	}
}

// TestREQ175_DecodeCodePhraseStringMissing pins when a CODE_PHRASE string
// carries a value (REQ-175): as text, when the text does; as a CODE_PHRASE,
// when its terminology_id value or its code_string does, whatever text
// stands beside them. Otherwise the member is reported once, by its own
// pointer, and nothing inside the CODE_PHRASE is.
func TestREQ175_DecodeCodePhraseStringMissing(t *testing.T) {
	missing := []string{"/property"}
	for _, tc := range []struct {
		name, property string
		want           []string
	}{
		{"CODE_PHRASE", `<property><terminology_id><value>openehr</value></terminology_id><code_string>125</code_string></property>`, nil},
		{"terminology_id value alone", `<property><terminology_id><value>openehr</value></terminology_id><code_string/></property>`, nil},
		{"code_string alone", `<property><terminology_id/><code_string>125</code_string></property>`, nil},
		{"text", `<property>openehr::125</property>`, nil},
		{"empty parts", `<property><terminology_id><value/></terminology_id><code_string/></property>`, missing},
		{"empty terminology_id", `<property><terminology_id/></property>`, missing},
		{"text before empty parts", `<property>junk<terminology_id><value/></terminology_id><code_string/></property>`, missing},
		{"text after empty parts", `<property><terminology_id><value/></terminology_id><code_string/>junk</property>`, missing},
		{"whitespace text", `<property> </property>`, missing},
		{"empty", `<property/>`, missing},
		{"nilled", `<property xsi:nil="true"><code_string>125</code_string></property>`, missing},
		{"absent", ``, missing},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := quantityDoc(tc.property)
			got, err := decodeMissing(doc, new(quantityTarget), false)
			if err != nil {
				t.Fatalf("Decode(%s) = %v; want nil", doc, err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("Decode(%s): missing members = %q; want %q", doc, got, tc.want)
			}
		})
	}
}

// symbolTarget returns the decode target of a cut-down ORDINAL whose symbol
// is a CODE_PHRASE written in either shape of the AM 1.4 schema type
// ORDINAL_SYMBOL: its class reads terminology_id and code_string as its own,
// and from defining_code too (Class.Inline), and the symbol decoder drops
// the DV_CODED_TEXT's value.
func symbolTarget() *described {
	id := &described{class: &Class{Name: "TERMINOLOGY_ID", Required: []string{"value"}}, prims: []string{"value"}}
	phrase := &described{
		class: &Class{Name: "CODE_PHRASE", Required: []string{"terminology_id", "code_string"}},
		prims: []string{"code_string"},
		kids:  map[string]*described{"terminology_id": id},
	}
	symbol := &described{
		class: &Class{Name: "CODE_PHRASE", Required: []string{"terminology_id", "code_string"}, Inline: []string{"defining_code"}},
		prims: []string{"code_string"},
		kids:  map[string]*described{"terminology_id": id, "defining_code": phrase},
		drop:  []string{"value"},
	}
	return &described{
		class: &Class{Name: "C_ORDINAL", Items: []string{"list"}},
		kids: map[string]*described{"list": {
			class: &Class{Name: "ORDINAL", Required: []string{"symbol", "value"}},
			prims: []string{"value"},
			kids:  map[string]*described{"symbol": symbol},
		}},
	}
}

// TestREQ175_MissingMembersInline pins a class that reads its own members
// from a child element too (Class.Inline), an ORDINAL's symbol written as a
// DV_CODED_TEXT whose defining_code holds them (REQ-175): each member the
// element carries a value for has one; the element reports nothing of its
// own; a member inside it is named by its pointer in the canonical-JSON form
// of the instance, where the element does not appear; and a nilled element
// carries nothing. The DV_CODED_TEXT's value is no member of the symbol.
func TestREQ175_MissingMembersInline(t *testing.T) {
	const symbol = "/list/0/symbol"
	for _, tc := range []struct {
		name, symbol string
		want         []string
	}{
		{"CODE_PHRASE", `<symbol><terminology_id><value>local</value></terminology_id><code_string>at0001</code_string></symbol>`, nil},
		{"defining_code", `<symbol><value/><defining_code><terminology_id><value>local</value></terminology_id><code_string>at0001</code_string></defining_code></symbol>`, nil},
		{"defining_code without code_string", `<symbol><value/><defining_code><terminology_id><value>local</value></terminology_id></defining_code></symbol>`, []string{symbol + "/code_string"}},
		{"defining_code with an empty terminology_id", `<symbol><defining_code><terminology_id/><code_string>at0001</code_string></defining_code></symbol>`, []string{symbol + "/terminology_id/value"}},
		{"empty defining_code", `<symbol><value/><defining_code/></symbol>`, []string{symbol + "/terminology_id", symbol + "/code_string"}},
		{"nilled defining_code", `<symbol><defining_code xsi:nil="true"><terminology_id><value>local</value></terminology_id><code_string>at0001</code_string></defining_code></symbol>`, []string{symbol + "/terminology_id", symbol + "/code_string"}},
		{"no defining_code", `<symbol><value/></symbol>`, []string{symbol + "/terminology_id", symbol + "/code_string"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := `<c_ordinal ` + xmlnsDecl + `><list><value>1</value>` + tc.symbol + `</list></c_ordinal>`
			got, err := decodeMissing(doc, symbolTarget(), false)
			if err != nil {
				t.Fatalf("Decode(%s) = %v; want nil", doc, err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("Decode(%s): missing members = %q; want %q", doc, got, tc.want)
			}
		})
	}
}

// TestREQ170_DropReportsNothing pins that Drop skips an element of another
// shape the schema admits with nothing reported inside it, unmatched or
// missing (REQ-170, REQ-175), where the default arm reports the same element
// as unmatched.
func TestREQ170_DropReportsNothing(t *testing.T) {
	doc := `<c_ordinal ` + xmlnsDecl + `><list><value>1</value><symbol><value foo="1">Mild<x/></value>` +
		`<defining_code><terminology_id><value>local</value></terminology_id><code_string>at0001</code_string></defining_code></symbol></list></c_ordinal>`
	var unmatched, missing []string
	tr := NewTracker(func(p string) { unmatched = append(unmatched, p) }, false)
	tr.ReportMissing(func(p string) { missing = append(missing, p) })
	if err := tr.Decode(tr.NewDecoder(strings.NewReader(doc)), symbolTarget()); err != nil {
		t.Fatalf("Decode(%s) = %v; want nil", doc, err)
	}
	if len(unmatched) != 0 || len(missing) != 0 {
		t.Errorf("Decode(%s) reported unmatched %q and missing %q; want nothing", doc, unmatched, missing)
	}
	strict := NewTracker(nil, true)
	if err := strict.Decode(strict.NewDecoder(strings.NewReader(doc)), symbolTarget()); err != nil {
		t.Errorf("strict Decode(%s) = %v; want nil", doc, err)
	}
}
