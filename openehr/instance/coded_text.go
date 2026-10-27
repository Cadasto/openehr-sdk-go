package instance

import (
	"slices"
	"strings"

	"github.com/cadasto/openehr-sdk-go/internal/bmmtype"
	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
	"github.com/cadasto/openehr-sdk-go/openehr/terminology"
)

// classAttribute names an attribute of an RM class.
type classAttribute struct {
	class, attr string
}

// attributeGroups pairs each coded-text attribute an invariant of its RM
// class ties to a group of the openEHR terminology with that group. It is
// the rows of codedRules that name a group, so the table the generator
// keeps a coded attribute to and the one it reads a rubric by are one; the
// rows are the pairs the template-less RM validator checks, which a test
// holds codedRules to.
var attributeGroups = func() map[classAttribute]*terminology.Group {
	groups := make(map[classAttribute]*terminology.Group)
	for _, r := range codedRules {
		if r.group != nil {
			groups[classAttribute{class: r.class, attr: r.attr}] = r.group
		}
	}
	return groups
}()

// useCodedText sets the text of v, a coded text the walk built under node,
// to the text its code has, where node admits the coded text with that
// text (takeText). An openehr code takes its rubric in the group the RM
// ties v's attribute to (nodeGroup); the rubric is the group's, since one
// code can have another rubric in another group. A code of any other
// terminology takes the text of the term node's own archetype defines for
// it (useTermText). A code the group has no rubric for, a code of an
// attribute the RM ties to no group, and a code the archetype defines no
// term for keep the text the walk wrote. A nil node leaves v alone.
func useCodedText(node *tcimpl.CompiledNode, v *rm.DVCodedText) {
	if node == nil {
		return
	}
	if v.DefiningCode.TerminologyID.Value != terminology.ID {
		useTermText(node, v)
		return
	}
	group := nodeGroup(node)
	if group == nil {
		return
	}
	if rubric, ok := group.Rubric(v.DefiningCode.CodeString); ok {
		takeText(node, v, rubric)
	}
}

// nodeGroup returns the group of the openEHR terminology the RM ties to
// the attribute that holds node (attributeGroups), the attribute of
// node's parent read under the class the walk builds that parent as. It
// returns nil when node has no parent or the RM ties that attribute to no
// group.
func nodeGroup(node *tcimpl.CompiledNode) *terminology.Group {
	parent := node.Parent()
	if parent == nil {
		return nil
	}
	class := bmmtype.Class(builtRMType(parent))
	for _, attr := range tcimpl.NodeAttributes(parent) {
		if slices.Contains(tcimpl.ChildNodes(attr), node) {
			return attributeGroups[classAttribute{class: class, attr: attr.Name()}]
		}
	}
	return nil
}

// useTermText sets the text of v, a coded text the walk built under node,
// to the text of the term that node's own archetype, its nearest enclosing
// archetype root, defines for its code (termText), where node admits that
// text (takeText). The text is the same whatever Options.Language asks for:
// an OPT carries one document language, whose term a language it has no
// translation for gets too. A code its own archetype
// defines no term for keeps its text, even where an enclosing archetype
// defines the same code, since that term names another concept; so does
// an openehr code, whose text is its rubric. A locatable's name whose
// code's term text node rejects takes another code node lists instead
// (useListedCode). For an ordinal's symbol node is the C_DV_ORDINAL, which
// constrains the symbol's code and not its text. A nil node leaves v
// alone.
func useTermText(node *tcimpl.CompiledNode, v *rm.DVCodedText) {
	if node == nil {
		return
	}
	text, ok := termText(node, v.DefiningCode)
	if !ok {
		return
	}
	takeText(node, v, text)
	if v.Value != text && holdingAttribute(node) == "name" {
		useListedCode(node, v)
	}
}

// useListedCode gives v, a locatable's name the walk built under node, the
// first code that the C_CODE_PHRASE on node's defining_code lists whose
// term text (termText) node admits, with that text, so that the name's
// text is its code's term. v keeps the code and the text the walk wrote
// where node lists no such code.
func useListedCode(node *tcimpl.CompiledNode, v *rm.DVCodedText) {
	cp, ok := primitiveOf(bindingChild(node, "defining_code")).(constraints.CodePhrase)
	if !ok {
		return
	}
	for _, code := range cp.CodeList {
		listed := *v
		listed.DefiningCode.CodeString = code
		text, ok := termText(node, listed.DefiningCode)
		if !ok {
			continue
		}
		listed.Value = text
		if codedTextAdmits(node, listed) {
			*v = listed
			return
		}
	}
}

// termText returns the text of the term that node's own archetype, its
// nearest enclosing archetype root, defines for code, as
// tcimpl.RootTermItem reads it: under termKey, else, for a code of a
// terminology other than local whose code string is not an at-code
// (atCode), under that code string alone, the key some OPTs give such a
// term. An at-code under its bare key names a term of the archetype's own,
// whatever terminology the code phrase names. ok is false where neither
// key gives a text.
func termText(node *tcimpl.CompiledNode, code rm.CodePhrase) (string, bool) {
	key, ok := termKey(code)
	if !ok {
		return "", false
	}
	if text, ok := tcimpl.RootTermItem(node, key, "text"); ok && text != "" {
		return text, true
	}
	if code.TerminologyID.Value == "local" || atCode(code.CodeString) {
		return "", false
	}
	text, ok := tcimpl.RootTermItem(node, code.CodeString, "text")
	return text, ok && text != ""
}

// termKey returns the key under which an OPT's archetype defines the term
// for code: the code string of a local code, and
// "<terminology id>::<code string>" for a code of any terminology other
// than openehr, such as "SNOMED-CT::248152002", which termText also reads
// under the code string alone. It reports false for an openehr code, a
// code with no terminology, and a code with no code string or the
// placeholder at0000 (noCode).
func termKey(code rm.CodePhrase) (string, bool) {
	tid := code.TerminologyID.Value
	switch {
	case noCode(code.CodeString), tid == "", tid == terminology.ID:
		return "", false
	case tid == "local":
		return code.CodeString, true
	default:
		return tid + "::" + code.CodeString, true
	}
}

// atCode reports whether code is an archetype term code: "at" followed by
// digits, with any dot extensions of digits for a specialisation, such as
// at0010 or at0010.2 (ADL 1.4, Node Identification).
func atCode(code string) bool {
	rest, ok := strings.CutPrefix(code, "at")
	if !ok {
		return false
	}
	for part := range strings.SplitSeq(rest, ".") {
		if part == "" || strings.Trim(part, "0123456789") != "" {
			return false
		}
	}
	return true
}
