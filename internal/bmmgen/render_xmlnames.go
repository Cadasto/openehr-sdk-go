package bmmgen

import (
	"bytes"
	"fmt"
	"go/format"
	"maps"
	"slices"
	"strings"
)

// xmlNamesFile is the per-target file that holds the ITS-XML schema names
// of the classes whose schema name differs from their BMM name
// ([Target.XMLTypeNames]), and the two helpers the canonical-XML codecs
// call to write and read them.
const xmlNamesFile = "xmlnames_gen.go"

// checkXMLTypeNames refuses a target whose [Target.XMLTypeNames] the
// codecs could not honour: a BMM name that is no concrete class the target
// emits, a schema name that is also the name of a declared class (the
// decoder would read it as that name's class, not as this one), two classes
// given one schema name, and, when the schemas are loaded, a schema name the
// target's own schema set does not declare.
func checkXMLTypeNames(plan *Plan) error {
	names := plan.Target.XMLTypeNames
	seen := make(map[string]string, len(names))
	for _, class := range slices.Sorted(maps.Keys(names)) {
		schema := names[class]
		pc, ok := plan.Classes[class]
		if !ok || pc.External || !slices.Contains(plan.ConcreteClasses, pc) {
			return fmt.Errorf("bmmgen: XML type name of %s: no concrete class of the %s target", class, plan.Target.GoPackage)
		}
		if _, ok := declaredClass(plan, schema); ok {
			return fmt.Errorf("bmmgen: XML type name %s of %s is the name of a declared class", schema, class)
		}
		if other, ok := seen[schema]; ok {
			return fmt.Errorf("bmmgen: XML type name %s is given to both %s and %s", schema, other, class)
		}
		seen[schema] = class
		if plan.xmlOrder != nil && len(plan.xmlOrder.sets) > 0 && !plan.xmlOrder.sets[0].has(schema) {
			return fmt.Errorf("bmmgen: XML type name %s of %s is not declared by the target's ITS-XML schemas", schema, class)
		}
	}
	return nil
}

// RenderXMLNamesFile renders <pkg>/xmlnames_gen.go for a target whose
// classes have ITS-XML schema names ([Target.XMLTypeNames]), and nil for
// one without.
func RenderXMLNamesFile(plan *Plan) ([]byte, error) {
	names := plan.Target.XMLTypeNames
	if len(names) == 0 {
		return nil, nil
	}
	var b bytes.Buffer
	b.WriteString(renderGeneratedHeader(plan))
	b.WriteString("\n")
	b.WriteString("import (\n")
	b.WriteString("\t\"encoding/xml\"\n")
	b.WriteString("\t\"slices\"\n\n")
	b.WriteString("\t\"github.com/cadasto/openehr-sdk-go/openehr/internal/xmlunmatched\"\n")
	b.WriteString(")\n\n")

	b.WriteString("// xmlBMMNames maps the name the ITS-XML schemas give a class to the\n")
	b.WriteString("// class's BMM name, for each class whose two names differ. The type\n")
	b.WriteString("// registry holds the BMM name; canonical XML writes the schema name.\n")
	b.WriteString("var xmlBMMNames = map[string]string{\n")
	for _, class := range slices.Sorted(maps.Keys(names)) {
		fmt.Fprintf(&b, "\t%q: %q,\n", names[class], class)
	}
	b.WriteString("}\n\n")

	b.WriteString("// xmlBMMTyped returns start with each `xsi:type` that names a class by\n")
	b.WriteString("// its schema name in xmlBMMNames rewritten to the class's BMM name, so\n")
	b.WriteString("// that dispatch through the type registry resolves it. The attributes\n")
	b.WriteString("// of start are not modified.\n")
	b.WriteString("func xmlBMMTyped(start xml.StartElement) xml.StartElement {\n")
	b.WriteString("\tvar attrs []xml.Attr\n")
	b.WriteString("\tfor i, a := range start.Attr {\n")
	b.WriteString("\t\tname, ok := xmlBMMNames[a.Value]\n")
	b.WriteString("\t\tif !ok || !xmlunmatched.IsDiscriminator(a.Name) {\n")
	b.WriteString("\t\t\tcontinue\n")
	b.WriteString("\t\t}\n")
	b.WriteString("\t\tif attrs == nil {\n")
	b.WriteString("\t\t\tattrs = slices.Clone(start.Attr)\n")
	b.WriteString("\t\t}\n")
	b.WriteString("\t\tattrs[i].Value = name\n")
	b.WriteString("\t}\n")
	b.WriteString("\tif attrs != nil {\n")
	b.WriteString("\t\tstart.Attr = attrs\n")
	b.WriteString("\t}\n")
	b.WriteString("\treturn start\n")
	b.WriteString("}\n\n")

	b.WriteString("// xmlSchemaTyped returns attrs with each `xsi:type` that names the class\n")
	b.WriteString("// bmmName spelled schemaName, the name the ITS-XML schemas give it.\n")
	b.WriteString("// attrs itself is not modified.\n")
	b.WriteString("func xmlSchemaTyped(attrs []xml.Attr, bmmName, schemaName string) []xml.Attr {\n")
	b.WriteString("\tvar out []xml.Attr\n")
	b.WriteString("\tfor i, a := range attrs {\n")
	b.WriteString("\t\tif a.Value != bmmName || !xmlunmatched.IsDiscriminator(a.Name) {\n")
	b.WriteString("\t\t\tcontinue\n")
	b.WriteString("\t\t}\n")
	b.WriteString("\t\tif out == nil {\n")
	b.WriteString("\t\t\tout = slices.Clone(attrs)\n")
	b.WriteString("\t\t}\n")
	b.WriteString("\t\tout[i].Value = schemaName\n")
	b.WriteString("\t}\n")
	b.WriteString("\tif out == nil {\n")
	b.WriteString("\t\treturn attrs\n")
	b.WriteString("\t}\n")
	b.WriteString("\treturn out\n")
	b.WriteString("}\n")

	formatted, err := format.Source(b.Bytes())
	if err != nil {
		return b.Bytes(), fmt.Errorf("gofmt %s: %w", xmlNamesFile, err)
	}
	return formatted, nil
}

// xmlSiteStart returns the expression a polymorphic decode site hands
// canxml as the element's start: `xmlBMMTyped(_t)` when the site's Go
// interface iface (or the narrow interface's parent) can hold a class with
// an ITS-XML schema name, so that name resolves, and `_t` otherwise.
func xmlSiteStart(plan *Plan, iface string) string {
	iface = strings.TrimSuffix(iface, "Like")
	for class := range plan.Target.XMLTypeNames {
		for _, name := range nearestLineage(plan, class) {
			if pc, ok := plan.Classes[name]; ok && (pc.GoName == iface || name == iface) {
				return "xmlBMMTyped(_t)"
			}
		}
	}
	return "_t"
}
