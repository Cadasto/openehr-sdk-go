package xmlunmatched

import (
	"encoding/xml"
	"errors"
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
// unmatched content; an empty one is a key. A String value reads as the text
// of a primitive element: a tracked decode reports or refuses any other
// attribute and any child element as unmatched. An entry with no key
// attribute, or whose key an earlier entry of the member holds, is refused
// ([putEntry]).
//
// For the missing-member report ([Tracker.ReportMissing]), an entry is a value
// of the member whatever its text, and the members inside a class value are
// named under the entry's key.
func DecodeEntry[V any](dec *xml.Decoder, start *xml.StartElement, attr string, m *map[string]V) error {
	key, entry, keyed := withoutKey(start, attr)
	setEntryKey(dec, func() string { return key })
	t, at := beginEntry(dec, m)
	var v V
	if err := decodeEntryValue(dec, &entry, &v); err != nil {
		return err
	}
	return putEntry(t, at, start, m, key, keyed, v)
}

// DecodeEntryKeyedBy decodes the element start opens, one entry of a `Hash`
// member whose class value carries its own key, such as the language of a
// RESOURCE_DESCRIPTION_ITEM, into m under the key keyOf reads from the decoded
// value. It allocates m when m is nil. start must be the token the decoder
// read last. An entry whose value has an empty key, as a language with no
// code has, or whose key an earlier entry of the member holds, is refused
// ([putEntry]).
//
// For the missing-member report ([Tracker.ReportMissing]), the members inside
// the value are named under the key keyOf reads from the value as decoded so
// far, which holds the key once the element that carries it has been read.
func DecodeEntryKeyedBy[V any](dec *xml.Decoder, start *xml.StartElement, keyOf func(*V) string, m *map[string]V) error {
	var v V
	setEntryKey(dec, func() string { return keyOf(&v) })
	t, at := beginEntry(dec, m)
	if err := decodeEntryValue(dec, start, &v); err != nil {
		return err
	}
	key := keyOf(&v)
	return putEntry(t, at, start, m, key, key != "", v)
}

// withoutKey returns the value of start's attribute attr in no namespace, the
// entry's key, start without it, and whether start carries it.
func withoutKey(start *xml.StartElement, attr string) (string, xml.StartElement, bool) {
	entry := *start
	i := slices.IndexFunc(start.Attr, func(a xml.Attr) bool { return a.Name.Space == "" && a.Name.Local == attr })
	if i < 0 {
		return "", entry, false
	}
	entry.Attr = slices.Delete(slices.Clone(start.Attr), i, i+1)
	return start.Attr[i].Value, entry, true
}

// beginEntry returns the tracker of dec, nil for an untracked decode, and the
// place of the entry the decoder read last in a tracked one, before the
// entry's value is read. The first entry of the member in the element that
// holds it starts m afresh in a tracked decode, so that a value decoded a
// second time, as [Unmarshal] decodes a document again to name a failure's
// path, holds no entry of the first decode for a key to repeat. An untracked
// decode, which knows no place, keeps m as it is.
func beginEntry[V any](dec *xml.Decoder, m *map[string]V) (*Tracker, place) {
	t := trackerOf(dec)
	if t == nil {
		return nil, place{}
	}
	at := t.here()
	if at.pos == 1 && len(*m) > 0 {
		*m = nil
	}
	return t, at
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

// putEntry adds v to m under key, the key of the entry start opens, which the
// decoder has read: keyed reports that the entry has one, and t and at are
// the decode's tracker and where the entry sits, as [beginEntry] returned
// them. It allocates m when it is nil.
//
// An entry with no key, or with the key of an earlier entry of the member, is
// refused in every decode, as the canonical-JSON decoder refuses a member name
// an object repeats: keeping one of the two would lose the other's content.
// The refusal is a primitive failure (see "Primitive failures" in the package
// documentation): a *typereg.DecodeError whose cause wraps [ErrInvalidShape],
// which names the entry by its path, from the root of the value in a tracked
// decode and by its name alone in an untracked one, and carries none of its
// key.
func putEntry[V any](t *Tracker, at place, start *xml.StartElement, m *map[string]V, key string, keyed bool, v V) error {
	cause := errEntryNoKey
	if keyed {
		cause = nil
		if _, repeated := (*m)[key]; repeated {
			cause = errEntryRepeatedKey
		}
	}
	if cause != nil {
		if t != nil {
			return t.fail(t.closedPath(at), cause)
		}
		return primitiveFailure("/"+start.Name.Local, cause)
	}
	if *m == nil {
		*m = make(map[string]V)
	}
	(*m)[key] = v
	return nil
}

var (
	// errEntryNoKey is the refusal of a Hash entry with no key: no key
	// attribute, or a value whose own key is empty.
	errEntryNoKey = errors.New("a Hash entry has no key")
	// errEntryRepeatedKey is the refusal of a Hash entry whose key an earlier
	// entry of the member holds. It names no key.
	errEntryRepeatedKey = errors.New("a Hash entry repeats the key of an earlier entry")
)

// pointerKey escapes key as one reference token of a JSON Pointer (RFC 6901):
// `~` as `~0` and `/` as `~1`.
var pointerKey = strings.NewReplacer("~", "~0", "/", "~1")
