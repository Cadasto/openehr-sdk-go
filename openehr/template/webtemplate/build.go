package webtemplate

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	impl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
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
		if o != nil {
			o(cfg)
		}
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
	dedupeSiblingIDs(tree)
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

// dedupeSiblingIDs gives every sibling a unique id, renaming the second and
// later claimants of a name to the next *free* ordinal spelling: `dv_text`,
// `dv_text2`, `dv_text3`. Visits each sibling group pre-order, in document
// order, so the result is deterministic.
//
// Ordinals are the reference's *last resort*, not its disambiguator of
// choice: REQ-116's template-level node name does that job, and across the
// corona / multi_occurrence / AlternativeEvents goldens no sibling group
// carries a suffix, because each sibling pins a distinct name. The upstream
// FLAT conformance corpus shows the fallback where names run out — that
// OPT's ACTION has two ELEMENTs both carrying the term text "DV_TEXT" (it
// appears 10× in the template) and neither pinning a name, and the upstream
// bodies key them `…/conformance_action/dv_text` and `…/dv_text2`.
//
// Numbering starts at 2 and the first claimant keeps the bare id, so a
// template with no collision is untouched — which is every template that
// could previously be built at all, since a collision used to be a hard
// [ErrIDCollision]. "Next free" matters: siblings sanitising to
// [x, x2, x] must not rename the third to the already-taken `x2` — it takes
// `x3`. Only the two-ELEMENT case is golden-evidenced; the next-free
// extension keeps every other collision loud-failure-free without ever
// emitting a duplicate, and [checkIDCollisions] still guards the invariant.
func dedupeSiblingIDs(n *Node) {
	counts := make(map[string]int, len(n.Children))
	taken := make(map[string]bool, len(n.Children))
	for _, ch := range n.Children {
		taken[ch.ID] = true
	}
	for _, ch := range n.Children {
		counts[ch.ID]++
		if counts[ch.ID] > 1 {
			c := counts[ch.ID]
			for taken[ch.ID+strconv.Itoa(c)] {
				c++
			}
			ch.ID += strconv.Itoa(c)
			taken[ch.ID] = true
		}
		dedupeSiblingIDs(ch)
	}
}

// checkIDCollisions rejects trees where two sibling nodes still share an id
// after [dedupeSiblingIDs]. The next-free ordinal makes sibling ids unique
// by construction, so this firing means a builder bug — it stays as a loud
// guard because an ambiguous FLAT path is far worse than a failed export.
func checkIDCollisions(n *Node) error {
	seen := map[string]bool{}
	for _, ch := range n.Children {
		if seen[ch.ID] {
			return fmt.Errorf("%w: %q under %q", ErrIDCollision, ch.ID, n.AQLPath)
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
		ic.Inputs = cloneInputs(ic.Inputs) // never alias the table's slices or pointers into returned trees
		out = append(out, &ic)
	}
	return out
}

// cloneInputs deep-copies an in-context input set. The table prototypes now
// carry Validation/Range pointers (unconstrainedDurationInputs), so a
// shallow slice clone would share them across every tree ever built — a
// consumer mutating one WebTemplate would corrupt all subsequent Builds.
func cloneInputs(in []Input) []Input {
	out := slices.Clone(in)
	for i := range out {
		out[i].List = slices.Clone(out[i].List)
		for j := range out[i].List {
			it := &out[i].List[j]
			if it.Ordinal != nil {
				o := *it.Ordinal
				it.Ordinal = &o
			}
			it.LocalizedLabels = maps.Clone(it.LocalizedLabels)
		}
		if v := out[i].Validation; v != nil {
			nv := *v
			nv.Range = cloneRange(v.Range)
			nv.Precision = cloneRange(v.Precision)
			out[i].Validation = &nv
		}
	}
	return out
}

func cloneRange(r *Range) *Range {
	if r == nil {
		return nil
	}
	nr := *r
	if r.Min != nil {
		m := *r.Min
		nr.Min = &m
	}
	if r.Max != nil {
		m := *r.Max
		nr.Max = &m
	}
	return &nr
}

// emitAll emits the WebTemplate nodes contributed by all of n's attribute
// children, with parentPath the accumulated aqlPath prefix.
//
// The LOCATABLE `name` attribute is skipped (REQ-116): when an OPT pins a
// node's name it constrains that attribute, and walking it as data emitted
// a spurious `…/name` DV_TEXT leaf the reference never has — the reference
// carries the name *on* the node (its `id` and its aqlPath predicate),
// never as a child. Measured against the goldens, dropping it is exactly
// right: it removes 4 surplus nodes on GECCO and 21 on Corona, and neither
// golden has a `…/name` node anywhere.
func emitAll(n *templatecompile.CompiledNode, parentPath string, cfg *config) []*Node {
	var out []*Node
	for _, attr := range n.Attributes() {
		if attr.Name() == "name" {
			continue
		}
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
	// INTERVAL_EVENT adds the interval descriptors on top of EVENT's time.
	// Width's DV_DURATION inputs follow the reference's per-field shape.
	intervalEventIC = append(
		slices.Clone(eventIC),
		Node{ID: "math_function", Name: "Math_function", RMType: "DV_CODED_TEXT", Max: 1, Inputs: settingIC},
		Node{ID: "width", Name: "Width", RMType: "DV_DURATION", Max: 1, Inputs: unconstrainedDurationInputs()},
	)
	instructionIC = append(
		slices.Clone(entryIC),
		Node{ID: "narrative", Name: "Narrative", RMType: "DV_TEXT", Max: 1, Inputs: []Input{{Type: "TEXT"}}},
		// The reference leaves expiry_time's name lower-case, unlike every
		// other in-context leaf — copied verbatim rather than normalised.
		Node{ID: "expiry_time", Name: "expiry_time", RMType: "DV_DATE_TIME", Max: 1, Inputs: dateTimeIC},
	)
	activityIC = []Node{
		{ID: "timing", Name: "Timing", RMType: "DV_PARSABLE", Max: 1, Inputs: []Input{{Suffix: "value", Type: "TEXT"}, {Suffix: "formalism", Type: "TEXT"}}},
		{ID: "action_archetype_id", Name: "Action_archetype_id", RMType: "STRING", Max: 1, Inputs: []Input{{Type: "TEXT"}}},
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
		"OBSERVATION": entryIC, "EVALUATION": entryIC, "INSTRUCTION": instructionIC,
		"ACTION": entryIC, "ADMIN_ENTRY": entryIC, "ACTIVITY": activityIC,
		"EVENT": eventIC, "POINT_EVENT": eventIC, "INTERVAL_EVENT": intervalEventIC,
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

	case isCollapsedEvent(c):
		// Lifted like a dropped wrapper, but through childrenOf: the
		// reference still emits the EVENT's in-context `time` leaf under
		// the (retained) events[…] path segment.
		return childrenOf(c, childPath, cfg)

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
	// "other" input, and the coded list becomes open (the free text admits
	// values beyond the enumerated codes), mirroring EHRbase.
	if v.RMTypeName() == "DV_CODED_TEXT" && hasTextAlternative(alts[1:]) {
		if len(leaf.Inputs) > 0 {
			leaf.Inputs[0].ListOpen = true // the "code" input
		}
		leaf.Inputs = append(leaf.Inputs, Input{Suffix: "other", Type: "TEXT"})
	}
	return leaf
}

// hasTextAlternative reports whether any of the value alternatives is a
// plain DV_TEXT (the "other, please specify" free-text option).
func hasTextAlternative(alts []*templatecompile.CompiledNode) bool {
	return slices.ContainsFunc(alts, func(a *templatecompile.CompiledNode) bool {
		return a.RMTypeName() == "DV_TEXT"
	})
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
// values carry no predicate). A node that pins a template-level name
// carries it inside the same brackets (REQ-116) — the reference's
// `[archetype_id,'Name']` form.
//
// This builder composes its own paths rather than reading
// CompiledNode.AQLPath (it drops structural wrappers, so its paths
// legitimately differ), which is why the predicate rule has to be applied
// here too; the quoting itself is shared with the compiled-path builder
// via impl.NamePredicate so the two cannot drift.
func predicate(c *templatecompile.CompiledNode) string {
	if id := nodeIDOf(c); id != "" {
		return "[" + impl.NamePredicate(id, c.NodeName()) + "]"
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

// isCollapsedEvent reports whether c is an abstract EVENT that can occur at
// most once — a degenerate wrapper the reference lifts rather than emitting
// as a node, keeping its events[…] path segment and its `time` leaf.
//
// Reverse-engineered from the goldens, and the discriminator is both the RM
// type and the cardinality — neither alone fits. Every case across the three
// vendored references agrees: corona drops 14 EVENT nodes, all max=1, and
// keeps 2 EVENT at max=-1; constrain_test keeps 3 EVENT at max=-1 *and* an
// INTERVAL_EVENT `a24_hour_average` at max=1. A concrete POINT_EVENT /
// INTERVAL_EVENT names a clinical concept worth addressing even when single;
// a bare EVENT that cannot repeat adds no addressable structure.
func isCollapsedEvent(c *templatecompile.CompiledNode) bool {
	if c.RMTypeName() != "EVENT" {
		return false
	}
	occ := c.Occurrences()
	if occ == nil {
		return true // absent occurrences default to 0..1
	}
	return !occ.UpperUnbounded() && occ.Upper() == 1
}

// isValueLeaf reports whether an RM type is a data value emitted as a leaf
// (no kept children; its constraint becomes inputs).
func isValueLeaf(rmType string) bool {
	return strings.HasPrefix(rmType, "DV_") || rmType == "CODE_PHRASE" || rmType == "PARTY_PROXY"
}
