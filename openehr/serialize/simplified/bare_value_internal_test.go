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

// withoutKeys is body without keys.
func withoutKeys(body map[string]any, keys ...string) map[string]any {
	out := maps.Clone(body)
	for _, k := range keys {
		delete(out, k)
	}
	return out
}
