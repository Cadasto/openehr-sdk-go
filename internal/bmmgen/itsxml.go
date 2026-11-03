package bmmgen

import (
	"cmp"
	"encoding/xml"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// xsdNamespace is the namespace of the XML Schema vocabulary.
const xsdNamespace = "http://www.w3.org/2001/XMLSchema"

// The two sets of ITS-XML schemas the XML encoders take their child order
// from, each named by the files its closure starts from, relative to the
// ITS-XML directory. A file a root includes, imports or redefines joins the
// set it was reached from.
var (
	// itsXMLRMRoots are the RM schemas of Release-1.1.0 and the BASE
	// schemas of Release-1.2.0.
	itsXMLRMRoots = []string{
		"components/RM/Release-1.1.0/*.xsd",
		"components/BASE/Release-1.2.0/*.xsd",
	}
	// itsXMLAMRoots are the AM schemas of Release-1.4: the archetype model,
	// the openEHR archetype profile and the operational template.
	itsXMLAMRoots = []string{
		"components/AM/Release-1.4/Archetype.xsd",
		"components/AM/Release-1.4/OpenehrProfile.xsd",
		"components/AM/Release-1.4/Template.xsd",
	}
)

// xsdNode is one element of a schema document, kept generic: its name, its
// attributes and its child elements. Text content is not read.
type xsdNode struct {
	XMLName  xml.Name
	Attrs    []xml.Attr `xml:",any,attr"`
	Children []xsdNode  `xml:",any"`
}

// attr returns the value of the unqualified attribute name, or "".
func (n *xsdNode) attr(name string) string {
	for _, a := range n.Attrs {
		if a.Name.Space == "" && a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

// is reports whether n is the XML Schema element local.
func (n *xsdNode) is(local string) bool {
	return n.XMLName.Space == xsdNamespace && n.XMLName.Local == local
}

// xsdType is one named complex type: the type it extends, the particle that
// holds its own elements and the XML attributes it declares itself.
type xsdType struct {
	file string
	name string
	// base is the local name of the xs:extension base, or "" when the type
	// extends nothing or a built-in type.
	base string
	// simple reports a type with simple content: text and attributes, no
	// child element. Its base may be a simple type, which declares neither.
	simple bool
	// restricts is the local name of the type an xs:restriction derives the
	// type from, or "" when there is none or it is a built-in type.
	restricts string
	// content is the type's own particle (xs:sequence, xs:choice, xs:all or
	// xs:group), or nil when it declares no element of its own.
	content *xsdNode
	// attrs names the XML attributes the type declares itself, in document
	// order, on the type or in its xs:extension or xs:restriction.
	attrs []string
}

// xsdSet is the complex types, the simple type names and the model groups of
// one closure of schema files, by name. An anonymous complex type, the type of
// an element declared inside a named type, is among the types under its path:
// the named type's name and the local names of the elements down to it,
// separated by "/", such as "T_VIEW/constraints/items", which no type name can
// be.
type xsdSet struct {
	types       map[string]*xsdType
	simpleTypes map[string]bool
	groups      map[string]*xsdNode
	// dups holds, per type name, the declarations after the first, which
	// [loadXSDSet] checks agree with the first.
	dups map[string][]*xsdType
	// globals maps a type name to the global element a schema file declares
	// with that type: the root element of a document of that type.
	globals map[string]string
}

// loadXSDSet reads the schemas the root patterns match under root, and every
// file they include, import or redefine, transitively. A pattern is a
// [path.Match] pattern relative to root that must match at least one file.
// A type two files declare must give the same element sequence and the same
// XML attributes in both.
func loadXSDSet(root *os.Root, roots []string) (*xsdSet, error) {
	s := &xsdSet{
		types:       make(map[string]*xsdType),
		simpleTypes: make(map[string]bool),
		groups:      make(map[string]*xsdNode),
		dups:        make(map[string][]*xsdType),
		globals:     make(map[string]string),
	}
	var queue []string
	for _, pattern := range roots {
		matches, err := fs.Glob(root.FS(), pattern)
		if err != nil {
			return nil, fmt.Errorf("its-xml: %s: %w", pattern, err)
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("its-xml: no schema matches %s under %s", pattern, root.Name())
		}
		queue = append(queue, matches...)
	}
	seen := make(map[string]bool)
	for i := 0; i < len(queue); i++ {
		file := queue[i]
		if seen[file] {
			continue
		}
		seen[file] = true
		refs, err := s.readFile(root, file)
		if err != nil {
			return nil, err
		}
		queue = append(queue, refs...)
	}
	for name, later := range s.dups {
		want, err := s.sequenceOf(s.types[name], nil)
		if err != nil {
			return nil, err
		}
		wantAttrs, err := s.attributesOf(s.types[name], nil)
		if err != nil {
			return nil, err
		}
		for _, t := range later {
			got, err := s.sequenceOf(t, nil)
			if err != nil {
				return nil, err
			}
			if !slices.Equal(got, want) {
				return nil, fmt.Errorf("its-xml: type %s has the sequence %v in %s and %v in %s",
					name, want, s.types[name].file, got, t.file)
			}
			gotAttrs, err := s.attributesOf(t, nil)
			if err != nil {
				return nil, err
			}
			if !slices.Equal(gotAttrs, wantAttrs) {
				return nil, fmt.Errorf("its-xml: type %s has the attributes %v in %s and %v in %s",
					name, wantAttrs, s.types[name].file, gotAttrs, t.file)
			}
		}
	}
	return s, nil
}

// readFile declares the top-level complex types, groups and elements of one
// schema file and returns the files it references, as paths relative to the
// root. A global element is read by its name and its type; one with an
// anonymous type, or a second of another name for a type, is an error.
func (s *xsdSet) readFile(root *os.Root, file string) ([]string, error) {
	raw, err := root.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("its-xml: %w", err)
	}
	var doc xsdNode
	if err := xml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("its-xml: %s: %w", file, err)
	}
	if !doc.is("schema") {
		return nil, fmt.Errorf("its-xml: %s: root element is %s, want xs:schema", file, doc.XMLName.Local)
	}
	var refs []string
	for i := range doc.Children {
		n := &doc.Children[i]
		switch {
		case n.is("include"), n.is("import"), n.is("redefine"):
			if n.is("redefine") && len(n.Children) > 0 {
				return nil, fmt.Errorf("its-xml: %s: xs:redefine with redefinitions is not supported", file)
			}
			if loc := n.attr("schemaLocation"); loc != "" {
				refs = append(refs, path.Join(path.Dir(file), loc))
			}
		case n.is("complexType"):
			t, err := parseComplexType(file, n)
			if err != nil {
				return nil, err
			}
			if err := s.declare(t); err != nil {
				return nil, err
			}
		case n.is("group"):
			content := particleOf(n)
			if content == nil {
				return nil, fmt.Errorf("its-xml: %s: group %s has no particle", file, n.attr("name"))
			}
			s.groups[n.attr("name")] = content
		case n.is("simpleType"):
			s.simpleTypes[n.attr("name")] = true
		case n.is("element"):
			name, typ := n.attr("name"), schemaLocalName(n.attr("type"))
			if name == "" || typ == "" {
				return nil, fmt.Errorf("its-xml: %s: a global element needs a name and a named type", file)
			}
			if other, dup := s.globals[typ]; dup && other != name {
				return nil, fmt.Errorf("its-xml: %s: the global elements %s and %s both have the type %s", file, other, name, typ)
			}
			s.globals[typ] = name
		}
	}
	return refs, nil
}

// declare adds t, a complex type read from a schema file, to the set under its
// name, and each anonymous complex type its own particle declares, at any
// depth, under its path. A name declared before keeps its first declaration;
// [loadXSDSet] checks that the later ones agree with it.
func (s *xsdSet) declare(t *xsdType) error {
	if _, dup := s.types[t.name]; dup {
		s.dups[t.name] = append(s.dups[t.name], t)
	} else {
		s.types[t.name] = t
	}
	if t.content == nil {
		return nil
	}
	return s.declareAnonymous(t.file, t.name, t.content)
}

// declareAnonymous declares the anonymous complex type of each element n, a
// particle of the type at path, declares, under path and the element's name.
func (s *xsdSet) declareAnonymous(file, path string, n *xsdNode) error {
	switch {
	case n.is("element"):
		for i := range n.Children {
			c := &n.Children[i]
			if !c.is("complexType") {
				continue
			}
			t, err := parseComplexType(file, c)
			if err != nil {
				return err
			}
			t.name = path + "/" + n.attr("name")
			return s.declare(t)
		}
	case n.is("sequence"), n.is("choice"), n.is("all"):
		for i := range n.Children {
			if err := s.declareAnonymous(file, path, &n.Children[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

// parseComplexType reads the base, the own particle and the own XML
// attributes of a named complex type. A construct the reader does not know
// fails, so a re-pinned schema that uses one cannot be read into a wrong
// order or a wrong set of attributes: an xs:attributeGroup among them, which
// the reader does not expand, and an xs:anyAttribute, which admits attributes
// no member declares.
func parseComplexType(file string, n *xsdNode) (*xsdType, error) {
	t := &xsdType{file: file, name: n.attr("name")}
	for i := range n.Children {
		c := &n.Children[i]
		switch {
		case c.is("annotation"):
		case c.is("attribute"):
			if err := t.addAttribute(c); err != nil {
				return nil, err
			}
		case c.is("sequence"), c.is("choice"), c.is("all"), c.is("group"):
			t.content = c
		case c.is("simpleContent"), c.is("complexContent"):
			// Simple content is text and attributes: no child element.
			t.simple = c.is("simpleContent")
			if err := t.parseDerivation(c); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("its-xml: %s: type %s: unexpected xs:%s", file, t.name, c.XMLName.Local)
		}
	}
	return t, nil
}

// parseDerivation reads the xs:extension or xs:restriction that c, an
// xs:simpleContent or xs:complexContent, holds: its base, its particle and
// its attributes. An extension adds them to its base's; a restriction
// restates the content it keeps, and its base's attributes are not read
// through it ([xsdSet.attributesOf]). An xs:attributeGroup or an
// xs:anyAttribute fails in either.
func (t *xsdType) parseDerivation(c *xsdNode) error {
	for j := range c.Children {
		d := &c.Children[j]
		switch {
		case d.is("annotation"):
		case d.is("extension"):
			t.base = schemaLocalName(d.attr("base"))
			for k := range d.Children {
				e := &d.Children[k]
				switch {
				case e.is("annotation"):
				case e.is("attribute"):
					if err := t.addAttribute(e); err != nil {
						return err
					}
				case !t.simple && (e.is("sequence") || e.is("choice") || e.is("all") || e.is("group")):
					t.content = e
				default:
					return fmt.Errorf("its-xml: %s: type %s: unexpected xs:%s in xs:extension", t.file, t.name, e.XMLName.Local)
				}
			}
		case d.is("restriction"):
			t.restricts = schemaLocalName(d.attr("base"))
			t.content = particleOf(d)
			for k := range d.Children {
				e := &d.Children[k]
				switch {
				case e.is("attribute"):
					if err := t.addAttribute(e); err != nil {
						return err
					}
				case e.is("attributeGroup"), e.is("anyAttribute"):
					// A restriction of a built-in type inherits nothing for
					// attributesOf to refuse, so the group the reader does
					// not expand, or the wildcard, is refused here.
					return fmt.Errorf("its-xml: %s: type %s: unexpected xs:%s in xs:restriction", t.file, t.name, e.XMLName.Local)
				}
			}
		default:
			return fmt.Errorf("its-xml: %s: type %s: unexpected xs:%s in xs:%s", t.file, t.name, d.XMLName.Local, c.XMLName.Local)
		}
	}
	return nil
}

// addAttribute records the XML attribute n declares. An attribute by
// reference names no attribute of the type's own vocabulary and is refused.
// So is a prohibited one, which declares no attribute and in a restriction
// takes away an inherited one, which the reader does not do.
func (t *xsdType) addAttribute(n *xsdNode) error {
	name := n.attr("name")
	if name == "" {
		return fmt.Errorf("its-xml: %s: type %s: an attribute has no name", t.file, t.name)
	}
	if n.attr("use") == "prohibited" {
		return fmt.Errorf("its-xml: %s: type %s: attribute %s is prohibited", t.file, t.name, name)
	}
	if !slices.Contains(t.attrs, name) {
		t.attrs = append(t.attrs, name)
	}
	return nil
}

// particleOf returns the first particle child of n, or nil.
func particleOf(n *xsdNode) *xsdNode {
	for i := range n.Children {
		c := &n.Children[i]
		if c.is("sequence") || c.is("choice") || c.is("all") || c.is("group") {
			return c
		}
	}
	return nil
}

// schemaLocalName strips a namespace prefix from a QName. A name in the XML
// Schema namespace's own prefix ("xs:string") is a built-in type and yields "".
func schemaLocalName(qname string) string {
	prefix, local, ok := strings.Cut(qname, ":")
	if !ok {
		return qname
	}
	if prefix == "xs" || prefix == "xsd" {
		return ""
	}
	return local
}

// has reports whether the set declares a complex type of that name.
func (s *xsdSet) has(name string) bool {
	_, ok := s.types[name]
	return ok
}

// sequence returns the element names of the named type in schema order: its
// extension bases' elements first, then its own. Within a particle the names
// follow document order, an xs:choice's branches one after another, and a
// name that occurs twice keeps its first place.
func (s *xsdSet) sequence(name string) ([]string, error) {
	t, ok := s.types[name]
	if !ok {
		return nil, fmt.Errorf("its-xml: no complex type %s", name)
	}
	return s.sequenceOf(t, nil)
}

func (s *xsdSet) sequenceOf(t *xsdType, visiting []string) ([]string, error) {
	var out []string
	base, err := s.complexBase(t, visiting)
	if err != nil {
		return nil, err
	}
	if base != nil {
		names, err := s.sequenceOf(base, append(visiting, t.base))
		if err != nil {
			return nil, err
		}
		out = names
	}
	if t.content == nil {
		return out, nil
	}
	return s.appendParticle(out, t.content, nil)
}

// complexBase returns the complex type t extends, or nil when t extends
// nothing, a built-in type or, with simple content, a simple type. A base
// that is declared nowhere, or that visiting already holds, is an error.
func (s *xsdSet) complexBase(t *xsdType, visiting []string) (*xsdType, error) {
	if t.base == "" {
		return nil, nil
	}
	if slices.Contains(visiting, t.base) {
		return nil, fmt.Errorf("its-xml: type %s extends itself", t.base)
	}
	if base, ok := s.types[t.base]; ok {
		return base, nil
	}
	if t.simple && s.simpleTypes[t.base] {
		return nil, nil
	}
	return nil, fmt.Errorf("its-xml: %s: base type %s is not declared", t.file, t.base)
}

// attributes returns the names of the XML attributes of the named type: its
// extension bases' first, then its own, each in document order.
func (s *xsdSet) attributes(name string) ([]string, error) {
	t, ok := s.types[name]
	if !ok {
		return nil, fmt.Errorf("its-xml: no complex type %s", name)
	}
	return s.attributesOf(t, nil)
}

// attributesOf returns the XML attribute names of t, its extension bases'
// first. A type derived by restriction from a declared type inherits the
// attributes it does not restate, which the reader does not work out: such a
// type is refused rather than given only the attributes it restates.
func (s *xsdSet) attributesOf(t *xsdType, visiting []string) ([]string, error) {
	if t.restricts != "" {
		return nil, fmt.Errorf("its-xml: %s: type %s restricts %s, and the reader does not derive attributes through a restriction", t.file, t.name, t.restricts)
	}
	var out []string
	base, err := s.complexBase(t, visiting)
	if err != nil {
		return nil, err
	}
	if base != nil {
		names, err := s.attributesOf(base, append(visiting, t.base))
		if err != nil {
			return nil, err
		}
		out = names
	}
	for _, name := range t.attrs {
		if !slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	return out, nil
}

// appendParticle appends the element names of particle n to out.
func (s *xsdSet) appendParticle(out []string, n *xsdNode, groups []string) ([]string, error) {
	switch {
	case n.is("element"):
		name := n.attr("name")
		if name == "" {
			name = schemaLocalName(n.attr("ref"))
		}
		if name == "" {
			return nil, errors.New("its-xml: an element has neither a name nor a ref")
		}
		if !slices.Contains(out, name) {
			out = append(out, name)
		}
		return out, nil
	case n.is("sequence"), n.is("choice"), n.is("all"):
		var err error
		for i := range n.Children {
			c := &n.Children[i]
			if c.is("annotation") || c.is("any") {
				continue
			}
			if out, err = s.appendParticle(out, c, groups); err != nil {
				return nil, err
			}
		}
		return out, nil
	case n.is("group"):
		ref := schemaLocalName(n.attr("ref"))
		if slices.Contains(groups, ref) {
			return nil, fmt.Errorf("its-xml: group %s refers to itself", ref)
		}
		g, ok := s.groups[ref]
		if !ok {
			return nil, fmt.Errorf("its-xml: group %s is not declared", ref)
		}
		return s.appendParticle(out, g, append(groups, ref))
	default:
		return nil, fmt.Errorf("its-xml: unexpected xs:%s in a content model", n.XMLName.Local)
	}
}

// elementType returns the local name of the type the named type declares for
// its child element elem, in its own particle or its extension bases', and
// false when it declares no such element or gives it no named type.
func (s *xsdSet) elementType(name, elem string) (string, bool) {
	visiting := []string{}
	for t, ok := s.types[name]; ok && !slices.Contains(visiting, t.name); t, ok = s.types[t.base] {
		visiting = append(visiting, t.name)
		if t.content == nil {
			continue
		}
		if n := s.findElement(t.content, elem, nil); n != nil {
			typ := schemaLocalName(n.attr("type"))
			return typ, typ != ""
		}
	}
	return "", false
}

// findElement returns the first declaration of the element elem in particle n,
// at any depth, through the groups it refers to, or nil.
func (s *xsdSet) findElement(n *xsdNode, elem string, groups []string) *xsdNode {
	switch {
	case n.is("element"):
		if n.attr("name") == elem {
			return n
		}
	case n.is("sequence"), n.is("choice"), n.is("all"):
		for i := range n.Children {
			if found := s.findElement(&n.Children[i], elem, groups); found != nil {
				return found
			}
		}
	case n.is("group"):
		ref := schemaLocalName(n.attr("ref"))
		if g, ok := s.groups[ref]; ok && !slices.Contains(groups, ref) {
			return s.findElement(g, elem, append(groups, ref))
		}
	}
	return nil
}

// optionalElements returns the names of the elements the named type's own
// particle declares optional: each declaration of the element has
// minOccurs="0", on itself or on a particle around it. The branches of an
// xs:choice count each as a particle of its own, so an element a branch
// requires is not optional, although the choice may take another branch.
func (s *xsdSet) optionalElements(name string) []string {
	t, ok := s.types[name]
	if !ok || t.content == nil {
		return nil
	}
	optional := map[string]bool{}
	var walk func(n *xsdNode, around bool, groups []string)
	walk = func(n *xsdNode, around bool, groups []string) {
		around = around || n.attr("minOccurs") == "0"
		switch {
		case n.is("element"):
			name := n.attr("name")
			if seen, ok := optional[name]; ok {
				optional[name] = seen && around
			} else {
				optional[name] = around
			}
		case n.is("sequence"), n.is("choice"), n.is("all"):
			for i := range n.Children {
				walk(&n.Children[i], around, groups)
			}
		case n.is("group"):
			ref := schemaLocalName(n.attr("ref"))
			if g, ok := s.groups[ref]; ok && !slices.Contains(groups, ref) {
				walk(g, around, append(groups, ref))
			}
		}
	}
	walk(t.content, false, nil)
	var out []string
	for _, name := range sortedStringKeys(optional) {
		if optional[name] {
			out = append(out, name)
		}
	}
	return out
}

// xmlChildOrder gives one target's XML encoders their child order, from the
// ITS-XML schema sets in the target's lookup order.
type xmlChildOrder struct {
	sets []*xsdSet
}

// itsXMLSchemas reads each schema set once per generator run: the targets
// share the two sets, in different lookup orders.
type itsXMLSchemas struct {
	dir  string
	sets map[string]*xsdSet
}

// orderFor returns the child order for a target that names the schema sets
// in roots, each a list of root patterns, in lookup order.
func (s *itsXMLSchemas) orderFor(roots [][]string) (*xmlChildOrder, error) {
	order := &xmlChildOrder{}
	var root *os.Root
	defer func() {
		if root != nil {
			_ = root.Close() // read-only handle
		}
	}()
	for _, patterns := range roots {
		key := strings.Join(patterns, "\n")
		set, ok := s.sets[key]
		if !ok {
			if root == nil {
				r, err := os.OpenRoot(s.dir)
				if err != nil {
					return nil, fmt.Errorf("its-xml: %w", err)
				}
				root = r
			}
			loaded, err := loadXSDSet(root, patterns)
			if err != nil {
				return nil, err
			}
			if s.sets == nil {
				s.sets = make(map[string]*xsdSet)
			}
			s.sets[key] = loaded
			set = loaded
		}
		order.sets = append(order.sets, set)
	}
	return order, nil
}

// typeFor returns the set that declares the schema type class takes and the
// type's name: the type of the class's own name, or of its nearest ancestor,
// from the first set that declares one. A class the target's roots name for an
// anonymous schema type, in their anonymous_type_names, takes the type at that
// path in place of a type of its own name. ok is false when no set declares the
// class or an ancestor.
func (o *xmlChildOrder) typeFor(plan *Plan, class string) (index int, name string, ok bool) {
	lineage := nearestLineage(plan, class)
	for j, c := range lineage {
		if path, named := plan.extensions.anonymousTypeOf[c]; named {
			lineage[j] = path
		}
	}
	for i, set := range o.sets {
		for _, name := range lineage {
			if set.has(name) {
				return i, name, true
			}
		}
	}
	return 0, "", false
}

// checkAnonymousTypes checks each path the target's roots name in their
// anonymous_type_names against the schema sets: the path must be that of an
// anonymous complex type one of them declares, so the class named for it
// takes that type's elements and attributes.
func (o *xmlChildOrder) checkAnonymousTypes(plan *Plan) error {
	var errs []error
	for _, path := range sortedStringKeys(plan.extensions.anonymousTypeNames) {
		if !slices.ContainsFunc(o.sets, func(set *xsdSet) bool { return set.has(path) }) {
			errs = append(errs, fmt.Errorf("bmmgen: %s.anonymous_type_names names %s, which no ITS-XML schema of the target declares as an anonymous type", sdkExtensionsMember, path))
		}
	}
	return errors.Join(errs...)
}

// sequenceFor returns the element names whose order class's encoder takes
// from the schemas. The class takes its type as [xmlChildOrder.typeFor]
// finds it; the type's extension bases come first. When another set
// declares a type of the same name, each element only that set declares is
// placed right after the nearest earlier element both declare. A nil result
// means no set declares the class or an ancestor.
func (o *xmlChildOrder) sequenceFor(plan *Plan, class string) ([]string, error) {
	i, name, ok := o.typeFor(plan, class)
	if !ok {
		return nil, nil
	}
	seq, err := o.sets[i].sequence(name)
	if err != nil {
		return nil, err
	}
	for j, other := range o.sets {
		if j == i || !other.has(name) {
			continue
		}
		otherSeq, err := other.sequence(name)
		if err != nil {
			return nil, err
		}
		seq = placeOtherOnly(seq, otherSeq)
	}
	return seq, nil
}

// attributesFor returns the names of the XML attributes of the schema type
// class takes, as [xmlChildOrder.typeFor] finds it, its extension bases'
// first, that every other set declaring a type of the same name declares too.
// Another set's type never adds an attribute, and one it lacks is dropped: a
// class two sets declare with different attributes takes those both agree on,
// so every target, whatever order it looks the sets up in, gives a class the
// same attributes. That gives the resource classes the AM Release-1.4 shape
// in both targets: BASE Release-1.2.0 declares an `id` on
// RESOURCE_DESCRIPTION_ITEM and TRANSLATION_DETAILS that AM Release-1.4 does
// not, so the Hash holding them is keyed by their language. A nil result means
// the type declares none, or no set declares the class or an ancestor.
func (o *xmlChildOrder) attributesFor(plan *Plan, class string) ([]string, error) {
	i, name, ok := o.typeFor(plan, class)
	if !ok {
		return nil, nil
	}
	attrs, err := o.sets[i].attributes(name)
	if err != nil {
		return nil, err
	}
	for j, other := range o.sets {
		if j == i || !other.has(name) {
			continue
		}
		agreed, err := other.attributes(name)
		if err != nil {
			return nil, err
		}
		attrs = slices.DeleteFunc(attrs, func(a string) bool { return !slices.Contains(agreed, a) })
	}
	return attrs, nil
}

// rootElementFor returns the global element the schemas declare with the type
// of class's own name, from the first set that declares one, or false when
// none does. A class that takes an ancestor's type takes no root element from
// it.
func (o *xmlChildOrder) rootElementFor(class string) (string, bool) {
	for _, set := range o.sets {
		if element, ok := set.globals[class]; ok {
			return element, true
		}
	}
	return "", false
}

// optionalChildrenOf returns the child elements the schemas make optional in
// the type of the element elem of class: the type class's schema type, as
// [xmlChildOrder.typeFor] finds it, declares for elem, read from the set that
// declares class's type. It returns nil when the schemas declare no such
// element or give it no named type.
func (o *xmlChildOrder) optionalChildrenOf(plan *Plan, class, elem string) []string {
	i, name, ok := o.typeFor(plan, class)
	if !ok {
		return nil
	}
	typ, ok := o.sets[i].elementType(name, elem)
	if !ok {
		return nil
	}
	return o.sets[i].optionalElements(typ)
}

// nearestLineage returns class and every class it descends from, nearest
// first: its parents in declaration order, then theirs. A generic ancestor
// is named without its type arguments.
func nearestLineage(plan *Plan, class string) []string {
	out := []string{class}
	for i := 0; i < len(out); i++ {
		c, ok := declaredClass(plan, out[i])
		if !ok {
			continue
		}
		for _, anc := range c.Ancestors() {
			anc, _, _ = strings.Cut(anc, "<")
			if !slices.Contains(out, anc) {
				out = append(out, anc)
			}
		}
	}
	return out
}

// placeOtherOnly returns own with each name only other declares inserted
// right after the nearest earlier name of other that is already placed, or
// first when there is none. Names keep other's order among themselves.
func placeOtherOnly(own, other []string) []string {
	out := slices.Clone(own)
	pos := 0
	for _, name := range other {
		if i := slices.Index(out, name); i >= 0 {
			pos = i + 1
			continue
		}
		out = slices.Insert(out, pos, name)
		pos++
	}
	return out
}

// xmlChildFields returns fields in the order pc's XML encoder writes them:
// the ones its schema sequence names in that order, then the rest in the
// order given. Without schemas the order given stands.
func xmlChildFields(plan *Plan, pc *PlannedClass, fields []emittedField) ([]emittedField, error) {
	if plan.xmlOrder == nil {
		return fields, nil
	}
	seq, err := plan.xmlOrder.sequenceFor(plan, pc.BMMName)
	if err != nil {
		return nil, fmt.Errorf("bmmgen: XML child order of %s: %w", pc.BMMName, err)
	}
	rank := func(f emittedField) int {
		if i := slices.Index(seq, f.Prop.PropertyName()); i >= 0 {
			return i
		}
		return len(seq)
	}
	out := slices.Clone(fields)
	slices.SortStableFunc(out, func(a, b emittedField) int { return cmp.Compare(rank(a), rank(b)) })
	return out, nil
}

// errXMLAttributeNotString reports a member the ITS-XML schema type of its
// class declares as an XML attribute that the generated codec has no rule
// for: the codec writes and reads an attribute's text as a mandatory String
// member, held as a Go string.
var errXMLAttributeNotString = errors.New("bmmgen: a member the ITS-XML schema declares as an XML attribute is not a mandatory String")

// xmlAttributeSet is what the ITS-XML schema type of one class declares as
// XML attributes.
type xmlAttributeSet struct {
	// members are the attributes that name a member of the class, which the
	// class's codec writes and reads as attributes, in the type's order.
	members []string
	// keys are the attributes that name no member of the class: each is the
	// key the Hash holding the class keeps a value under, which the class's
	// own codec does not write or read.
	keys []string
}

// xmlAttributesOf returns the XML attributes the ITS-XML schema type of pc
// declares, found as [xmlChildOrder.attributesFor] finds them, split by
// whether they name one of fields, pc's members. A member it names that is
// not a mandatory String is refused with [errXMLAttributeNotString]. A plan
// built without the schemas, as a test may build one, makes no member an
// attribute.
func xmlAttributesOf(plan *Plan, pc *PlannedClass, fields []emittedField) (xmlAttributeSet, error) {
	if plan.xmlOrder == nil {
		return xmlAttributeSet{}, nil
	}
	names, err := plan.xmlOrder.attributesFor(plan, pc.BMMName)
	if err != nil {
		return xmlAttributeSet{}, fmt.Errorf("bmmgen: XML attributes of %s: %w", pc.BMMName, err)
	}
	var set xmlAttributeSet
	for _, name := range names {
		i := slices.IndexFunc(fields, func(ef emittedField) bool { return ef.Prop.PropertyName() == name })
		if i < 0 {
			set.keys = append(set.keys, name)
			continue
		}
		if !isMandatoryString(fields[i].Prop) {
			return xmlAttributeSet{}, fmt.Errorf("%w: %s.%s", errXMLAttributeNotString, pc.BMMName, name)
		}
		set.members = append(set.members, name)
	}
	return set, nil
}

// isMandatoryString reports whether prop is a mandatory single property of
// the BMM primitive String, which the generator holds as a Go string.
func isMandatoryString(prop bmm.Property) bool {
	p, ok := prop.(*bmm.SingleProperty)
	return ok && p.IsMandatory && p.TypeName == "String"
}
