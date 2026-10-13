package validation_test

// rmfloor_generated_access_test.go: REQ-112 and REQ-171 — the floor reads
// every registered class through the generated attribute access (REQ-171
// Structural access): an RM-mandatory list that is nil is absent and one that
// is set and empty is present and empty (REQ-112 Driver), the entries of a
// Hash attribute are walked in ascending key order and written by their
// position (REQ-102 Issue paths), a list of a primitive type is counted and
// never read item by item, by the floor and by the template walk, and the
// parts of a reference are reported once.

import (
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// TestValidateRM_REQ112_MandatoryListAbsentOrEmpty pins REQ-112 Driver on the
// floor: a multi-valued RM-mandatory attribute that is absent (a nil list)
// draws `required`, rule mandatory, and one that is present and empty draws
// `cardinality`, rule cardinality, both on the RM layer and at the
// attribute's path. CLUSTER items is RM-mandatory.
func TestValidateRM_REQ112_MandatoryListAbsentOrEmpty(t *testing.T) {
	tests := []struct {
		name  string
		items []rm.Item
		want  []string // code path layer rule
	}{
		{name: "nil items", items: nil, want: []string{"required /items rm mandatory"}},
		{name: "present and empty items", items: []rm.Item{}, want: []string{"cardinality /items rm cardinality"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := &rm.Cluster{ArchetypeNodeID: "at0001", Name: &rm.DVText{Value: "cluster"}, Items: tc.items}
			if got := identityKeys(validation.ValidateRM(root).Issues); !slices.Equal(got, tc.want) {
				t.Errorf("ValidateRM(CLUSTER with %s) issues (code path layer rule) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

// describedResource is a RESOURCE_DESCRIPTION whose details holds one item
// per key, every item complete but the one at key broken, which has no
// purpose.
func describedResource(keys []string, broken string) *rm.ResourceDescription {
	details := make(map[string]rm.ResourceDescriptionItem, len(keys))
	for _, k := range keys {
		item := rm.ResourceDescriptionItem{
			Language: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "ISO_639-1"}, CodeString: "en"},
			Purpose:  "purpose",
		}
		if k == broken {
			item.Purpose = ""
		}
		details[k] = item
	}
	return &rm.ResourceDescription{
		LifecycleState: "published",
		OriginalAuthor: map[string]string{"name": "author"},
		Details:        details,
	}
}

// TestValidateRM_REQ171_HashEntriesWalkedInKeyOrder pins REQ-171 Structural
// access and REQ-102 Issue paths on a Hash attribute: the floor enters the
// entries of RESOURCE_DESCRIPTION details in ascending key order and writes
// each by its 1-based position there, never by its key. Of five items, the
// fourth by key order lacks its RM-mandatory purpose, so `required` is
// reported at /details[4]/purpose whatever order the map holds them in, and
// no issue path carries a key. The walk is run several times because Go
// iterates a map in a random order. ValidateRM and Validate(root, nil) both
// report it.
func TestValidateRM_REQ171_HashEntriesWalkedInKeyOrder(t *testing.T) {
	keys := []string{"key-echo", "key-alpha", "key-delta", "key-charlie", "key-bravo"}
	// In key order: alpha, bravo, charlie, delta, echo; delta is the fourth.
	const want = "required /details[4]/purpose rm mandatory"
	for range 10 {
		root := describedResource(keys, "key-delta")
		for name, r := range map[string]validation.Result{
			"ValidateRM":          validation.ValidateRM(root),
			"Validate(root, nil)": validation.New().Validate(root, nil),
		} {
			got := identityKeys(r.Issues)
			if !slices.Contains(got, want) {
				t.Fatalf("%s(RESOURCE_DESCRIPTION, delta lacks purpose) issues = %q, want %q", name, got, want)
			}
			for _, issue := range r.Issues {
				for _, k := range keys {
					if strings.Contains(issue.Path, k) {
						t.Errorf("%s issue path %q carries the hash key %q", name, issue.Path, k)
					}
				}
				if strings.HasPrefix(issue.Path, "/details[") && issue.Path != "/details[4]/purpose" {
					t.Errorf("%s reports %s at %s, want only the fourth entry's purpose", name, issue.Code, issue.Path)
				}
			}
		}
	}
}

// TestValidateRM_REQ171_PrimitiveListNotWalked pins that the floor does not
// read the items of a list of a primitive type: DV_MULTIMEDIA data and
// integrity_check are Arrays of Octet, one item per byte, with nothing below
// them. A payload larger than the default visit limit (100,000) validates
// with no `max_nodes` and nothing in Coverage.Stopped, and the validation
// allocates less than the payload's size: a walk that boxed each byte as an
// item would allocate many times that.
func TestValidateRM_REQ171_PrimitiveListNotWalked(t *testing.T) {
	const size = 1 << 20
	root := &rm.DVMultimedia{
		MediaType:      rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "IANA_media-types"}, CodeString: "image/png"},
		Size:           size,
		Data:           make([]byte, size),
		IntegrityCheck: make([]byte, size),
	}
	var r validation.Result
	allocated := allocatedBytes(func() { r = validation.ValidateRM(root) })
	if containsCode(r.Issues, "max_nodes") || len(r.Coverage.Stopped) != 0 {
		t.Errorf("ValidateRM(DV_MULTIMEDIA with %d bytes of data) issues = %q, Coverage.Stopped = %q; want no max_nodes and nothing stopped", size, issueKeys(r.Issues), r.Coverage.Stopped)
	}
	if allocated >= size {
		t.Errorf("ValidateRM(DV_MULTIMEDIA with %d bytes of data and of integrity_check) allocated %d bytes, want fewer than %d: the bytes are read one item at a time", size, allocated, size)
	}
}

// allocatedBytes returns the bytes the heap allocated while f ran. A test
// that calls it does not run in parallel, so no other test's allocations are
// counted.
func allocatedBytes(f func()) uint64 {
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	f()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

// multimediaOPT constrains a DV_MULTIMEDIA's data, an Array of Octet, to
// exist.
const multimediaOPT = `<?xml version="1.0" encoding="utf-8"?>
<template xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns="http://schemas.openehr.org/v1">
<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
<template_id><value>multimedia</value></template_id><concept>multimedia</concept>
<definition><rm_type_name>DV_MULTIMEDIA</rm_type_name><node_id>at0000</node_id>
<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>data</rm_attribute_name>
<existence><lower_included>true</lower_included><upper_included>true</upper_included><lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>1</lower><upper>1</upper></existence>
<cardinality><is_ordered>true</is_ordered><is_unique>false</is_unique><interval><lower_included>true</lower_included><lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded><lower>0</lower></interval></cardinality>
</attributes>
</definition>
</template>`

// TestValidate_REQ171_PrimitiveListCountedNotRead pins, in the template walk,
// that a list of a primitive type the template constrains is counted and
// never read item by item: DV_MULTIMEDIA data that the template requires is
// reported `required` when it is absent and when it is present and empty,
// and a present payload of a mebibyte draws no template issue while the
// validation allocates less than the payload's size.
func TestValidate_REQ171_PrimitiveListCountedNotRead(t *testing.T) {
	c := mustCompileInline(t, multimediaOPT)
	media := func(data []byte) *rm.DVMultimedia {
		return &rm.DVMultimedia{
			MediaType: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "IANA_media-types"}, CodeString: "image/png"},
			Size:      rm.Integer(len(data)),
			Data:      data,
		}
	}
	for _, tc := range []struct {
		name string
		data []byte
		want []string
	}{
		{name: "absent data", data: nil, want: []string{"required /data template existence"}},
		{name: "present and empty data", data: []byte{}, want: []string{"required /data template existence"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := templateIssues(validation.Validate(media(tc.data), c)); !slices.Equal(got, tc.want) {
				t.Errorf("Validate(DV_MULTIMEDIA with %s) template issues = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
	const size = 1 << 20
	root := media(make([]byte, size))
	var r validation.Result
	allocated := allocatedBytes(func() { r = validation.Validate(root, c) })
	if got := templateIssues(r); len(got) != 0 {
		t.Errorf("Validate(DV_MULTIMEDIA with %d bytes of data) template issues = %q, want none", size, got)
	}
	if allocated >= size {
		t.Errorf("Validate(DV_MULTIMEDIA with %d bytes of data) allocated %d bytes, want fewer than %d: the bytes are read one item at a time", size, allocated, size)
	}
}

// refFolder returns a FOLDER whose items hold the one reference item.
func refFolder(item rm.ObjectRefLike) *rm.Folder {
	return &rm.Folder{
		ArchetypeNodeID:  "openEHR-EHR-FOLDER.generic.v1",
		Name:             rm.DVText{Value: "folder"},
		ArchetypeDetails: &rm.Archetyped{ArchetypeID: rm.ArchetypeID{Value: "openEHR-EHR-FOLDER.generic.v1"}, RMVersion: "1.1.0"},
		Items:            []rm.ObjectRefLike{item},
	}
}

// TestREQ112_FloorReportsAMissingReferencePartOfEveryReferenceOnce extends
// TestREQ112_FloorReportsAMissingReferencePartOnce to every OBJECT_REF
// subtype the generated access reads: an OBJECT_REF, a LOCATABLE_REF and an
// ACCESS_GROUP_REF, each empty, as the root, as a member of a FOLDER's items,
// and, for the PARTY_REF a PARTY_SELF holds by pointer, below a proxy. The
// walk goes below a reference, but each missing id, namespace or type is
// reported exactly once, as rm_invariant, at or below its path.
func TestREQ112_FloorReportsAMissingReferencePartOfEveryReferenceOnce(t *testing.T) {
	cases := []struct {
		name string
		root any
		at   string // the reference's path
	}{
		{name: "OBJECT_REF root", root: &rm.ObjectRef{}, at: ""},
		{name: "LOCATABLE_REF root", root: &rm.LocatableRef{}, at: ""},
		{name: "ACCESS_GROUP_REF root", root: &rm.AccessGroupRef{}, at: ""},
		{name: "OBJECT_REF in FOLDER.items", root: refFolder(rm.ObjectRef{}), at: "/items[1]"},
		{name: "LOCATABLE_REF in FOLDER.items", root: refFolder(rm.LocatableRef{}), at: "/items[1]"},
		{name: "ACCESS_GROUP_REF in FOLDER.items", root: refFolder(rm.AccessGroupRef{}), at: "/items[1]"},
		{name: "PARTY_REF a PARTY_SELF points to", root: &rm.PartySelf{ExternalRef: &rm.PartyRef{}}, at: "/external_ref"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := validation.ValidateRM(tc.root)
			for _, part := range []string{"id", "namespace", "type"} {
				path := tc.at + "/" + part
				var at []validation.Issue
				for _, issue := range r.Issues {
					if issue.Path == path || strings.HasPrefix(issue.Path, path+"/") {
						at = append(at, issue)
					}
				}
				if len(at) != 1 || at[0].Code != "rm_invariant" || at[0].Rule != "mandatory" {
					t.Errorf("ValidateRM(%s) issues at %s = %+v, want exactly one rm_invariant, rule mandatory", tc.name, path, at)
				}
			}
		})
	}
}

// TestREQ112_FloorReportsALocatableRefsOwnMissingID pins the OBJECT_REF
// entry of REQ-112 on a LOCATABLE_REF whose own id, the UID_BASED_ID it
// redefines id as, is unset while the OBJECT_REF id it embeds is set: the id
// is missing, as the generated access reads it, and it is reported exactly
// once, as rm_invariant, rule mandatory, at the reference's id, as the root
// and as a member of a FOLDER's items. Its namespace and type are there, so
// nothing is reported at them.
func TestREQ112_FloorReportsALocatableRefsOwnMissingID(t *testing.T) {
	ref := rm.LocatableRef{ObjectRef: rm.ObjectRef{
		ID:        &rm.HierObjectID{Value: "6f2d7a10-3c4b-4e5f-8a9b-0c1d2e3f4a5b"},
		Namespace: "local",
		Type:      "COMPOSITION",
	}}
	for _, tc := range []struct {
		name string
		root any
		at   string // the reference's path
	}{
		{name: "LOCATABLE_REF root", root: &ref, at: ""},
		{name: "LOCATABLE_REF in FOLDER.items", root: refFolder(ref), at: "/items[1]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for _, issue := range validation.ValidateRM(tc.root).Issues {
				if issue.Path == tc.at || strings.HasPrefix(issue.Path, tc.at+"/") {
					got = append(got, issue.Code+" "+issue.Path+" "+issue.Rule)
				}
			}
			if want := []string{"rm_invariant " + tc.at + "/id mandatory"}; !slices.Equal(got, want) {
				t.Errorf("ValidateRM(%s with only the embedded OBJECT_REF id) issues at or below %q = %q, want %q", tc.name, tc.at, got, want)
			}
		})
	}
}

// TestValidateRM_REQ112_TimeValidityWalked pins that the floor walks the
// time_validity interval of a ROLE and of a CAPABILITY, which the generated
// access reads: a DV_DATE bound that is no ISO 8601 date is reported as
// rm_invariant at the bound's path, and a side marked both unbounded and
// included as upper_included_valid at the interval's.
func TestValidateRM_REQ112_TimeValidityWalked(t *testing.T) {
	validity := func() *rm.DVInterval[rm.DVDate] {
		return &rm.DVInterval[rm.DVDate]{
			Lower:          rm.DVDate{Value: "not-a-date"},
			LowerIncluded:  true,
			UpperUnbounded: true,
			UpperIncluded:  true,
		}
	}
	role := &rm.Role{
		ArchetypeNodeID: "openEHR-DEMOGRAPHIC-ROLE.role.v1",
		Name:            rm.DVText{Value: "GP"},
		TimeValidity:    validity(),
		Capabilities: []rm.Capability{{
			ArchetypeNodeID: "at0001",
			Name:            rm.DVText{Value: "licence"},
			Credentials:     &rm.ItemTree{ArchetypeNodeID: "at0002", Name: rm.DVText{Value: "credentials"}},
			TimeValidity:    validity(),
		}},
	}
	got := identityKeys(validation.ValidateRM(role).Issues)
	for _, want := range []string{
		"rm_invariant /time_validity/lower rm DV_DATE.Value_valid",
		"upper_included_valid /time_validity rm Interval.Upper_included_valid",
		"rm_invariant /capabilities[at0001]/time_validity/lower rm DV_DATE.Value_valid",
		"upper_included_valid /capabilities[at0001]/time_validity rm Interval.Upper_included_valid",
	} {
		if !slices.Contains(got, want) {
			t.Errorf("ValidateRM(ROLE with faulty time_validity) issues = %q, want one %q", got, want)
		}
	}
}

// hashTemplateOPT constrains a RESOURCE_DESCRIPTION's details, a Hash
// attribute, by one RESOURCE_DESCRIPTION_ITEM child that requires `use`.
const hashTemplateOPT = `<?xml version="1.0" encoding="utf-8"?>
<template xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns="http://schemas.openehr.org/v1">
<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
<template_id><value>hash</value></template_id><concept>hash</concept>
<definition><rm_type_name>RESOURCE_DESCRIPTION</rm_type_name><node_id>at0000</node_id>
<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>details</rm_attribute_name>
<existence><lower_included>true</lower_included><upper_included>true</upper_included><lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>1</lower><upper>1</upper></existence>
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>RESOURCE_DESCRIPTION_ITEM</rm_type_name><node_id></node_id>
<occurrences><lower_included>true</lower_included><upper_included>true</upper_included><lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded><lower>0</lower></occurrences>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>use</rm_attribute_name>
<existence><lower_included>true</lower_included><upper_included>true</upper_included><lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>1</lower><upper>1</upper></existence>
</attributes>
</children>
<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval><lower_included>true</lower_included><lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded><lower>0</lower></interval></cardinality>
</attributes>
</definition>
</template>`

// TestValidate_REQ102_HashEntriesBoundByPosition pins REQ-102 Issue paths and
// REQ-171 Structural access in the template walk: a template that constrains
// a Hash attribute binds each entry, in ascending key order, and writes it by
// its 1-based position there, never by its key. Of three items, the second
// by key order lacks the `use` the template requires, so `required` is
// reported at /details[2]/use on the template layer, whatever order the map
// holds them in, and no issue path carries a key.
func TestValidate_REQ102_HashEntriesBoundByPosition(t *testing.T) {
	c := mustCompileInline(t, hashTemplateOPT)
	use := "use"
	const want = "required /details[2]/use template existence"
	for range 10 {
		root := describedResource([]string{"key-charlie", "key-alpha", "key-bravo"}, "")
		for _, k := range []string{"key-alpha", "key-charlie"} {
			item := root.Details[k]
			item.Use = &use
			root.Details[k] = item
		}
		r := validation.Validate(root, c)
		got := templateIssues(r)
		if !slices.Equal(got, []string{want}) {
			t.Fatalf("Validate(RESOURCE_DESCRIPTION, bravo lacks use) template issues = %q, want [%q]", got, want)
		}
		for _, issue := range r.Issues {
			if strings.Contains(issue.Path, "key-") {
				t.Errorf("Validate issue path %q carries a hash key", issue.Path)
			}
		}
	}
}
