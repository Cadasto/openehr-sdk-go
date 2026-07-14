package webtemplate

import (
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

// Build projects a compiled OPT into the typed WebTemplate tree (REQ-106).
//
// The transform follows the EHRbase v2.3 node model (ADR-0014): it keeps
// COMPOSITION / ENTRY / EVENT / EVENT_CONTEXT / CLUSTER, collapses each
// ELEMENT into a value leaf, drops the pure structural wrappers (HISTORY /
// ITEM_TREE / ITEM_LIST / ITEM_STRUCTURE) while folding their node
// predicates into descendant aqlPaths, and emits data-bearing RM
// attributes as leaves.
func Build(c *templatecompile.Compiled, opts ...Option) (*WebTemplate, error) {
	if c == nil || c.Root() == nil {
		return nil, ErrEmptyTemplate
	}
	cfg := &config{version: defaultVersion, defaultLanguage: c.Language()}
	for _, o := range opts {
		o(cfg)
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

	return &WebTemplate{
		TemplateID:      c.TemplateID(),
		Version:         cfg.version,
		DefaultLanguage: cfg.defaultLanguage,
		Languages:       cfg.languages,
		Tree:            tree,
	}, nil
}

// childrenOf walks a kept container's attributes and returns the emitted
// WebTemplate child nodes, with parentPath the container's aqlPath. It then
// appends the fixed "inContext" RM-attribute leaves that EHRbase emits for
// the container regardless of template constraint (composer, subject,
// language, encoding, territory, time, …) when the template did not already
// supply them.
func childrenOf(n *templatecompile.CompiledNode, parentPath string, cfg *config) []*Node {
	var out []*Node
	seen := map[string]bool{}
	for _, attr := range n.Attributes() {
		for _, child := range attr.Children() {
			for _, nd := range emit(child, attr, parentPath, cfg) {
				out = append(out, nd)
				seen[nd.AQLPath] = true
			}
		}
	}
	for _, ic := range inContextLeaves(n.RMTypeName()) {
		p := parentPath + "/" + ic.attr
		if seen[p] {
			continue
		}
		out = append(out, &Node{
			RMType:  ic.rmType,
			AQLPath: p,
			ID:      ic.attr,
			Min:     ic.min,
			Max:     ic.max,
			Inputs:  ic.inputs,
		})
	}
	return out
}

// inContextLeaf is a fixed RM-attribute leaf EHRbase emits for a container.
type inContextLeaf struct {
	attr     string
	rmType   string
	min, max int
	inputs   []Input
}

var (
	partyProxyIC = []Input{
		{Suffix: "id", Type: "TEXT"},
		{Suffix: "id_scheme", Type: "TEXT"},
		{Suffix: "id_namespace", Type: "TEXT"},
		{Suffix: "name", Type: "TEXT"},
	}
	dateTimeIC = []Input{{Type: "DATETIME"}}
	settingIC  = []Input{{Suffix: "code", Type: "TEXT"}, {Suffix: "value", Type: "TEXT"}}
)

// inContextLeaves returns the RM-attribute leaves EHRbase emits for a
// container RM type independent of the template (WebTemplate "inContext"
// nodes), derived from the reference fixture.
func inContextLeaves(containerRM string) []inContextLeaf {
	switch containerRM {
	case "COMPOSITION":
		return []inContextLeaf{
			{attr: "language", rmType: "CODE_PHRASE", min: 0, max: 1},
			{attr: "territory", rmType: "CODE_PHRASE", min: 0, max: 1},
			{attr: "composer", rmType: "PARTY_PROXY", min: 0, max: 1, inputs: partyProxyIC},
		}
	case "EVENT_CONTEXT":
		return []inContextLeaf{
			{attr: "start_time", rmType: "DV_DATE_TIME", min: 0, max: 1, inputs: dateTimeIC},
			{attr: "setting", rmType: "DV_CODED_TEXT", min: 0, max: 1, inputs: settingIC},
		}
	case "OBSERVATION", "EVALUATION", "INSTRUCTION", "ACTION", "ADMIN_ENTRY":
		return []inContextLeaf{
			{attr: "language", rmType: "CODE_PHRASE", min: 0, max: 1},
			{attr: "encoding", rmType: "CODE_PHRASE", min: 0, max: 1},
			{attr: "subject", rmType: "PARTY_PROXY", min: 0, max: 1, inputs: partyProxyIC},
		}
	case "EVENT", "POINT_EVENT", "INTERVAL_EVENT":
		return []inContextLeaf{
			{attr: "time", rmType: "DV_DATE_TIME", min: 0, max: 1, inputs: dateTimeIC},
		}
	}
	return nil
}

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
		var out []*Node
		for _, a := range c.Attributes() {
			for _, gc := range a.Children() {
				out = append(out, emit(gc, a, childPath, cfg)...)
			}
		}
		return out

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
	if t, ok := c.Term(c.NodeID(), cfg.defaultLanguage); ok {
		node.Name = t.Items["text"]
		node.LocalizedName = node.Name
	}
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
// the at-code, else empty (the archetype root's internal at0000 is not a
// nodeId — the archetype id is used instead).
func nodeIDOf(c *templatecompile.CompiledNode) string {
	if a := c.ArchetypeID(); a != "" {
		return a
	}
	if id := c.NodeID(); id != "" && id != "at0000" {
		return id
	}
	return ""
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
