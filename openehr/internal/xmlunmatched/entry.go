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
// attribute is refused ([putEntry]). An entry whose key an earlier entry of
// the member holds is unmatched content, as an element the decoder does not
// read is ([SkipUnmatched]): the earlier entry is kept and the repeat is
// skipped unread, so by default the member reads as if the repeat were
// absent, a tracked decode reports the repeat's element as one item, and a
// strict one refuses it.
//
// begun is the decoder's mark of the member in the element being decoded:
// false until the element's first entry of the member, which starts m empty
// and sets it ([beginEntry]).
//
// For the missing-member report ([Tracker.ReportMissing]), an entry is a value
// of the member whatever its text, and the members inside a class value are
// named under the entry's key.
func DecodeEntry[V any](dec *xml.Decoder, start *xml.StartElement, attr string, m *map[string]V, begun *bool) error {
	key, entry, keyed := withoutKey(start, attr)
	setEntryKey(dec, func() string { return key })
	t, at := beginEntry(dec, m, begun)
	if _, repeated := (*m)[key]; keyed && repeated {
		return SkipUnmatched(dec, *start)
	}
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
// code has, is refused ([putEntry]). begun is the decoder's mark of the
// member, as for [DecodeEntry].
//
// An entry whose key an earlier entry of the member holds is unmatched
// content, as for [DecodeEntry]: the earlier entry is kept, so by default the
// member reads as if the repeat were absent, a tracked decode reports the
// repeat's element as one item, and a strict one refuses it. The key lies
// inside the value, so the value is read to find it, and a value that fails
// to decode fails the decode, repeat or not. Meanwhile a tracked decode holds
// back the unmatched content, the missing members and the members sent
// without a value the value holds, and applies its policy to them, in document order, only once the key shows that
// the entry repeats none.
//
// For the missing-member and presence reports ([Tracker.ReportMissing],
// [Tracker.ReportPresence]), the members inside the value are named under the
// key keyOf reads from the value as decoded so far, which holds the key once
// the element that carries it has been read.
func DecodeEntryKeyedBy[V any](dec *xml.Decoder, start *xml.StartElement, keyOf func(*V) string, m *map[string]V, begun *bool) error {
	var v V
	setEntryKey(dec, func() string { return keyOf(&v) })
	t, at := beginEntry(dec, m, begun)
	if t == nil {
		if err := decodeEntryValue(dec, start, &v); err != nil {
			return err
		}
		key := keyOf(&v)
		if _, repeated := (*m)[key]; key != "" && repeated {
			return nil
		}
		return putEntry(nil, at, start, m, key, key != "", v)
	}
	held, err := t.holdEntry(func() error { return decodeEntryValue(dec, start, &v) })
	key := keyOf(&v)
	if _, repeated := (*m)[key]; err == nil && key != "" && repeated {
		return t.unmatched(t.closedPath(at), false)
	}
	if refusal := t.release(held); refusal != nil {
		return refusal
	}
	if err != nil {
		return err
	}
	return putEntry(t, at, start, m, key, key != "", v)
}

// holdEntry runs read, which reads the value of a Hash entry whose key lies
// inside the value, with the unmatched items, the missing members and the
// members sent without a value the value holds held back, in document order,
// and returns them with read's error: only the key, known once the entry has
// closed, tells whether the entry repeats an earlier one, whose element is
// then one unmatched item and whose content counts for nothing. A hold inside
// another adds what it releases to the outer one.
func (t *Tracker) holdEntry(read func() error) ([]heldItem, error) {
	outer := t.held
	var held []heldItem
	t.held = &held
	err := read()
	t.held = outer
	return held, err
}

// release applies the decode's policy to what [Tracker.holdEntry] held, in
// document order: to an unmatched item as [Tracker.unmatched] does, so a
// strict decode refuses the first, to a missing member as the missing-member
// report does, and to a member sent without a value as its report does.
func (t *Tracker) release(held []heldItem) error {
	for _, it := range held {
		if it.missing {
			t.missingMember(it.path)
			continue
		}
		if it.sent {
			t.sentMember(it.path, it.nilled)
			continue
		}
		if err := t.unmatched(it.path, it.foreign); err != nil {
			return err
		}
	}
	return nil
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

// beginEntry starts m empty at the element's first entry of the member, the
// one begun does not mark yet, and marks it, in every decode: the member then
// holds the entries of this element alone, so a key repeats only within the
// element, whatever the value held before. That covers a value a caller
// decodes into twice and the second decode [Unmarshal] runs of a document to
// name a failure's path. beginEntry returns the tracker of dec, nil for an
// untracked decode, and the place of the entry the decoder read last in a
// tracked one, before the entry's value is read.
func beginEntry[V any](dec *xml.Decoder, m *map[string]V, begun *bool) (*Tracker, place) {
	if !*begun {
		*begun = true
		*m = nil
	}
	t := trackerOf(dec)
	if t == nil {
		return nil, place{}
	}
	return t, t.here()
}

// setEntryKey records key as the way to read the key of the entry the decoder
// read last, when the decode reports missing members or members sent without
// a value: the members inside the entry are then named under it.
func setEntryKey(dec *xml.Decoder, key func() string) {
	if t := trackerOf(dec); t != nil && t.checksPresence() {
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
// decoder has read and no earlier entry of the member holds: keyed reports
// that the entry has one, and t and at are the decode's tracker and where the
// entry sits, as [beginEntry] returned them. It allocates m when it is nil.
//
// An entry with no key is refused in every decode: the member has no key to
// hold it under. The refusal is a primitive failure (see "Primitive failures"
// in the package documentation): a *typereg.DecodeError whose cause wraps
// [ErrInvalidShape], which names the entry by its path, from the root of the
// value in a tracked decode and by its name alone in an untracked one.
func putEntry[V any](t *Tracker, at place, start *xml.StartElement, m *map[string]V, key string, keyed bool, v V) error {
	if !keyed {
		if t != nil {
			return t.fail(t.closedPath(at), errEntryNoKey)
		}
		return primitiveFailure("/"+start.Name.Local, errEntryNoKey)
	}
	if *m == nil {
		*m = make(map[string]V)
	}
	(*m)[key] = v
	return nil
}

// errEntryNoKey is the refusal of a Hash entry with no key: no key attribute,
// or a value whose own key is empty.
var errEntryNoKey = errors.New("a Hash entry has no key")

// pointerKey escapes key as one reference token of a JSON Pointer (RFC 6901):
// `~` as `~0` and `/` as `~1`.
var pointerKey = strings.NewReplacer("~", "~0", "/", "~1")
