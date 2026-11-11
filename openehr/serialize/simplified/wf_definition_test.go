package simplified_test

// REQ-140 — INSTRUCTION `wf_definition`, which ITS-REST 1.1.0 Simplified
// Formats § INSTRUCTION maps at `<instruction>/_wf_definition` to a
// DV_PARSABLE (issue 426). Decode reads it through the DV_PARSABLE value
// grammar, the value bare or as `|value` beside `|formalism`; encode writes
// `|value` + `|formalism`, as the PROBE-086 corpus spells it, and a value
// those two suffixes cannot carry rides `_wf_definition|raw`.

import (
	"encoding/json"
	"errors"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rmpath"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// wfInstruction is the test_all_types INSTRUCTION issue 426 writes under, and
// wfInstructionEarlier the spelling the issue gives it, its SECTION's earlier
// spelling test_all_types3.
const (
	wfInstruction        = "test_all_types/test_all_types_2:0/section_2/section_3/test_all_types:0"
	wfInstructionEarlier = "test_all_types/test_all_types3:0/section_2/section_3/test_all_types:0"
)

// wfConformanceInstruction is the PROBE-086 corpus template's INSTRUCTION.
const wfConformanceInstruction = "conformance-ehrbase.de.v0/conformance_section/conformance_instruction"

// wfIssueBody is issue 426's body: the shorthand test base with the
// INSTRUCTION's narrative and the wf_definition keys of extra.
func wfIssueBody(extra map[string]any) map[string]any {
	body := shorthandBody(map[string]any{wfInstruction + "/narrative": "Human readable instruction narrative"})
	maps.Copy(body, extra)
	return body
}

// wfDefinitionOf returns the wf_definition of the one INSTRUCTION wt models,
// found in comp at the INSTRUCTION's path in the Web Template.
func wfDefinitionOf(t *testing.T, wt *webtemplate.WebTemplate, comp *rm.Composition) *rm.DVParsable {
	t.Helper()
	var paths []string
	var walk func(n *webtemplate.Node)
	walk = func(n *webtemplate.Node) {
		if n.RMType == "INSTRUCTION" {
			paths = append(paths, n.AQLPath)
		}
		for _, ch := range n.Children {
			walk(ch)
		}
	}
	walk(wt.Tree)
	if len(paths) != 1 {
		t.Fatalf("the Web Template models %d INSTRUCTION nodes, want 1 — fixture changed?", len(paths))
	}
	items, err := rmpath.ItemsAtPath(comp, paths[0])
	if err != nil || len(items) != 1 {
		t.Fatalf("ItemsAtPath(%s) = %d items, %v; want the INSTRUCTION", paths[0], len(items), err)
	}
	instr, ok := items[0].(*rm.Instruction)
	if !ok {
		t.Fatalf("%s holds a %T, want *rm.Instruction", paths[0], items[0])
	}
	return instr.WfDefinition
}

// TestREQ140WfDefinitionDecodes — REQ-140. Issue 426's body decodes, with and
// without the template, in FLAT and in STRUCTURED, and its INSTRUCTION's
// wf_definition is the DV_PARSABLE the keys spell, whether the value is
// written `|value` or bare, and under the SECTION's earlier spelling too.
func TestREQ140WfDefinitionDecodes(t *testing.T) {
	wt, compiled := shorthandTarget(t)
	want := rm.DVParsable{Value: "wf_definition", Formalism: "formalism"}
	for _, spelling := range []struct {
		name string
		keys map[string]any
	}{
		{"|value", map[string]any{
			wfInstruction + "/_wf_definition|value":     "wf_definition",
			wfInstruction + "/_wf_definition|formalism": "formalism",
		}},
		{"bare value", map[string]any{
			wfInstruction + "/_wf_definition":           "wf_definition",
			wfInstruction + "/_wf_definition|formalism": "formalism",
		}},
		{"earlier spelling", map[string]any{
			wfInstructionEarlier + "/_wf_definition|value":     "wf_definition",
			wfInstructionEarlier + "/_wf_definition|formalism": "formalism",
		}},
	} {
		body := mustJSON(t, wfIssueBody(spelling.keys))
		for _, mode := range shorthandModes(compiled) {
			for _, format := range []string{"FLAT", "STRUCTURED"} {
				t.Run(spelling.name+"/"+mode.name+"/"+format, func(t *testing.T) {
					comp := decodeAs(t, format, body, wt, mode.opts...)
					got := wfDefinitionOf(t, wt, comp)
					if got == nil {
						t.Fatal("decoded INSTRUCTION has no wf_definition")
					}
					if got.Value != want.Value || got.Formalism != want.Formalism || got.Charset != nil || got.Language != nil {
						t.Errorf("wf_definition = %+v, want %+v", *got, want)
					}
				})
			}
		}
	}
}

// TestREQ140WfDefinitionEncodes — REQ-140. Encode writes a wf_definition as
// `_wf_definition|value` + `|formalism`, the corpus spelling, never the bare
// key, and decode reads that back to the same value.
func TestREQ140WfDefinitionEncodes(t *testing.T) {
	wt, compiled := shorthandTarget(t)
	keys := map[string]any{
		wfInstruction + "/_wf_definition":           "wf_definition",
		wfInstruction + "/_wf_definition|formalism": "formalism",
	}
	comp := decodeWith(t, wt, wfIssueBody(keys), simplified.WithTemplate(compiled))
	out, err := simplified.MarshalFlat(comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]any{
		wfInstruction + "/_wf_definition|value":     "wf_definition",
		wfInstruction + "/_wf_definition|formalism": "formalism",
	} {
		if got[key] != want {
			t.Errorf("MarshalFlat wrote %q = %#v, want %#v", key, got[key], want)
		}
	}
	if v, written := got[wfInstruction+"/_wf_definition"]; written {
		t.Errorf("MarshalFlat wrote the bare %q = %#v; the corpus spelling is |value", wfInstruction+"/_wf_definition", v)
	}
	back := wfDefinitionOf(t, wt, decodeAs(t, "FLAT", out, wt))
	if back == nil || back.Value != "wf_definition" || back.Formalism != "formalism" {
		t.Errorf("decode of the encoded body: wf_definition = %+v, want value wf_definition, formalism formalism", back)
	}
}

// TestREQ140WfDefinitionCorpusRoundTrip — REQ-140, PROBE-086. The corpus body
// that writes `_wf_definition|value` + `|formalism` decodes and re-encodes
// with both keys and their values, in FLAT and through STRUCTURED.
func TestREQ140WfDefinitionCorpusRoundTrip(t *testing.T) {
	wt, compiled := conformanceTarget(t)
	raw, err := os.ReadFile(fixtures.FlatConformanceFlat("ehrbase_conformance_instruction"))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		wfConformanceInstruction + "/_wf_definition|value":     "wf_definition",
		wfConformanceInstruction + "/_wf_definition|formalism": "formalism",
	}
	for _, mode := range shorthandModes(compiled) {
		t.Run(mode.name, func(t *testing.T) {
			comp, err := simplified.UnmarshalFlat(raw, wt, mode.opts...)
			if err != nil {
				t.Fatalf("UnmarshalFlat(ehrbase_conformance_instruction): %v", err)
			}
			flat, err := simplified.MarshalFlat(comp, wt)
			if err != nil {
				t.Fatalf("MarshalFlat: %v", err)
			}
			structured, err := simplified.MarshalStructured(comp, wt)
			if err != nil {
				t.Fatalf("MarshalStructured: %v", err)
			}
			viaStructured, err := simplified.UnmarshalStructured(structured, wt, mode.opts...)
			if err != nil {
				t.Fatalf("UnmarshalStructured: %v", err)
			}
			flatViaStructured, err := simplified.MarshalFlat(viaStructured, wt)
			if err != nil {
				t.Fatalf("MarshalFlat (via STRUCTURED): %v", err)
			}
			for leg, out := range map[string][]byte{"FLAT": flat, "STRUCTURED": flatViaStructured} {
				var got map[string]any
				if err := json.Unmarshal(out, &got); err != nil {
					t.Fatal(err)
				}
				for _, key := range slices.Sorted(maps.Keys(want)) {
					if got[key] != want[key] {
						t.Errorf("%s re-encode wrote %q = %#v, want %#v", leg, key, got[key], want[key])
					}
				}
			}
		})
	}
}

// TestREQ140WfDefinitionRaw — REQ-140. A wf_definition carrying a charset or a
// language, which `|value` + `|formalism` cannot carry, rides
// `_wf_definition|raw` at its own key, as an `original_content` does, and
// reads back whole.
func TestREQ140WfDefinitionRaw(t *testing.T) {
	wt, compiled := shorthandTarget(t)
	comp := decodeWith(t, wt, wfIssueBody(map[string]any{
		wfInstruction + "/_wf_definition|value":     "wf_definition",
		wfInstruction + "/_wf_definition|formalism": "formalism",
	}), simplified.WithTemplate(compiled))
	wf := wfDefinitionOf(t, wt, comp)
	if wf == nil {
		t.Fatal("decoded INSTRUCTION has no wf_definition")
	}
	wf.Charset = &rm.CodePhrase{CodeString: "UTF-8", TerminologyID: rm.TerminologyID{Value: "IANA_character-sets"}}
	wf.Language = &rm.CodePhrase{CodeString: "en", TerminologyID: rm.TerminologyID{Value: "ISO_639-1"}}
	want, err := canjson.Marshal(wf)
	if err != nil {
		t.Fatal(err)
	}
	out, err := simplified.MarshalFlat(comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	var written []string
	for key := range got {
		if strings.HasPrefix(key, wfInstruction+"/_wf_definition") {
			written = append(written, key)
		}
	}
	if slices.Sort(written); !slices.Equal(written, []string{wfInstruction + "/_wf_definition|raw"}) {
		t.Errorf("MarshalFlat wrote the wf_definition as %q, want it at %q alone", written, wfInstruction+"/_wf_definition|raw")
	}
	back := wfDefinitionOf(t, wt, decodeAs(t, "FLAT", out, wt))
	if back == nil {
		t.Fatal("decode of the encoded body: no wf_definition")
	}
	if gotJSON, err := canjson.Marshal(back); err != nil || string(gotJSON) != string(want) {
		t.Errorf("decode of the encoded body: wf_definition = %s (%v), want %s", gotJSON, err, want)
	}
}

// TestREQ140WfDefinitionHalfValueRefused — REQ-140, REQ-170. DV_PARSABLE's
// value and formalism are both RM-mandatory, so a wf_definition that spells
// one without the other is a missing required suffix, refused in every mode
// naming the family's key, as at a DV_PARSABLE leaf: never decoded to a
// coerced empty string, and never unmatched content.
func TestREQ140WfDefinitionHalfValueRefused(t *testing.T) {
	wt, compiled := shorthandTarget(t)
	for _, tc := range []struct {
		name, missing string
		keys          map[string]any
	}{
		{"no |formalism", "missing required |formalism", map[string]any{wfInstruction + "/_wf_definition|value": "wf_definition"}},
		{"no value", "missing required bare value", map[string]any{wfInstruction + "/_wf_definition|formalism": "formalism"}},
	} {
		body := mustJSON(t, wfIssueBody(tc.keys))
		modes := []struct {
			name string
			opts []simplified.Option
		}{
			{"default", nil},
			{"with template", []simplified.Option{simplified.WithTemplate(compiled)}},
			{"strict", []simplified.Option{simplified.WithStrict()}},
		}
		for _, mode := range modes {
			t.Run(tc.name+"/"+mode.name, func(t *testing.T) {
				var reported []string
				opts := append(slices.Clone(mode.opts), simplified.WithUnmatched(func(key string) { reported = append(reported, key) }))
				_, err := simplified.UnmarshalFlat(body, wt, opts...)
				if !errors.Is(err, simplified.ErrUnsupportedDatatype) {
					t.Fatalf("UnmarshalFlat err = %v, want ErrUnsupportedDatatype", err)
				}
				for _, part := range []string{wfInstruction + "/_wf_definition", tc.missing} {
					if !strings.Contains(err.Error(), part) {
						t.Errorf("err = %v, want it to name %q", err, part)
					}
				}
				if len(reported) != 0 {
					t.Errorf("reported %q as unmatched, but the decode reads the key", reported)
				}
			})
		}
	}
}

// TestREQ140WfDefinitionUnmatchedElsewhere — REQ-140, REQ-170. Only an
// INSTRUCTION declares wf_definition, so at any other owner the family names
// nothing and is unmatched content: ignored by default, reported, and refused
// under strict.
func TestREQ140WfDefinitionUnmatchedElsewhere(t *testing.T) {
	wt, _ := conformanceTarget(t)
	for _, owner := range []string{
		"conformance-ehrbase.de.v0/conformance_section",
		"conformance-ehrbase.de.v0/conformance_section/conformance_observation",
	} {
		t.Run(owner, func(t *testing.T) {
			keys := map[string]any{
				owner + "/_wf_definition|value":     "wf_definition",
				owner + "/_wf_definition|formalism": "formalism",
			}
			body := map[string]any{
				"ctx/language":  "en",
				"ctx/territory": "US",
				"ctx/time":      "2021-12-21T14:19:31.649613+01:00",
				"conformance-ehrbase.de.v0/conformance_section/conformance_observation/any_event:0/dv_quantity|magnitude": 65.9,
				"conformance-ehrbase.de.v0/conformance_section/conformance_observation/any_event:0/dv_quantity|unit":      "unit",
			}
			maps.Copy(body, keys)
			var reported []string
			if _, err := simplified.UnmarshalFlat(mustJSON(t, body), wt,
				simplified.WithUnmatched(func(key string) { reported = append(reported, key) })); err != nil {
				t.Fatalf("UnmarshalFlat (report) = %v, want the keys ignored", err)
			}
			if want := slices.Sorted(maps.Keys(keys)); !slices.Equal(reported, want) {
				t.Errorf("reported %q, want %q", reported, want)
			}
			if _, err := simplified.UnmarshalFlat(mustJSON(t, body), wt, simplified.WithStrict()); !errors.Is(err, simplified.ErrUnknownPath) {
				t.Errorf("UnmarshalFlat (strict) err = %v, want ErrUnknownPath", err)
			}
		})
	}
}

// conformanceTarget builds the PROBE-086 corpus Web Template and compiled
// template.
func conformanceTarget(t *testing.T) (*webtemplate.WebTemplate, *templatecompile.Compiled) {
	t.Helper()
	opt, err := readOPTFile(fixtures.FlatConformanceOpt())
	if err != nil {
		t.Fatalf("readOPTFile(corpus OPT): %v", err)
	}
	return compileTemplate(t, "conformance-ehrbase.de.v0", opt)
}
