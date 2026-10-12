package simplified_test

// REQ-053, the ctx/ bullet: a real path whose segments under the template root
// carry the index `:0` (`<root>/language:0|code`, `<root>/composer:0|name`,
// `<root>/context:0/start_time:0`) is the same real path. Each of those
// attributes is single-valued, and OPT-free STRUCTURED → FLAT conversion writes
// every segment with an explicit index, so decode reads the indexed spelling
// exactly as the unindexed one, under the same disagreement and witness rules,
// and refuses any other index there.

import (
	"encoding/json"
	"errors"
	"maps"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

// corpusTarget builds the Web Template and the compiled template of the
// PROBE-086 corpus OPT, the template the reported bug was found on.
func corpusTarget(t *testing.T) (*webtemplate.WebTemplate, *templatecompile.Compiled) {
	t.Helper()
	parsed, err := template.ParseFile(corpusFlatOPT)
	if err != nil {
		t.Fatalf("ParseFile(%s): %v", corpusFlatOPT, err)
	}
	compiled, err := templatecompile.Compile(parsed)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	wt, err := webtemplate.Build(compiled)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return wt, compiled
}

// indexedMetaBase is the ctx/ context every body below starts from: the
// mandatory pair plus the time and the setting pair.
func indexedMetaBase() map[string]any {
	return map[string]any{
		"ctx/language": "en", "ctx/territory": "NL",
		"ctx/setting|code": "238", "ctx/setting|value": "other care",
		"ctx/time": "2026-08-05T09:00:00Z",
	}
}

// indexedMetaCase is one aliased composition-metadata field.
type indexedMetaCase struct {
	name string
	// ctxKey is the ctx/ short form the field normalises to, and ctxVal a value
	// for it that differs from val.
	ctxKey string
	ctxVal any
	// indexed is the root-relative real path with every segment at :0, and val
	// the value it carries.
	indexed string
	val     any
	// overIndexed are root-relative spellings with an index other than :0 on a
	// single-valued segment.
	overIndexed []string
	// structured is the root member that StructuredToFlat flattens to indexed.
	structured map[string]any
	// read reads the field back from a decoded composition.
	read func(*rm.Composition) any
}

func indexedMetaCases() []indexedMetaCase {
	return []indexedMetaCase{
		{
			name:   "language",
			ctxKey: "ctx/language", ctxVal: "en",
			indexed: "language:0|code", val: "de",
			overIndexed: []string{"language:1|code"},
			structured:  map[string]any{"language": []any{map[string]any{"|code": "de"}}},
			read:        func(c *rm.Composition) any { return c.Language.CodeString },
		},
		{
			name:   "territory",
			ctxKey: "ctx/territory", ctxVal: "NL",
			indexed: "territory:0|code", val: "BE",
			overIndexed: []string{"territory:1|code"},
			structured:  map[string]any{"territory": []any{map[string]any{"|code": "BE"}}},
			read:        func(c *rm.Composition) any { return c.Territory.CodeString },
		},
		{
			name:   "composer name",
			ctxKey: "ctx/composer_name", ctxVal: "Dr Ctx",
			indexed: "composer:0|name", val: "Dr Indexed",
			overIndexed: []string{"composer:1|name"},
			structured:  map[string]any{"composer": []any{map[string]any{"|name": "Dr Indexed"}}},
			read: func(c *rm.Composition) any {
				if p, ok := c.Composer.(*rm.PartyIdentified); ok && p.Name != nil {
					return *p.Name
				}
				return nil
			},
		},
		{
			// val is true and ctxVal false: a PARTY_SELF composer is the only value
			// the field can make observable, since false builds no composer at all.
			name:   "composer_self",
			ctxKey: "ctx/composer_self", ctxVal: false,
			indexed: "composer_self:0", val: true,
			overIndexed: []string{"composer_self:1"},
			structured:  map[string]any{"composer_self": []any{true}},
			read: func(c *rm.Composition) any {
				_, self := c.Composer.(*rm.PartySelf)
				return self
			},
		},
		{
			name:   "context start_time",
			ctxKey: "ctx/time", ctxVal: "2026-08-05T09:00:00Z",
			indexed: "context:0/start_time:0", val: "2026-08-06T10:30:00Z",
			overIndexed: []string{"context:1/start_time:0", "context:0/start_time:1", "context:1/start_time"},
			structured: map[string]any{"context": []any{map[string]any{
				"start_time": []any{"2026-08-06T10:30:00Z"},
			}}},
			read: func(c *rm.Composition) any {
				if c.Context == nil {
					return nil
				}
				return c.Context.StartTime.Value
			},
		},
		{
			name:   "context setting code",
			ctxKey: "ctx/setting|code", ctxVal: "238",
			indexed: "context:0/setting:0|code", val: "225",
			overIndexed: []string{"context:1/setting:0|code", "context:0/setting:1|code"},
			structured: map[string]any{"context": []any{map[string]any{
				"setting": []any{map[string]any{"|code": "225"}},
			}}},
			read: func(c *rm.Composition) any {
				if c.Context == nil {
					return nil
				}
				return c.Context.Setting.DefiningCode.CodeString
			},
		},
		{
			name:   "context setting value",
			ctxKey: "ctx/setting|value", ctxVal: "other care",
			indexed: "context:0/setting:0|value", val: "home",
			overIndexed: []string{"context:1/setting|value", "context/setting:1|value"},
			structured: map[string]any{"context": []any{map[string]any{
				"setting": []any{map[string]any{"|value": "home"}},
			}}},
			read: func(c *rm.Composition) any {
				if c.Context == nil {
					return nil
				}
				return c.Context.Setting.Value
			},
		},
	}
}

// marshalBody is json.Marshal for a test body, failing the test on error.
func marshalBody(t *testing.T, body map[string]any) []byte {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	return raw
}

// wantRefusal asserts err wraps want and names every string in named.
func wantRefusal(t *testing.T, err, want error, named ...string) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
	for _, n := range named {
		if !strings.Contains(err.Error(), n) {
			t.Errorf("error %q does not name %q", err, n)
		}
	}
}

// TestREQ053IndexedMetadataSpelling runs every aliased field through the rules
// the unindexed spelling obeys: a disagreeing ctx/ form or unindexed real path
// is a conflict, the indexed path alone or beside an agreeing ctx/ form decodes
// to its value, and an index other than :0 is refused naming the key.
func TestREQ053IndexedMetadataSpelling(t *testing.T) {
	wt, compiled := corpusTarget(t)
	root := wt.Tree.ID
	modes := []struct {
		name string
		opts []simplified.Option
	}{
		{"with template", []simplified.Option{simplified.WithTemplate(compiled)}},
		// Without the template no RM-mandatory default is filled in, so a
		// dropped composer_self cannot hide behind the PARTY_SELF default.
		{"without template", nil},
	}
	for _, tc := range indexedMetaCases() {
		indexedKey := root + "/" + tc.indexed
		unindexedKey := root + "/" + strings.ReplaceAll(tc.indexed, ":0", "")
		t.Run(tc.name, func(t *testing.T) {
			t.Run("disagreeing ctx form is a conflict", func(t *testing.T) {
				body := indexedMetaBase()
				body[tc.ctxKey] = tc.ctxVal
				body[indexedKey] = tc.val
				_, err := simplified.UnmarshalFlat(marshalBody(t, body), wt, simplified.WithTemplate(compiled))
				wantRefusal(t, err, simplified.ErrUnknownPath, "conflicting spellings of "+tc.ctxKey, indexedKey)
			})
			t.Run("disagreeing unindexed real path is a conflict", func(t *testing.T) {
				body := indexedMetaBase()
				delete(body, tc.ctxKey)
				body[unindexedKey] = tc.ctxVal
				body[indexedKey] = tc.val
				_, err := simplified.UnmarshalFlat(marshalBody(t, body), wt, simplified.WithTemplate(compiled))
				wantRefusal(t, err, simplified.ErrUnknownPath, "conflicting spellings of "+tc.ctxKey, unindexedKey, indexedKey)
			})
			for _, m := range modes {
				t.Run("alone decodes, "+m.name, func(t *testing.T) {
					body := indexedMetaBase()
					delete(body, tc.ctxKey)
					body[indexedKey] = tc.val
					got, err := simplified.UnmarshalFlat(marshalBody(t, body), wt, m.opts...)
					if err != nil {
						t.Fatalf("UnmarshalFlat(%s alone): %v", indexedKey, err)
					}
					if v := tc.read(got); v != tc.val {
						t.Errorf("UnmarshalFlat(%s = %#v) read back %#v", indexedKey, tc.val, v)
					}
				})
			}
			t.Run("agreeing ctx form decodes", func(t *testing.T) {
				body := indexedMetaBase()
				body[tc.ctxKey] = tc.val
				body[indexedKey] = tc.val
				got, err := simplified.UnmarshalFlat(marshalBody(t, body), wt, simplified.WithTemplate(compiled))
				if err != nil {
					t.Fatalf("UnmarshalFlat(%s beside an agreeing %s): %v", indexedKey, tc.ctxKey, err)
				}
				if v := tc.read(got); v != tc.val {
					t.Errorf("read back %#v, want %#v", v, tc.val)
				}
			})
			for _, rel := range tc.overIndexed {
				t.Run("index other than :0 refused: "+rel, func(t *testing.T) {
					body := indexedMetaBase()
					delete(body, tc.ctxKey)
					key := root + "/" + rel
					body[key] = tc.val
					_, err := simplified.UnmarshalFlat(marshalBody(t, body), wt, simplified.WithTemplate(compiled))
					wantRefusal(t, err, simplified.ErrUnknownPath, key)
				})
			}
		})
	}
}

// structuredBody builds a STRUCTURED body from the FLAT ctx/ entries in ctx,
// with member added under the template root.
func structuredBody(t *testing.T, root string, ctx map[string]any, member map[string]any) []byte {
	t.Helper()
	conv, err := simplified.FlatToStructured(marshalBody(t, ctx))
	if err != nil {
		t.Fatalf("FlatToStructured(ctx): %v", err)
	}
	var s map[string]any
	if err := json.Unmarshal(conv, &s); err != nil {
		t.Fatal(err)
	}
	s[root] = maps.Clone(member)
	return marshalBody(t, s)
}

// TestREQ053IndexedMetadataStructured covers the path that writes the indexed
// spelling: STRUCTURED carries every segment as an array, so StructuredToFlat
// spells a root-level metadata field with `:0` on each segment. Both the
// STRUCTURED → FLAT → decode round trip and UnmarshalStructured must keep the
// value, and a disagreeing ctx/ form must still be a conflict.
func TestREQ053IndexedMetadataStructured(t *testing.T) {
	wt, compiled := corpusTarget(t)
	root := wt.Tree.ID
	for _, tc := range indexedMetaCases() {
		indexedKey := root + "/" + tc.indexed
		t.Run(tc.name, func(t *testing.T) {
			ctx := indexedMetaBase()
			delete(ctx, tc.ctxKey)
			body := structuredBody(t, root, ctx, tc.structured)

			t.Run("StructuredToFlat then UnmarshalFlat keeps the value", func(t *testing.T) {
				flat, err := simplified.StructuredToFlat(body)
				if err != nil {
					t.Fatalf("StructuredToFlat: %v", err)
				}
				var m map[string]any
				if err := json.Unmarshal(flat, &m); err != nil {
					t.Fatal(err)
				}
				if _, ok := m[indexedKey]; !ok {
					t.Fatalf("StructuredToFlat wrote %v, want the indexed spelling %q", sortedKeys(m), indexedKey)
				}
				got, err := simplified.UnmarshalFlat(flat, wt, simplified.WithTemplate(compiled))
				if err != nil {
					t.Fatalf("UnmarshalFlat(StructuredToFlat output): %v", err)
				}
				if v := tc.read(got); v != tc.val {
					t.Errorf("read back %#v, want %#v", v, tc.val)
				}
			})
			t.Run("UnmarshalStructured keeps the value", func(t *testing.T) {
				got, err := simplified.UnmarshalStructured(body, wt, simplified.WithTemplate(compiled))
				if err != nil {
					t.Fatalf("UnmarshalStructured: %v", err)
				}
				if v := tc.read(got); v != tc.val {
					t.Errorf("read back %#v, want %#v", v, tc.val)
				}
			})
			t.Run("UnmarshalStructured refuses a disagreeing ctx form", func(t *testing.T) {
				ctx := indexedMetaBase()
				ctx[tc.ctxKey] = tc.ctxVal
				_, err := simplified.UnmarshalStructured(structuredBody(t, root, ctx, tc.structured), wt, simplified.WithTemplate(compiled))
				wantRefusal(t, err, simplified.ErrUnknownPath, "conflicting spellings of "+tc.ctxKey, indexedKey)
			})
		})
	}
}

// TestREQ053IndexedMetadataWitness: an indexed `|terminology` witness is checked
// against the terminology the ctx/ form implies and discarded, exactly as the
// unindexed witness is, and an index other than :0 on it is refused.
func TestREQ053IndexedMetadataWitness(t *testing.T) {
	wt, compiled := corpusTarget(t)
	root := wt.Tree.ID
	for _, tc := range []struct{ rel, implied, other string }{
		{"language:0|terminology", "ISO_639-1", "ISO_639-2"},
		{"territory:0|terminology", "ISO_3166-1", "ISO_3166-2"},
		{"context:0/setting:0|terminology", "openehr", "SNOMED-CT"},
	} {
		key := root + "/" + tc.rel
		t.Run(tc.rel, func(t *testing.T) {
			t.Run("matching witness accepted", func(t *testing.T) {
				body := indexedMetaBase()
				body[key] = tc.implied
				if _, err := simplified.UnmarshalFlat(marshalBody(t, body), wt, simplified.WithTemplate(compiled)); err != nil {
					t.Fatalf("UnmarshalFlat(%s = %q): %v", key, tc.implied, err)
				}
			})
			t.Run("differing witness refused", func(t *testing.T) {
				body := indexedMetaBase()
				body[key] = tc.other
				_, err := simplified.UnmarshalFlat(marshalBody(t, body), wt, simplified.WithTemplate(compiled))
				wantRefusal(t, err, simplified.ErrUnsupportedDatatype, key)
			})
			t.Run("index other than :0 refused", func(t *testing.T) {
				body := indexedMetaBase()
				over := root + "/" + strings.Replace(tc.rel, ":0", ":1", 1)
				body[over] = tc.implied
				_, err := simplified.UnmarshalFlat(marshalBody(t, body), wt, simplified.WithTemplate(compiled))
				wantRefusal(t, err, simplified.ErrUnknownPath, over)
			})
		})
	}
}

// TestREQ053IndexedMetadataRefusalsHold: the refusals the unindexed spelling
// meets apply to the indexed one as well. A suffix the ctx/ form cannot carry on
// a leaf it owns is refused, the composer's party sub-structure is the ADR 0015
// boundary, and the composer external_ref stays refused.
func TestREQ053IndexedMetadataRefusalsHold(t *testing.T) {
	wt, compiled := corpusTarget(t)
	root := wt.Tree.ID
	for _, tc := range []struct {
		name  string
		rel   string
		val   any
		want  error
		named []string
	}{
		{"setting formatting", "context:0/setting:0|formatting", "plain", simplified.ErrUnsupportedDatatype, nil},
		{"setting bare", "context:0/setting:0", "other care", simplified.ErrUnsupportedDatatype, nil},
		{"start_time raw", "context:0/start_time:0|raw", map[string]any{"_type": "DV_DATE_TIME", "value": "2026-08-05T10:00:00Z"}, simplified.ErrUnsupportedDatatype, nil},
		{"composer nested identifier", "composer:0/_identifier:0|id", "122", simplified.ErrUnsupportedDatatype, []string{"ADR 0015"}},
		{"composer relationship", "composer:0/relationship|code", "122", simplified.ErrUnsupportedDatatype, []string{"ADR 0015"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := indexedMetaBase()
			key := root + "/" + tc.rel
			body[key] = tc.val
			_, err := simplified.UnmarshalFlat(marshalBody(t, body), wt, simplified.WithTemplate(compiled))
			wantRefusal(t, err, tc.want, append([]string{key}, tc.named...)...)
		})
	}
	t.Run("composer external_ref", func(t *testing.T) {
		body := indexedMetaBase()
		body[root+"/composer:0|id"] = "12345"
		if _, err := simplified.UnmarshalFlat(marshalBody(t, body), wt, simplified.WithTemplate(compiled)); err == nil {
			t.Error("indexed composer external_ref decoded without error; want a refusal (ADR 0015)")
		}
	})
}

// TestREQ053PartlyIndexedMetadataSpelling: each segment is single-valued on its
// own, so a path indexed on some segments and not others is still the same real
// path.
func TestREQ053PartlyIndexedMetadataSpelling(t *testing.T) {
	wt, compiled := corpusTarget(t)
	root := wt.Tree.ID
	for _, rel := range []string{"context:0/start_time", "context/start_time:0"} {
		t.Run(rel, func(t *testing.T) {
			body := indexedMetaBase()
			delete(body, "ctx/time")
			body[root+"/"+rel] = "2026-08-06T10:30:00Z"
			got, err := simplified.UnmarshalFlat(marshalBody(t, body), wt, simplified.WithTemplate(compiled))
			if err != nil {
				t.Fatalf("UnmarshalFlat(%s): %v", rel, err)
			}
			if got.Context == nil || got.Context.StartTime.Value != "2026-08-06T10:30:00Z" {
				t.Errorf("start_time = %#v, want 2026-08-06T10:30:00Z", got.Context)
			}
		})
	}
}
