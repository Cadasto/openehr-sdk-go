package simplified

// REQ-140 — underscore-prefixed RM attributes: the decode router, the encode
// hook, and the families this phase carries (`_uid`, `_link:N`,
// `_work_flow_id`, `_guideline_id`, `context/_end_time`, `context/_location`).
//
// The fixtures are hand-authored FLAT maps against the vendored PROBE-086
// corpus template, so every path and suffix spelling here is the reference
// implementation's (ADR 0014) — the same keys the upstream corpus bodies carry.

import (
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// FLAT path fragments of the corpus template, so the tests read like the
// upstream bodies they mirror.
const (
	rmattrRoot    = "conformance-ehrbase.de.v0"
	rmattrContext = rmattrRoot + "/context"
	rmattrSection = rmattrRoot + "/conformance_section"
	rmattrObs     = rmattrSection + "/conformance_observation"
	rmattrEvent   = rmattrObs + "/any_event:0"
	rmattrElement = rmattrEvent + "/dv_quantity"
	rmattrAction  = rmattrSection + "/conformance_action"
	// The corpus template carries one collapsed ELEMENT per datatype under
	// `any_event:0`, which is how the value-decoration families reach every
	// DV_ORDERED anchor (REQ-140 Phase C1).
	rmattrCount      = rmattrEvent + "/dv_count"
	rmattrOrdinal    = rmattrEvent + "/dv_ordinal"
	rmattrProportion = rmattrEvent + "/dv_proportion"
	rmattrDateTime   = rmattrEvent + "/dv_date_time"
	rmattrText       = rmattrEvent + "/dv_text"
	rmattrCodedText  = rmattrEvent + "/dv_coded_text"
	rmattrBoolean    = rmattrEvent + "/dv_boolean"
)

// conformanceWT builds the PROBE-086 corpus Web Template and compiled
// template — the single OPT every corpus fixture instantiates.
func conformanceWT(t *testing.T) (*webtemplate.WebTemplate, *templatecompile.Compiled) {
	t.Helper()
	opt, err := template.ParseFile(fixtures.FlatConformanceOpt())
	if err != nil {
		t.Fatalf("parse corpus OPT: %v", err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("compile corpus OPT: %v", err)
	}
	wt, err := webtemplate.Build(c)
	if err != nil {
		t.Fatalf("build corpus Web Template: %v", err)
	}
	if wt.Tree.ID != rmattrRoot {
		t.Fatalf("corpus template root is %q, want %q — fixture changed?", wt.Tree.ID, rmattrRoot)
	}
	return wt, c
}

// rmattrBody is the minimal decodable FLAT scaffolding every case shares: the
// mandatory context plus one modelled leaf, so a case's own keys are the only
// thing under test.
func rmattrBody(extra map[string]any) map[string]any {
	body := map[string]any{
		"ctx/language":                      "en",
		"ctx/territory":                     "US",
		"ctx/time":                          "2021-12-21T14:19:31.649613+01:00",
		rmattrElement + "|magnitude":        65.9,
		rmattrElement + "|unit":             "unit",
		rmattrObs + "/any_event:0/time":     "2021-12-21T16:02:58.0094262+01:00",
		rmattrObs + "/language|code":        "en",
		rmattrObs + "/language|terminology": "ISO_639-1",
	}
	maps.Copy(body, extra)
	return body
}

// decodeRMAttr decodes a hand-authored FLAT body, failing the test on error.
func decodeRMAttr(t *testing.T, wt *webtemplate.WebTemplate, body map[string]any) *rm.Composition {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	comp, err := UnmarshalFlat(b, wt)
	if err != nil {
		t.Fatalf("UnmarshalFlat: %v", err)
	}
	return comp
}

// reencodeRMAttr re-encodes a decoded composition to a FLAT map.
func reencodeRMAttr(t *testing.T, wt *webtemplate.WebTemplate, comp *rm.Composition) map[string]any {
	t.Helper()
	b, err := MarshalFlat(comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// assertRMAttrRoundTrip decodes body, re-encodes it, and asserts every `_` key
// in want comes back with the same value — the byte-exact leg REQ-140 requires
// of each family (design constraint 2: encode and decode land together).
func assertRMAttrRoundTrip(t *testing.T, wt *webtemplate.WebTemplate, want map[string]any) *rm.Composition {
	t.Helper()
	comp := decodeRMAttr(t, wt, rmattrBody(want))
	got := reencodeRMAttr(t, wt, comp)
	for _, k := range slices.Sorted(maps.Keys(want)) {
		have, ok := got[k]
		if !ok {
			t.Errorf("re-encode dropped %q; emitted keys: %v", k, slices.Sorted(maps.Keys(got)))
			continue
		}
		if !sameCtxValue(want[k], have) {
			t.Errorf("re-encode of %q = %#v, want %#v", k, have, want[k])
		}
	}
	// Nothing extra under the underscore grammar: an emitted `_` key the body
	// did not carry is a fabricated attribute.
	for _, k := range slices.Sorted(maps.Keys(got)) {
		if !strings.Contains(k, "/_") {
			continue
		}
		if _, expected := want[k]; !expected {
			t.Errorf("re-encode invented underscore key %q = %#v", k, got[k])
		}
	}
	return comp
}

// decodeRMAttrErr decodes a body expected to fail and returns the error.
func decodeRMAttrErr(t *testing.T, wt *webtemplate.WebTemplate, body map[string]any) error {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	comp, err := UnmarshalFlat(b, wt)
	if err == nil {
		t.Fatalf("UnmarshalFlat succeeded, want a typed refusal (decoded %d content items)", len(comp.Content))
	}
	return err
}

// --- _uid ---------------------------------------------------------------

// TestRMAttrUIDRoundTrip — REQ-140. `_uid` rides a bare string on any
// LOCATABLE. The concrete UID_BASED_ID subtype comes from the lexical form:
// the corpus writes an OBJECT_VERSION_ID at the composition root and a bare
// UUID (HIER_OBJECT_ID) on the ENTRY, and both must survive the round-trip as
// the subtype they arrived as.
func TestRMAttrUIDRoundTrip(t *testing.T) {
	wt, _ := conformanceWT(t)
	comp := assertRMAttrRoundTrip(t, wt, map[string]any{
		rmattrRoot + "/_uid":    "6e3a9506-b81c-4d74-a37f-1464fb7106b2::ehrbase.org::1",
		rmattrSection + "/_uid": "9fcc1c70-9349-444d-b9cb-8fa817697f5e",
		rmattrObs + "/_uid":     "aaaa1c70-9349-444d-b9cb-8fa817697f5e",
	})

	ovid, ok := comp.UID.(*rm.ObjectVersionID)
	if !ok {
		t.Fatalf("root uid = %T, want *rm.ObjectVersionID (three-part lexical form)", comp.UID)
	}
	if ovid.Value != "6e3a9506-b81c-4d74-a37f-1464fb7106b2::ehrbase.org::1" {
		t.Errorf("root uid value = %q", ovid.Value)
	}
	sec, ok := comp.Content[0].(*rm.Section)
	if !ok {
		t.Fatalf("content[0] = %T, want *rm.Section", comp.Content[0])
	}
	hier, ok := sec.UID.(*rm.HierObjectID)
	if !ok {
		t.Fatalf("section uid = %T, want *rm.HierObjectID (bare UUID)", sec.UID)
	}
	if hier.Value != "9fcc1c70-9349-444d-b9cb-8fa817697f5e" {
		t.Errorf("section uid value = %q", hier.Value)
	}
}

// TestRMAttrUIDOnCollapsedElement — REQ-140. The Web Template folds
// ELEMENT.value into its leaf node, so `<leaf>/_uid` and `<leaf>/_link:N`
// belong to the ELEMENT one attribute up (the corpus spells them exactly this
// way on `…/any_event:0/dv_quantity`). Both directions must find that owner.
func TestRMAttrUIDOnCollapsedElement(t *testing.T) {
	wt, _ := conformanceWT(t)
	comp := assertRMAttrRoundTrip(t, wt, map[string]any{
		rmattrElement + "/_uid":            "9fcc1c70-9349-444d-b9cb-8fa817697f5e",
		rmattrElement + "/_link:0|meaning": "problem related note",
		rmattrElement + "/_link:0|type":    "problem",
		rmattrElement + "/_link:0|target":  "ehr://ehr.network/347a5490-55ee-4da9-b91a-9bba710f730e",
	})
	el := firstElement(t, comp)
	if el.UID == nil {
		t.Fatal("ELEMENT uid not decoded — the collapsed leaf's owner was not resolved")
	}
	if len(el.Links) != 1 {
		t.Fatalf("ELEMENT links = %d, want 1", len(el.Links))
	}
}

// firstElement digs out the OBSERVATION's dv_quantity ELEMENT.
func firstElement(t *testing.T, comp *rm.Composition) *rm.Element {
	t.Helper()
	sec, ok := comp.Content[0].(*rm.Section)
	if !ok {
		t.Fatalf("content[0] = %T, want *rm.Section", comp.Content[0])
	}
	for _, item := range sec.Items {
		obs, ok := item.(*rm.Observation)
		if !ok {
			continue
		}
		for _, ev := range obs.Data.Events {
			pe, ok := ev.(*rm.PointEvent[rm.ItemStructure])
			if !ok {
				continue
			}
			tree, ok := pe.Data.(*rm.ItemTree)
			if !ok {
				continue
			}
			for _, it := range tree.Items {
				if el, ok := it.(*rm.Element); ok {
					return el
				}
			}
		}
	}
	t.Fatal("no ELEMENT found under the observation")
	return nil
}

// TestRMAttrUIDSubtypeMismatchRefusedOnEncode — REQ-140. `_uid` is one bare
// string, so decode re-derives the concrete subtype from the lexical form. A
// HIER_OBJECT_ID whose value is spelled like an OBJECT_VERSION_ID would come
// back as the other subtype: encode refuses rather than retyping silently.
func TestRMAttrUIDSubtypeMismatchRefusedOnEncode(t *testing.T) {
	wt, _ := conformanceWT(t)
	comp := decodeRMAttr(t, wt, rmattrBody(nil))
	comp.UID = &rm.HierObjectID{Value: "6e3a9506-b81c-4d74-a37f-1464fb7106b2::ehrbase.org::1"}
	if _, err := MarshalFlat(comp, wt); !errors.Is(err, ErrUnsupportedDatatype) {
		t.Errorf("MarshalFlat with a mis-spelled HIER_OBJECT_ID err = %v, want ErrUnsupportedDatatype", err)
	}
}

// --- _link:N ------------------------------------------------------------

// TestRMAttrLinkRoundTrip — REQ-140. `_link:N` carries `|meaning`, `|type`
// and `|target`; the list round-trips in order.
func TestRMAttrLinkRoundTrip(t *testing.T) {
	wt, _ := conformanceWT(t)
	comp := assertRMAttrRoundTrip(t, wt, map[string]any{
		rmattrRoot + "/_link:0|meaning": "problem related note",
		rmattrRoot + "/_link:0|type":    "problem",
		rmattrRoot + "/_link:0|target":  "ehr://ehr.network/347a5490-55ee-4da9-b91a-9bba710f730e",
		rmattrRoot + "/_link:1|meaning": "follow-up to",
		rmattrRoot + "/_link:1|type":    "issue",
		rmattrRoot + "/_link:1|target":  "ehr://ehr.network/2",
	})
	if len(comp.Links) != 2 {
		t.Fatalf("links = %d, want 2", len(comp.Links))
	}
	if got := comp.Links[0].Meaning.GetValue(); got != "problem related note" {
		t.Errorf("links[0].meaning = %q", got)
	}
	if got := comp.Links[1].Target.Value; got != "ehr://ehr.network/2" {
		t.Errorf("links[1].target = %q", got)
	}
}

// TestRMAttrLinkDecoratedMeaningRefusedOnEncode — REQ-140. `|meaning` /
// `|type` are plain strings and LINK has no `|raw` carrier, so a coded or
// decorated DV_TEXT cannot be narrowed to its rubric silently.
func TestRMAttrLinkDecoratedMeaningRefusedOnEncode(t *testing.T) {
	wt, _ := conformanceWT(t)
	formatting := "markdown"
	for name, meaning := range map[string]rm.DVTextLike{
		"coded": &rm.DVCodedText{
			DVText:       rm.DVText{Value: "problem related note"},
			DefiningCode: rm.CodePhrase{CodeString: "1", TerminologyID: rm.TerminologyID{Value: "local"}},
		},
		"formatted": &rm.DVText{Value: "problem related note", Formatting: &formatting},
	} {
		t.Run(name, func(t *testing.T) {
			comp := decodeRMAttr(t, wt, rmattrBody(nil))
			comp.Links = []rm.Link{{
				Meaning: meaning,
				Type:    &rm.DVText{Value: "problem"},
				Target:  rm.DVEHRURI{DVURI: rm.DVURI{Value: "ehr://ehr.network/1"}},
			}}
			if _, err := MarshalFlat(comp, wt); !errors.Is(err, ErrUnsupportedDatatype) {
				t.Errorf("MarshalFlat err = %v, want ErrUnsupportedDatatype", err)
			}
		})
	}
}

// --- _work_flow_id / _guideline_id --------------------------------------

// TestRMAttrObjectRefRoundTrip — REQ-140. `_work_flow_id` and `_guideline_id`
// carry an OBJECT_REF as `|id`, `|id_scheme`, `|namespace`, `|type` — the
// corpus's spelling, verified against
// `ehrbase_conformance_{action,evaluation,instruction,observation}`. Note
// `|namespace`, not the party grammar's `|id_namespace`.
func TestRMAttrObjectRefRoundTrip(t *testing.T) {
	wt, _ := conformanceWT(t)
	comp := assertRMAttrRoundTrip(t, wt, map[string]any{
		rmattrObs + "/_work_flow_id|id":        "335645",
		rmattrObs + "/_work_flow_id|id_scheme": "HOSPITAL-NS",
		rmattrObs + "/_work_flow_id|namespace": "HOSPITAL-NS",
		rmattrObs + "/_work_flow_id|type":      "WORKFLOW",
		rmattrObs + "/_guideline_id|id":        "3445",
		rmattrObs + "/_guideline_id|id_scheme": "HOSPITAL-NS",
		rmattrObs + "/_guideline_id|namespace": "HOSPITAL-NS",
		rmattrObs + "/_guideline_id|type":      "GUIDELINE",
	})
	obs := firstObservation(t, comp)
	ref, ok := obs.WorkflowID.(*rm.ObjectRef)
	if !ok {
		t.Fatalf("workflow_id = %T, want *rm.ObjectRef", obs.WorkflowID)
	}
	// |id_scheme is the OBJECT_ID discriminator: with it the id is a GENERIC_ID
	// carrying that scheme.
	gid, ok := ref.ID.(*rm.GenericID)
	if !ok {
		t.Fatalf("workflow_id.id = %T, want *rm.GenericID (|id_scheme present)", ref.ID)
	}
	if gid.Value != "335645" || gid.Scheme != "HOSPITAL-NS" {
		t.Errorf("workflow_id.id = %+v", *gid)
	}
	if ref.Namespace != "HOSPITAL-NS" || ref.Type != "WORKFLOW" {
		t.Errorf("workflow_id namespace/type = %q/%q", ref.Namespace, ref.Type)
	}
}

// TestRMAttrObjectRefWithoutScheme — REQ-140. Without `|id_scheme` the
// OBJECT_ID is a HIER_OBJECT_ID: it is the scheme-less subtype the round-trip
// can name unambiguously, and encode emits no `|id_scheme` for it.
func TestRMAttrObjectRefWithoutScheme(t *testing.T) {
	wt, _ := conformanceWT(t)
	comp := assertRMAttrRoundTrip(t, wt, map[string]any{
		rmattrObs + "/_work_flow_id|id":        "335645",
		rmattrObs + "/_work_flow_id|namespace": "local",
		rmattrObs + "/_work_flow_id|type":      "WORKFLOW",
	})
	obs := firstObservation(t, comp)
	ref, ok := obs.WorkflowID.(*rm.ObjectRef)
	if !ok {
		t.Fatalf("workflow_id = %T, want *rm.ObjectRef", obs.WorkflowID)
	}
	if _, ok := ref.ID.(*rm.HierObjectID); !ok {
		t.Fatalf("workflow_id.id = %T, want *rm.HierObjectID (no |id_scheme)", ref.ID)
	}
}

// TestRMAttrGuidelineIDNotOnAdminEntry — REQ-140. The grammar is typed by the
// owning RM class, and `guideline_id` is declared on CARE_ENTRY: ADMIN_ENTRY is
// an ENTRY but not a CARE_ENTRY, so it carries `_work_flow_id` and no
// `_guideline_id`. The corpus agrees — `ehrbase_conformance_admin_entry` writes
// exactly that pair of presences.
func TestRMAttrGuidelineIDNotOnAdminEntry(t *testing.T) {
	wt, _ := conformanceWT(t)
	const adminEntry = rmattrSection + "/conformance_admin_entry"
	if err := decodeRMAttrErr(t, wt, rmattrBody(map[string]any{
		adminEntry + "/_guideline_id|id":        "3445",
		adminEntry + "/_guideline_id|namespace": "local",
		adminEntry + "/_guideline_id|type":      "GUIDELINE",
	})); !errors.Is(err, ErrUnknownPath) {
		t.Errorf("_guideline_id on an ADMIN_ENTRY err = %v, want ErrUnknownPath", err)
	}
	// …while `_work_flow_id` is admitted on the same owner.
	assertRMAttrRoundTrip(t, wt, map[string]any{
		adminEntry + "/_work_flow_id|id":        "335645",
		adminEntry + "/_work_flow_id|namespace": "local",
		adminEntry + "/_work_flow_id|type":      "WORKFLOW",
	})
}

// TestRMAttrObjectRefRefusedOnNonEntry — REQ-140. A SECTION, CLUSTER or
// collapsed ELEMENT declares neither attribute, so both families are
// ErrUnknownPath there.
func TestRMAttrObjectRefRefusedOnNonEntry(t *testing.T) {
	wt, _ := conformanceWT(t)
	for _, owner := range []string{rmattrRoot, rmattrSection, rmattrElement} {
		for _, family := range []string{"_work_flow_id", "_guideline_id"} {
			t.Run(owner+family, func(t *testing.T) {
				if err := decodeRMAttrErr(t, wt, rmattrBody(map[string]any{
					owner + "/" + family + "|id":        "1",
					owner + "/" + family + "|namespace": "local",
					owner + "/" + family + "|type":      "ANY",
				})); !errors.Is(err, ErrUnknownPath) {
					t.Errorf("err = %v, want ErrUnknownPath", err)
				}
			})
		}
	}
}

// TestRMAttrObjectRefUnexpressibleIDRefusedOnEncode — REQ-140. Only GENERIC_ID
// (with `|id_scheme`) and HIER_OBJECT_ID (without) are distinguishable in this
// suffix set; every other OBJECT_ID subtype is scheme-less and would come back
// as a HIER_OBJECT_ID, so encode refuses rather than retyping.
func TestRMAttrObjectRefUnexpressibleIDRefusedOnEncode(t *testing.T) {
	wt, _ := conformanceWT(t)
	comp := decodeRMAttr(t, wt, rmattrBody(nil))
	obs := firstObservation(t, comp)
	obs.WorkflowID = &rm.ObjectRef{
		ID:        rm.ObjectVersionID{Value: "6e3a9506-b81c-4d74-a37f-1464fb7106b2::ehrbase.org::1"},
		Namespace: "local", Type: "WORKFLOW",
	}
	if _, err := MarshalFlat(comp, wt); !errors.Is(err, ErrUnsupportedDatatype) {
		t.Errorf("MarshalFlat with an OBJECT_VERSION_ID reference err = %v, want ErrUnsupportedDatatype", err)
	}
}

// firstObservation digs out the corpus template's conformance_observation.
func firstObservation(t *testing.T, comp *rm.Composition) *rm.Observation {
	t.Helper()
	sec, ok := comp.Content[0].(*rm.Section)
	if !ok {
		t.Fatalf("content[0] = %T, want *rm.Section", comp.Content[0])
	}
	for _, item := range sec.Items {
		if obs, ok := item.(*rm.Observation); ok {
			return obs
		}
	}
	t.Fatal("no OBSERVATION found under the section")
	return nil
}

// --- context/_end_time, context/_location -------------------------------

// TestRMAttrEventContextRoundTrip — REQ-140 / ADR 0016. The EVENT_CONTEXT
// optionals ride the underscore grammar under the real `context` segment, not
// the ITS `ctx/` sketches. `_end_time` is a bare DV_DATE_TIME value,
// `_location` a bare String.
func TestRMAttrEventContextRoundTrip(t *testing.T) {
	wt, _ := conformanceWT(t)
	comp := assertRMAttrRoundTrip(t, wt, map[string]any{
		rmattrRoot + "/context/_end_time": "2021-12-21T15:19:31.649613+01:00",
		rmattrRoot + "/context/_location": "microbiology lab 2",
	})
	if comp.Context == nil {
		t.Fatal("no EVENT_CONTEXT decoded")
	}
	if comp.Context.EndTime == nil || comp.Context.EndTime.Value != "2021-12-21T15:19:31.649613+01:00" {
		t.Errorf("end_time = %+v", comp.Context.EndTime)
	}
	if comp.Context.Location == nil || *comp.Context.Location != "microbiology lab 2" {
		t.Errorf("location = %v", comp.Context.Location)
	}
}

// TestRMAttrEventContextWithoutTemplateNode — REQ-140 / ADR 0016. The
// EVENT_CONTEXT optionals are RM-optional attributes a template need not
// constrain, so `<root>/context/_*` must resolve its owner whether or not the
// Web Template carries a `context` node. The corpus template does; this pins
// the other half by stripping it.
func TestRMAttrEventContextWithoutTemplateNode(t *testing.T) {
	wt, _ := conformanceWT(t)
	stripped := *wt
	tree := *wt.Tree
	tree.Children = nil
	for _, ch := range wt.Tree.Children {
		if ch.ID == "context" {
			continue
		}
		tree.Children = append(tree.Children, ch)
	}
	if len(tree.Children) == len(wt.Tree.Children) {
		t.Fatal("corpus template has no `context` node any more — fixture changed?")
	}
	stripped.Tree = &tree

	body := rmattrBody(map[string]any{
		rmattrRoot + "/context/_end_time": "2021-12-21T15:19:31.649613+01:00",
		rmattrRoot + "/context/_location": "microbiology lab 2",
	})
	comp := decodeRMAttr(t, &stripped, body)
	if comp.Context == nil || comp.Context.Location == nil {
		t.Fatalf("EVENT_CONTEXT not rebuilt without a template `context` node: %+v", comp.Context)
	}
	got := reencodeRMAttr(t, &stripped, comp)
	for _, k := range []string{rmattrRoot + "/context/_end_time", rmattrRoot + "/context/_location"} {
		if got[k] != body[k] {
			t.Errorf("re-encode of %q = %#v, want %#v", k, got[k], body[k])
		}
	}
}

// TestRMAttrEndTimeDecoratedRefusedOnEncode — REQ-140. `_end_time` is a bare
// value with no `|raw` carrier anywhere in the family grammar, so a decorated
// DV_DATE_TIME is a typed error rather than a narrowed emit.
func TestRMAttrEndTimeDecoratedRefusedOnEncode(t *testing.T) {
	wt, _ := conformanceWT(t)
	comp := decodeRMAttr(t, wt, rmattrBody(nil))
	if comp.Context == nil {
		t.Fatal("fixture: no EVENT_CONTEXT")
	}
	status := "~"
	comp.Context.EndTime = &rm.DVDateTime{Value: "2021-12-21T15:19:31+01:00", MagnitudeStatus: &status}
	if _, err := MarshalFlat(comp, wt); !errors.Is(err, ErrUnsupportedDatatype) {
		t.Errorf("MarshalFlat with a decorated end_time err = %v, want ErrUnsupportedDatatype", err)
	}
}

// --- router: refusals ---------------------------------------------------

// TestRMAttrDeferredFamiliesRefused — REQ-140. Every family this phase does
// not carry must keep refusing loudly, so a later phase flips a test rather
// than a behaviour. The refusal names the family prefix, which is what the
// PROBE-086 census scopes its exclusions by.
func TestRMAttrDeferredFamiliesRefused(t *testing.T) {
	wt, _ := conformanceWT(t)
	for _, key := range []string{
		// `_identifier` is not a router family at all: every position that
		// reaches a party reaches its identifier list through the party grammar
		// (rmattr_party.go), so an ENTRY owner is a path that names no family.
		rmattrObs + "/_identifier:0|id",
		rmattrAction + "/_instruction_details|activity_id",
		rmattrSection + "/_wf_definition|value",
		// Not a family at all: a typo must not be mistaken for one.
		rmattrRoot + "/_nonsense",
	} {
		t.Run(key, func(t *testing.T) {
			err := decodeRMAttrErr(t, wt, rmattrBody(map[string]any{key: "x"}))
			if !errors.Is(err, ErrUnknownPath) {
				t.Fatalf("err = %v, want ErrUnknownPath", err)
			}
			// The message must name the family prefix so the census can scope
			// the exclusion to it (SKIPPED.md § How the exclusion list is
			// produced).
			prefix, _, _ := strings.Cut(key, "|")
			prefix, _, _ = strings.Cut(prefix, "/_")
			if !strings.Contains(err.Error(), prefix) {
				t.Errorf("err = %v, want it to name %q", err, prefix)
			}
		})
	}
}

// TestRMAttrUnknownSuffixRefused — REQ-140. A recognised family carrying an
// unrecognised suffix is ErrUnsupportedDatatype naming the offending FLAT key,
// not ErrUnknownPath: the path resolved, the grammar did not.
func TestRMAttrUnknownSuffixRefused(t *testing.T) {
	wt, _ := conformanceWT(t)
	key := rmattrRoot + "/_link:0|typo"
	err := decodeRMAttrErr(t, wt, rmattrBody(map[string]any{
		rmattrRoot + "/_link:0|meaning": "m",
		rmattrRoot + "/_link:0|type":    "t",
		rmattrRoot + "/_link:0|target":  "ehr://x",
		key:                             "boom",
	}))
	if !errors.Is(err, ErrUnsupportedDatatype) {
		t.Fatalf("err = %v, want ErrUnsupportedDatatype", err)
	}
	if !strings.Contains(err.Error(), key) {
		t.Errorf("err = %v, want it to name %q", err, key)
	}
}

// TestRMAttrMissingMandatorySuffixRefused — REQ-140. LINK's three attributes
// are RM-mandatory; a half-spelled link must not decode to a coerced empty
// string.
func TestRMAttrMissingMandatorySuffixRefused(t *testing.T) {
	wt, _ := conformanceWT(t)
	err := decodeRMAttrErr(t, wt, rmattrBody(map[string]any{
		rmattrRoot + "/_link:0|meaning": "m",
	}))
	if !errors.Is(err, ErrUnsupportedDatatype) {
		t.Fatalf("err = %v, want ErrUnsupportedDatatype", err)
	}
}

// TestRMAttrIndexRules — REQ-140. `_family:N` obeys the same `:index` rules
// every other FLAT segment does: canonical spelling, no sparse gaps, and the
// repeat bound. A scalar family admits `:0` — the OPT-free FLAT ↔ STRUCTURED
// interconversion normalises every segment to an explicit index — but nothing
// higher.
func TestRMAttrIndexRules(t *testing.T) {
	wt, _ := conformanceWT(t)
	link := func(idx string) map[string]any {
		p := rmattrRoot + "/_link" + idx
		return map[string]any{
			p + "|meaning": "m", p + "|type": "t", p + "|target": "ehr://x",
		}
	}
	for name, tc := range map[string]struct {
		extra map[string]any
		ok    bool
	}{
		"link:0 alone":            {link(":0"), true},
		"link with no index":      {link(""), true},
		"link:1 without link:0":   {link(":1"), false},
		"link:0 beside bare link": {mergeRMAttr(link(":0"), link("")), false},
		"link:-1":                 {link(":-1"), false},
		"link:00":                 {link(":00"), false},
		"link:10001":              {link(":10001"), false},
		"uid:0":                   {map[string]any{rmattrRoot + "/_uid:0": "abc"}, true},
		"uid:1":                   {map[string]any{rmattrRoot + "/_uid:1": "abc"}, false},
	} {
		t.Run(name, func(t *testing.T) {
			body := rmattrBody(tc.extra)
			if tc.ok {
				decodeRMAttr(t, wt, body)
				return
			}
			if err := decodeRMAttrErr(t, wt, body); !errors.Is(err, ErrUnknownPath) {
				t.Errorf("err = %v, want ErrUnknownPath", err)
			}
		})
	}
}

func mergeRMAttr(maps_ ...map[string]any) map[string]any {
	out := map[string]any{}
	for _, m := range maps_ {
		maps.Copy(out, m)
	}
	return out
}

// TestRMAttrOwnerKindRefused — REQ-140. The grammar is typed by the owning RM
// class: a family the owner's RM class does not declare is ErrUnknownPath, the
// same refusal an unknown family gets.
//
//   - ISM_TRANSITION and EVENT_CONTEXT are not LOCATABLEs, so neither has a
//     `_uid` or `_link`;
//   - a childless leaf whose canonical path does not end in `/value` hides no
//     collapsed ELEMENT — COMPOSITION `category`, an ENTRY `language`, an
//     ACTIVITY `timing` — so there is no LOCATABLE for the family to own.
func TestRMAttrOwnerKindRefused(t *testing.T) {
	wt, _ := conformanceWT(t)
	for _, key := range []string{
		rmattrAction + "/transition/_uid",
		rmattrRoot + "/context/_uid",
		rmattrRoot + "/context/_link:0|meaning",
		rmattrRoot + "/category/_uid",
		rmattrObs + "/language/_uid",
		rmattrSection + "/conformance_instruction/current_activity/timing/_uid",
	} {
		t.Run(key, func(t *testing.T) {
			if err := decodeRMAttrErr(t, wt, rmattrBody(map[string]any{key: "x"})); !errors.Is(err, ErrUnknownPath) {
				t.Errorf("err = %v, want ErrUnknownPath", err)
			}
		})
	}
}
