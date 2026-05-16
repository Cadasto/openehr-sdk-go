package bmmdiff

import (
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// loadSchemaWithClass round-trips through bmm.Load to build a schema
// containing a single class with the requested properties. The JSON
// is composed inline.
func loadSchemaWithClass(t *testing.T, pub, name, release string, classes map[string]string) *bmm.Schema {
	t.Helper()
	var b strings.Builder
	b.WriteString(`{`)
	b.WriteString(`"bmm_version":"2.4",`)
	b.WriteString(`"rm_publisher":"` + pub + `",`)
	b.WriteString(`"schema_name":"` + name + `",`)
	b.WriteString(`"rm_release":"` + release + `",`)
	b.WriteString(`"schema_revision":"r1",`)
	b.WriteString(`"schema_lifecycle_state":"dstu",`)
	b.WriteString(`"class_definitions":{`)
	first := true
	for cName, cJSON := range classes {
		if !first {
			b.WriteString(",")
		}
		first = false
		b.WriteString(`"` + cName + `":` + cJSON)
	}
	b.WriteString(`}}`)
	s, err := bmm.Load(strings.NewReader(b.String()))
	if err != nil {
		t.Fatalf("loadSchemaWithClass: bmm.Load: %v", err)
	}
	return s
}

func TestDiff_NoChanges(t *testing.T) {
	classJSON := `{"name":"DV_QUANTITY","properties":{"magnitude":{"_type":"P_BMM_SINGLE_PROPERTY","name":"magnitude","type":"Real","is_mandatory":true}}}`
	oldS := loadSchemaWithClass(t, "openehr", "rm", "1.2.0", map[string]string{"DV_QUANTITY": classJSON})
	newS := loadSchemaWithClass(t, "openehr", "rm", "1.2.0", map[string]string{"DV_QUANTITY": classJSON})

	r := Diff(oldS, newS)
	if r.HasChanges() {
		t.Fatalf("expected no changes, got %+v", r)
	}
	out := Format(r)
	if !strings.Contains(out, "no semantic changes") {
		t.Errorf("Format(no-changes) missing sentinel: %s", out)
	}
}

func TestDiff_AddedProperty(t *testing.T) {
	oldJSON := `{"name":"DV_QUANTITY","properties":{"magnitude":{"_type":"P_BMM_SINGLE_PROPERTY","name":"magnitude","type":"Real","is_mandatory":true}}}`
	newJSON := `{"name":"DV_QUANTITY","properties":{"magnitude":{"_type":"P_BMM_SINGLE_PROPERTY","name":"magnitude","type":"Real","is_mandatory":true},"test_property":{"_type":"P_BMM_SINGLE_PROPERTY","name":"test_property","type":"String"}}}`
	oldS := loadSchemaWithClass(t, "openehr", "rm", "1.2.0", map[string]string{"DV_QUANTITY": oldJSON})
	newS := loadSchemaWithClass(t, "openehr", "rm", "1.2.1", map[string]string{"DV_QUANTITY": newJSON})

	r := Diff(oldS, newS)
	if !r.HasChanges() {
		t.Fatalf("expected changes, got none")
	}
	if len(r.ChangedClasses) != 1 || r.ChangedClasses[0].ClassName != "DV_QUANTITY" {
		t.Fatalf("expected one changed class DV_QUANTITY, got %+v", r.ChangedClasses)
	}
	added := r.ChangedClasses[0].AddedProperties
	if len(added) != 1 || added[0].Name != "test_property" {
		t.Errorf("expected added property test_property, got %+v", added)
	}
	if added[0].TypeName != "String" {
		t.Errorf("expected type String, got %s", added[0].TypeName)
	}
	// CHANGELOG suggestion contains the bump direction.
	suggestion := SuggestChangelogEntry(r)
	if !strings.Contains(suggestion, "1.2.0 -> 1.2.1") {
		t.Errorf("changelog suggestion missing version bump: %s", suggestion)
	}
	if !strings.Contains(suggestion, "test_property") {
		t.Errorf("changelog suggestion missing property name: %s", suggestion)
	}
	if !strings.Contains(suggestion, "[bmm-bump]") {
		t.Errorf("changelog suggestion missing [bmm-bump] marker: %s", suggestion)
	}
}

func TestDiff_RemovedProperty(t *testing.T) {
	oldJSON := `{"name":"X","properties":{"a":{"_type":"P_BMM_SINGLE_PROPERTY","name":"a","type":"String"},"b":{"_type":"P_BMM_SINGLE_PROPERTY","name":"b","type":"Integer"}}}`
	newJSON := `{"name":"X","properties":{"a":{"_type":"P_BMM_SINGLE_PROPERTY","name":"a","type":"String"}}}`
	oldS := loadSchemaWithClass(t, "openehr", "rm", "1.2.0", map[string]string{"X": oldJSON})
	newS := loadSchemaWithClass(t, "openehr", "rm", "1.2.1", map[string]string{"X": newJSON})

	r := Diff(oldS, newS)
	if len(r.ChangedClasses) != 1 {
		t.Fatalf("expected one changed class, got %d", len(r.ChangedClasses))
	}
	rem := r.ChangedClasses[0].RemovedProperties
	if len(rem) != 1 || rem[0] != "b" {
		t.Errorf("expected removed [b], got %+v", rem)
	}
}

func TestDiff_AddedClass(t *testing.T) {
	oldS := loadSchemaWithClass(t, "openehr", "rm", "1.2.0", map[string]string{
		"A": `{"name":"A"}`,
	})
	newS := loadSchemaWithClass(t, "openehr", "rm", "1.2.1", map[string]string{
		"A":         `{"name":"A"}`,
		"NEW_CLASS": `{"name":"NEW_CLASS"}`,
	})
	r := Diff(oldS, newS)
	if len(r.AddedClasses) != 1 || r.AddedClasses[0].ClassName != "NEW_CLASS" {
		t.Errorf("expected added [NEW_CLASS], got %+v", r.AddedClasses)
	}
	if len(r.RemovedClasses) != 0 {
		t.Errorf("expected no removed, got %+v", r.RemovedClasses)
	}
}

func TestDiff_RemovedClass(t *testing.T) {
	oldS := loadSchemaWithClass(t, "openehr", "rm", "1.2.0", map[string]string{
		"A":    `{"name":"A"}`,
		"GONE": `{"name":"GONE"}`,
	})
	newS := loadSchemaWithClass(t, "openehr", "rm", "1.2.1", map[string]string{
		"A": `{"name":"A"}`,
	})
	r := Diff(oldS, newS)
	if len(r.RemovedClasses) != 1 || r.RemovedClasses[0].ClassName != "GONE" {
		t.Errorf("expected removed [GONE], got %+v", r.RemovedClasses)
	}
}

func TestDiff_PropertyTypeChange(t *testing.T) {
	oldJSON := `{"name":"X","properties":{"a":{"_type":"P_BMM_SINGLE_PROPERTY","name":"a","type":"Integer"}}}`
	newJSON := `{"name":"X","properties":{"a":{"_type":"P_BMM_SINGLE_PROPERTY","name":"a","type":"Real","is_mandatory":true}}}`
	oldS := loadSchemaWithClass(t, "openehr", "rm", "1.2.0", map[string]string{"X": oldJSON})
	newS := loadSchemaWithClass(t, "openehr", "rm", "1.2.1", map[string]string{"X": newJSON})
	r := Diff(oldS, newS)
	if len(r.ChangedClasses) != 1 {
		t.Fatalf("expected 1 changed class, got %d", len(r.ChangedClasses))
	}
	changed := r.ChangedClasses[0].ChangedProperties
	if len(changed) != 1 {
		t.Fatalf("expected 1 changed property, got %d", len(changed))
	}
	pc := changed[0]
	if !pc.TypeDiff || pc.OldType != "Integer" || pc.NewType != "Real" {
		t.Errorf("expected type Integer->Real, got %+v", pc)
	}
	if !pc.MandatoryDiff || pc.OldMandatory != false || pc.NewMandatory != true {
		t.Errorf("expected mandatory false->true, got %+v", pc)
	}
}

func TestDiff_CardinalityChange(t *testing.T) {
	oldJSON := `{"name":"X","properties":{"items":{"_type":"P_BMM_CONTAINER_PROPERTY","name":"items","type_def":{"container_type":"List","type":"String"},"cardinality":{"lower":0,"upper_unbounded":true}}}}`
	newJSON := `{"name":"X","properties":{"items":{"_type":"P_BMM_CONTAINER_PROPERTY","name":"items","type_def":{"container_type":"List","type":"String"},"cardinality":{"lower":1,"upper":5}}}}`
	oldS := loadSchemaWithClass(t, "openehr", "rm", "1.2.0", map[string]string{"X": oldJSON})
	newS := loadSchemaWithClass(t, "openehr", "rm", "1.2.1", map[string]string{"X": newJSON})
	r := Diff(oldS, newS)
	if len(r.ChangedClasses) != 1 {
		t.Fatalf("expected 1 changed class, got %d", len(r.ChangedClasses))
	}
	cc := r.ChangedClasses[0].CardinalityChanges
	if len(cc) != 1 {
		t.Fatalf("expected 1 cardinality change, got %d", len(cc))
	}
	if cc[0].OldLower != 0 || cc[0].OldUpper != "*" {
		t.Errorf("expected old {0,*}, got %+v", cc[0])
	}
	if cc[0].NewLower != 1 || cc[0].NewUpper != "5" {
		t.Errorf("expected new {1,5}, got %+v", cc[0])
	}
}

func TestDiff_AddedRemovedPrimitives(t *testing.T) {
	// Build primitive entries via Load so the bmm.Class type is correctly
	// populated.
	jsonBody := `{"schema_name":"rm","rm_publisher":"openehr","rm_release":"1.2.0",` +
		`"primitive_types":{"Boolean":{"name":"Boolean"},"Integer":{"name":"Integer"}}}`
	old, err := bmm.Load(strings.NewReader(jsonBody))
	if err != nil {
		t.Fatalf("Load old: %v", err)
	}
	newJSONBody := `{"schema_name":"rm","rm_publisher":"openehr","rm_release":"1.2.1",` +
		`"primitive_types":{"Boolean":{"name":"Boolean"},"NEW_PRIM":{"name":"NEW_PRIM"}}}`
	newer, err := bmm.Load(strings.NewReader(newJSONBody))
	if err != nil {
		t.Fatalf("Load new: %v", err)
	}
	r := Diff(old, newer)
	if len(r.AddedPrimitives) != 1 || r.AddedPrimitives[0] != "NEW_PRIM" {
		t.Errorf("expected added [NEW_PRIM], got %+v", r.AddedPrimitives)
	}
	if len(r.RemovedPrimitives) != 1 || r.RemovedPrimitives[0] != "Integer" {
		t.Errorf("expected removed [Integer], got %+v", r.RemovedPrimitives)
	}
}

func TestDiff_AncestorChange(t *testing.T) {
	oldJSON := `{"name":"X","ancestors":["A"]}`
	newJSON := `{"name":"X","ancestors":["A","B"]}`
	oldS := loadSchemaWithClass(t, "openehr", "rm", "1.2.0", map[string]string{"X": oldJSON})
	newS := loadSchemaWithClass(t, "openehr", "rm", "1.2.1", map[string]string{"X": newJSON})
	r := Diff(oldS, newS)
	if len(r.ChangedClasses) != 1 {
		t.Fatalf("expected 1 changed class, got %d", len(r.ChangedClasses))
	}
	cc := r.ChangedClasses[0]
	if !cc.AncestorsDiffer {
		t.Errorf("expected AncestorsDiffer, got %+v", cc)
	}
}

func TestDiff_AddedAndRemovedFunctions(t *testing.T) {
	oldJSON := `{"name":"X","functions":{"old_fn":{"name":"old_fn"}}}`
	newJSON := `{"name":"X","functions":{"new_fn":{"name":"new_fn"}}}`
	oldS := loadSchemaWithClass(t, "openehr", "rm", "1.2.0", map[string]string{"X": oldJSON})
	newS := loadSchemaWithClass(t, "openehr", "rm", "1.2.1", map[string]string{"X": newJSON})
	r := Diff(oldS, newS)
	if len(r.ChangedClasses) != 1 {
		t.Fatalf("expected 1 changed class, got %d", len(r.ChangedClasses))
	}
	cc := r.ChangedClasses[0]
	if len(cc.AddedFunctions) != 1 || cc.AddedFunctions[0] != "new_fn" {
		t.Errorf("expected added functions [new_fn], got %+v", cc.AddedFunctions)
	}
	if len(cc.RemovedFunctions) != 1 || cc.RemovedFunctions[0] != "old_fn" {
		t.Errorf("expected removed functions [old_fn], got %+v", cc.RemovedFunctions)
	}
}

func TestDiff_NilSchemasReturnEmptyReport(t *testing.T) {
	r := Diff(nil, nil)
	if r == nil {
		t.Fatalf("Diff(nil,nil) returned nil")
	}
	if r.HasChanges() {
		t.Errorf("nil report should report no changes")
	}
}

func TestSuggestChangelogEntry_Empty(t *testing.T) {
	r := &Report{}
	got := SuggestChangelogEntry(r)
	if got != "" {
		t.Errorf("expected empty suggestion for empty report, got %q", got)
	}
}

func TestParseSchemaID(t *testing.T) {
	cases := []struct {
		in          string
		pub, n, rel string
	}{
		{"openehr_rm_1.2.0", "openehr", "rm", "1.2.0"},
		{"openehr_am_1.4.0", "openehr", "am", "1.4.0"},
		{"openehr_base_1.3.0", "openehr", "base", "1.3.0"},
		{"foo_bar_2.0", "foo", "bar", "2.0"},
		{"single", "", "single", ""},
		{"", "", "", ""},
	}
	for _, c := range cases {
		p, n, r := parseSchemaID(c.in)
		if p != c.pub || n != c.n || r != c.rel {
			t.Errorf("parseSchemaID(%q) = (%q,%q,%q), want (%q,%q,%q)",
				c.in, p, n, r, c.pub, c.n, c.rel)
		}
	}
}

func TestFormat_FullReport(t *testing.T) {
	r := &Report{
		OldSchemaID: "openehr_rm_1.2.0",
		NewSchemaID: "openehr_rm_1.2.1",
		AddedClasses: []ClassRef{
			{ClassName: "NEW_CLASS", Package: "org.openehr.rm.x"},
		},
		RemovedClasses: []ClassRef{
			{ClassName: "OLD_CLASS", Package: "org.openehr.rm.y"},
		},
		ChangedClasses: []ClassChange{
			{
				ClassName: "DV_QUANTITY",
				AddedProperties: []PropertyRef{
					{Name: "test_property", TypeName: "String"},
				},
			},
		},
		AddedPrimitives:   []string{"NEW_PRIM"},
		RemovedPrimitives: []string{"OLD_PRIM"},
	}
	out := Format(r)
	checks := []string{
		"openehr_rm_1.2.0 -> openehr_rm_1.2.1",
		"Added classes:",
		"NEW_CLASS",
		"Removed classes:",
		"OLD_CLASS",
		"Changed classes:",
		"DV_QUANTITY",
		"test_property: String",
		"Primitives:",
		"added:   [NEW_PRIM]",
		"removed: [OLD_PRIM]",
	}
	for _, s := range checks {
		if !strings.Contains(out, s) {
			t.Errorf("Format output missing %q\n--- got ---\n%s", s, out)
		}
	}
}
