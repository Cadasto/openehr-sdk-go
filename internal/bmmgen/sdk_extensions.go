package bmmgen

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// sdkExtensionsMember is the top-level member of a schema's JSON that holds its
// SDK extension keys: what a root schema says that P_BMM has no construct for.
// No P_BMM member has this name, so the public loader, which leaves a member it
// does not know aside, never reads it; the generator parses it from the same
// bytes the loader reads, through the same resolver.
const sdkExtensionsMember = "sdk_extensions"

// sdkExtensions holds the SDK extension keys of one schema, or of a target's
// include chain merged: one key per kind of statement P_BMM cannot make. Each
// entry carries the reason and the upstream reference it rests on, as a class
// a root declares does in its documentation; the root check holds every entry
// to them, and the generator reads neither.
type sdkExtensions struct {
	// ClassesLeftOut names the classes the target leaves out: the plan holds
	// none of them, so no generated file, registry entry or decode hook names
	// one. A root leaves out a pinned class it declares again under the name
	// the schemas give it.
	ClassesLeftOut []classLeftOut `json:"classes_left_out"`
	// TypeAliases names the type names a decoder reads as another class, such
	// as an `xsi:type` older tools write. The type registry does not hold an
	// alias.
	TypeAliases []typeAlias `json:"type_aliases"`
	// DroppedElements names the elements a class's ITS-XML schema type accepts
	// that no member of the class holds: the XML decoder of the class, and of
	// every class that inherits from it, reads each and drops it when its text
	// is the one the entry gives, and refuses any other text. The encoder never
	// writes one.
	DroppedElements []droppedElement `json:"dropped_elements"`
	// AnonymousTypeNames gives a class name to an anonymous type of an ITS-XML
	// schema, the type of an element declared inside a named type.
	AnonymousTypeNames []anonymousTypeName `json:"anonymous_type_names"`
	// SchemaForms names the properties whose element an ITS-XML schema gives a
	// form P_BMM cannot express, such as a choice between two element shapes.
	SchemaForms []schemaForm `json:"schema_forms"`
}

// classLeftOut is one entry of classes_left_out.
type classLeftOut struct {
	// Class is the BMM name of the class left out.
	Class    string `json:"class"`
	Reason   string `json:"reason"`
	Upstream string `json:"upstream"`
}

// typeAlias is one entry of type_aliases.
type typeAlias struct {
	// Alias is the type name a decoder reads as Class. No schema of the merge
	// declares it.
	Alias string `json:"alias"`
	// Class is the BMM name of the class the alias reads as.
	Class    string `json:"class"`
	Reason   string `json:"reason"`
	Upstream string `json:"upstream"`
}

// droppedElement is one entry of dropped_elements.
type droppedElement struct {
	// Class is the BMM name of the class whose schema type accepts the
	// element; the classes that inherit from it accept it too.
	Class string `json:"class"`
	// Element is the element's local name.
	Element string `json:"element"`
	// Text is the one text the element may hold, less XML white space
	// around it.
	Text     string `json:"text"`
	Reason   string `json:"reason"`
	Upstream string `json:"upstream"`
}

// anonymousTypeName is one entry of anonymous_type_names.
type anonymousTypeName struct {
	// SchemaType is the named ITS-XML schema type the anonymous type is
	// declared in.
	SchemaType string `json:"schema_type"`
	// ElementPath is the path of local element names from SchemaType down to
	// the element whose type is anonymous, separated by "/", such as
	// "constraints/items".
	ElementPath string `json:"element_path"`
	// Class is the BMM name of the class the root declares for the anonymous
	// type.
	Class    string `json:"class"`
	Reason   string `json:"reason"`
	Upstream string `json:"upstream"`
}

// schemaForm is one entry of schema_forms.
type schemaForm struct {
	// Class is the BMM name of the class whose property the schema shapes.
	Class string `json:"class"`
	// Property is the name of the property, one of the class's own or
	// inherited ones.
	Property string `json:"property"`
	// Form is the kind of form: [schemaFormChoice] is the one known.
	Form string `json:"form"`
	// Shapes are, for a choice, the BMM names of the classes whose element
	// shape the property's element takes, one of them, in the order the
	// schema lists them.
	Shapes   []string `json:"shapes"`
	Reason   string   `json:"reason"`
	Upstream string   `json:"upstream"`
}

// schemaFormChoice is the form of a property whose element takes the shape of
// one of two or more classes, such as an ITS-XML `xs:choice` between two
// sequences.
const schemaFormChoice = "choice"

// extensionEntry is one entry of an extension key as the root check reads it:
// the key, the subject the entry is about, and the entry's note.
type extensionEntry struct {
	key, subject     string
	reason, upstream string
}

// entries lists every entry of x, key by key, each key's in the order written.
func (x sdkExtensions) entries() []extensionEntry {
	var out []extensionEntry
	for _, e := range x.ClassesLeftOut {
		out = append(out, extensionEntry{"classes_left_out", e.Class, e.Reason, e.Upstream})
	}
	for _, e := range x.TypeAliases {
		out = append(out, extensionEntry{"type_aliases", e.Alias, e.Reason, e.Upstream})
	}
	for _, e := range x.DroppedElements {
		out = append(out, extensionEntry{"dropped_elements", e.Class + "." + e.Element, e.Reason, e.Upstream})
	}
	for _, e := range x.AnonymousTypeNames {
		out = append(out, extensionEntry{"anonymous_type_names", e.SchemaType + "/" + e.ElementPath, e.Reason, e.Upstream})
	}
	for _, e := range x.SchemaForms {
		out = append(out, extensionEntry{"schema_forms", e.Class + "." + e.Property, e.Reason, e.Upstream})
	}
	return out
}

// merge appends the entries of other to those of x.
func (x *sdkExtensions) merge(other sdkExtensions) {
	x.ClassesLeftOut = append(x.ClassesLeftOut, other.ClassesLeftOut...)
	x.TypeAliases = append(x.TypeAliases, other.TypeAliases...)
	x.DroppedElements = append(x.DroppedElements, other.DroppedElements...)
	x.AnonymousTypeNames = append(x.AnonymousTypeNames, other.AnonymousTypeNames...)
	x.SchemaForms = append(x.SchemaForms, other.SchemaForms...)
}

// parseSDKExtensions returns the SDK extension keys raw, one schema's JSON,
// holds, or none when it has no sdk_extensions member. A member of that object
// or of one of its entries that the generator does not read is an error, so a
// misspelt key is never left unread, and so is an entry that leaves out what it
// is about.
func parseSDKExtensions(raw []byte) (sdkExtensions, error) {
	var doc struct {
		Extensions jsontext.Value `json:"sdk_extensions"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return sdkExtensions{}, err
	}
	var x sdkExtensions
	if len(doc.Extensions) == 0 {
		return x, nil
	}
	if err := json.Unmarshal(doc.Extensions, &x, json.RejectUnknownMembers(true)); err != nil {
		return sdkExtensions{}, fmt.Errorf("%s: %w", sdkExtensionsMember, err)
	}
	return x, x.checkSubjects()
}

// checkSubjects reports each entry of x that leaves out what it is about, and
// each schema form that is no form the generator knows.
func (x sdkExtensions) checkSubjects() error {
	var errs []error
	lacks := func(key string, i int, member string) {
		errs = append(errs, fmt.Errorf("%s.%s[%d] has no %s", sdkExtensionsMember, key, i, member))
	}
	for i, e := range x.ClassesLeftOut {
		if e.Class == "" {
			lacks("classes_left_out", i, "class")
		}
	}
	for i, e := range x.TypeAliases {
		if e.Alias == "" {
			lacks("type_aliases", i, "alias")
		}
		if e.Class == "" {
			lacks("type_aliases", i, "class")
		}
	}
	for i, e := range x.DroppedElements {
		if e.Class == "" {
			lacks("dropped_elements", i, "class")
		}
		if e.Element == "" {
			lacks("dropped_elements", i, "element")
		}
		if e.Text == "" {
			lacks("dropped_elements", i, "text")
		}
	}
	for i, e := range x.AnonymousTypeNames {
		if e.SchemaType == "" {
			lacks("anonymous_type_names", i, "schema_type")
		}
		if e.ElementPath == "" || slices.Contains(strings.Split(e.ElementPath, "/"), "") {
			lacks("anonymous_type_names", i, `element_path of element names separated by "/"`)
		}
		if e.Class == "" {
			lacks("anonymous_type_names", i, "class")
		}
	}
	for i, e := range x.SchemaForms {
		if e.Class == "" {
			lacks("schema_forms", i, "class")
		}
		if e.Property == "" {
			lacks("schema_forms", i, "property")
		}
		switch e.Form {
		case schemaFormChoice:
			if len(e.Shapes) < 2 || slices.Contains(e.Shapes, "") {
				lacks("schema_forms", i, "two or more shapes for its choice")
			}
		case "":
			lacks("schema_forms", i, "form")
		default:
			errs = append(errs, fmt.Errorf("%s.schema_forms[%d] has the form %q; the generator knows only %q", sdkExtensionsMember, i, e.Form, schemaFormChoice))
		}
	}
	return errors.Join(errs...)
}

// chainExtensions parses the SDK extension keys of every schema in chain, a
// target's root and the schemas it includes, and merges them.
func chainExtensions(chain []chainSchema) (sdkExtensions, error) {
	var x sdkExtensions
	for _, s := range chain {
		own, err := parseSDKExtensions(s.raw)
		if err != nil {
			return sdkExtensions{}, fmt.Errorf("bmmgen: %s: %w", s.id, err)
		}
		x.merge(own)
	}
	return x, nil
}

// planExtensions is what a plan keeps of its target's SDK extension keys, by
// subject, once [Plan.applyExtensions] has checked them against the plan.
type planExtensions struct {
	// leftOut is the set of classes the target leaves out.
	leftOut map[string]bool
	// typeAliases maps each alias to the class it reads as.
	typeAliases map[string]string
	// droppedElements maps a class to the elements it accepts and drops, each
	// to the one text it may hold.
	droppedElements map[string]map[string]string
	// anonymousTypeNames maps the path of an anonymous ITS-XML schema type,
	// "<schema type>/<element path>", to the class named for it.
	anonymousTypeNames map[string]string
	// schemaForms maps a class and one of its properties to the form the
	// schema gives the property's element.
	schemaForms map[string]map[string]schemaForm
}

// leftOutSet returns the classes x leaves out, each of which schema, the
// target's merged schema, must declare once.
func (x sdkExtensions) leftOutSet(schema *bmm.Schema) (map[string]bool, error) {
	if len(x.ClassesLeftOut) == 0 {
		return nil, nil
	}
	set := make(map[string]bool, len(x.ClassesLeftOut))
	var errs []error
	for _, e := range x.ClassesLeftOut {
		_, isClass := schema.ClassDefinitions[e.Class]
		_, isPrimitive := schema.PrimitiveTypes[e.Class]
		switch {
		case set[e.Class]:
			errs = append(errs, fmt.Errorf("bmmgen: %s.classes_left_out leaves out %s twice", sdkExtensionsMember, e.Class))
		case !isClass && !isPrimitive:
			errs = append(errs, fmt.Errorf("bmmgen: %s.classes_left_out names %s, which no schema of the merge declares", sdkExtensionsMember, e.Class))
		}
		set[e.Class] = true
	}
	return set, errors.Join(errs...)
}

// applyExtensions checks x against p, whose classes are planned, and keeps it
// in p.extensions by subject. Every class an entry names but a type alias must
// be one the plan holds, so a class left out is named by no other entry, and a
// class the target emits must not name a class left out, which has no Go type.
// A subject given twice is an error.
func (p *Plan) applyExtensions(x sdkExtensions) error {
	var errs []error
	fail := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("bmmgen: "+sdkExtensionsMember+"."+format, args...))
	}
	held := func(key, class string) bool {
		if _, ok := p.Classes[class]; ok {
			return true
		}
		fail("%s names %s, which the target does not hold", key, class)
		return false
	}

	if len(p.extensions.leftOut) > 0 {
		for _, name := range sortedStringKeys(p.Classes) {
			pc := p.Classes[name]
			if pc.External {
				continue
			}
			form, err := classForm(pc.Class)
			if err != nil {
				return fmt.Errorf("bmmgen: %s: %w", name, err)
			}
			for _, used := range typeNamesIn(form) {
				if p.extensions.leftOut[used] {
					fail("classes_left_out: %s names %s, which the target leaves out", name, used)
				}
			}
		}
	}

	for _, e := range x.TypeAliases {
		_, isClass := p.Schema.ClassDefinitions[e.Alias]
		_, isPrimitive := p.Schema.PrimitiveTypes[e.Alias]
		switch {
		case p.extensions.typeAliases[e.Alias] != "":
			fail("type_aliases gives the alias %s twice", e.Alias)
			continue
		case isClass || isPrimitive:
			fail("type_aliases gives the alias %s, which a schema declares", e.Alias)
			continue
		}
		if held("type_aliases", e.Class) {
			if p.extensions.typeAliases == nil {
				p.extensions.typeAliases = map[string]string{}
			}
			p.extensions.typeAliases[e.Alias] = e.Class
		}
	}

	for _, e := range x.DroppedElements {
		if _, twice := p.extensions.droppedElements[e.Class][e.Element]; twice {
			fail("dropped_elements drops %s.%s twice", e.Class, e.Element)
			continue
		}
		if !held("dropped_elements", e.Class) {
			continue
		}
		if p.extensions.droppedElements == nil {
			p.extensions.droppedElements = map[string]map[string]string{}
		}
		if p.extensions.droppedElements[e.Class] == nil {
			p.extensions.droppedElements[e.Class] = map[string]string{}
		}
		p.extensions.droppedElements[e.Class][e.Element] = e.Text
	}

	namedFor := map[string]string{} // class → the path it names
	for _, e := range x.AnonymousTypeNames {
		path := e.SchemaType + "/" + e.ElementPath
		if _, twice := p.extensions.anonymousTypeNames[path]; twice {
			fail("anonymous_type_names names %s twice", path)
			continue
		}
		if other, ok := namedFor[e.Class]; ok {
			fail("anonymous_type_names names %s for %s and for %s", e.Class, other, path)
			continue
		}
		if !held("anonymous_type_names", e.Class) {
			continue
		}
		if p.extensions.anonymousTypeNames == nil {
			p.extensions.anonymousTypeNames = map[string]string{}
		}
		p.extensions.anonymousTypeNames[path] = e.Class
		namedFor[e.Class] = path
	}

	for _, e := range x.SchemaForms {
		if _, twice := p.extensions.schemaForms[e.Class][e.Property]; twice {
			fail("schema_forms gives %s.%s twice", e.Class, e.Property)
			continue
		}
		ok := held("schema_forms", e.Class)
		if ok {
			if props, _ := effectiveProperties(p, e.Class); props != nil {
				if _, has := props[e.Property]; !has {
					fail("schema_forms: %s has no property %s", e.Class, e.Property)
					ok = false
				}
			}
		}
		for _, shape := range e.Shapes {
			ok = held("schema_forms", shape) && ok
		}
		if !ok {
			continue
		}
		if p.extensions.schemaForms == nil {
			p.extensions.schemaForms = map[string]map[string]schemaForm{}
		}
		if p.extensions.schemaForms[e.Class] == nil {
			p.extensions.schemaForms[e.Class] = map[string]schemaForm{}
		}
		p.extensions.schemaForms[e.Class][e.Property] = e
	}
	return errors.Join(errs...)
}

// classForm returns c in its P_BMM JSON form, decoded into maps and lists.
func classForm(c bmm.Class) (map[string]any, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	var form map[string]any
	if err := json.Unmarshal(b, &form); err != nil {
		return nil, err
	}
	return form, nil
}

// typeNamesIn lists every type name a class form uses: its ancestors, the
// bounds of its generic parameters, and the types of its properties and of
// its functions' parameters and results.
func typeNamesIn(form map[string]any) []string {
	var names []string
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for _, k := range slices.Sorted(maps.Keys(x)) {
				switch k {
				case "documentation", "invariants", "pre_conditions", "post_conditions", "item_names", "item_documentations", "item_values", "aliases":
					continue
				case "type", "root_type", "container_type", "conforms_to_type":
					if s, ok := x[k].(string); ok {
						names = append(names, s)
						continue
					}
				case "ancestors", "generic_parameters":
					items, _ := x[k].([]any)
					for _, item := range items {
						if s, ok := item.(string); ok {
							names = append(names, s)
						} else {
							walk(item)
						}
					}
					continue
				}
				walk(x[k])
			}
		case []any:
			for _, item := range x {
				walk(item)
			}
		}
	}
	walk(form)
	return names
}
