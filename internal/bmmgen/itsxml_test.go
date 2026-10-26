package bmmgen

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// testITSXML is the vendored ITS-XML schema tree, relative to this package.
const testITSXML = "../../resources/its-xml"

// openTestITSXML opens the vendored ITS-XML tree as a root.
func openTestITSXML(t *testing.T) *os.Root {
	t.Helper()
	root, err := os.OpenRoot(testITSXML)
	if err != nil {
		t.Fatalf("open %s: %v", testITSXML, err)
	}
	t.Cleanup(func() { _ = root.Close() }) // read-only handle
	return root
}

// loadTestXSDSets reads the two vendored schema sets: RM and BASE
// Release-1.1.0, and AM Release-1.4.
func loadTestXSDSets(t *testing.T) (rmSet, amSet *xsdSet) {
	t.Helper()
	root := openTestITSXML(t)
	rmSet, err := loadXSDSet(root, itsXMLRMRoots)
	if err != nil {
		t.Fatalf("loadXSDSet(RM and BASE): %v", err)
	}
	amSet, err = loadXSDSet(root, itsXMLAMRoots)
	if err != nil {
		t.Fatalf("loadXSDSet(AM): %v", err)
	}
	return rmSet, amSet
}

// TestXSDSequenceOfVendoredTypes pins the reader on the vendored schemas: an
// extension's base elements come first, a choice's branches are read in
// document order, a group reference is expanded in place, a name repeated
// across choice branches keeps its first place, and the two sets read their
// own copy of a type both declare.
func TestXSDSequenceOfVendoredTypes(t *testing.T) {
	rmSet, amSet := loadTestXSDSets(t)
	locatable := []string{"name", "uid", "links", "archetype_details", "feeder_audit"}
	dvOrdered := []string{"normal_range", "other_reference_ranges", "normal_status"}
	cases := []struct {
		set      string
		typeName string
		want     []string
	}{
		{"rm", "ORIGINAL_VERSION", []string{
			"contribution", "commit_audit", "signature",
			"uid", "data", "preceding_version_uid", "other_input_version_uids", "attestations", "lifecycle_state",
		}},
		{"rm", "OBSERVATION", slices.Concat(locatable,
			[]string{"language", "encoding", "subject", "provider", "other_participations", "workflow_id"},
			[]string{"protocol", "guideline_id", "data", "state"})},
		{"rm", "ELEMENT", slices.Concat(locatable, []string{"value", "null_flavour", "null_reason"})},
		{"rm", "OBJECT_REF", []string{"id", "namespace", "type"}},
		{"rm", "Interval", []string{"lower_included", "upper_included", "lower_unbounded", "upper_unbounded"}},
		{"rm", "MultiplicityInterval", []string{"lower_included", "upper_included", "lower_unbounded", "upper_unbounded", "lower", "upper"}},
		{"rm", "RESOURCE_DESCRIPTION_ITEM", []string{
			"language", "purpose", "keywords", "use", "misuse", "original_resource_uri", "other_details",
		}},
		{"am", "RESOURCE_DESCRIPTION_ITEM", []string{
			"language", "purpose", "keywords", "use", "misuse", "copyright", "original_resource_uri", "other_details",
		}},
		{"am", "ORDINAL", slices.Concat(dvOrdered, []string{"value", "symbol"})},
		{"am", "SCALE", slices.Concat(dvOrdered, []string{"value", "symbol"})},
		{"am", "LOCATABLE", locatable},
	}
	for _, tc := range cases {
		t.Run(tc.set+"/"+tc.typeName, func(t *testing.T) {
			set := rmSet
			if tc.set == "am" {
				set = amSet
			}
			got, err := set.sequence(tc.typeName)
			if err != nil {
				t.Fatalf("sequence(%s): %v", tc.typeName, err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("sequence(%s) = %q\nwant %q", tc.typeName, got, tc.want)
			}
		})
	}
	if amSet.has("EHR") {
		t.Error("the AM set declares EHR; it should hold only the AM files and what they include")
	}
	if !rmSet.has("EHR") {
		t.Error("the RM set does not declare EHR")
	}
}

// writeSchemaTree writes files under a fresh directory and opens it as a
// root.
func writeSchemaTree(t *testing.T, files map[string]string) *os.Root {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", name, err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatalf("open %s: %v", dir, err)
	}
	t.Cleanup(func() { _ = root.Close() }) // read-only handle
	return root
}

// xsdDoc wraps body in an xs:schema element.
func xsdDoc(body string) string {
	return `<?xml version="1.0"?><xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">` + body + `</xs:schema>`
}

// TestLoadXSDSetRefuses pins each input the reader refuses rather than read
// into a wrong or partial order.
func TestLoadXSDSetRefuses(t *testing.T) {
	seqA := `<xs:complexType name="A"><xs:sequence><xs:element name="a"/><xs:element name="b"/></xs:sequence></xs:complexType>`
	seqAB := `<xs:complexType name="A"><xs:sequence><xs:element name="b"/><xs:element name="a"/></xs:sequence></xs:complexType>`
	cases := []struct {
		name    string
		files   map[string]string
		roots   []string
		wantErr string
	}{
		{
			name:    "a root pattern that matches nothing",
			files:   map[string]string{"s/a.xsd": xsdDoc(seqA)},
			roots:   []string{"s/missing*.xsd"},
			wantErr: "no schema matches",
		},
		{
			name: "a type two files declare with different sequences",
			files: map[string]string{
				"s/a.xsd": xsdDoc(`<xs:include schemaLocation="b.xsd"/>` + seqA),
				"s/b.xsd": xsdDoc(seqAB),
			},
			roots:   []string{"s/a.xsd"},
			wantErr: "type A has the sequence",
		},
		{
			name:    "an include outside the schema tree",
			files:   map[string]string{"s/a.xsd": xsdDoc(`<xs:include schemaLocation="../../outside.xsd"/>` + seqA)},
			roots:   []string{"s/a.xsd"},
			wantErr: "outside.xsd",
		},
		{
			name: "an unknown construct in a complex type",
			files: map[string]string{"s/a.xsd": xsdDoc(
				`<xs:complexType name="A"><xs:openContent/></xs:complexType>`)},
			roots:   []string{"s/a.xsd"},
			wantErr: "unexpected xs:openContent",
		},
		{
			name: "a redefinition",
			files: map[string]string{
				"s/a.xsd": xsdDoc(`<xs:redefine schemaLocation="b.xsd">` + seqA + `</xs:redefine>`),
				"s/b.xsd": xsdDoc(seqA),
			},
			roots:   []string{"s/a.xsd"},
			wantErr: "xs:redefine with redefinitions",
		},
		{
			name: "a file whose root is not xs:schema",
			files: map[string]string{
				"s/a.xsd": `<?xml version="1.0"?><schema>` + seqA + `</schema>`,
			},
			roots:   []string{"s/a.xsd"},
			wantErr: "want xs:schema",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := writeSchemaTree(t, tc.files)
			_, err := loadXSDSet(root, tc.roots)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("loadXSDSet = %v, want an error containing %q", err, tc.wantErr)
			}
		})
	}
}

// TestXSDSequenceRefuses pins each content model the sequence reader refuses:
// a base or group that is not declared, and a group that refers to itself.
func TestXSDSequenceRefuses(t *testing.T) {
	root := writeSchemaTree(t, map[string]string{"s/a.xsd": xsdDoc(`
		<xs:complexType name="MissingBase"><xs:complexContent><xs:extension base="NOPE"/></xs:complexContent></xs:complexType>
		<xs:complexType name="MissingGroup"><xs:sequence><xs:group ref="NOPE"/></xs:sequence></xs:complexType>
		<xs:group name="Loop"><xs:sequence><xs:group ref="Loop"/></xs:sequence></xs:group>
		<xs:complexType name="Looping"><xs:sequence><xs:group ref="Loop"/></xs:sequence></xs:complexType>
		<xs:complexType name="Nameless"><xs:sequence><xs:element/></xs:sequence></xs:complexType>`)})
	set, err := loadXSDSet(root, []string{"s/a.xsd"})
	if err != nil {
		t.Fatalf("loadXSDSet: %v", err)
	}
	for typeName, wantErr := range map[string]string{
		"MissingBase":  "base type NOPE is not declared",
		"MissingGroup": "group NOPE is not declared",
		"Looping":      "group Loop refers to itself",
		"Nameless":     "neither a name nor a ref",
		"Undeclared":   "no complex type Undeclared",
	} {
		if _, err := set.sequence(typeName); err == nil || !strings.Contains(err.Error(), wantErr) {
			t.Errorf("sequence(%s) = %v, want an error containing %q", typeName, err, wantErr)
		}
	}
}
