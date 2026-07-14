package webtemplate

import (
	"fmt"
	"slices"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

// Build projects a compiled OPT into the typed WebTemplate tree (REQ-106).
//
// The transform follows the EHRbase v2.3 node model (ADR-0014): it keeps
// COMPOSITION / ENTRY / EVENT / EVENT_CONTEXT / CLUSTER, collapses each
// ELEMENT into a value leaf, drops the pure structural wrappers (HISTORY
// and the ITEM_STRUCTURE family — ITEM_TREE / ITEM_LIST / ITEM_SINGLE /
// ITEM_TABLE) while folding their node predicates into descendant
// aqlPaths, and emits data-bearing RM attributes as leaves.
func Build(c *templatecompile.Compiled, opts ...Option) (*WebTemplate, error) {
	if c == nil || c.Root() == nil {
		return nil, ErrEmptyTemplate
	}
	cfg := &config{defaultLanguage: c.Language()}
	for _, o := range opts {
		o(cfg)
	}
	if cfg.defaultLanguage == "" {
		return nil, ErrNoDefaultLanguage
	}
	if len(cfg.languages) == 0 {
		cfg.languages = []string{cfg.defaultLanguage}
	}

	root := c.Root()
	tree := &Node{
		RMType:  root.RMTypeName(),
		NodeID:  nodeIDOf(root),
		AQLPath: "",
		ID:      idOf(root, "", cfg),
	}
	setOccurrences(tree, root)
	setNames(tree, root, cfg)
	tree.Children = childrenOf(root, "", cfg)
	if err := checkIDCollisions(tree); err != nil {
		return nil, err
	}

	return &WebTemplate{
		TemplateID:      c.TemplateID(),
		Version:         defaultVersion,
		DefaultLanguage: cfg.defaultLanguage,
		Languages:       cfg.languages,
		Tree:            tree,
	}, nil
}

// checkIDCollisions rejects trees where two sibling nodes share an id. The
// reference disambiguates such siblings; until that rule is implemented
// (deviations.md) the export fails loudly rather than emit ambiguous
// FLAT paths.
func checkIDCollisions(n *Node) error {
	seen := map[string]bool{}
	for _, ch := range n.Children {
		if seen[ch.ID] {
			return fmt.Errorf("%w: %q under %q (sibling disambiguation not implemented — see deviations.md)", ErrIDCollision, ch.ID, n.AQLPath)
		}
		seen[ch.ID] = true
		if err := checkIDCollisions(ch); err != nil {
			return err
		}
	}
	return nil
}

// childrenOf walks a kept container's attributes and returns the emitted
// WebTemplate child nodes, with parentPath the container's aqlPath. It then
// appends the fixed "inContext" RM-attribute leaves that EHRbase emits for
// the container regardless of template constraint (composer, subject,
// language, encoding, territory, time, …) when the template did not already
// supply them.
func childrenOf(n *templatecompile.CompiledNode, parentPath string, cfg *config) []*Node {
	out := emitAll(n, parentPath, cfg)
	seen := map[string]bool{}
	for _, nd := range out {
		seen[nd.AQLPath] = true
	}
	for _, proto := range inContextByRM[n.RMTypeName()] {
		ic := proto // per-leaf copy; the table entries stay pristine
		ic.AQLPath = parentPath + "/" + ic.ID
		if seen[ic.AQLPath] {
			continue
		}
		ic.Inputs = slices.Clone(ic.Inputs) // never alias the table's slices into returned trees
		out = append(out, &ic)
	}
	return out
}

// emitAll emits the WebTemplate nodes contributed by all of n's attribute
// children, with parentPath the accumulated aqlPath prefix.
func emitAll(n *templatecompile.CompiledNode, parentPath string, cfg *config) []*Node {
	var out []*Node
	for _, attr := range n.Attributes() {
		for _, child := range attr.Children() {
			out = append(out, emit(child, attr, parentPath, cfg)...)
		}
	}
	return out
}

var (
	partyProxyIC = partyProxyInputs()
	dateTimeIC   = []Input{{Type: "DATETIME"}}
	settingIC    = []Input{{Suffix: "code", Type: "TEXT"}, {Suffix: "value", Type: "TEXT"}}

	entryIC = []Node{
		{ID: "language", Name: "Language", RMType: "CODE_PHRASE", Max: 1},
		{ID: "encoding", Name: "Encoding", RMType: "CODE_PHRASE", Max: 1},
		{ID: "subject", Name: "Subject", RMType: "PARTY_PROXY", Max: 1, Inputs: partyProxyIC},
	}
	eventIC = []Node{
		{ID: "time", Name: "Time", RMType: "DV_DATE_TIME", Max: 1, Inputs: dateTimeIC},
	}

	// inContextByRM lists the fixed RM-attribute leaves EHRbase emits per
	// container RM type independent of the template (WebTemplate
	// "inContext" nodes), derived from the reference fixture. The ID
	// doubles as the RM attribute name; AQLPath is stamped at emission.
	inContextByRM = map[string][]Node{
		"COMPOSITION": {
			{ID: "language", Name: "Language", RMType: "CODE_PHRASE", Max: 1},
			{ID: "territory", Name: "Territory", RMType: "CODE_PHRASE", Max: 1},
			{ID: "composer", Name: "Composer", RMType: "PARTY_PROXY", Max: 1, Inputs: partyProxyIC},
		},
		"EVENT_CONTEXT": {
			{ID: "start_time", Name: "Start_time", RMType: "DV_DATE_TIME", Max: 1, Inputs: dateTimeIC},
			{ID: "setting", Name: "Setting", RMType: "DV_CODED_TEXT", Max: 1, Inputs: settingIC},
		},
		"OBSERVATION": entryIC, "EVALUATION": entryIC, "INSTRUCTION": entryIC,
		"ACTION": entryIC, "ADMIN_ENTRY": entryIC,
		"EVENT": eventIC, "POINT_EVENT": eventIC, "INTERVAL_EVENT": eventIC,
	}
)

// emit returns the WebTemplate node(s) contributed by compiled child c,
// reached from its parent through attribute attr whose parent sits at
// parentPath. A dropped structural wrapper contributes its lifted kept
// descendants (its predicate stays in the accumulated path); an ELEMENT
// contributes a single collapsed value leaf; a value type contributes a
// leaf; any other kept container contributes a node with recursed children.
func emit(c *templatecompile.CompiledNode, attr *templatecompile.CompiledAttribute, parentPath string, cfg *config) []*Node {
	childPath := parentPath + "/" + attr.Name() + predicate(c)

	switch {
	case isDroppedContainer(c.RMTypeName()):
		return emitAll(c, childPath, cfg)

	case c.RMTypeName() == "ELEMENT":
		if leaf := collapseElement(c, childPath, attr, cfg); leaf != nil {
			return []*Node{leaf}
		}
		return nil // ELEMENT with no value constraint — EHRbase omits it

	case isValueLeaf(c.RMTypeName()):
		leaf := newNode(c, childPath, attr, cfg)
		leaf.Inputs = inputsFor(c)
		return []*Node{leaf}

	default: // kept container
		node := newNode(c, childPath, attr, cfg)
		node.Children = childrenOf(c, childPath, cfg)
		if len(node.Children) == 0 {
			return nil // EHRbase prunes empty containers (e.g. unfilled slots)
		}
		return []*Node{node}
	}
}

// collapseElement folds an ELEMENT and its value into a single leaf whose
// rmType is the constrained value type, nodeId is the ELEMENT's node id,
// and aqlPath is the ELEMENT path extended by /value.
func collapseElement(el *templatecompile.CompiledNode, elPath string, attr *templatecompile.CompiledAttribute, cfg *config) *Node {
	va := el.Attribute("value")
	if va == nil || len(va.Children()) == 0 {
		return nil // no constrained value — EHRbase omits the ELEMENT
	}
	alts := va.Children()
	v := alts[0] // primary value alternative
	leaf := newNode(el, elPath, attr, cfg)
	leaf.RMType = v.RMTypeName()
	leaf.AQLPath = elPath + "/value"
	leaf.Inputs = inputsFor(v)
	// A DV_CODED_TEXT with a DV_TEXT alternative renders an extra free-text
	// "other" input, mirroring EHRbase.
	if v.RMTypeName() == "DV_CODED_TEXT" && hasTextAlternative(alts[1:]) {
		leaf.Inputs = append(leaf.Inputs, Input{Suffix: "other", Type: "TEXT"})
	}
	return leaf
}

// hasTextAlternative reports whether any of the value alternatives is a
// plain DV_TEXT (the "other, please specify" free-text option).
func hasTextAlternative(alts []*templatecompile.CompiledNode) bool {
	for _, a := range alts {
		if a.RMTypeName() == "DV_TEXT" {
			return true
		}
	}
	return false
}

// newNode builds the common fields of a WebTemplate node from a compiled node.
func newNode(c *templatecompile.CompiledNode, aqlPath string, attr *templatecompile.CompiledAttribute, cfg *config) *Node {
	node := &Node{
		RMType:  c.RMTypeName(),
		NodeID:  nodeIDOf(c),
		AQLPath: aqlPath,
		ID:      idOf(c, attr.Name(), cfg),
	}
	setOccurrences(node, c)
	setNames(node, c, cfg)
	return node
}

func setOccurrences(node *Node, c *templatecompile.CompiledNode) {
	occ := c.Occurrences()
	if occ == nil {
		node.Min, node.Max = 0, 1
		return
	}
	node.Min = occ.Lower()
	if occ.UpperUnbounded() {
		node.Max = -1
	} else {
		node.Max = occ.Upper()
	}
}

func setNames(node *Node, c *templatecompile.CompiledNode, cfg *config) {
	node.Name = termText(c, cfg.defaultLanguage)
	node.LocalizedName = node.Name
	for _, lang := range cfg.languages {
		t, ok := c.Term(c.NodeID(), lang)
		if !ok {
			continue
		}
		if txt := t.Items["text"]; txt != "" {
			if node.LocalizedNames == nil {
				node.LocalizedNames = map[string]string{}
			}
			node.LocalizedNames[lang] = txt
		}
		if d := t.Items["description"]; d != "" {
			if node.LocalizedDescriptions == nil {
				node.LocalizedDescriptions = map[string]string{}
			}
			node.LocalizedDescriptions[lang] = d
		}
	}
}

// predicate returns the aqlPath node predicate for c: its archetype id if
// it is a slot/archetype root, else its at-code, else empty (RM-attribute
// values carry no predicate).
func predicate(c *templatecompile.CompiledNode) string {
	if id := nodeIDOf(c); id != "" {
		return "[" + id + "]"
	}
	return ""
}

// nodeIDOf returns the WebTemplate nodeId: archetype id when present, else
// the at-code, else empty (the archetype root's internal at0000 — or a
// specialized at0000.1 — is not a nodeId; the archetype id is used instead).
func nodeIDOf(c *templatecompile.CompiledNode) string {
	if a := c.ArchetypeID(); a != "" {
		return a
	}
	if id := c.NodeID(); id != "" && !isArchetypeRootCode(id) {
		return id
	}
	return ""
}

// isArchetypeRootCode reports whether an at-code is the archetype root
// concept (at0000, or a specialized at0000.N…).
func isArchetypeRootCode(id string) bool {
	return id == "at0000" || strings.HasPrefix(id, "at0000.")
}

// isDroppedContainer reports whether an RM type is a pure structural
// wrapper that is dropped as a node (its predicate folds into the path).
func isDroppedContainer(rmType string) bool {
	switch rmType {
	case "HISTORY", "ITEM_TREE", "ITEM_LIST", "ITEM_STRUCTURE", "ITEM_SINGLE", "ITEM_TABLE":
		return true
	}
	return false
}

// isValueLeaf reports whether an RM type is a data value emitted as a leaf
// (no kept children; its constraint becomes inputs).
func isValueLeaf(rmType string) bool {
	return strings.HasPrefix(rmType, "DV_") || rmType == "CODE_PHRASE" || rmType == "PARTY_PROXY"
}
