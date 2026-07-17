package instance

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/internal/templateinstance/rmwrite"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

// Generate synthesises an RM instance for the compiled template's
// root type per REQ-107.
//
// The walk is template-driven: the compiled OPT drives traversal,
// rmwrite materialises RM values, and primitive leaves call
// PrimitiveConstraint.ExampleValue. The returned root is typed as
// any — use [AsComposition], [AsObservation], etc. for the concrete
// access path.
func Generate(ctx context.Context, c *templatecompile.Compiled, opts Options) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c == nil || c.Root() == nil {
		return nil, ErrNilCompiled
	}

	rootType := c.Root().RMTypeName()

	// COMPOSITION roots require Composer + Territory; fail fast
	// before constructing any RM tree.
	if rootType == "COMPOSITION" {
		if opts.Composer == nil {
			return nil, ErrComposerRequired
		}
		if opts.Territory == "" {
			return nil, ErrTerritoryRequired
		}
	}

	if opts.Now.IsZero() {
		opts.Now = time.Now().UTC()
	}
	if opts.Language == "" {
		opts.Language = c.Language()
	}
	if opts.Language == "" {
		opts.Language = "en"
	}

	g := &generator{
		compiled: c,
		opts:     opts,
	}
	if opts.ValueFill == RandomFill {
		g.valueSampler = newSampler(opts.ValueSource)
	}

	root, err := rmwrite.NewRM(rootType)
	if err != nil {
		return nil, fmt.Errorf("Generate: root %q: %w", rootType, err)
	}

	// The root carries the template_id; nested archetype roots only
	// get archetype_details with the archetype_id.
	g.setLocatableIdentity(c.Root(), root, true /* isTemplateRoot */)

	if err := g.walkNode(c.Root(), root); err != nil {
		return nil, err
	}

	// Apply root-type-specific defaults once the structure is in place.
	switch rootType {
	case "COMPOSITION":
		if err := g.applyCompositionDefaults(root.(*rm.Composition)); err != nil {
			return nil, err
		}
	}

	return root, nil
}

// generator carries per-call state. Constructed once per Generate.
type generator struct {
	compiled *templatecompile.Compiled
	opts     Options
	// valueSampler draws in-constraint leaf values when opts.ValueFill
	// is RandomFill; the zero value (used under ExampleFill) is never
	// consulted. REQ-107.
	valueSampler sampler
}

// nextUID returns the next LOCATABLE.uid pointer. Honours
// [Options.UIDSource] when set (tests pin a counter for golden
// fixtures); falls back to a random v4 UUID otherwise.
func (g *generator) nextUID() *rm.HierObjectID {
	if g.opts.UIDSource != nil {
		return g.opts.UIDSource()
	}
	return newHierObjectID()
}

// walkNode descends optNode under the bound rmValue, recursively
// materialising each attribute's children. Mirrors the lockstep
// shape of openehr/validation/walk_composition.go but in the
// opposite direction — the OPT drives, rmwrite attaches.
func (g *generator) walkNode(optNode *tcimpl.CompiledNode, rmValue any) error {
	if optNode == nil || rmValue == nil {
		return nil
	}
	// Slots are leaf fill-points: the synthesiser leaves slot bodies
	// empty and the caller composes them via REQ-101 builder Set
	// calls. The parsed REQ-104 grammar is used only to stamp a
	// conforming archetype id when a lower-bound top-up forces a
	// slot fill and a safe example can be derived (see stampSlotFill).
	if optNode.IsSlot() {
		return nil
	}
	// Primitive leaves: ExampleValue if policy allows, then return —
	// the primitive's RM-mandatory child attributes are implicitly
	// captured by the value (e.g. DV_QUANTITY embeds magnitude and
	// units). Validation v2 does not descend into primitive subtrees
	// either.
	if pc := optNode.PrimitiveConstraint(); pc != nil {
		if g.opts.Policy == Example {
			return g.applyPrimitiveExample(optNode, rmValue, pc)
		}
		// Under Minimal we still populate the leaf so the resulting
		// tree is valid (bounded constraints require a value); leaving
		// a zero RM value would surface as primitive_wrong_type or
		// out_of_range at validation. Cheap and aligned with the
		// "structurally complete" Minimal contract.
		return g.applyPrimitiveExample(optNode, rmValue, pc)
	}

	for _, attr := range optNode.Attributes() {
		if rminfo.IsNonStorableAttr(optNode.RMTypeName(), attr.Name()) {
			continue
		}
		if !g.shouldVisit(attr) {
			continue
		}
		switch attr.Cardinality() {
		case template.Single:
			if err := g.materialiseSingle(optNode, attr, rmValue); err != nil {
				return err
			}
		case template.Multiple:
			if err := g.materialiseMultiple(optNode, attr, rmValue); err != nil {
				return err
			}
		}
	}
	return nil
}

// shouldVisit decides whether an attribute is in scope under the
// current policy. Under Example: every attribute. Under Minimal:
// every attribute that is required (BMM-mandatory OR existence ≥ 1)
// OR has OPT-pinned children. The "has OPT children" arm captures
// the case where the OPT explicitly constrains a structurally
// optional attribute (e.g. COMPOSITION.content with archetype-root
// pins) — the act of pinning is itself a signal that the resulting
// tree should carry those children even under the smallest viable
// build.
func (g *generator) shouldVisit(attr *tcimpl.CompiledAttribute) bool {
	if g.opts.Policy == Example {
		return true
	}
	if isRequired(attr) {
		return true
	}
	return len(attr.Children()) > 0
}

// materialiseSingle synthesises and attaches one child under a
// C_SINGLE_ATTRIBUTE. For OPT alternatives (multiple children) the
// first wins — same convention validation uses for matchSingleAlternative.
//
// Implicit BMM-mandatory attributes (no OPT children) get a default
// RM value materialised from the attribute's BMM type so the
// resulting tree satisfies REQ-102 v2's "required attribute absent"
// check without the OPT pinning structure for every BMM mandatory.
func (g *generator) materialiseSingle(
	optNode *tcimpl.CompiledNode,
	attr *tcimpl.CompiledAttribute,
	parentRM any,
) error {
	children := attr.Children()
	if len(children) == 0 {
		// Implicit / OPT-silent attribute. When the attribute carries
		// a BMM-resolved RM type, materialise a default child of that
		// type so REQ-102 v2's required-attribute check passes; root-
		// type-specific defaults (composition.language, .territory,
		// .start_time) overwrite the placeholder afterwards.
		return g.materialiseImplicitSingle(optNode, attr, parentRM)
	}
	child := children[0]
	// AOM 1.4 primitive short name (DURATION, DATE, BOOLEAN, …)
	// under a BMM-primitive attribute (e.g. DV_DURATION.value): the
	// parent is itself the DV wrapper; populatePrimitiveDefault has
	// already stamped its primary value channel. A nested DV
	// materialised via makeChild would be attached to .value (a
	// String slot) and fail. When the leaf carries a parsed
	// primitive constraint (the REQ-107 + C_PRIMITIVE_OBJECT
	// wire-parser happy path), use its ExampleValue to override the
	// default sentinel on the parent. When the constraint is absent
	// (a C_PRIMITIVE_OBJECT wrapper whose inner item the OPT author
	// omitted, or an unknown xsi:type the parser admitted leniently),
	// the populatePrimitiveDefault sentinel holds.
	if tcimpl.IsAOMPrimitiveShortName(child.RMTypeName()) {
		if pc := child.PrimitiveConstraint(); pc != nil {
			return g.applyPrimitiveExample(child, parentRM, pc)
		}
		return nil
	}
	rmChild, err := g.makeChild(child)
	if err != nil {
		return err
	}
	// Stamp default primitive values BEFORE descending. When the
	// child is a DV scalar wrapper (DV_DURATION, DV_DATE, …),
	// populatePrimitiveDefault gives the wrapper a non-empty
	// canonical-JSON shape (`"value":"P0D"`, etc.) as a safe fallback
	// when the OPT does not pin a leaf primitive constraint. When a
	// constraint IS present, applyPrimitiveExample inside walkNode
	// overwrites the default. No-op for non-primitive wrappers.
	g.populatePrimitiveDefault(rmChild)
	if child.IsSlot() && !g.stampSlotFill(rmChild, child) {
		return fmt.Errorf("%w: %s", ErrSlotFillUnsupported, child.AQLPath())
	}
	if err := g.walkNode(child, rmChild); err != nil {
		return err
	}
	if err := rmwrite.EnsureSingle(parentRM, optNode.RMTypeName(), attr.Name(), rmChild); err != nil {
		return fmt.Errorf("attach %s.%s: %w", optNode.RMTypeName(), attr.Name(), err)
	}
	return nil
}

// materialiseImplicitSingle creates a default value for a
// BMM-mandatory single attribute the OPT did not pin. The default
// is a fresh zero-value RM instance of the attribute's BMM type;
// the post-walk defaults pass (applyCompositionDefaults etc.) fills
// in well-known fields (e.g. CODE_PHRASE.terminology_id). String-
// typed BMM attributes (e.g. DV_TEXT.value) go through a separate
// primitive-default path because they don't belong in typereg.
//
// Composition-level fields the post-walk defaults pass owns
// (language, territory, composer, category, context) are skipped
// here so user-supplied Options values are not clobbered with a
// placeholder.
func (g *generator) materialiseImplicitSingle(
	optNode *tcimpl.CompiledNode,
	attr *tcimpl.CompiledAttribute,
	parentRM any,
) error {
	if optNode.RMTypeName() == "COMPOSITION" {
		switch attr.Name() {
		case "language", "territory", "composer", "category", "context":
			return nil
		}
	}
	rmType := attr.RMTypeName()
	if rmType == "" {
		return nil
	}
	if rmType == "String" {
		// BMM String → a literal placeholder string. The rmwrite
		// dispatcher routes "value" / "code_string" / etc. to the
		// matching field; for everything else the silent best-effort
		// attach is acceptable.
		_ = rmwrite.EnsureSingle(parentRM, optNode.RMTypeName(), attr.Name(), "example")
		return nil
	}
	rmChild, err := newRMForOPTType(rmType)
	if err != nil {
		// Unknown RM type — silently skip; the OPT is mis-modelled or
		// the attribute is outside the current registry, both of
		// which the validator will flag.
		return nil //nolint:nilerr // intentional: defer to validator
	}
	// Stamp documented sentinel values on DV primitives so the
	// validator's "required attribute absent" check passes for
	// BMM-mandatory implicit attrs the OPT did not constrain.
	g.populatePrimitiveDefault(rmChild)
	g.populateBMMRequiredAttrs(rmChild, concreteFor(rmType), 0)
	// Best-effort attach; if the slot rejects the default (e.g. type
	// mismatch on a polymorphic attr), let downstream defaults
	// (applyCompositionDefaults) own the field.
	_ = rmwrite.EnsureSingle(parentRM, optNode.RMTypeName(), attr.Name(), rmChild)
	return nil
}

// populateBMMRequiredAttrs walks the BMM-required attribute set of
// the supplied RM value's type and materialises a default value for
// each. Used when the OPT did not constrain the attribute but the
// BMM marks it mandatory — keeps the resulting tree REQ-102 v2
// "required attribute absent" clean.
//
// `parentRMType` is the RM class name of `parent` (typereg-style,
// e.g. "ITEM_TREE"). Recursion bottoms out on primitive RM types
// (DataValue concretes, CODE_PHRASE) and on cycles via a
// visited-type ceiling depth.
func (g *generator) populateBMMRequiredAttrs(parent any, parentRMType string, depth int) {
	const maxDepth = 6
	if depth >= maxDepth || parent == nil || parentRMType == "" {
		return
	}
	for _, attrName := range rminfo.Default.RequiredAttributes(parentRMType) {
		// Skip identity / link metadata we already stamped or never
		// validate as "required".
		switch attrName {
		case "archetype_node_id", "name", "uid", "archetype_details",
			"links", "feeder_audit":
			continue
		}
		rmType, ok := rminfo.Default.AttributeRMType(parentRMType, attrName)
		if !ok || rmType == "" {
			continue
		}
		isContainer, _ := rminfo.Default.IsContainer(parentRMType, attrName)
		if rmType == "String" {
			_ = rmwrite.EnsureSingle(parent, parentRMType, attrName, "example")
			continue
		}
		concrete := concreteFor(rmType)
		rmChild, err := rmwrite.NewRM(concrete)
		if err != nil {
			continue
		}
		g.populatePrimitiveDefault(rmChild)
		// Recurse so nested BMM-required attrs (e.g. CODE_PHRASE
		// inside DV_CODED_TEXT) get filled.
		g.populateBMMRequiredAttrs(rmChild, concrete, depth+1)
		if isContainer {
			_ = rmwrite.AppendMultiple(parent, parentRMType, attrName, rmChild)
		} else {
			_ = rmwrite.EnsureSingle(parent, parentRMType, attrName, rmChild)
		}
	}
}

// populatePrimitiveDefault stamps a minimal-valid sentinel on a
// freshly-built DV value so its primary "value" channel is
// non-empty. Mirrors the REQ-103 ExampleValue sentinels for the
// unbounded cases. RM types that carry no primary value (CLUSTER,
// ELEMENT, party proxies) silently no-op.
func (g *generator) populatePrimitiveDefault(rmValue any) {
	switch v := rmValue.(type) {
	case *rm.DVText:
		v.Value = "example"
	case *rm.DVCodedText:
		v.Value = "example"
		v.DefiningCode = rm.CodePhrase{
			CodeString:    "at0000",
			TerminologyID: rm.TerminologyID{Value: "local"},
		}
	case *rm.CodePhrase:
		v.CodeString = "at0000"
		v.TerminologyID = rm.TerminologyID{Value: "local"}
	case *rm.DVDate:
		v.Value = "2020-01-01"
	case *rm.DVTime:
		v.Value = "12:00:00"
	case *rm.DVDateTime:
		v.Value = g.opts.Now.Format("2006-01-02T15:04:05Z07:00")
	case *rm.DVDuration:
		v.Value = "P0D"
	case *rm.DVBoolean:
		v.Value = true
	case *rm.DVCount:
		v.Magnitude = 0
	case *rm.DVQuantity:
		// Leave zero — the OPT primitive constraint may further pin.
	case *rm.DVProportion:
		v.Numerator = 1
		v.Denominator = 1
	case *rm.DVURI:
		v.Value = "http://example.com"
	case *rm.DVIdentifier:
		v.ID = "example"
	case *rm.DVParsable:
		v.Value = "example"
		v.Formalism = "text/plain"
	case *rm.DVInterval[rm.DVQuantity]:
		v.LowerUnbounded = true
		v.UpperUnbounded = true
	case *rm.DVInterval[rm.DVCount]:
		v.LowerUnbounded = true
		v.UpperUnbounded = true
	case *rm.DVInterval[rm.DVDateTime]:
		v.LowerUnbounded = true
		v.UpperUnbounded = true
	case *rm.DVInterval[rm.DVDate]:
		v.LowerUnbounded = true
		v.UpperUnbounded = true
	case *rm.DVInterval[rm.DVTime]:
		v.LowerUnbounded = true
		v.UpperUnbounded = true
	case *rm.DVInterval[rm.DVProportion]:
		v.LowerUnbounded = true
		v.UpperUnbounded = true
	case *rm.DVInterval[rm.DVOrdered]:
		v.LowerUnbounded = true
		v.UpperUnbounded = true
	}
}

// materialiseMultiple synthesises and appends children under a
// C_MULTIPLE_ATTRIBUTE. Per-child counts honour each OPT child's
// occurrences.lower (default 1) and the overall attribute's
// cardinality.upper (default unbounded). The synthesised count
// never exceeds the OPT-declared upper bound.
func (g *generator) materialiseMultiple(
	optNode *tcimpl.CompiledNode,
	attr *tcimpl.CompiledAttribute,
	parentRM any,
) error {
	children := attr.Children()
	if len(children) == 0 {
		// Implicit / OPT-silent multi-valued attribute. Synthesise one
		// default child of the BMM-resolved element type so the
		// validator's required-attribute / cardinality.lower check
		// passes; downstream consumers (REQ-101 Builder) overwrite.
		return g.materialiseImplicitMultiple(optNode, attr, parentRM)
	}
	upperBound := -1 // -1 == unbounded
	if cm := attr.ChildMultiplicity(); cm != nil && !cm.UpperUnbounded() {
		upperBound = cm.Upper()
	}
	total := 0
	for _, child := range children {
		// Slots are caller-filled; the synthesiser does not invent
		// archetype roots to fill them and instead leaves the count
		// to satisfy the slot's occurrences.lower (often 0 — most
		// slots are optional).
		if child.IsSlot() {
			continue
		}
		if g.opts.Policy == Minimal && optionalSiblingIDCollides(child, children) &&
			!firstCollidingOptionalSibling(child, children) {
			continue
		}
		childCount := 1
		if occ := child.Occurrences(); occ != nil && !occ.LowerUnbounded() {
			if occ.Lower() > 0 {
				childCount = occ.Lower()
			}
			// occ.Lower()==0 → still produce one fill so the
			// resulting tree carries every OPT-pinned archetype-root
			// child at least once. Drops to a per-child loop body of
			// `1` which is the minimal-yet-complete contract.
		}
		for i := 0; i < childCount; i++ {
			if upperBound >= 0 && total >= upperBound {
				return nil
			}
			rmChild, err := g.makeChild(child)
			if err != nil {
				return err
			}
			// Mirror materialiseSingle: stamp default primitive values
			// before walkNode descends so DV scalar wrappers in a
			// multi-attribute slot also carry a non-empty canonical-
			// JSON shape under the wire-parser primitive-constraint
			// gap. No-op for non-primitive RM children.
			g.populatePrimitiveDefault(rmChild)
			if err := g.walkNode(child, rmChild); err != nil {
				return err
			}
			if err := rmwrite.AppendMultiple(parentRM, optNode.RMTypeName(), attr.Name(), rmChild); err != nil {
				return fmt.Errorf("append %s.%s: %w", optNode.RMTypeName(), attr.Name(), err)
			}
			total++
		}
	}
	// Top-up to satisfy the attribute's overall lower bound when
	// nothing was appended (e.g. all OPT children had occurrence
	// lower 0 under Example with no top-level cardinality block).
	// When every OPT child is a slot we synthesise a slot-shaped
	// fill: an RM value of the slot's RMTypeName stamped (by
	// stampSlotFill) with an archetype id drawn from the parsed
	// REQ-104 include grammar, or from the RM-type-prefix fallback
	// only when no includes were parsed.
	if needed := remainingLowerNeeded(attr, total); needed > 0 {
		seed := firstNonSlot(children)
		if seed == nil && len(children) > 0 {
			seed = children[0]
		}
		if seed == nil {
			return nil
		}
		for needed > 0 && (upperBound < 0 || total < upperBound) {
			rmChild, err := g.makeChild(seed)
			if err != nil {
				return err
			}
			if seed.IsSlot() {
				if !g.stampSlotFill(rmChild, seed) {
					return fmt.Errorf("%w: %s", ErrSlotFillUnsupported, seed.AQLPath())
				}
			}
			if err := g.walkNode(seed, rmChild); err != nil {
				return err
			}
			if err := rmwrite.AppendMultiple(parentRM, optNode.RMTypeName(), attr.Name(), rmChild); err != nil {
				// Silent skip — the BMM-fallback child may not satisfy
				// the OPT-pinned attribute slot (e.g. an ELEMENT
				// fallback into ITEM_TREE.items where the attribute
				// expects an Item interface but the rmwrite check
				// finds a type mismatch). Stop top-up gracefully so
				// the caller (REQ-101 builder) can fill the slot
				// later.
				return nil //nolint:nilerr // intentional: defer to validator
			}
			total++
			needed--
		}
	}
	return nil
}

// stampSlotFill overrides the archetype_node_id and archetype_details
// on a freshly-constructed RM value when a valid slot-fill archetype
// id can be synthesized. It falls back to the RM-type-prefix example
// only for slots without parsed includes; parsed includes must be
// satisfied explicitly. Returns false when no safe id can be derived.
func (g *generator) stampSlotFill(rmValue any, slot *tcimpl.CompiledNode) bool {
	rules := slot.SlotRules()
	archetypeID := rules.ExampleArchetypeID()
	if archetypeID == "" && !rules.HasParsedIncludes() {
		archetypeID = "openEHR-EHR-" + slot.RMTypeName() + ".example.v1"
	}
	if archetypeID == "" || !rules.AllowsArchetypeID(archetypeID) {
		return false
	}
	ad := &rm.Archetyped{
		ArchetypeID: rm.ArchetypeID{Value: archetypeID},
		RMVersion:   "1.1.0",
	}
	applyLocatableIdentity(rmValue, archetypeID, slot.RMTypeName(), ad, g.nextUID)
	return true
}

// firstNonSlot returns the first OPT child that is not a slot, or
// nil when every child is a slot.
func firstNonSlot(children []*tcimpl.CompiledNode) *tcimpl.CompiledNode {
	for _, c := range children {
		if !c.IsSlot() {
			return c
		}
	}
	return nil
}

// materialiseImplicitMultiple creates one default child for a
// BMM-mandatory multi-valued attribute the OPT did not pin. Uses
// the attribute's BMM element type via [concreteFor]; silently no-op
// when the type is outside the typereg registry — the validator
// will flag it.
func (g *generator) materialiseImplicitMultiple(
	optNode *tcimpl.CompiledNode,
	attr *tcimpl.CompiledAttribute,
	parentRM any,
) error {
	rmType := attr.RMTypeName()
	if rmType == "" {
		return nil
	}
	rmChild, err := newRMForOPTType(rmType)
	if err != nil {
		return nil //nolint:nilerr // intentional: defer to validator
	}
	g.populatePrimitiveDefault(rmChild)
	g.populateBMMRequiredAttrs(rmChild, concreteFor(rmType), 0)
	_ = rmwrite.AppendMultiple(parentRM, optNode.RMTypeName(), attr.Name(), rmChild)
	return nil
}

// remainingLowerNeeded returns the count still required to satisfy
// the attribute's lower bound. Combines cardinality.lower (when
// present) with the existence ≥ 1 requirement — REQ-102 v2 flags
// an empty multi-valued attribute as "required" whenever existence
// pins lower ≥ 1, regardless of cardinality.lower (cardinality and
// existence are orthogonal in AOM 1.4).
func remainingLowerNeeded(attr *tcimpl.CompiledAttribute, current int) int {
	low := 0
	if cm := attr.ChildMultiplicity(); cm != nil && !cm.LowerUnbounded() {
		low = cm.Lower()
	}
	if isRequired(attr) && low == 0 {
		low = 1
	}
	if current >= low {
		return 0
	}
	return low - current
}

// makeChild constructs a fresh RM instance for the OPT child's
// rm_type_name and stamps LOCATABLE bookkeeping at the construction
// site (so the value is ready for caller attachment without an
// in-place mutation post-attach). Abstract RM types named by the
// OPT (EVENT, ITEM_STRUCTURE, DATA_VALUE, ITEM, CONTENT_ITEM,
// CARE_ENTRY, ENTRY, LOCATABLE) resolve to a documented concrete
// substitute — see [concreteFor].
func (g *generator) makeChild(child *tcimpl.CompiledNode) (any, error) {
	rmChild, err := newRMForOPTType(child.RMTypeName())
	if err != nil {
		return nil, fmt.Errorf("makeChild %s: %w", child.RMTypeName(), err)
	}
	g.setLocatableIdentity(child, rmChild, false /* isTemplateRoot */)
	return rmChild, nil
}

// concreteFor maps abstract RM class names the OPT may declare on
// child constraints to the documented concrete substitute the
// generator materialises. Mirrors the validation walker's
// bmmSubtypes "first concrete" pick — POINT_EVENT for EVENT,
// ITEM_TREE for ITEM_STRUCTURE, ELEMENT for ITEM, etc. Concrete RM
// types pass through unchanged.
//
// AOM 1.4 primitive short names (DURATION, DATE, TIME, DATE_TIME,
// BOOLEAN) appear under C_PRIMITIVE_OBJECT in some OPTs where the
// modeller constrains the primitive directly rather than its DV
// wrapper. The generator materialises the canonical DV wrapper for
// each; the validator's bmmSubtypes carries the lockstep admission
// rule so checkRMType does not reject the substitute.
func concreteFor(rmType string) string {
	switch rmType {
	case "EVENT":
		return "POINT_EVENT"
	case "ITEM_STRUCTURE":
		return "ITEM_TREE"
	case "ITEM":
		return "ELEMENT"
	case "CONTENT_ITEM":
		return "OBSERVATION"
	case "CARE_ENTRY", "ENTRY":
		return "OBSERVATION"
	case "DATA_VALUE":
		return "DV_TEXT"
	case "LOCATABLE":
		return "CLUSTER"
	case "PARTY_PROXY":
		return "PARTY_IDENTIFIED"
	// AOM 1.4 primitive short names → canonical DV wrapper.
	case "DURATION":
		return "DV_DURATION"
	case "DATE":
		return "DV_DATE"
	case "TIME":
		return "DV_TIME"
	case "DATE_TIME":
		return "DV_DATE_TIME"
	case "BOOLEAN":
		return "DV_BOOLEAN"
	case "INTEGER":
		return "DV_COUNT"
	}
	return rmType
}

// setLocatableIdentity stamps archetype_node_id, name, uid (when
// mandated by RM), and archetype_details on the freshly-built RM
// value. The isTemplateRoot flag controls whether template_id is
// stamped on archetype_details — only the very top-level root
// carries it.
func (g *generator) setLocatableIdentity(opt *tcimpl.CompiledNode, rmValue any, isTemplateRoot bool) {
	if opt == nil || rmValue == nil {
		return
	}
	// Pick the identity string per OPT shape: archetype-root carries
	// an archetype id; inner nodes carry an at-code.
	id := opt.NodeID()
	if arch := opt.ArchetypeID(); arch != "" {
		id = arch
	}
	if id == "" {
		// Some inner nodes (data values, anonymous attribute
		// containers) have neither — leave archetype_node_id
		// untouched and let downstream layers populate.
		return
	}

	// Resolve a human-readable runtime name from the OPT term
	// definitions when available; the RM type acts as the fallback.
	name := opt.RMTypeName()
	if id != "" {
		if t, ok := opt.Term(id, ""); ok {
			if text, found := t.Items["text"]; found && text != "" {
				name = text
			}
		}
	}

	// Archetype-root pins also get a populated archetype_details
	// block with the archetype id; the template id rides on the
	// top-level root only.
	var archetypeDetails *rm.Archetyped
	if arch := opt.ArchetypeID(); arch != "" || isTemplateRoot {
		ad := &rm.Archetyped{RMVersion: "1.1.0"}
		if arch != "" {
			ad.ArchetypeID = rm.ArchetypeID{Value: arch}
		} else if id != "" {
			// Template root with no explicit ArchetypeID on the OPT
			// node — leave the slot empty rather than fabricating one.
			ad.ArchetypeID = rm.ArchetypeID{Value: ""}
		}
		if isTemplateRoot && g.compiled.TemplateID() != "" {
			ad.TemplateID = &rm.TemplateID{Value: g.compiled.TemplateID()}
		}
		archetypeDetails = ad
	}

	applyLocatableIdentity(rmValue, id, name, archetypeDetails, g.nextUID)
}

// applyPrimitiveExample materialises a primitive leaf's ExampleValue
// against the RM value bound at this OPT node. Closed switch on the
// constraint type because the value shape differs per primitive
// (REQ-103 closed set).
func (g *generator) applyPrimitiveExample(
	_ *tcimpl.CompiledNode,
	rmValue any,
	pc constraints.PrimitiveConstraint,
) error {
	ex := pc.ExampleValue()
	if g.opts.ValueFill == RandomFill {
		// In-constraint sampled value (valid by construction); same Go
		// shape as ExampleValue so the switch below is unchanged. REQ-107.
		ex = sampleValue(pc, g.valueSampler)
	}
	switch v := rmValue.(type) {
	case *rm.DVQuantity:
		q, ok := ex.(constraints.QuantityValue)
		if !ok {
			return fmt.Errorf("DV_QUANTITY example value is %T, want QuantityValue", ex)
		}
		v.Magnitude = rm.Real(q.Magnitude)
		v.Units = q.Units
		return nil
	case *rm.DVText:
		s, ok := ex.(string)
		if !ok {
			return fmt.Errorf("DV_TEXT example value is %T, want string", ex)
		}
		v.Value = s
		return nil
	case *rm.DVCodedText:
		ref, ok := ex.(constraints.CodedTermRef)
		if !ok {
			return fmt.Errorf("DV_CODED_TEXT example value is %T, want CodedTermRef", ex)
		}
		v.Value = ref.CodeString
		v.DefiningCode = rm.CodePhrase{
			CodeString:    ref.CodeString,
			TerminologyID: rm.TerminologyID{Value: ref.Terminology},
		}
		return nil
	case *rm.CodePhrase:
		ref, ok := ex.(constraints.CodedTermRef)
		if !ok {
			return fmt.Errorf("CODE_PHRASE example value is %T, want CodedTermRef", ex)
		}
		v.CodeString = ref.CodeString
		v.TerminologyID = rm.TerminologyID{Value: ref.Terminology}
		return nil
	case *rm.DVBoolean:
		b, ok := ex.(bool)
		if !ok {
			return fmt.Errorf("DV_BOOLEAN example value is %T, want bool", ex)
		}
		v.Value = b
		return nil
	case *rm.DVCount:
		n, ok := rm.AsInt64(ex)
		if !ok {
			return fmt.Errorf("DV_COUNT example value is %T, want integer", ex)
		}
		v.Magnitude = n
		return nil
	case *rm.DVOrdinal:
		n, ok := rm.AsInt64(ex)
		if !ok {
			return fmt.Errorf("DV_ORDINAL example value is %T, want integer", ex)
		}
		v.Value = rm.Integer(n)
		return nil
	case *rm.DVDate:
		s, ok := ex.(string)
		if !ok {
			return fmt.Errorf("DV_DATE example value is %T, want string", ex)
		}
		v.Value = s
		return nil
	case *rm.DVTime:
		s, ok := ex.(string)
		if !ok {
			return fmt.Errorf("DV_TIME example value is %T, want string", ex)
		}
		v.Value = s
		return nil
	case *rm.DVDateTime:
		s, ok := ex.(string)
		if !ok {
			return fmt.Errorf("DV_DATE_TIME example value is %T, want string", ex)
		}
		v.Value = s
		return nil
	case *rm.DVDuration:
		s, ok := ex.(string)
		if !ok {
			return fmt.Errorf("DV_DURATION example value is %T, want string", ex)
		}
		v.Value = s
		return nil
	}
	// Unknown RM target for this constraint — silently no-op so the
	// generator stays sound on RM types REQ-103 does not yet have a
	// typed primitive for.
	return nil
}

// applyCompositionDefaults sets the COMPOSITION-specific fields
// per REQ-107: category 433|event|, language, territory, composer,
// context.start_time. Called once after the OPT-driven walk so the
// values land regardless of whether the OPT pinned them.
func (g *generator) applyCompositionDefaults(c *rm.Composition) error {
	if c.Category.DefiningCode.CodeString == "" {
		c.Category = rm.DVCodedText{
			DVText: rm.DVText{Value: "event"},
			DefiningCode: rm.CodePhrase{
				CodeString:    "433",
				TerminologyID: rm.TerminologyID{Value: "openehr"},
			},
		}
	}
	if c.Language.CodeString == "" {
		c.Language = rm.CodePhrase{
			CodeString:    g.opts.Language,
			TerminologyID: rm.TerminologyID{Value: "ISO_639-1"},
		}
	}
	if c.Territory.CodeString == "" {
		c.Territory = rm.CodePhrase{
			CodeString:    g.opts.Territory,
			TerminologyID: rm.TerminologyID{Value: "ISO_3166-1"},
		}
	}
	if c.Composer == nil {
		c.Composer = g.opts.Composer
	}
	if c.Context == nil {
		c.Context = &rm.EventContext{}
	}
	if c.Context.StartTime.Value == "" {
		c.Context.StartTime = rm.DVDateTime{Value: g.opts.Now.Format(time.RFC3339)}
	}
	// EventContext.Setting is BMM-mandatory; pin a documented default
	// when the template doesn't constrain it.
	if c.Context.Setting.DefiningCode.CodeString == "" {
		c.Context.Setting = rm.DVCodedText{
			DVText: rm.DVText{Value: "other care"},
			DefiningCode: rm.CodePhrase{
				CodeString:    "238",
				TerminologyID: rm.TerminologyID{Value: "openehr"},
			},
		}
	}
	return nil
}

// isRequired mirrors validation/walk_composition.go's isRequired —
// BMM-mandatory OR existence lower ≥ 1.
func isRequired(attr *tcimpl.CompiledAttribute) bool {
	if attr.Required() {
		return true
	}
	e := attr.Existence()
	if e == nil {
		return false
	}
	if e.LowerUnbounded() {
		return false
	}
	return e.Lower() >= 1
}

// newHierObjectID generates a HierObjectID with a random
// UUID-shaped value. Used for LOCATABLE.uid where openEHR mandates
// uniqueness (Composition, Entry root types). Returns a pointer so
// canjson's polymorphic dispatch on the UIDBasedID interface emits
// the `_type:"HIER_OBJECT_ID"` discriminator the decoder needs to
// round-trip the field. Falls back to a time-derived hex string when
// crypto/rand fails.
func newHierObjectID() *rm.HierObjectID {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Random source exhausted; fall back to nanosecond timestamp
		// so the generator keeps producing distinct ids.
		ts := uint64(time.Now().UnixNano())
		for i := range 8 {
			b[i] = byte(ts >> (i * 8))
		}
	}
	// RFC 4122 v4 layout.
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	s := hex.EncodeToString(b[:])
	uuid := s[0:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:32]
	return &rm.HierObjectID{Value: uuid}
}

// optionalSiblingIDCollides reports whether this optional child shares
// its node_id with another optional sibling under the same attribute.
func optionalSiblingIDCollides(child *tcimpl.CompiledNode, siblings []*tcimpl.CompiledNode) bool {
	if child.IsSlot() {
		return false
	}
	occ := child.Occurrences()
	if occ != nil && !occ.LowerUnbounded() && occ.Lower() > 0 {
		return false
	}
	id := child.NodeID()
	if id == "" {
		return false
	}
	count := 0
	for _, sib := range siblings {
		if sib.IsSlot() {
			continue
		}
		sibOcc := sib.Occurrences()
		if sibOcc != nil && !sibOcc.LowerUnbounded() && sibOcc.Lower() > 0 {
			continue
		}
		if sib.NodeID() == id {
			count++
		}
	}
	return count > 1
}

// firstCollidingOptionalSibling is true when child is the first
// optional sibling among those sharing its node_id.
func firstCollidingOptionalSibling(child *tcimpl.CompiledNode, siblings []*tcimpl.CompiledNode) bool {
	id := child.NodeID()
	for _, sib := range siblings {
		if sib.IsSlot() {
			continue
		}
		sibOcc := sib.Occurrences()
		if sibOcc != nil && !sibOcc.LowerUnbounded() && sibOcc.Lower() > 0 {
			continue
		}
		if sib.NodeID() == id {
			return sib == child
		}
	}
	return false
}
