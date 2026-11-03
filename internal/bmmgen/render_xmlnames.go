package bmmgen

import (
	"bytes"
	"fmt"
	"go/format"
	"strings"
)

// xmlNamesFile is the per-target file that registers with the canonical-XML
// codec the names the target's classes take beyond their own: the type
// aliases the decoder reads and the root elements the encoder writes.
const xmlNamesFile = "xml_names_gen.go"

// xmlRootElement is one class whose root element is not its lower-cased name.
type xmlRootElement struct {
	class, element string
}

// xmlRootElements returns the classes the target owns whose ITS-XML schema
// declares a global element for the type of the class's own name under
// another name than the lower-cased class name, the encoder's default, such as
// `template` for OPERATIONAL_TEMPLATE, in the order of their names. A plan
// built without the schemas has none.
func xmlRootElements(plan *Plan) []xmlRootElement {
	if plan.xmlOrder == nil {
		return nil
	}
	var out []xmlRootElement
	for _, name := range sortedStringKeys(plan.Classes) {
		if pc := plan.Classes[name]; pc.External {
			continue
		}
		element, ok := plan.xmlOrder.rootElementFor(name)
		if ok && element != strings.ToLower(name) { // canxml.ElementName's default
			out = append(out, xmlRootElement{class: name, element: element})
		}
	}
	return out
}

// RenderXMLNamesFile renders the target's xml_names_gen.go, or returns nil when
// the target registers no name: no type alias of a class it owns, which its
// roots name in their type_aliases, and no root element other than a class's
// lower-cased name.
func RenderXMLNamesFile(plan *Plan) ([]byte, error) {
	roots := xmlRootElements(plan)
	aliases := sortedStringKeys(plan.extensions.typeAliases)
	if len(roots) == 0 && len(aliases) == 0 {
		return nil, nil
	}
	var b bytes.Buffer
	b.WriteString(renderGeneratedHeader(plan))
	b.WriteString("\nimport \"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml\"\n\n")
	b.WriteString("// init registers with the canonical-XML codec the names this package's\n")
	b.WriteString("// classes take beyond their own BMM names: each type alias the decoder\n")
	b.WriteString("// reads as one of them, and the root element of each class whose ITS-XML\n")
	b.WriteString("// schema gives its global element another name.\n")
	b.WriteString("func init() {\n")
	for _, alias := range aliases {
		fmt.Fprintf(&b, "\tcanxml.RegisterTypeAlias(%q, %q)\n", alias, plan.extensions.typeAliases[alias])
	}
	for _, r := range roots {
		fmt.Fprintf(&b, "\tcanxml.RegisterRootElement(%q, %q)\n", r.class, r.element)
	}
	b.WriteString("}\n")
	formatted, err := format.Source(b.Bytes())
	if err != nil {
		return b.Bytes(), fmt.Errorf("gofmt %s: %w", xmlNamesFile, err)
	}
	return formatted, nil
}
