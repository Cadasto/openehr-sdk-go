package xmlunmatched

import (
	"encoding/xml"
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// The targets below read as a generated decoder reads (REQ-175): on entry
// each hands its class to Enter, a child named in prims reads as text through
// DecodeElement, a child named in kids decodes into a copy of that target,
// and anything else is unmatched.

// described is one such target.
type described struct {
	class *Class
	prims []string
	kids  map[string]*described
}

func (d *described) UnmarshalXML(dec *xml.Decoder, start xml.StartElement) error {
	if err := Enter(dec, start, d.class); err != nil {
		return err
	}
	return d.readChildren(dec)
}

// readChildren reads the children of the element d decodes, after its entry.
func (d *described) readChildren(dec *xml.Decoder) error {
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch tk := tok.(type) {
		case xml.StartElement:
			name := LocalName(tk.Name)
			if slices.Contains(d.prims, name) {
				var s string
				if err := DecodeElement(dec, &tk, &s); err != nil {
					return err
				}
				continue
			}
			if kid, ok := d.kids[name]; ok {
				child := *kid
				if err := dec.DecodeElement(&child, &tk); err != nil {
					return err
				}
				continue
			}
			if err := SkipUnmatched(dec, tk); err != nil {
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

// plainEntry is described with the entry call every decoder made before
// REQ-175: it hands its attributes to CheckUnmatchedAttrs and no class to the
// runtime. The allocation test holds Enter to its cost.
type plainEntry described

func (d *plainEntry) UnmarshalXML(dec *xml.Decoder, start xml.StartElement) error {
	if err := CheckUnmatchedAttrs(dec, start, d.class.Attrs...); err != nil {
		return err
	}
	return (*described)(d).readChildren(dec)
}

// The classes of the test tree, a cut-down COMPOSITION: its content items
// are ENTRY-like, each with a HISTORY whose events hold a time and a data
// tree. The root reads archetype_node_id as an XML attribute, as LOCATABLE
// does, and lists it among its mandatory members.
var (
	treeClass    = &Class{Name: "ITEM_TREE"}
	eventClass   = &Class{Name: "EVENT", Required: []string{"time", "data"}}
	historyClass = &Class{Name: "HISTORY", Required: []string{"origin", "events"}, Items: []string{"events"}}
	entryClass   = &Class{Name: "ENTRY", Required: []string{"language", "data"}}
	textClass    = &Class{Name: "DV_TEXT", Required: []string{"value"}}
	compClass    = &Class{
		Name:     "COMPOSITION",
		Attrs:    []string{"archetype_node_id"},
		Required: []string{"name", "archetype_node_id", "language", "content"},
		Items:    []string{"content"},
	}
)

// compositionTarget returns the decode target of the test tree.
func compositionTarget() *described {
	event := &described{class: eventClass, prims: []string{"time"}, kids: map[string]*described{"data": {class: treeClass}}}
	history := &described{class: historyClass, prims: []string{"origin"}, kids: map[string]*described{"events": event}}
	entry := &described{class: entryClass, prims: []string{"language"}, kids: map[string]*described{"data": history}}
	text := &described{class: textClass, prims: []string{"value"}}
	return &described{
		class: compClass,
		prims: []string{"language", "archetype_node_id"},
		kids:  map[string]*described{"name": text, "content": entry},
	}
}

const xmlnsDecl = `xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"`

// decodeMissing decodes doc into target through a tracker whose missing-member
// hook collects the pointers, and returns them with the decode's error.
func decodeMissing(doc string, target any, strict bool) ([]string, error) {
	var got []string
	tr := NewTracker(nil, strict)
	tr.ReportMissing(func(p string) { got = append(got, p) })
	err := tr.Decode(tr.NewDecoder(strings.NewReader(doc)), target)
	return got, err
}

// TestREQ175_MissingMembersNestedPointers pins the pointer of a missing
// member and the order of the report: a member of the root is `/name`; an
// element below the root adds its local name and, when its parent's class
// lists the name among its multi-valued members, its 0-based position among
// its same-named siblings; an element with no content is present; the members
// of one element come in the class's Required order as it closes, so inner
// elements report before the ones around them.
func TestREQ175_MissingMembersNestedPointers(t *testing.T) {
	doc := `<composition ` + xmlnsDecl + ` archetype_node_id="at0000">
  <content><language>en</language><data><events><data/></events><events><time>t</time></events></data></content>
  <content><language>en</language><data><origin>o</origin></data></content>
</composition>`
	got, err := decodeMissing(doc, compositionTarget(), false)
	if err != nil {
		t.Fatalf("Decode = %v; want nil", err)
	}
	want := []string{
		"/content/0/data/events/0/time",
		"/content/0/data/events/1/data",
		"/content/0/data/origin",
		"/content/1/data/events",
		"/name",
		"/language",
	}
	if !slices.Equal(got, want) {
		t.Errorf("missing members = %q; want %q", got, want)
	}
}

// TestREQ175_MissingMembersNilAndEmpty pins what carries no value: a child
// with xsi:nil="true", whatever it holds, and a primitive element whose text
// is empty or XML whitespace only. Nothing inside a nilled element is
// reported: the member is reported once, by its parent, as the canonical-JSON
// decode reports a null.
func TestREQ175_MissingMembersNilAndEmpty(t *testing.T) {
	doc := `<composition ` + xmlnsDecl + ` archetype_node_id="at0000">
  <name xsi:nil="true"/>
  <language> </language>
  <content xsi:nil="true"><language>en</language><data><origin>o</origin><events><time>t</time><data/></events></data></content>
</composition>`
	got, err := decodeMissing(doc, compositionTarget(), false)
	if err != nil {
		t.Fatalf("Decode = %v; want nil", err)
	}
	want := []string{"/name", "/language", "/content"}
	if !slices.Equal(got, want) {
		t.Errorf("missing members = %q; want %q", got, want)
	}

	// An empty primitive beside one with a value: the member has a value.
	doc = `<composition ` + xmlnsDecl + ` archetype_node_id="at0000"><name><value/></name><language/><language>en</language><content/></composition>`
	got, err = decodeMissing(doc, compositionTarget(), false)
	if err != nil {
		t.Fatalf("Decode = %v; want nil", err)
	}
	if want := []string{"/name/value", "/content/0/language", "/content/0/data"}; !slices.Equal(got, want) {
		t.Errorf("missing members = %q; want %q", got, want)
	}
}

// TestREQ175_IsNilled pins the forms of xsi:nil the reader reads: in the
// resolved XML Schema instance namespace or written literally, with the
// XML Schema boolean lexical forms `true` and `1`; `false`, `0` and a nil in
// another namespace do not count.
func TestREQ175_IsNilled(t *testing.T) {
	xsi := xml.Name{Space: NSXSI, Local: "nil"}
	for _, tc := range []struct {
		name string
		attr xml.Attr
		want bool
	}{
		{"resolved true", xml.Attr{Name: xsi, Value: "true"}, true},
		{"resolved 1", xml.Attr{Name: xsi, Value: "1"}, true},
		{"resolved false", xml.Attr{Name: xsi, Value: "false"}, false},
		{"resolved 0", xml.Attr{Name: xsi, Value: "0"}, false},
		{"literal", xml.Attr{Name: xml.Name{Local: "xsi:nil"}, Value: "true"}, true},
		{"spaced", xml.Attr{Name: xsi, Value: " true "}, true},
		{"foreign namespace", xml.Attr{Name: xml.Name{Space: "urn:x", Local: "nil"}, Value: "true"}, false},
		{"bare nil", xml.Attr{Name: xml.Name{Local: "nil"}, Value: "true"}, false},
	} {
		if got := isNilled([]xml.Attr{tc.attr}); got != tc.want {
			t.Errorf("%s: isNilled(%v) = %v; want %v", tc.name, tc.attr, got, tc.want)
		}
	}
	if isNilled(nil) {
		t.Error("isNilled(no attributes) = true; want false")
	}
}

// TestREQ175_MissingMembersXMLAttribute pins that a member the class reads as
// an XML attribute is present when the start element carries the attribute,
// present when a child element of that name carries it instead, and missing
// otherwise, reported in the class's order among the other members.
func TestREQ175_MissingMembersXMLAttribute(t *testing.T) {
	for _, tc := range []struct {
		name string
		doc  string
		want []string
	}{
		{"attribute", `<composition ` + xmlnsDecl + ` archetype_node_id="at0000"><content/></composition>`, []string{"/name", "/language"}},
		{"child element", `<composition ` + xmlnsDecl + `><archetype_node_id>at0000</archetype_node_id><content/></composition>`, []string{"/name", "/language"}},
		{"absent", `<composition ` + xmlnsDecl + `><content/></composition>`, []string{"/name", "/archetype_node_id", "/language"}},
		{"empty child", `<composition ` + xmlnsDecl + `><archetype_node_id/><content/></composition>`, []string{"/name", "/archetype_node_id", "/language"}},
	} {
		// The empty content item, an ENTRY, reports its own members first.
		tc.want = append([]string{"/content/0/language", "/content/0/data"}, tc.want...)
		got, err := decodeMissing(tc.doc, compositionTarget(), false)
		if err != nil {
			t.Fatalf("%s: Decode = %v; want nil", tc.name, err)
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s: missing members = %q; want %q", tc.name, got, tc.want)
		}
	}
}

// codedTextTarget returns the decode target of a cut-down C_SINGLE_ATTRIBUTE
// whose children are C_CODED_TEXT-like: each requires node_id and
// terminology, and its class names terminology_id, a TERMINOLOGY_ID-like
// element whose value holds the string, as the element terminology is
// written under. The child reads both terminology_id and terminology.
func codedTextTarget() *described {
	id := &described{class: &Class{Name: "TERMINOLOGY_ID", Required: []string{"value"}}, prims: []string{"value"}}
	coded := &described{
		class: &Class{
			Name:     "C_CODED_TEXT",
			Required: []string{"node_id", "terminology"},
			Elements: []Element{{Member: "terminology", Name: "terminology_id"}},
		},
		prims: []string{"node_id", "terminology"},
		kids:  map[string]*described{"terminology_id": id},
	}
	return &described{
		class: &Class{Name: "C_SINGLE_ATTRIBUTE", Items: []string{"children"}},
		kids:  map[string]*described{"children": coded},
	}
}

// TestREQ175_MissingMembersElementOfAnotherName pins a member written under
// an element of another name (Class.Elements): the element gives the member a
// value when it carries every mandatory member of its own class, and the
// member's own element still does. When the element carries no value, the
// member is reported once, by its own pointer in the canonical-JSON form,
// `/children/0/terminology`, and nothing inside the element is reported,
// such as `/children/0/terminology_id/value`, which the canonical-JSON form
// does not have.
func TestREQ175_MissingMembersElementOfAnotherName(t *testing.T) {
	const missing = "/children/0/terminology"
	for _, tc := range []struct {
		name, terminology string
		want              []string
	}{
		{"element with a value", `<terminology_id><value>local</value></terminology_id>`, nil},
		{"own element", `<terminology>local</terminology>`, nil},
		{"empty element", `<terminology_id/>`, []string{missing}},
		{"element with an empty value", `<terminology_id><value/></terminology_id>`, []string{missing}},
		{"element with a blank value", `<terminology_id><value> </value></terminology_id>`, []string{missing}},
		{"nilled element", `<terminology_id xsi:nil="true"><value>local</value></terminology_id>`, []string{missing}},
		{"empty own element", `<terminology/>`, []string{missing}},
		{"absent", ``, []string{missing}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := `<c_single_attribute ` + xmlnsDecl + `><children><node_id>at0001</node_id>` + tc.terminology + `</children></c_single_attribute>`
			got, err := decodeMissing(doc, codedTextTarget(), false)
			if err != nil {
				t.Fatalf("Decode(%s) = %v; want nil", doc, err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("Decode(%s): missing members = %q; want %q", doc, got, tc.want)
			}
		})
	}
}

// TestREQ175_MissingMembersReportOnly pins that the report is the whole
// policy: a strict decode refuses no missing member, Enter still hands the
// attributes the class does not read to the unmatched-content policy, a
// tracked decode without the hook reports nothing and fails nothing, and an
// untracked decode behaves as encoding/xml does.
func TestREQ175_MissingMembersReportOnly(t *testing.T) {
	const doc = `<composition xmlns="http://schemas.openehr.org/v1"><content/></composition>`
	got, err := decodeMissing(doc, compositionTarget(), true)
	if err != nil {
		t.Errorf("strict Decode = %v; want nil: a missing member is never refused", err)
	}
	if want := []string{"/content/0/language", "/content/0/data", "/name", "/archetype_node_id", "/language"}; !slices.Equal(got, want) {
		t.Errorf("strict missing members = %q; want %q", got, want)
	}

	var unmatched, missing []string
	tr := NewTracker(func(p string) { unmatched = append(unmatched, p) }, false)
	tr.ReportMissing(func(p string) { missing = append(missing, p) })
	withAttr := `<composition xmlns="http://schemas.openehr.org/v1" foo="1" archetype_node_id="at0000"><content/></composition>`
	if err := tr.Decode(tr.NewDecoder(strings.NewReader(withAttr)), compositionTarget()); err != nil {
		t.Fatalf("Decode = %v; want nil", err)
	}
	if want := []string{"/composition[1]/@foo"}; !slices.Equal(unmatched, want) {
		t.Errorf("unmatched = %q; want %q: Enter checks the attributes as CheckUnmatchedAttrs did", unmatched, want)
	}
	if want := []string{"/content/0/language", "/content/0/data", "/name", "/language"}; !slices.Equal(missing, want) {
		t.Errorf("missing members = %q; want %q", missing, want)
	}

	tr = NewTracker(func(p string) { t.Errorf("unmatched %q reported; want nothing", p) }, true)
	if err := tr.Decode(tr.NewDecoder(strings.NewReader(doc)), compositionTarget()); err != nil {
		t.Errorf("tracked Decode without the hook = %v; want nil", err)
	}
	if err := xml.NewDecoder(strings.NewReader(doc)).Decode(compositionTarget()); err != nil {
		t.Errorf("untracked Decode = %v; want nil", err)
	}
	if err := Enter(xml.NewDecoder(strings.NewReader(doc)), xml.StartElement{}, compClass); err != nil {
		t.Errorf("Enter(untracked decoder) = %v; want nil", err)
	}
}

// TestREQ175_MissingMembersAfterFailure pins that a decode that fails makes
// no promise beyond the members reported before the failure, and that the
// failure comes back as itself.
func TestREQ175_MissingMembersAfterFailure(t *testing.T) {
	const doc = `<composition xmlns="http://schemas.openehr.org/v1"><content><data><events/></data></content><content>`
	got, err := decodeMissing(doc, compositionTarget(), false)
	if err == nil {
		t.Fatal("Decode(truncated document) = nil; want a syntax error")
	}
	if errors.Is(err, ErrUnknownElement) {
		t.Errorf("Decode(truncated document) = %v; want the syntax error, not a refusal", err)
	}
	want := []string{"/content/0/data/events/0/time", "/content/0/data/events/0/data", "/content/0/data/origin", "/content/0/language"}
	if !slices.Equal(got, want) {
		t.Errorf("missing members before the failure = %q; want %q", got, want)
	}
}

// TestREQ175_EnterCostsNothingWithoutTheHook pins the cost path: a tracked
// decode that does not ask for missing members allocates no more through
// Enter than it did through CheckUnmatchedAttrs, so the report costs nothing
// to a decode that only reports or refuses unmatched content.
func TestREQ175_EnterCostsNothingWithoutTheHook(t *testing.T) {
	doc := `<composition ` + xmlnsDecl + ` archetype_node_id="at0000">
  <name><value>v</value></name><language>en</language>
  <content><language>en</language><data><origin>o</origin><events><time>t</time><data/></events></data></content>
  <content><language>en</language><data><origin>o</origin><events><time>t</time><data/></events></data></content>
</composition>`
	decode := func(target func() any) float64 {
		return testing.AllocsPerRun(50, func() {
			tr := NewTracker(func(string) {}, false)
			if err := tr.Decode(tr.NewDecoder(strings.NewReader(doc)), target()); err != nil {
				t.Fatalf("Decode = %v; want nil", err)
			}
		})
	}
	withEnter := decode(func() any { return compositionTarget() })
	withPlain := decode(func() any { return (*plainEntry)(compositionTarget()) })
	if withEnter != withPlain {
		t.Errorf("a tracked decode without the hook allocates %v times through Enter and %v through CheckUnmatchedAttrs; want the same", withEnter, withPlain)
	}
}

// TestEnter_REQ175_RefusesMoreThan64Required pins the cap a frame's seen word
// gives Class.Required: a description with a 65th name would report that
// member missing whatever the document carries, so Enter refuses it before
// any decode, tracker or not, while one with exactly 64 names is accepted.
// Removing the guard makes the second half fail: Enter returns nil.
func TestEnter_REQ175_RefusesMoreThan64Required(t *testing.T) {
	names := func(n int) []string {
		s := make([]string, n)
		for i := range s {
			s[i] = "m" + strconv.Itoa(i)
		}
		return s
	}
	start := xml.StartElement{Name: xml.Name{Local: "a"}}
	dec := xml.NewDecoder(strings.NewReader("<a/>"))
	if err := Enter(dec, start, &Class{Name: "WIDE", Required: names(maxRequired)}); err != nil {
		t.Fatalf("Enter(class with %d required members) = %v, want nil", maxRequired, err)
	}
	wide := &Class{Name: "TOO_WIDE", Required: names(maxRequired + 1)}
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("Enter(class with %d required members) returned; want a panic", len(wide.Required))
		}
		msg, _ := r.(string)
		if !strings.Contains(msg, "TOO_WIDE") || !strings.Contains(msg, strconv.Itoa(maxRequired+1)) {
			t.Errorf("Enter panic = %v; want it to name the class and its count", r)
		}
	}()
	_ = Enter(dec, start, wide)
}
