package xmlunmatched

// scope_internal_test.go: the namespace scope a tracked or scoped decode keeps,
// which canxml resolves the prefix of an `xsi:type` against where the
// attribute stands (REQ-056, wire.md § REQ-056), and the second decode
// Unmarshal runs when an untracked decode cannot resolve one.

import (
	"encoding/xml"
	"slices"
	"strings"
	"testing"
)

// prefixProbe records, for every element it reads, the element's local name
// and what LookupPrefix answers for the prefixes `p` and `q` there.
type prefixProbe struct{ got []string }

// lookups describes what LookupPrefix answers for prefix at start.
func lookups(dec *xml.Decoder, start xml.StartElement, prefix string) string {
	uri, bound, known := LookupPrefix(dec, start, prefix)
	switch {
	case !known:
		return prefix + "?"
	case !bound:
		return prefix + "-"
	}
	return prefix + "=" + uri
}

func (p *prefixProbe) UnmarshalXML(dec *xml.Decoder, start xml.StartElement) error {
	p.got = append(p.got, start.Name.Local+" "+lookups(dec, start, "p")+" "+lookups(dec, start, "q"))
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch tk := tok.(type) {
		case xml.StartElement:
			if err := p.UnmarshalXML(dec, tk.Copy()); err != nil {
				return err
			}
		case xml.EndElement:
			return nil
		}
	}
}

// scopeDoc declares `p` on the root, again on `b`, whose child `c` inherits
// it, and nowhere else; `d` follows `b`'s end, where the root's binding holds
// again. No element declares `q`.
const scopeDoc = `<root xmlns:p="urn:root"><a><b xmlns:p="urn:b"><c/></b><d/></a></root>`

// TestREQ056_LookupPrefixResolvesInScope pins LookupPrefix: a tracked decode
// and a scoped one resolve a prefix against the innermost declaration in scope
// where an element stands, its own or an ancestor's, drop an element's
// declarations as it closes, and answer unbound for a prefix no element in
// scope declares; a plain encoding/xml decoder knows only what an element
// declares itself.
func TestREQ056_LookupPrefixResolvesInScope(t *testing.T) {
	inScope := []string{
		"root p=urn:root q-", "a p=urn:root q-", "b p=urn:b q-", "c p=urn:b q-", "d p=urn:root q-",
	}
	decodes := map[string]struct {
		decode func(*prefixProbe) error
		want   []string
	}{
		"tracked": {func(p *prefixProbe) error {
			tr := NewTracker(nil, false)
			return tr.Decode(tr.NewDecoder(strings.NewReader(scopeDoc)), p)
		}, inScope},
		"scoped": {func(p *prefixProbe) error {
			var s Scope
			return s.Decode(s.NewDecoder(strings.NewReader(scopeDoc)), p)
		}, inScope},
		"plain": {func(p *prefixProbe) error {
			return xml.NewDecoder(strings.NewReader(scopeDoc)).Decode(p)
		}, []string{"root p=urn:root q?", "a p? q?", "b p=urn:b q?", "c p? q?", "d p? q?"}},
	}
	for name, d := range decodes {
		var p prefixProbe
		if err := d.decode(&p); err != nil {
			t.Fatalf("%s decode: %v", name, err)
		}
		if !slices.Equal(p.got, d.want) {
			t.Errorf("%s decode: LookupPrefix answered\n%q\nwant\n%q", name, p.got, d.want)
		}
	}
}

// prefixedList reads as a generated decoder reads a class with a list member,
// `item`, each item the namespace its `type` attribute's prefix is bound to,
// which, like canxml's dispatch, it refuses with UnresolvedPrefix when the
// decode cannot tell.
type prefixedList struct{ items []string }

var prefixedListClass = &Class{Name: "PREFIXED_LIST", Items: []string{"item"}}

func (l *prefixedList) UnmarshalXML(dec *xml.Decoder, start xml.StartElement) error {
	if fresh, err := Enter(dec, start, prefixedListClass); err != nil {
		return err
	} else if fresh {
		*l = prefixedList{}
	}
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch tk := tok.(type) {
		case xml.StartElement:
			if LocalName(tk.Name) != "item" {
				if err := SkipUnmatched(dec, tk); err != nil {
					return err
				}
				continue
			}
			prefix, _, _ := strings.Cut(tk.Attr[0].Value, ":")
			uri, _, known := LookupPrefix(dec, tk, prefix)
			if !known {
				return UnresolvedPrefix()
			}
			l.items = append(l.items, uri)
			if err := dec.Skip(); err != nil {
				return err
			}
		case xml.EndElement:
			return nil
		}
	}
}

// TestREQ056_UnmarshalDecodesAgainForUnresolvedPrefix pins the second decode
// of an untracked Unmarshal that meets a prefix no element it can see
// declares: it reads the document again through a decoder that keeps the
// namespace scope, into the value cleared, so the value holds each item once,
// as a tracked decode reads it; the first item, whose element declares its
// prefix itself, the untracked decode read before it gave up.
func TestREQ056_UnmarshalDecodesAgainForUnresolvedPrefix(t *testing.T) {
	const doc = `<list xmlns:r="urn:root"><item type="o:x" xmlns:o="urn:own"/><item type="r:x"/><item type="r:y"/></list>`
	want := []string{"urn:own", "urn:root", "urn:root"}
	for mode, tracker := range documentModes() {
		var l prefixedList
		if err := Unmarshal([]byte(doc), &l, tracker()); err != nil {
			t.Fatalf("Unmarshal(%s) = %v", mode, err)
		}
		if !slices.Equal(l.items, want) {
			t.Errorf("Unmarshal(%s) items = %q; want %q", mode, l.items, want)
		}
	}
}
