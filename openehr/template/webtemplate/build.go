package webtemplate

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	impl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

// Build projects a compiled OPT into the typed WebTemplate tree.
//
// The transform follows the EHRbase v2.3 node model: it keeps
// COMPOSITION / ENTRY / EVENT / EVENT_CONTEXT / CLUSTER, drops the pure
// structural wrappers (HISTORY and the ITEM_STRUCTURE family: ITEM_TREE /
// ITEM_LIST / ITEM_SINGLE / ITEM_TABLE) while folding their node predicates
// into descendant aqlPaths, and emits data-bearing RM attributes as leaves.
//
// An ELEMENT whose value admits one type collapses into a single value leaf
// at the ELEMENT's path extended by /value. So does one admitting a coded
// text and a free text, in either order: its leaf is the coded text, with an
// extra free-text "other" input. An ELEMENT admitting any other set of
// types stays a node with one child per type, all at that /value path,
// each with an id spelled from the type ("quantity_value",
// "interval_of_date_time_value"). One that constrains no value type stays a
// node with a child for each of the 17 value types EHRbase offers it.
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
	text := termText(root)
	tree := &Node{
		RMType:  root.RMTypeName(),
		NodeID:  nodeIDOf(root),
		AQLPath: "",
		ID:      idOf(root, "", text),
	}
	setOccurrences(tree, root)
	setNames(tree, root, text, cfg)
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

// dedupeSiblingIDs gives every sibling a unique id. The first claimant of an
// id keeps it, and each later claimant takes the next free `<id>_N`, N from
// 1: `dv_text`, `dv_text_1`, `dv_text_2`, the Simplified Formats spelling of
// a duplicate sibling. Visits each sibling group pre-order, in document
// order, so the result is deterministic.
//
// The suffix is a last resort, not the disambiguator: the template-level
// node name separates siblings that reuse one archetype, so no sibling group
// in the corona / multi_occurrence / AlternativeEvents goldens needs one.
// Siblings still collide where nothing names them apart, as the two ELEMENTs
// under the upstream FLAT conformance template's ACTION do: both carry the
// term text "DV_TEXT" and neither pins a name.
//
// "Free" is over the whole sibling list, in either document order: a sibling
// named "Blood Pressure 1" keeps blood_pressure_1, and the duplicate of a
// "Blood Pressure" takes blood_pressure_2, since renaming onto a taken id
// would recreate the collision. [checkIDCollisions] still guards the
// invariant.
//
// The reference writes `dv_text2`, `dv_text3`: deviations.md records the
// difference.
func dedupeSiblingIDs(n *Node) {
	taken := make(map[string]bool, len(n.Children))
	for _, ch := range n.Children {
		taken[ch.ID] = true
	}
	claimed := make(map[string]bool, len(n.Children))
	for _, ch := range n.Children {
		if !claimed[ch.ID] {
			claimed[ch.ID] = true
		} else {
			i := 1
			for taken[ch.ID+"_"+strconv.Itoa(i)] {
				i++
			}
			ch.ID += "_" + strconv.Itoa(i)
			taken[ch.ID] = true
		}
		dedupeSiblingIDs(ch)
	}
}

// checkIDCollisions rejects trees where two sibling nodes still share an id
// after [dedupeSiblingIDs]. The next free suffix makes sibling ids unique by
// construction, so this firing means a builder bug — it stays as a loud
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
	out := emitAll(nil, n, parentPath, cfg)
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
	if path := parentPath + "/ism_transition"; n.RMTypeName() == "ACTION" && !seen[path] {
		out = append(out, ismTransitionIC(parentPath, out))
	}
	return out
}

// ismTransitionIC returns the reference's in-context ISM transition node for
// the ACTION at actionPath whose other emitted children are siblings. ACTION
// `ism_transition` is RM-mandatory, so without the node the FLAT encoder would
// have nowhere to write it.
//
// Where the template models the transition as archetyped
// `ism_transition[atNNNN]` nodes, one per careflow step, the node merges the
// ones the walk emitted ([mergedIsmTransition]); those nodes stay beside it.
// They are read from the emitted nodes rather than from the template, because
// an archetyped transition that constrains none of its attributes is pruned
// as an empty container and so carries nothing. Where the walk emitted none,
// the node is the unconstrained transition: its
// `current_state`, a code of the openehr terminology written as `|code` and
// `|value` as EVENT_CONTEXT `setting` is, and its `transition`. The reference
// leaves `careflow_step` out there, and writes a `_reason` family under either
// shape, which the builder does not model. That node's min is 0, the
// in-context convention (§ REQ-106 deviation 5), and its name follows the
// in-context capitalisation (`Start_time`).
func ismTransitionIC(actionPath string, siblings []*Node) *Node {
	path := actionPath + "/ism_transition"
	var steps []*Node
	for _, s := range siblings {
		if s.RMType == "ISM_TRANSITION" && s.NodeID != "" {
			steps = append(steps, s)
		}
	}
	if len(steps) > 0 {
		return mergedIsmTransition(path, steps)
	}
	return &Node{
		ID: "ism_transition", Name: "Ism_transition", RMType: "ISM_TRANSITION", Max: 1, AQLPath: path,
		Children: []*Node{unconstrainedCurrentState(path), transitionIC(path)},
	}
}

// mergedIsmTransition returns the in-context ISM transition node at path that
// the reference builds from an ACTION's archetyped transitions, steps, in
// document order (OPTParser.handleActionIsmTransitions). Its min and max are
// the first step's, and it holds three leaves:
//
//   - careflow_step, listing every step's careflow-step codes, each with the
//     current_state codes that step admits as its currentStates; local codes,
//     so the input names no terminology, as on any coded input. A step that
//     constrains no careflow_step still names one, its node id, which decode
//     gives a transition spelled under it, so that step is listed by its node
//     id, labelled with its name, where the reference fails on it;
//   - current_state, listing every step's current_state codes in step order,
//     each step's list as is, so a state two steps admit is listed twice, in
//     the openehr terminology;
//   - transition, the first step's own transition leaf where that step
//     constrains one, else the open leaf of the unconstrained node
//     ([mergedTransition]).
//
// A leaf's min is the first step's that has it, else 0, and its max 1. A step
// that constrains no current_state adds nothing to that list; with no step
// constraining one, current_state is the unconstrained one. A list is open
// where a step's own list is, or where it lists no code.
func mergedIsmTransition(path string, steps []*Node) *Node {
	careflow := &Node{ID: "careflow_step", Name: "Careflow_step", RMType: "DV_CODED_TEXT", Max: 1, AQLPath: path + "/careflow_step"}
	state := &Node{ID: "current_state", Name: "Current_state", RMType: "DV_CODED_TEXT", Max: 1, AQLPath: path + "/current_state"}
	careflowIn := Input{Suffix: "code", Type: "CODED_TEXT"}
	stateIn := Input{Suffix: "code", Type: "CODED_TEXT", Terminology: "openehr"}
	var hasCareflow, hasState bool
	for _, s := range steps {
		stepState := childAt(s, s.AQLPath+"/current_state")
		states, stateOpen := codeInput(stepState)
		if stepState != nil {
			if !hasState {
				state.Min, hasState = stepState.Min, true
			}
			stateIn.ListOpen = stateIn.ListOpen || stateOpen
			stateIn.List = append(stateIn.List, states...)
		}
		stepCareflow := childAt(s, s.AQLPath+"/careflow_step")
		items, open := codeInput(stepCareflow)
		switch {
		case stepCareflow == nil:
			items = []InputListItem{{Value: s.NodeID, Label: s.Name}}
		case !hasCareflow:
			careflow.Min, hasCareflow = stepCareflow.Min, true
		}
		careflowIn.ListOpen = careflowIn.ListOpen || open
		for _, it := range items {
			for _, st := range states {
				it.CurrentStates = append(it.CurrentStates, st.Value)
			}
			careflowIn.List = append(careflowIn.List, it)
		}
	}
	careflowIn.ListOpen = careflowIn.ListOpen || len(careflowIn.List) == 0
	stateIn.ListOpen = stateIn.ListOpen || len(stateIn.List) == 0
	careflow.Inputs = []Input{careflowIn}
	if hasState {
		state.Inputs = []Input{stateIn}
	} else {
		state = unconstrainedCurrentState(path)
	}
	return &Node{
		ID: "ism_transition", Name: "ism_transition", RMType: "ISM_TRANSITION", Min: steps[0].Min, Max: steps[0].Max, AQLPath: path,
		Children: []*Node{careflow, state, mergedTransition(path, steps[0])},
	}
}

// mergedTransition returns the transition leaf of the merged in-context ISM
// transition node at path whose first step is first: a copy of that step's own
// transition leaf, with its min, max and inputs, at path extended by
// /transition and named "transition", where the step constrains one, else the
// open leaf ([transitionIC]). Only the first step counts, as the reference
// takes the first step's transition whether it constrains one or not. The
// step stays beside the merged node, so the copy shares nothing with it.
func mergedTransition(path string, first *Node) *Node {
	own := childAt(first, first.AQLPath+"/transition")
	if own == nil {
		return transitionIC(path)
	}
	return &Node{
		ID: "transition", Name: "transition", RMType: own.RMType, Min: own.Min, Max: own.Max,
		AQLPath: path + "/transition", Inputs: cloneInputs(own.Inputs),
	}
}

// unconstrainedCurrentState returns the current_state leaf of the in-context
// ISM transition node at path for a template that constrains none.
func unconstrainedCurrentState(path string) *Node {
	return &Node{
		ID: "current_state", Name: "Current_state", RMType: "DV_CODED_TEXT", Max: 1,
		AQLPath: path + "/current_state", Inputs: cloneInputs(settingIC),
	}
}

// transitionIC returns the transition leaf of the in-context ISM transition
// node at path: the ISM transition that led to the current state, a code of
// the openehr terminology written as `|code` and `|value`, optional and named
// as the reference's test_all_types golden names it.
func transitionIC(path string) *Node {
	return &Node{
		ID: "transition", Name: "transition", RMType: "DV_CODED_TEXT", Max: 1,
		AQLPath: path + "/transition", Inputs: cloneInputs(settingIC),
	}
}

// childAt returns the child of n at aqlPath, or nil.
func childAt(n *Node, aqlPath string) *Node {
	if i := slices.IndexFunc(n.Children, func(c *Node) bool { return c.AQLPath == aqlPath }); i >= 0 {
		return n.Children[i]
	}
	return nil
}

// codeInput returns a copy of the list of leaf's `code` input, and whether
// that list is open; none and false for a nil leaf or one without the input.
func codeInput(leaf *Node) ([]InputListItem, bool) {
	if leaf == nil {
		return nil, false
	}
	i := slices.IndexFunc(leaf.Inputs, func(in Input) bool { return in.Suffix == "code" })
	if i < 0 {
		return nil, false
	}
	in := cloneInputs(leaf.Inputs[i : i+1])[0]
	return in.List, in.ListOpen
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
			it.CurrentStates = slices.Clone(it.CurrentStates)
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

// emitAll appends to dst the WebTemplate nodes contributed by all of n's
// attribute children, with parentPath the accumulated aqlPath prefix, and
// returns the extended slice.
//
// The LOCATABLE `name` attribute is skipped (REQ-111): when an OPT pins a
// node's name it constrains that attribute, and walking it as data emitted
// a spurious `…/name` DV_TEXT leaf the reference never has — the reference
// carries the name *on* the node (its `id` and its aqlPath predicate),
// never as a child. Measured against the goldens, dropping it is exactly
// right: it removes 4 surplus nodes on GECCO and 21 on Corona, and neither
// golden has a `…/name` node anywhere.
func emitAll(dst []*Node, n *templatecompile.CompiledNode, parentPath string, cfg *config) []*Node {
	for _, attr := range impl.NodeAttributes(n) {
		if attr.Name() == "name" {
			continue
		}
		for _, child := range impl.ChildNodes(attr) {
			dst = emit(dst, child, attr, parentPath, cfg)
		}
	}
	return dst
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
	// ACTION adds its `time`, shaped as an EVENT's, after the ENTRY leaves,
	// as the reference's test_all_types golden places it.
	actionIC   = append(slices.Clone(entryIC), eventIC...)
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
		"ACTION": actionIC, "ADMIN_ENTRY": entryIC, "ACTIVITY": activityIC,
		"EVENT": eventIC, "POINT_EVENT": eventIC, "INTERVAL_EVENT": intervalEventIC,
	}
)

// emit appends to dst the WebTemplate node(s) contributed by compiled
// child c, reached from its parent through attribute attr whose parent sits
// at parentPath, and returns the extended slice. A dropped structural
// wrapper contributes its lifted kept descendants (its predicate stays in
// the accumulated path); an ELEMENT contributes a single collapsed value
// leaf, or a node with a child per value type ([elementNode]); a value type
// contributes a leaf; any other kept container contributes a node with
// recursed children.
func emit(dst []*Node, c *templatecompile.CompiledNode, attr *templatecompile.CompiledAttribute, parentPath string, cfg *config) []*Node {
	path := childPath(parentPath, attr.Name(), c)

	switch {
	case isDroppedContainer(c.RMTypeName()):
		return emitAll(dst, c, path, cfg)

	case isCollapsedEvent(c):
		// Lifted like a dropped wrapper, but through childrenOf: the
		// reference still emits the EVENT's in-context `time` leaf under
		// the (retained) events[…] path segment.
		return append(dst, childrenOf(c, path, cfg)...)

	case c.RMTypeName() == "ELEMENT" && !c.IsSlot():
		// An unfilled ELEMENT slot falls through: like any unfilled slot it
		// is pruned below as an empty container, as the reference skips it.
		return append(dst, elementNode(c, path, attr, cfg))

	case isValueLeaf(c.RMTypeName()):
		leaf := newNode(c, path, attr, cfg)
		leaf.Inputs = inputsFor(c)
		return append(dst, leaf)

	default: // kept container
		node := newNode(c, path, attr, cfg)
		node.Children = childrenOf(c, path, cfg)
		if len(node.Children) == 0 {
			return dst // EHRbase prunes empty containers (e.g. unfilled slots)
		}
		return append(dst, node)
	}
}

// elementNode returns the node an ELEMENT el at elPath gives, by the value
// alternatives its template admits, as the reference builds it:
//
//   - one alternative: a single leaf whose rmType is that value type, whose
//     nodeId and names are the ELEMENT's, and whose aqlPath is elPath
//     extended by /value;
//   - one DV_CODED_TEXT and one DV_TEXT, in either order: that leaf for the
//     DV_CODED_TEXT, its coded list open and a free-text `other` input added,
//     since the free text admits values beyond the listed codes;
//   - any other two or more: the ELEMENT as a node without inputs, holding one
//     child per alternative in template order ([valueChild]);
//   - none: the ELEMENT as a node holding one child per value type the
//     reference offers an ELEMENT that constrains no value ([anyValueTypes]).
func elementNode(el *templatecompile.CompiledNode, elPath string, attr *templatecompile.CompiledAttribute, cfg *config) *Node {
	var alts []*templatecompile.CompiledNode
	if va := el.Attribute("value"); va != nil {
		alts = impl.ChildNodes(va)
	}
	if len(alts) == 1 {
		return valueLeaf(el, alts[0], elPath, attr, cfg)
	}
	if coded := codedTextOfTextChoice(alts); coded != nil {
		leaf := valueLeaf(el, coded, elPath, attr, cfg)
		if len(leaf.Inputs) > 0 {
			leaf.Inputs[0].ListOpen = true // the "code" input
		}
		leaf.Inputs = append(leaf.Inputs, Input{Suffix: "other", Type: "TEXT"})
		return leaf
	}
	node := newNode(el, elPath, attr, cfg)
	if len(alts) == 0 {
		for _, rmType := range anyValueTypes {
			node.Children = append(node.Children, valueChild(node, rmType, nil))
		}
		return node
	}
	for _, v := range alts {
		node.Children = append(node.Children, valueChild(node, v.RMTypeName(), v))
	}
	return node
}

// valueLeaf returns the leaf an ELEMENT el at elPath collapses into when v is
// the value it gives.
func valueLeaf(el, v *templatecompile.CompiledNode, elPath string, attr *templatecompile.CompiledAttribute, cfg *config) *Node {
	leaf := newNode(el, elPath, attr, cfg)
	leaf.RMType = v.RMTypeName()
	leaf.AQLPath = elPath + "/value"
	leaf.Inputs = inputsFor(v)
	return leaf
}

// codedTextOfTextChoice returns the DV_CODED_TEXT of alts when alts are one
// DV_CODED_TEXT and one DV_TEXT, in either order, and nil otherwise.
func codedTextOfTextChoice(alts []*templatecompile.CompiledNode) *templatecompile.CompiledNode {
	if len(alts) != 2 {
		return nil
	}
	for i, a := range alts {
		if a.RMTypeName() == "DV_CODED_TEXT" && alts[1-i].RMTypeName() == "DV_TEXT" {
			return a
		}
	}
	return nil
}

// anyValueTypes are the value types, in the reference's order, of the children
// it gives an ELEMENT that constrains no value (OPTParser.addAnyNode).
var anyValueTypes = []string{
	"DV_TEXT", "DV_CODED_TEXT", "DV_MULTIMEDIA", "DV_PARSABLE", "DV_STATE", "DV_BOOLEAN",
	"DV_IDENTIFIER", "DV_URI", "DV_EHR_URI", "DV_DURATION", "DV_QUANTITY", "DV_COUNT",
	"DV_PROPORTION", "DV_DATE_TIME", "DV_TIME", "DV_ORDINAL", "DV_DATE",
}

// valueChild returns the child of ELEMENT node el for a value of rmType, at
// el's aqlPath extended by /value and named as el is, with its id from the
// type ([choiceID]). v is the value alternative the template constrains, whose
// occurrences and constraint the child carries, along with el's nodeId; with v
// nil, for an ELEMENT that constrains no value, the child has no nodeId,
// occurs 0..1, and carries the inputs of an unconstrained value of rmType, as
// the reference's are.
func valueChild(el *Node, rmType string, v *templatecompile.CompiledNode) *Node {
	child := &Node{
		ID:                    choiceID(rmType),
		Name:                  el.Name,
		LocalizedName:         el.LocalizedName,
		RMType:                rmType,
		Min:                   0,
		Max:                   1,
		LocalizedNames:        maps.Clone(el.LocalizedNames),
		LocalizedDescriptions: maps.Clone(el.LocalizedDescriptions),
		AQLPath:               el.AQLPath + "/value",
		Inputs:                valueInputs(rmType, v),
	}
	if v != nil {
		child.NodeID = el.NodeID
		setOccurrences(child, v)
	}
	return child
}

// newNode builds the common fields of a WebTemplate node from a compiled node.
func newNode(c *templatecompile.CompiledNode, aqlPath string, attr *templatecompile.CompiledAttribute, cfg *config) *Node {
	text := termText(c)
	node := &Node{
		RMType:  c.RMTypeName(),
		NodeID:  nodeIDOf(c),
		AQLPath: aqlPath,
		ID:      idOf(c, attr.Name(), text),
	}
	setOccurrences(node, c)
	setNames(node, c, text, cfg)
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

// setNames sets node's name and localized name, and its localized name and
// description in each language of cfg. The name is the one the template pins
// on c, where it pins one, else text: the text of the term for c's node id
// that c's nearest enclosing archetype root defines, as termText returns it.
// The reference names a node so, and [idOf] derives its id from the same
// name. The description is always the term's. A compiled template carries
// one set of terms, in its default language, so every language reads the
// same name and description, which are looked up once.
func setNames(node *Node, c *templatecompile.CompiledNode, text string, cfg *config) {
	name := cmp.Or(c.NodeName(), text)
	node.Name = name
	node.LocalizedName = name
	desc, _ := impl.RootTermItem(c, c.NodeID(), "description")
	for _, lang := range cfg.languages {
		if name != "" {
			if node.LocalizedNames == nil {
				node.LocalizedNames = map[string]string{}
			}
			node.LocalizedNames[lang] = name
		}
		if desc != "" {
			if node.LocalizedDescriptions == nil {
				node.LocalizedDescriptions = map[string]string{}
			}
			node.LocalizedDescriptions[lang] = desc
		}
	}
}

// childPath returns the aqlPath of c, reached through the attribute
// attrName from a parent at parentPath: the attribute's segment followed by
// c's node predicate. The predicate is c's archetype id if it is a
// slot/archetype root, else its at-code, else there is none (RM-attribute
// values carry no predicate). A node that pins a template-level name
// carries it inside the same brackets (REQ-111) — the reference's
// `[archetype_id,'Name']` form. The path is built in one concatenation.
//
// This builder composes its own paths rather than reading
// CompiledNode.AQLPath (it drops structural wrappers, so its paths
// legitimately differ), which is why the predicate rule has to be applied
// here too; the quoting itself is shared with the compiled-path builder
// via impl.NamePredicate so the two cannot drift.
func childPath(parentPath, attrName string, c *templatecompile.CompiledNode) string {
	if id := nodeIDOf(c); id != "" {
		return parentPath + "/" + attrName + "[" + impl.NamePredicate(id, c.NodeName()) + "]"
	}
	return parentPath + "/" + attrName
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
