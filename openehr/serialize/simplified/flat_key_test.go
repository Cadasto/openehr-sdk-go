package simplified

import (
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"
)

// TestFlatDecodeRefusesFirstMalformedKey pins which key a body with several
// malformed indexes is refused for: the first in sorted order, on every run,
// whichever pass before the leaf loop meets it (an underscore family, a party
// leaf, an in-context spelling, an ordinary leaf). The refusal is the key's own
// parse error, unwrapped.
func TestFlatDecodeRefusesFirstMalformedKey(t *testing.T) {
	wt, _ := conformanceWT(t)
	malformed := []string{
		rmattrElement + "/_uid:-1",
		rmattrObs + "/subject:01|name",
		rmattrEvent + "/width:00",
		rmattrObs + "/any_event:00/dv_text",
		rmattrSection + ":+1/conformance_observation/any_event:0/dv_text",
	}
	extra := map[string]any{
		// A null in-context key, which the first pass removes from content.
		rmattrEvent + "/math_function": nil,
	}
	for _, k := range malformed {
		extra[k] = "x"
	}
	first := slices.Min(malformed)
	_, want := parseFlatKey(first)
	if want == nil {
		t.Fatalf("parseFlatKey(%q) accepted the key; the case needs a malformed index", first)
	}
	body := mustJSON(t, rmattrBody(extra))
	for range 20 {
		_, err := UnmarshalFlat(body, wt)
		if err == nil || err.Error() != want.Error() || !errors.Is(err, ErrUnknownPath) {
			t.Fatalf("UnmarshalFlat = %v, want %v (the parse error of the first malformed key, %q)", err, want, first)
		}
	}
}

// TestContentPassesSkipKeysAnEarlierPassTook pins that compositeLeafGroups and
// rmattrGroups read only the keys still in content: a key in the shared parse
// that an earlier pass took out is neither grouped nor refused.
func TestContentPassesSkipKeysAnEarlierPassTook(t *testing.T) {
	wt, _ := conformanceWT(t)
	subject := rmattrObs + "/subject"
	kept := map[string]any{
		subject + "|id":                    "1",
		rmattrElement + "/_link:0|meaning": "m",
	}
	taken := map[string]any{
		subject + "|name":                   "n",
		rmattrElement + "/_uid":             "u",
		rmattrObs + "/any_event:00/dv_text": "malformed",
	}
	all := maps.Clone(kept)
	maps.Copy(all, taken)
	parsed := parseContentKeys(all)
	content := maps.Clone(kept)

	composite, err := compositeLeafGroups(content, parsed, wt, nil)
	if err != nil {
		t.Fatalf("compositeLeafGroups = %v, want no refusal for a malformed key no longer in content", err)
	}
	if len(composite) != 1 || composite[0].prefix() != subject || !slices.Equal(slices.Sorted(maps.Keys(composite[0].tails)), []string{"|id"}) {
		t.Fatalf("compositeLeafGroups = %+v, want one %q group holding only |id", composite, subject)
	}
	attrs, err := rmattrGroups(content, parsed, nil)
	if err != nil {
		t.Fatalf("rmattrGroups = %v, want no refusal for a malformed key no longer in content", err)
	}
	if len(attrs) != 1 || attrs[0].prefix() != rmattrElement+"/_link:0" {
		var got []string
		for _, g := range attrs {
			got = append(got, g.prefix())
		}
		t.Fatalf("rmattrGroups = %q, want only %q", strings.Join(got, ", "), rmattrElement+"/_link:0")
	}
	if len(content) != 0 {
		t.Errorf("content left = %v, want both kept keys consumed", slices.Sorted(maps.Keys(content)))
	}
}
