package instance

import (
	"slices"

	"github.com/cadasto/openehr-sdk-go/internal/bmmtype"
	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
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
func useCodedText(node *tcimpl.CompiledNode, v *rm.DVCodedText, lang string) {
	if node == nil {
		return
	}
	if v.DefiningCode.TerminologyID.Value != terminology.ID {
		useTermText(node, v, lang)
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
// archetype root, defines for its code under termKey, where node admits
// that text (takeText). The term is the one tcimpl.RootTermItem reads.
// lang is the language asked for: an OPT carries one document language,
// whose term a language it has no translation for gets too, so the term
// is the same in every lang. A code its own archetype defines no term for
// keeps its text, even where an enclosing archetype defines the same code,
// since that term names another concept; so does an openehr code, whose
// text is its rubric. For an ordinal's symbol node is the C_DV_ORDINAL,
// which constrains the symbol's code and not its text. A nil node leaves
// v alone.
func useTermText(node *tcimpl.CompiledNode, v *rm.DVCodedText, lang string) {
	key, ok := termKey(v.DefiningCode)
	if node == nil || !ok {
		return
	}
	_ = lang // one document language per OPT; see above.
	if text, ok := tcimpl.RootTermItem(node, key, "text"); ok && text != "" {
		takeText(node, v, text)
	}
}

// termKey returns the key under which an OPT's archetype defines the term
// for code: the code string of a local code, and
// "<terminology id>::<code string>" for a code of any terminology other
// than openehr, such as "SNOMED-CT::248152002". It reports false for an
// openehr code, a code with no terminology, and a code with no code string
// or the placeholder at0000 (noCode).
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
