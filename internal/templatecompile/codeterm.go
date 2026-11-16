package templatecompile

// codeterm.go: the term text of a code a template lists, as a coded name or
// value reads it. The compile step names a node by it, and the FLAT and
// STRUCTURED decode completes a coded value with it, so the rule has this one
// home.

import (
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/terminology"
)

// localTerminology is the terminology id of the codes an archetype defines
// for itself.
const localTerminology = "local"

// CodeTermText returns the term text of code in terminologyID, as a coded
// name or value under the node n reads it. A code of the openEHR terminology
// takes its rubric, where every group that holds the code gives it the same
// one. Any other takes the text of the term that n's own archetype, its
// nearest enclosing archetype root, defines for it ([RootTermItem]): under
// the code for a local code, else under "<terminology>::<code>", or under the
// code alone where that is no at-code, the key some OPTs give such a term. ok
// is false where none gives a text. It is a function rather than a method so
// that the public alias of [CompiledNode] gains no method.
func CodeTermText(n *CompiledNode, terminologyID, code string) (string, bool) {
	return codeTermText(rootTermText(n), terminologyID, code)
}

// rootTermText returns the term-text reader of [CodeTermText] for the node n:
// the text of the term n's nearest enclosing archetype root defines under a
// key, or "".
func rootTermText(n *CompiledNode) func(key string) string {
	return func(key string) string {
		text, _ := RootTermItem(n, key, "text")
		return text
	}
}

// codeTermText is [CodeTermText] over termText, which returns the text of the
// term the code's own archetype defines under a key, or "" where it defines
// none or one with no text.
func codeTermText(termText func(key string) string, terminologyID, code string) (string, bool) {
	switch terminologyID {
	case terminology.ID:
		return openehrRubric(code)
	case localTerminology:
		text := termText(code)
		return text, text != ""
	}
	if text := termText(terminologyID + "::" + code); text != "" {
		return text, true
	}
	if isAtCode(code) {
		return "", false
	}
	text := termText(code)
	return text, text != ""
}

// openehrRubric returns the rubric the openEHR terminology gives code. A
// rubric belongs to a code within a group, and the RM ties a node's name to
// no group, so code has a rubric only where every group that holds it gives
// it the same one: 532 is "complete" in one group and "completed" in another,
// and has none.
func openehrRubric(code string) (string, bool) {
	rubric := ""
	for g := range terminology.Groups() {
		r, ok := g.Rubric(code)
		if !ok {
			continue
		}
		if rubric != "" && r != rubric {
			return "", false
		}
		rubric = r
	}
	return rubric, rubric != ""
}

// isAtCode reports whether code is an archetype term code: "at" followed by
// digits, with any dot extensions of digits for a specialisation, such as
// at0010 or at0010.2 (ADL 1.4, Node Identification).
func isAtCode(code string) bool {
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
