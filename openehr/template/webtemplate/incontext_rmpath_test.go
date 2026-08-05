package webtemplate

// REQ-106 / REQ-121 — the in-context ↔ rmpath seam.
//
// The WebTemplate synthesizes in-context leaves per container RM type
// (§ Output shape): nodes that come from the RM, not from the template. The
// FLAT encoder (REQ-053) resolves every emitted node through
// openehr/rm/rmpath and treats rmpath's ErrPathNotFound as "absent optional",
// so an in-context leaf rmpath cannot resolve is **silently dropped** — no
// error, no output, data gone.
//
// PROBE-086 found several of these (EVENT `time`, INSTRUCTION `narrative` and
// `expiry_time`, and every EVENT_CONTEXT attribute); they were invisible to
// PROBE-076 because its input is this SDK's own output, which never carried the
// keys. This test guards the class rather than those instances: for every
// in-context leaf, rmpath must resolve the attribute on a populated instance of
// the container.
//
// Exempted leaves are listed in unserialisableIC. None of them is
// rmpath-attributable data loss today, for one of two reasons (see that map's
// doc): the encoder's leaf mapping drops the datatype regardless of what rmpath
// does, or the value is deliberately carried on the ctx/ surface instead. Each
// exemption records which reason applies and what the encode consequence is;
// when the blocking condition clears, delete the entry and the guard then
// enforces resolution. An entry that no longer matches any emitted leaf is
// reported as stale.

import (
	"reflect"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rmpath"
)

// unserialisableIC maps "RMTYPE.attr" to why the leaf is deliberately left
// unresolved in rmpath, and what that costs on encode. Two reason classes are
// present, and each entry states the mechanism that applies to it:
//
//   - Codec gap — a non-DV_ datatype (PARTY_PROXY, STRING) that leafToFlat drops
//     whether or not rmpath resolves it, so resolving would change nothing.
//   - Deliberate deferral — the value is already carried on another surface
//     (the ctx/ short forms, whose encode-only spelling ADR 0015 made
//     permanent), so resolving would double-spell it; or its ctx/ emission is
//     not written yet, so nothing consumes the resolution.
//
// Datatype is therefore not a standing reason: CODE_PHRASE is a non-DV_ type
// that leafToFlat *does* map since the PROBE-086 ratchet.
var unserialisableIC = map[string]string{
	"COMPOSITION.language":  "carried by ctx/language on encode; resolving here would double-spell it (the CODE_PHRASE leaf mapping exists since the PROBE-086 ratchet, so the datatype is no longer the reason)",
	"COMPOSITION.territory": "carried by ctx/territory on encode; resolving here would double-spell it (see COMPOSITION.language)",
	"COMPOSITION.composer":  "PARTY_PROXY: leafToFlat silently skips non-DV_ values; ctx/composer_name carries the name (external_ref is dropped — known deviation)",
	"EVENT_CONTEXT.start_time": "carried by ctx/time on encode; resolving here would double-spell the value " +
		"(ADR 0015 keeps encode ctx/-only) — no data loss today",
	"EVENT_CONTEXT.setting": "carried by ctx/setting|code + |value on encode (REQ-053 amended 2026-08-05, ADR 0015's " +
		"emission gap closed); resolving here would double-spell the value (encode stays ctx/-only, permanent under " +
		"ADR 0015, like start_time) — no data loss today",
	// ENTRY-level language / encoding are deliberately absent from this map: the
	// CODE_PHRASE leaf mapping landed, so those leaves now resolve through rmpath
	// — this guard enforces that, and TestEntryLanguageEncodingResolveToValues
	// pins the resolved value.
	"OBSERVATION.subject":          "PARTY_PROXY: leafToFlat silently skips non-DV_ values — codec gap, not an rmpath gap",
	"EVALUATION.subject":           "PARTY_PROXY: leafToFlat silently skips non-DV_ values — codec gap, not an rmpath gap",
	"INSTRUCTION.subject":          "PARTY_PROXY: leafToFlat silently skips non-DV_ values — codec gap, not an rmpath gap",
	"ACTION.subject":               "PARTY_PROXY: leafToFlat silently skips non-DV_ values — codec gap, not an rmpath gap",
	"ADMIN_ENTRY.subject":          "PARTY_PROXY: leafToFlat silently skips non-DV_ values — codec gap, not an rmpath gap",
	"ACTIVITY.action_archetype_id": "STRING: leafToFlat silently skips non-DV_ values — codec gap, not an rmpath gap",
}

// populated returns a LOCATABLE root carrying a populated instance of the
// container RM type, plus the path prefix at which that container sits inside
// the root, so a resolvable attribute yields a child and an unresolvable one
// does not — the distinction the guard turns on.
//
// EVENT_CONTEXT is not rm.Locatable and so cannot be an ItemAtPath root: it is
// reached through a populated COMPOSITION at /context, exactly as the FLAT
// encoder reaches it. Every other container is its own root (prefix "").
func populated(rmType string) (root rm.Locatable, prefix string, ok bool) {
	name := rm.DVText{Value: "x"}
	when := rm.DVDateTime{Value: "2026-08-01T00:00:00Z"}
	code := func(s string) rm.CodePhrase {
		return rm.CodePhrase{CodeString: s, TerminologyID: rm.TerminologyID{Value: "openehr"}}
	}
	// ENTRY language / encoding are set on every ENTRY subtype so that
	// TestEntryLanguageEncodingResolveToValues can compare a resolution against
	// a distinguishable value rather than a zero CODE_PHRASE.
	lang := rm.CodePhrase{CodeString: "en", TerminologyID: rm.TerminologyID{Value: "ISO_639-1"}}
	enc := rm.CodePhrase{CodeString: "UTF-8", TerminologyID: rm.TerminologyID{Value: "IANA_character-sets"}}
	switch rmType {
	case "COMPOSITION", "EVENT_CONTEXT":
		who, where := "Dr Who", "ward A3"
		comp := &rm.Composition{
			Name:      name,
			Language:  code("en"),
			Territory: code("NL"),
			Composer:  rm.PartyIdentified{Name: &who},
			Category:  rm.DVCodedText{DVText: rm.DVText{Value: "event"}, DefiningCode: code("433")},
			Context: &rm.EventContext{
				StartTime:          when,
				Setting:            rm.DVCodedText{DVText: rm.DVText{Value: "other care"}, DefiningCode: code("238")},
				OtherContext:       &rm.ItemTree{Name: rm.DVText{Value: "tree"}},
				HealthCareFacility: rm.PartyIdentified{Name: &who},
				Location:           &where,
				Participations:     []rm.Participation{{Function: name, Performer: rm.PartyIdentified{Name: &who}}},
			},
		}
		if rmType == "EVENT_CONTEXT" {
			return comp, "/context", true
		}
		return comp, "", true
	case "OBSERVATION":
		return &rm.Observation{Name: name, Language: lang, Encoding: enc}, "", true
	case "EVALUATION":
		return &rm.Evaluation{Name: name, Language: lang, Encoding: enc}, "", true
	case "INSTRUCTION":
		exp := when
		return &rm.Instruction{
			Name: name, Language: lang, Encoding: enc,
			Narrative: &rm.DVText{Value: "n"}, ExpiryTime: &exp,
		}, "", true
	case "ACTION":
		return &rm.Action{Name: name, Language: lang, Encoding: enc, Time: when}, "", true
	case "ADMIN_ENTRY":
		return &rm.AdminEntry{Name: name, Language: lang, Encoding: enc}, "", true
	case "ACTIVITY":
		return &rm.Activity{
			Name:   name,
			Timing: &rm.DVParsable{Value: "R2/2026-08-01T00:00:00Z/P1D", Formalism: "ISO8601"},
		}, "", true
	case "EVENT", "POINT_EVENT":
		return &rm.PointEvent[rm.ItemStructure]{Name: name, Time: when}, "", true
	case "INTERVAL_EVENT":
		return &rm.IntervalEvent[rm.ItemStructure]{
			Name: name, Time: when,
			MathFunction: rm.DVCodedText{DVText: rm.DVText{Value: "actual"}},
			Width:        rm.DVDuration{Value: "PT1H"},
		}, "", true
	}
	// A new container type reaching here fails loudly below rather than being
	// skipped.
	return nil, "", false
}

func TestInContextLeavesResolveViaRmpath(t *testing.T) {
	exempted := map[string]bool{}
	for rmType, leaves := range inContextByRM {
		for _, leaf := range leaves {
			key := rmType + "." + leaf.ID
			if why, ok := unserialisableIC[key]; ok {
				exempted[key] = true
				// A subtest so the skip and its reason are visible in the
				// output, not buried in a t.Logf on the parent.
				t.Run(key, func(t *testing.T) { t.Skip(why) })
				continue
			}
			t.Run(key, func(t *testing.T) {
				parent, prefix, ok := populated(rmType)
				if !ok {
					t.Fatalf("no populated instance for container %s — add one; "+
						"until then its in-context leaves cannot be guarded", rmType)
				}
				path := prefix + "/" + leaf.ID
				got, err := rmpath.ItemAtPath(parent, path)
				if err != nil {
					t.Fatalf("rmpath cannot resolve %s on %s: %v\n\n"+
						"The WebTemplate emits this leaf, so flat_encode will resolve it through "+
						"rmpath, get ErrPathNotFound, and silently drop the value. Either add the "+
						"attribute to rmpath's childrenAt switch, or — if the leaf must stay "+
						"unresolved — add %q to unserialisableIC with the reason and the encode "+
						"consequence.", path, rmType, err, key)
				}
				if got == nil {
					t.Errorf("rmpath resolved %s on %s to nil", path, rmType)
				}
			})
		}
	}
	// An exemption nobody consumed is stale: the leaf was renamed, dropped from
	// inContextByRM, or the key is a typo — either way the map is lying.
	for key, why := range unserialisableIC {
		if !exempted[key] {
			t.Errorf("stale unserialisableIC entry %q (%s): no in-context leaf with that "+
				"RMTYPE.attr is emitted — delete it or fix the key", key, why)
		}
	}
}

// entryCodes returns the ENTRY-level language / encoding an instance carries, so
// a resolution can be compared against the real field instead of against
// "non-nil". A type switch keeps it reflection-free, as rmpath is (REQ-024).
func entryCodes(root rm.Locatable) (language, encoding rm.CodePhrase, ok bool) {
	switch e := root.(type) {
	case *rm.Observation:
		return e.Language, e.Encoding, true
	case *rm.Evaluation:
		return e.Language, e.Encoding, true
	case *rm.Instruction:
		return e.Language, e.Encoding, true
	case *rm.Action:
		return e.Language, e.Encoding, true
	case *rm.AdminEntry:
		return e.Language, e.Encoding, true
	}
	return rm.CodePhrase{}, rm.CodePhrase{}, false
}

// TestEntryLanguageEncodingResolveToValues pins *what* the ENTRY language /
// encoding resolutions yield, which the guard above cannot: it only checks
// non-nil, and entryChildren always returns a child for these non-pointer
// CODE_PHRASE fields — a zero CODE_PHRASE would satisfy it. rmpath resolves both
// by value, so the FLAT encoder receives the instance's real code; that is the
// property the encode path depends on and this test asserts.
func TestEntryLanguageEncodingResolveToValues(t *testing.T) {
	for _, rmType := range []string{"OBSERVATION", "EVALUATION", "INSTRUCTION", "ACTION", "ADMIN_ENTRY"} {
		t.Run(rmType, func(t *testing.T) {
			root, prefix, ok := populated(rmType)
			if !ok {
				t.Fatalf("no populated instance for %s", rmType)
			}
			language, encoding, ok := entryCodes(root)
			if !ok {
				t.Fatalf("populated(%s) = %T, not an ENTRY subtype entryCodes knows", rmType, root)
			}
			if language.CodeString == "" || encoding.CodeString == "" {
				t.Fatalf("populated(%s) leaves language/encoding unset — a value assertion "+
					"would then pass on a zero CODE_PHRASE", rmType)
			}
			for attr, want := range map[string]rm.CodePhrase{"language": language, "encoding": encoding} {
				path := prefix + "/" + attr
				got, err := rmpath.ItemAtPath(root, path)
				if err != nil {
					t.Fatalf("ItemAtPath(%s) on %s = %v", path, rmType, err)
				}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("ItemAtPath(%s) on %s = %#v, want the CODE_PHRASE value %#v",
						path, rmType, got, want)
				}
			}
		})
	}
}

// TestPopulatedEventContextIsReachable keeps the EVENT_CONTEXT arm of the guard
// honest. Both its in-context leaves are exempt today, so nothing above
// exercises the /context prefix; if that arm were broken (as it was before —
// populated returned no instance at all), the two exemptions would be
// unauditable rather than deliberate. other_context stands in for a future
// non-exempt leaf, and the exempted attributes must be *set* on the instance so
// that deleting an exemption fails for the right reason.
func TestPopulatedEventContextIsReachable(t *testing.T) {
	root, prefix, ok := populated("EVENT_CONTEXT")
	if !ok {
		t.Fatal("populated(EVENT_CONTEXT) yields no instance — its exemptions cannot be audited")
	}
	if prefix == "" {
		t.Fatal("EVENT_CONTEXT needs a non-empty path prefix: it is not rm.Locatable, " +
			"so it can only be reached through a COMPOSITION")
	}
	if _, err := rmpath.ItemAtPath(root, prefix+"/other_context"); err != nil {
		t.Fatalf("ItemAtPath(%s/other_context) = %v", prefix, err)
	}
	comp, ok := root.(*rm.Composition)
	if !ok || comp.Context == nil {
		t.Fatalf("populated(EVENT_CONTEXT) root = %T, want a *rm.Composition carrying a context", root)
	}
	if comp.Context.StartTime.Value == "" || comp.Context.Setting.Value == "" {
		t.Error("the populated EVENT_CONTEXT leaves start_time/setting unset — deleting either " +
			"exemption would then fail because the value is missing, not because rmpath cannot resolve it")
	}
}
