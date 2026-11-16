package simplified

// REQ-053 — `|value` spells the bare value of every datatype that has one, in
// FLAT, and in STRUCTURED, where `"|"` and `""` spell it too. The cases run
// over [allowedSuffixes], so a datatype that gains a bare value is covered
// without a case of its own.

import (
	"bytes"
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// otherValue is a value of v's JSON kind that differs from v.
func otherValue(t *testing.T, v any) any {
	t.Helper()
	switch v := v.(type) {
	case bool:
		return !v
	case int:
		return v + 1
	case float64:
		return v + 1
	}
	t.Fatalf("otherValue(%#v): no other value of this kind", v)
	return nil
}

// bareValueBody is the conformance body with the leaf of rmType under
// any_event:0 spelled by sfx, the bare value under "".
func bareValueBody(rmType string, sfx map[string]any) map[string]any {
	leaf := rmattrEvent + "/" + strings.ToLower(rmType)
	keys := make(map[string]any, len(sfx))
	for suffix, v := range sfx {
		key := leaf
		if suffix != "" {
			key += "|" + suffix
		}
		keys[key] = v
	}
	return rmattrBody(keys)
}

// bareValueDatatypes are the datatypes with a bare value, in sorted order.
func bareValueDatatypes() []string {
	var out []string
	for _, rmType := range slices.Sorted(maps.Keys(allowedSuffixes)) {
		if readsBareValue(rmType) {
			out = append(out, rmType)
		}
	}
	return out
}

// respellLeafMember rewrites doc, the STRUCTURED form of a [bareValueBody],
// so that the leaf's element holds its |value member under name instead.
func respellLeafMember(t *testing.T, doc []byte, rmType, name string) []byte {
	t.Helper()
	var root map[string]any
	if err := json.Unmarshal(doc, &root); err != nil {
		t.Fatal(err)
	}
	cur := root[rmattrRoot]
	for _, member := range []string{"conformance_section", "conformance_observation", "any_event", strings.ToLower(rmType)} {
		obj, _ := cur.(map[string]any)
		arr, _ := obj[member].([]any)
		if len(arr) == 0 {
			t.Fatalf("no %q element in %s", member, doc)
		}
		cur = arr[0]
	}
	el, ok := cur.(map[string]any)
	if !ok {
		t.Fatalf("the %s element in %s is no object", rmType, doc)
	}
	v, ok := el[bareValueMember]
	if !ok {
		t.Fatalf("the %s element %v holds no %q member", rmType, el, bareValueMember)
	}
	delete(el, bareValueMember)
	el[name] = v
	out, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestREQ053ValueSpellsTheBareValueOfEveryDatatypeThatHasOne — REQ-053:
// "Decode reads |value as the bare value, for every datatype that has one".
// At each such leaf a strict FLAT decode reads `x|value` as `x`, and a strict
// STRUCTURED decode reads the leaf's `|value`, `"|"` and `""` members as its
// bare value, each giving the composition the bare key gives. The datatypes
// with a `|value` of their own have no bare value.
func TestREQ053ValueSpellsTheBareValueOfEveryDatatypeThatHasOne(t *testing.T) {
	wt, _ := conformanceWT(t)
	datatypes := bareValueDatatypes()
	if len(datatypes) == 0 {
		t.Fatal("no datatype in allowedSuffixes has a bare value")
	}
	for _, rmType := range []string{"DV_CODED_TEXT", "DV_ORDINAL", "DV_SCALE"} {
		if readsBareValue(rmType) {
			t.Errorf("readsBareValue(%s) = true, want false: its |value is its own", rmType)
		}
	}
	for _, rmType := range datatypes {
		t.Run(rmType, func(t *testing.T) {
			full, ok := fullSuffixSets[rmType]
			if !ok {
				t.Fatalf("fullSuffixSets has no %s", rmType)
			}
			bare := maps.Clone(full)
			delete(bare, bareValueSuffix)
			spelled := maps.Clone(bare)
			delete(spelled, "")
			spelled[bareValueSuffix] = bare[""]

			want, err := UnmarshalFlat(mustJSON(t, bareValueBody(rmType, bare)), wt, WithStrict())
			if err != nil {
				t.Fatalf("UnmarshalFlat(bare key, strict) = %v", err)
			}
			got, err := UnmarshalFlat(mustJSON(t, bareValueBody(rmType, spelled)), wt, WithStrict())
			if err != nil {
				t.Fatalf("UnmarshalFlat(|value, strict) = %v", err)
			}
			if !bytes.Equal(canonical(t, got), canonical(t, want)) {
				t.Errorf("UnmarshalFlat(|value) =\n%s\nwant what the bare key decodes to\n%s", canonical(t, got), canonical(t, want))
			}

			doc, err := FlatToStructured(mustJSON(t, bareValueBody(rmType, spelled)))
			if err != nil {
				t.Fatalf("FlatToStructured = %v", err)
			}
			for _, name := range []string{bareValueMember, bareStructuredKey, emptyBareMember} {
				body := respellLeafMember(t, doc, rmType, name)
				got, err := UnmarshalStructured(body, wt, WithStrict())
				if err != nil {
					t.Fatalf("UnmarshalStructured(%q member, strict) = %v", name, err)
				}
				if !bytes.Equal(canonical(t, got), canonical(t, want)) {
					t.Errorf("UnmarshalStructured(%q member) =\n%s\nwant\n%s", name, canonical(t, got), canonical(t, want))
				}
			}
		})
	}
}

// TestREQ053ValueAndBareKeyThatDisagreeAreRefused — REQ-053: "Two spellings
// of one bare value that disagree are refused". At every datatype with a bare
// value, `x` beside an `x|value` of another value is refused in every mode
// with ErrUnknownPath naming both keys and neither value; beside an equal one
// it is read once. A lone `x|value` whose value does not fit the bare value's
// attribute is refused naming `x|value`, the key the body carries.
func TestREQ053ValueAndBareKeyThatDisagreeAreRefused(t *testing.T) {
	wt, _ := conformanceWT(t)
	for _, rmType := range bareValueDatatypes() {
		t.Run(rmType, func(t *testing.T) {
			bare := maps.Clone(fullSuffixSets[rmType])
			delete(bare, bareValueSuffix)
			leaf := rmattrEvent + "/" + strings.ToLower(rmType)

			agree := maps.Clone(bare)
			agree[bareValueSuffix] = bare[""]
			want, err := UnmarshalFlat(mustJSON(t, bareValueBody(rmType, bare)), wt, WithStrict())
			if err != nil {
				t.Fatalf("UnmarshalFlat(bare key) = %v", err)
			}
			got, err := UnmarshalFlat(mustJSON(t, bareValueBody(rmType, agree)), wt, WithStrict())
			if err != nil {
				t.Fatalf("UnmarshalFlat(bare key and an equal |value, strict) = %v", err)
			}
			if !bytes.Equal(canonical(t, got), canonical(t, want)) {
				t.Errorf("UnmarshalFlat(bare key and an equal |value) differs from the bare key alone")
			}

			// A string value is one no error text holds by chance, so the
			// refusal can be shown to carry neither.
			differ := maps.Clone(bare)
			if _, isString := bare[""].(string); isString {
				differ[""], differ[bareValueSuffix] = "SECRET-BARE-0454", "SECRET-VALUE-0454"
			} else {
				differ[bareValueSuffix] = otherValue(t, bare[""])
			}
			err = refusedInEveryMode(t, wt, bareValueBody(rmType, differ))
			if !errors.Is(err, ErrUnknownPath) {
				t.Errorf("decode = %v, want ErrUnknownPath", err)
			}
			for _, k := range []string{leaf, leaf + "|" + bareValueSuffix} {
				if !strings.Contains(err.Error(), strconv.Quote(k)) {
					t.Errorf("decode = %v, want it to name %q", err, k)
				}
			}
			if strings.Contains(err.Error(), "SECRET") {
				t.Errorf("decode = %v carries a value", err)
			}

			misfit := maps.Clone(bare)
			delete(misfit, "")
			misfit[bareValueSuffix] = map[string]any{"x": 1}
			err = refusedInEveryMode(t, wt, bareValueBody(rmType, misfit))
			if !errors.Is(err, ErrUnsupportedDatatype) || !strings.Contains(err.Error(), strconv.Quote(leaf+"|"+bareValueSuffix)) {
				t.Errorf("decode = %v, want ErrUnsupportedDatatype naming %q", err, leaf+"|"+bareValueSuffix)
			}

			// A datatype whose bare value is itself a shorthand, as a
			// DV_IDENTIFIER's is for its |id, reads |value as that shorthand
			// too, and a refusal of the pair names |value as written.
			sp, has := shorthandSpellings[rmType]
			if !has || sp.short != "" {
				return
			}
			chained := maps.Clone(bare)
			delete(chained, "")
			chained[bareValueSuffix], chained[sp.full] = "SECRET-VALUE-0454", "SECRET-FULL-0454"
			err = refusedInEveryMode(t, wt, bareValueBody(rmType, chained))
			for _, k := range []string{leaf + "|" + bareValueSuffix, leaf + "|" + sp.full} {
				if !errors.Is(err, ErrUnknownPath) || !strings.Contains(err.Error(), strconv.Quote(k)) {
					t.Errorf("decode = %v, want ErrUnknownPath naming %q", err, k)
				}
			}
		})
	}
}

// TestREQ053UnsetBareValueInEitherSpelling — REQ-053: an empty string at the
// bare value of a DV_DATE_TIME leaf is absent where nothing else of the leaf
// holds a value, since encode writes no key for it, whether the body spells it
// as the bare key, as `|value`, or as both.
func TestREQ053UnsetBareValueInEitherSpelling(t *testing.T) {
	wt, _ := conformanceWT(t)
	want, err := UnmarshalFlat(mustJSON(t, rmattrBody(nil)), wt, WithStrict())
	if err != nil {
		t.Fatalf("UnmarshalFlat(without the leaf) = %v", err)
	}
	for name, keys := range map[string]map[string]any{
		"bare key": {rmattrDateTime: ""},
		"|value":   {rmattrDateTime + "|value": ""},
		"both":     {rmattrDateTime: "", rmattrDateTime + "|value": ""},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := UnmarshalFlat(mustJSON(t, rmattrBody(keys)), wt, WithStrict())
			if err != nil {
				t.Fatalf("UnmarshalFlat(%v, strict) = %v", keys, err)
			}
			if !bytes.Equal(canonical(t, got), canonical(t, want)) {
				t.Errorf("UnmarshalFlat(%v) =\n%s\nwant the body without the leaf\n%s", keys, canonical(t, got), canonical(t, want))
			}
		})
	}
}

// TestREQ053StartTimeValueIsTheStartTime — REQ-053: the start time's real
// path is a DV_DATE_TIME leaf, so its `|value` is its bare value, another
// spelling of ctx/time: read as ctx/time is, and refused with ErrUnknownPath
// beside a ctx/time of another value.
func TestREQ053StartTimeValueIsTheStartTime(t *testing.T) {
	wt, _ := conformanceWT(t)
	const startTime = "2021-12-21T14:19:31.649613+01:00"
	key := rmattrContext + "/start_time|value"
	withCtx := rmattrBody(nil)
	if withCtx["ctx/time"] != startTime {
		t.Fatalf("fixture: rmattrBody ctx/time = %v, want %s", withCtx["ctx/time"], startTime)
	}
	want, err := UnmarshalFlat(mustJSON(t, withCtx), wt, WithStrict())
	if err != nil {
		t.Fatalf("UnmarshalFlat(ctx/time) = %v", err)
	}
	for name, body := range map[string]map[string]any{
		"alone":                 withoutKeys(rmattrBody(map[string]any{key: startTime}), "ctx/time"),
		"beside equal ctx/time": rmattrBody(map[string]any{key: startTime}),
	} {
		t.Run(name, func(t *testing.T) {
			got, err := UnmarshalFlat(mustJSON(t, body), wt, WithStrict())
			if err != nil {
				t.Fatalf("UnmarshalFlat(%s, strict) = %v", key, err)
			}
			if !bytes.Equal(canonical(t, got), canonical(t, want)) {
				t.Errorf("UnmarshalFlat(%s) =\n%s\nwant what ctx/time decodes to\n%s", key, canonical(t, got), canonical(t, want))
			}
		})
	}
	err = refusedInEveryMode(t, wt, rmattrBody(map[string]any{key: "2020-01-01T00:00:00Z"}))
	if !errors.Is(err, ErrUnknownPath) || !strings.Contains(err.Error(), strconv.Quote(key)) {
		t.Errorf("decode = %v, want ErrUnknownPath naming %q", err, key)
	}
}

// TestREQ140EndTimeValueIsTheEndTime — REQ-140, REQ-053: EVENT_CONTEXT's
// `_end_time` is a DV_DATE_TIME, a datatype with a bare value, so its `|value`
// spells that value again, in FLAT, with the `:0` OPT-free conversion writes,
// and as a STRUCTURED `|value` member. Each decodes under strict to what the
// bare key decodes to; beside an equal bare key it is read once, and beside
// one of another value the pair is refused in every mode with ErrUnknownPath
// naming both keys and neither value.
func TestREQ140EndTimeValueIsTheEndTime(t *testing.T) {
	wt, _ := conformanceWT(t)
	const endTime = "2021-12-21T15:19:31.649613+01:00"
	key := rmattrContext + "/_end_time"
	want, err := UnmarshalFlat(mustJSON(t, rmattrBody(map[string]any{key: endTime})), wt, WithStrict())
	if err != nil {
		t.Fatalf("UnmarshalFlat(bare key, strict) = %v", err)
	}
	if want.Context == nil || want.Context.EndTime == nil || want.Context.EndTime.Value != endTime {
		t.Fatalf("fixture: the bare key decodes to context %+v, want end_time %s", want.Context, endTime)
	}
	for name, keys := range map[string]map[string]any{
		"|value":               {key + "|value": endTime},
		":0|value":             {key + ":0|value": endTime},
		"bare and equal|value": {key: endTime, key + "|value": endTime},
	} {
		body := rmattrBody(keys)
		t.Run("FLAT/"+name, func(t *testing.T) {
			got, err := UnmarshalFlat(mustJSON(t, body), wt, WithStrict())
			if err != nil {
				t.Fatalf("UnmarshalFlat(%v, strict) = %v", keys, err)
			}
			if !bytes.Equal(canonical(t, got), canonical(t, want)) {
				t.Errorf("UnmarshalFlat(%v) =\n%s\nwant what the bare key decodes to\n%s", keys, canonical(t, got), canonical(t, want))
			}
		})
		t.Run("STRUCTURED/"+name, func(t *testing.T) {
			doc, err := FlatToStructured(mustJSON(t, body))
			if err != nil {
				t.Fatalf("FlatToStructured = %v", err)
			}
			got, err := UnmarshalStructured(doc, wt, WithStrict())
			if err != nil {
				t.Fatalf("UnmarshalStructured(%s, strict) = %v", doc, err)
			}
			if !bytes.Equal(canonical(t, got), canonical(t, want)) {
				t.Errorf("UnmarshalStructured(%s) =\n%s\nwant what the bare key decodes to\n%s", doc, canonical(t, got), canonical(t, want))
			}
		})
	}

	differ := rmattrBody(map[string]any{key: "2021-12-21T15:19:31+01:00", key + "|value": "2020-01-01T00:00:00Z"})
	err = refusedInEveryMode(t, wt, differ)
	if !errors.Is(err, ErrUnknownPath) {
		t.Errorf("decode of a bare key and |value that disagree = %v, want ErrUnknownPath", err)
	}
	for _, k := range []string{key, key + "|value"} {
		if !strings.Contains(err.Error(), strconv.Quote(k)) {
			t.Errorf("decode = %v, want it to name %q", err, k)
		}
	}
	for _, v := range []string{"2021-12-21", "2020-01-01"} {
		if strings.Contains(err.Error(), v) {
			t.Errorf("decode = %v carries a value", err)
		}
	}
}

// TestREQ140UnderscoreValueSpellsTheBareValueOfADataValueOnly — REQ-140,
// REQ-053: a `_`-attribute whose value is a data value with a bare value reads
// its `|value` as that value, as the temporal `_accuracy`, a DV_DURATION, does;
// `_uid` and `_location`, a UID and a String rather than data values, have no
// `|value`, which is unmatched content there.
func TestREQ140UnderscoreValueSpellsTheBareValueOfADataValueOnly(t *testing.T) {
	wt, _ := conformanceWT(t)
	accuracy := rmattrDateTime + "/_accuracy"
	want, err := UnmarshalFlat(mustJSON(t, rmattrBody(map[string]any{rmattrDateTime: "2022-01-12T13:22:34+01:00", accuracy: "PT1S"})), wt, WithStrict())
	if err != nil {
		t.Fatalf("UnmarshalFlat(bare _accuracy, strict) = %v", err)
	}
	got, err := UnmarshalFlat(mustJSON(t, rmattrBody(map[string]any{rmattrDateTime: "2022-01-12T13:22:34+01:00", accuracy + "|value": "PT1S"})), wt, WithStrict())
	if err != nil {
		t.Fatalf("UnmarshalFlat(_accuracy|value, strict) = %v", err)
	}
	if !bytes.Equal(canonical(t, got), canonical(t, want)) {
		t.Errorf("UnmarshalFlat(_accuracy|value) =\n%s\nwant what the bare _accuracy decodes to\n%s", canonical(t, got), canonical(t, want))
	}

	for _, key := range []string{rmattrObs + "/_uid|value", rmattrContext + "/_location|value"} {
		t.Run(key, func(t *testing.T) {
			assertUnmatched(t, wt, rmattrBody(map[string]any{key: "x"}), key)
		})
	}
}

// withoutKeys is body without keys.
func withoutKeys(body map[string]any, keys ...string) map[string]any {
	out := maps.Clone(body)
	for _, k := range keys {
		delete(out, k)
	}
	return out
}

// TestREQ053BareValueBesideAnotherDatatypesSuffixKeepsThePipe — REQ-053:
// FLAT → STRUCTURED keeps a bare value as the `"|"` member where its element
// holds a suffix no datatype with a bare value reads, so the conversion does
// not change what the body means. A bare value at a DV_CODED_TEXT leaf beside
// `|code` and `|terminology`, and one at a DV_QUANTITY leaf beside null
// suffixes, are refused in FLAT; their STRUCTURED form is refused alike,
// rather than read as the coded text's `|value` or dropped, and it converts
// back to the bare key.
func TestREQ053BareValueBesideAnotherDatatypesSuffixKeepsThePipe(t *testing.T) {
	wt, _ := conformanceWT(t)
	for name, tc := range map[string]struct {
		leaf string
		keys map[string]any
	}{
		"DV_CODED_TEXT beside |code": {
			leaf: rmattrCodedText,
			keys: map[string]any{rmattrCodedText: "Bare words", rmattrCodedText + "|code": "at0007", rmattrCodedText + "|terminology": "local"},
		},
		"DV_QUANTITY beside null suffixes": {
			leaf: rmattrElement,
			keys: map[string]any{rmattrElement: 5.0, rmattrElement + "|magnitude": nil, rmattrElement + "|unit": nil},
		},
	} {
		t.Run(name, func(t *testing.T) {
			body := mustJSON(t, rmattrBody(tc.keys))
			doc, err := FlatToStructured(body)
			if err != nil {
				t.Fatalf("FlatToStructured = %v", err)
			}
			for mode, opts := range map[string][]Option{"default": nil, "strict": {WithStrict()}} {
				_, flatErr := UnmarshalFlat(body, wt, opts...)
				_, structuredErr := UnmarshalStructured(doc, wt, opts...)
				if flatErr == nil || structuredErr == nil {
					t.Fatalf("%s decode: FLAT = %v, STRUCTURED = %v; want both refused", mode, flatErr, structuredErr)
				}
				for _, sentinel := range []error{ErrUnknownPath, ErrUnsupportedDatatype} {
					if errors.Is(flatErr, sentinel) != errors.Is(structuredErr, sentinel) {
						t.Errorf("%s decode: STRUCTURED refusal %v, want it to wrap what the FLAT refusal %v wraps", mode, structuredErr, flatErr)
					}
				}
			}
			back, err := StructuredToFlat(doc)
			if err != nil {
				t.Fatalf("StructuredToFlat = %v", err)
			}
			var flat map[string]any
			if err := json.Unmarshal(back, &flat); err != nil {
				t.Fatal(err)
			}
			key := indexedKey(tc.leaf)
			if _, kept := flat[key]; !kept {
				t.Errorf("FLAT → STRUCTURED → FLAT = %s, want the bare key %s back", back, key)
			}
			if _, spelled := flat[key+"|"+bareValueSuffix]; spelled {
				t.Errorf("FLAT → STRUCTURED → FLAT = %s, wrote the bare value as |value", back)
			}
		})
	}
}

// indexedKey is key with `:0` on every segment that carries no index, the
// spelling OPT-free STRUCTURED → FLAT conversion writes, the template root's
// segment aside.
func indexedKey(key string) string {
	segs := strings.Split(key, "/")
	for i := 1; i < len(segs); i++ {
		if !strings.Contains(segs[i], ":") {
			segs[i] += ":0"
		}
	}
	return strings.Join(segs, "/")
}

// TestREQ053BareValueMemberFollowsTheSuffixesBesideIt — REQ-053: FLAT →
// STRUCTURED moves a bare value to `|value` beside suffixes that some datatype
// with a bare value reads, and keeps `"|"` beside any other, each suffix
// judged off the suffix sets decode reads, so a datatype that gains a suffix
// moves with it. A suffix only datatypes with no bare value read, such as a
// coded text's `|code` or a quantity's `|magnitude`, keeps `"|"`.
func TestREQ053BareValueMemberFollowsTheSuffixesBesideIt(t *testing.T) {
	read := map[string]bool{}
	other := map[string]bool{}
	for rmType, set := range allowedSuffixes {
		for suffix := range set {
			if suffix == "" || suffix == bareValueSuffix {
				continue
			}
			if readsBareValue(rmType) {
				read[suffix] = true
			} else {
				other[suffix] = true
			}
		}
	}
	for _, suffix := range []string{"code", "magnitude", "unit", "terminology", "ordinal"} {
		if read[suffix] || !other[suffix] {
			t.Errorf("fixture: |%s is read by a datatype with a bare value, or by none without one", suffix)
		}
	}
	for _, suffix := range slices.Sorted(maps.Keys(other)) {
		if read[suffix] {
			continue
		}
		t.Run("|"+suffix, func(t *testing.T) {
			assertBareMember(t, suffix, bareStructuredKey)
		})
	}
	for _, suffix := range slices.Sorted(maps.Keys(read)) {
		t.Run("|"+suffix, func(t *testing.T) {
			assertBareMember(t, suffix, bareValueMember)
		})
	}
}

// assertBareMember converts a FLAT leaf holding a bare value beside suffix to
// STRUCTURED and asserts that the bare value is its element's member want.
func assertBareMember(t *testing.T, suffix, want string) {
	t.Helper()
	doc, err := FlatToStructured(mustJSON(t, map[string]any{"t/x": "v", "t/x|" + suffix: "s"}))
	if err != nil {
		t.Fatalf("FlatToStructured = %v", err)
	}
	var root map[string]map[string][]map[string]any
	if err := json.Unmarshal(doc, &root); err != nil {
		t.Fatalf("FlatToStructured = %s: %v", doc, err)
	}
	el := root["t"]["x"][0]
	if el[want] != "v" {
		t.Errorf("FlatToStructured beside |%s = %s, want the bare value as the %q member", suffix, doc, want)
	}
}

// TestREQ053FlatToStructuredKeepsTheBareKey — REQ-053: OPT-free FLAT →
// STRUCTURED keeps a bare value beside its leaf's members as the `"|"`
// member, whatever suffixes stand beside it, so the conversion never changes
// what a body means. A bare value at a leaf whose datatype has none is
// refused in FLAT, and its STRUCTURED form is refused alike in every mode,
// wrapping what the FLAT refusal wraps, rather than read as the datatype's
// own `|value` or set aside as unmatched while the value is lost: a
// DV_QUANTITY beside a null `|normal_status`, a suffix a DV_COUNT also reads;
// a DV_QUANTITY beside null `|magnitude` and `|unit`; and a DV_CODED_TEXT
// beside `|code` and `|terminology`. STRUCTURED → FLAT gives the bare key
// back, never `|value`.
func TestREQ053FlatToStructuredKeepsTheBareKey(t *testing.T) {
	wt, _ := conformanceWT(t)
	for name, tc := range map[string]struct {
		leaf string
		body map[string]any
	}{
		"DV_QUANTITY beside a null |normal_status": {
			leaf: rmattrElement,
			body: map[string]any{
				"ctx/language": "en", "ctx/territory": "US", "ctx/time": "2021-12-21T14:19:31.649613+01:00",
				rmattrElement: 5, rmattrElement + "|normal_status": nil,
			},
		},
		"DV_QUANTITY beside null |magnitude and |unit": {
			leaf: rmattrElement,
			body: rmattrBody(map[string]any{rmattrElement: 5.0, rmattrElement + "|magnitude": nil, rmattrElement + "|unit": nil}),
		},
		"DV_CODED_TEXT beside |code and |terminology": {
			leaf: rmattrCodedText,
			body: rmattrBody(map[string]any{rmattrCodedText: "Bare words", rmattrCodedText + "|code": "at0007", rmattrCodedText + "|terminology": "local"}),
		},
	} {
		t.Run(name, func(t *testing.T) {
			body := mustJSON(t, tc.body)
			doc, err := FlatToStructured(body)
			if err != nil {
				t.Fatalf("FlatToStructured = %v", err)
			}
			for mode, opts := range map[string][]Option{
				"default":       nil,
				"WithUnmatched": {WithUnmatched(func(string) {})},
				"WithStrict":    {WithStrict()},
			} {
				_, flatErr := UnmarshalFlat(body, wt, opts...)
				_, structuredErr := UnmarshalStructured(doc, wt, opts...)
				if flatErr == nil || structuredErr == nil {
					t.Errorf("%s decode: FLAT = %v, STRUCTURED = %v; want both refused", mode, flatErr, structuredErr)
					continue
				}
				for _, sentinel := range []error{ErrUnknownPath, ErrUnsupportedDatatype} {
					if errors.Is(flatErr, sentinel) != errors.Is(structuredErr, sentinel) {
						t.Errorf("%s decode: STRUCTURED refusal %v, want it to wrap what the FLAT refusal %v wraps", mode, structuredErr, flatErr)
					}
				}
			}
			back, err := StructuredToFlat(doc)
			if err != nil {
				t.Fatalf("StructuredToFlat = %v", err)
			}
			var flat map[string]any
			if err := json.Unmarshal(back, &flat); err != nil {
				t.Fatal(err)
			}
			key := indexedKey(tc.leaf)
			if _, kept := flat[key]; !kept {
				t.Errorf("FLAT → STRUCTURED → FLAT = %s, want the bare key %s back", back, key)
			}
			if _, spelled := flat[key+"|"+bareValueSuffix]; spelled {
				t.Errorf("FLAT → STRUCTURED → FLAT = %s, wrote the bare value as |value", back)
			}
		})
	}
}
