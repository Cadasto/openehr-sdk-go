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
		{rel: "context:0/other_context/x:01|y", norm: "context:0/other_context/x:01|y"},
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

// splitKeyCase is one key split around a segment: the owner's path, the
// segment's id and index, and the tail after it.
type splitKeyCase struct {
	key    string
	base   string
	family string
	index  int
	tail   string
	ok     bool
}

// TestSplitRMAttrKeyEdgeKeys pins the base and tail an underscore key splits
// into, byte for byte: indexes and non-numeric `:` text kept as written, a
// trailing bare `|` dropped, and every `|` before the last one kept in its
// segment.
func TestSplitRMAttrKeyEdgeKeys(t *testing.T) {
	tests := []splitKeyCase{
		{key: "a/b:0/_uid", base: "a/b:0", family: "_uid", index: -1, ok: true},
		{key: "a/b:0/_link:1|meaning", base: "a/b:0", family: "_link", index: 1, tail: "|meaning", ok: true},
		{key: "a/_x:0", base: "a", family: "_x", index: 0, ok: true},
		{key: "a/_feeder_audit/originating_system_audit|system_id", base: "a", family: "_feeder_audit", index: -1, tail: "/originating_system_audit|system_id", ok: true},
		{key: "a/b:c/_x:0/d:e:2", base: "a/b:c", family: "_x", index: 0, tail: "/d:e:2", ok: true},
		{key: "a/_x/c:2/d|", base: "a", family: "_x", index: -1, tail: "/c:2/d", ok: true},
		{key: "a/_x|", base: "a", family: "_x", index: -1, ok: true},
		{key: "a/_x/c|x|y", base: "a", family: "_x", index: -1, tail: "/c|x|y", ok: true},
		{key: "a|p/_x:3/c|y", base: "a|p", family: "_x", index: 3, tail: "/c|y", ok: true},
		{key: "a/_x|y/z", base: "a", family: "_x", index: -1, tail: "|y/z", ok: true},
		{key: "a//_x", base: "a/", family: "_x", index: -1, ok: true},
		{key: "a/_x/_y:0|z", base: "a", family: "_x", index: -1, tail: "/_y:0|z", ok: true},
		{key: "a:0|q/_x"},
		{key: "_x/a"},
		{key: "a/b|_x"},
	}
	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			pk, err := parseFlatKey(tc.key)
			if err != nil {
				t.Fatalf("parseFlatKey(%q): %v", tc.key, err)
			}
			var got splitKeyCase
			got.key = tc.key
			got.base, got.family, got.index, got.tail, got.ok = splitRMAttrKey(tc.key, pk)
			if got != tc {
				t.Errorf("splitRMAttrKey(%q) = %+v, want %+v", tc.key, got, tc)
			}
		})
	}
}

// TestSplitCompositeLeafKeyEdgeKeys pins the base and tail a key at a party
// leaf splits into, byte for byte, by the same rules as an underscore key.
func TestSplitCompositeLeafKeyEdgeKeys(t *testing.T) {
	wt, _ := conformanceWT(t)
	const indexedObs = rmattrRoot + "/conformance_section:0/conformance_observation:0"
	subject := rmattrObs + "/subject"
	tests := []splitKeyCase{
		{key: subject, base: rmattrObs, family: "subject", index: -1, ok: true},
		{key: subject + "|name", base: rmattrObs, family: "subject", index: -1, tail: "|name", ok: true},
		{key: subject + "|", base: rmattrObs, family: "subject", index: -1, ok: true},
		{key: indexedObs + "/subject:0/_identifier:0|id", base: indexedObs, family: "subject", index: 0, tail: "/_identifier:0|id", ok: true},
		{key: subject + "/relationship|code|", base: rmattrObs, family: "subject", index: -1, tail: "/relationship|code", ok: true},
		{key: subject + "/relationship|a|b", base: rmattrObs, family: "subject", index: -1, tail: "/relationship|a|b", ok: true},
		{key: subject + "|a/b", base: rmattrObs, family: "subject", index: -1, tail: "|a/b", ok: true},
		{key: subject + "/_uid"},
		{key: rmattrObs + "/_uid"},
		{key: rmattrElement + "|magnitude"},
	}
	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			pk, err := parseFlatKey(tc.key)
			if err != nil {
				t.Fatalf("parseFlatKey(%q): %v", tc.key, err)
			}
			var got splitKeyCase
			got.key = tc.key
			got.base, got.family, got.index, got.tail, got.ok = splitCompositeLeafKey(wt, tc.key, pk)
			if got != tc {
				t.Errorf("splitCompositeLeafKey(%q) = %+v, want %+v", tc.key, got, tc)
			}
		})
	}
}

// respellAround re-spells pk around segment i from its parsed segments with
// writeFlatSeg: the bytes cutAtSeg must slice out of the key.
func respellAround(pk parsedKey, i int) (base, tail string) {
	var b, t strings.Builder
	for j, s := range pk.segs[:i] {
		if j > 0 {
			b.WriteByte('/')
		}
		writeFlatSeg(&b, s)
	}
	for _, s := range pk.segs[i+1:] {
		t.WriteByte('/')
		writeFlatSeg(&t, s)
	}
	if pk.suffix != "" {
		t.WriteByte('|')
		t.WriteString(pk.suffix)
	}
	return b.String(), t.String()
}

// FuzzCutAtSegMatchesRespelling holds cutAtSeg to the re-spelling of the
// parsed key around every segment, for any key parseFlatKey accepts. The seeds
// are the keys where a slice and a re-spelling could part: a trailing bare `|`,
// two pipes, a `|` or `/` in the suffix, empty segments, and `:` text that is
// no index.
func FuzzCutAtSegMatchesRespelling(f *testing.F) {
	for _, key := range []string{
		"a", "a/b", "a:0/b:12/c", "a/b|", "a/b|x", "a/b|x|y", "a|p/b:3/c|y",
		"a/b|y/z", "a//b", "/a/", "a:x/b:0:1/c:", "a/b:0|", "|", "", "a/_x/_y:0|z",
	} {
		f.Add(key)
	}
	f.Fuzz(func(t *testing.T, key string) {
		pk, err := parseFlatKey(key)
		if err != nil {
			return
		}
		for i := range pk.segs {
			base, tail := cutAtSeg(key, i)
			wantBase, wantTail := respellAround(pk, i)
			if base != wantBase || tail != wantTail {
				t.Errorf("cutAtSeg(%q, %d) = %q, %q, want %q, %q", key, i, base, tail, wantBase, wantTail)
			}
		}
	})
}
