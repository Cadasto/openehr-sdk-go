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

// loadTestXSDSets reads the two vendored schema sets: RM Release-1.1.0
// with BASE Release-1.2.0, and AM Release-1.4.
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

// TestRunRefusesMissingITSXML pins that a run whose ITS-XML directory holds
// no schemas fails, rather than write the XML encoders in the generator's
// property order.
func TestRunRefusesMissingITSXML(t *testing.T) {
	_, err := Run(Options{
		ResourcesDir:  testResources,
		ITSXMLDir:     t.TempDir(),
		OutDir:        t.TempDir(),
		Targets:       []Target{TargetRM},
		SkipInventory: true,
	})
	if err == nil || !strings.Contains(err.Error(), "no schema matches") {
		t.Errorf("Run with an empty ITS-XML directory = %v, want an error naming the unmatched schema roots", err)
	}
}

// testAttributesXSD is the fixture schema tree whose types declare XML
// attributes in each place a schema can put them.
const testAttributesXSD = "testdata/itsxml_attributes"

// TestXSDAttributesOfFixtureTypes pins where the reader finds a type's XML
// attributes: on the type itself, in a complexContent extension after its
// base's, in a simpleContent extension of a built-in type, of a complex type
// with simple content and of a named simple type, and through the base chain.
// A type with simple content has no child element, and its sequence is empty.
//
// REQ-056: the generated codec writes and reads as an XML attribute what the
// class's ITS-XML schema type declares as one.
func TestXSDAttributesOfFixtureTypes(t *testing.T) {
	root, err := os.OpenRoot(testAttributesXSD)
	if err != nil {
		t.Fatalf("open %s: %v", testAttributesXSD, err)
	}
	t.Cleanup(func() { _ = root.Close() }) // read-only handle
	set, err := loadXSDSet(root, []string{"attributes.xsd"})
	if err != nil {
		t.Fatalf("loadXSDSet(%s): %v", testAttributesXSD, err)
	}
	cases := []struct {
		typeName string
		attrs    []string
		seq      []string
	}{
		{typeName: "OWN", attrs: []string{"language"}, seq: []string{"items"}},
		{typeName: "EXTENDED", attrs: []string{"language", "code"}, seq: []string{"items", "value"}},
		{typeName: "INHERITED", attrs: []string{"language", "code"}, seq: []string{"items", "value", "extra"}},
		{typeName: "TEXT_ITEM", attrs: []string{"id"}},
		{typeName: "TEXT_ITEM_AT_PATH", attrs: []string{"id", "path"}},
		{typeName: "TOKEN_ITEM", attrs: []string{"terminology"}},
		{typeName: "PLAIN", seq: []string{"value"}},
	}
	for _, tc := range cases {
		t.Run(tc.typeName, func(t *testing.T) {
			attrs, err := set.attributes(tc.typeName)
			if err != nil {
				t.Fatalf("attributes(%s): %v", tc.typeName, err)
			}
			if !slices.Equal(attrs, tc.attrs) {
				t.Errorf("attributes(%s) = %q, want %q", tc.typeName, attrs, tc.attrs)
			}
			seq, err := set.sequence(tc.typeName)
			if err != nil {
				t.Fatalf("sequence(%s): %v", tc.typeName, err)
			}
			if !slices.Equal(seq, tc.seq) {
				t.Errorf("sequence(%s) = %q, want %q", tc.typeName, seq, tc.seq)
			}
		})
	}
}

// TestXSDAttributesOfVendoredTypes pins the attributes the reader finds in
// the vendored schemas: `archetype_node_id` on LOCATABLE and, through the
// base chain, on its descendants; the `id` the BASE resource classes and
// StringDictionaryItem declare; and the AM Release-1.4 attributes of the
// ontology and the operational template, on a type or in its extension.
//
// REQ-056: the generated codec writes and reads as an XML attribute what the
// class's ITS-XML schema type declares as one.
func TestXSDAttributesOfVendoredTypes(t *testing.T) {
	rmSet, amSet := loadTestXSDSets(t)
	cases := []struct {
		set      string
		typeName string
		want     []string
	}{
		{"rm", "LOCATABLE", []string{"archetype_node_id"}},
		{"rm", "OBSERVATION", []string{"archetype_node_id"}},
		{"rm", "ELEMENT", []string{"archetype_node_id"}},
		{"rm", "DV_TEXT", nil},
		{"rm", "StringDictionaryItem", []string{"id"}},
		{"rm", "RESOURCE_DESCRIPTION_ITEM", []string{"id"}},
		{"rm", "TRANSLATION_DETAILS", []string{"id"}},
		{"am", "RESOURCE_DESCRIPTION_ITEM", nil},
		{"am", "ARCHETYPE_TERM", []string{"code"}},
		{"am", "CodeDefinitionSet", []string{"language"}},
		{"am", "TermBindingSet", []string{"terminology"}},
		{"am", "FLAT_ARCHETYPE_ONTOLOGY", []string{"archetype_id"}},
		{"am", "ANNOTATION", []string{"path"}},
		{"am", "C_COMPLEX_OBJECT", nil},
	}
	for _, tc := range cases {
		t.Run(tc.set+"/"+tc.typeName, func(t *testing.T) {
			set := rmSet
			if tc.set == "am" {
				set = amSet
			}
			got, err := set.attributes(tc.typeName)
			if err != nil {
				t.Fatalf("attributes(%s): %v", tc.typeName, err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("attributes(%s) = %q, want %q", tc.typeName, got, tc.want)
			}
		})
	}
}

// TestXSDAttributesRefuses pins each declaration the attribute reader refuses
// rather than read into a wrong attribute set: an attribute group, which it
// does not expand, on a type, in an extension or in a restriction; an
// attribute wildcard in a restriction; an attribute with no name; and a type
// two files declare with different attributes; a simple-content base that is
// declared nowhere; and a restriction of a declared type, whose inherited
// attributes it does not work out.
//
// REQ-056: the attributes the codec writes are the ones the schema declares.
func TestXSDAttributesRefuses(t *testing.T) {
	plainA := `<xs:complexType name="A"><xs:sequence><xs:element name="a"/></xs:sequence><xs:attribute name="id"/></xs:complexType>`
	codeA := `<xs:complexType name="A"><xs:sequence><xs:element name="a"/></xs:sequence><xs:attribute name="code"/></xs:complexType>`
	for _, tc := range []struct {
		name    string
		files   map[string]string
		wantErr string
	}{
		{
			name: "an attribute group on a type",
			files: map[string]string{"s/a.xsd": xsdDoc(
				`<xs:attributeGroup name="G"><xs:attribute name="id"/></xs:attributeGroup>` +
					`<xs:complexType name="A"><xs:sequence/><xs:attributeGroup ref="G"/></xs:complexType>`)},
			wantErr: "unexpected xs:attributeGroup",
		},
		{
			name: "an attribute group in an extension",
			files: map[string]string{"s/a.xsd": xsdDoc(
				`<xs:complexType name="B"><xs:sequence/></xs:complexType>` +
					`<xs:complexType name="A"><xs:complexContent><xs:extension base="B"><xs:attributeGroup ref="G"/></xs:extension></xs:complexContent></xs:complexType>`)},
			wantErr: "unexpected xs:attributeGroup",
		},
		{
			// A built-in base leaves the type nothing to inherit, so only
			// the parse can refuse the group.
			name: "an attribute group in a restriction of a built-in type",
			files: map[string]string{"s/a.xsd": xsdDoc(
				`<xs:complexType name="A"><xs:complexContent><xs:restriction base="xs:anyType"><xs:sequence/><xs:attributeGroup ref="G"/></xs:restriction></xs:complexContent></xs:complexType>`)},
			wantErr: "unexpected xs:attributeGroup in xs:restriction",
		},
		{
			name: "an attribute wildcard in a restriction of a built-in type",
			files: map[string]string{"s/a.xsd": xsdDoc(
				`<xs:complexType name="A"><xs:complexContent><xs:restriction base="xs:anyType"><xs:sequence/><xs:anyAttribute/></xs:restriction></xs:complexContent></xs:complexType>`)},
			wantErr: "unexpected xs:anyAttribute in xs:restriction",
		},
		{
			name: "an attribute wildcard on a type",
			files: map[string]string{"s/a.xsd": xsdDoc(
				`<xs:complexType name="A"><xs:sequence/><xs:anyAttribute/></xs:complexType>`)},
			wantErr: "unexpected xs:anyAttribute",
		},
		{
			name: "an attribute wildcard in an extension",
			files: map[string]string{"s/a.xsd": xsdDoc(
				`<xs:complexType name="B"><xs:sequence/></xs:complexType>` +
					`<xs:complexType name="A"><xs:complexContent><xs:extension base="B"><xs:anyAttribute/></xs:extension></xs:complexContent></xs:complexType>`)},
			wantErr: "unexpected xs:anyAttribute in xs:extension",
		},
		{
			// Recorded by name, a prohibited attribute would be listed as one
			// the type has.
			name: "a prohibited attribute on a type",
			files: map[string]string{"s/a.xsd": xsdDoc(
				`<xs:complexType name="A"><xs:sequence/><xs:attribute name="id" use="prohibited"/></xs:complexType>`)},
			wantErr: `attribute id is prohibited`,
		},
		{
			name: "a prohibited attribute in an extension",
			files: map[string]string{"s/a.xsd": xsdDoc(
				`<xs:complexType name="B"><xs:sequence/><xs:attribute name="id"/></xs:complexType>` +
					`<xs:complexType name="A"><xs:complexContent><xs:extension base="B"><xs:attribute name="id" use="prohibited"/></xs:extension></xs:complexContent></xs:complexType>`)},
			wantErr: `attribute id is prohibited`,
		},
		{
			name: "a prohibited attribute in a restriction of a built-in type",
			files: map[string]string{"s/a.xsd": xsdDoc(
				`<xs:complexType name="A"><xs:complexContent><xs:restriction base="xs:anyType"><xs:sequence/><xs:attribute name="id" use="prohibited"/></xs:restriction></xs:complexContent></xs:complexType>`)},
			wantErr: `attribute id is prohibited`,
		},
		{
			name: "an attribute with no name",
			files: map[string]string{"s/a.xsd": xsdDoc(
				`<xs:complexType name="A"><xs:simpleContent><xs:extension base="xs:string"><xs:attribute ref="xml:lang"/></xs:extension></xs:simpleContent></xs:complexType>`)},
			wantErr: "an attribute has no name",
		},
		{
			name: "a type two files declare with different attributes",
			files: map[string]string{
				"s/a.xsd": xsdDoc(`<xs:include schemaLocation="b.xsd"/>` + plainA),
				"s/b.xsd": xsdDoc(codeA),
			},
			wantErr: "type A has the attributes",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := writeSchemaTree(t, tc.files)
			_, err := loadXSDSet(root, []string{"s/a.xsd"})
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("loadXSDSet = %v, want an error containing %q", err, tc.wantErr)
			}
		})
	}

	root := writeSchemaTree(t, map[string]string{"s/a.xsd": xsdDoc(
		`<xs:complexType name="MissingSimpleBase"><xs:simpleContent><xs:extension base="NOPE"><xs:attribute name="id"/></xs:extension></xs:simpleContent></xs:complexType>` +
			`<xs:complexType name="B"><xs:sequence><xs:element name="b"/></xs:sequence><xs:attribute name="id"/></xs:complexType>` +
			`<xs:complexType name="Restricted"><xs:complexContent><xs:restriction base="B"><xs:sequence><xs:element name="b"/></xs:sequence></xs:restriction></xs:complexContent></xs:complexType>`)})
	set, err := loadXSDSet(root, []string{"s/a.xsd"})
	if err != nil {
		t.Fatalf("loadXSDSet: %v", err)
	}
	const wantErr = "base type NOPE is not declared"
	if _, err := set.attributes("MissingSimpleBase"); err == nil || !strings.Contains(err.Error(), wantErr) {
		t.Errorf("attributes(MissingSimpleBase) = %v, want an error containing %q", err, wantErr)
	}
	if _, err := set.sequence("MissingSimpleBase"); err == nil || !strings.Contains(err.Error(), wantErr) {
		t.Errorf("sequence(MissingSimpleBase) = %v, want an error containing %q", err, wantErr)
	}
	const wantRestricted = "does not derive attributes through a restriction"
	if _, err := set.attributes("Restricted"); err == nil || !strings.Contains(err.Error(), wantRestricted) {
		t.Errorf("attributes(Restricted) = %v, want an error containing %q", err, wantRestricted)
	}
	if seq, err := set.sequence("Restricted"); err != nil || !slices.Equal(seq, []string{"b"}) {
		t.Errorf("sequence(Restricted) = %q, %v; want the restated [b]", seq, err)
	}
}
