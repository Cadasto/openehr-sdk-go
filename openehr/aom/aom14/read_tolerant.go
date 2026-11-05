package aom14

import (
	"bytes"
	"cmp"
	"encoding/xml"
	"errors"
	"io"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/internal/xmlschema"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// read_tolerant.go: the second reading of [ReadOperationalTemplate]. Where an
// operational template holds a shape or a value the reader tolerates and
// canonical XML does not read, the reader rewrites the document into the one
// canonical XML decodes as the reader reads it, and decodes that. The
// rewrite keeps every element at its path, so a report of unmatched content
// names the paths of the input, but for what an item it writes or takes off
// holds, which a report names by its path in the object that holds it then.

// Namespaces the tolerant reading knows beyond canxml's.
const (
	// nsOpenEHRv2 is the namespace the AM Release-1.4 schemas declare, which
	// the reader reads as canonical XML's.
	nsOpenEHRv2 = "http://schemas.openehr.org/v2"
	// nsLeftOut is the namespace an element a tolerance leaves out is moved
	// into in the default mode, so that the decode skips it as unmatched
	// content at its own path.
	nsLeftOut = "urn:x-openehr-sdk-go:left-out"
)

// byteOrderMark is the UTF-8 byte-order mark a document may start with.
var byteOrderMark = []byte{0xEF, 0xBB, 0xBF}

// role is what an element of the document is to the tolerant reading.
type role uint8

const (
	roleNone            role = iota // an element no tolerance reads
	roleRoot                        // the root element
	roleObject                      // a constraint object: the definition, or a <children> of an attribute
	roleAttribute                   // an <attributes> of a complex object
	roleItem                        // the <item> of a C_PRIMITIVE_OBJECT
	roleQuantityItem                // a <list> entry of a C_DV_QUANTITY
	roleQuantityAssumed             // the assumed_value of a C_DV_QUANTITY
	roleRange                       // the range of a C_INTEGER or C_REAL item, or the magnitude or precision of a quantity
	roleArchetypeID                 // the <archetype_id> of a <children> that names no type
	roleValue                       // an element whose text a tolerance reads
	roleLeftOut                     // an element a tolerance leaves out, with all it holds
)

// The flags of a C_BOOLEAN, as bits of element.seen.
const (
	seenTrueValid uint8 = 1 << iota
	seenFalseValid
)

// element is an element of the document the tolerant reading has opened.
type element struct {
	parent *element
	name   xml.Name   // as written: Space holds the prefix
	attrs  []xml.Attr // as written, but for the rewrites of the reading
	off    int        // the offset of its start tag
	mark   int        // the number of edits the reading had made when it opened
	tagEnd int        // the offset just past its start tag
	endTag int        // the offset of its end tag, or tagEnd when its start tag closes it
	end    int        // the offset just past it
	self   bool       // its start tag closes it
	scope  int        // the namespace bindings in scope before its own
	role   role
	// class is the class an object, an attribute or an item decodes as, and
	// for a range the class of the item it constrains, or "C_QUANTITY_ITEM".
	class  string
	typeAt int    // the index in attrs of its xsi:type, or -1
	bareAt int    // the index in attrs of a bare type attribute, or -1
	retype string // the xsi:type the reading gives it, or ""
	dirty  bool   // its start tag differs from the input: attrs, or its name
	// renamed is an element the default mode writes without the prefix of
	// its name (written), in the default namespace, which is none or
	// canonical XML's there.
	renamed bool
	// untyped is an attribute or object that names no type in the XSI
	// namespace, which the reading types when it closes.
	untyped bool
	// leaf is an object of no class of the AOM 1.4 constraint model, or of
	// one it cannot hold, read as a C_COMPLEX_OBJECT with its rm_type_name,
	// node_id and occurrences alone.
	leaf bool
	// itemLevel is, for a definition typed C_PRIMITIVE_OBJECT, 1, and for an
	// <item> such a definition holds, or such an item, one more than the
	// level of the item it is: the level of the items it holds, whose types
	// decide whether the definition carries a primitive constraint
	// (definitionItem).
	itemLevel int
	// complexTyped is an object typed C_COMPLEX_OBJECT, which the default
	// mode reads as a C_ARCHETYPE_ROOT where it holds an archetype id.
	complexTyped bool
	// wrap is an object typed with a primitive class, read as a
	// C_PRIMITIVE_OBJECT whose item is of that class and holds its members,
	// but its rm_type_name, occurrences and node_id.
	wrap bool
	// members are the elements in the openEHR namespace a wrap holds.
	members []*element
	// unwrap is an item of a class isUnwrapClass names, read as the object
	// that holds it: host, of the item's class unless it is a
	// C_PRIMITIVE_OBJECT, whose own item is then host's.
	unwrap bool
	host   *element
	// inherits is an element an item the reading takes off holds, which
	// declares what the item declares that it and what it holds use.
	inherits bool
	// uses are the prefixes an element that inherits, and what it holds, use
	// in their names, but those an element that inherits inside it uses.
	uses map[string]bool
	// declares are the namespace declarations an item the reading takes off
	// makes, by prefix, once read.
	declares    map[string]xml.Attr
	cardinality bool         // an attribute that holds a <cardinality>
	archetypeID bool         // an object that holds an archetype id
	seen        uint8        // the flags a C_BOOLEAN item holds
	sides       [2]rangeSide // the lower and upper sides of a range
	text        []byte       // the text of a value
	nested      bool         // it holds an element
	insert      []byte       // the elements the reading adds before its end tag
}

// rangeSide is one side of a range: its bound and flags, nil where the
// range does not give them.
type rangeSide struct {
	bound, unbounded, included *element
}

// edit replaces the bytes off to end of the input with text, unless an edit
// of an element that holds them has replaced it since (dead).
type edit struct {
	off, end int
	text     string
	dead     bool
}

// binding is a namespace declaration in scope.
type binding struct {
	prefix, uri string
}

// tolerantReading rewrites one document; see [tolerate].
type tolerantReading struct {
	data   []byte
	strict bool
	// scope holds the namespace declarations in scope, in the order the
	// reading met them, and bound the namespaces each prefix is bound to
	// there, the innermost last, so that a lookup costs one step.
	scope []binding
	bound map[string][]string
	open  []*element
	// inheriting holds the open elements that inherit, the innermost last.
	inheriting []*element
	edits      []edit
	xsiFresh   string // a prefix the input does not use, once one is chosen
	// failed is set where two edits a rendering makes overlap, which the
	// reading does not make. The input is then left as it is, for the decode
	// to refuse.
	failed bool
	// refusal is the refusal of a shape strict mode refuses that the decode
	// reads, or of one every mode refuses, which tolerate returns once the
	// reading meets it.
	refusal error
	// itemTypes are the types, without the white space around them, of the
	// last <item> that names one at each level below a definition typed
	// C_PRIMITIVE_OBJECT, the definition's own items first: the items of a
	// level count as one, the last that names a type deciding.
	itemTypes []string
}

// tolerate returns data rewritten so that canonical XML decodes it as the
// reader reads it, each shape and value the reader tolerates in its weaker
// form, and whether it rewrote anything. Given strict, it keeps only the
// readings that hold in every mode, and it rewrites the shapes strict mode
// refuses, and canonical XML reads, into ones canonical XML refuses as well.
// It returns data unchanged when encoding/xml cannot read a token of data: the
// decode then fails on it as it would have. It reads up to the end of the root
// element, and leaves what follows it to the decode.
//
// A reading may leave out content the decode then does not read, so tolerate
// holds data to maxDepth itself: it reads no further than the first element
// nested deeper, and returns the refusal of that element (depthRefusal). For
// the same reason it refuses an end tag that does not close the innermost
// open element, and the end of data inside the root, with the error
// encoding/xml gives for it (syntaxRefusal).
func tolerate(data []byte, strict bool, maxDepth int) ([]byte, bool, error) {
	t := &tolerantReading{data: data, strict: strict, bound: make(map[string][]string)}
	base := 0
	if bytes.HasPrefix(data, byteOrderMark) {
		base = len(byteOrderMark)
	}
	dec := xml.NewDecoder(bytes.NewReader(data[base:]))
read:
	for {
		off := base + int(dec.InputOffset())
		tok, err := dec.RawToken()
		if errors.Is(err, io.EOF) {
			if len(t.open) > 0 {
				return nil, false, syntaxRefusal(data[base:])
			}
			break
		}
		if err != nil {
			return data, false, nil
		}
		end := base + int(dec.InputOffset())
		switch tk := tok.(type) {
		case xml.StartElement:
			if len(t.open) >= maxDepth {
				return nil, false, depthRefusal(data, maxDepth)
			}
			t.start(tk.Copy(), off, end)
			if t.refusal != nil {
				return nil, false, t.refusal
			}
		case xml.EndElement:
			n := len(t.open)
			if n == 0 {
				// A stray end tag before the root, which the decode refuses.
				continue
			}
			if t.open[n-1].name != tk.Name {
				return nil, false, syntaxRefusal(data[base:])
			}
			t.close(off, end)
			if t.refusal != nil {
				return nil, false, t.refusal
			}
			if n == 1 {
				// The root element ends here.
				break read
			}
		case xml.CharData:
			if n := len(t.open); n > 0 && t.open[n-1].role == roleValue {
				t.open[n-1].text = append(t.open[n-1].text, tk...)
			}
		}
	}
	if len(t.edits) == 0 || t.failed {
		return data, false, nil
	}
	read, changed := t.apply()
	return read, changed, nil
}

// depthRefusal returns the refusal of the first element of data nested deeper
// than limit, the one [canxml.WithMaxDepth] gives: a decode with that limit
// alone, into a value that reads no element, follows each element of the root
// up to that one and refuses it there. A decode that fails before it, on
// markup encoding/xml does not read, returns its own error. Where the root
// holds no such element, the element follows the root, and the refusal is
// that of content after the root.
func depthRefusal(data []byte, limit int) error {
	dec := canxml.NewDecoder(bytes.NewReader(bytes.TrimPrefix(data, byteOrderMark)), canxml.WithMaxDepth(limit))
	if err := dec.Decode(new(struct{})); err != nil {
		return err
	}
	return errAfterRoot()
}

// syntaxRefusal returns the refusal of data, a document whose tokens
// encoding/xml reads but whose elements do not nest: the error its Token
// gives at the first end tag that does not close the innermost open element,
// or at the end of data inside an element, which names the line it stands on
// in data.
func syntaxRefusal(data []byte) error {
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		if _, err := dec.Token(); err != nil {
			if errors.Is(err, io.EOF) {
				// Token refuses what the reading found; this keeps a bare io.EOF,
				// which a caller would take for the end of its own stream, out.
				return io.ErrUnexpectedEOF
			}
			return err
		}
	}
}

// start opens the element whose start tag tk spans data from off to end.
func (t *tolerantReading) start(tk xml.StartElement, off, end int) {
	var parent *element
	if n := len(t.open); n > 0 {
		parent = t.open[n-1]
		parent.nested = true
	}
	e := &element{
		parent: parent,
		name:   tk.Name,
		attrs:  tk.Attr,
		off:    off,
		tagEnd: end,
		self:   bytes.HasSuffix(t.data[off:end], []byte("/>")),
		scope:  len(t.scope),
		mark:   len(t.edits),
		typeAt: -1,
		bareAt: -1,
	}
	for i, a := range e.attrs {
		prefix, ok := declared(a.Name)
		if !ok {
			continue
		}
		// An element in the namespace the AM Release-1.4 schemas declare reads
		// as the element of that name in canonical XML's, and, in the default
		// mode, so does one in a namespace of no openEHR schema, matched by
		// its local name. The default mode rebinds a default namespace, which
		// no attribute takes, and renames an element under a prefix (below),
		// which an attribute or a type, such as an xsi:type or xsd:string, may
		// use too.
		if a.Value == nsOpenEHRv2 || (!t.strict && prefix == "" && isForeignNamespace(a.Value)) {
			e.attrs[i].Value = canxml.NSDefault
			e.dirty = true
		}
		t.push(prefix, e.attrs[i].Value)
	}
	// An element under a prefix bound to a namespace of no openEHR schema, the
	// XSI one and the XML one among them, or bound by no declaration, which
	// encoding/xml reads as a namespace of that name, reads in the default
	// mode as the element of that name in canonical XML's, matched by its
	// local name. The xml prefix needs no declaration. It is written without
	// its prefix, in the default namespace, which the default mode reads as
	// canonical XML's (isForeignNamespace), and the prefix keeps its
	// namespace, so an attribute such as xml:lang stays. xmlns is a
	// declaration, not an element prefix.
	if p := e.name.Space; !t.strict && p != "" && p != "xmlns" {
		if uri, ok := t.lookup(p); !ok || isForeignNamespace(uri) {
			e.renamed, e.dirty = true, true
		}
	}
	for i, a := range e.attrs {
		switch {
		case isDeclaration(a.Name):
		case a.Name.Space == "":
			if a.Name.Local == "type" && e.bareAt < 0 {
				e.bareAt = i
			}
		case a.Name.Local == "type" && e.typeAt < 0:
			if uri, ok := t.lookup(a.Name.Space); ok && uri == canxml.NSXSI {
				e.typeAt = i
			}
		}
	}
	e.end = end
	if e.self {
		e.endTag = end
	}
	if parent != nil && parent.unwrap {
		e.inherits, e.uses = true, make(map[string]bool)
		t.inheriting = append(t.inheriting, e)
	}
	if n := len(t.inheriting); n > 0 {
		t.inheriting[n-1].useNames(e)
	}
	if parent != nil && (e.renamed || t.isOpenEHR(e.name)) {
		e.role = t.roleOf(parent, e)
		if parent.wrap {
			parent.members = append(parent.members, e)
		}
	} else if parent == nil {
		e.role = roleRoot
	}
	t.open = append(t.open, e)
}

// roleOf returns the role of e, an element in the openEHR namespace whose
// parent is parent, and gives an attribute, an object or an item its class.
func (t *tolerantReading) roleOf(parent, e *element) role {
	local := e.name.Local
	if parent.itemLevel > 0 && local == "item" {
		t.definitionItem(parent, e)
	}
	switch parent.role {
	case roleRoot:
		if local == "definition" {
			e.class = "C_ARCHETYPE_ROOT"
			t.typeDefinition(e)
			return roleObject
		}
	case roleObject:
		switch {
		case parent.unwrap && isObjectMember(local):
			// The object that holds the item has its own.
			return roleLeftOut
		case parent.leaf && !isObjectMember(local):
			// A leaf has the rm_type_name, node_id and occurrences it holds,
			// and none of its attributes and other members.
			return roleLeftOut
		case local == "attributes" && isComplexObject(parent.class):
			t.typeAttribute(e)
			return roleAttribute
		case local == "item" && parent.class == "C_PRIMITIVE_OBJECT":
			return t.typeItem(e)
		case local == "list" && parent.class == "C_DV_QUANTITY":
			return roleQuantityItem
		case local == "assumed_value" && parent.class == "C_DV_QUANTITY":
			return roleQuantityAssumed
		case local == "archetype_id" && (parent.untyped || parent.complexTyped):
			return roleArchetypeID
		}
	case roleAttribute:
		switch local {
		case "children":
			return t.typeObject(e)
		case "cardinality":
			parent.cardinality = true
		}
	case roleItem:
		return itemMemberRole(parent, e)
	case roleQuantityItem:
		if local == "magnitude" || local == "precision" {
			e.class = "C_QUANTITY_ITEM"
			return roleRange
		}
	case roleRange:
		switch local {
		case "lower", "upper", "lower_unbounded", "upper_unbounded", "lower_included", "upper_included":
			return roleValue
		}
	case roleArchetypeID:
		if local == "value" {
			return roleValue
		}
	case roleNone, roleValue, roleLeftOut, roleQuantityAssumed:
	}
	return roleNone
}

// itemMemberRole returns the role of e, a member of the primitive item p.
func itemMemberRole(p, e *element) role {
	local := e.name.Local
	switch p.class {
	case "C_BOOLEAN":
		if local == "true_valid" || local == "false_valid" || local == "assumed_value" {
			return roleValue
		}
	case "C_STRING":
		if local == "list_open" {
			return roleValue
		}
	case "C_INTEGER", "C_REAL":
		switch local {
		case "list", "assumed_value":
			return roleValue
		case "range":
			e.class = p.class
			return roleRange
		}
	case "C_DATE", "C_TIME", "C_DATE_TIME":
		if local == "timezone_validity" {
			return roleValue
		}
	}
	return roleNone
}

// typeAttribute reads the type of e, an <attributes> of a complex object.
func (t *tolerantReading) typeAttribute(e *element) {
	written, typed := e.xsiType()
	name := strings.TrimSpace(written)
	if !typed || name == "" {
		t.untypedSlot(e)
		return
	}
	e.class = name
	// The default mode reads the type without the white space around it.
	if name != written && !t.strict && (name == "C_SINGLE_ATTRIBUTE" || name == "C_MULTIPLE_ATTRIBUTE") {
		e.retype = name
	}
}

// typeDefinition reads the type of e, the definition, which the decode reads
// as a C_ARCHETYPE_ROOT whatever its type. The default mode reads a
// definition typed with a class it cannot hold, of no class of the AOM 1.4
// constraint model or of one other than C_ARCHETYPE_ROOT and
// C_COMPLEX_OBJECT, as a leaf; strict mode refuses it, a type with white
// space around it among them, as canonical XML refuses such a type at a slot
// that dispatches on it. One typed ARCHETYPE_SLOT, or with a class that
// carries a primitive constraint (carriesPrimitiveConstraint), is a slot or a
// leaf that carries that constraint, which the model's definition holds
// neither of: every mode refuses it, as strict mode refuses any other class
// it cannot hold, and one typed C_PRIMITIVE_OBJECT where the items it holds
// carry such a constraint, once it closes (definitionRefusal).
func (t *tolerantReading) typeDefinition(e *element) {
	written, typed := e.xsiType()
	name := strings.TrimSpace(written)
	if !typed || name == "" {
		return
	}
	if t.strict {
		name = written
	}
	if alias, ok := xmlschema.AliasOf(name); ok {
		name = alias
	}
	switch {
	case name == "C_ARCHETYPE_ROOT", name == "C_COMPLEX_OBJECT":
	case t.strict:
		inner := typereg.ErrTypeMismatch
		if kindOf(name) == kindNone {
			inner = typereg.ErrUnknownType
		}
		t.refusal = &canxml.DecodeError{Path: "/definition", Type: written, Inner: inner}
	case name == "ARCHETYPE_SLOT", carriesPrimitiveConstraint(name):
		t.refusal = definitionRefusal(written)
	case name == "C_PRIMITIVE_OBJECT":
		e.leaf, e.itemLevel = true, 1
	default:
		e.leaf = true
	}
}

// definitionRefusal is the refusal, in every mode, of a definition of the
// type written, a slot or a class that carries a primitive constraint.
func definitionRefusal(written string) error {
	return &canxml.DecodeError{Path: "/definition", Type: written, Inner: typereg.ErrTypeMismatch}
}

// definitionItem reads e, an <item> of parent, a definition typed
// C_PRIMITIVE_OBJECT or an item it holds an item of: it keeps the type e
// names, if any, as that of the items of its level, and gives e the level of
// the items it holds.
func (t *tolerantReading) definitionItem(parent, e *element) {
	level := parent.itemLevel - 1
	e.itemLevel = parent.itemLevel + 1
	written, typed := e.xsiType()
	if !typed {
		return
	}
	for len(t.itemTypes) <= level {
		t.itemTypes = append(t.itemTypes, "")
	}
	t.itemTypes[level] = strings.TrimSpace(written)
}

// itemsHoldConstraint reports whether the items below a definition typed
// C_PRIMITIVE_OBJECT, whose types t.itemTypes holds, carry a primitive
// constraint: whether, at the first level whose items name another type than
// C_PRIMITIVE_OBJECT, they name a class that carries one.
func (t *tolerantReading) itemsHoldConstraint() bool {
	for _, class := range t.itemTypes {
		if class != "C_PRIMITIVE_OBJECT" {
			return carriesPrimitiveConstraint(class)
		}
	}
	return false
}

// typeObject reads the type of e, a <children> of an attribute, and returns
// its role.
func (t *tolerantReading) typeObject(e *element) role {
	written, typed := e.xsiType()
	name := strings.TrimSpace(written)
	if !typed || name == "" {
		t.untypedSlot(e)
		// Either type it takes is a complex object.
		e.class = "C_COMPLEX_OBJECT"
		return roleObject
	}
	switch kindOf(name) {
	case kindObject:
		e.class = name
		if alias, ok := xmlschema.AliasOf(name); ok {
			e.class = alias
		}
		if name != written && !t.strict {
			e.retype = name
		}
		e.complexTyped = name == "C_COMPLEX_OBJECT" && !t.strict
	case kindNone, kindAttribute:
		// The default mode reads an object of no class of the constraint
		// model, or of an attribute's, which a C_OBJECT slot does not hold,
		// as a leaf; strict mode leaves it to the decode, which refuses it.
		if !t.strict {
			e.retype, e.class, e.leaf = "C_COMPLEX_OBJECT", "C_COMPLEX_OBJECT", true
		}
	case kindPrimitive:
		// The default mode reads the object as a leaf that holds a constraint
		// of its class, which a C_PRIMITIVE_OBJECT holds as its item; strict
		// mode leaves it to the decode, which refuses it in a C_OBJECT slot.
		if !t.strict {
			e.retype, e.class, e.wrap = "C_PRIMITIVE_OBJECT", name, true
			return roleItem
		}
	}
	return roleObject
}

// typeItem reads the type of e, the <item> of a C_PRIMITIVE_OBJECT, and
// returns its role.
func (t *tolerantReading) typeItem(e *element) role {
	written, typed := e.xsiType()
	name := strings.TrimSpace(written)
	if !typed || name == "" {
		if t.strict {
			t.untypedSlot(e)
			return roleNone
		}
		return roleLeftOut
	}
	switch {
	case kindOf(name) == kindPrimitive:
		e.class = name
		if name != written && !t.strict {
			e.retype = name
		}
		return roleItem
	case t.strict:
		return roleNone
	case isUnwrapClass(name):
		// The default mode reads the constraint of the item's class from the
		// item, on the object that holds it.
		e.class, e.unwrap, e.host = name, true, e.parent
		if e.parent.unwrap {
			e.host = e.parent.host
		}
		return roleObject
	}
	return roleLeftOut
}

// untypedSlot reads e, an attribute, an object or an item that names no type
// in the XSI namespace. The default mode types it when it closes. Strict mode
// leaves it to the decode, which refuses it, once it has taken off a bare type
// attribute, which the decode would otherwise read as its type.
func (t *tolerantReading) untypedSlot(e *element) {
	if !t.strict {
		e.untyped = true
		return
	}
	if e.bareAt >= 0 {
		e.attrs = slices.Delete(slices.Clone(e.attrs), e.bareAt, e.bareAt+1)
		if e.typeAt > e.bareAt {
			e.typeAt--
		}
		e.bareAt = -1
		e.dirty = true
	}
}

// close closes the innermost open element, whose end tag spans data from off
// to end, or which its start tag closed.
func (t *tolerantReading) close(off, end int) {
	n := len(t.open)
	if n == 0 {
		return
	}
	e := t.open[n-1]
	t.open = t.open[:n-1]
	if !e.self {
		e.endTag, e.end = off, end
	}
	if e.inherits {
		t.inheriting = t.inheriting[:len(t.inheriting)-1]
		inheritDeclarations(e)
		if n := len(t.inheriting); n > 0 {
			for p := range e.uses {
				t.inheriting[n-1].uses[p] = true
			}
		}
	}
	switch e.role {
	case roleValue:
		t.closeValue(e)
	case roleRange:
		t.closeRange(e)
		t.closeTag(e)
	case roleItem:
		t.closeItem(e)
	case roleAttribute:
		if e.untyped {
			e.retype = "C_SINGLE_ATTRIBUTE"
			if e.cardinality {
				e.retype = "C_MULTIPLE_ATTRIBUTE"
			}
		}
		t.closeTag(e)
	case roleObject:
		if e.itemLevel == 1 && t.itemsHoldConstraint() {
			// The definition, read as a leaf that carries a constraint.
			written, _ := e.xsiType()
			t.refusal = definitionRefusal(written)
		}
		switch {
		case e.untyped:
			e.retype = "C_COMPLEX_OBJECT"
			if e.archetypeID {
				e.retype = "C_ARCHETYPE_ROOT"
			}
		case e.complexTyped && e.archetypeID:
			// The default mode reads it as the archetype root it names.
			e.retype = "C_ARCHETYPE_ROOT"
		}
		if e.unwrap {
			t.unwrapItem(e)
		} else {
			t.closeTag(e)
		}
	case roleQuantityAssumed:
		// An assumed value of a C_DV_QUANTITY that the model cannot hold
		// reads as absent, in every mode.
		if t.holdsQuantity(e) {
			t.closeTag(e)
		} else {
			t.leaveOut(e)
		}
	case roleLeftOut:
		t.leaveOut(e)
	case roleNone, roleRoot, roleQuantityItem, roleArchetypeID:
		t.closeTag(e)
	}
	t.popScope(e.scope)
}

// closeValue reads the text of the value e.
func (t *tolerantReading) closeValue(e *element) {
	p := e.parent
	switch p.role {
	case roleItem:
		switch p.class {
		case "C_BOOLEAN":
			t.closeBooleanMember(p, e)
		case "C_STRING":
			t.closeListOpen(e)
		case "C_INTEGER", "C_REAL":
			t.closeNumberMember(p, e)
		default:
			t.closeTimezoneValidity(e)
		}
		return
	case roleRange:
		side := &p.sides[0]
		if strings.HasPrefix(e.name.Local, "upper") {
			side = &p.sides[1]
		}
		switch e.name.Local {
		case "lower", "upper":
			side.bound = e
		case "lower_unbounded", "upper_unbounded":
			side.unbounded = e
		default:
			side.included = e
		}
	case roleArchetypeID:
		if !e.nested && !isBlank(e.text) {
			p.parent.archetypeID = true
		}
	case roleNone, roleRoot, roleObject, roleAttribute, roleQuantityItem, roleQuantityAssumed, roleValue, roleLeftOut:
	}
	t.closeTag(e)
}

// closeBooleanMember reads e, a true_valid, false_valid or assumed_value of
// the C_BOOLEAN item p.
func (t *tolerantReading) closeBooleanMember(p, e *element) {
	switch e.name.Local {
	case "true_valid":
		p.seen |= seenTrueValid
	case "false_valid":
		p.seen |= seenFalseValid
	}
	if e.nested {
		t.closeTag(e)
		return
	}
	switch e.name.Local {
	case "true_valid", "false_valid":
		// An empty flag reads as true, as an absent one does (closeItem).
		if isBlank(e.text) {
			t.setText(e, "true")
			return
		}
	case "assumed_value":
		text := strings.TrimSpace(string(e.text))
		var want bool
		switch strings.ToLower(text) {
		case "true", "1":
			want = true
		case "false", "0":
		default:
			// No boolean the reader reads: the item assumes no value.
			t.leaveOut(e)
			return
		}
		if v, err := strconv.ParseBool(text); err != nil || v != want {
			t.setText(e, strconv.FormatBool(want))
			return
		}
	}
	t.closeTag(e)
}

// closeListOpen reads e, the list_open of a C_STRING item. One of white
// space only reads as an empty one, which the decode reads as false, in every
// mode, so the list stays closed: the reader's own reading, the one it gives
// an empty C_BOOLEAN flag, not XML Schema's, which refuses an empty boolean.
func (t *tolerantReading) closeListOpen(e *element) {
	if !e.nested && len(e.text) > 0 && isBlank(e.text) {
		t.setText(e, "")
		return
	}
	t.closeTag(e)
}

// closeNumberMember reads e, a list entry or the assumed_value of the
// C_INTEGER or C_REAL item p. The default mode reads a real number Go reads
// and canonical XML does not as the number Go reads.
func (t *tolerantReading) closeNumberMember(p, e *element) {
	if e.nested {
		t.closeTag(e)
		return
	}
	if isGoNumber(p.class, e.text) {
		if text, ok := boundText(p.class, e.text); ok && text != "" && !t.strict {
			t.setText(e, text)
			return
		}
		t.closeTag(e)
		return
	}
	switch {
	case e.name.Local == "assumed_value":
		// Not a number of the item's type: the item assumes no value.
		t.leaveOut(e)
	case !t.strict:
		// A list entry that is not a number of the item's type is left out.
		t.leaveOut(e)
	case len(e.text) == 0:
		// The decode reads an empty entry as 0; strict mode refuses it, as the
		// decode refuses any other text that is no number.
		t.setText(e, "-")
	default:
		t.closeTag(e)
	}
}

// closeTimezoneValidity reads e, the timezone_validity of a C_DATE, C_TIME or
// C_DATE_TIME item. An empty one reads as optional, as an absent one does, in
// every mode; the default mode reads one that names no validity kind as
// optional too.
func (t *tolerantReading) closeTimezoneValidity(e *element) {
	if e.nested {
		t.closeTag(e)
		return
	}
	var kind TimezoneValidityKind
	if isBlank(e.text) || (!t.strict && kind.UnmarshalText(e.text) != nil) {
		t.leaveOut(e)
		return
	}
	t.closeTag(e)
}

// holdsQuantity reports whether the model holds e, the assumed_value of a
// C_DV_QUANTITY: whether canonical XML decodes it, as the reading writes it,
// inside an element that binds the prefixes it uses as they are bound around
// it. Nothing e holds is nested past the depth limit, which tolerate refused
// on the way in.
func (t *tolerantReading) holdsQuantity(e *element) bool {
	var frag bytes.Buffer
	frag.WriteString(t.startTag(e))
	if e.self {
		frag.WriteString("/>")
	} else {
		frag.WriteByte('>')
		frag.Write(t.render(e.mark, e.tagEnd, e.endTag))
		frag.WriteString(e.closing())
	}
	var b bytes.Buffer
	b.WriteString("<holder")
	for _, prefix := range usedPrefixes(frag.Bytes()) {
		// e's own declarations, in scope where it closes, bind its prefixes
		// as its start tag does.
		uri, ok := t.lookup(prefix)
		if !ok {
			continue
		}
		name := xml.Name{Space: "xmlns", Local: prefix}
		if prefix == "" {
			name = xml.Name{Local: "xmlns"}
		}
		b.WriteString(" " + qualified(name) + `="`)
		_ = xml.EscapeText(&b, []byte(uri)) // a bytes.Buffer does not fail
		b.WriteByte('"')
	}
	b.WriteByte('>')
	b.Write(frag.Bytes())
	b.WriteString("</holder>")
	return canxml.Unmarshal(b.Bytes(), new(CDVQuantity)) == nil
}

// usedPrefixes returns the prefixes the names of the elements and attributes
// of frag, a fragment of the input, use, but those of namespace
// declarations, the default namespace's "" first and the others in the order
// of their names. A fragment encoding/xml does not read uses those it reads.
func usedPrefixes(frag []byte) []string {
	used := map[string]bool{"": true}
	d := xml.NewDecoder(bytes.NewReader(frag))
	for {
		tok, err := d.RawToken()
		if err != nil {
			break
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		used[start.Name.Space] = true
		for _, a := range start.Attr {
			if !isDeclaration(a.Name) && a.Name.Space != "" {
				used[a.Name.Space] = true
			}
		}
	}
	return slices.Sorted(maps.Keys(used))
}

// closeRange reads the bounds of the range r. A bound Go reads a number from
// is read as closeNumberBound says. Any other bound is left out, and its side
// read as unbounded: in every mode on a side written unbounded, and on a side
// of a quantity's magnitude or precision; elsewhere in the default mode, the
// side included unless the range says otherwise, as a bounded side is.
// Strict mode refuses the bound of a C_INTEGER or C_REAL on a bounded side,
// an empty one too, which the decode reads as 0.
func (t *tolerantReading) closeRange(r *element) {
	kind := r.class
	if kind == "C_QUANTITY_ITEM" && r.name.Local == "precision" {
		kind = "precision"
	}
	for i := range r.sides {
		s := r.sides[i]
		b := s.bound
		if b == nil || b.nested {
			continue
		}
		written, open := boundaryFlag(s.unbounded)
		if t.closeNumberBound(r, i, kind, written && open) {
			continue
		}
		if written && open {
			t.leaveOut(b)
			continue
		}
		if t.strict && r.class != "C_QUANTITY_ITEM" {
			if len(b.text) == 0 {
				t.setText(b, "-")
			}
			continue
		}
		t.unboundSide(r, i)
	}
}

// unboundSide leaves out the bound of side i of the range r and reads the
// side as unbounded: the unbounded flag the range writes reads true, and the
// side is included unless the range says otherwise, as a bounded side is. A
// flag the range writes that is no boolean is left as it is, with the bound,
// for the decode to refuse.
func (t *tolerantReading) unboundSide(r *element, i int) {
	s := r.sides[i]
	written, _ := boundaryFlag(s.unbounded)
	if s.unbounded != nil && !s.unbounded.nested && !written && !isBlank(s.unbounded.text) {
		return
	}
	t.leaveOut(s.bound)
	if written {
		t.setText(s.unbounded, "true")
	}
	t.includeSide(r, i, false)
}

// closeNumberBound reads the bound of side i of the range r, a range of kind,
// as closeRange names it, and reports whether strconv.ParseFloat reads a
// number from it. A number canonical XML reads as written stays. Any other is
// left out on a side written unbounded, where openSide. On a bounded side, a
// bound of a C_INTEGER range or of a quantity's precision is read, in every
// mode, as the integer bound that admits the same integers (integerBound),
// its side included where the number is not a whole one, or, where it is the
// infinity that admits every integer there, negative on the lower side or
// positive on the upper one, or NaN, which bounds no integer, as the side
// unbounded (unboundSide); and, in the default mode, a bound of a C_REAL
// range or of a quantity's magnitude as the Real Go reads. An integer bound
// past 32 bits is written as that integer, which the decode refuses as such,
// and an infinity that admits no integer stays as it is, which the decode
// refuses as no integer.
func (t *tolerantReading) closeNumberBound(r *element, i int, kind string, openSide bool) bool {
	b := r.sides[i].bound
	integer := kind == "C_INTEGER" || kind == "precision"
	var (
		text                        string
		rounded, unbounded, differs bool
	)
	switch {
	case integer && isInteger32(b.text):
		return true
	case integer:
		f, ok := goFloat(b.text)
		if !ok {
			return false
		}
		switch {
		case math.IsNaN(f), (i == 0) == math.IsInf(f, -1) && math.IsInf(f, 0):
			// It admits every integer on its side, so the side is unbounded.
			unbounded = true
		case !math.IsInf(f, 0):
			text, rounded = integerBound(f, i == 0)
		}
		differs = true
	case !isGoNumber(kind, b.text):
		return false
	default:
		text, differs = boundText(kind, b.text)
	}
	switch {
	case !differs:
	case openSide:
		t.leaveOut(b)
	case unbounded:
		t.unboundSide(r, i)
	case text == "", t.strict && !integer:
		// Left as it is, for the decode to refuse.
	default:
		t.setText(b, text)
		if rounded {
			t.includeSide(r, i, true)
		}
	}
	return true
}

// includeSide writes side i of the range r included where the range gives it
// no included flag, or an empty one, and, where force, in place of the flag
// it gives. It leaves a flag that holds an element as it is.
func (t *tolerantReading) includeSide(r *element, i int, force bool) {
	included := r.sides[i].included
	name := "lower_included"
	if i == 1 {
		name = "upper_included"
	}
	switch {
	case included == nil:
		r.insert = appendElement(r.insert, r.written().Space, name, "true")
	case included.nested:
	case force || isBlank(included.text):
		t.setText(included, "true")
	}
}

// boundaryFlag reads the boundary flag e: whether the range writes it, as a
// boolean, and its value. A flag the range leaves out, leaves empty or writes
// otherwise is not written.
func boundaryFlag(e *element) (written, value bool) {
	if e == nil || e.nested || isBlank(e.text) {
		return false, false
	}
	v, err := strconv.ParseBool(strings.TrimSpace(string(e.text)))
	return err == nil, v
}

// closeItem adds to the C_BOOLEAN item p the flags it leaves out, each true,
// writes the item a wrap p holds around its members, and closes its start
// tag.
func (t *tolerantReading) closeItem(p *element) {
	if p.class == "C_BOOLEAN" {
		if p.seen&seenTrueValid == 0 {
			p.insert = appendElement(p.insert, p.written().Space, "true_valid", "true")
		}
		if p.seen&seenFalseValid == 0 {
			p.insert = appendElement(p.insert, p.written().Space, "false_valid", "true")
		}
	}
	if p.wrap {
		t.wrapMembers(p)
	}
	t.closeTag(p)
}

// wrapMembers writes, inside p, an object typed with a primitive class, an
// item of that class that holds what p holds, but p's rm_type_name,
// occurrences and node_id, which it moves before the item, and the elements
// the reading adds to p. p's start tag then reads as a C_PRIMITIVE_OBJECT.
// The members the item holds have their paths in it.
func (t *tolerantReading) wrapMembers(p *element) {
	name := qualified(xml.Name{Space: p.written().Space, Local: "item"})
	open := "<" + name + " " + qualified(p.attrs[p.typeAt].Name) + `="` + p.class + `">`
	end := "</" + name + ">"
	if p.self {
		p.insert = slices.Concat([]byte(open), p.insert, []byte(end))
		return
	}
	// The spans the members cut p's content into follow one another, so one
	// pass over the edits made inside p, in the order of their offsets,
	// renders them all.
	edits := t.editsIn(p.mark, p.tagEnd, p.endTag)
	var before, inside []byte
	pos := p.tagEnd
	for _, m := range p.members {
		if !isObjectMember(m.name.Local) {
			continue
		}
		inside = t.renderSpan(inside, &edits, pos, m.off)
		before = t.renderSpan(before, &edits, m.off, m.end)
		pos = m.end
	}
	inside = t.renderSpan(inside, &edits, pos, p.endTag)
	t.drop(p)
	t.add(p.tagEnd, p.endTag, string(before)+open+string(inside)+string(p.insert)+end)
	p.insert = nil
}

// unwrapItem reads e, an item of a class isUnwrapClass names, as the object
// that holds it: it takes off e's start and end tags, so that what e holds is
// held by e.host, which takes e's class unless that is C_PRIMITIVE_OBJECT.
// Each element e holds declares the namespaces e declares
// (inheritDeclarations), so that it keeps its names. The members e holds have
// their paths in the host.
func (t *tolerantReading) unwrapItem(e *element) {
	if e.class != "C_PRIMITIVE_OBJECT" {
		e.host.retype = e.class
	}
	if e.self {
		t.add(e.off, e.end, "")
		return
	}
	t.add(e.off, e.tagEnd, "")
	t.add(e.endTag, e.end, "")
}

// useNames adds to e.uses the prefixes the name of x, as written and as the
// reading writes it, and the names of its attributes but its namespace
// declarations, carry, the default namespace's "" for a name without one.
func (e *element) useNames(x *element) {
	e.uses[x.name.Space] = true
	e.uses[x.written().Space] = true
	for _, a := range x.attrs {
		if !isDeclaration(a.Name) && a.Name.Space != "" {
			e.uses[a.Name.Space] = true
		}
	}
}

// inheritDeclarations adds to the start tag of e, an element that inherits,
// each namespace declaration of its parent, an item the reading takes off,
// for a prefix e or what it holds uses (e.uses) that e does not declare
// itself. Taking the item's tags off then changes the name of no element: e
// and what it holds keep the names they had inside the item, and no element
// outside it sees the item's declarations. Each element the item holds
// declares only what it uses, so that the item's declarations are not
// written once for each of them.
func inheritDeclarations(e *element) {
	item := e.parent
	if item.declares == nil {
		item.declares = make(map[string]xml.Attr)
		for _, a := range item.attrs {
			if prefix, ok := declared(a.Name); ok {
				item.declares[prefix] = a
			}
		}
	}
	own := make(map[string]bool)
	for _, a := range e.attrs {
		if prefix, ok := declared(a.Name); ok {
			own[prefix] = true
		}
	}
	for _, prefix := range slices.Sorted(maps.Keys(e.uses)) {
		if a, ok := item.declares[prefix]; ok && !own[prefix] {
			e.attrs = append(e.attrs, a)
			e.dirty = true
		}
	}
}

// render returns the input from off to end with the edits made that start
// there, of those made since the reading made mark edits: the span of an
// element the reading opened when it had made mark edits, which no edit made
// before it touches.
func (t *tolerantReading) render(mark, off, end int) []byte {
	edits := t.editsIn(mark, off, end)
	return t.renderSpan(nil, &edits, off, end)
}

// editsIn returns the live edits made since the reading made mark edits that
// start from off to end, in the order of their offsets, and in the order the
// reading made them where two start alike.
func (t *tolerantReading) editsIn(mark, off, end int) []edit {
	var edits []edit
	for _, d := range t.edits[mark:] {
		if !d.dead && d.off >= off && d.off < end {
			edits = append(edits, d)
		}
	}
	slices.SortStableFunc(edits, func(a, b edit) int { return cmp.Compare(a.off, b.off) })
	return edits
}

// renderSpan appends to out the input from off to end with the edits that
// start there made, which it takes off the front of *edits, edits in the
// order of their offsets of which none starts before off. Two edits that
// overlap, which the reading does not make, set t.failed, and the span is
// then appended as it is.
func (t *tolerantReading) renderSpan(out []byte, edits *[]edit, off, end int) []byte {
	start, pos := len(out), off
	for len(*edits) > 0 && (*edits)[0].off < end {
		d := (*edits)[0]
		*edits = (*edits)[1:]
		if d.off < pos || d.end > end {
			t.failed = true
			return append(out[:start], t.data[off:end]...)
		}
		out = append(out, t.data[pos:d.off]...)
		out = append(out, d.text...)
		pos = d.end
	}
	return append(out, t.data[pos:end]...)
}

// closeTag writes the start tag of e where the reading changed it, and the
// elements it adds before e's end tag.
func (t *tolerantReading) closeTag(e *element) {
	if !e.dirty && e.retype == "" && len(e.insert) == 0 {
		return
	}
	tag := t.startTag(e)
	switch {
	case e.self && len(e.insert) > 0:
		t.add(e.off, e.end, tag+">"+string(e.insert)+e.closing())
	case e.self:
		t.add(e.off, e.end, tag+"/>")
	default:
		if e.dirty || e.retype != "" {
			t.add(e.off, e.tagEnd, tag+">")
		}
		switch {
		case e.renamed:
			t.add(e.endTag, e.end, string(e.insert)+e.closing())
		case len(e.insert) > 0:
			t.add(e.endTag, e.endTag, string(e.insert))
		}
	}
}

// setText replaces the text of the value e with text, keeping its start tag.
func (t *tolerantReading) setText(e *element, text string) {
	t.drop(e)
	tag := t.startTag(e)
	if e.self {
		t.add(e.off, e.end, tag+">"+text+e.closing())
		return
	}
	if e.dirty {
		t.add(e.off, e.tagEnd, tag+">")
	}
	if e.renamed {
		t.add(e.tagEnd, e.end, text+e.closing())
		return
	}
	t.add(e.tagEnd, e.endTag, text)
}

// leaveOut leaves e out of the document, with all it holds. In the default
// mode it becomes an empty element of its name in a namespace the decode does
// not read, which the decode skips as unmatched content at e's path; strict
// mode, which would refuse that, removes it.
func (t *tolerantReading) leaveOut(e *element) {
	t.drop(e)
	if t.strict {
		t.add(e.off, e.end, "")
		return
	}
	t.add(e.off, e.end, "<"+e.name.Local+` xmlns="`+nsLeftOut+`"/>`)
}

// drop forgets the edits that start inside e, which an edit of the whole of
// e replaces. Only an edit made since e opened starts there, so it reads no
// edit made before.
func (t *tolerantReading) drop(e *element) {
	for i := e.mark; i < len(t.edits); i++ {
		if d := &t.edits[i]; d.off >= e.off && d.off < e.end {
			d.dead = true
		}
	}
}

// add records an edit.
func (t *tolerantReading) add(off, end int, text string) {
	t.edits = append(t.edits, edit{off: off, end: end, text: text})
}

// startTag writes the start tag of e, without its closing `>` or `/>`, with
// the type the reading gives it.
func (t *tolerantReading) startTag(e *element) string {
	attrs := e.attrs
	if e.retype != "" {
		attrs = slices.Clone(attrs)
		if e.typeAt >= 0 {
			attrs[e.typeAt].Value = e.retype
		} else {
			prefix, declare := t.xsiPrefix()
			if declare {
				attrs = append(attrs, xml.Attr{Name: xml.Name{Space: "xmlns", Local: prefix}, Value: canxml.NSXSI})
			}
			attrs = append(attrs, xml.Attr{Name: xml.Name{Space: prefix, Local: "type"}, Value: e.retype})
		}
	}
	var b bytes.Buffer
	b.WriteString("<" + qualified(e.written()))
	for _, a := range attrs {
		b.WriteString(" " + qualified(a.Name) + `="`)
		_ = xml.EscapeText(&b, []byte(a.Value)) // a bytes.Buffer does not fail
		b.WriteByte('"')
	}
	return b.String()
}

// xsiPrefix returns a prefix to write an xsi:type with on the innermost open
// element: xsi where it is bound to the XSI namespace there, or else one the
// input does not use (freshPrefix), which the element must declare.
func (t *tolerantReading) xsiPrefix() (string, bool) {
	if uri, _ := t.lookup("xsi"); uri == canxml.NSXSI {
		return "xsi", false
	}
	if t.xsiFresh == "" {
		t.xsiFresh = freshPrefix(t.data)
	}
	return t.xsiFresh, true
}

// freshPrefix returns xsiN for the least N such that data holds no xsi
// followed by the digits of N: a prefix data neither declares nor uses. It
// reads data once.
func freshPrefix(data []byte) string {
	used := make(map[int]bool)
	word := []byte("xsi")
	for i := 0; ; {
		j := bytes.Index(data[i:], word)
		if j < 0 {
			break
		}
		i += j + len(word)
		k := i
		// A number of more digits than this names no prefix a small N gives.
		for k < len(data) && k-i < 9 && '0' <= data[k] && data[k] <= '9' {
			k++
		}
		if n, err := strconv.Atoi(string(data[i:k])); err == nil {
			used[n] = true
		}
	}
	n := 0
	for used[n] {
		n++
	}
	return "xsi" + strconv.Itoa(n)
}

// push binds prefix to uri where the reading stands.
func (t *tolerantReading) push(prefix, uri string) {
	t.scope = append(t.scope, binding{prefix: prefix, uri: uri})
	t.bound[prefix] = append(t.bound[prefix], uri)
}

// popScope unbinds the declarations past the first n in scope.
func (t *tolerantReading) popScope(n int) {
	for len(t.scope) > n {
		b := t.scope[len(t.scope)-1]
		t.scope = t.scope[:len(t.scope)-1]
		stack := t.bound[b.prefix]
		t.bound[b.prefix] = stack[:len(stack)-1]
	}
}

// lookup returns the namespace prefix is bound to where the reading stands.
func (t *tolerantReading) lookup(prefix string) (string, bool) {
	stack := t.bound[prefix]
	if len(stack) == 0 {
		return "", false
	}
	return stack[len(stack)-1], true
}

// isForeignNamespace reports whether the default mode reads uri, the
// namespace of an element, as canonical XML's, as every mode reads the one
// the AM Release-1.4 schemas declare: a namespace other than none, canonical
// XML's and that one, the XSI one among them.
func isForeignNamespace(uri string) bool {
	switch uri {
	case "", canxml.NSDefault, nsOpenEHRv2:
		return false
	}
	return true
}

// isOpenEHR reports whether an element named name, as written, is in the
// namespace canonical XML reads, the one the AM Release-1.4 schemas declare,
// or none.
func (t *tolerantReading) isOpenEHR(name xml.Name) bool {
	uri, ok := t.lookup(name.Space)
	if !ok {
		return name.Space == ""
	}
	return uri == "" || uri == canxml.NSDefault || uri == nsOpenEHRv2
}

// apply returns the input with the edits made, or the input as it is and
// false should two edits overlap, which the reading does not make.
func (t *tolerantReading) apply() ([]byte, bool) {
	edits := slices.DeleteFunc(t.edits, func(d edit) bool { return d.dead })
	slices.SortStableFunc(edits, func(a, b edit) int { return cmp.Compare(a.off, b.off) })
	size := len(t.data)
	for _, d := range edits {
		size += len(d.text) - (d.end - d.off)
	}
	out := make([]byte, 0, size)
	pos := 0
	for _, d := range edits {
		if d.off < pos {
			return t.data, false
		}
		out = append(out, t.data[pos:d.off]...)
		out = append(out, d.text...)
		pos = d.end
	}
	return append(out, t.data[pos:]...), true
}

// written returns the name the reading writes e with: its name as written,
// or its local name alone where renamed.
func (e *element) written() xml.Name {
	if e.renamed {
		return xml.Name{Local: e.name.Local}
	}
	return e.name
}

// closing returns the end tag the reading writes e with.
func (e *element) closing() string {
	return "</" + qualified(e.written()) + ">"
}

// xsiType returns the value of the xsi:type of e, and whether it has one.
func (e *element) xsiType() (string, bool) {
	if e.typeAt < 0 {
		return "", false
	}
	return e.attrs[e.typeAt].Value, true
}

// classKind is what the class an xsi:type names is to a slot of the
// constraint model.
type classKind uint8

const (
	kindNone      classKind = iota // no class of the AOM 1.4 constraint model
	kindObject                     // a C_OBJECT
	kindAttribute                  // a C_ATTRIBUTE
	kindPrimitive                  // a C_PRIMITIVE
)

// kindOf returns the kind of the class an xsi:type of name reads as.
func kindOf(name string) classKind {
	ctor, ok := xmlschema.LookupType(name)
	if !ok {
		return kindNone
	}
	switch ctor().(type) {
	case CObject:
		return kindObject
	case CAttribute:
		return kindAttribute
	case CPrimitive:
		return kindPrimitive
	}
	return kindNone
}

// isUnwrapClass reports whether the default mode unwraps an <item> of a
// C_PRIMITIVE_OBJECT whose xsi:type names class, which the decode refuses:
// it reads an item typed C_CODE_PHRASE, C_DV_QUANTITY, C_DV_ORDINAL or
// CONSTRAINT_REF as the object that holds it, and one typed
// C_PRIMITIVE_OBJECT as the item it holds (unwrapItem). An item of any other
// class that is no C_PRIMITIVE it reads as no item.
func isUnwrapClass(class string) bool {
	switch class {
	case "C_CODE_PHRASE", "C_DV_QUANTITY", "C_DV_ORDINAL", "CONSTRAINT_REF", "C_PRIMITIVE_OBJECT":
		return true
	}
	return false
}

// carriesPrimitiveConstraint reports whether an object of class carries a
// primitive constraint: a C_PRIMITIVE class, or a class isUnwrapClass names
// but C_PRIMITIVE_OBJECT, whose item decides.
func carriesPrimitiveConstraint(class string) bool {
	return kindOf(class) == kindPrimitive || (class != "C_PRIMITIVE_OBJECT" && isUnwrapClass(class))
}

// isComplexObject reports whether an object of class holds attributes.
func isComplexObject(class string) bool {
	return class == "C_COMPLEX_OBJECT" || class == "C_ARCHETYPE_ROOT"
}

// isObjectMember reports whether local names a member every constraint
// object has.
func isObjectMember(local string) bool {
	return local == "rm_type_name" || local == "occurrences" || local == "node_id"
}

// isGoNumber reports whether Go's strconv reads text as a number of the
// class: an integer of 64 bits for a C_INTEGER, and a float of 64 bits for a
// C_REAL and for the bounds of a quantity, each without the white space
// around it.
func isGoNumber(class string, text []byte) bool {
	s := strings.TrimSpace(string(text))
	if class == "C_INTEGER" {
		_, err := strconv.ParseInt(s, 10, 64)
		return err == nil
	}
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

// boundText reads text, which Go reads as a number of kind (isGoNumber), and
// reports whether canonical XML reads it otherwise or not at all; it then
// returns the text canonical XML reads as the number Go reads. kind is
// C_REAL, or C_QUANTITY_ITEM for a quantity's magnitude, each read as a Real;
// text of any other kind reads alike.
func boundText(kind string, text []byte) (string, bool) {
	if kind != "C_REAL" && kind != "C_QUANTITY_ITEM" {
		return "", false
	}
	var r Real
	if r.UnmarshalText(text) == nil {
		return "", false
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(string(text)), 64)
	if err != nil {
		return "", false
	}
	b, _ := Real(f).MarshalText() // a Real always writes
	return string(b), true
}

// goFloat reads text, a bound of a C_INTEGER range or of a quantity's
// precision, with strconv.ParseFloat, without the white space around it, and
// reports whether it reads one.
func goFloat(text []byte) (float64, bool) {
	f, err := strconv.ParseFloat(strings.TrimSpace(string(text)), 64)
	return f, err == nil
}

// integerBound returns the text of the integer bound that admits the
// integers f, a finite bound on the lower side of its range where lower,
// admits: f rounded up on the lower side and down on the upper one, and
// whether rounding changed it.
func integerBound(f float64, lower bool) (bound string, rounded bool) {
	n := math.Floor(f)
	if lower {
		n = math.Ceil(f)
	}
	return strconv.FormatFloat(n, 'f', -1, 64), n != f
}

// isInteger32 reports whether text, without the white space around it, is an
// integer of 32 bits, which canonical XML reads as the model's Integer.
func isInteger32(text []byte) bool {
	_, err := strconv.ParseInt(strings.TrimSpace(string(text)), 10, 32)
	return err == nil
}

// isBlank reports whether text is empty or XML white space only.
func isBlank(text []byte) bool {
	return len(bytes.Trim(text, " \t\r\n")) == 0
}

// declared returns the prefix a namespace declaration named name binds, ""
// for the default namespace, and whether name is a declaration.
func declared(name xml.Name) (string, bool) {
	switch {
	case name.Space == "xmlns":
		return name.Local, true
	case name.Space == "" && name.Local == "xmlns":
		return "", true
	}
	return "", false
}

// isDeclaration reports whether name is a namespace declaration.
func isDeclaration(name xml.Name) bool {
	_, ok := declared(name)
	return ok
}

// qualified writes name as the input writes it, with its prefix.
func qualified(name xml.Name) string {
	if name.Space == "" {
		return name.Local
	}
	return name.Space + ":" + name.Local
}

// appendElement appends to b an element named local, under prefix, that holds
// text.
func appendElement(b []byte, prefix, local, text string) []byte {
	name := qualified(xml.Name{Space: prefix, Local: local})
	return append(b, "<"+name+">"+text+"</"+name+">"...)
}
