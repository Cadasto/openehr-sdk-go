package xmlunmatched

import (
	"encoding/xml"
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// TestREQ170_TrackerPaths pins the path arithmetic of a tracked decode: one
// `/name[n]` segment per open element, n counting the earlier siblings of the
// same name, a closed element leaving its siblings' counts in place, and a
// new value starting its paths afresh.
func TestREQ170_TrackerPaths(t *testing.T) {
	tr := NewTracker(nil, false)
	var got []string
	step := func(open ...string) {
		for _, name := range open {
			if name == "" {
				tr.pop()
				continue
			}
			tr.push(name)
		}
		got = append(got, tr.path())
	}
	step("a")             // /a[1]
	step("b")             // /a[1]/b[1]
	step("", "b")         // /a[1]/b[2]
	step("", "c")         // /a[1]/c[1]
	step("", "b", "b")    // /a[1]/b[3]/b[1]
	step("", "", "", "a") // /a[2]: a second top-level element of the same value
	tr.reset()
	step("", "a") // a new value: /a[1] again
	want := []string{"/a[1]", "/a[1]/b[1]", "/a[1]/b[2]", "/a[1]/c[1]", "/a[1]/b[3]/b[1]", "/a[2]", "/a[1]"}
	if !slices.Equal(got, want) {
		t.Errorf("tracker paths = %q; want %q", got, want)
	}
}

// TestREQ170_WideElementCountsByMap pins that an element with many
// differently named children costs a bounded amount of work per child: its
// frame keeps at most linearChildNames names in the slice it scans and moves
// the counts to a map past that, so a report or strict decode of a document
// with many distinct sibling names grows linearly, not quadratically, with
// its size. The positions stay right across the move, a name seen before it
// keeps counting, and a frame reused for a later element starts without the
// map.
func TestREQ170_WideElementCountsByMap(t *testing.T) {
	const n = 1000
	tr := NewTracker(nil, false)
	tr.push("root")
	child := func(name string) string {
		tr.push(name)
		defer tr.pop()
		return tr.path()
	}
	for i := range n {
		if got, want := child("x"+strconv.Itoa(i)), "/root[1]/x"+strconv.Itoa(i)+"[1]"; got != want {
			t.Fatalf("path of child %d = %q; want %q", i, got, want)
		}
	}
	root := &tr.open[1]
	if len(root.children) > linearChildNames {
		t.Errorf("the frame of an element with %d distinct child names scans %d names; want at most %d, the rest counted by map",
			n, len(root.children), linearChildNames)
	}
	if len(root.byName) != n {
		t.Errorf("the frame's map counts %d names; want all %d once the element outgrew the slice", len(root.byName), n)
	}
	// A name counted in the slice before the move, and one counted after it.
	for _, tc := range []struct{ name, want string }{
		{"x0", "/root[1]/x0[2]"},
		{"x" + strconv.Itoa(n-1), "/root[1]/x" + strconv.Itoa(n-1) + "[2]"},
		{"fresh", "/root[1]/fresh[1]"},
	} {
		if got := child(tc.name); got != tc.want {
			t.Errorf("path of another %s = %q; want %q", tc.name, got, tc.want)
		}
	}

	// The frame's slot, reused for the next element, starts afresh.
	tr.pop()
	tr.push("root")
	if tr.open[1].byName != nil || len(tr.open[1].children) != 0 {
		t.Errorf("a reused frame keeps %d names and a map of %d; want neither", len(tr.open[1].children), len(tr.open[1].byName))
	}
	if got, want := child("x0"), "/root[2]/x0[1]"; got != want {
		t.Errorf("path in the next element = %q; want %q", got, want)
	}
}

// readsNothing is a decode target whose UnmarshalXML reads none of its
// content, so every attribute and child element is unmatched.
type readsNothing struct{}

func (*readsNothing) UnmarshalXML(dec *xml.Decoder, start xml.StartElement) error {
	if err := CheckUnmatchedAttrs(dec, start); err != nil {
		return err
	}
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch tk := tok.(type) {
		case xml.StartElement:
			if err := SkipUnmatched(dec, tk); err != nil {
				return err
			}
		case xml.EndElement:
			return nil
		}
	}
}

// TestREQ170_TrackedDecodeUnregisters pins that a tracked decode registers
// its decoder for the decode only: after a decode that reports, one that is
// refused and one that fails on malformed input, no decoder is left
// registered and the in-progress count is back at zero, so an untracked
// decode skips the lookup again and no decoder is kept alive.
func TestREQ170_TrackedDecodeUnregisters(t *testing.T) {
	decode := func(doc string, report func(string), strict bool) error {
		tr := NewTracker(report, strict)
		return tr.Decode(tr.NewDecoder(strings.NewReader(doc)), new(readsNothing))
	}
	const doc = `<r a="1"><x/></r>`
	var got []string
	if err := decode(doc, func(p string) { got = append(got, p) }, false); err != nil {
		t.Fatalf("Decode(reporting) = %v; want nil", err)
	}
	if want := []string{"/r[1]/@a", "/r[1]/x[1]"}; !slices.Equal(got, want) {
		t.Errorf("reported %q; want %q", got, want)
	}
	if err := decode(doc, nil, true); !errors.Is(err, ErrUnknownElement) {
		t.Errorf("Decode(strict) = %v; want a refusal wrapping ErrUnknownElement", err)
	}
	if err := decode(`<r><x>`, func(string) {}, false); err == nil {
		t.Error("Decode(truncated document) = nil; want a syntax error")
	}
	if n := tracking.Load(); n != 0 {
		t.Errorf("tracked decodes in progress = %d after every decode returned; want 0", n)
	}
	left := 0
	trackers.Range(func(_, _ any) bool { left++; return true })
	if left != 0 {
		t.Errorf("%d decoders still registered after every decode returned; want none", left)
	}
	if trackerOf(nil) != nil {
		t.Error("trackerOf answered a tracker with no tracked decode in progress; want nil")
	}
}

// TestREQ170_RefusalMessage pins the text and classification of a strict
// refusal: the sentinel's text and the quoted path, and for a foreign element
// both sentinels.
func TestREQ170_RefusalMessage(t *testing.T) {
	plain := &unknownElementError{path: "/a[1]/@x"}
	if got, want := plain.Error(), `canxml: unknown element or attribute "/a[1]/@x"`; got != want {
		t.Errorf("Error() = %q; want %q", got, want)
	}
	if !errors.Is(plain, ErrUnknownElement) || errors.Is(plain, ErrNamespace) {
		t.Errorf("refusal of an attribute: errors.Is ErrUnknownElement = %v, ErrNamespace = %v; want true, false",
			errors.Is(plain, ErrUnknownElement), errors.Is(plain, ErrNamespace))
	}
	foreign := &unknownElementError{path: "/a[1]/b[1]", foreign: true}
	if !errors.Is(foreign, ErrUnknownElement) || !errors.Is(foreign, ErrNamespace) {
		t.Errorf("refusal of a foreign element: want errors.Is both ErrUnknownElement and ErrNamespace; got %v", foreign)
	}
	if !strings.Contains(foreign.Error(), `"/a[1]/b[1]"`) {
		t.Errorf("Error() = %q; want the quoted path", foreign.Error())
	}
}

// elementsOnly is a decode target whose content is elements, read as a
// generated decoder reads it: each child element decodes into another
// elementsOnly, and the text between them goes to CheckUnmatchedText.
type elementsOnly struct{}

func (*elementsOnly) UnmarshalXML(dec *xml.Decoder, _ xml.StartElement) error {
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch tk := tok.(type) {
		case xml.StartElement:
			if err := dec.DecodeElement(new(elementsOnly), &tk); err != nil {
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

// TestREQ170_TextInElementContent pins the tracker's rule for text in an
// element whose content is elements: text other than XML whitespace is one
// item per element, at the element's path with a last `/text()` segment,
// however many runs it holds, a CDATA section among them; a later element
// whose frame reuses the slot of one already reported starts afresh; XML
// whitespace never counts; strict refuses the first item without its text;
// and an untracked decode does nothing.
func TestREQ170_TextInElementContent(t *testing.T) {
	const doc = "<r>t1<x>t2</x>t3<x> \t\r\n</x><x>t4<y>t5</y>t6<![CDATA[t7]]></x><![CDATA[t8]]></r>"
	var got []string
	tr := NewTracker(func(p string) { got = append(got, p) }, false)
	if err := tr.Decode(tr.NewDecoder(strings.NewReader(doc)), new(elementsOnly)); err != nil {
		t.Fatalf("Decode(reporting) = %v; want nil", err)
	}
	want := []string{"/r[1]/text()", "/r[1]/x[1]/text()", "/r[1]/x[3]/text()", "/r[1]/x[3]/y[1]/text()"}
	if !slices.Equal(got, want) {
		t.Errorf("reported %q; want %q", got, want)
	}

	tr = NewTracker(nil, true)
	err := tr.Decode(tr.NewDecoder(strings.NewReader(doc)), new(elementsOnly))
	if !errors.Is(err, ErrUnknownElement) || errors.Is(err, ErrNamespace) {
		t.Fatalf("Decode(strict) = %v; want a refusal wrapping ErrUnknownElement only", err)
	}
	if msg := err.Error(); !strings.Contains(msg, `"/r[1]/text()"`) || strings.Contains(msg, "t1") {
		t.Errorf("Decode(strict) = %q; want the quoted path /r[1]/text() and not the text", msg)
	}

	if err := xml.NewDecoder(strings.NewReader(doc)).Decode(new(elementsOnly)); err != nil {
		t.Errorf("untracked Decode = %v; want nil", err)
	}
	if err := CheckUnmatchedText(xml.NewDecoder(strings.NewReader(doc)), xml.CharData("t1")); err != nil {
		t.Errorf("CheckUnmatchedText(untracked decoder) = %v; want nil", err)
	}
}
