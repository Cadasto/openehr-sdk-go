// Package rmjsonwalk reads a canonical-JSON document against the openEHR
// Reference Model's own attribute lists, as openehr/rm/rminfo publishes them,
// without decoding it into Go values.
//
// It answers three questions for every JSON object in the document: which RM
// type the object decodes as, which of its members name no attribute of that
// type, and which attributes the RM requires of that type that the object
// carries no value for. The canonical-JSON codec reports or refuses the
// unmatched members on request and reports the missing ones, and
// testkit/expectedform removes the unmatched members from the expected form
// of a round trip, so all read the same answer from here.
//
// An object's RM type is its "_type" when it carries one. Otherwise it is the
// declared type of the attribute that holds it, and the root's type is the
// decode target's. A [Model] holds the rules; [Model.Walk] walks a document
// with them.
package rmjsonwalk

import (
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
)

// Model answers what a walk over a canonical-JSON document asks of the RM:
// the RM type of an object and what the RM declares about each member. Build
// one with [New], or use [Default] for the pinned RM.
//
// A Model is safe for concurrent use.
type Model struct {
	lookup  rminfo.Lookup
	hier    rminfo.Hierarchy
	absence rminfo.AbsenceReporter
	roots   sync.Map // reflect.Type → rootOf, see [Model.RootOf]
	lists   sync.Map // class name → []string, see [Model.required]
}

// intervalFlags are the four boundary flags BASE Interval declares. The RM
// makes them mandatory, and the decoder gives each a value when the document
// leaves it out, so a walk never reports one missing.
var intervalFlags = []string{"lower_unbounded", "upper_unbounded", "lower_included", "upper_included"}

// required returns the presence-checked attributes of class: the attributes
// the RM makes mandatory on it, in the RM's order (an ancestor's before the
// class's own), minus the attributes no serialization carries
// ([rminfo.IsNonStorableAttr]: a function the model computes, and
// RESOURCE_DESCRIPTION.parent_resource) and minus the interval boundary
// flags. The list of a class is read from the RM once and kept; callers must
// not change it. A name that is no class gives nil.
func (m *Model) required(class string) []string {
	if cached, ok := m.lists.Load(class); ok {
		return cached.([]string)
	}
	attrs := slices.DeleteFunc(m.lookup.RequiredAttributes(class), func(attr string) bool {
		return rminfo.IsNonStorableAttr(class, attr) || slices.Contains(intervalFlags, attr)
	})
	cached, _ := m.lists.LoadOrStore(class, slices.Clip(attrs))
	return cached.([]string)
}

// New returns a Model over l. l must also implement [rminfo.Hierarchy] and
// [rminfo.AbsenceReporter], as [rminfo.Default] does; New fails rather than
// misread a type without them.
func New(l rminfo.Lookup) (*Model, error) {
	h, ok := l.(rminfo.Hierarchy)
	if !ok {
		return nil, errors.New("rmjsonwalk: the RM lookup does not implement rminfo.Hierarchy")
	}
	a, ok := l.(rminfo.AbsenceReporter)
	if !ok {
		return nil, errors.New("rmjsonwalk: the RM lookup does not implement rminfo.AbsenceReporter")
	}
	return &Model{lookup: l, hier: h, absence: a}, nil
}

// Default returns the Model over [rminfo.Default], built on first use.
var Default = sync.OnceValues(func() (*Model, error) {
	return New(rminfo.Default)
})

// Kind classifies the declared type of an attribute.
type Kind int

const (
	// KindClass is an RM class in the class universe.
	KindClass Kind = iota
	// KindPrimitive is a BMM primitive type, such as String, Integer, Real,
	// Octet, Hash or Any. A walk does not look inside its value.
	KindPrimitive
	// KindParameter is a generic type parameter, such as "T", that the slot
	// holding the object does not bind.
	KindParameter
)

// Attribute is what the RM declares about one attribute of an object's RM
// type, as [Model.Attribute] reads it.
type Attribute struct {
	// Type is the declared type of the attribute's values, the element type
	// for a container. For a type parameter the slot binds, it is the bound
	// class; for one it does not bind, it is empty.
	Type string
	// Kind classifies Type. A type parameter the slot binds is a class.
	Kind Kind
	// Container reports a multi-valued attribute.
	Container bool
	// DeclaredOn names the class that declares the attribute: the object's
	// RM type or the ancestor it inherits the attribute from.
	DeclaredOn string
	// Bound is the class the attribute's value binds its own type parameter
	// to, or empty. See [Model.Attribute].
	Bound string
	// Payload reports a generic payload: a type parameter the SDK's decoder
	// holds as plain JSON rather than as an RM value. A walk does not look
	// inside it.
	Payload bool
	// Hash reports a Hash attribute whose values are RM objects: the JSON
	// value is an object whose members each hold one value of class Type,
	// keyed by any name.
	Hash bool
}

// boundBindings binds the type parameter of a DV_INTERVAL slot whose RM type
// argument is a concrete class, keyed by the class that declares the slot and
// the slot's name. rminfo reports the interval's bounds by the parameter's name
// ("T") and does not carry the binding, so it is written out here from the
// openEHR RM 1.2 class definitions. A slot not listed binds the parameter to an
// abstract class (DV_ORDERED) or leaves it open, so a bound there must carry
// "_type". TestBoundBindingsMatchTheRM checks the table against rminfo and the
// generated rm types, in both directions.
var boundBindings = map[string]string{
	"DV_QUANTITY.normal_range":         "DV_QUANTITY",
	"DV_COUNT.normal_range":            "DV_COUNT",
	"DV_PROPORTION.normal_range":       "DV_PROPORTION",
	"PARTICIPATION.time":               "DV_DATE_TIME",
	"CAPABILITY.time_validity":         "DV_DATE",
	"CONTACT.time_validity":            "DV_DATE",
	"PARTY_RELATIONSHIP.time_validity": "DV_DATE",
	"ROLE.time_validity":               "DV_DATE",
}

// payloads lists the type-parameter attributes whose value the decoder holds
// as plain JSON, keyed by the class that holds the attribute and its name. The
// type the registry builds for each of these classes instantiates the
// parameter with Go's any, so encoding/json/v2 decodes the value into maps,
// slices and scalars, not into RM types, and no member inside it is read as an
// RM attribute. TestPayloadsMatchTheRM checks the table against rminfo and the
// registered rm types, in both directions.
var payloads = map[string]bool{
	"ORIGINAL_VERSION.data": true,
	"Point_interval.lower":  true,
	"Point_interval.upper":  true,
	"Proper_interval.lower": true,
	"Proper_interval.upper": true,
}

// hashValues gives the class of the values of each Hash attribute that holds
// RM objects, keyed by the class that declares the attribute and its name.
// rminfo types every Hash attribute as plain "Hash" and drops its value type,
// so it is written out here from the openEHR RM 1.2 class definitions. A Hash
// attribute not listed holds strings or other plain JSON, which a walk does
// not look inside. AUTHORED_RESOURCE.translations has no RM descendant in
// this RM release; the AOM 1.4 ARCHETYPE inherits it. TestHashValuesMatchTheRM
// checks the table against rminfo and the Go map fields of the registered
// types, in both directions.
var hashValues = map[string]string{
	"AUTHORED_RESOURCE.translations": "TRANSLATION_DETAILS",
	"RESOURCE_DESCRIPTION.details":   "RESOURCE_DESCRIPTION_ITEM",
}

// IsClass reports whether name is an RM class in the model's class universe.
func (m *Model) IsClass(name string) bool {
	_, known := m.hier.IsAbstract(name)
	return known
}

// kindOf classifies a declared type name.
func (m *Model) kindOf(name string) Kind {
	if m.IsClass(name) {
		return KindClass
	}
	if m.absence.AbsenceReason(name) == rminfo.AbsencePrimitive {
		return KindPrimitive
	}
	return KindParameter
}

// Resolve returns the RM type of an object. typ is the object's "_type", and
// hasType is false when the object has none or holds null there. declared is
// the type of the slot that holds the object: the declared type of its
// attribute, the class a type parameter is bound to, or the decode target's
// type for the root; it is empty where nothing declares a type, such as a
// type parameter the slot does not bind.
//
// An object's own "_type" wins. Without one the declared type applies, and it
// must be a concrete RM class. Resolve does not guess: an error says why the
// object's type cannot be read, and names no value other than the "_type".
func (m *Model) Resolve(typ string, hasType bool, declared string) (string, error) {
	if hasType {
		if !m.IsClass(typ) {
			return "", fmt.Errorf("_type %q names no RM class", typ)
		}
		return typ, nil
	}
	if declared == "" {
		return "", errors.New("_type is elided where the RM declares a type parameter with no binding")
	}
	abstract, known := m.hier.IsAbstract(declared)
	if !known {
		return "", fmt.Errorf("_type is elided and the declared type %q is not an RM class", declared)
	}
	if abstract {
		return "", fmt.Errorf("_type is elided where the declared type %s is abstract", declared)
	}
	return declared, nil
}

// Attribute returns what the RM declares about the attribute name of rmType.
// bound is the class the slot holding the object binds rmType's type
// parameter to (an Attribute's Bound), or empty. The second result is false
// when rmType has no attribute name: the member is unmatched.
//
// A type-parameter attribute takes bound as its type; with bound empty its
// Kind stays [KindParameter] and its value must carry its own "_type", unless
// the attribute is a generic payload. A binding overrides the payload rule:
// the slot names the class the value decodes as. The returned Bound is the
// binding the attribute's own value passes down: a DV_INTERVAL attribute
// whose RM type argument is a concrete class, such as
// DV_QUANTITY.normal_range, binds its bounds to that class. A Hash attribute
// whose values are RM objects reports the values' class as its Type.
func (m *Model) Attribute(rmType, name, bound string) (Attribute, bool) {
	typ, ok := m.lookup.AttributeRMType(rmType, name)
	if !ok {
		return Attribute{}, false
	}
	container, _ := m.lookup.IsContainer(rmType, name)
	site, _ := m.hier.DeclaredOn(rmType, name)
	a := Attribute{
		Type:       typ,
		Kind:       m.kindOf(typ),
		Container:  container,
		DeclaredOn: site,
		Bound:      boundBindings[site+"."+name],
	}
	switch {
	case a.Kind == KindParameter && bound != "":
		a.Type, a.Kind = bound, KindClass
	case a.Kind == KindParameter:
		a.Type, a.Payload = "", payloads[rmType+"."+name]
	case typ == "Hash" && hashValues[site+"."+name] != "":
		a.Type, a.Kind, a.Hash = hashValues[site+"."+name], KindClass, true
	}
	return a, true
}
