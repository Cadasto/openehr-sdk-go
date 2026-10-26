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
)

// xsdNamespace is the namespace of the XML Schema vocabulary.
const xsdNamespace = "http://www.w3.org/2001/XMLSchema"

// The two sets of ITS-XML schemas the XML encoders take their child order
// from, each named by the files its closure starts from, relative to the
// ITS-XML directory. A file a root includes, imports or redefines joins the
// set it was reached from.
var (
	// itsXMLRMRoots are the RM and BASE schemas of Release-1.1.0.
	itsXMLRMRoots = []string{
		"components/RM/Release-1.1.0/*.xsd",
		"components/BASE/Release-1.1.0/*.xsd",
	}
	// itsXMLAMRoots are the AM schemas of Release-1.4: the archetype model
	// and the openEHR archetype profile.
	itsXMLAMRoots = []string{
		"components/AM/Release-1.4/Archetype.xsd",
		"components/AM/Release-1.4/OpenehrProfile.xsd",
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

// xsdType is one named complex type: the type it extends and the particle
// that holds its own elements.
type xsdType struct {
	file string
	// base is the local name of the xs:extension base, or "" when the type
	// extends nothing or a built-in type.
	base string
	// content is the type's own particle (xs:sequence, xs:choice, xs:all or
	// xs:group), or nil when it declares no element of its own.
	content *xsdNode
}

// xsdSet is the complex types and model groups of one closure of schema
// files, by name.
type xsdSet struct {
	types  map[string]*xsdType
	groups map[string]*xsdNode
	// dups holds, per type name, the declarations after the first, which
	// [loadXSDSet] checks agree with the first.
	dups map[string][]*xsdType
}

// loadXSDSet reads the schemas the root patterns match under root, and every
// file they include, import or redefine, transitively. A pattern is a
// [path.Match] pattern relative to root that must match at least one file.
// A type two files declare must give the same element sequence in both.
func loadXSDSet(root *os.Root, roots []string) (*xsdSet, error) {
	s := &xsdSet{
		types:  make(map[string]*xsdType),
		groups: make(map[string]*xsdNode),
		dups:   make(map[string][]*xsdType),
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
		for _, t := range later {
			got, err := s.sequenceOf(t, nil)
			if err != nil {
				return nil, err
			}
			if !slices.Equal(got, want) {
				return nil, fmt.Errorf("its-xml: type %s has the sequence %v in %s and %v in %s",
					name, want, s.types[name].file, got, t.file)
			}
		}
	}
	return s, nil
}

// readFile declares the top-level complex types and groups of one schema
// file and returns the files it references, as paths relative to the root.
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
			name := n.attr("name")
			if _, dup := s.types[name]; dup {
				s.dups[name] = append(s.dups[name], t)
				continue
			}
			s.types[name] = t
		case n.is("group"):
			content := particleOf(n)
			if content == nil {
				return nil, fmt.Errorf("its-xml: %s: group %s has no particle", file, n.attr("name"))
			}
			s.groups[n.attr("name")] = content
		}
	}
	return refs, nil
}

// parseComplexType reads the base and the own particle of a named complex
// type. A construct the reader does not know fails, so a re-pinned schema
// that uses one cannot be read into a wrong order.
func parseComplexType(file string, n *xsdNode) (*xsdType, error) {
	t := &xsdType{file: file}
	for i := range n.Children {
		c := &n.Children[i]
		switch {
		case c.is("annotation"), c.is("attribute"), c.is("attributeGroup"), c.is("anyAttribute"):
		case c.is("sequence"), c.is("choice"), c.is("all"), c.is("group"):
			t.content = c
		case c.is("simpleContent"):
			// Text content with attributes: no child element.
		case c.is("complexContent"):
			for j := range c.Children {
				d := &c.Children[j]
				switch {
				case d.is("annotation"):
				case d.is("extension"):
					t.base = schemaLocalName(d.attr("base"))
					t.content = particleOf(d)
				case d.is("restriction"):
					// A restriction restates the content it keeps.
					t.content = particleOf(d)
				default:
					return nil, fmt.Errorf("its-xml: %s: type %s: unexpected xs:%s in xs:complexContent", file, n.attr("name"), d.XMLName.Local)
				}
			}
		default:
			return nil, fmt.Errorf("its-xml: %s: type %s: unexpected xs:%s", file, n.attr("name"), c.XMLName.Local)
		}
	}
	return t, nil
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
	if t.base != "" {
		if slices.Contains(visiting, t.base) {
			return nil, fmt.Errorf("its-xml: type %s extends itself", t.base)
		}
		base, ok := s.types[t.base]
		if !ok {
			return nil, fmt.Errorf("its-xml: %s: base type %s is not declared", t.file, t.base)
		}
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

// sequenceFor returns the element names whose order class's encoder takes
// from the schemas. The class takes the type of its own name, or of its
// nearest ancestor, from the first set that declares one; the type's
// extension bases come first. When another set declares a type of the same
// name, each element only that set declares is placed right after the
// nearest earlier element both declare. A nil result means no set declares
// the class or an ancestor.
func (o *xmlChildOrder) sequenceFor(plan *Plan, class string) ([]string, error) {
	lineage := nearestLineage(plan, class)
	for i, set := range o.sets {
		for _, name := range lineage {
			if !set.has(name) {
				continue
			}
			seq, err := set.sequence(name)
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
	}
	return nil, nil
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
