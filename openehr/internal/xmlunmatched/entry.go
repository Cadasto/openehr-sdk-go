package xmlunmatched

import (
	"encoding/xml"
	"slices"
	"strings"
)

// DecodeEntry decodes the element start opens, one entry of a `Hash` member,
// into m under the key the entry's XML attribute attr holds: the `id` of a
// StringDictionaryItem, the shape of a `Hash<String,String>`, whose value is
// the element's text, or the key attribute a class value's ITS-XML schema
// type declares, whose value the class's own decoder reads. It allocates m
// when m is nil. start must be the token the decoder read last.
//
// The key attribute is taken off before the value is read, so it is never
// unmatched content. A String value reads as the text of a primitive
// element: a tracked decode reports or refuses any other attribute and any
// child element as unmatched. An entry with no key attribute is kept under
// the empty key, and a later entry with the key of an earlier one replaces it.
//
// For the missing-member report ([Tracker.ReportMissing]), an entry is a value
// of the member whatever its text, and the members inside a class value are
// named under the entry's key.
func DecodeEntry[V any](dec *xml.Decoder, start *xml.StartElement, attr string, m *map[string]V) error {
	key, entry := withoutKey(start, attr)
	setEntryKey(dec, func() string { return key })
	var v V
	if err := decodeEntryValue(dec, &entry, &v); err != nil {
		return err
	}
	putEntry(m, key, v)
	return nil
}

// DecodeEntryKeyedBy decodes the element start opens, one entry of a `Hash`
// member whose class value carries its own key, such as the language of a
// RESOURCE_DESCRIPTION_ITEM, into m under the key keyOf reads from the decoded
// value. It allocates m when m is nil. start must be the token the decoder
// read last. A later entry with the key of an earlier one replaces it.
//
// For the missing-member report ([Tracker.ReportMissing]), the members inside
// the value are named under the key keyOf reads from the value as decoded so
// far, which holds the key once the element that carries it has been read.
func DecodeEntryKeyedBy[V any](dec *xml.Decoder, start *xml.StartElement, keyOf func(*V) string, m *map[string]V) error {
	var v V
	setEntryKey(dec, func() string { return keyOf(&v) })
	if err := decodeEntryValue(dec, start, &v); err != nil {
		return err
	}
	putEntry(m, keyOf(&v), v)
	return nil
}

// withoutKey returns the value of start's attribute attr in no namespace, the
// entry's key, and start without it.
func withoutKey(start *xml.StartElement, attr string) (string, xml.StartElement) {
	entry := *start
	i := slices.IndexFunc(start.Attr, func(a xml.Attr) bool { return a.Name.Space == "" && a.Name.Local == attr })
	if i < 0 {
		return "", entry
	}
	entry.Attr = slices.Delete(slices.Clone(start.Attr), i, i+1)
	return start.Attr[i].Value, entry
}

// setEntryKey records key as the way to read the key of the entry the decoder
// read last, when the decode reports missing members: the members inside the
// entry are then named under it.
func setEntryKey(dec *xml.Decoder, key func() string) {
	if t := trackerOf(dec); t != nil && t.missing != nil {
		t.open[len(t.open)-1].key = key
	}
}

// decodeEntryValue decodes the element start opens into v: a String value as
// its text, which a tracked decode reads with [Tracker.readText], so an empty
// value is still a value; any other value through its own decoder.
func decodeEntryValue(dec *xml.Decoder, start *xml.StartElement, v any) error {
	s, ok := v.(*string)
	if !ok {
		return dec.DecodeElement(v, start)
	}
	if t := trackerOf(dec); t != nil {
		return t.readText(dec, start, s)
	}
	return dec.DecodeElement(s, start)
}

// putEntry adds v to m under key, allocating m when it is nil.
func putEntry[V any](m *map[string]V, key string, v V) {
	if *m == nil {
		*m = make(map[string]V)
	}
	(*m)[key] = v
}

// pointerKey escapes key as one reference token of a JSON Pointer (RFC 6901):
// `~` as `~0` and `/` as `~1`.
var pointerKey = strings.NewReplacer("~", "~0", "/", "~1")
