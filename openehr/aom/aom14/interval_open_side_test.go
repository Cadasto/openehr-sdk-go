package aom14_test

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// The member order of an AOM 1.4 constraint interval on the wire. JSON follows
// the Go struct declaration; XML follows the BMM property order, bounds first.
var (
	intervalJSONMembers = []string{"lower", "lower_included", "lower_unbounded", "upper", "upper_included", "upper_unbounded"}
	intervalXMLElements = []string{"lower", "upper", "lower_unbounded", "upper_unbounded", "lower_included", "upper_included"}
)

// without returns names minus the listed ones, order kept.
func without(names []string, drop ...string) []string {
	var out []string
	for _, n := range names {
		if !slices.Contains(drop, n) {
			out = append(out, n)
		}
	}
	return out
}

// openSideCase is one AOM 1.4 interval and the members its wire must carry.
type openSideCase struct {
	name string
	iv   rm.Interval[aom14.Integer]
	// wantJSON and wantXML list the interval's members, in order.
	wantJSON, wantXML []string
}

// openSideCases are the intervals of REQ-052 and REQ-056 over an AOM 1.4
// `occurrences`: an open side with its zero bound, an open side that still
// carries a bound, and a closed side whose bound is zero.
func openSideCases() []openSideCase {
	return []openSideCase{
		{
			name:     "open upper side, zero bound omitted (0..*)",
			iv:       rm.Interval[aom14.Integer]{Lower: 0, LowerIncluded: true, UpperUnbounded: true},
			wantJSON: without(intervalJSONMembers, "upper"),
			wantXML:  without(intervalXMLElements, "upper"),
		},
		{
			name:     "open upper side after a non-zero lower (1..*)",
			iv:       rm.Interval[aom14.Integer]{Lower: 1, LowerIncluded: true, UpperUnbounded: true},
			wantJSON: without(intervalJSONMembers, "upper"),
			wantXML:  without(intervalXMLElements, "upper"),
		},
		{
			name:     "open lower side, zero bound omitted (*..3)",
			iv:       rm.Interval[aom14.Integer]{LowerUnbounded: true, Upper: 3, UpperIncluded: true},
			wantJSON: without(intervalJSONMembers, "lower"),
			wantXML:  without(intervalXMLElements, "lower"),
		},
		{
			name:     "both sides open, both zero bounds omitted (*..*)",
			iv:       rm.Interval[aom14.Integer]{LowerUnbounded: true, UpperUnbounded: true},
			wantJSON: without(intervalJSONMembers, "lower", "upper"),
			wantXML:  without(intervalXMLElements, "lower", "upper"),
		},
		{
			name:     "non-empty bound beside its own open flag is kept",
			iv:       rm.Interval[aom14.Integer]{Lower: 1, LowerIncluded: true, Upper: 5, UpperUnbounded: true},
			wantJSON: intervalJSONMembers,
			wantXML:  intervalXMLElements,
		},
		{
			name:     "closed zero bounds are real bounds and are kept (0..0)",
			iv:       rm.Interval[aom14.Integer]{Lower: 0, LowerIncluded: true, Upper: 0, UpperIncluded: true},
			wantJSON: intervalJSONMembers,
			wantXML:  intervalXMLElements,
		},
		{
			name:     "closed lower zero beside an open upper side (lower kept, upper omitted)",
			iv:       rm.Interval[aom14.Integer]{Lower: 0, UpperUnbounded: true},
			wantJSON: without(intervalJSONMembers, "upper"),
			wantXML:  without(intervalXMLElements, "upper"),
		},
	}
}

// TestREQ052AOM14OccurrencesOpenSideJSON pins the REQ-052 rule for the
// abstract BASE Interval that AOM 1.4 holds in `occurrences`: an open side's
// empty bound is left out of canonical JSON, a non-empty bound beside its own
// flag and a closed side's zero bound are kept, and the output decodes back to
// the same interval.
func TestREQ052AOM14OccurrencesOpenSideJSON(t *testing.T) {
	for _, tc := range openSideCases() {
		t.Run(tc.name, func(t *testing.T) {
			obj := &aom14.CComplexObject{NodeID: "at0000", RMTypeName: "OBSERVATION", Occurrences: tc.iv}
			b, err := canjson.Marshal(obj)
			if err != nil {
				t.Fatalf("canjson.Marshal: %v", err)
			}
			got := jsonMemberNames(t, b, "occurrences")
			if !slices.Equal(got, tc.wantJSON) {
				t.Errorf("occurrences members = %v, want %v\nwire: %s", got, tc.wantJSON, b)
			}
			back := &aom14.CComplexObject{}
			if err := canjson.Unmarshal(b, back); err != nil {
				t.Fatalf("canjson.Unmarshal: %v\nwire: %s", err, b)
			}
			if back.Occurrences != tc.iv {
				t.Errorf("round trip: occurrences = %+v, want %+v", back.Occurrences, tc.iv)
			}
		})
	}
}

// TestREQ056AOM14OccurrencesOpenSideXML pins the REQ-056 reading for the same
// interval in canonical XML: snake_case element names in BMM property order,
// an open side's empty bound ABSENT, and the output decodes back to the same
// interval.
func TestREQ056AOM14OccurrencesOpenSideXML(t *testing.T) {
	for _, tc := range openSideCases() {
		t.Run(tc.name, func(t *testing.T) {
			obj := &aom14.CComplexObject{NodeID: "at0000", RMTypeName: "OBSERVATION", Occurrences: tc.iv}
			b, err := canxml.Marshal(obj)
			if err != nil {
				t.Fatalf("canxml.Marshal: %v", err)
			}
			got := xmlChildNames(t, b, "occurrences")
			if !slices.Equal(got, tc.wantXML) {
				t.Errorf("occurrences elements = %v, want %v\nwire: %s", got, tc.wantXML, b)
			}
			back := &aom14.CComplexObject{}
			if err := canxml.Unmarshal(b, back); err != nil {
				t.Fatalf("canxml.Unmarshal: %v\nwire: %s", err, b)
			}
			if back.Occurrences != tc.iv {
				t.Errorf("round trip: occurrences = %+v, want %+v", back.Occurrences, tc.iv)
			}
		})
	}
}

// TestREQ052REQ056AOM14IntervalHolders runs the rule through the other AOM 1.4
// fields that hold an abstract Interval: an optional `range` (a pointer) of an
// Integer, a Real and a string, and a mandatory `existence`.
func TestREQ052REQ056AOM14IntervalHolders(t *testing.T) {
	cases := []struct {
		name   string
		value  any
		member string
	}{
		{"C_INTEGER range", &aom14.CInteger{Range: &rm.Interval[aom14.Integer]{Lower: 0, LowerIncluded: true, UpperUnbounded: true}}, "range"},
		{"C_REAL range", &aom14.CReal{Range: &rm.Interval[aom14.Real]{Lower: 0, LowerIncluded: true, UpperUnbounded: true}}, "range"},
		{"C_DATE range", &aom14.CDate{Range: &rm.Interval[string]{Lower: "2026-01-01", LowerIncluded: true, UpperUnbounded: true}}, "range"},
		{"C_SINGLE_ATTRIBUTE existence", &aom14.CSingleAttribute{RMAttributeName: "items", Existence: rm.Interval[aom14.Integer]{Lower: 0, LowerIncluded: true, UpperUnbounded: true}}, "existence"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			js, err := canjson.Marshal(tc.value)
			if err != nil {
				t.Fatalf("canjson.Marshal: %v", err)
			}
			if got := jsonMemberNames(t, js, tc.member); slices.Contains(got, "upper") {
				t.Errorf("%s JSON members = %v: an open upper side with an empty bound must carry no `upper`\nwire: %s", tc.member, got, js)
			}
			xs, err := canxml.Marshal(tc.value)
			if err != nil {
				t.Fatalf("canxml.Marshal: %v", err)
			}
			got := xmlChildNames(t, xs, tc.member)
			if slices.Contains(got, "upper") || slices.Contains(got, "Upper") {
				t.Errorf("%s XML elements = %v: an open upper side with an empty bound must carry no `upper`\nwire: %s", tc.member, got, xs)
			}
			for _, name := range got {
				if name != "lower" && name != "upper" && name != "lower_unbounded" && name != "upper_unbounded" && name != "lower_included" && name != "upper_included" {
					t.Errorf("%s XML element %q is not a snake_case BMM property name\nwire: %s", tc.member, name, xs)
				}
			}
		})
	}
}

// corpusInterval is one constraint interval read from the vendored OPT 1.4
// corpus, parsed here without the code under test.
type corpusInterval struct {
	// name is the file, the interval's position in it and its element name.
	name string
	// inner is the XML between the interval's tags, as the corpus spells it.
	inner string
	// want is the interval the corpus states.
	want rm.Interval[aom14.Integer]
	// hasLower reports whether the corpus gives a lower bound.
	hasLower bool
}

// problems collects the failures of a corpus walk by kind, so a regression
// reads as a count and one example per kind, not one line per interval.
type problems struct {
	count   map[string]int
	example map[string]string
}

func (p *problems) add(kind, example string) {
	if p.count == nil {
		p.count, p.example = map[string]int{}, map[string]string{}
	}
	if p.count[kind] == 0 {
		p.example[kind] = example
	}
	p.count[kind]++
}

func (p *problems) report(t *testing.T) {
	t.Helper()
	for kind, n := range p.count {
		t.Errorf("%s: %d intervals; first: %s", kind, n, p.example[kind])
	}
}

// TestREQ052AOM14CorpusIntervalsJSON round-trips every constraint interval of
// the vendored OPT 1.4 corpus (`occurrences`, `existence` and a cardinality's
// `interval`: the ADL 1.4 constraint trees the AOM 1.4 types model) through
// canonical JSON. An open side carries no `lower` or `upper` beside its flag, a
// closed side keeps its bound, zero included, and the output decodes back to
// the interval the corpus states.
func TestREQ052AOM14CorpusIntervalsJSON(t *testing.T) {
	var bad problems
	for _, ci := range corpusIntervals(t) {
		obj := &aom14.CComplexObject{NodeID: "at0000", RMTypeName: "OBSERVATION", Occurrences: ci.want}
		js, err := canjson.Marshal(obj)
		if err != nil {
			t.Fatalf("%s: canjson.Marshal: %v", ci.name, err)
		}
		members := jsonMemberNames(t, js, "occurrences")
		if slices.Contains(members, "lower") == ci.want.LowerUnbounded {
			bad.add("JSON `lower` member present beside an open flag, or missing on a closed side", ci.name+": "+string(js))
		}
		if slices.Contains(members, "upper") == ci.want.UpperUnbounded {
			bad.add("JSON `upper` member present beside an open flag, or missing on a closed side", ci.name+": "+string(js))
		}
		back := &aom14.CComplexObject{}
		if err := canjson.Unmarshal(js, back); err != nil {
			t.Fatalf("%s: canjson.Unmarshal: %v", ci.name, err)
		}
		if back.Occurrences != ci.want {
			bad.add("JSON round trip changed the interval", ci.name+": "+string(js))
		}
	}
	bad.report(t)
}

// TestREQ056AOM14CorpusIntervalsXML is the same walk for canonical XML: the
// encoder writes snake_case elements in BMM property order, leaves out an open
// side's bound, and its output decodes back to the interval the corpus states.
func TestREQ056AOM14CorpusIntervalsXML(t *testing.T) {
	var bad problems
	var openUpper, closedZero int
	for _, ci := range corpusIntervals(t) {
		openUpper += b2i(ci.want.UpperUnbounded)
		closedZero += b2i(ci.hasLower && !ci.want.LowerUnbounded && ci.want.Lower == 0)
		obj := &aom14.CComplexObject{NodeID: "at0000", RMTypeName: "OBSERVATION", Occurrences: ci.want}
		xs, err := canxml.Marshal(obj)
		if err != nil {
			t.Fatalf("%s: canxml.Marshal: %v", ci.name, err)
		}
		var drop []string
		if ci.want.LowerUnbounded {
			drop = append(drop, "lower")
		}
		if ci.want.UpperUnbounded {
			drop = append(drop, "upper")
		}
		if got, want := xmlChildNames(t, xs, "occurrences"), without(intervalXMLElements, drop...); !slices.Equal(got, want) {
			bad.add("XML elements are not snake_case in BMM order without the open bounds", ci.name+": "+string(xs))
		}
		back := &aom14.CComplexObject{}
		if err := canxml.Unmarshal(xs, back); err != nil {
			t.Fatalf("%s: canxml.Unmarshal: %v", ci.name, err)
		}
		if back.Occurrences != ci.want {
			bad.add("XML round trip changed the interval", ci.name+": "+string(xs))
		}
	}
	bad.report(t)
	if openUpper == 0 || closedZero == 0 {
		t.Fatalf("the corpus held %d open upper sides and %d closed zero lower bounds: the test would pass without exercising the rule", openUpper, closedZero)
	}
}

// TestREQ056AOM14CorpusXMLSpellingDecodes decodes each corpus interval in the
// corpus's own XML spelling, snake_case elements in whatever order the corpus
// writes them, and expects the interval the corpus states.
func TestREQ056AOM14CorpusXMLSpellingDecodes(t *testing.T) {
	var bad problems
	for _, ci := range corpusIntervals(t) {
		doc := "<c_complex_object><rm_type_name>OBSERVATION</rm_type_name><occurrences>" + ci.inner +
			"</occurrences><node_id>at0000</node_id></c_complex_object>"
		got := &aom14.CComplexObject{}
		if err := canxml.Unmarshal([]byte(doc), got); err != nil {
			t.Fatalf("%s: canxml.Unmarshal: %v", ci.name, err)
		}
		if got.Occurrences != ci.want {
			bad.add("decoded interval differs from the corpus", ci.name+": got "+strconv.Quote(sprintInterval(got.Occurrences))+", want "+strconv.Quote(sprintInterval(ci.want)))
		}
	}
	bad.report(t)
}

func sprintInterval(iv rm.Interval[aom14.Integer]) string {
	return strconv.FormatInt(int64(iv.Lower), 10) + ".." + strconv.FormatInt(int64(iv.Upper), 10) +
		" lower_included=" + strconv.FormatBool(iv.LowerIncluded) + " upper_included=" + strconv.FormatBool(iv.UpperIncluded) +
		" lower_unbounded=" + strconv.FormatBool(iv.LowerUnbounded) + " upper_unbounded=" + strconv.FormatBool(iv.UpperUnbounded)
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// corpusIntervals returns the `occurrences`, `existence` and cardinality
// `interval` elements of every OPT in the vendored corpus, each with Integer
// bounds or none.
func corpusIntervals(t *testing.T) []corpusInterval {
	t.Helper()
	files, err := filepath.Glob("../../../testkit/corpus/templates/*.opt")
	if err != nil || len(files) == 0 {
		t.Fatalf("no OPT corpus found (%d files, error %v)", len(files), err)
	}
	var out []corpusInterval
	for _, file := range files {
		out = append(out, readCorpusIntervals(t, file)...)
	}
	return out
}

// readCorpusIntervals returns the constraint intervals of one OPT file.
func readCorpusIntervals(t *testing.T, file string) []corpusInterval {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	type rawInterval struct {
		LowerIncluded  bool    `xml:"lower_included"`
		UpperIncluded  bool    `xml:"upper_included"`
		LowerUnbounded bool    `xml:"lower_unbounded"`
		UpperUnbounded bool    `xml:"upper_unbounded"`
		Lower          *string `xml:"lower"`
		Upper          *string `xml:"upper"`
		Inner          string  `xml:",innerxml"`
	}
	dec := xml.NewDecoder(bytes.NewReader(data))
	var out []corpusInterval
	var parents []string
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		switch el := tok.(type) {
		case xml.StartElement:
			parent := ""
			if len(parents) > 0 {
				parent = parents[len(parents)-1]
			}
			isInterval := el.Name.Local == "occurrences" || el.Name.Local == "existence" ||
				(el.Name.Local == "interval" && parent == "cardinality")
			if !isInterval {
				parents = append(parents, el.Name.Local)
				continue
			}
			var raw rawInterval
			if err := dec.DecodeElement(&raw, &el); err != nil {
				t.Fatalf("read %s: %s: %v", file, el.Name.Local, err)
			}
			ci := corpusInterval{
				name:  filepath.Base(file) + "#" + strconv.Itoa(len(out)) + " " + el.Name.Local,
				inner: raw.Inner,
				want: rm.Interval[aom14.Integer]{
					LowerIncluded:  raw.LowerIncluded,
					UpperIncluded:  raw.UpperIncluded,
					LowerUnbounded: raw.LowerUnbounded,
					UpperUnbounded: raw.UpperUnbounded,
				},
				hasLower: raw.Lower != nil,
			}
			for _, side := range []struct {
				text *string
				into *aom14.Integer
			}{{raw.Lower, &ci.want.Lower}, {raw.Upper, &ci.want.Upper}} {
				if side.text == nil {
					continue
				}
				n, err := strconv.ParseInt(*side.text, 10, 64)
				if err != nil {
					t.Fatalf("read %s: %s bound %q is not an integer", file, el.Name.Local, *side.text)
				}
				*side.into = aom14.Integer(n)
			}
			out = append(out, ci)
		case xml.EndElement:
			parents = parents[:len(parents)-1]
		}
	}
}

// jsonMemberNames returns, in order, the member names of the object found at
// the top-level member `key` of the JSON object b.
func jsonMemberNames(t *testing.T, b []byte, key string) []string {
	t.Helper()
	dec := jsontext.NewDecoder(bytes.NewReader(b))
	if tok, err := dec.ReadToken(); err != nil || tok.Kind() != '{' {
		t.Fatalf("wire is not a JSON object (token %v, error %v): %s", tok, err, b)
	}
	for dec.PeekKind() != '}' {
		name, err := dec.ReadToken()
		if err != nil {
			t.Fatalf("read member name: %v: %s", err, b)
		}
		if name.String() != key {
			if err := dec.SkipValue(); err != nil {
				t.Fatalf("skip member %q: %v: %s", name.String(), err, b)
			}
			continue
		}
		if tok, err := dec.ReadToken(); err != nil || tok.Kind() != '{' {
			t.Fatalf("member %q is not an object (token %v, error %v): %s", key, tok, err, b)
		}
		var names []string
		for dec.PeekKind() != '}' {
			n, err := dec.ReadToken()
			if err != nil {
				t.Fatalf("read member of %q: %v: %s", key, err, b)
			}
			names = append(names, n.String())
			if err := dec.SkipValue(); err != nil {
				t.Fatalf("skip member of %q: %v: %s", key, err, b)
			}
		}
		return names
	}
	t.Fatalf("wire has no %q member: %s", key, b)
	return nil
}

// xmlChildNames returns, in order, the child element names of the first child
// element of the root of b that is named key.
func xmlChildNames(t *testing.T, b []byte, key string) []string {
	t.Helper()
	dec := xml.NewDecoder(bytes.NewReader(b))
	depth := 0
	inside := false
	var names []string
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			t.Fatalf("wire has no %q element: %s", key, b)
		}
		if err != nil {
			t.Fatalf("read XML token: %v: %s", err, b)
		}
		switch el := tok.(type) {
		case xml.StartElement:
			depth++
			if depth == 2 && el.Name.Local == key {
				inside = true
			} else if depth == 3 && inside {
				names = append(names, el.Name.Local)
			}
		case xml.EndElement:
			if depth == 2 && inside {
				return names
			}
			depth--
		}
	}
}

// TestREQ056AOM14IntervalLegacyXMLSpellingDecodes pins the decode tolerance
// for the spelling SDK v0.28.0 and earlier wrote for an AOM 1.4 constraint
// interval: Go field names (Lower, LowerIncluded, UpperUnbounded, ...) in place
// of the snake_case BMM names. The decoder reads both onto the same fields, so
// a stored document does not decode to an empty interval without an error. The
// encoder writes snake_case only.
func TestREQ056AOM14IntervalLegacyXMLSpellingDecodes(t *testing.T) {
	want := rm.Interval[aom14.Integer]{Lower: 1, LowerIncluded: true, UpperUnbounded: true}
	legacy := "<c_complex_object><rm_type_name>OBSERVATION</rm_type_name><occurrences>" +
		"<Lower>1</Lower><LowerIncluded>true</LowerIncluded><LowerUnbounded>false</LowerUnbounded>" +
		"<Upper>0</Upper><UpperIncluded>false</UpperIncluded><UpperUnbounded>true</UpperUnbounded>" +
		"</occurrences><node_id>at0000</node_id></c_complex_object>"
	canonical := "<c_complex_object><rm_type_name>OBSERVATION</rm_type_name><occurrences>" +
		"<lower>1</lower><lower_included>true</lower_included><lower_unbounded>false</lower_unbounded>" +
		"<upper>0</upper><upper_included>false</upper_included><upper_unbounded>true</upper_unbounded>" +
		"</occurrences><node_id>at0000</node_id></c_complex_object>"
	for name, doc := range map[string]string{"legacy PascalCase": legacy, "snake_case twin": canonical} {
		t.Run(name, func(t *testing.T) {
			got := &aom14.CComplexObject{}
			if err := canxml.Unmarshal([]byte(doc), got); err != nil {
				t.Fatalf("canxml.Unmarshal: %v", err)
			}
			if got.Occurrences != want {
				t.Errorf("occurrences = %+v, want %+v", got.Occurrences, want)
			}
		})
	}

	// The encoder never writes the legacy names.
	xs, err := canxml.Marshal(&aom14.CComplexObject{NodeID: "at0000", RMTypeName: "OBSERVATION", Occurrences: want})
	if err != nil {
		t.Fatalf("canxml.Marshal: %v", err)
	}
	for _, name := range xmlChildNames(t, xs, "occurrences") {
		if name != strings.ToLower(name) {
			t.Errorf("encoder wrote the legacy element %q: %s", name, xs)
		}
	}
}
