package simplified_test

// REQ-140 — ACTION `instruction_details`, which EHRbase writes on every ACTION
// as `<action>/_instruction_details|composition_uid`, `|activity_id` and
// `|path` (issue 457). Decode reads it into an INSTRUCTION_DETAILS whose
// instruction_id is a LOCATABLE_REF to an INSTRUCTION in namespace local, its
// id a HIER_OBJECT_ID and its path the `|path` an empty one leaves absent;
// encode writes the three suffixes back, `|path` only when there is one.

import (
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
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

// The ACTION of the IDCR Procedures List the issue writes under, its
// `_instruction_details` family, and the values the issue's keys carry.
const (
	idProcedure = "procedures_list/procedures/procedure:0"
	idFamily    = idProcedure + "/_instruction_details"
	idUID       = "2ffc2f14-68cd-3c98-8d2e-41fd34638ac1"
	idActivity  = "activities[at0001]"
	idPath      = "/content[openEHR-EHR-SECTION.procedures_rcp.v1]/items[openEHR-EHR-INSTRUCTION.request.v0]"
)

// proceduresTarget builds the Web Template and the compiled template of the
// IDCR Procedures List, the EHRbase test template the issue's body is
// written against.
func proceduresTarget(t *testing.T) (*webtemplate.WebTemplate, *templatecompile.Compiled) {
	t.Helper()
	const id = "IDCR - Procedures List.v1"
	opt, err := readOPTFile(filepath.Join(fixtures.CorpusRoot(), "ehrbase-service", "templates", id+".opt"))
	if err != nil {
		t.Fatalf("readOPTFile(%s): %v", id, err)
	}
	return compileTemplate(t, id, opt)
}

// proceduresBody is one completed procedure, with extra merged over it.
func proceduresBody(extra map[string]any) map[string]any {
	body := cdrBody(map[string]any{
		idProcedure + "/procedure_name":                           "Appendectomy",
		idProcedure + "/time":                                     "2026-10-26T09:00:00Z",
		idProcedure + "/ism_transition/current_state|code":        "532",
		idProcedure + "/ism_transition/current_state|value":       "completed",
		idProcedure + "/ism_transition/current_state|terminology": "openehr",
		idProcedure + "/ism_transition/careflow_step|code":        "at0043",
		idProcedure + "/ism_transition/careflow_step|value":       "Procedure completed",
		idProcedure + "/ism_transition/careflow_step|terminology": "local",
	})
	maps.Copy(body, extra)
	return body
}

// idKeys is the family's three keys as EHRbase writes them, with path as the
// |path value; a nil path leaves |path out.
func idKeys(path any, withPath bool) map[string]any {
	keys := map[string]any{
		idFamily + "|composition_uid": idUID,
		idFamily + "|activity_id":     idActivity,
	}
	if withPath {
		keys[idFamily+"|path"] = path
	}
	return keys
}

// soleAction returns the one ACTION wt models, found in comp at the ACTION's
// path in the Web Template.
func soleAction(t *testing.T, wt *webtemplate.WebTemplate, comp *rm.Composition) *rm.Action {
	t.Helper()
	var paths []string
	var walk func(n *webtemplate.Node)
	walk = func(n *webtemplate.Node) {
		if n.RMType == "ACTION" {
			paths = append(paths, n.AQLPath)
		}
		for _, ch := range n.Children {
			walk(ch)
		}
	}
	walk(wt.Tree)
	if len(paths) != 1 {
		t.Fatalf("the Web Template models %d ACTION nodes, want 1 — fixture changed?", len(paths))
	}
	items, err := rmpath.ItemsAtPath(comp, paths[0])
	if err != nil || len(items) != 1 {
		t.Fatalf("ItemsAtPath(%s) = %d items, %v; want the ACTION", paths[0], len(items), err)
	}
	action, ok := items[0].(*rm.Action)
	if !ok {
		t.Fatalf("%s holds a %T, want *rm.Action", paths[0], items[0])
	}
	return action
}

// wantInstructionDetails is the INSTRUCTION_DETAILS the family's keys spell:
// a reference to an INSTRUCTION in namespace local whose id is the
// HIER_OBJECT_ID uid, at path (nil for none), from activity.
func wantInstructionDetails(uid, activity string, path *string) *rm.InstructionDetails {
	return &rm.InstructionDetails{
		ActivityID: activity,
		InstructionID: rm.LocatableRef{
			ObjectRef: rm.ObjectRef{Namespace: "local", Type: "INSTRUCTION"},
			ID:        rm.HierObjectID{Value: uid},
			Path:      path,
		},
	}
}

// sameInstructionDetails compares got with want by their canonical JSON.
func sameInstructionDetails(t *testing.T, got, want *rm.InstructionDetails) {
	t.Helper()
	if got == nil {
		t.Fatal("the ACTION has no instruction_details")
	}
	g, err := canjson.Marshal(got)
	if err != nil {
		t.Fatalf("canjson.Marshal(instruction_details): %v", err)
	}
	w, err := canjson.Marshal(want)
	if err != nil {
		t.Fatalf("canjson.Marshal(want): %v", err)
	}
	if string(g) != string(w) {
		t.Errorf("instruction_details = %s, want %s", g, w)
	}
}

// familyKeys returns the keys of the FLAT document out that sit under the
// `_instruction_details` family at family, with their values.
func familyKeys(t *testing.T, out []byte, family string) map[string]any {
	t.Helper()
	var flat map[string]any
	if err := json.Unmarshal(out, &flat); err != nil {
		t.Fatal(err)
	}
	got := make(map[string]any)
	for key, v := range flat {
		if strings.HasPrefix(key, family) {
			got[key] = v
		}
	}
	return got
}

// TestREQ140InstructionDetailsDecodes — REQ-140, REQ-053, issue 457. The
// issue's keys decode, in FLAT and in STRUCTURED, with and without the
// template, by default and under strict, into the ACTION's
// instruction_details: an empty or null |path, or none, leaves the
// reference's path absent, and a |path sets it.
func TestREQ140InstructionDetailsDecodes(t *testing.T) {
	wt, compiled := proceduresTarget(t)
	modes := shorthandModes(compiled)
	for _, mode := range strictModes(compiled) {
		mode.name += ", strict"
		modes = append(modes, mode)
	}
	for _, tc := range []struct {
		name string
		keys map[string]any
		path *string
	}{
		{"the issue's empty |path", idKeys("", true), nil},
		{"a |path", idKeys(idPath, true), new(idPath)},
		{"no |path", idKeys(nil, false), nil},
		{"a null |path", idKeys(nil, true), nil},
	} {
		body := mustJSON(t, proceduresBody(tc.keys))
		for _, mode := range modes {
			for _, format := range cdrFormats {
				t.Run(tc.name+"/"+mode.name+"/"+format, func(t *testing.T) {
					comp := decodeAs(t, format, body, wt, mode.opts...)
					sameInstructionDetails(t, soleAction(t, wt, comp).InstructionDetails, wantInstructionDetails(idUID, idActivity, tc.path))
				})
			}
		}
	}
}

// TestREQ140InstructionDetailsEncodes — REQ-140. Encode writes an ACTION's
// instruction_details as `|composition_uid`, `|activity_id` and, when the
// reference has a path that is not empty, `|path`; FLAT and STRUCTURED read
// that back to the same value, and the STRUCTURED document carries the family
// as an `_instruction_details` member holding the three suffixes.
func TestREQ140InstructionDetailsEncodes(t *testing.T) {
	wt, compiled := proceduresTarget(t)
	for _, tc := range []struct {
		name     string
		path     *string
		readBack *string
		want     map[string]any
	}{
		{"with a path", new(idPath), new(idPath), idKeys(idPath, true)},
		{"without a path", nil, nil, idKeys(nil, false)},
		{"with an empty path", new(""), nil, idKeys(nil, false)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			comp := decodeWith(t, wt, proceduresBody(nil), simplified.WithTemplate(compiled))
			soleAction(t, wt, comp).InstructionDetails = wantInstructionDetails(idUID, idActivity, tc.path)
			out, err := simplified.MarshalFlat(comp, wt)
			if err != nil {
				t.Fatalf("MarshalFlat: %v", err)
			}
			if got := familyKeys(t, out, idFamily); !maps.Equal(got, tc.want) {
				t.Errorf("MarshalFlat wrote %v, want %v", got, tc.want)
			}
			want := wantInstructionDetails(idUID, idActivity, tc.readBack)
			sameInstructionDetails(t, soleAction(t, wt, decodeAs(t, "FLAT", out, wt)).InstructionDetails, want)

			structured, err := simplified.MarshalStructured(comp, wt)
			if err != nil {
				t.Fatalf("MarshalStructured: %v", err)
			}
			member, ok := structuredMember(t, structured, "_instruction_details")
			if !ok {
				t.Fatalf("MarshalStructured wrote no _instruction_details member: %s", structured)
			}
			var suffixes []string
			if list, isList := member.([]any); isList && len(list) == 1 {
				if obj, isObj := list[0].(map[string]any); isObj {
					suffixes = slices.Sorted(maps.Keys(obj))
				}
			}
			var wantSuffixes []string
			for key := range tc.want {
				wantSuffixes = append(wantSuffixes, strings.TrimPrefix(key, idFamily))
			}
			if slices.Sort(wantSuffixes); !slices.Equal(suffixes, wantSuffixes) {
				t.Errorf("STRUCTURED _instruction_details = %v, want one object holding %q", member, wantSuffixes)
			}
			back, err := simplified.UnmarshalStructured(structured, wt)
			if err != nil {
				t.Fatalf("UnmarshalStructured: %v", err)
			}
			sameInstructionDetails(t, soleAction(t, wt, back).InstructionDetails, want)
		})
	}
}

// structuredMember returns the value of the first member named name in the
// STRUCTURED document doc, searched depth first in member-name order.
func structuredMember(t *testing.T, doc []byte, name string) (any, bool) {
	t.Helper()
	var root any
	if err := json.Unmarshal(doc, &root); err != nil {
		t.Fatal(err)
	}
	var find func(node any) (any, bool)
	find = func(node any) (any, bool) {
		switch n := node.(type) {
		case map[string]any:
			if v, ok := n[name]; ok {
				return v, true
			}
			for _, k := range slices.Sorted(maps.Keys(n)) {
				if v, ok := find(n[k]); ok {
					return v, true
				}
			}
		case []any:
			for _, el := range n {
				if v, ok := find(el); ok {
					return v, true
				}
			}
		}
		return nil, false
	}
	return find(root)
}

// TestREQ140InstructionDetailsCorpusRoundTrip — REQ-140, PROBE-086. The
// corpus ACTION body, which writes the three keys with a |path, decodes and
// re-encodes with all three and their values, in FLAT and through STRUCTURED.
func TestREQ140InstructionDetailsCorpusRoundTrip(t *testing.T) {
	wt, compiled := conformanceTarget(t)
	raw, err := os.ReadFile(fixtures.FlatConformanceFlat("ehrbase_conformance_action"))
	if err != nil {
		t.Fatal(err)
	}
	family := notModelledAction + "/_instruction_details"
	want := map[string]any{
		family + "|composition_uid": "4cdc3017-d8c5-4cd3-9900-f3bb7171d006",
		family + "|activity_id":     "activities[at0001]",
		family + "|path":            "/content[openEHR-EHR-SECTION.conformance_section.v0]/items[openEHR-EHR-INSTRUCTION.conformance_instruction.v0]",
	}
	for _, mode := range shorthandModes(compiled) {
		t.Run(mode.name, func(t *testing.T) {
			comp, err := simplified.UnmarshalFlat(raw, wt, mode.opts...)
			if err != nil {
				t.Fatalf("UnmarshalFlat(ehrbase_conformance_action): %v", err)
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
				if got := familyKeys(t, out, family); !maps.Equal(got, want) {
					t.Errorf("%s re-encode wrote %v, want %v", leg, got, want)
				}
			}
		})
	}
}

// TestREQ140InstructionDetailsMissingRequiredRefused — REQ-140, REQ-170,
// REQ-053. INSTRUCTION_DETAILS' activity_id and its reference's id are
// RM-mandatory, so a family without |composition_uid or |activity_id, or with
// a null one, is a missing required suffix: refused in every mode, in FLAT
// and in STRUCTURED, naming the family and the suffix, and never reported as
// unmatched content.
func TestREQ140InstructionDetailsMissingRequiredRefused(t *testing.T) {
	wt, compiled := proceduresTarget(t)
	modes := []struct {
		name string
		opts []simplified.Option
	}{
		{"default", nil},
		{"with template", []simplified.Option{simplified.WithTemplate(compiled)}},
		{"strict", []simplified.Option{simplified.WithStrict()}},
	}
	for _, tc := range []struct {
		name, missing string
		keys          map[string]any
	}{
		{"no |composition_uid", "|composition_uid", map[string]any{idFamily + "|activity_id": idActivity, idFamily + "|path": ""}},
		{"a null |composition_uid", "|composition_uid", map[string]any{idFamily + "|composition_uid": nil, idFamily + "|activity_id": idActivity}},
		{"no |activity_id", "|activity_id", map[string]any{idFamily + "|composition_uid": idUID, idFamily + "|path": idPath}},
		{"a null |activity_id", "|activity_id", map[string]any{idFamily + "|composition_uid": idUID, idFamily + "|activity_id": nil}},
	} {
		body := mustJSON(t, proceduresBody(tc.keys))
		for _, mode := range modes {
			for _, format := range cdrFormats {
				t.Run(tc.name+"/"+mode.name+"/"+format, func(t *testing.T) {
					var reported []string
					opts := append(slices.Clone(mode.opts), simplified.WithUnmatched(func(key string) { reported = append(reported, key) }))
					err := decodeErr(t, format, body, wt, opts...)
					if !errors.Is(err, simplified.ErrUnsupportedDatatype) {
						t.Fatalf("err = %v, want ErrUnsupportedDatatype", err)
					}
					for _, part := range []string{"/_instruction_details", "required " + tc.missing} {
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
}

// TestREQ140InstructionDetailsAllNullIsAbsent — REQ-053, REQ-140. A family
// whose every key holds JSON null carries nothing: the ACTION has no
// instruction_details, and strict decodes the body.
func TestREQ140InstructionDetailsAllNullIsAbsent(t *testing.T) {
	wt, compiled := proceduresTarget(t)
	body := mustJSON(t, proceduresBody(map[string]any{
		idFamily + "|composition_uid": nil,
		idFamily + "|activity_id":     nil,
		idFamily + "|path":            nil,
	}))
	for _, mode := range strictModes(compiled) {
		for _, format := range cdrFormats {
			t.Run(mode.name+"/"+format, func(t *testing.T) {
				if got := soleAction(t, wt, decodeAs(t, format, body, wt, mode.opts...)).InstructionDetails; got != nil {
					t.Errorf("instruction_details = %+v, want none", *got)
				}
			})
		}
	}
}

// TestREQ140InstructionDetailsComputedInputsUnmatched — REQ-140, REQ-170. The
// reference also reads `|instruction_uid`, `|wt_path`, `|instruction_index`
// and `|activity_index`, from which it would compute the path or the
// activity_id. They are outside the family's suffix set, so they are
// unmatched content: by default the decode reads the family without them, the
// report option names each, and strict refuses them as unmatched, not as a
// path this SDK does not model.
func TestREQ140InstructionDetailsComputedInputsUnmatched(t *testing.T) {
	wt, _ := proceduresTarget(t)
	computed := map[string]any{
		idFamily + "|instruction_uid":   idUID,
		idFamily + "|wt_path":           "procedures_list/request",
		idFamily + "|instruction_index": 0,
		idFamily + "|activity_index":    0,
	}
	keys := idKeys(idPath, true)
	maps.Copy(keys, computed)
	body := mustJSON(t, proceduresBody(keys))

	var reported []string
	comp, err := simplified.UnmarshalFlat(body, wt, simplified.WithUnmatched(func(key string) { reported = append(reported, key) }))
	if err != nil {
		t.Fatalf("UnmarshalFlat (report) = %v, want the computed inputs ignored", err)
	}
	sameInstructionDetails(t, soleAction(t, wt, comp).InstructionDetails, wantInstructionDetails(idUID, idActivity, new(idPath)))
	if slices.Sort(reported); !slices.Equal(reported, slices.Sorted(maps.Keys(computed))) {
		t.Errorf("reported %q, want %q", reported, slices.Sorted(maps.Keys(computed)))
	}
	_, err = simplified.UnmarshalFlat(body, wt, simplified.WithStrict())
	if !errors.Is(err, simplified.ErrUnknownPath) || errors.Is(err, simplified.ErrNotModelled) {
		t.Errorf("UnmarshalFlat (strict) err = %v, want ErrUnknownPath and not ErrNotModelled", err)
	}
}

// TestREQ140InstructionDetailsUnmatchedElsewhere — REQ-140, REQ-170. Only an
// ACTION declares instruction_details, so at a SECTION the family names
// nothing and is unmatched content: ignored by default, reported, and refused
// under strict as unmatched.
func TestREQ140InstructionDetailsUnmatchedElsewhere(t *testing.T) {
	wt, _ := proceduresTarget(t)
	section := "procedures_list/procedures/_instruction_details"
	keys := map[string]any{section + "|composition_uid": idUID, section + "|activity_id": idActivity}
	body := mustJSON(t, proceduresBody(keys))
	var reported []string
	if _, err := simplified.UnmarshalFlat(body, wt, simplified.WithUnmatched(func(key string) { reported = append(reported, key) })); err != nil {
		t.Fatalf("UnmarshalFlat (report) = %v, want the keys ignored", err)
	}
	if want := slices.Sorted(maps.Keys(keys)); !slices.Equal(reported, want) {
		t.Errorf("reported %q, want %q", reported, want)
	}
	_, err := simplified.UnmarshalFlat(body, wt, simplified.WithStrict())
	if !errors.Is(err, simplified.ErrUnknownPath) || errors.Is(err, simplified.ErrNotModelled) {
		t.Errorf("UnmarshalFlat (strict) err = %v, want ErrUnknownPath and not ErrNotModelled", err)
	}
}

// TestREQ140InstructionDetailsEncodeRefusals — REQ-140. The suffix set
// implies a reference to an INSTRUCTION in namespace local whose id is a
// HIER_OBJECT_ID, and has no channel for wf_details or a `|raw` carrier. An
// instruction_details it cannot carry is refused, in FLAT and in STRUCTURED,
// with ErrUnsupportedDatatype naming the family's key and not the value,
// rather than written as a different value or dropped.
func TestREQ140InstructionDetailsEncodeRefusals(t *testing.T) {
	wt, compiled := proceduresTarget(t)
	const secret = "secret-value-0457"
	for _, tc := range []struct {
		name   string
		mutate func(d *rm.InstructionDetails)
	}{
		{"a namespace other than local", func(d *rm.InstructionDetails) { d.InstructionID.Namespace = secret }},
		{"a type other than INSTRUCTION", func(d *rm.InstructionDetails) { d.InstructionID.Type = secret }},
		{"an OBJECT_VERSION_ID", func(d *rm.InstructionDetails) {
			d.InstructionID.ID = rm.ObjectVersionID{Value: secret + "::ehrbase.org::1"}
		}},
		{"no id", func(d *rm.InstructionDetails) { d.InstructionID.ID = nil }},
		{"a nil id", func(d *rm.InstructionDetails) { d.InstructionID.ID = (*rm.HierObjectID)(nil) }},
		{"wf_details", func(d *rm.InstructionDetails) {
			d.WfDetails = &rm.ItemTree{ArchetypeNodeID: "at0001", Name: &rm.DVText{Value: secret}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			comp := decodeWith(t, wt, proceduresBody(nil), simplified.WithTemplate(compiled))
			d := wantInstructionDetails(idUID, idActivity, new(idPath))
			tc.mutate(d)
			soleAction(t, wt, comp).InstructionDetails = d
			for format, encode := range map[string]func(*rm.Composition, *webtemplate.WebTemplate) ([]byte, error){
				"FLAT":       simplified.MarshalFlat,
				"STRUCTURED": simplified.MarshalStructured,
			} {
				_, err := encode(comp, wt)
				if !errors.Is(err, simplified.ErrUnsupportedDatatype) {
					t.Fatalf("Marshal %s err = %v, want ErrUnsupportedDatatype", format, err)
				}
				msg := err.Error()
				if !strings.Contains(msg, `"`+idFamily) {
					t.Errorf("Marshal %s err = %q, want it to name %q", format, msg, idFamily)
				}
				for _, banned := range []string{secret, "REQ-"} {
					if strings.Contains(msg, banned) {
						t.Errorf("Marshal %s err = %q carries %q", format, msg, banned)
					}
				}
			}
		})
	}
}
