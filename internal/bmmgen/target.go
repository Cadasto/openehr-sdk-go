package bmmgen

import "strings"

// Target describes one generation destination: which BMM root to
// load, which Go package name to emit, where on disk to write it,
// and how to handle cross-target class references.
//
// There are two targets, RM and AOM 1.4. They share
// `openehr_base_1.3.0` via include, but the AOM target does not re-emit
// base classes; it references them as `rm.<GoName>` through a one-way
// `aom14 → rm` dependency.
type Target struct {
	// RootID is the BMM include id to load (no .bmm.json suffix).
	// Example: "openehr_rm_1.2.0" or "openehr_am_1.4.0".
	RootID string
	// GoPackage is the Go package name written in the file header.
	// Example: "rm", "aom14".
	GoPackage string
	// OutSubDir is the sub-directory under OutDir where _gen.go files
	// land. Example: "openehr/rm", "openehr/aom/aom14".
	OutSubDir string
	// SourceLabel is rendered into the generated-file header after
	// "// Source: ". Example:
	// "openehr_rm_1.2.0.bmm.json + openehr_base_1.3.0.bmm.json".
	SourceLabel string
	// OwnPackagePrefixes is the list of BMM package dotted-name
	// prefixes whose classes this target *owns* (emits). Classes whose
	// enclosing BMM package starts with one of these prefixes are
	// emitted in this target; classes elsewhere are treated as
	// "external" and referenced via [ExternalQualifier] (if non-empty)
	// or `any` (if empty).
	//
	// For the RM target this is empty: RM owns every class that
	// survives the skip rules (including base types).
	//
	// For the AOM target this is ["org.openehr.am."]: only AOM-namespaced
	// classes are emitted. Everything else (base classes, RM types
	// transitively included by AOM properties) is rendered as
	// `rm.<GoName>`.
	OwnPackagePrefixes []string
	// ExternalQualifier is the Go package qualifier prepended to class
	// references that fall outside [OwnPackagePrefixes]. Example: "rm".
	// Empty means "no qualifier": the class is assumed to live in the
	// same package (used for the RM target).
	ExternalQualifier string
	// ExternalImport is the import path corresponding to
	// [ExternalQualifier]. Empty when ExternalQualifier is empty.
	// Example: "github.com/cadasto/openehr-sdk-go/openehr/rm".
	ExternalImport string
	// RegistryImport is the import path for the typereg package used
	// by this target. Both RM and AOM share the same registry:
	// "github.com/cadasto/openehr-sdk-go/openehr/rm/typereg". The
	// _type discriminator strings are disjoint between the two models.
	RegistryImport string
	// ITSXMLSchemaSets lists the ITS-XML schema sets the target's XML
	// encoders take their child order from, each by the root files of its
	// closure: [path.Match] patterns relative to the ITS-XML directory. A
	// class is looked up in the sets in this order; the first is the
	// target's own. Empty leaves the encoders in the generator's property
	// order.
	ITSXMLSchemaSets [][]string
	// XMLTypeNames maps the BMM name of a class to the name the target's
	// own ITS-XML schema set gives it, for each class whose two names
	// differ. The XML encoder writes the schema name as the class's
	// `xsi:type`, the decoder reads either name, and the class takes its
	// child order from the schema type of that name. Canonical JSON keeps
	// the BMM name. Empty when the schemas use the BMM names.
	XMLTypeNames map[string]string
	// XMLIdentifierElements maps a String member, keyed "CLASS.member" by
	// the BMM names of the class that declares it and of the member, to the
	// element the class's ITS-XML schema type declares in its place: one of
	// another name, of an identifier class whose `value` holds the string.
	// The XML encoder writes that element, the decoder reads it and the
	// member's own element, and the child order places the member where
	// the schema places the element. Canonical JSON keeps the member. Empty
	// when the schemas declare each member under its own name.
	XMLIdentifierElements map[string]XMLIdentifierElement
}

// XMLIdentifierElement is the element an ITS-XML schema type declares in
// place of a String member ([Target.XMLIdentifierElements]).
type XMLIdentifierElement struct {
	// Name is the element's name, such as terminology_id.
	Name string
	// Type is the BMM name of the element's class, such as TERMINOLOGY_ID,
	// whose `value` member holds the string.
	Type string
}

// TargetRM is the RM generation target.
var TargetRM = Target{
	RootID:             "openehr_rm_1.2.0",
	GoPackage:          "rm",
	OutSubDir:          "openehr/rm",
	SourceLabel:        "openehr_rm_1.2.0.bmm.json + openehr_base_1.3.0.bmm.json",
	OwnPackagePrefixes: nil, // RM owns all surviving classes.
	ExternalQualifier:  "",
	ExternalImport:     "",
	RegistryImport:     "github.com/cadasto/openehr-sdk-go/openehr/rm/typereg",
	ITSXMLSchemaSets:   [][]string{itsXMLRMRoots, itsXMLAMRoots},
}

// TargetAOM14 is the AOM 1.4 generation target. Emits to
// `openehr/aom/aom14/`. AOM-defined classes are emitted locally;
// base/RM classes referenced by AOM (e.g. AUTHORED_RESOURCE,
// ARCHETYPE_ID, HIER_OBJECT_ID, VALIDITY_KIND) are imported from
// the rm package as `rm.<GoName>`.
var TargetAOM14 = Target{
	RootID:             "openehr_am_1.4.0",
	GoPackage:          "aom14",
	OutSubDir:          "openehr/aom/aom14",
	SourceLabel:        "openehr_am_1.4.0.bmm.json + openehr_base_1.3.0.bmm.json",
	OwnPackagePrefixes: []string{"org.openehr.am."},
	ExternalQualifier:  "rm",
	ExternalImport:     "github.com/cadasto/openehr-sdk-go/openehr/rm",
	RegistryImport:     "github.com/cadasto/openehr-sdk-go/openehr/rm/typereg",
	ITSXMLSchemaSets:   [][]string{itsXMLAMRoots, itsXMLRMRoots},
	// The AM 1.4 OpenehrProfile.xsd names three openEHR-profile
	// constraint classes after the data value they constrain.
	XMLTypeNames: map[string]string{
		"C_CODED_TEXT": "C_CODE_PHRASE",
		"C_ORDINAL":    "C_DV_ORDINAL",
		"C_QUANTITY":   "C_DV_QUANTITY",
	},
	// Its C_CODE_PHRASE declares C_CODED_TEXT's terminology as the
	// TERMINOLOGY_ID element terminology_id.
	XMLIdentifierElements: map[string]XMLIdentifierElement{
		"C_CODED_TEXT.terminology": {Name: "terminology_id", Type: "TERMINOLOGY_ID"},
	},
}

// DefaultTargets is the set of targets emitted when the CLI is invoked
// with no explicit target list.
func DefaultTargets() []Target {
	return []Target{TargetRM, TargetAOM14}
}

// owns reports whether t emits the given BMM package path. Empty
// OwnPackagePrefixes means "owns everything" (the RM target).
func (t Target) owns(pkgPath string) bool {
	if len(t.OwnPackagePrefixes) == 0 {
		return true
	}
	for _, p := range t.OwnPackagePrefixes {
		if strings.HasPrefix(pkgPath, p) {
			return true
		}
	}
	return false
}
