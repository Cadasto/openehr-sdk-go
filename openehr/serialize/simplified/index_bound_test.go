package simplified_test

// REQ-053 and REQ-170: an explicit :index on a FLAT segment that holds one
// value. The template root's own segment is the one COMPOSITION, and a
// single-valued RM attribute (COMPOSITION.context, ACTION.ism_transition,
// ENTRY.language, …) holds one value. `:0` is that value's explicit-index
// spelling and reads as no index at all. Any other index is past the bound,
// which REQ-170 lists as malformed matched content, so decode refuses it naming
// the key rather than merging the key into the one value.

import (
	"cmp"
	"errors"
	"maps"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
)

// TestREQ053IndexedRootSegmentMetadata: a metadata real path whose template
// root segment carries `:0` (`<root>:0/language|code`,
// `<root>:0/composer:0|name`) is the same real path, read under the same
// disagreement and witness rules as the unindexed spelling.
func TestREQ053IndexedRootSegmentMetadata(t *testing.T) {
	wt, compiled := corpusTarget(t)
	root0 := wt.Tree.ID + ":0/"
	modes := []struct {
		name string
		opts []simplified.Option
	}{
		{"with template", []simplified.Option{simplified.WithTemplate(compiled)}},
		{"without template", nil},
	}
	for _, tc := range indexedMetaCases() {
		for _, rel := range []string{tc.indexed, strings.ReplaceAll(tc.indexed, ":0", "")} {
			key := root0 + rel
			t.Run(key, func(t *testing.T) {
				t.Run("disagreeing ctx form is a conflict", func(t *testing.T) {
					body := indexedMetaBase()
					body[tc.ctxKey] = tc.ctxVal
					body[key] = tc.val
					_, err := simplified.UnmarshalFlat(marshalBody(t, body), wt, simplified.WithTemplate(compiled))
					wantRefusal(t, err, simplified.ErrUnknownPath, "conflicting spellings of "+tc.ctxKey, key)
				})
				for _, m := range modes {
					t.Run("alone decodes, "+m.name, func(t *testing.T) {
						body := indexedMetaBase()
						delete(body, tc.ctxKey)
						body[key] = tc.val
						got, err := simplified.UnmarshalFlat(marshalBody(t, body), wt, m.opts...)
						if err != nil {
							t.Fatalf("UnmarshalFlat(%s alone): %v", key, err)
						}
						if v := tc.read(got); v != tc.val {
							t.Errorf("UnmarshalFlat(%s = %#v) read back %#v", key, tc.val, v)
						}
					})
				}
				t.Run("agreeing ctx form decodes", func(t *testing.T) {
					body := indexedMetaBase()
					body[tc.ctxKey] = tc.val
					body[key] = tc.val
					got, err := simplified.UnmarshalFlat(marshalBody(t, body), wt, simplified.WithTemplate(compiled))
					if err != nil {
						t.Fatalf("UnmarshalFlat(%s beside an agreeing %s): %v", key, tc.ctxKey, err)
					}
					if v := tc.read(got); v != tc.val {
						t.Errorf("read back %#v, want %#v", v, tc.val)
					}
				})
			})
		}
	}
}

// TestREQ053IndexedRootSegmentRules: the witness check and the refusals the
// unindexed spelling meets hold under `<root>:0/` too, a content key under
// `<root>:0/` decodes as under `<root>/`, and `<root>:1/` is refused whatever
// follows it.
func TestREQ053IndexedRootSegmentRules(t *testing.T) {
	wt, compiled := corpusTarget(t)
	root := wt.Tree.ID
	for _, tc := range []struct {
		name  string
		key   string
		val   any
		want  error
		named []string
	}{
		{"differing witness", root + ":0/language|terminology", "ISO_639-2", simplified.ErrUnsupportedDatatype, nil},
		{"indexed differing witness", root + ":0/context:0/setting:0|terminology", "SNOMED-CT", simplified.ErrUnsupportedDatatype, nil},
		{"setting formatting", root + ":0/context/setting|formatting", "plain", simplified.ErrUnsupportedDatatype, nil},
		// The composer's real path carries its identifiers (ADR 0029); a value
		// that does not fit DV_IDENTIFIER.id is refused under `<root>:0/` too.
		{"composer nested identifier that does not fit", root + ":0/composer/_identifier:0|id", 122, simplified.ErrUnsupportedDatatype, []string{"DV_IDENTIFIER.id"}},
		// The metadata values agree with the base ctx/ context, so only the index
		// can be what refuses them.
		{"metadata under root :1", root + ":1/language|code", "en", simplified.ErrUnknownPath, []string{"not an indexed list"}},
		{"indexed metadata under root :1", root + ":1/context:0/start_time:0", "2026-08-05T09:00:00Z", simplified.ErrUnknownPath, []string{"not an indexed list"}},
		{"underscore family under root :1", root + ":1/_uid", "8f8f8f8f-0000-4000-8000-000000000000", simplified.ErrUnknownPath, []string{"not an indexed list"}},
		{"content under root :1", root + ":1/context/_health_care_facility|name", "Hospital", simplified.ErrUnknownPath, []string{"not an indexed list"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := indexedMetaBase()
			body[tc.key] = tc.val
			_, err := simplified.UnmarshalFlat(marshalBody(t, body), wt, simplified.WithTemplate(compiled))
			wantRefusal(t, err, tc.want, append([]string{tc.key}, tc.named...)...)
		})
	}
	t.Run("matching witness accepted", func(t *testing.T) {
		body := indexedMetaBase()
		body[root+":0/language|terminology"] = "ISO_639-1"
		if _, err := simplified.UnmarshalFlat(marshalBody(t, body), wt, simplified.WithTemplate(compiled)); err != nil {
			t.Fatalf("UnmarshalFlat: %v", err)
		}
	})
	t.Run("content under root :0 decodes", func(t *testing.T) {
		body := indexedMetaBase()
		body[root+":0/context/_health_care_facility|name"] = "Hospital"
		got, err := simplified.UnmarshalFlat(marshalBody(t, body), wt, simplified.WithTemplate(compiled))
		if err != nil {
			t.Fatalf("UnmarshalFlat: %v", err)
		}
		if got.Context == nil || got.Context.HealthCareFacility == nil {
			t.Fatalf("health_care_facility not decoded: %#v", got.Context)
		}
	})
}

// singleValuedCase is a key family that reaches one single-valued position:
// keys builds it with the segment under test carrying idx, and read reports the
// value it decoded to.
type singleValuedCase struct {
	name string
	// seg is the FLAT id of the single-valued segment under test.
	seg string
	// key is the offending key a refusal must name, for idx.
	key  func(root string, idx string) string
	keys func(root string, idx string) map[string]any
	read func(*rm.Composition) any
	want any
}

func singleValuedCases() []singleValuedCase {
	const action = "/conformance_section/conformance_action"
	const obs = "/conformance_section/conformance_observation"
	return []singleValuedCase{
		{
			// COMPOSITION.context through the underscore router (REQ-140).
			name: "context, underscore family",
			seg:  "context",
			key:  func(root, idx string) string { return root + "/context" + idx + "/_health_care_facility" },
			keys: func(root, idx string) map[string]any {
				return map[string]any{root + "/context" + idx + "/_health_care_facility|name": "Hospital"}
			},
			read: func(c *rm.Composition) any {
				if c.Context == nil {
					return nil
				}
				if p, ok := c.Context.HealthCareFacility.(*rm.PartyIdentified); ok && p.Name != nil {
					return *p.Name
				}
				return nil
			},
			want: "Hospital",
		},
		{
			// COMPOSITION.context through the underscore router, a second family.
			name: "context, end_time",
			seg:  "context",
			key:  func(root, idx string) string { return root + "/context" + idx + "/_end_time" },
			keys: func(root, idx string) map[string]any {
				return map[string]any{root + "/context" + idx + "/_end_time": "2026-08-05T11:00:00Z"}
			},
			read: func(c *rm.Composition) any {
				if c.Context == nil || c.Context.EndTime == nil {
					return nil
				}
				return c.Context.EndTime.Value
			},
			want: "2026-08-05T11:00:00Z",
		},
		{
			// ACTION.ism_transition is single-valued; the Web Template models it as
			// an archetyped node (ism_transition[at0005], FLAT id `transition`).
			name: "action ism_transition",
			seg:  "transition",
			key:  func(root, idx string) string { return root + action + "/transition" + idx + "/current_state" },
			keys: func(root, idx string) map[string]any {
				base := root + action + "/transition" + idx
				return map[string]any{
					base + "/current_state|code":        "524",
					base + "/current_state|value":       "initial",
					base + "/current_state|terminology": "openehr",
				}
			},
			read: func(c *rm.Composition) any {
				for _, item := range sectionItems(c) {
					if a, ok := item.(*rm.Action); ok {
						return a.IsmTransition.CurrentState.DefiningCode.CodeString
					}
				}
				return nil
			},
			want: "524",
		},
		{
			// ENTRY.language: an in-context leaf the Web Template does not
			// archetype.
			name: "entry language",
			seg:  "language",
			key:  func(root, idx string) string { return root + obs + "/language" + idx },
			keys: func(root, idx string) map[string]any {
				return map[string]any{
					root + obs + "/language" + idx + "|code":        "nl",
					root + obs + "/language" + idx + "|terminology": "ISO_639-1",
					root + obs + "/any_event:0/time":                "2026-08-05T09:30:00Z",
					root + obs + "/any_event:0/dv_count":            7,
				}
			},
			read: func(c *rm.Composition) any {
				for _, item := range sectionItems(c) {
					if o, ok := item.(*rm.Observation); ok {
						return o.Language.CodeString
					}
				}
				return nil
			},
			want: "nl",
		},
	}
}

// sectionItems returns the items of the composition's first SECTION.
func sectionItems(c *rm.Composition) []rm.ContentItem {
	for _, ci := range c.Content {
		if s, ok := ci.(*rm.Section); ok {
			return s.Items
		}
	}
	return nil
}

// TestREQ170SingleValuedIndexPastBound: a key whose segment on a single-valued
// position carries an index other than :0 is refused naming the key, in FLAT
// and through STRUCTURED, and the `:0` spelling of the same keys decodes.
func TestREQ170SingleValuedIndexPastBound(t *testing.T) {
	wt, compiled := corpusTarget(t)
	root := wt.Tree.ID
	for _, tc := range singleValuedCases() {
		t.Run(tc.name, func(t *testing.T) {
			for _, idx := range []string{":0", ""} {
				t.Run("index "+cmp.Or(idx, "none")+" decodes", func(t *testing.T) {
					body := indexedMetaBase()
					maps.Copy(body, tc.keys(root, idx))
					got, err := simplified.UnmarshalFlat(marshalBody(t, body), wt, simplified.WithTemplate(compiled))
					if err != nil {
						t.Fatalf("UnmarshalFlat: %v", err)
					}
					if v := tc.read(got); v != tc.want {
						t.Errorf("read back %#v, want %#v", v, tc.want)
					}
				})
			}
			t.Run("index :1 refused in FLAT", func(t *testing.T) {
				body := indexedMetaBase()
				maps.Copy(body, tc.keys(root, ":1"))
				_, err := simplified.UnmarshalFlat(marshalBody(t, body), wt, simplified.WithTemplate(compiled))
				wantRefusal(t, err, simplified.ErrUnknownPath, tc.key(root, ":1"), "single-valued")
			})
			t.Run("second element refused through STRUCTURED", func(t *testing.T) {
				// STRUCTURED spells a second instance as a second array element, which
				// flattens to :1. The :0 element carries the same keys, so the body is
				// otherwise well formed. The flattened key indexes every segment, so
				// the refusal is checked for the segment it names.
				flat := indexedMetaBase()
				maps.Copy(flat, tc.keys(root, ":0"))
				maps.Copy(flat, tc.keys(root, ":1"))
				s, err := simplified.FlatToStructured(marshalBody(t, flat))
				if err != nil {
					t.Fatalf("FlatToStructured: %v", err)
				}
				_, err = simplified.UnmarshalStructured(s, wt, simplified.WithTemplate(compiled))
				wantRefusal(t, err, simplified.ErrUnknownPath, `"`+tc.seg+`" carries :1`, "single-valued")
			})
		})
	}
}

// TestREQ170SingleValuedIndexCarriesNoValue: each refusal of an index past
// the bound names the key and carries none of its value, by default, under the
// report option and under strict alike, since the key is matched content. One
// row per refusal site: an underscore family's owner segment, a composition
// metadata segment, the template root's own segment, and an ENTRY leaf
// segment.
func TestREQ170SingleValuedIndexCarriesNoValue(t *testing.T) {
	wt, compiled := corpusTarget(t)
	root := wt.Tree.ID
	const secret = "Secret Value 7f3a"
	for _, tc := range []struct {
		name string
		key  string
		// names is what the refusal must name: the key, or the family prefix
		// or leaf base the decode reports it by.
		names string
	}{
		{"underscore family owner", root + "/context:1/_health_care_facility|name", root + "/context:1/_health_care_facility"},
		{"composition metadata", root + "/composer:1|name", root + "/composer:1|name"},
		{"template root", root + ":1/composer|name", root + ":1/composer|name"},
		{"entry leaf", root + "/conformance_section/conformance_observation/language:1|code", root + "/conformance_section/conformance_observation/language:1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := indexedMetaBase()
			body[tc.key] = secret
			var reported []string
			for mode, opts := range map[string][]simplified.Option{
				"default":       nil,
				"WithUnmatched": {simplified.WithUnmatched(func(p string) { reported = append(reported, p) })},
				"WithStrict":    {simplified.WithStrict()},
			} {
				_, err := simplified.UnmarshalFlat(marshalBody(t, body), wt, append(opts, simplified.WithTemplate(compiled))...)
				if !errors.Is(err, simplified.ErrUnknownPath) {
					t.Fatalf("%s: err = %v, want ErrUnknownPath", mode, err)
				}
				if !strings.Contains(err.Error(), tc.names) {
					t.Errorf("%s: error %q does not name %q", mode, err, tc.names)
				}
				if strings.Contains(err.Error(), secret) {
					t.Errorf("%s: error %q carries the key's value", mode, err)
				}
			}
			if len(reported) != 0 {
				t.Errorf("WithUnmatched reported %q, want nothing for a matched key", reported)
			}
		})
	}
}
