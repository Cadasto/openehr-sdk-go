package bmmgen

// REQ-041 § SDK-authored root schemas — what P_BMM has no construct for, a
// root says in SDK extension keys of its JSON, one key per kind, which the
// generator reads and the public loader leaves aside. The fixture root under
// testdata/sdk_extensions/sdk carries one entry of each key over the pinned
// fixture beside it. Each test pins one key's rule against the plan or the
// generated source, with the pinned fixture alone as the control where the rule
// changes generated code, and the refusals on scratch copies of the root, each
// edited by one replacement.

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// The SDK extension key fixtures: a pinned schema and a root over it.
const (
	extensionFixtures = "testdata/sdk_extensions"
	extensionBaseID   = "test_base_1.0.0"
	extensionRootID   = "test_extended_1.0.0"
)

// extensionTarget is the target that loads rootID from the fixtures.
func extensionTarget(rootID string) Target {
	return Target{RootID: rootID, GoPackage: "fixture", OutSubDir: "fixture", RegistryImport: "example.org/typereg"}
}

// extensionPlan builds the plan of rootID over the fixtures.
func extensionPlan(t *testing.T, rootID string) *Plan {
	t.Helper()
	p, err := BuildPlanForTarget(t.Context(), extensionTarget(rootID), NewResolver(extensionFixtures))
	if err != nil {
		t.Fatalf("BuildPlanForTarget(%s) = %v", rootID, err)
	}
	return p
}

// readExtensionFixture returns the fixture file at path under the fixtures.
func readExtensionFixture(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(extensionFixtures, filepath.FromSlash(path)))
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return string(b)
}

// editedRootPlan copies the fixtures to a scratch directory with old replaced
// by replacement in the root's JSON, once, and returns the plan built over the
// copy, or the error building it.
func editedRootPlan(t *testing.T, old, replacement string) (*Plan, error) {
	t.Helper()
	root := readExtensionFixture(t, "sdk/"+extensionRootID+schemaFileSuffix)
	if strings.Count(root, old) != 1 {
		t.Fatalf("the fixture root holds %q %d times; the edit wants it once", old, strings.Count(root, old))
	}
	dir := t.TempDir()
	writeSchemaFiles(t, dir, map[string]string{
		extensionBaseID + schemaFileSuffix:          readExtensionFixture(t, extensionBaseID+schemaFileSuffix),
		"sdk/" + extensionRootID + schemaFileSuffix: strings.Replace(root, old, replacement, 1),
	})
	return BuildPlanForTarget(t.Context(), extensionTarget(extensionRootID), NewResolver(dir))
}

// refusal is one edit of the fixture root that generation must refuse, with
// what the error must say.
type refusal struct {
	name     string
	old, new string
	want     string
}

// checkRefusals builds the plan of each edit and holds it to failing with an
// error that says the case's want. render, when set, also renders the plan,
// for a rule generation applies when it renders.
func checkRefusals(t *testing.T, cases []refusal, render func(*Plan) error) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p, err := editedRootPlan(t, tc.old, tc.new)
			if err == nil && render != nil {
				err = render(p)
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("generation with %q for %q = %v; want an error saying %q", tc.new, tc.old, err, tc.want)
			}
		})
	}
}

// renderedFiles renders every per-package file of p, the struct file and its
// four codec companions, and returns them by file name.
func renderedFiles(t *testing.T, p *Plan) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, f := range p.Files {
		for suffix, render := range map[string]func(*Plan, *PlannedFile) ([]byte, error){
			"_gen.go":           RenderFile,
			"_jsonmar_gen.go":   RenderMarshalJSONFile,
			"_jsonunmar_gen.go": RenderUnmarshalJSONFile,
			"_xmlmar_gen.go":    RenderMarshalXMLFile,
			"_xmlunmar_gen.go":  RenderUnmarshalXMLFile,
		} {
			b, err := render(p, f)
			if err != nil {
				t.Fatalf("render %s%s: %v", f.FileBase, suffix, err)
			}
			out[f.FileBase+suffix] = string(b)
		}
	}
	reg, err := RenderTypeRegFile(p)
	if err != nil {
		t.Fatalf("RenderTypeRegFile: %v", err)
	}
	out["typereg_gen.go"] = string(reg)
	hooks, err := RenderJSONHooksFile(p)
	if err != nil {
		t.Fatalf("RenderJSONHooksFile: %v", err)
	}
	out["jsonhooks_gen.go"] = string(hooks)
	return out
}

// TestSDKExtensions_REQ041_FixtureRootPassesTheRootCheck holds the fixture root
// to the root check, so its entries are the well-formed ones the tests below
// build on, and pins that the public loader leaves the extension keys aside.
func TestSDKExtensions_REQ041_FixtureRootPassesTheRootCheck(t *testing.T) {
	t.Parallel()
	problems, err := checkRoot(t.Context(), extensionFixtures, testITSXMLDir, extensionRootID)
	if err != nil {
		t.Fatalf("checkRoot(%s) = %v", extensionRootID, err)
	}
	for _, p := range problems {
		t.Error(p)
	}
	merged, err := bmm.LoadAll(extensionRootID, NewResolver(extensionFixtures))
	if err != nil {
		t.Fatalf("bmm.LoadAll(%s) = %v; the loader must read a root that carries extension keys", extensionRootID, err)
	}
	if _, ok := merged.ClassDefinitions["CODED_ITEM"]; !ok {
		t.Errorf("bmm.LoadAll(%s) has no CODED_ITEM: the loader must leave classes_left_out aside, the generator applies it", extensionRootID)
	}
}

// TestSDKExtensions_REQ041_ParseRefusesWhatItCannotRead pins that the generator
// reads every member of the extension keys or refuses the root: a key or an
// entry member it does not know, an entry that leaves out its subject, and a
// schema form it does not know.
func TestSDKExtensions_REQ041_ParseRefusesWhatItCannotRead(t *testing.T) {
	t.Parallel()
	checkRefusals(t, []refusal{
		{name: "an unknown key", old: `"classes_left_out": [`, new: `"classes_left_aside": [`, want: "classes_left_aside"},
		{name: "an unknown entry member", old: `"texts": ["false"],`, new: `"texts": ["false"], "value": "false",`, want: `"value"`},
		{name: "an entry with no subject", old: `{"class": "CODED_ITEM", `, new: `{`, want: "sdk_extensions.classes_left_out[0] has no class"},
		{name: "a dropped element with no text", old: `"texts": ["false"],`, new: ``, want: "sdk_extensions.dropped_elements[0] has no text"},
		{name: "an element path with an empty step", old: `"element_path": "constraints",`, new: `"element_path": "constraints//items",`, want: "sdk_extensions.anonymous_type_names[0] has no element_path"},
		{name: "an unknown schema form", old: `"form": "choice",`, new: `"form": "sequence",`, want: `has the form "sequence"`},
		{name: "a choice of one shape", old: `"shapes": ["TEXT", "CODE"]`, new: `"shapes": ["TEXT"]`, want: "sdk_extensions.schema_forms[0] has no two or more shapes"},
	}, nil)
}

// TestSDKExtensions_REQ041_ClassesLeftOut pins classes_left_out: the class the
// root leaves out, which the pinned fixture alone emits, gets no generated type,
// registry entry or decode hook, and the class universe reports it as an
// excluded class. A class no schema declares, a class left out twice, and a
// left-out class that a class the target emits still names are refused.
func TestSDKExtensions_REQ041_ClassesLeftOut(t *testing.T) {
	t.Parallel()
	control := renderedFiles(t, extensionPlan(t, extensionBaseID))
	if !strings.Contains(control["org_test_model_gen.go"], "type CodedItem struct") ||
		!strings.Contains(control["typereg_gen.go"], `Register("CODED_ITEM"`) ||
		!strings.Contains(control["jsonhooks_gen.go"], "ItemLike") {
		t.Fatalf("the pinned fixture alone emits no CodedItem, registers no CODED_ITEM or hooks no ItemLike; the control proves nothing:\n%s\n%s", control["typereg_gen.go"], control["jsonhooks_gen.go"])
	}

	p := extensionPlan(t, extensionRootID)
	if _, ok := p.Classes["CODED_ITEM"]; ok {
		t.Errorf("the plan holds CODED_ITEM, which the root leaves out")
	}
	for name, src := range renderedFiles(t, p) {
		if strings.Contains(src, "CodedItem") || strings.Contains(src, `"CODED_ITEM"`) {
			t.Errorf("%s names CODED_ITEM, which the root leaves out:\n%s", name, src)
		}
	}
	entries, err := rmInfoAbsence(p)
	if err != nil {
		t.Fatalf("rmInfoAbsence: %v", err)
	}
	if i := slices.IndexFunc(entries, func(e absenceEntry) bool { return e.className == "CODED_ITEM" }); i < 0 || entries[i].reason != absenceExcludedClass {
		t.Errorf("the absence table = %v; want CODED_ITEM as %s", entries, absenceExcludedClass)
	}

	checkRefusals(t, []refusal{
		{name: "a class no schema declares", old: `{"class": "CODED_ITEM", `, new: `{"class": "NO_SUCH_CLASS", `, want: "NO_SUCH_CLASS"},
		{name: "a class left out twice", old: `"classes_left_out": [`, new: `"classes_left_out": [{"class": "CODED_ITEM", "reason": "r.", "upstream": "https://example.org/pull/1"}, `, want: "leaves out CODED_ITEM twice"},
		{name: "a class an emitted class names", old: `{"class": "CODED_ITEM", `, new: `{"class": "ITEM", `, want: "ITEM_LIST names ITEM, which the target leaves out"},
	}, nil)
}

// TestSDKExtensions_REQ041_TypeAliases pins type_aliases: the plan carries
// each alias and the class it reads as, for the canonical-XML dispatch, and the
// type registry holds the class and not the alias. An alias a schema declares
// as a class, an alias of a class the target does not hold, and an alias given
// twice are refused.
func TestSDKExtensions_REQ041_TypeAliases(t *testing.T) {
	t.Parallel()
	p := extensionPlan(t, extensionRootID)
	if want := map[string]string{"LEGACY_ROOT_OBJECT": "ROOT_OBJECT"}; !maps.Equal(p.extensions.typeAliases, want) {
		t.Errorf("plan type aliases = %v; want %v", p.extensions.typeAliases, want)
	}
	reg := renderedFiles(t, p)["typereg_gen.go"]
	if !strings.Contains(reg, `Register("ROOT_OBJECT"`) || strings.Contains(reg, "LEGACY_ROOT_OBJECT") {
		t.Errorf("typereg_gen.go must register ROOT_OBJECT and not its alias:\n%s", reg)
	}

	checkRefusals(t, []refusal{
		{name: "an alias a schema declares", old: `"alias": "LEGACY_ROOT_OBJECT"`, new: `"alias": "ITEM"`, want: "ITEM, which a schema declares"},
		{name: "a class the target does not hold", old: `"class": "ROOT_OBJECT"`, new: `"class": "NO_SUCH_CLASS"`, want: "NO_SUCH_CLASS, which the target does not hold"},
		{name: "an alias of a class left out", old: `"class": "ROOT_OBJECT"`, new: `"class": "CODED_ITEM"`, want: "CODED_ITEM, which the target does not hold"},
		{name: "an alias given twice", old: `"type_aliases": [`, new: `"type_aliases": [{"alias": "LEGACY_ROOT_OBJECT", "class": "ITEM", "reason": "r.", "upstream": "https://example.org/pull/2"}, `, want: "LEGACY_ROOT_OBJECT twice"},
	}, nil)
}

// TestSDKExtensions_REQ041_REQ176_DroppedElements pins dropped_elements, the
// rule behind REQ-176's `match_negated`: the decoder of every class that
// inherits from the class the entry names reads the element through
// xmlunmatched.DropElement with the one text it may hold, where the pinned
// fixture alone hands it to the default arm as unmatched content, and the
// encoder never writes it. An element that names a member, a class the target
// does not hold and an element dropped twice are refused.
func TestSDKExtensions_REQ041_REQ176_DroppedElements(t *testing.T) {
	t.Parallel()
	const want = "\t\t\tcase \"legacy_flag\":\n" +
		"\t\t\t\t// The schema accepts the element, which no member holds: it is read, and dropped when its text is \"false\".\n" +
		"\t\t\t\tif _err := xmlunmatched.DropElement(_dec, &_t, \"false\"); _err != nil {\n" +
		"\t\t\t\t\treturn _err\n" +
		"\t\t\t\t}\n"
	control := renderedFiles(t, extensionPlan(t, extensionBaseID))["org_test_model_xmlunmar_gen.go"]
	if !strings.Contains(control, "func (s *SingleAttribute) UnmarshalXML") || xmlCaseArm(control, "legacy_flag") != "" {
		t.Fatalf("the pinned fixture alone decodes no SingleAttribute, or reads legacy_flag; the control proves nothing:\n%s", control)
	}

	files := renderedFiles(t, extensionPlan(t, extensionRootID))
	unmar := files["org_test_model_xmlunmar_gen.go"]
	decoder := unmar[strings.Index(unmar, "func (s *SingleAttribute) UnmarshalXML"):]
	if got := "\t" + xmlCaseArm(decoder, "legacy_flag"); got != want {
		t.Errorf("SingleAttribute.UnmarshalXML's legacy_flag arm =\n%s\nwant\n%s", got, want)
	}
	for name, src := range files {
		if strings.HasSuffix(name, "_xmlunmar_gen.go") {
			continue
		}
		if strings.Contains(src, "legacy_flag") {
			t.Errorf("%s names legacy_flag, which only the XML decoder reads:\n%s", name, src)
		}
	}

	renderXML := func(p *Plan) error {
		var errs []error
		for _, f := range p.Files {
			_, err := RenderUnmarshalXMLFile(p, f)
			errs = append(errs, err)
		}
		return errors.Join(errs...)
	}
	checkRefusals(t, []refusal{
		{name: "an element that names a member", old: `"element": "legacy_flag"`, new: `"element": "rm_attribute_name"`, want: "drops rm_attribute_name, which is one of its members"},
		{name: "a class the target does not hold", old: `{"class": "ATTRIBUTE", `, new: `{"class": "CODED_ITEM", `, want: "CODED_ITEM, which the target does not hold"},
		{name: "an element dropped twice", old: `"dropped_elements": [`, new: `"dropped_elements": [{"class": "ATTRIBUTE", "element": "legacy_flag", "texts": ["true"], "reason": "r.", "upstream": "https://example.org/pull/3"}, `, want: "drops ATTRIBUTE.legacy_flag twice"},
	}, renderXML)
}

// TestSDKExtensions_REQ041_REQ176_DroppedElementTexts pins that one
// dropped_elements entry admits several texts, as REQ-176's `match_negated`
// admits false written `false` or `0`: the decoder's arm hands every text the
// entry gives to xmlunmatched.DropElement, which drops the element when its
// text is any of them. An entry whose texts are none, or hold an empty one, is
// refused.
func TestSDKExtensions_REQ041_REQ176_DroppedElementTexts(t *testing.T) {
	t.Parallel()
	p, err := editedRootPlan(t, `"texts": ["false"],`, `"texts": ["false", "0"],`)
	if err != nil {
		t.Fatalf("generation with two texts = %v; want a plan", err)
	}
	unmar := renderedFiles(t, p)["org_test_model_xmlunmar_gen.go"]
	decoder := unmar[strings.Index(unmar, "func (s *SingleAttribute) UnmarshalXML"):]
	const want = `xmlunmatched.DropElement(_dec, &_t, "false", "0")`
	if arm := xmlCaseArm(decoder, "legacy_flag"); !strings.Contains(arm, want) {
		t.Errorf("SingleAttribute.UnmarshalXML's legacy_flag arm =\n%s\nwant it to call %s", arm, want)
	}

	checkRefusals(t, []refusal{
		{name: "no texts", old: `"texts": ["false"],`, new: `"texts": [],`, want: "sdk_extensions.dropped_elements[0] has no text"},
		{name: "an empty text", old: `"texts": ["false"],`, new: `"texts": ["false", ""],`, want: "sdk_extensions.dropped_elements[0] has no text"},
	}, nil)
}

// TestSDKExtensions_REQ041_AnonymousTypeNames pins anonymous_type_names: the
// plan carries the class named for each anonymous schema type, by the type's
// path, for the schema reading that consumes it. A class the target does not
// hold, a path named twice and a class named for two paths are refused.
func TestSDKExtensions_REQ041_AnonymousTypeNames(t *testing.T) {
	t.Parallel()
	p := extensionPlan(t, extensionRootID)
	if want := map[string]string{"VIEW/constraints": "VIEW_CONSTRAINT"}; !maps.Equal(p.extensions.anonymousTypeNames, want) {
		t.Errorf("plan anonymous type names = %v; want %v", p.extensions.anonymousTypeNames, want)
	}

	checkRefusals(t, []refusal{
		{name: "a class the target does not hold", old: `"class": "VIEW_CONSTRAINT"`, new: `"class": "NO_SUCH_CLASS"`, want: "NO_SUCH_CLASS, which the target does not hold"},
		{name: "a path named twice", old: `"anonymous_type_names": [`, new: `"anonymous_type_names": [{"schema_type": "VIEW", "element_path": "constraints", "class": "ITEM", "reason": "r.", "upstream": "https://example.org/pull/4"}, `, want: "VIEW/constraints twice"},
		{name: "a class named for two paths", old: `"anonymous_type_names": [`, new: `"anonymous_type_names": [{"schema_type": "VIEW", "element_path": "items", "class": "VIEW_CONSTRAINT", "reason": "r.", "upstream": "https://example.org/pull/4"}, `, want: "names VIEW_CONSTRAINT for VIEW/items and for VIEW/constraints"},
	}, nil)
}

// TestSDKExtensions_REQ041_REQ176_AnonymousTypeNamesTakeTheSchemaType pins
// that a class anonymous_type_names names takes the anonymous ITS-XML schema
// type at its path, as the XML codec reads a class's schema type: VIEW_CONSTRAINT
// takes the `path` attribute of the type declared inside VIEW's constraints,
// which no named type gives it. The entry is checked against the schemas: a
// path that names no anonymous type of the target's schema sets is refused.
func TestSDKExtensions_REQ041_REQ176_AnonymousTypeNamesTakeTheSchemaType(t *testing.T) {
	t.Parallel()
	withOrder := func(t *testing.T, view string) (*Plan, error) {
		t.Helper()
		p := extensionPlan(t, extensionRootID)
		root := writeSchemaTree(t, map[string]string{"s/view.xsd": xsdDoc(view)})
		set, err := loadXSDSet(root, []string{"s/view.xsd"})
		if err != nil {
			t.Fatalf("loadXSDSet: %v", err)
		}
		p.xmlOrder = &xmlChildOrder{sets: []*xsdSet{set}}
		return p, p.xmlOrder.checkAnonymousTypes(p)
	}

	p, err := withOrder(t, `<xs:complexType name="VIEW"><xs:sequence><xs:element name="constraints" maxOccurs="unbounded">`+
		`<xs:complexType><xs:sequence/><xs:attribute name="path" type="xs:string" use="required"/></xs:complexType>`+
		`</xs:element></xs:sequence></xs:complexType>`)
	if err != nil {
		t.Fatalf("checkAnonymousTypes over a schema that declares VIEW/constraints = %v; want nil", err)
	}
	if got := classXMLAttributes(t, p, "VIEW_CONSTRAINT"); !slices.Equal(got.members, []string{"path"}) {
		t.Errorf("VIEW_CONSTRAINT's XML attribute members = %q; want [path], from the anonymous type at VIEW/constraints", got.members)
	}

	if _, err := withOrder(t, `<xs:complexType name="VIEW_CONSTRAINT"><xs:sequence/></xs:complexType>`+
		`<xs:complexType name="VIEW"><xs:sequence><xs:element name="constraints" type="VIEW_CONSTRAINT"/></xs:sequence></xs:complexType>`); err == nil ||
		!strings.Contains(err.Error(), "anonymous_type_names names VIEW/constraints, which no ITS-XML schema of the target declares") {
		t.Errorf("checkAnonymousTypes over a schema whose VIEW/constraints has a named type = %v; want a refusal naming the path", err)
	}
}

// TestSDKExtensions_REQ041_REQ176_SchemaFormChoice pins the choice form, the
// rule behind REQ-176's ORDINAL.symbol: the decoder hands the member's element
// to xmlunmatched.DecodeChoice with the shapes the entry lists, in its order,
// each by the child elements of its class and a constructor of it, where the
// pinned fixture alone decodes the element as its one class; and the encoder
// writes the value of whichever shape the member holds, by pointer or by
// value, under the member's element, and refuses a value of any other type. A
// choice whose member is not typed Any, whose shapes share an element, or one
// of whose shapes has no decoder of its own is refused.
func TestSDKExtensions_REQ041_REQ176_SchemaFormChoice(t *testing.T) {
	t.Parallel()
	method := func(src, recv string) string {
		i := strings.Index(src, recv)
		if i < 0 {
			return ""
		}
		return src[i:]
	}
	control := renderedFiles(t, extensionPlan(t, extensionBaseID))["org_test_model_xmlunmar_gen.go"]
	if arm := xmlCaseArm(method(control, "func (o *Ordinal) UnmarshalXML"), "symbol"); arm == "" || strings.Contains(arm, "DecodeChoice") {
		t.Fatalf("the pinned fixture alone decodes no ORDINAL.symbol, or decodes it as a choice; the control proves nothing:\n%s", arm)
	}

	files := renderedFiles(t, extensionPlan(t, extensionRootID))
	unmar := files["org_test_model_xmlunmar_gen.go"]
	const wantVar = "var xmlChoiceOrdinalSymbol = []xmlunmatched.Shape{\n" +
		"\t{Elements: []string{\"value\", \"defining_code\"}, New: func() any { return new(Text) }},\n" +
		"\t{Elements: []string{\"terminology_id\", \"code_string\"}, New: func() any { return new(Code) }},\n" +
		"}\n"
	if !strings.Contains(unmar, wantVar) {
		t.Errorf("the XML decoders lack the shapes of ORDINAL.symbol:\n%s\nwant\n%s", unmar, wantVar)
	}
	arm := xmlCaseArm(method(unmar, "func (o *Ordinal) UnmarshalXML"), "symbol")
	for _, want := range []string{"xmlunmatched.DecodeChoice(_dec, _t, xmlChoiceOrdinalSymbol)", "o.Symbol = _v"} {
		if !strings.Contains(arm, want) {
			t.Errorf("Ordinal.UnmarshalXML's symbol arm =\n%s\nwant it to hold %s", arm, want)
		}
	}
	encoder := method(files["org_test_model_xmlmar_gen.go"], "func (o *Ordinal) MarshalXML")
	for _, want := range []string{
		"switch _v := o.Symbol.(type) {",
		"case *Text, *Code:\n\t\tif err := _e.EncodeElement(_v, xml.StartElement{Name: xml.Name{Local: \"symbol\"}}); err != nil {",
		"case Text:\n\t\tif err := _e.EncodeElement(&_v, xml.StartElement{Name: xml.Name{Local: \"symbol\"}}); err != nil {",
		"case Code:\n",
		"default:\n\t\treturn fmt.Errorf(",
	} {
		if !strings.Contains(encoder, want) {
			t.Errorf("Ordinal.MarshalXML lacks %q:\n%s", want, encoder)
		}
	}

	renderXML := func(p *Plan) error {
		var errs []error
		for _, f := range p.Files {
			_, err := RenderUnmarshalXMLFile(p, f)
			errs = append(errs, err)
			_, err = RenderMarshalXMLFile(p, f)
			errs = append(errs, err)
		}
		return errors.Join(errs...)
	}
	checkRefusals(t, []refusal{
		{name: "a member not typed Any", old: `"type": "Any", "is_mandatory": true}`, new: `"type": "CODE", "is_mandatory": true}`, want: "ORDINAL.symbol is typed CODE; a choice needs a member typed Any"},
		{name: "a shape twice", old: `"shapes": ["TEXT", "CODE"]`, new: `"shapes": ["TEXT", "TEXT"]`, want: "the shapes TEXT and TEXT of ORDINAL.symbol share the element value"},
		{name: "a shape with no decoder of its own", old: `"shapes": ["TEXT", "CODE"]`, new: `"shapes": ["TEXT", "ATTRIBUTE"]`, want: "the shape ATTRIBUTE of ORDINAL.symbol has no XML decoder of its own"},
	}, renderXML)
}

// TestSDKExtensions_REQ041_SchemaForms pins schema_forms: the plan carries the
// form the schema gives a property's element, the choice of shapes for
// ORDINAL.symbol, for the decoder that consumes it. A property the class does
// not have, a shape the target does not hold, a class the target does not hold
// and a property given two forms are refused.
func TestSDKExtensions_REQ041_SchemaForms(t *testing.T) {
	t.Parallel()
	p := extensionPlan(t, extensionRootID)
	got, ok := p.extensions.schemaForms["ORDINAL"]["symbol"]
	if !ok || got.Form != schemaFormChoice || !slices.Equal(got.Shapes, []string{"TEXT", "CODE"}) {
		t.Errorf("plan schema form of ORDINAL.symbol = %+v (%t); want a choice of TEXT and CODE", got, ok)
	}

	checkRefusals(t, []refusal{
		{name: "a property the class does not have", old: `"property": "symbol"`, new: `"property": "glyph"`, want: "ORDINAL has no property glyph"},
		{name: "a shape the target does not hold", old: `"shapes": ["TEXT", "CODE"]`, new: `"shapes": ["TEXT", "NO_SUCH_CLASS"]`, want: "NO_SUCH_CLASS, which the target does not hold"},
		{name: "a class the target does not hold", old: `{"class": "ORDINAL", `, new: `{"class": "NO_SUCH_CLASS", `, want: "NO_SUCH_CLASS, which the target does not hold"},
		{name: "a property given two forms", old: `"schema_forms": [`, new: `"schema_forms": [{"class": "ORDINAL", "property": "symbol", "form": "choice", "shapes": ["CODE", "TEXT"], "reason": "r.", "upstream": "https://example.org/pull/5"}, `, want: "ORDINAL.symbol twice"},
	}, nil)
}
