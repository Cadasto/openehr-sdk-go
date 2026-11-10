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

// TestCutRootEdgeKeys pins how a key splits at the template root's segment:
// `|` is cut off the first segment before it is parsed, so `root|/x` is rooted
// and `root|a/x` is not, and a malformed index on the root keeps the key out.
func TestCutRootEdgeKeys(t *testing.T) {
	tests := []struct {
		key    string
		rel    string
		seg    flatSeg
		rooted bool
	}{
		{key: "root/x", rel: "x", seg: flatSeg{"root", -1}, rooted: true},
		{key: "root:0/x", rel: "x", seg: flatSeg{"root", 0}, rooted: true},
		{key: "root:1/x/y", rel: "x/y", seg: flatSeg{"root", 1}, rooted: true},
		{key: "root/", rel: "", seg: flatSeg{"root", -1}, rooted: true},
		{key: "root|/x", rel: "x", seg: flatSeg{"root", -1}, rooted: true},
		{key: "root:0|/x|code", rel: "x|code", seg: flatSeg{"root", 0}, rooted: true},
		{key: "root|a/x"},
		{key: "root|a|/x"},
		{key: "root|x:0/y"},
		{key: "root:00/x"},
		{key: "root:-1/x"},
		{key: "root:x/y"},
		{key: "root"},
		{key: "other/x"},
	}
	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			rel, seg, rooted := cutRoot(tc.key, "root")
			if rel != tc.rel || seg != tc.seg || rooted != tc.rooted {
				t.Errorf("cutRoot(%q) = %q, %+v, %t, want %q, %+v, %t", tc.key, rel, seg, rooted, tc.rel, tc.seg, tc.rooted)
			}
		})
	}
}

// TestRootSegmentAloneEdgeKeys pins how a key with no `/` is read as the
// template root's segment: the last `|` and what follows it are the suffix,
// and a malformed index keeps the key out.
func TestRootSegmentAloneEdgeKeys(t *testing.T) {
	tests := []struct {
		key   string
		seg   flatSeg
		alone bool
	}{
		{key: "root", seg: flatSeg{"root", -1}, alone: true},
		{key: "root:2", seg: flatSeg{"root", 2}, alone: true},
		{key: "root:2|code", seg: flatSeg{"root", 2}, alone: true},
		{key: "root|", seg: flatSeg{"root", -1}, alone: true},
		{key: "root:1|a:0", seg: flatSeg{"root", 1}, alone: true},
		{key: "root|a|b"},
		{key: "root:02"},
		{key: "root:02|code"},
		{key: "root/x"},
		{key: "rootx"},
		{key: "root:x"},
	}
	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			seg, alone := rootSegmentAlone(tc.key, "root")
			if seg != tc.seg || alone != tc.alone {
				t.Errorf("rootSegmentAlone(%q) = %+v, %t, want %+v, %t", tc.key, seg, alone, tc.seg, tc.alone)
			}
		})
	}
}

// TestMetadataRelEdgeKeys pins the unindexed spelling metadataRel gives a
// root-relative key: only the leading run of metadata segments loses its
// index, the suffix after the first `|` is kept as written, and a key with a
// malformed index anywhere comes back unchanged and not over-indexed.
func TestMetadataRelEdgeKeys(t *testing.T) {
	tests := []struct {
		rel         string
		norm        string
		overIndexed bool
	}{
		{rel: "language|code", norm: "language|code"},
		{rel: "language:0|code", norm: "language|code"},
		{rel: "language:1|code", norm: "language|code", overIndexed: true},
		{rel: "language:0|", norm: "language|"},
		{rel: "language:0|a|b", norm: "language|a|b"},
		{rel: "language:0:0|code", norm: "language:0:0|code"},
		{rel: "context:0", norm: "context"},
		{rel: "context:0/start_time:0", norm: "context/start_time"},
		{rel: "context:0/start_time:2|x", norm: "context/start_time|x", overIndexed: true},
		{rel: "context:1/setting:0|code", norm: "context/setting|code", overIndexed: true},
		{rel: "context:0/start_time:0/x|a:0/b", norm: "context/start_time/x|a:0/b"},
		{rel: "context:0/other_context/x:1|y", norm: "context/other_context/x:1|y"},
		{rel: "context:0/x:y", norm: "context/x:y"},
		{rel: "composer:0/_identifier:0|id", norm: "composer/_identifier:0|id"},
		{rel: "start_time:0", norm: "start_time:0"},
		{rel: "conformance_section:0/x:0|y", norm: "conformance_section:0/x:0|y"},
		{rel: "context:0/x:01", norm: "context:0/x:01"},
		{rel: "context:1/x:01", norm: "context:1/x:01"},
		{rel: "context/x:01", norm: "context/x:01"},
		{rel: "context:00/start_time", norm: "context:00/start_time"},
		{rel: "", norm: ""},
	}
	for _, tc := range tests {
		t.Run(tc.rel, func(t *testing.T) {
			norm, over := metadataRel(tc.rel)
			if norm != tc.norm || over != tc.overIndexed {
				t.Errorf("metadataRel(%q) = %q, %t, want %q, %t", tc.rel, norm, over, tc.norm, tc.overIndexed)
			}
		})
	}
}
