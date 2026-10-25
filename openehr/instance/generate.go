package instance

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp/syntax"
	"slices"
	"strings"
	"sync"
	"time"
	"uuid"

	"github.com/cadasto/openehr-sdk-go/internal/bmmtype"
	"github.com/cadasto/openehr-sdk-go/internal/rmroots"
	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/internal/templateinstance/rmwrite"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/terminology"
)

// Generate synthesises an RM instance for the compiled template's
// root type.
//
// The walk is template-driven: the compiled OPT drives traversal,
// rmwrite materialises RM values, and primitive leaves are valued as
// [Options.ValueFill] says: the constraint's example value, or an
// in-constraint draw. The returned root is typed as
// any; use [AsComposition], [AsObservation], etc. for the concrete
// access path.
//
// A Policy other than Medium, Required and Large makes Generate return
// an error wrapping [ErrUnsupportedPolicy] and no root, before it builds
// anything.
//
// When the template asks for an object of a class that is always an
// archetype root but names no archetype for it, Generate returns an
// error wrapping [ErrArchetypeIDMissing] and no root. Such a template
// node is content the template does not describe, as a slot is: under
// Large and Medium an optional one gets no member, and an attribute that
// is not required and whose members would all need one is left out. A
// slot is not such an object: the slot-fill rule gives a required slot an
// archetype id from its includes, or the RM-type-prefix example id
// when it has none, or refuses it with [ErrSlotFillUnsupported].
//
// A list whose children's occurrences lower bounds add up past its
// cardinality upper bound, where the walk visits it, and an attribute the
// template requires that the generator has no writer for, such as a
// LOCATABLE's feeder_audit or links, make Generate return an error
// wrapping [ErrConstraintUnsatisfiable] and no root. A list that sits in
// an optional node the policy builds no member of, as Required builds
// none of an optional child, is not visited, and the root leaves it out. Where the template leaves
// such an attribute optional, it is left out, as is an attribute the
// node's RM class does not have, whatever the template asks of it.
func Generate(ctx context.Context, c *templatecompile.Compiled, opts Options) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c == nil || c.Root() == nil {
		return nil, ErrNilCompiled
	}
	switch opts.Policy {
	case Medium, Required, Large:
		// The policies the package defines.
	default:
		// Policy's String names every other value "unknown", so the
		// error prints its number.
		return nil, fmt.Errorf("%w: %d", ErrUnsupportedPolicy, int(opts.Policy))
	}

	rootType := c.Root().RMTypeName()

	// A root of an archetype-root class needs the archetype id the
	// template names for it; the generator does not invent one.
	if rmroots.IsArchetypeRoot(rootType) && c.Root().ArchetypeID() == "" {
		return nil, fmt.Errorf("%w: %s at %s (the template root)", ErrArchetypeIDMissing, rootType, c.Root().AQLPath())
	}

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
		opts.Now = time.Now()
	}
	// Every date-time the generator writes is in UTC, one layout, so the
	// interval ordering compares like with like.
	opts.Now = opts.Now.UTC()
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
	// A data-value or code-phrase root gets the primitive default a child
	// of its type gets when it is built, so a root and a nested value of
	// one type get the same placeholder. It does nothing for any other
	// root.
	g.populatePrimitiveDefault(root)

	// The root carries the template_id; nested archetype roots only
	// get archetype_details with the archetype_id.
	g.setLocatableIdentity(c.Root(), root, true /* isTemplateRoot */)

	if err := g.walkNode(c.Root(), root); err != nil {
		return nil, err
	}

	// Apply root-type-specific defaults once the structure is in place.
	if rootType == "COMPOSITION" {
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
	// nowText is opts.Now as dateTimeDefault writes it, formatted on first
	// use; "" until then.
	nowText string
	// undescribed holds, per template node, whether it needs content the
	// template does not describe (needsUndescribed), and locatableTypes,
	// per RM type, whether the value built for it is a LOCATABLE
	// (buildsLocatable). Both are filled on first use.
	undescribed    map[*tcimpl.CompiledNode]bool
	locatableTypes map[string]bool
}

// nextUID returns the next LOCATABLE.uid pointer. Honours
// [Options.UIDSource] when set (a counter in tests, or [UIDsFrom] on a
// seeded source); falls back to a random v4 UUID otherwise.
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
	// Slots are leaf fill-points: the slot body is not in this OPT, and
	// the caller composes it via REQ-101 builder Set calls. A slot the
	// walk must fill has been stamped by stampSlotFill with an archetype
	// id from the parsed REQ-104 grammar, or the RM-type-prefix example.
	// The fill then gets the RM-mandatory attributes of its class, with
	// the defaults an attribute the OPT leaves silent gets, so it passes
	// the RM floor: an entry's language, encoding, subject and its own
	// mandatory attributes. The fill holds only its identity here, which
	// the BMM fill skips, so nothing is filled twice. A CLUSTER or ELEMENT
	// fill takes finishNode's item defaults alone: one placeholder
	// element in CLUSTER.items, its only mandatory attribute, and a null
	// flavour on an ELEMENT, which has none.
	if optNode.IsSlot() {
		if _, isItem := rmValue.(rm.Item); !isItem {
			g.populateBMMRequiredAttrs(rmValue, concreteFor(optNode.RMTypeName()), 0)
		}
		g.finishNode(optNode, rmValue)
		return nil
	}
	// Primitive leaves: valued as the ValueFill says, then return —
	// the primitive's RM-mandatory child attributes are implicitly
	// captured by the value (e.g. DV_QUANTITY embeds magnitude and
	// units). Validation v2 does not descend into primitive subtrees
	// either. Every policy values the leaf, so the tree is valid: a
	// bounded constraint requires a value, and a zero RM value would
	// surface as primitive_wrong_type or out_of_range at validation.
	if pc := optNode.PrimitiveConstraint(); pc != nil {
		return g.applyPrimitiveExample(optNode, rmValue, "", pc)
	}

	if g.opts.ValueFill == RandomFill {
		if unit, ok := sharedQuantityUnit(optNode, g.valueSampler); ok {
			saved := g.valueSampler.quantityUnit
			g.valueSampler.quantityUnit = unit
			defer func() { g.valueSampler.quantityUnit = saved }()
		}
	}

	for _, attr := range tcimpl.NodeAttributes(optNode) {
		if !g.visits(optNode, attr) {
			continue
		}
		// visits lets through an attribute the generator has no writer for
		// only where the template or the RM requires it, and nothing the
		// walk can build satisfies that requirement.
		if noWriter(optNode, attr) {
			return fmt.Errorf("%w: %s.%s at %s (required, but the generator has no writer for it)",
				ErrConstraintUnsatisfiable, optNode.RMTypeName(), attr.Name(), optNode.AQLPath())
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
	g.fillUncarriedRequired(optNode, rmValue)
	if err := g.writeRMNeeded(optNode, rmValue); err != nil {
		return err
	}
	orderIntervalBounds(optNode, rmValue, g.opts.Language)
	settleIntervalEndpoints(optNode, rmValue)
	if err := settleAccuracy(optNode, rmValue); err != nil {
		return err
	}
	g.finishNode(optNode, rmValue)
	// After finishNode, which settles a DV_PROPORTION's kind, so the value
	// and its normal_range are final.
	return settleNormalRange(optNode, rmValue)
}

// fillUncarriedRequired fills each attribute the BMM marks mandatory on
// rmValue's class that optNode does not carry, as under
// templatecompile.WithoutImplicitAttributes, the way an attribute the OPT
// leaves silent is filled. An attribute optNode carries is the walk's
// business, a prohibited one included. A COMPOSITION's language,
// territory, composer and category are left to applyCompositionDefaults,
// as materialiseImplicitSingle leaves them.
func (g *generator) fillUncarriedRequired(optNode *tcimpl.CompiledNode, rmValue any) {
	class := bmmtype.Class(rmTypeOf(rmValue))
	for _, attrName := range requiredAttributes(class) {
		if optNode.Attribute(attrName) != nil || compositionDefaultOwns(class, attrName) {
			continue
		}
		g.fillBMMAttr(rmValue, class, attrName, 0)
	}
}

// writeRMNeeded writes, as a visit writes it, an attribute of rmValue that
// an RM rule needs and the walk left unwritten: the policy did not visit
// it, as Required leaves an optional attribute alone, or the OPT prohibits
// it (REQ-107 Precedence 2). For an ELEMENT, a PARTY_IDENTIFIED, a
// PARTY_RELATED and a DV_MULTIMEDIA it writes only where optNode names an
// allowed OPT child for the attribute, and builds the attribute from its
// first allowed child (materialiseSingle), as the walk would build it, so
// it keeps the OPT's constraints rather than taking a default of its BMM
// type. The attributes are an ELEMENT's null_flavour when it has no value,
// or its value where the OPT prohibits the null_flavour and allows the
// value, a PARTY_IDENTIFIED's or PARTY_RELATED's name when it has no name,
// identifiers or external_ref, and a DV_MULTIMEDIA's uri when it has no
// uri and the OPT prohibits its data, which it then never gets (RM
// Not_empty). finishNode then gives each the RM default it gives a visited
// one, and builds one with no allowed OPT child from its BMM type; a uri
// with none takes the default settleMultimedia writes. A HISTORY with
// neither events nor a summary gets one of them (writeEventOrSummary):
// from an allowed OPT child, or from its BMM type where the OPT names no
// child under it.
func (g *generator) writeRMNeeded(optNode *tcimpl.CompiledNode, rmValue any) error {
	switch v := rmValue.(type) {
	case *rm.History[rm.ItemStructure]:
		if len(v.Events) > 0 || (v.Summary != nil && !rm.IsTypedNil(v.Summary)) {
			return nil
		}
		return g.writeEventOrSummary(optNode, v)
	case *rm.DVMultimedia:
		if (v.URI != nil && !rm.IsTypedNil(v.URI)) || len(v.Data) > 0 || !prohibited(optNode, "data") {
			return nil
		}
		return g.visitNeeded(optNode, "uri", v)
	case *rm.Element:
		if (v.Value != nil && !rm.IsTypedNil(v.Value)) || v.NullFlavour != nil {
			return nil
		}
		if prohibited(optNode, "null_flavour") && !nullFlavourNeeded(optNode) {
			return g.visitNeeded(optNode, "value", v)
		}
		return g.visitNeeded(optNode, "null_flavour", v)
	case *rm.PartyIdentified:
		if partyNamed(v.Name, len(v.Identifiers), v.ExternalRef) {
			return nil
		}
		return g.visitNeeded(optNode, "name", v)
	case *rm.PartyRelated:
		if partyNamed(v.Name, len(v.Identifiers), v.ExternalRef) {
			return nil
		}
		return g.visitNeeded(optNode, "name", v)
	}
	return nil
}

// writeEventOrSummary gives h, a HISTORY the walk left with neither events
// nor a summary, what RM Events_valid needs, (events /= Void and then not
// events.is_empty) or summary /= Void (REQ-107 Precedence 2). It writes the
// first of these that optNode allows: a member of events from its first
// allowed child that needs no content the OPT does not describe
// (needsUndescribed, which a slot always needs); a summary from such a
// child of summary; a member of events built from its BMM type, where
// optNode names no child under events; a summary built from its BMM type,
// where optNode names no child under summary; a member of events from its
// first allowed child that is not a slot; a summary from such a child of
// summary. So a child that needs undescribed content, which can make
// Generate fail on a slot it cannot fill, is passed over while a later
// source is allowed. An attribute optNode prohibits allows none, and nor
// does an events whose cardinality leaves no room for a member. Where
// optNode allows none of them, h is left as it is, and the root breaks the
// rule (REQ-107 Exceptions and known gaps).
func (g *generator) writeEventOrSummary(optNode *tcimpl.CompiledNode, h *rm.History[rm.ItemStructure]) error {
	if name, child := historySource(optNode, g.firstNeedingNone); child != nil {
		return g.attachHistoryMember(optNode, h, name, child)
	}
	for _, name := range historyMembers {
		attr := optNode.Attribute(name)
		if attr != nil && (len(tcimpl.ChildNodes(attr)) > 0 || !historyAllows(optNode, attr)) {
			continue
		}
		g.fillBMMAttr(h, "HISTORY", name, 0)
		return nil
	}
	if name, child := historySource(optNode, firstNonSlot); child != nil {
		return g.attachHistoryMember(optNode, h, name, child)
	}
	return nil
}

// historySource returns the first of historyMembers that optNode, a
// HISTORY, allows a member of (historyAllows) and the child pick returns
// among its allowed children, or nil where there is none.
func historySource(optNode *tcimpl.CompiledNode, pick func([]*tcimpl.CompiledNode) *tcimpl.CompiledNode) (string, *tcimpl.CompiledNode) {
	for _, name := range historyMembers {
		attr := optNode.Attribute(name)
		if attr == nil || !historyAllows(optNode, attr) {
			continue
		}
		if child := pick(tcimpl.AllowedChildNodes(attr)); child != nil {
			return name, child
		}
	}
	return "", nil
}

// firstNeedingNone returns the first of children that needs no content the
// OPT does not describe (needsUndescribed), or nil where every one of them
// needs some. A slot always needs it.
func (g *generator) firstNeedingNone(children []*tcimpl.CompiledNode) *tcimpl.CompiledNode {
	for _, c := range children {
		if !g.needsUndescribed(c) {
			return c
		}
	}
	return nil
}

// attachHistoryMember builds child, an allowed child of name, the events or
// the summary of optNode, and attaches it to h: as a member of events, or
// as its summary.
func (g *generator) attachHistoryMember(optNode *tcimpl.CompiledNode, h *rm.History[rm.ItemStructure], name string, child *tcimpl.CompiledNode) error {
	built, err := g.buildChild(child)
	if err != nil {
		return err
	}
	if name == "events" {
		err = rmwrite.AppendMultiple(h, name, built)
	} else {
		err = rmwrite.EnsureSingle(h, name, built)
	}
	if err != nil {
		return fmt.Errorf("attach %s.%s: %w", optNode.RMTypeName(), name, err)
	}
	return nil
}

// historyMembers are the attributes of a HISTORY that keep RM Events_valid,
// in the order writeEventOrSummary tries them.
var historyMembers = []string{"events", "summary"}

// historyAllows reports whether optNode, a HISTORY, allows attr, its events
// or its summary, a member: the OPT does not prohibit it, and, for events,
// its cardinality leaves room for one.
func historyAllows(optNode *tcimpl.CompiledNode, attr *tcimpl.CompiledAttribute) bool {
	if prohibited(optNode, attr.Name()) {
		return false
	}
	return attr.Name() != "events" || memberCap(attr) != 0
}

// visitNeeded builds attrName of optNode, an attribute an RM rule needs,
// on rmValue from its first allowed OPT child, as a visit builds it. An
// attribute optNode does not name, or names with no allowed child, is
// left alone.
func (g *generator) visitNeeded(optNode *tcimpl.CompiledNode, attrName string, rmValue any) error {
	attr := optNode.Attribute(attrName)
	if attr == nil || len(tcimpl.AllowedChildNodes(attr)) == 0 {
		return nil
	}
	return g.materialiseSingle(optNode, attr, rmValue)
}

// compositionDefaultOwns reports whether attrName of class is one
// applyCompositionDefaults fills, from Options or its RM default, so no
// generic default is built for it first.
func compositionDefaultOwns(class, attrName string) bool {
	if class != "COMPOSITION" {
		return false
	}
	switch attrName {
	case "language", "territory", "composer", "category", "context":
		return true
	}
	return false
}

// visits decides whether the walk descends into attr of optNode. Under
// every policy it never visits an attribute the OPT prohibits (an
// existence of 0..0, or OPT children it all prohibits) that no RM rule
// needs (attrProhibited), whatever children the OPT names under it, nor
// one no serialization carries, the BMM functions and
// RESOURCE_DESCRIPTION.parent_resource that rminfo.IsNonStorableAttr
// names: the generator has nothing to write there. Nor does it visit a
// locatable's uid, which setLocatableIdentity stamps, nor the normal_range or
// other_reference_ranges of a bound of a REFERENCE_RANGE's range, which
// the RM keeps absent, nor an attribute the node's RM class does not have,
// nor a DV_MULTIMEDIA's data, which settleMultimedia writes (walkSkips). A
// prohibited ELEMENT null_flavour the RM rule needs, because the OPT
// prohibits value too, is visited as if the OPT allowed it. An attribute
// the generator has no writer for (noWriter) is visited only where the
// template requires it (writerNeeded), for the walk to refuse it. Any other
// attribute is visited when the policy says so (shouldVisit).
func (g *generator) visits(optNode *tcimpl.CompiledNode, attr *tcimpl.CompiledAttribute) bool {
	if walkSkips(optNode, attr) {
		return false
	}
	if noWriter(optNode, attr) {
		return writerNeeded(optNode, attr)
	}
	return g.shouldVisit(optNode, attr)
}

// walkSkips reports whether the walk leaves attr of optNode alone under
// every policy: a locatable's uid, an attribute the OPT prohibits that no RM
// rule needs, one the RM computes rather than stores, the normal_range and
// other_reference_ranges of a bound of a REFERENCE_RANGE's range, which the
// OPT may name and the RM keeps absent, one the compile marks as absent
// from the node's RM class (NotInRM), such as ISM_TRANSITION.name, which no
// instance can carry and the template validator leaves out, whatever
// existence the OPT gives it, and a DV_MULTIMEDIA's data, whose octets bind
// to no OPT child and which settleMultimedia writes instead.
func walkSkips(optNode *tcimpl.CompiledNode, attr *tcimpl.CompiledAttribute) bool {
	if locatableUID(optNode, attr) || referenceRangeBoundRanges(optNode, attr) {
		return true
	}
	if attr.NotInRM() || multimediaData(optNode, attr) {
		return true
	}
	if attrProhibited(attr) && !rmNeedsProhibited(optNode, attr) {
		return true
	}
	// rminfo knows each class by its bare BMM name; the OPT may declare
	// a generic instantiation (DV_INTERVAL<DV_QUANTITY>).
	return rminfo.IsNonStorableAttr(bmmtype.Class(optNode.RMTypeName()), attr.Name())
}

// nodeClass is the bare BMM class of the value the walk builds for
// optNode: the class it names, or its concrete substitute (concreteFor),
// without generic parameters.
func nodeClass(optNode *tcimpl.CompiledNode) string {
	return bmmtype.Class(concreteFor(strings.TrimSpace(optNode.RMTypeName())))
}

// multimediaData reports whether attr is the data of optNode, a
// DV_MULTIMEDIA: an array of octets, to which no OPT child binds.
func multimediaData(optNode *tcimpl.CompiledNode, attr *tcimpl.CompiledAttribute) bool {
	return attr.Name() == "data" && nodeClass(optNode) == "DV_MULTIMEDIA"
}

// noWriter reports whether the generator has no writer for attr of
// optNode: a LOCATABLE's links and feeder_audit, an EHR_ACCESS's settings,
// whose RM type ACCESS_CONTROL_SETTINGS has no concrete class, and a list
// whose items the pinned RM types as primitive values (primitiveList), such
// as a DV_MULTIMEDIA's integrity_check, other than its data, which
// walkSkips leaves to settleMultimedia. The RM requires none of them.
func noWriter(optNode *tcimpl.CompiledNode, attr *tcimpl.CompiledAttribute) bool {
	class := nodeClass(optNode)
	switch name := attr.Name(); name {
	case "links", "feeder_audit":
		if declaredOnLocatable(class, name) {
			return true
		}
	case "settings":
		if class == "EHR_ACCESS" {
			return true
		}
	}
	return primitiveList(class, attr) && !multimediaData(optNode, attr)
}

// writerNeeded reports whether the template requires attr of optNode, an
// attribute the generator has no writer for, so that a root without it
// fails the template validator: it is required (isRequired, which a
// cardinality lower bound alone does not make so, since that counts the
// members of a list that exists), and, for a list the RM declares, more
// than its existence asks it for a member (existenceOnly), since a list
// with no member meets its existence.
func writerNeeded(optNode *tcimpl.CompiledNode, attr *tcimpl.CompiledAttribute) bool {
	if !isRequired(attr) {
		return false
	}
	isList, _ := rminfo.Default.IsContainer(nodeClass(optNode), attr.Name())
	return !isList || !existenceOnly(optNode, attr)
}

// primitiveList reports whether the pinned RM declares attr of class a list
// whose items are primitive values, such as the octets of a DV_MULTIMEDIA's
// data and integrity_check, as the RM's absence table names them
// ([rminfo.AbsencePrimitive]). A Hash is no such list.
func primitiveList(class string, attr *tcimpl.CompiledAttribute) bool {
	isList, _ := rminfo.Default.IsContainer(class, attr.Name())
	if !isList {
		return false
	}
	itemType, ok := rminfo.Default.AttributeRMType(class, attr.Name())
	if !ok || itemType == "" || itemType == "Hash" {
		return false
	}
	ar, ok := rminfo.Default.(rminfo.AbsenceReporter)
	return ok && ar.AbsenceReason(itemType) == rminfo.AbsencePrimitive
}

// declaredOnLocatable reports whether LOCATABLE declares attrName for
// class, as it declares uid, links and feeder_audit.
func declaredOnLocatable(class, attrName string) bool {
	h, ok := rminfo.Default.(rminfo.Hierarchy)
	if !ok {
		return false
	}
	site, ok := h.DeclaredOn(class, attrName)
	return ok && site == "LOCATABLE"
}

// referenceRangeBoundRanges reports whether attr is the normal_range or the
// other_reference_ranges of optNode, and optNode is a bound of a
// REFERENCE_RANGE's range: the node held by the lower or the upper of a
// DV_INTERVAL that is the range of a REFERENCE_RANGE. A bound that carries
// either is not simple, which breaks REFERENCE_RANGE Range_is_simple, so
// the RM wins over the OPT, whether the OPT names the attribute optional or
// required (Precedence 2). Its normal_status alone is not a reference range
// and is left to the walk.
func referenceRangeBoundRanges(optNode *tcimpl.CompiledNode, attr *tcimpl.CompiledAttribute) bool {
	if name := attr.Name(); name != "normal_range" && name != "other_reference_ranges" {
		return false
	}
	if side := holdingAttribute(optNode); side != "lower" && side != "upper" {
		return false
	}
	interval := optNode.Parent()
	if bmmtype.Class(interval.RMTypeName()) != "DV_INTERVAL" || holdingAttribute(interval) != "range" {
		return false
	}
	return bmmtype.Class(interval.Parent().RMTypeName()) == "REFERENCE_RANGE"
}

// locatableUID reports whether attr is the uid of a locatable, the one
// LOCATABLE declares, which the identity rule rather than the walk
// decides.
func locatableUID(optNode *tcimpl.CompiledNode, attr *tcimpl.CompiledAttribute) bool {
	return attr.Name() == "uid" && declaredOnLocatable(nodeClass(optNode), "uid")
}

// requiresUID reports whether the OPT requires the uid of opt, with an
// existence lower bound of at least 1.
func requiresUID(opt *tcimpl.CompiledNode) bool {
	if opt == nil {
		return false
	}
	attr := opt.Attribute("uid")
	if attr == nil {
		return false
	}
	return tcimpl.LowerAtLeastOne(attr.Existence())
}

// uidFor returns the uid source for a value built without an OPT node
// that names its uid: Options.UIDSource, or its fallback, for a class
// stampsUID names, and nil, no uid, for any other.
func (g *generator) uidFor(v any) func() *rm.HierObjectID {
	if stampsUID(v) {
		return g.nextUID
	}
	return nil
}

// rmNeedsProhibited reports whether an RM rule needs attr of optNode
// where the OPT prohibits it, so the walk visits it as if the OPT allowed
// it: an ELEMENT's null_flavour where the OPT prohibits value too
// (nullFlavourNeeded). An attribute the BMM marks mandatory is never
// prohibited (attrProhibited), so it needs no case here.
func rmNeedsProhibited(optNode *tcimpl.CompiledNode, attr *tcimpl.CompiledAttribute) bool {
	return attr.Name() == "null_flavour" && nullFlavourNeeded(optNode)
}

// attrProhibited reports whether the OPT prohibits attr and no RM rule
// needs it. The OPT prohibits an attribute in two ways: an existence upper
// bound that is bounded and 0, or OPT children that it all prohibits (an
// occurrences upper bound of 0). An attribute the BMM marks mandatory is
// never prohibited: the RM rule wins over the prohibition, so it is
// visited and filled as an attribute the OPT leaves silent is.
func attrProhibited(attr *tcimpl.CompiledAttribute) bool {
	return !attr.Required() && tcimpl.Prohibited(attr)
}

// shouldVisit decides whether an attribute is in scope under the
// current policy. Under Large: every attribute, but for the rule on
// undescribed content below. Under Required: every attribute that is
// required (isRequired), because the BMM marks it mandatory, its existence
// lower bound is bounded and at least 1, or it is a list with an allowed
// child, not a slot, whose occurrences lower bound is bounded and at least
// 1 (holdsRequiredChild, which leaves out a list the OPT prohibits and one
// of primitive values), and no other: a cardinality lower bound alone does
// not make Required visit a list, since it counts the members of a list
// that exists and does not ask for the list. Under Medium: every attribute that is required in
// the wider sense of visitRequired, which a cardinality lower bound of 1
// or more also meets, OR that has OPT-pinned children the OPT does not
// prohibit. The "has OPT children" arm captures the case where the OPT
// explicitly constrains a structurally optional attribute (e.g. COMPOSITION.content
// with archetype-root pins): under Medium the act of pinning is itself
// a signal that the resulting tree should carry those children. Required
// does not take that arm, so it builds only what the template makes
// mandatory, except on a COMPOSITION's context, which it visits as
// Medium does: applyCompositionDefaults writes a context under every
// policy, so the walk builds it under the OPT's constraints (a pinned
// setting, a required other_context) rather than leaving it bare. Inside
// the context Required's own rule applies. Under Large and Medium, an
// attribute that is not required (isRequired), other than that context, is
// not visited either when every member the walk would build for it is
// undescribed or needs undescribed content (membersUndescribed): a slot
// fill, a LOCATABLE built from a BMM type where the OPT names no node, an
// unnamed archetype root, or a node with a required attribute that can only
// hold such members. A list that only its cardinality lower bound requires
// is not required here, as Required reads it: that bound counts the members
// of a list that exists and does not ask for the list. The context is
// visited whatever its members need, since RM defaults would write it
// without them. No other Policy value gets here: Generate refuses one
// before the walk starts.
func (g *generator) shouldVisit(optNode *tcimpl.CompiledNode, attr *tcimpl.CompiledAttribute) bool {
	isContext := compositionContext(optNode, attr)
	if g.opts.Policy == Required && !isContext {
		return isRequired(attr)
	}
	if isRequired(attr) {
		return true
	}
	if !isContext && g.membersUndescribed(optNode, attr) {
		return false
	}
	if visitRequired(attr) {
		return true
	}
	return g.opts.Policy == Large || len(tcimpl.AllowedChildNodes(attr)) > 0
}

// visitRequired reports whether attr is required as the visit rule reads
// it, so Large and Medium visit it: it is required (isRequired), or its
// cardinality lower bound is bounded and at least 1. Cardinality lower and
// existence are separate, so a 1..* list is required when existence does
// not say so. Required leaves out a list only that cardinality requires,
// and so do Large and Medium where every member it would get is
// undescribed or needs undescribed content (shouldVisit). Whether a node
// needs undescribed content (needsUndescribed) reads this wider sense under
// every policy.
func visitRequired(attr *tcimpl.CompiledAttribute) bool {
	return isRequired(attr) || tcimpl.LowerAtLeastOne(attr.ChildMultiplicity())
}

// compositionContext reports whether attr is the context of a
// COMPOSITION, the one attribute Required visits as Medium does and the
// one the rule on undescribed content never keeps the walk out of.
func compositionContext(optNode *tcimpl.CompiledNode, attr *tcimpl.CompiledAttribute) bool {
	return attr.Name() == "context" && bmmtype.Class(optNode.RMTypeName()) == "COMPOSITION"
}

// bindingAlternative returns the allowed alternative of a single-valued
// attribute whose constraints the walk builds under. built is the RM
// type the walk builds: the type the first alternative names, or its
// concrete substitute ([builtRMType]). It is the first child of the
// binding tier for that type: the first alternative that names the type
// exactly, else the first that is the AOM 1.4 primitive short name the
// type wraps, else the first alternative. The template validator tries
// that child first and keeps it when the value validates there, which a
// value built under its constraints does (REQ-102 alternatives rule,
// REQ-107 What a visit writes). Under
// [DATA_VALUE, DV_TEXT] the walk therefore builds a DV_TEXT under the
// DV_TEXT alternative's constraints, not the open DATA_VALUE's.
func bindingAlternative(children []*tcimpl.CompiledNode, built string) *tcimpl.CompiledNode {
	for _, c := range children {
		if strings.TrimSpace(c.RMTypeName()) == built {
			return c
		}
	}
	for _, c := range children {
		name := strings.TrimSpace(c.RMTypeName())
		if tcimpl.IsAOMPrimitiveShortName(name) && concreteFor(name) == built {
			return c
		}
	}
	return children[0]
}

// builtRMType is the RM type the walk builds for an alternative: the
// type it names, or its concrete substitute (concreteFor).
func builtRMType(alternative *tcimpl.CompiledNode) string {
	return concreteFor(strings.TrimSpace(alternative.RMTypeName()))
}

// attrTypedByRMClass reports whether the pinned BMM types attr of
// optNode by an RM class (ELEMENT.value, a DATA_VALUE), rather than by
// a primitive such as String or Boolean (DV_DURATION.value,
// DV_BOOLEAN.value) or not at all (an attribute the pinned RM does not
// have).
func attrTypedByRMClass(optNode *tcimpl.CompiledNode, attr *tcimpl.CompiledAttribute) bool {
	h, ok := rminfo.Default.(rminfo.Hierarchy)
	if !ok {
		return false
	}
	_, known := h.IsAbstract(bmmtype.Class(attrType(optNode.RMTypeName(), attr.RMTypeName())))
	return known
}

// materialiseSingle synthesises and attaches one child under a
// C_SINGLE_ATTRIBUTE. The built value takes the RM type the first
// allowed alternative names, or that alternative's concrete
// substitute, and is built under the constraints of the first child of
// the first binding tier that admits that type ([bindingAlternative]),
// which the validator tries first. The value validates there, so
// validation checks what the generator built against the child whose
// constraints built it (REQ-107, What a visit writes).
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
	// An alternative the OPT prohibits is skipped: the next allowed one
	// wins. With none allowed the attribute is prohibited and not visited
	// (attrProhibited), unless the BMM marks it mandatory: then it gets
	// here and is built as one the OPT leaves silent.
	children := tcimpl.AllowedChildNodes(attr)
	if len(children) == 0 {
		// An attribute the RM declares a list, which the template writes
		// as a single attribute (COMPOSITION.content as a
		// C_SINGLE_ATTRIBUTE), is read as that list, as the template
		// validator reads it. Where the BMM does not mark it mandatory,
		// only its existence asks for a member, so it gets none: a list
		// with no member meets its existence. A list the RM makes non-empty
		// once present does not: at an existence of 1 or more it gets the
		// member its BMM type gives, as a list the OPT writes so does
		// (materialiseImplicitMultiple), which refuses an archetype root.
		if !attr.Required() && rmDeclaresList(parentRM, attr.Name()) {
			if !rmNonEmptyList(optNode, attr) || !tcimpl.LowerAtLeastOne(attr.Existence()) {
				return nil
			}
			return g.materialiseImplicitMultiple(optNode, attr, parentRM)
		}
		// Implicit / OPT-silent attribute. When the attribute carries
		// a BMM-resolved RM type, materialise a default child of that
		// type so REQ-102 v2's required-attribute check passes; root-
		// type-specific defaults (composition.language, .territory,
		// .start_time) overwrite the placeholder afterwards.
		return g.materialiseImplicitSingle(optNode, attr, parentRM)
	}
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
	// the populatePrimitiveDefault sentinel holds. Under an attribute
	// an RM class types (ELEMENT.value) the short name is built below
	// as its DV wrapper, which concreteFor names.
	if first := children[0]; tcimpl.IsAOMPrimitiveShortName(first.RMTypeName()) && !attrTypedByRMClass(optNode, attr) {
		if pc := first.PrimitiveConstraint(); pc != nil {
			return g.applyPrimitiveExample(first, parentRM, attr.Name(), pc)
		}
		return nil
	}
	rmChild, err := g.buildChild(bindingAlternative(children, builtRMType(children[0])))
	if err != nil {
		return err
	}
	if replacesTerminology(parentRM, attr.Name(), rmChild) {
		return nil
	}
	if err := rmwrite.EnsureSingle(parentRM, attr.Name(), rmChild); err != nil {
		return fmt.Errorf("attach %s.%s: %w", optNode.RMTypeName(), attr.Name(), err)
	}
	return nil
}

// rmDeclaresList reports whether the pinned RM declares attrName a list on
// the class of parentRM, the value the walk built, whatever the template
// writes it as.
func rmDeclaresList(parentRM any, attrName string) bool {
	container, _ := rminfo.Default.IsContainer(bmmtype.Class(rmTypeOf(parentRM)), attrName)
	return container
}

// replacesTerminology reports whether attaching child as attr of parent
// would replace a code phrase's terminology with an empty TERMINOLOGY_ID.
// The generator cannot write a TERMINOLOGY_ID's value except where the
// OPT constrains it, so one built from the BMM, or from an OPT node that
// does not constrain its value, is empty. The terminology the code phrase
// already carries (local from its primitive default, or the one a
// C_CODE_PHRASE named) is kept instead.
func replacesTerminology(parent any, attr string, child any) bool {
	cp, ok := parent.(*rm.CodePhrase)
	if !ok || attr != "terminology_id" || cp.TerminologyID.Value == "" {
		return false
	}
	tid, ok := child.(*rm.TerminologyID)
	return ok && tid.Value == ""
}

// materialiseImplicitSingle creates a default value for a
// BMM-mandatory single attribute the OPT did not pin. The default
// is a fresh zero-value RM instance of the attribute's BMM type;
// the post-walk defaults pass (applyCompositionDefaults etc.) fills
// in well-known fields (e.g. CODE_PHRASE.terminology_id). String-
// and Character-typed BMM attributes (e.g. DV_TEXT.value,
// TERM_MAPPING.match) go through a separate primitive-default path
// because they don't belong in typereg.
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
	// A locatable's node id and name are the identity rule's:
	// setLocatableIdentity has stamped both, the name from the node's
	// term, and fillBMMAttr skips them too, so a node that carries them
	// implicitly is built as one that does not.
	switch attr.Name() {
	case "archetype_node_id":
		return nil
	case "name":
		if _, ok := parentRM.(rm.MutableLocatable); ok {
			return nil
		}
	}
	if compositionDefaultOwns(optNode.RMTypeName(), attr.Name()) {
		return nil
	}
	if g.fillCodeDefault(optNode, parentRM, attr.Name()) {
		return nil
	}
	rmType := attrType(optNode.RMTypeName(), attr.RMTypeName())
	if rmType == "" {
		return nil
	}
	if textTyped(rmType) {
		// BMM String or Character. Write only when the field is still
		// empty, so a clock or code already stored on the parent is left
		// alone.
		g.writeBMMString(parentRM, attr.Name())
		return nil
	}
	rmChild, ok := newRMIfKnown(rmType)
	if !ok {
		// No RM class to build: a BMM primitive such as Boolean or
		// Integer, which the parent holds as a Go value, or a type
		// outside the registry, which the validator flags.
		return nil
	}
	// A default built from the BMM alone has no archetype to name, so it
	// must not be an archetype root: an optional attribute gets nothing,
	// and a required one is refused, as in materialiseImplicitMultiple.
	// No attribute the pinned RM declares single-valued has such a
	// default. A template that writes a list as a single attribute, such
	// as COMPOSITION.content, reaches this only where the BMM marks the
	// list mandatory: materialiseSingle reads any other as the list the
	// RM declares and builds it no member. No list the pinned RM marks
	// mandatory holds archetype roots, so the refusal is a backstop.
	if built := rmTypeOf(rmChild); rmroots.IsArchetypeRoot(built) {
		if !isRequired(attr) {
			return nil
		}
		return fmt.Errorf("%w: %s for %s.%s at %s (required, but the template names no child)",
			ErrArchetypeIDMissing, built, optNode.RMTypeName(), attr.Name(), optNode.AQLPath())
	}
	// Stamp documented sentinel values on DV primitives so the
	// validator's "required attribute absent" check passes for
	// BMM-mandatory implicit attrs the OPT did not constrain.
	g.populatePrimitiveDefault(rmChild)
	// A locatable the OPT does not name (OBSERVATION.data's HISTORY, an
	// ENTRY's ITEM_TREE) still needs the node id and name the RM floor
	// requires.
	g.stampIfLocatable(rmChild, concreteFor(rmType))
	g.populateBMMRequiredAttrs(rmChild, concreteFor(rmType), 0)
	g.settleBuilt(rmChild, attr.Name())
	if replacesTerminology(parentRM, attr.Name(), rmChild) {
		return nil
	}
	// Best-effort attach; if the slot rejects the default (e.g. type
	// mismatch on a polymorphic attr), let downstream defaults
	// (applyCompositionDefaults) own the field.
	_ = rmwrite.EnsureSingle(parentRM, attr.Name(), rmChild)
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
	for _, attrName := range requiredAttributes(parentRMType) {
		g.fillBMMAttr(parent, parentRMType, attrName, depth)
	}
}

// requiredAttributes returns the attributes the pinned RM marks mandatory on
// the bare class class, in declaration order, as
// rminfo.Default.RequiredAttributes does, and nil for a class it does not
// know. It reads them from a table built once rather than building a slice
// per call. The caller must not change the slice.
func requiredAttributes(class string) []string { return requiredAttrTable()[class] }

// requiredAttrTable holds what rminfo.Default.RequiredAttributes answers for
// each class rminfo.Default knows. The pinned RM does not change, so it is
// built on first use and only read after, by any number of goroutines.
var requiredAttrTable = sync.OnceValue(func() map[string][]string {
	known := rminfo.Default.KnownRMTypes()
	table := make(map[string][]string, len(known))
	for _, class := range known {
		table[class] = rminfo.Default.RequiredAttributes(class)
	}
	return table
})

// fillBMMAttr builds a default for attrName of parent from its BMM type
// alone, fills that default's own BMM-mandatory attributes, and attaches
// it. A multi-valued attribute gets one member. Identity and link
// metadata is skipped: the generator stamps it, or the RM does not
// require it.
func (g *generator) fillBMMAttr(parent any, parentRMType, attrName string, depth int) {
	switch attrName {
	case "archetype_node_id", "name", "uid", "archetype_details",
		"links", "feeder_audit":
		return
	}
	rmType, ok := rminfo.Default.AttributeRMType(parentRMType, attrName)
	if !ok || rmType == "" {
		return
	}
	rmType = attrType(parentRMType, rmType)
	if g.fillCodeDefault(nil, parent, attrName) {
		return
	}
	isContainer, _ := rminfo.Default.IsContainer(parentRMType, attrName)
	if textTyped(rmType) {
		g.writeBMMString(parent, attrName)
		return
	}
	concrete := concreteFor(rmType)
	rmChild, ok := newRMIfKnown(concrete)
	if !ok {
		return
	}
	g.populatePrimitiveDefault(rmChild)
	g.stampIfLocatable(rmChild, concrete)
	// Recurse so nested BMM-required attrs (e.g. CODE_PHRASE inside
	// DV_CODED_TEXT) get filled.
	g.populateBMMRequiredAttrs(rmChild, concrete, depth+1)
	g.settleBuilt(rmChild, attrName)
	// Best-effort attach: a default the slot rejects (a polymorphic
	// attribute the BMM cannot narrow) is left to the validator, as in
	// materialiseImplicitSingle.
	switch {
	case isContainer:
		_ = rmwrite.AppendMultiple(parent, attrName, rmChild)
	case !replacesTerminology(parent, attrName, rmChild):
		_ = rmwrite.EnsureSingle(parent, attrName, rmChild)
	}
}

// textTyped reports whether rmType, the BMM type of an attribute, is one the
// generator holds as text: String, or Character, the one-character string
// of a TERM_MAPPING's match. Neither is a class the type registry builds,
// so writeBMMString writes either.
func textTyped(rmType string) bool {
	return rmType == "String" || rmType == "Character"
}

// builtTypeArgument is the type argument the typereg constructor gives a
// generic class the generator builds from its bare name, for the classes
// with a required attribute the BMM types by the class's formal parameter:
// an event is built over ITEM_STRUCTURE, which types its data. An abstract
// EVENT is built as a POINT_EVENT (concreteFor). A HISTORY needs no row:
// the BMM types its events as EVENT, not by its parameter.
var builtTypeArgument = map[string]string{
	"EVENT":          "ITEM_STRUCTURE",
	"POINT_EVENT":    "ITEM_STRUCTURE",
	"INTERVAL_EVENT": "ITEM_STRUCTURE",
}

// attrType resolves declared, the BMM type of an attribute on a value of
// parentType, where it is a formal generic parameter such as the T of
// EVENT.data: to the actual type parentType names, or else to the type
// argument the generator builds parentType's class with. Any other type
// is returned unchanged.
func attrType(parentType, declared string) string {
	if actual := bmmtype.Substitute(parentType, declared); actual != declared {
		return actual
	}
	class := bmmtype.Class(parentType)
	arg, ok := builtTypeArgument[class]
	if !ok {
		return declared
	}
	// Only a formal parameter of class is replaced, so any other type
	// returns before the instantiated name is built.
	if _, formal := bmmtype.ParameterConstraint(class, declared); !formal {
		return declared
	}
	return bmmtype.Substitute(class+"<"+arg+">", declared)
}

// settleBuilt gives a value built from the BMM alone, which the walk
// never reaches, the RM default finishNode gives a walked one where no
// other pass does: an ISM_TRANSITION's current state and a DV_MULTIMEDIA's
// media type (settleCoded), a party's name, a DV_MULTIMEDIA's data and
// size, such as a thumbnail the OPT names with no child, the id, namespace
// and type of a reference (fillReference), a PARTY_RELATIONSHIP's node
// id, uid and references (fillPartyRelationship), and a HISTORY's event,
// built from its BMM type, which RM Events_valid needs where the HISTORY
// has no summary either (REQ-107 Precedence 2: no OPT node names a child
// under its events). attr is the attribute v is built for, which decides
// the type of a reference. No attribute of the pinned RM has the type
// PARTY_RELATED, so the BMM alone never builds one, and its relationship
// is finishNode's alone.
func (g *generator) settleBuilt(v any, attr string) {
	switch b := v.(type) {
	case *rm.History[rm.ItemStructure]:
		if len(b.Events) == 0 && (b.Summary == nil || rm.IsTypedNil(b.Summary)) {
			g.fillBMMAttr(b, "HISTORY", "events", 0)
		}
	case *rm.IsmTransition:
		g.settleCoded(nil, b)
	case *rm.DVMultimedia:
		settleMultimedia(nil, b)
		g.settleCoded(nil, b)
	case *rm.PartyIdentified:
		fillPartyName(&b.Name, len(b.Identifiers), b.ExternalRef)
	case *rm.PartyRelated:
		fillPartyName(&b.Name, len(b.Identifiers), b.ExternalRef)
	case *rm.ObjectRef, *rm.PartyRef, *rm.AccessGroupRef, *rm.LocatableRef:
		g.fillReference(v, attr)
	case *rm.PartyRelationship:
		g.fillPartyRelationship(nil, b)
	}
}

// fillPartyName gives a PARTY_IDENTIFIED, or a PARTY_RELATED, with no
// name, identifiers or external_ref the name "example", so RM
// Basic_validity holds. The RM rule wins over the OPT, whether the OPT
// prohibits name or the policy would not visit it. A name the walk wrote,
// one a C_STRING admits, stays, as does one writeRMNeeded built from the
// name's first allowed OPT child; so "example" goes only where the OPT
// names no allowed child for name.
func fillPartyName(name **string, identifiers int, externalRef *rm.PartyRef) {
	if partyNamed(*name, identifiers, externalRef) {
		return
	}
	example := "example"
	*name = &example
}

// partyNamed reports whether a party already meets RM Basic_validity: it
// has a name, an identifier or an external_ref.
func partyNamed(name *string, identifiers int, externalRef *rm.PartyRef) bool {
	return name != nil || identifiers > 0 || externalRef != nil
}

// populatePrimitiveDefault stamps a minimal-valid sentinel on a
// freshly-built DV value so its primary "value" channel is
// non-empty. Mirrors the REQ-103 ExampleValue sentinels for the
// unbounded cases. RM types that carry no primary value (CLUSTER,
// ELEMENT, party proxies) silently no-op.
func (g *generator) populatePrimitiveDefault(rmValue any) {
	if f, ok := intervalFlags(rmValue); ok {
		// A fresh interval is open on both sides. Each bound the walk
		// writes closes its own side, and settleIntervalEndpoints then
		// decides whether a closed side includes its bound.
		*f.lowerUnbounded = true
		*f.upperUnbounded = true
		return
	}
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
		v.Value = g.temporalSentinel(v)
	case *rm.DVTime:
		v.Value = g.temporalSentinel(v)
	case *rm.DVDateTime:
		v.Value = g.temporalSentinel(v)
	case *rm.DVDuration:
		v.Value = g.temporalSentinel(v)
	case *rm.DVBoolean:
		v.Value = true
	case *rm.DVCount:
		v.Magnitude = 0
	case *rm.DVOrdinal:
		if symbolBlank(v.Symbol) {
			v.Symbol = localSymbol()
		}
	case *rm.DVScale:
		if symbolBlank(v.Symbol) {
			v.Symbol = localSymbol()
		}
	case *rm.DVQuantity:
		// Leave zero — the OPT primitive constraint may further pin.
	case *rm.DVProportion:
		v.Numerator = 1
		v.Denominator = 1
	case *rm.DVURI:
		v.Value = "http://example.com"
	case *rm.DVEHRURI:
		v.Value = "ehr://example"
	case *rm.DVIdentifier:
		v.ID = "example"
	case *rm.DVParsable:
		v.Value = "example"
		v.Formalism = "text/plain"
	}
}

// dateTimeDefault is the ISO 8601 form Options.Now contributes to
// DV_DATE_TIME values, and to EventContext.start_time in
// applyCompositionDefaults. The clock does not change during a call, so
// it is formatted once.
func (g *generator) dateTimeDefault() string {
	if g.nowText == "" {
		g.nowText = g.opts.Now.Format(time.RFC3339)
	}
	return g.nowText
}

// temporalSentinel is the valid ISO 8601 value the generator writes on
// an empty value of a DV_DATE, DV_TIME, DV_DATE_TIME or DV_DURATION, and
// "" for any other value. populatePrimitiveDefault and writeBMMString
// both take it from here, so a temporal value gets the same default
// whichever pass fills it.
func (g *generator) temporalSentinel(v any) string {
	switch v.(type) {
	case *rm.DVDate:
		return "2020-01-01"
	case *rm.DVTime:
		return "12:00:00"
	case *rm.DVDateTime:
		return g.dateTimeDefault()
	case *rm.DVDuration:
		return "P0D"
	}
	return ""
}

// writeBMMString stores a BMM String attribute, or a Character one. A
// field that already holds a value is left alone: populatePrimitiveDefault
// may have set a clock or a code before this pass. The value is
// bmmStringDefault's. It is written through stringField's setter where
// that covers the attribute, and through the writer otherwise.
func (g *generator) writeBMMString(parent any, attr string) {
	ref, covered := stringField(parent, attr)
	if covered && ref.get() != "" {
		return
	}
	val, ok := g.bmmStringDefault(parent, attr)
	if !ok {
		return
	}
	if covered {
		ref.set(val)
		return
	}
	// Best-effort, on purpose: rmwrite refuses a String attribute it does
	// not address, which another default fills or the validator reports,
	// and returning the error would fail Generate on such an OPT. A
	// locatable's archetype_node_id never gets here: the identity rule
	// stamps it, and both callers skip it.
	_ = rmwrite.EnsureSingle(parent, attr, val)
}

// bmmStringDefault is the value writeBMMString gives an empty String
// attribute attr of parent: a temporal value's sentinel, so it stays a
// valid ISO 8601 value; "=" for the magnitude_status of a quantified value
// (a quantity, a count or a temporal value), the value its RM invariant
// Magnitude_status_valid admits; defaultMatch for a TERM_MAPPING's match,
// the RM default, which Match_valid admits; anyActionArchetype, the RM
// default, for an ACTIVITY's action_archetype_id; and the open-string
// example otherwise. ok is false for a TERMINOLOGY_ID's value, which stays
// empty: the code phrase it belongs to keeps the terminology it already
// carries (replacesTerminology). It is false as well for the value of any other
// identifier and for a reference's namespace and type, which stay empty
// until the reference that holds them is settled (fillReference): its id
// value is drawn like a uid, and its type depends on where it sits.
func (g *generator) bmmStringDefault(parent any, attr string) (string, bool) {
	if s := g.temporalSentinel(parent); attr == "value" && s != "" {
		return s, true
	}
	if attr == "magnitude_status" {
		return defaultMagnitudeStatus, true
	}
	switch parent.(type) {
	case *rm.TerminologyID:
		return "", false
	case *rm.HierObjectID, *rm.ObjectVersionID, *rm.ArchetypeID, *rm.TemplateID, *rm.GenericID:
		if attr == "value" {
			return "", false
		}
	case *rm.ObjectRef, *rm.PartyRef, *rm.AccessGroupRef, *rm.LocatableRef:
		if attr == "namespace" || attr == "type" {
			return "", false
		}
	case *rm.TermMapping:
		if attr == "match" {
			return defaultMatch, true
		}
	case *rm.Activity:
		if attr == "action_archetype_id" {
			return anyActionArchetype, true
		}
	}
	return "example", true
}

// anyActionArchetype is the RM default of ACTIVITY.action_archetype_id, a
// pattern in // delimiters that matches the id of any archetype.
const anyActionArchetype = "/.*/"

// stringField returns a reference to the BMM String attribute attr of
// parent, through which it is read and written. It covers every String
// attribute of the data values the generator builds, the magnitude_status
// of each quantified value (a quantity, a count, a proportion and a
// temporal value) and a quantity's units_system and units_display_name
// among them, plus ACTIVITY.action_archetype_id, EVENT_CONTEXT.location,
// INSTRUCTION_DETAILS.activity_id, the value of every identifier class
// and a GENERIC_ID's scheme, the namespace and type of every reference
// class and a LOCATABLE_REF's path, a PARTY_IDENTIFIED's or
// PARTY_RELATED's name, and a TERM_MAPPING's match, a BMM Character held
// as text, so the walk writes a C_STRING the OPT pins on any of them. A
// pin on a reference holds, because the defaults fill only the parts that
// are empty (fillReference). An optional attribute reads as "" while
// unset, and writing it sets it. ok is false when parent has no such
// field; when ok is true, the reference addresses that field.
func stringField(parent any, attr string) (ref stringRef, ok bool) {
	switch p := parent.(type) {
	case *rm.DVText:
		return textField(&p.Value, &p.Formatting, attr)
	case *rm.DVCodedText:
		return textField(&p.Value, &p.Formatting, attr)
	case *rm.CodePhrase:
		switch attr {
		case "code_string":
			return requiredString(&p.CodeString)
		case "preferred_term":
			return optionalString(&p.PreferredTerm)
		}
	case *rm.DVDate:
		return quantifiedField(&p.Value, &p.MagnitudeStatus, attr)
	case *rm.DVTime:
		return quantifiedField(&p.Value, &p.MagnitudeStatus, attr)
	case *rm.DVDateTime:
		return quantifiedField(&p.Value, &p.MagnitudeStatus, attr)
	case *rm.DVDuration:
		return quantifiedField(&p.Value, &p.MagnitudeStatus, attr)
	case *rm.DVCount:
		if attr == "magnitude_status" {
			return optionalString(&p.MagnitudeStatus)
		}
	case *rm.DVURI:
		return valueField(&p.Value, attr)
	case *rm.DVEHRURI:
		return valueField(&p.Value, attr)
	case *rm.DVIdentifier:
		switch attr {
		case "id":
			return requiredString(&p.ID)
		case "issuer":
			return optionalString(&p.Issuer)
		case "assigner":
			return optionalString(&p.Assigner)
		case "type":
			return optionalString(&p.Type)
		}
	case *rm.DVParsable:
		switch attr {
		case "value":
			return requiredString(&p.Value)
		case "formalism":
			return requiredString(&p.Formalism)
		}
	case *rm.DVMultimedia:
		if attr == "alternate_text" {
			return optionalString(&p.AlternateText)
		}
	case *rm.DVQuantity:
		switch attr {
		case "units":
			return requiredString(&p.Units)
		case "magnitude_status":
			return optionalString(&p.MagnitudeStatus)
		case "units_system":
			return optionalString(&p.UnitsSystem)
		case "units_display_name":
			return optionalString(&p.UnitsDisplayName)
		}
	case *rm.DVProportion:
		if attr == "magnitude_status" {
			return optionalString(&p.MagnitudeStatus)
		}
	case *rm.EventContext:
		if attr == "location" {
			return optionalString(&p.Location)
		}
	case *rm.Activity:
		if attr == "action_archetype_id" {
			return requiredString(&p.ActionArchetypeID)
		}
	case *rm.TerminologyID:
		return valueField(&p.Value, attr)
	case *rm.HierObjectID:
		return valueField(&p.Value, attr)
	case *rm.ObjectVersionID:
		return valueField(&p.Value, attr)
	case *rm.ArchetypeID:
		return valueField(&p.Value, attr)
	case *rm.TemplateID:
		return valueField(&p.Value, attr)
	case *rm.GenericID:
		if attr == "scheme" {
			return requiredString(&p.Scheme)
		}
		return valueField(&p.Value, attr)
	case *rm.ObjectRef:
		return referenceField(p, attr)
	case *rm.PartyRef:
		return referenceField(&p.ObjectRef, attr)
	case *rm.AccessGroupRef:
		return referenceField(&p.ObjectRef, attr)
	case *rm.LocatableRef:
		if attr == "path" {
			return optionalString(&p.Path)
		}
		return referenceField(&p.ObjectRef, attr)
	case *rm.InstructionDetails:
		if attr == "activity_id" {
			return requiredString(&p.ActivityID)
		}
	case *rm.PartyIdentified:
		if attr == "name" {
			return optionalString(&p.Name)
		}
	case *rm.PartyRelated:
		if attr == "name" {
			return optionalString(&p.Name)
		}
	case *rm.TermMapping:
		if attr == "match" {
			return stringRef{character: &p.Match}, true
		}
	}
	return stringRef{}, false
}

func textField(value *string, formatting **string, attr string) (stringRef, bool) {
	switch attr {
	case "value":
		return requiredString(value)
	case "formatting":
		return optionalString(formatting)
	}
	return stringRef{}, false
}

// referenceField addresses the String attributes every reference class
// inherits from OBJECT_REF: its namespace and its type.
func referenceField(ref *rm.ObjectRef, attr string) (stringRef, bool) {
	switch attr {
	case "namespace":
		return requiredString(&ref.Namespace)
	case "type":
		return requiredString(&ref.Type)
	}
	return stringRef{}, false
}

func valueField(value *string, attr string) (stringRef, bool) {
	if attr != "value" {
		return stringRef{}, false
	}
	return requiredString(value)
}

// quantifiedField addresses the String attributes of a temporal value: its
// ISO 8601 value and its magnitude_status.
func quantifiedField(value *string, status **string, attr string) (stringRef, bool) {
	if attr == "magnitude_status" {
		return optionalString(status)
	}
	return valueField(value, attr)
}

// stringRef addresses one String attribute of an RM value: a mandatory one
// held as a string, or an optional one held as a *string; or a mandatory
// Character attribute, held as an rm.Character. Exactly one of the three
// pointers is set.
type stringRef struct {
	required  *string
	optional  **string
	character *rm.Character
}

// get returns the attribute's value, or "" while an optional one is unset.
func (r stringRef) get() string {
	switch {
	case r.required != nil:
		return *r.required
	case r.character != nil:
		return string(*r.character)
	case *r.optional == nil:
		return ""
	}
	return **r.optional
}

// set stores s in the attribute, setting an optional one. A Character
// attribute takes s as it is: the walk writes only a match that
// termMappingMatch chose, or defaultMatch, each one character.
func (r stringRef) set(s string) {
	switch {
	case r.required != nil:
		*r.required = s
	case r.character != nil:
		*r.character = rm.Character(s)
	default:
		*r.optional = &s
	}
}

func requiredString(f *string) (stringRef, bool) {
	return stringRef{required: f}, true
}

// optionalString addresses an optional String attribute. It reads as ""
// while unset; writing it sets the attribute, so a C_STRING constraint on
// it is honoured like any other String leaf.
func optionalString(f **string) (stringRef, bool) {
	return stringRef{optional: f}, true
}

// fillCodeDefault writes the RM default code on attr of parent where the
// walk writes that attribute with no OPT child, or from the BMM alone: an
// ENTRY's language and encoding, a DV_TEXT's or DV_CODED_TEXT's language
// and encoding, and a DV_ENCAPSULATED's language and charset
// (codeDefaultFor). The default is written where the attribute holds no
// code (noCode) and the OPT's constraint on it admits the default
// (codeAdmitted); opt is the OPT node of parent, or nil for a value built
// from the BMM alone. It reports whether attr is one of those attributes,
// so the caller does not also build a generic code phrase.
func (g *generator) fillCodeDefault(opt *tcimpl.CompiledNode, parent any, attr string) bool {
	d, ok := g.codeDefaultFor(parent, attr)
	if !ok {
		return false
	}
	if (d.set && !noCode(d.held)) || !codeAdmitted(opt, attr, d.code) {
		return true
	}
	// Best-effort: rmwrite addresses each of these attributes on every
	// class codeDefaultFor names, and a value the walk builds is checked by
	// the validator afterwards.
	_ = rmwrite.EnsureSingle(parent, attr, &d.code)
	return true
}

// codeDefault is the RM default of a code phrase attribute and what the
// attribute holds now.
type codeDefault struct {
	code rm.CodePhrase // the default
	held string        // the code the attribute holds, "" while unset
	set  bool          // the attribute holds a code phrase; an ENTRY's always do
}

// codeDefaultFor returns the RM default of attr of parent: Options.Language
// in ISO_639-1 for a language, and UTF-8 in IANA_character-sets for an
// ENTRY's, a DV_TEXT's or a DV_CODED_TEXT's encoding and a
// DV_ENCAPSULATED's charset, so Language_valid, Encoding_valid and
// Charset_valid hold. ok is false for any other attribute or parent.
func (g *generator) codeDefaultFor(parent any, attr string) (codeDefault, bool) {
	language, charset, charsetAttr, ok := heldCodes(parent)
	if !ok {
		return codeDefault{}, false
	}
	switch attr {
	case "language":
		return codeDefault{code: g.languageCode(), held: codeString(language), set: language != nil}, true
	case charsetAttr:
		return codeDefault{code: utf8Code(), held: codeString(charset), set: charset != nil}, true
	}
	return codeDefault{}, false
}

// codeString is the code of cp, or "" for no code phrase.
func codeString(cp *rm.CodePhrase) string {
	if cp == nil {
		return ""
	}
	return cp.CodeString
}

// noCode reports whether code is no real code: empty, or the placeholder
// at0000 the walk writes where the OPT names a code phrase without a code
// (the primitive default of an unconstrained CODE_PHRASE, and the example
// value of a C_CODE_PHRASE with an empty code list). An RM default
// replaces such a code. at0000 is an archetype node code, never an ISO
// 639-1, ISO 3166-1, IANA character-set or openEHR code, so an OPT cannot
// give it as a real value of an attribute an RM default fills.
func noCode(code string) bool {
	return code == "" || code == "at0000"
}

// heldCodes returns the language parent holds and the code phrase it holds
// in charsetAttr, the attribute that names its character set: encoding on
// an ENTRY, a DV_TEXT and a DV_CODED_TEXT, charset on a DV_MULTIMEDIA and
// a DV_PARSABLE. A nil code phrase is an optional attribute left unset; an
// ENTRY holds both as values, so they are never nil. ok is false for any
// other parent.
func heldCodes(parent any) (language, charset *rm.CodePhrase, charsetAttr string, ok bool) {
	switch p := parent.(type) {
	case *rm.Observation:
		return &p.Language, &p.Encoding, "encoding", true
	case *rm.Evaluation:
		return &p.Language, &p.Encoding, "encoding", true
	case *rm.Instruction:
		return &p.Language, &p.Encoding, "encoding", true
	case *rm.Action:
		return &p.Language, &p.Encoding, "encoding", true
	case *rm.AdminEntry:
		return &p.Language, &p.Encoding, "encoding", true
	case *rm.DVText:
		return p.Language, p.Encoding, "encoding", true
	case *rm.DVCodedText:
		return p.Language, p.Encoding, "encoding", true
	case *rm.DVMultimedia:
		return p.Language, p.Charset, "charset", true
	case *rm.DVParsable:
		return p.Language, p.Charset, "charset", true
	}
	return nil, nil, "", false
}

// settleIntervalEndpoints decides, once the walk has written the bounds
// the OPT constrains, whether each side of an interval includes its
// bound. A bounded side includes it unless the OPT constrains that side's
// *_included, whose example value then applies. An open side never
// includes it: BASE Interval requires that lower_unbounded implies not
// lower_included, and the same for upper. A value that is not an interval
// is left alone.
//
// Two limits follow. A C_BOOLEAN on *_unbounded is not read: a side is
// open or bounded by whether the walk wrote it a bound. And an open side
// keeps *_included false even where the OPT admits only true.
func settleIntervalEndpoints(optNode *tcimpl.CompiledNode, rmValue any) {
	f, ok := intervalFlags(rmValue)
	if !ok {
		return
	}
	*f.lowerIncluded = !*f.lowerUnbounded && includedPerOPT(optNode, "lower_included")
	*f.upperIncluded = !*f.upperUnbounded && includedPerOPT(optNode, "upper_included")
}

// includedPerOPT returns the value the OPT gives an interval's
// lower_included or upper_included: the example value of its first
// C_BOOLEAN the OPT does not prohibit, which is true whenever the
// constraint admits true. Without a constraint
// it returns true, the closed endpoint the template parser also assumes
// when an OPT range omits the flag. It uses the example value under
// RandomFill too, on purpose: a constraint that admits both values then
// still gives a closed endpoint, so the interval stays a sensible minimal
// example rather than a randomly half-open one.
func includedPerOPT(optNode *tcimpl.CompiledNode, attrName string) bool {
	attr := optNode.Attribute(attrName)
	if attr == nil {
		return true
	}
	for _, child := range tcimpl.AllowedChildNodes(attr) {
		if c, ok := child.PrimitiveConstraint().(constraints.CBoolean); ok {
			included, _ := c.ExampleValue().(bool)
			return included
		}
	}
	return true
}

// materialiseMultiple synthesises and appends children under a
// C_MULTIPLE_ATTRIBUTE. Per-child counts honour each OPT child's
// occurrences.lower: max(occurrences.lower, 1) under Large and Medium,
// and occurrences.lower under Required, so an optional child (bounded
// occurrences with a lower bound of 0) gets none of its own there; a child
// with no occurrences, or an unbounded lower, counts as required
// (childRequired) and gets one under every policy. Under Large and Medium a
// child ownMemberSkipped names gets none of its own either. The overall attribute's
// cardinality.upper (default unbounded) caps the total. Within that cap the
// members the children that ask the list for members want (askedMembers)
// get the room first: any other child, before or after them, takes only
// the room they leave, so an optional child cannot take the room a later
// required sibling needs, which checkCardinalityRoom has proved exists.
// The members stay in OPT order. The count the attribute needs
// (remainingLowerNeeded) is met by the top-up below under every policy,
// from the child topUpSeed names. Where only the attribute's
// existence asks for a member, the top-up builds one only of a child that
// needs no undescribed content, and none where there is no such child.
func (g *generator) materialiseMultiple(
	optNode *tcimpl.CompiledNode,
	attr *tcimpl.CompiledAttribute,
	parentRM any,
) error {
	// A child the OPT prohibits gets no member, neither from the
	// per-child fill nor as the seed of the top-up. With every child
	// prohibited the attribute is prohibited and not visited
	// (attrProhibited), unless the BMM marks it mandatory: then it gets
	// here and is built as one the OPT leaves silent.
	children := tcimpl.AllowedChildNodes(attr)
	if len(children) == 0 {
		// Implicit / OPT-silent multi-valued attribute. One the BMM marks
		// mandatory, or whose cardinality lower bound is 1 or more, gets
		// the members it needs (remainingLowerNeeded) as default children
		// of the BMM-resolved element type, so the RM floor's
		// mandatory-attribute check and the validator's cardinality.lower
		// check pass; downstream consumers (REQ-101 Builder) overwrite.
		// One only its existence requires, and an optional one, get no
		// child: a list with no member meets its existence.
		return g.materialiseImplicitMultiple(optNode, attr, parentRM)
	}
	if err := checkCardinalityRoom(optNode, attr, children); err != nil {
		return err
	}
	upperBound := memberCap(attr)
	reserved := askedMembers(children)
	total := 0
	for i, child := range children {
		// Slots are caller-filled; the synthesiser does not invent
		// archetype roots to fill them and instead leaves the count
		// to satisfy the slot's occurrences.lower (often 0 — most
		// slots are optional).
		if child.IsSlot() {
			continue
		}
		if g.ownMemberSkipped(children, i) {
			continue
		}
		asks := asksMember(child)
		childCount := 1
		if occ := child.Occurrences(); occ != nil && !occ.LowerUnbounded() {
			if occ.Lower() > 0 || g.opts.Policy == Required {
				childCount = occ.Lower()
			}
			// occ.Lower()==0 → under Large and Medium, still produce one
			// fill so the resulting tree carries each child the policy
			// keeps at least once (ownMemberSkipped above drops the
			// others). Under Required an optional child gets none: only
			// the top-up below can build one, to meet the attribute's
			// lower bound.
		}
		for range childCount {
			// A child that asks for no member leaves the room the others
			// still need to them.
			if room := upperBound - total; upperBound >= 0 && (room <= 0 || !asks && room <= reserved) {
				break
			}
			rmChild, err := g.buildChild(child)
			if err != nil {
				return err
			}
			if err := rmwrite.AppendMultiple(parentRM, attr.Name(), rmChild); err != nil {
				return fmt.Errorf("append %s.%s: %w", optNode.RMTypeName(), attr.Name(), err)
			}
			total++
			if asks {
				reserved--
			}
		}
	}
	// Top-up to the count the attribute needs when the members appended
	// so far fall short of it, under every policy (e.g. Large or Medium
	// skipped a child the lower bound still needs, or Required built no
	// member of an optional child). Where the BMM, the cardinality or a
	// child's occurrences asks for the member (not existenceOnly) and
	// every OPT child is a slot, the seed is the first slot, and
	// buildChild synthesises a slot-shaped fill: an RM value of the slot's
	// RMTypeName stamped (by stampSlotFill) with an archetype id drawn
	// from the parsed REQ-104 include grammar, or from the RM-type-prefix
	// fallback only when no includes were parsed. Where only the
	// existence asks for it, there is no seed unless a child needs no
	// undescribed content (topUpSeed), and the attribute keeps no member.
	if needed := remainingLowerNeeded(attr, total); needed > 0 {
		seed := g.topUpSeed(optNode, attr, children)
		if seed == nil {
			return nil
		}
		for needed > 0 && (upperBound < 0 || total < upperBound) {
			rmChild, err := g.buildChild(seed)
			if err != nil {
				return err
			}
			if err := rmwrite.AppendMultiple(parentRM, attr.Name(), rmChild); err != nil {
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

// checkCardinalityRoom returns an error wrapping ErrConstraintUnsatisfiable
// when no instance of attr of optNode, a list the OPT declares with the
// allowed children children, can hold every member those children ask for:
// its cardinality upper bound is bounded and less than the sum of their
// occurrences lower bounds (ADL 1.4 VCOC, AOM 2 VACMCO). Only a bounded
// lower bound of 1 or more counts: a child that states no occurrences, or
// whose lower bound is unbounded, asks the list for no member, nor does a
// slot, whose occurrences the parser does not read. The bound read is the
// OPT's, not memberCap's, which a mandatory BMM attribute lifts to 1.
func checkCardinalityRoom(optNode *tcimpl.CompiledNode, attr *tcimpl.CompiledAttribute, children []*tcimpl.CompiledNode) error {
	cm := attr.ChildMultiplicity()
	if cm == nil || cm.UpperUnbounded() {
		return nil
	}
	asked := askedMembers(children)
	if asked <= cm.Upper() {
		return nil
	}
	return fmt.Errorf("%w: %s.%s at %s: the occurrences lower bounds of its children add up to %d, past its cardinality upper bound %d",
		ErrConstraintUnsatisfiable, optNode.RMTypeName(), attr.Name(), optNode.AQLPath(), asked, cm.Upper())
}

// askedMembers returns the number of members children, the allowed children
// of a list, ask the list for: the sum of the occurrences lower bounds of
// those that ask for a member (asksMember).
func askedMembers(children []*tcimpl.CompiledNode) int {
	asked := 0
	for _, c := range children {
		if asksMember(c) {
			asked += c.Occurrences().Lower()
		}
	}
	return asked
}

// asksMember reports whether child asks its list for members: it is not a
// slot, whose occurrences the parser does not read, and its occurrences
// lower bound is bounded and at least 1. A child that states no
// occurrences, or whose lower bound is unbounded, asks for none, although
// materialiseMultiple gives it one member where there is room.
func asksMember(child *tcimpl.CompiledNode) bool {
	return !child.IsSlot() && tcimpl.LowerAtLeastOne(child.Occurrences())
}

// buildChild builds the RM value for the OPT child child and walks it. The
// value is built with its identity (makeChild), then given the primitive
// default of its type before the walk descends: a DV scalar wrapper
// (DV_DURATION, DV_DATE, …) gets a non-empty value (`"value":"P0D"`, …) as a
// fallback where the OPT pins no primitive constraint, which a constraint
// the walk reaches overwrites; any other value is left alone. A slot then
// gets the archetype id of its fill (stampSlotFill), and is refused with
// ErrSlotFillUnsupported when none can be derived. Every attribute builds
// its OPT children here, so a child built to make up a lower bound is the
// same as one built for its own occurrences.
func (g *generator) buildChild(child *tcimpl.CompiledNode) (any, error) {
	rmChild, err := g.makeChild(child)
	if err != nil {
		return nil, err
	}
	g.populatePrimitiveDefault(rmChild)
	if child.IsSlot() && !g.stampSlotFill(rmChild, child) {
		return nil, fmt.Errorf("%w: %s", ErrSlotFillUnsupported, child.AQLPath())
	}
	if err := g.walkNode(child, rmChild); err != nil {
		return nil, err
	}
	return rmChild, nil
}

// stampSlotFill overrides the archetype_node_id and archetype_details
// on a freshly-constructed RM value when a valid slot-fill archetype
// id can be synthesized. It falls back to the RM-type-prefix example
// only for slots without parsed includes; parsed includes must be
// satisfied explicitly. Returns false when no safe id can be derived.
func (g *generator) stampSlotFill(rmValue any, slot *tcimpl.CompiledNode) bool {
	archetypeID := slot.ExampleSlotFillArchetypeID()
	if archetypeID == "" && !slot.SlotRules().HasParsedIncludes() {
		archetypeID = "openEHR-EHR-" + slot.RMTypeName() + ".example.v1"
	}
	if archetypeID == "" || !slot.AllowsArchetypeID(archetypeID) {
		return false
	}
	ad := &rm.Archetyped{
		ArchetypeID: rm.ArchetypeID{Value: archetypeID},
		RMVersion:   rm.Release,
	}
	applyLocatableIdentity(rmValue, archetypeID, slot.RMTypeName(), ad, g.uidFor(rmValue))
	return true
}

// topUpSeed returns the allowed child of attr whose members top attr up to
// the count it needs: the first child that needs no content the OPT does
// not describe and whose identity no earlier sibling carries
// (firstDescribed), else the first that is not a slot, else the first slot.
// Where only attr's existence asks for a member (existenceOnly), it returns
// the first of these or nil, so the attribute gets no undescribed member: a
// list with no member meets its existence. A list the RM makes non-empty
// once present is not such a list (rmNonEmptyList): its existence asks for
// a member, so it takes every source.
func (g *generator) topUpSeed(optNode *tcimpl.CompiledNode, attr *tcimpl.CompiledAttribute, children []*tcimpl.CompiledNode) *tcimpl.CompiledNode {
	if seed := g.firstDescribed(children); seed != nil {
		return seed
	}
	if existenceOnly(optNode, attr) {
		return nil
	}
	if seed := firstNonSlot(children); seed != nil {
		return seed
	}
	// Every child is a slot; children is not empty here.
	return children[0]
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

// materialiseImplicitMultiple creates the default children of a
// multi-valued attribute the OPT gives no allowed child: as many as it
// needs (remainingLowerNeeded) where the BMM marks it mandatory, its
// cardinality lower bound is 1 or more, or it is a list the RM makes
// non-empty once present (rmNonEmptyList) at an existence of 1 or more,
// which materialiseSingle also passes here where the OPT writes it as a
// single attribute. Uses the attribute's BMM element type via
// [concreteFor]; silently no-op when the type is outside the typereg
// registry — the validator will flag it. An attribute that is optional
// (neither BMM-mandatory, nor existence or cardinality lower ≥ 1) gets no
// child, and neither does one only its existence requires (existenceOnly),
// since a list with no member meets its existence. One that needs a child
// that would be an archetype root, such as a COMPOSITION's content or a
// SECTION's items, is refused with [ErrArchetypeIDMissing].
func (g *generator) materialiseImplicitMultiple(
	optNode *tcimpl.CompiledNode,
	attr *tcimpl.CompiledAttribute,
	parentRM any,
) error {
	rmType := attr.RMTypeName()
	if rmType == "" || existenceOnly(optNode, attr) {
		return nil
	}
	// An optional attribute the OPT leaves empty stays empty. A child
	// built from the BMM alone has no archetype to name, so an
	// archetype-rooted one (COMPOSITION.content) would break the RM
	// floor's archetype_details rule; the RM rule needs no such child.
	needed := remainingLowerNeeded(attr, 0)
	if upper := memberCap(attr); upper >= 0 {
		needed = min(needed, upper)
	}
	for range needed {
		rmChild, ok := newRMIfKnown(rmType)
		if !ok {
			// Outside the registry: left to the validator.
			return nil
		}
		// An attribute that needs a child that would be an archetype root
		// is refused instead, for the same reason: the generator does not
		// invent an archetype id.
		if built := rmTypeOf(rmChild); rmroots.IsArchetypeRoot(built) {
			return fmt.Errorf("%w: %s for %s.%s at %s (required, but the template names no child)",
				ErrArchetypeIDMissing, built, optNode.RMTypeName(), attr.Name(), optNode.AQLPath())
		}
		g.populatePrimitiveDefault(rmChild)
		g.stampIfLocatable(rmChild, concreteFor(rmType))
		g.populateBMMRequiredAttrs(rmChild, concreteFor(rmType), 0)
		g.settleBuilt(rmChild, attr.Name())
		_ = rmwrite.AppendMultiple(parentRM, attr.Name(), rmChild)
	}
	return nil
}

// memberCap returns the most members attr may get, or -1 for no bound: its
// cardinality upper bound, but at least 1 where the BMM marks attr
// mandatory, since that RM rule wins over the OPT (REQ-107 Precedence 2).
// So a PARTY's identities at cardinality 0..0 still get the member the
// BMM requires, and the root breaks the OPT's bound instead of the RM's.
func memberCap(attr *tcimpl.CompiledAttribute) int {
	cm := attr.ChildMultiplicity()
	if cm == nil || cm.UpperUnbounded() {
		return -1
	}
	if attr.Required() {
		return max(cm.Upper(), 1)
	}
	return cm.Upper()
}

// remainingLowerNeeded returns the count of members attr still needs,
// given current: its cardinality lower bound (when present), or one member
// when that bound is 0 and the attribute is required (isRequired): the BMM
// marks it mandatory, its existence lower bound is at least 1, or a child
// asks the list for a member (holdsRequiredChild). Cardinality and
// existence are separate in AOM 1.4: existence asks for the container and
// cardinality for its members. So where only the existence asks for the member
// (existenceOnly), the top-up takes it only from a child that needs no
// undescribed content (topUpSeed), and materialiseImplicitMultiple builds
// none: a list with no member meets that existence.
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

// existenceOnly reports whether only the existence of attr, an attribute
// of optNode, asks for a member: its existence lower bound is bounded and
// at least 1, the BMM does not mark it mandatory, the template asks it for
// no member ([tcimpl.AsksForMember]): neither its cardinality lower bound
// nor the occurrences lower bound of an allowed child is bounded and at
// least 1, and it is no list the RM makes non-empty once present
// (rmNonEmptyList). A child with no occurrences does not count, nor does a
// slot, whose occurrences the parser does not read. A multi-valued
// attribute with no member meets such an existence, since existence asks
// for the container and the counts for its members. The template validator
// reads a list's existence the same way, and asks a list the RM makes
// non-empty for a member by its existence too.
func existenceOnly(optNode *tcimpl.CompiledNode, attr *tcimpl.CompiledAttribute) bool {
	return !attr.Required() && !tcimpl.AsksForMember(attr) && tcimpl.LowerAtLeastOne(attr.Existence()) &&
		!rmNonEmptyList(optNode, attr)
}

// rmNonEmptyList reports whether attr of optNode is a list the RM makes
// non-empty once present and that nothing but an item of it keeps so: one
// rminfo.NonEmptyLists names with no alternative, as COMPOSITION
// Content_valid, EVENT_CONTEXT Participations_validity, INSTRUCTION
// Activities_valid, SECTION Items_valid and ATTESTATION Items_valid name
// theirs (`x /= Void implies not x.is_empty`). The template validator reads
// the same table and lets the existence of such a list ask for a member, so
// a list left with none draws its `required`. A HISTORY's events are not
// such a list: Events_valid names the summary as their alternative, and
// writeEventOrSummary gives a HISTORY one of the two.
func rmNonEmptyList(optNode *tcimpl.CompiledNode, attr *tcimpl.CompiledAttribute) bool {
	l, ok := rminfo.NonEmptyListOf(nodeClass(optNode), attr.Name())
	return ok && l.Alternative == ""
}

// makeChild constructs a fresh RM instance for the OPT child's
// rm_type_name and stamps LOCATABLE bookkeeping at the construction
// site (so the value is ready for caller attachment without an
// in-place mutation post-attach). Abstract RM types named by the
// OPT (EVENT, ITEM_STRUCTURE, DATA_VALUE, ITEM, CONTENT_ITEM,
// CARE_ENTRY, ENTRY, LOCATABLE) resolve to a documented concrete
// substitute — see [concreteFor].
//
// A child that would be an archetype root, but for which the OPT
// names no archetype id, is refused with [ErrArchetypeIDMissing]. A
// slot is left to its own path: [generator.stampSlotFill] gives it an
// archetype id, or its caller refuses it with ErrSlotFillUnsupported.
func (g *generator) makeChild(child *tcimpl.CompiledNode) (any, error) {
	rmChild, err := newRMForOPTType(child.RMTypeName())
	if err != nil {
		return nil, fmt.Errorf("makeChild %s: %w", child.RMTypeName(), err)
	}
	if built := rmTypeOf(rmChild); !child.IsSlot() && child.ArchetypeID() == "" && rmroots.IsArchetypeRoot(built) {
		declared := child.RMTypeName()
		if strings.TrimSpace(declared) != built {
			// An abstract declared type is built as a concrete class.
			declared += " (built as " + built + ")"
		}
		return nil, fmt.Errorf("%w: %s at %s", ErrArchetypeIDMissing, declared, child.AQLPath())
	}
	g.setLocatableIdentity(child, rmChild, false /* isTemplateRoot */)
	return rmChild, nil
}

// concreteFor maps abstract RM class names the OPT may declare on
// child constraints to the documented concrete substitute the
// generator materialises — POINT_EVENT for EVENT, ITEM_TREE for
// ITEM_STRUCTURE, ELEMENT for ITEM, etc. Each substitute descends
// from the abstract class in the pinned BMM, and the validation
// walker admits a value by that class hierarchy, so the substitute
// passes its type check. Concrete RM types pass through unchanged.
//
// AOM 1.4 primitive short names (DURATION, DATE, TIME, DATE_TIME,
// BOOLEAN, INTEGER) appear under C_PRIMITIVE_OBJECT in some OPTs
// where the modeller constrains the primitive directly rather than
// its DV wrapper. The generator materialises the canonical DV
// wrapper for each. The short names are no BMM classes, so the
// binding rule the validator and the composition builder share keeps
// its own short-name → wrapper table, primitiveWrappers in
// openehr/internal/rmbind, in lockstep with these cases so neither
// rejects the substitute.
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
	// PARTY_SELF, not PARTY_IDENTIFIED: the generator fills an unconstrained
	// PARTY_PROXY with no attributes, and PARTY_SELF is the only subtype that
	// is RM-valid empty. PARTY_IDENTIFIED requires at least one of `name`,
	// `identifiers` or `external_ref` (invariant `Basic_validity`), so the
	// empty one this used to build was invalid the moment it was generated —
	// and PARTY_SELF is in any case the conventional default for a
	// self-referencing ENTRY `subject`.
	case "PARTY_PROXY":
		return "PARTY_SELF"
	// The id of a reference: a HIER_OBJECT_ID, whose value Identity draws
	// like a uid (fillReference). It is both an OBJECT_ID and a
	// UID_BASED_ID, so it suits an OBJECT_REF's id and a LOCATABLE_REF's.
	case "OBJECT_ID", "UID_BASED_ID":
		return "HIER_OBJECT_ID"
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

// setLocatableIdentity stamps archetype_node_id, name, uid (on the
// classes stampsUID names, unless opt prohibits uid on one that is not a
// PARTY, and on any locatable whose uid opt requires), and
// archetype_details on the freshly-built RM value. The
// isTemplateRoot flag controls whether template_id is stamped on
// archetype_details — only the very top-level root carries it.
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
		// Data values are not locatable. A locatable the OPT left
		// without a node id still needs one: the RM requires it.
		if _, ok := rmValue.(rm.MutableLocatable); !ok || rm.IsTypedNil(rmValue) {
			return
		}
		id = "at0000"
	}

	// Resolve a human-readable runtime name from the OPT term
	// definitions when available; the RM type acts as the fallback. The
	// term is the node's own: an archetype root's is its node id's
	// (at0000), since no term table is keyed by an archetype id. It comes
	// from the node's own archetype, its nearest enclosing archetype root,
	// alone: a node id that archetype does not define keeps the fallback,
	// even where an enclosing archetype defines the same id, since that
	// term names another concept.
	termCode := id
	if opt.ArchetypeID() != "" {
		termCode = opt.NodeID()
	}
	name := opt.RMTypeName()
	if text, found := tcimpl.RootTermItem(opt, termCode, "text"); found && text != "" {
		name = text
	}

	// Archetype-root pins also get a populated archetype_details
	// block with the archetype id; the template id rides on the
	// top-level root only. A node the OPT names no archetype for, the
	// template root included, gets none: an ARCHETYPED needs an
	// archetype id, and the generator does not invent one. Generate has
	// already refused such a template root when its class is always an
	// archetype root; any other class may leave archetype_details out.
	var archetypeDetails *rm.Archetyped
	if arch := opt.ArchetypeID(); arch != "" {
		ad := &rm.Archetyped{
			ArchetypeID: rm.ArchetypeID{Value: arch},
			RMVersion:   rm.Release,
		}
		if isTemplateRoot && g.compiled.TemplateID() != "" {
			ad.TemplateID = &rm.TemplateID{Value: g.compiled.TemplateID()}
		}
		archetypeDetails = ad
	}

	uidSource := g.uidFor(rmValue)
	switch {
	case prohibited(opt, "uid") && !partyNeedsUID(rmValue):
		uidSource = nil
	case requiresUID(opt):
		uidSource = g.nextUID
	}
	applyLocatableIdentity(rmValue, id, name, archetypeDetails, uidSource)
}

// applyPrimitiveExample materialises a primitive leaf's ExampleValue
// against the RM value bound at this OPT node. Closed switch on the
// constraint type because the value shape differs per primitive
// (REQ-103 closed set). leaf is the OPT node that carries pc; attr is
// the attribute of rmValue it constrains, or "" when pc constrains
// rmValue itself. A leaf on an attribute of a data value other than the
// one the switch values (routedPrimitive) is written through rmwrite
// (writePrimitive). A constraint an RM rule narrows, such as a C_INTEGER
// on an attribute of the RM type Integer, which is 32 bits wide, is
// narrowed first, and the value is taken from what is left of it
// (narrowedLeaf); the leaf fails with an error wrapping
// ErrConstraintUnsatisfiable where nothing is left. A C_INTEGER on a
// DV_ORDINAL itself, which AOM 1.4 builds no node for but an OPT can name,
// gives the ordinal its value (applyOrdinal), so it is narrowed as one on
// the value is.
func (g *generator) applyPrimitiveExample(leaf *tcimpl.CompiledNode, rmValue any, attr string, pc constraints.PrimitiveConstraint) error {
	narrowedAttr := attr
	if _, isOrdinal := rmValue.(*rm.DVOrdinal); isOrdinal && attr == "" {
		if _, isInteger := pc.(constraints.CInteger); isInteger {
			narrowedAttr = "value"
		}
	}
	pc, ex, err := narrowedLeaf(leaf, rmValue, narrowedAttr, pc)
	if err != nil {
		return err
	}
	if g.opts.ValueFill == RandomFill {
		// In-constraint sampled value (valid by construction); same Go
		// shape as ExampleValue so the switch below is unchanged. REQ-107.
		ex = sampleValue(pc, g.valueSampler)
	}
	if cs, ok := pc.(constraints.CString); ok {
		return applyStringLeaf(leaf, rmValue, attr, cs, ex)
	}
	if routedPrimitive(rmValue, attr) {
		return writePrimitive(leaf, rmValue, attr, ex)
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
	case *rm.DVText, *rm.DVDate, *rm.DVTime, *rm.DVDateTime, *rm.DVDuration:
		// Each holds its whole value in its String attribute value.
		s, ok := ex.(string)
		if !ok {
			return fmt.Errorf("%s example value is %T, want string", rmTypeOf(rmValue), ex)
		}
		ref, _ := stringField(rmValue, "value")
		ref.set(s)
		return nil
	case *rm.DVCodedText:
		// A C_STRING on .value arrives as a string. A code constraint
		// on the coded text itself arrives as a CodedTermRef.
		switch ex := ex.(type) {
		case string:
			v.Value = ex
			return nil
		case constraints.CodedTermRef:
			v.Value = ex.CodeString
			v.DefiningCode = rm.CodePhrase{
				CodeString:    ex.CodeString,
				TerminologyID: rm.TerminologyID{Value: ex.Terminology},
			}
			useCodedText(leaf, v, g.opts.Language)
			return nil
		default:
			return fmt.Errorf("DV_CODED_TEXT example value is %T, want CodedTermRef or string", ex)
		}
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
		v.Magnitude = rm.Integer64(n)
		return nil
	case *rm.DVOrdinal:
		if err := applyOrdinal(v, pc, ex); err != nil {
			return err
		}
		// The C_DV_ORDINAL lists the symbol's code, not its text.
		useTermText(leaf, &v.Symbol, g.opts.Language)
		return nil
	case *rm.DVMultimedia:
		// A C_INTEGER on size; settleMultimedia replaces it with the
		// byte count of the data where this constraint admits that.
		if attr != "size" {
			return nil
		}
		n, ok := rm.AsInt64(ex)
		if !ok {
			return fmt.Errorf("DV_MULTIMEDIA size example value is %T, want integer", ex)
		}
		v.Size = rm.Integer(n)
		return nil
	}
	// Unknown RM target for this constraint — silently no-op so the
	// generator stays sound on RM types REQ-103 does not yet have a
	// typed primitive for.
	return nil
}

// routedPrimitive reports whether a primitive leaf on attr of rmValue is
// one applyPrimitiveExample writes through rmwrite: a leaf on an attribute
// of a DV_QUANTITY, a DV_COUNT, a temporal value, a DV_BOOLEAN, a DV_TEXT,
// a DV_CODED_TEXT, a DV_MULTIMEDIA, a DV_STATE, a DV_ORDINAL, a DV_SCALE
// or a DV_PROPORTION other than the one the switch there values, such as
// a DV_QUANTITY's precision, a DV_COUNT's or a DV_DURATION's accuracy, a
// DV_STATE's is_terminal, a DV_SCALE's value, or a DV_PROPORTION's
// numerator, an INTERVAL_EVENT's sample_count, and an EHR_STATUS's
// is_queryable and is_modifiable. A leaf on rmValue itself (attr "") and
// a leaf on any other value go to the switch, which leaves one it has no
// case for alone, as an interval's *_unbounded flag, which the walk decides
// by the bounds it writes.
func routedPrimitive(rmValue any, attr string) bool {
	if attr == "" {
		return false
	}
	switch rmValue.(type) {
	case *rm.DVQuantity, *rm.DVState, *rm.EHRStatus, *rm.DVOrdinal, *rm.DVScale, *rm.DVProportion:
		// The switch values a DV_QUANTITY only from a C_DV_QUANTITY on the
		// value itself, and a DV_ORDINAL only from a C_DV_ORDINAL on it. A
		// DV_STATE and an EHR_STATUS, whose primitive attributes are their
		// flags, a DV_SCALE and a DV_PROPORTION have no case there.
		return true
	case *rm.DVText, *rm.DVCodedText, *rm.DVDate, *rm.DVTime, *rm.DVDateTime, *rm.DVDuration, *rm.DVBoolean:
		return attr != "value"
	case *rm.DVCount:
		return attr != "magnitude"
	case *rm.DVMultimedia:
		return attr != "size"
	case *rm.IntervalEvent[rm.ItemStructure]:
		// The one primitive attribute of an event; its others hold
		// values the walk builds from their own OPT nodes.
		return attr == "sample_count"
	}
	return false
}

// writePrimitive writes ex, the value of a primitive leaf on attr of
// rmValue, through rmwrite. rmwrite writes every primitive attribute of the
// values routedPrimitive names but a DV_MULTIMEDIA's data and
// integrity_check, arrays of octets no AOM 1.4 primitive constraint
// describes, so an attribute it does not address
// (rmwrite.ErrUnknownAttribute) is one the pinned RM does not have; it is
// left unwritten, and the template validator reports the constraint. A
// value of the wrong type for the attribute, such as a C_REAL on a
// Boolean, is returned as an error.
func writePrimitive(leaf *tcimpl.CompiledNode, rmValue any, attr string, ex any) error {
	err := rmwrite.EnsureSingle(rmValue, attr, ex)
	if err == nil || errors.Is(err, rmwrite.ErrUnknownAttribute) {
		return nil
	}
	return fmt.Errorf("%s.%s at %s: %w", rmTypeOf(rmValue), attr, leafPath(leaf), err)
}

// applyOrdinal sets the integer and, when the constraint lists pairs,
// the symbol of the pair for that integer. ExampleFill's integer is
// the first pair's value. An empty list keeps the integer and does
// not invent a code.
func applyOrdinal(v *rm.DVOrdinal, pc constraints.PrimitiveConstraint, ex any) error {
	ord, isOrd := pc.(constraints.CDvOrdinal)
	if !isOrd || len(ord.Values) == 0 {
		n, ok := rm.AsInt64(ex)
		if !ok {
			if isOrd {
				v.Value = 0
				return nil
			}
			return fmt.Errorf("DV_ORDINAL example value is %T, want integer", ex)
		}
		v.Value = rm.Integer(n)
		return nil
	}
	n, ok := rm.AsInt64(ex)
	if !ok {
		return fmt.Errorf("DV_ORDINAL example value is %T, want integer", ex)
	}
	pair, found := ordinalPair(ord, n)
	if !found {
		v.Value = rm.Integer(n)
		return nil
	}
	v.Value = rm.Integer(pair.Value)
	v.Symbol = ordinalSymbolText(pair.Symbol)
	return nil
}

// The PROPORTION_KIND codes a DV_PROPORTION's type takes.
const (
	pkRatio           = 0
	pkUnitary         = 1
	pkPercent         = 2
	pkFraction        = 3
	pkIntegerFraction = 4
)

// settleProportion gives a DV_PROPORTION the walk wrote a type, a numerator
// and a denominator that meet the RM rules of its kind: a type that is a
// proportion kind (Type_validity), a denominator of 1 for a unitary
// proportion and of 100 for a percent (Unitary_validity, Percent_validity),
// whole numbers for a fraction or an integer fraction and wherever the
// precision is 0 (Fraction_validity, Precision_validity), and a denominator
// that is not 0 (Valid_denominator). The OPT wins: every value it writes is
// one the OPT's own constraint on that attribute of opt accepts. The type
// the walk wrote is kept where its rules can be met. Else the first other
// kind, in code order, that the OPT admits and whose rules can be met is
// taken. Where none can, the value stays as the walk wrote it. A numerator
// or a denominator that already meets the rules is kept, and a new one is
// the first of a fixed list of candidates (proportionOperand), so nothing
// is drawn and the value is as reproducible as the walk's. The precision is
// never changed. The walk writes each attribute from its own OPT
// constraint, or, where the OPT says nothing, leaves its placeholder: 1
// for the numerator and the denominator, 0 (a ratio) for the type, and no
// precision. So the kind is settled here, once all of them are written.
// opt is the OPT node of p.
func settleProportion(opt *tcimpl.CompiledNode, p *rm.DVProportion) {
	walked := int64(p.Type)
	if settleProportionAs(opt, p, walked) {
		return
	}
	for kind := range int64(pkIntegerFraction + 1) {
		if kind != walked && settleProportionAs(opt, p, kind) {
			return
		}
	}
}

// settleProportionAs settles p as a proportion of kind where the OPT admits
// that type and the rules of kind can be met inside the OPT's constraints
// on numerator and denominator, and reports whether it did. It leaves p
// alone when it did not.
func settleProportionAs(opt *tcimpl.CompiledNode, p *rm.DVProportion, kind int64) bool {
	if kind < pkRatio || kind > pkIntegerFraction || !componentFits(bindingChild(opt, "type"), kind) {
		return false
	}
	whole := wholeOperands(kind, p.Precision)
	denominator, ok := proportionDenominator(bindingChild(opt, "denominator"), float64(p.Denominator), kind, whole)
	if !ok {
		return false
	}
	numerator, ok := proportionOperand(bindingChild(opt, "numerator"), float64(p.Numerator), whole, false)
	if !ok {
		return false
	}
	p.Type = rm.Integer(kind)
	p.Numerator = rm.Real(numerator)
	p.Denominator = rm.Real(denominator)
	return true
}

// wholeOperands reports whether a proportion of kind with precision needs
// a whole-number numerator and denominator: a fraction and an integer
// fraction do (Fraction_validity), and so does any proportion of precision
// 0 (Precision_validity). A whole number is one whose floor is itself, as
// the RM floor reads Is_integral_validity.
func wholeOperands(kind int64, precision *rm.Integer) bool {
	return kind == pkFraction || kind == pkIntegerFraction || (precision != nil && *precision == 0)
}

// proportionDenominator returns the denominator a proportion of kind takes
// under node, the OPT node of its denominator: 1 for a unitary proportion
// and 100 for a percent, where node admits it, and for any other kind a
// value proportionOperand gives that is not 0. It reports false when there
// is none.
func proportionDenominator(node *tcimpl.CompiledNode, current float64, kind int64, whole bool) (float64, bool) {
	switch kind {
	case pkUnitary:
		return 1, componentFits(node, 1.0)
	case pkPercent:
		return 100, componentFits(node, 100.0)
	}
	return proportionOperand(node, current, whole, true)
}

// proportionOperand returns a numerator or a denominator that node, its
// OPT node, accepts, that is a whole number where whole says so, and that
// is not 0 where nonZero says so. It is the first of these that does:
// current; current rounded, then its floor and its ceiling; the members of
// the C_REAL's list; the whole numbers nearest the ends of its range,
// inside it; 1; and the ends of the range themselves. It reports false
// when none does.
func proportionOperand(node *tcimpl.CompiledNode, current float64, whole, nonZero bool) (float64, bool) {
	fits := func(v float64) bool {
		return !math.IsNaN(v) && !math.IsInf(v, 0) &&
			(!whole || math.Trunc(v) == v) && (!nonZero || v != 0) &&
			componentFits(node, v)
	}
	candidates := []float64{current, math.Round(current), math.Floor(current), math.Ceil(current)}
	if c, ok := primitiveOf(node).(constraints.CReal); ok {
		candidates = append(candidates, c.List...)
	}
	lo, hasLo := realEnd(node, true)
	hi, hasHi := realEnd(node, false)
	if hasLo {
		candidates = append(candidates, math.Ceil(lo))
	}
	if hasHi {
		candidates = append(candidates, math.Floor(hi))
	}
	candidates = append(candidates, 1)
	if hasHi {
		candidates = append(candidates, hi)
	}
	if hasLo {
		candidates = append(candidates, lo)
	}
	for _, v := range candidates {
		if fits(v) {
			return v, true
		}
	}
	return 0, false
}

func ordinalPair(c constraints.CDvOrdinal, n int64) (constraints.OrdinalSymbol, bool) {
	for _, s := range c.Values {
		if int64(s.Value) == n {
			return s, true
		}
	}
	return constraints.OrdinalSymbol{}, false
}

func ordinalSymbolText(ref constraints.CodedTermRef) rm.DVCodedText {
	term := ref.Terminology
	if term == "" {
		term = "local"
	}
	return rm.DVCodedText{
		Value: ref.CodeString,
		DefiningCode: rm.CodePhrase{
			CodeString:    ref.CodeString,
			TerminologyID: rm.TerminologyID{Value: term},
		},
	}
}

// applyCompositionDefaults sets the COMPOSITION-specific fields
// per REQ-107 that the walk does not: the composer, and an EVENT_CONTEXT
// with start_time and setting where the walk built none. Called once
// after the OPT-driven walk, whose finishNode has settled the category,
// language and territory (settleCoded): each takes its RM default,
// 433|event|, Options.Language and Options.Territory, where the OPT gives
// it no member of its group or code set and admits the default. No
// EVENT_CONTEXT is created where the OPT prohibits context. The composer
// is Options.Composer, as given, over any party the walk built from the
// OPT. The OPT node of c is the template root.
func (g *generator) applyCompositionDefaults(c *rm.Composition) error {
	root := g.compiled.Root()
	c.Composer = g.opts.Composer
	contextNode := bindingChild(root, "context")
	if c.Context == nil {
		if prohibited(root, "context") {
			return nil
		}
		// The walk settled the setting of a context it built; this one
		// gets the RM default, 238|other care|, where the OPT admits it.
		c.Context = &rm.EventContext{}
		g.settleCoded(contextNode, c.Context)
	}
	if c.Context.StartTime.Value == "" && !prohibited(contextNode, "start_time") {
		c.Context.StartTime = rm.DVDateTime{Value: g.dateTimeDefault()}
	}
	return nil
}

// isRequired reports whether the BMM marks attr mandatory, its existence
// lower bound is bounded and at least 1, or it holds a mandatory child
// (holdsRequiredChild). On a multi-valued attribute that existence asks
// for the container alone: existenceOnly tells where it is the only thing
// that asks for a member. Its cardinality is not read: visitRequired adds
// it.
func isRequired(attr *tcimpl.CompiledAttribute) bool {
	return attr.Required() || tcimpl.LowerAtLeastOne(attr.Existence()) || holdsRequiredChild(attr)
}

// holdsRequiredChild reports whether a child asks the list attr for a
// member while the list holds none ([tcimpl.MandatoryListChildren]): an
// allowed child that is not a slot, with a bounded occurrences lower bound
// of 1 or more, of a list the OPT declares multi-valued. ADL 1.4 makes
// such a child mandatory in its container, and the container is then
// required too, which is the reading the visit rule chooses. The template
// validator reads the same children, and reports each as missing where an
// optional list holds no item, so a list Required leaves out is one it
// accepts as absent. No child counts where the OPT prohibits the list, or
// where the RM types its items as primitive values, such as a
// DV_MULTIMEDIA's data, to which no item binds and of which makeChild
// builds no member. Nor does the child of a single-valued attribute, whose
// existence decides how often that child occurs, or a child that states no
// occurrences, although materialiseMultiple builds one member of it once
// the list is visited for another reason.
func holdsRequiredChild(attr *tcimpl.CompiledAttribute) bool {
	for range tcimpl.MandatoryListChildren(attr) {
		return true // one such child is enough
	}
	return false
}

// newHierObjectID generates a HierObjectID with a random RFC 9562
// version-4 UUID. When Options.UIDSource is nil it gives the uid of each
// locatable stampsUID names (an Entry, a Party) and of any locatable whose
// OPT requires its uid, a PARTY_RELATIONSHIP's uid, and the id value of
// each reference that has none (fillReference). Returns a pointer so
// canjson's polymorphic dispatch on the UIDBasedID interface emits
// the `_type:"HIER_OBJECT_ID"` discriminator the decoder needs to
// round-trip the field. uuid.NewV4 has no error path — it draws from
// crypto/rand, which aborts the process rather than returning an error
// if the operating system has no entropy — so the timestamp fallback
// this replaced was already unreachable.
func newHierObjectID() *rm.HierObjectID {
	return &rm.HierObjectID{Value: uuid.NewV4().String()}
}

// ownMemberSkipped reports whether children[i], an allowed child that is
// not a slot, gets no member of its own under the policy in force. Under
// Large and Medium, an optional child that needs content the OPT does not
// describe gets none (optionalUndescribed), nor does a child whose identity
// a sibling carries (identityTaken). Under Medium, an optional archetype
// root after the first optional archetype root of the attribute that needs
// no such content gets none either (laterOptionalArchetype), so the
// attribute holds at most one optional archetype. Required skips no child
// here: it sizes each child by its occurrences lower bound, which gives an
// optional child no member. The top-up to the attribute's lower bound can
// still build a member of a skipped child.
func (g *generator) ownMemberSkipped(children []*tcimpl.CompiledNode, i int) bool {
	switch g.opts.Policy {
	case Large:
		return g.optionalUndescribed(children[i]) || identityTaken(children, i)
	case Medium:
		return g.optionalUndescribed(children[i]) || identityTaken(children, i) || g.laterOptionalArchetype(children, i)
	case Required:
		return false
	}
	// Unreachable: Generate refuses any other Policy value first.
	return false
}

// laterOptionalArchetype reports whether children[i] is an optional
// archetype root that comes after another optional archetype root among
// its siblings, in OPT order, whatever the archetype of either, that needs
// no content the OPT does not describe. Such an earlier root counts even
// where identityTaken skips it, and a slot is not an archetype root.
func (g *generator) laterOptionalArchetype(children []*tcimpl.CompiledNode, i int) bool {
	return optionalArchetypeRoot(children[i]) && slices.ContainsFunc(children[:i], func(c *tcimpl.CompiledNode) bool {
		return optionalArchetypeRoot(c) && !g.needsUndescribed(c)
	})
}

// optionalArchetypeRoot reports whether child is an archetype root the OPT
// leaves optional: it names an archetype id, is not a slot, and its
// occurrences have a bounded lower bound of 0 (childRequired). A root with
// no occurrences, or with an unbounded lower bound, counts as required.
func optionalArchetypeRoot(child *tcimpl.CompiledNode) bool {
	return !child.IsSlot() && child.ArchetypeID() != "" && !childRequired(child)
}

// identityTaken reports whether children[i] is optional and a sibling
// carries its identity: a required sibling before or after it, or an
// optional sibling before it. Under Large and Medium such a child gets no
// member of its own, a choice of the generator's own (the validator binds
// such members by occurrences room and by trial), though the top-up to the
// attribute's lower bound can still build one, from the child it takes
// (materialiseMultiple); a required child is always built. A slot does not
// count as a sibling. A sibling carries an identity as the validator binds
// a member by it (carriesIdentity), so an archetype root also carries its
// node id: an
// optional child that is not a root gets no member after a root with its
// node id, while optional archetype roots of different archetypes are not
// skipped here, since a root's identity is its archetype id
// (childIdentity); Medium skips the later ones by another rule
// (laterOptionalArchetype). A child with no identity is never skipped. A
// name the OPT pins on a child is not part of it. Optional and required
// are as childRequired reads them: a child with no occurrences, or with an
// unbounded lower bound, counts as required.
func identityTaken(children []*tcimpl.CompiledNode, i int) bool {
	child := children[i]
	if childRequired(child) {
		return false
	}
	if identityCarriedBefore(children, i) {
		return true
	}
	id := childIdentity(child)
	if id == "" {
		return false
	}
	for _, sib := range children[i+1:] {
		if !sib.IsSlot() && childRequired(sib) && carriesIdentity(sib, id) {
			return true
		}
	}
	return false
}

// identityCarriedBefore reports whether a sibling before children[i], other
// than a slot, carries its identity (childIdentity, carriesIdentity). A
// child with no identity is never carried.
func identityCarriedBefore(children []*tcimpl.CompiledNode, i int) bool {
	id := childIdentity(children[i])
	if id == "" {
		return false
	}
	return slices.ContainsFunc(children[:i], func(sib *tcimpl.CompiledNode) bool {
		return !sib.IsSlot() && carriesIdentity(sib, id)
	})
}

// childRequired reports whether child counts as required: every child does
// unless the OPT gives it bounded occurrences whose lower bound is 0, which
// makes it optional. A child with no occurrences, or with an unbounded lower
// bound, is required, as materialiseMultiple sizes it as one member under
// every policy.
func childRequired(child *tcimpl.CompiledNode) bool {
	occ := child.Occurrences()
	return occ == nil || occ.LowerUnbounded() || occ.Lower() > 0
}

// childIdentity returns child's identity: the archetype id for an archetype
// root and the node id for any other child, or "" when it has neither.
func childIdentity(child *tcimpl.CompiledNode) string {
	if id := child.ArchetypeID(); id != "" {
		return id
	}
	return child.NodeID()
}

// carriesIdentity reports whether sib carries the identity id, which is not
// empty: id is sib's archetype id or its node id, the ids the validator
// binds a member to a child by. An archetype root carries both.
func carriesIdentity(sib *tcimpl.CompiledNode, id string) bool {
	return sib.ArchetypeID() == id || sib.NodeID() == id
}

// finishNode fills RM-mandatory fields the OPT walk left empty.
// REQ-107: generated output has to pass the template-less floor.
func (g *generator) finishNode(opt *tcimpl.CompiledNode, rmValue any) {
	g.finishValue(opt, rmValue)
	// After the fills above, which decide whether an ELEMENT keeps its
	// value or its null flavour: each coded attribute the RM keeps within
	// a group or a code set of the openEHR terminology is settled there,
	// the RM default included, where the OPT's constraint admits a member.
	g.settleCoded(opt, rmValue)
}

// finishValue fills the RM-mandatory fields of rmValue, whose OPT node is
// opt, that the OPT walk left empty, but for its coded attributes, which
// finishNode settles next (settleCoded).
func (g *generator) finishValue(opt *tcimpl.CompiledNode, rmValue any) {
	switch v := rmValue.(type) {
	case *rm.DVMultimedia:
		settleMultimedia(opt, v)
	case *rm.DVPeriodicTimeSpecification:
		fillPeriodicFormalism(opt, v)
	case *rm.DVProportion:
		// A bound of an interval is settled here too, before the interval's
		// own walk puts its two bounds in order.
		settleProportion(opt, v)
	case *rm.DVCodedText:
		// Any coded text the OPT builds as a node of its own: an ELEMENT's
		// value or null flavour, a locatable's name, an INTERVAL_EVENT's
		// math function, an ISM_TRANSITION's current state, transition or
		// careflow step, a COMPOSITION's category, a PARTY_RELATED's
		// relationship, a PARTICIPATION's function or mode, a TERM_MAPPING's
		// purpose, and so on.
		useCodedText(opt, v, g.opts.Language)
	case *rm.Cluster:
		// CLUSTER.items is RM-mandatory. ITEM_TREE.items and ITEM_LIST.items
		// are optional, so they get no member here: the walk gives them one
		// where the BMM, the attribute's cardinality or a child's
		// occurrences asks for one, and none where only their existence
		// does.
		g.ensureItems(&v.Items)
	case *rm.PartyRelationship:
		g.fillPartyRelationship(opt, v)
	case *rm.Role:
		// Its walked performer settled itself; this fills one the walk did
		// not write.
		g.fillReference(&v.Performer, "performer")
	case *rm.ObjectRef, *rm.PartyRef, *rm.AccessGroupRef, *rm.LocatableRef:
		g.fillReference(v, holdingAttribute(opt))
	case *rm.PartyIdentified:
		fillPartyName(&v.Name, len(v.Identifiers), v.ExternalRef)
	case *rm.PartyRelated:
		fillPartyName(&v.Name, len(v.Identifiers), v.ExternalRef)
	case *rm.Element:
		g.settleElement(opt, v)
	case *rm.DVEHRURI:
		// Backstop for a DV_EHR_URI the primitive default did not reach;
		// every one the generator emits is walked.
		if v.Value == "" {
			v.Value = "ehr://example"
		}
	case *rm.DVOrdinal:
		if symbolBlank(v.Symbol) {
			v.Symbol = localSymbol()
		}
	case *rm.DVScale:
		if symbolBlank(v.Symbol) {
			v.Symbol = localSymbol()
		}
	}
}

// periodicFormalisms are the two formalisms RM Value_valid admits on a
// DV_PERIODIC_TIME_SPECIFICATION's value, the RM default HL7:PIVL first.
var periodicFormalisms = [...]string{"HL7:PIVL", "HL7:EIVL"}

// fillPeriodicFormalism keeps the formalism of the value of a
// DV_PERIODIC_TIME_SPECIFICATION within the two RM Value_valid admits. A
// formalism the walk wrote that is one of them stays. Otherwise the value
// takes, without a draw, HL7:PIVL where the OPT's C_STRING on the value's
// formalism admits it, else HL7:EIVL where it admits that, and the
// formalism the walk wrote stays where it admits neither. opt is the OPT
// node of p.
func fillPeriodicFormalism(opt *tcimpl.CompiledNode, p *rm.DVPeriodicTimeSpecification) {
	if slices.Contains(periodicFormalisms[:], p.Value.Formalism) {
		return
	}
	var formalism *tcimpl.CompiledAttribute
	if value := bindingChild(opt, "value"); value != nil {
		formalism = value.Attribute("formalism")
	}
	for _, f := range periodicFormalisms {
		if stringAdmitted(formalism, f) {
			p.Value.Formalism = f
			return
		}
	}
}

// openehrCoded is the coded text of code in the openEHR terminology, with
// the pinned rubric of that code in group as its text.
func openehrCoded(group *terminology.Group, code string) rm.DVCodedText {
	rubric, _ := group.Rubric(code)
	return rm.DVCodedText{
		Value: rubric,
		DefiningCode: rm.CodePhrase{
			CodeString:    code,
			TerminologyID: rm.TerminologyID{Value: terminology.ID},
		},
	}
}

// multimediaExample is the inline data the generator gives a
// DV_MULTIMEDIA: the seven bytes of the text "example", which suits the
// text/plain media type it defaults to. It is fixed, so the data is the
// same under both value fills.
const multimediaExample = "example"

// settleMultimedia gives a DV_MULTIMEDIA the RM defaults the OPT left it
// without, where the OPT's own constraint admits them, but for its coded
// attributes, its media type among them, which settleCoded settles. A
// value with no data gets multimediaExample as its data, unless the OPT
// prohibits data, and a value with data takes the number of its bytes as
// its size, the size of the data before any encoding, as the RM defines
// it, unless the OPT's constraint on size rejects that number, where the
// size the walk wrote stays. A value with neither uri nor data, because the OPT
// prohibits data and names no allowed child for uri to build it from
// (writeRMNeeded), gets the uri http://example.com, so RM Not_empty holds,
// where the OPT prohibits uri too: the RM rule wins over the prohibition.
// opt is the OPT node of m, or nil for one built from the BMM alone, such
// as a thumbnail the OPT names with no child.
func settleMultimedia(opt *tcimpl.CompiledNode, m *rm.DVMultimedia) {
	if len(m.Data) == 0 && !prohibited(opt, "data") {
		m.Data = []byte(multimediaExample)
	}
	if size := int64(len(m.Data)); size > 0 && componentFits(bindingChild(opt, "size"), size) {
		m.Size = rm.Integer(size)
	}
	if (m.URI == nil || rm.IsTypedNil(m.URI)) && len(m.Data) == 0 {
		m.URI = &rm.DVURI{Value: "http://example.com"}
	}
}

// prohibited reports whether the OPT prohibits attrName of opt and no RM
// rule needs it (attrProhibited). opt is nil
// for a value built from the BMM alone, which no OPT constrains.
func prohibited(opt *tcimpl.CompiledNode, attrName string) bool {
	if opt == nil {
		return false
	}
	attr := opt.Attribute(attrName)
	return attr != nil && attrProhibited(attr)
}

// codeAdmitted reports whether the OPT's own constraint on attrName of opt
// admits phrase as the attribute's code, so an RM default may be written
// there. It reads the constraint through phraseAdmitted; see
// defaultAdmitted for the cases that admit anything or nothing.
func codeAdmitted(opt *tcimpl.CompiledNode, attrName string, phrase rm.CodePhrase) bool {
	return defaultAdmitted(opt, attrName, func(node *tcimpl.CompiledNode) bool {
		return phraseAdmitted(node, phrase)
	})
}

// codedTextAdmitted reports whether the OPT's own constraint on the coded
// text attrName of opt admits ct: its code through phraseAdmitted, and its
// text through a C_STRING the OPT puts on the coded text's value.
func codedTextAdmitted(opt *tcimpl.CompiledNode, attrName string, ct rm.DVCodedText) bool {
	return defaultAdmitted(opt, attrName, func(node *tcimpl.CompiledNode) bool {
		return codedTextAdmits(node, ct)
	})
}

// codedTextAdmits reports whether node, the OPT child a coded text is
// built from, admits ct: its code through phraseAdmitted, its text
// through a C_STRING on its value. A nil node admits anything.
func codedTextAdmits(node *tcimpl.CompiledNode, ct rm.DVCodedText) bool {
	return node == nil || (phraseAdmitted(node, ct.DefiningCode) && stringAdmitted(node.Attribute("value"), ct.Value))
}

// defaultAdmitted reports whether the OPT's own constraint on attrName of
// opt admits an RM default, as admits says of the alternative the walk
// builds the attribute under ([bindingChild]), among those the OPT does
// not prohibit. An attribute the OPT prohibits admits nothing. opt is nil for a value built
// from the BMM alone, and an attribute the OPT does not name, or names
// with no child, admits any default, as does a mandatory one whose
// children the OPT all prohibits.
func defaultAdmitted(opt *tcimpl.CompiledNode, attrName string, admits func(*tcimpl.CompiledNode) bool) bool {
	if opt == nil {
		return true
	}
	attr := opt.Attribute(attrName)
	if attr == nil {
		return true
	}
	if attrProhibited(attr) {
		return false
	}
	if node := bindingChild(opt, attrName); node != nil {
		return admits(node)
	}
	return true
}

// phraseAdmitted reports whether the OPT node that constrains a code
// phrase, or a coded text through its defining_code, admits phrase. It
// reads the two shapes an OPT gives that constraint, as the template
// validator does: a C_CODE_PHRASE, and a CODE_PHRASE node whose
// code_string, or whose terminology_id's value, carries a C_STRING. Below
// node it reads, like the walk, the alternative the walk builds under
// ([bindingChild]), among those the OPT does not prohibit.
func phraseAdmitted(node *tcimpl.CompiledNode, phrase rm.CodePhrase) bool {
	if cp, ok := node.PrimitiveConstraint().(constraints.CodePhrase); ok {
		ref := constraints.CodedTermRef{Terminology: phrase.TerminologyID.Value, CodeString: phrase.CodeString}
		return len(cp.Validate(ref)) == 0
	}
	if dc := bindingChild(node, "defining_code"); dc != nil {
		return phraseAdmitted(dc, phrase)
	}
	if !stringAdmitted(node.Attribute("code_string"), phrase.CodeString) {
		return false
	}
	if tid := bindingChild(node, "terminology_id"); tid != nil {
		return stringAdmitted(tid.Attribute("value"), phrase.TerminologyID.Value)
	}
	return true
}

// stringAdmitted reports whether the C_STRING the OPT puts on attr, its
// first child the OPT does not prohibit, accepts s. An attribute the OPT
// does not name, or constrains with no C_STRING, accepts any string.
func stringAdmitted(attr *tcimpl.CompiledAttribute, s string) bool {
	if attr == nil {
		return true
	}
	children := tcimpl.AllowedChildNodes(attr)
	if len(children) == 0 {
		return true
	}
	cs, ok := children[0].PrimitiveConstraint().(constraints.CString)
	return !ok || len(cs.Validate(s)) == 0
}

// ensureItems puts the placeholder ELEMENT in an RM-mandatory items list
// the walk left empty: a slot fill's, whose body the OPT does not
// describe. A list the OPT itself sizes is filled by the walk, from its
// OPT children or from its BMM type, and is never empty here, a
// prohibited one included, since the RM requires a member.
func (g *generator) ensureItems(items *[]rm.Item) {
	if len(*items) > 0 {
		return
	}
	*items = append(*items, g.placeholderElement())
}

// placeholderElement is the one member the generator adds to an RM-mandatory
// items list that the OPT does not describe. It has no value constraint to
// fill, so it carries a null flavour (RM Inv_null_flavour_indicated).
func (g *generator) placeholderElement() *rm.Element {
	el := &rm.Element{}
	applyLocatableIdentity(el, "at0000", "element", nil, nil)
	g.settleElement(nil, el)
	return el
}

// settleElement makes an ELEMENT carry exactly one of value and null_flavour
// (RM Inv_null_flavour_indicated), and a null_reason only while it is null
// (RM Inv_null_reason_valid). A value wins, and the null flavour and the
// null reason are dropped, unless it gives way (valueGivesWay): the OPT
// allows both value and null_flavour and requires null_flavour or
// null_reason but not value; the value is then dropped. An ELEMENT with
// no value keeps any null reason and gets the null flavour
// "no information" when it has none, or one with no code (noCode), unless
// the OPT's constraint on null_flavour rejects that code (codeAdmitted);
// where a code-phrase constraint rejects it, the walk's null flavour
// stays. Where the OPT prohibits null_flavour, the RM rule wins: the
// ELEMENT takes a value, which writeRMNeeded has built from the value's
// first allowed OPT child where the walk did not visit it, and which is
// built here as for an ELEMENT.value the OPT leaves silent where the OPT
// names no allowed child; or, where the OPT prohibits value as well, the
// null flavour the walk wrote as if the OPT allowed it (visits,
// writeRMNeeded), with "no information" where that leaves no code its own
// child admits. A null flavour with a code keeps it, with the text the
// walk gave it: the rubric of that code in the openEHR null flavours
// group, where the OPT's constraint on its text admits the rubric
// (useCodedText). opt is the OPT node of e, or nil for an ELEMENT built
// from the BMM alone.
func (g *generator) settleElement(opt *tcimpl.CompiledNode, e *rm.Element) {
	if e.Value != nil && !rm.IsTypedNil(e.Value) {
		if !valueGivesWay(opt) {
			e.NullFlavour = nil
			e.NullReason = nil
			return
		}
		e.Value = nil
	}
	if e.NullFlavour != nil {
		// The walk wrote it: settleCoded, which finishNode runs next, keeps
		// it within the null flavours group.
		return
	}
	nf := noInformation()
	if nullFlavourNeeded(opt) {
		// The OPT prohibits value and null_flavour, and the RM rule needs
		// the null flavour: the walk wrote it as if the OPT allowed it, and
		// the default goes where its own child admits it.
		if codedTextAdmits(bindingChild(opt, "null_flavour"), *nf) {
			e.NullFlavour = nf
		}
		return
	}
	if codedTextAdmitted(opt, "null_flavour", *nf) {
		e.NullFlavour = nf
		return
	}
	if e.NullFlavour != nil {
		return
	}
	// The OPT prohibits null_flavour, and the RM rule that an ELEMENT
	// carry one of value and null_flavour wins: a value as for a silent
	// ELEMENT.value.
	if g.fillElementValue(e) {
		e.NullReason = nil
	}
}

// nullFlavourNeeded reports whether opt, an ELEMENT's OPT node, prohibits
// both value and null_flavour, so the RM rule Inv_null_flavour_indicated
// needs the null flavour, which is then written as if the OPT allowed it.
func nullFlavourNeeded(opt *tcimpl.CompiledNode) bool {
	return prohibited(opt, "null_flavour") && prohibited(opt, "value")
}

// valueGivesWay reports whether an ELEMENT's value gives way to its null
// attributes: the OPT allows both value and null_flavour, and requires
// null_flavour or null_reason but not value. Otherwise the value wins.
func valueGivesWay(opt *tcimpl.CompiledNode) bool {
	if opt == nil || prohibited(opt, "value") || prohibited(opt, "null_flavour") || requiresAttr(opt, "value") {
		return false
	}
	return requiresAttr(opt, "null_flavour") || requiresAttr(opt, "null_reason")
}

// requiresAttr reports whether opt names attrName as required
// (isRequired): the BMM marks it mandatory, its existence lower bound is
// at least 1, or a child asks the list for a member (holdsRequiredChild).
func requiresAttr(opt *tcimpl.CompiledNode, attrName string) bool {
	attr := opt.Attribute(attrName)
	return attr != nil && isRequired(attr)
}

// fillElementValue gives e the value the generator builds for an
// ELEMENT.value the OPT leaves silent: a default of the attribute's BMM
// type. A value the OPT names an allowed child for is built from that
// child instead (writeRMNeeded), so it never gets here. It reports whether
// it wrote one.
func (g *generator) fillElementValue(e *rm.Element) bool {
	rmType, ok := rminfo.Default.AttributeRMType("ELEMENT", "value")
	if !ok {
		return false
	}
	value, known := newRMIfKnown(rmType)
	if !known {
		return false
	}
	g.populatePrimitiveDefault(value)
	g.populateBMMRequiredAttrs(value, concreteFor(rmType), 0)
	return rmwrite.EnsureSingle(e, "value", value) == nil
}

// takeText sets the text of v, a coded text the walk built under node, to
// text where node admits the coded text with it (codedTextAdmits): a
// C_STRING the OPT puts on its value that rejects text leaves the text the
// walk wrote.
func takeText(node *tcimpl.CompiledNode, v *rm.DVCodedText, text string) {
	withText := *v
	withText.Value = text
	if codedTextAdmits(node, withText) {
		*v = withText
	}
}

// noInformation is the "no information" code (271) of the openEHR null
// flavours group.
func noInformation() *rm.DVCodedText {
	nf := openehrCoded(terminology.NullFlavours, "271")
	return &nf
}

func symbolBlank(s rm.DVCodedText) bool {
	return s.DefiningCode.CodeString == "" && s.Value == ""
}

func localSymbol() rm.DVCodedText {
	return rm.DVCodedText{
		Value: "example",
		DefiningCode: rm.CodePhrase{
			CodeString:    "at0000",
			TerminologyID: rm.TerminologyID{Value: "local"},
		},
	}
}

// stampIfLocatable gives a BMM-synthesised locatable the node id and
// name the floor requires when the OPT did not name the node.
func (g *generator) stampIfLocatable(rmValue any, rmType string) {
	loc, ok := rmValue.(rm.Locatable)
	if !ok || rm.IsTypedNil(rmValue) || loc.GetArchetypeNodeID() != "" {
		return
	}
	name := rmType
	if name == "" {
		name = "element"
	}
	applyLocatableIdentity(rmValue, "at0000", name, nil, g.uidFor(rmValue))
	if el, ok := rmValue.(*rm.Element); ok {
		g.settleElement(nil, el)
	}
}

// fillPartyRelationship gives a PARTY_RELATIONSHIP what the RM requires
// and the walk leaves empty: its node id, its uid unless the OPT
// prohibits it or Options.UIDSource returns nil, and the parts of its
// source and target references that are empty (fillReference), so a part
// the OPT pins on either is kept. opt is the relationship's OPT node, or
// nil for one built from the BMM alone.
func (g *generator) fillPartyRelationship(opt *tcimpl.CompiledNode, rel *rm.PartyRelationship) {
	if rel.GetArchetypeNodeID() == "" {
		applyLocatableIdentity(rel, "at0000", "relationship", nil, nil)
	}
	if rel.GetUID() == nil && !prohibited(opt, "uid") {
		if uid := g.nextUID(); uid != nil {
			rel.SetUID(uid)
		}
	}
	g.fillReference(&rel.Source, "source")
	g.fillReference(&rel.Target, "target")
}

// referenceNamespace is the namespace the generator gives a reference
// whose OPT leaves it open.
const referenceNamespace = "local"

// fillReference gives ref, a reference the generator writes, each part
// the RM requires that is empty, so a part the OPT pins is kept. A
// reference with no id takes a HIER_OBJECT_ID (RM defaults), whose value is
// drawn like a uid, from Options.UIDSource or its random fallback
// (Identity). An id with no value, of any identifier class, takes a value
// in its class's form (emptyIDValue). An empty namespace becomes
// referenceNamespace, and an empty type referenceType's for ref held in
// attr. A LOCATABLE_REF holds the UID_BASED_ID id it declares itself. A
// UIDSource that returns nil gives no id and no uid, so a reference with no
// id keeps none, with no nil pointer stored in its place, and an id whose
// value takes the uid keeps its empty one. Any other value is left alone.
func (g *generator) fillReference(ref any, attr string) {
	var base *rm.ObjectRef
	var id any
	var setID func(*rm.HierObjectID)
	switch r := ref.(type) {
	case *rm.ObjectRef:
		base = r
	case *rm.PartyRef:
		base = &r.ObjectRef
	case *rm.AccessGroupRef:
		base = &r.ObjectRef
	case *rm.LocatableRef:
		base = &r.ObjectRef
		id, setID = r.ID, func(h *rm.HierObjectID) { r.ID = h }
	default:
		return
	}
	if setID == nil {
		id, setID = base.ID, func(h *rm.HierObjectID) { base.ID = h }
	}
	if id == nil || rm.IsTypedNil(id) {
		if uid := g.nextUID(); uid != nil {
			setID(uid)
		}
	} else if value, ok := stringField(id, "value"); ok && value.get() == "" {
		if v, ok := g.emptyIDValue(id); ok {
			value.set(v)
		}
	}
	if base.Namespace == "" {
		base.Namespace = referenceNamespace
	}
	if base.Type == "" {
		base.Type = referenceType(ref, attr)
	}
}

// exampleVersionSuffix follows the uid's value in the value of an
// OBJECT_VERSION_ID the generator writes: the creating system example and
// the first trunk version, so the value reads object_id::creating_system_id::
// version_tree_id.
const exampleVersionSuffix = "::example::1"

// exampleArchetypeID is the value of an ARCHETYPE_ID the generator writes
// where the OPT gives none: an id in the form
// rm_originator-rm_name-rm_entity.concept.vN.
const exampleArchetypeID = "openEHR-EHR-CLUSTER.example.v1"

// emptyIDValue returns the value the generator gives id, an identifier an
// object reference holds with no value, in its class's form (Identity): the
// value of a uid drawn from Options.UIDSource or its random fallback for a
// HIER_OBJECT_ID, a GENERIC_ID, a TEMPLATE_ID and a TERMINOLOGY_ID; that
// value followed by exampleVersionSuffix for an OBJECT_VERSION_ID; and
// exampleArchetypeID, with no uid drawn, for an ARCHETYPE_ID. ok is false
// where the form takes a uid and UIDSource returns nil, so the id keeps its
// empty value.
func (g *generator) emptyIDValue(id any) (value string, ok bool) {
	if _, isArchetypeID := id.(*rm.ArchetypeID); isArchetypeID {
		return exampleArchetypeID, true
	}
	uid := g.nextUID()
	if uid == nil {
		return "", false
	}
	if _, isVersionID := id.(*rm.ObjectVersionID); isVersionID {
		return uid.Value + exampleVersionSuffix, true
	}
	return uid.Value, true
}

// referenceType is the type the generator gives ref, a reference held in
// attr, where the OPT leaves the type open: ROLE for a member of an
// ACTOR's roles, the one attribute named roles, and PERSON for any other
// PARTY_REF, so BASE PARTY_REF Type_validity holds; ACCESS_GROUP for an
// ACCESS_GROUP_REF, the one its Type_validity admits; INSTRUCTION for a
// LOCATABLE_REF, which an ACTION's instruction_details holds; and ANY for
// an OBJECT_REF, the type the RM gives for any type.
func referenceType(ref any, attr string) string {
	switch ref.(type) {
	case *rm.PartyRef:
		if attr == "roles" {
			return "ROLE"
		}
		return defaultPartyRefType
	case *rm.AccessGroupRef:
		return accessGroupType
	case *rm.LocatableRef:
		return "INSTRUCTION"
	}
	return "ANY"
}

// holdingAttribute returns the name of the attribute of opt's parent node
// that names opt, or "" for the template root and for a nil opt.
func holdingAttribute(opt *tcimpl.CompiledNode) string {
	if opt == nil || opt.Parent() == nil {
		return ""
	}
	for _, attr := range tcimpl.NodeAttributes(opt.Parent()) {
		if slices.Contains(tcimpl.ChildNodes(attr), opt) {
			return attr.Name()
		}
	}
	return ""
}

// defaultPartyRefType is the type the generator gives a PARTY_REF where the
// OPT leaves it open, but for a member of an ACTOR's roles.
const defaultPartyRefType = "PERSON"

// partyRefTypes are the class names BASE PARTY_REF Type_validity admits as
// a reference's type, in the invariant's order.
var partyRefTypes = []string{"PERSON", "ORGANISATION", "GROUP", "AGENT", "ROLE", "PARTY", "ACTOR"}

// errNoPartyRefType reports a C_STRING on a PARTY_REF's type that accepts
// none of the class names PARTY_REF Type_validity admits.
var errNoPartyRefType = errors.New("the C_STRING accepts no class name PARTY_REF Type_validity admits")

// partyRefType returns the type the generator writes on a PARTY_REF whose
// OPT constrains it with cs, so BASE PARTY_REF Type_validity holds, which
// neither validator evaluates. It is chosen, the string the C_STRING path
// picked, when that is an admitted class name, so a list pin keeps its
// example value and RandomFill its draw; else def, the type the reference
// takes where the OPT leaves it open (referenceType), when cs accepts it;
// else the first admitted class name cs accepts. chosen is "" when the
// C_STRING path found no string. It returns errNoPartyRefType when cs
// accepts no admitted class name.
func partyRefType(cs constraints.CString, chosen, def string) (string, error) {
	return narrowString(cs, chosen, append([]string{def}, partyRefTypes...), errNoPartyRefType)
}

// accessGroupType is the one type ACCESS_GROUP_REF Type_validity admits.
const accessGroupType = "ACCESS_GROUP"

// errNoAccessGroupType reports a C_STRING on an ACCESS_GROUP_REF's type
// that rejects the one type ACCESS_GROUP_REF Type_validity admits.
var errNoAccessGroupType = errors.New("the C_STRING rejects " + accessGroupType + ", the one type ACCESS_GROUP_REF Type_validity admits")

// defaultMatch is the match the generator writes on a TERM_MAPPING whose
// OPT gives it none: the RM default, the kind of mapping is unknown.
const defaultMatch = "?"

// matchCharacters are the characters TERM_MAPPING Match_valid admits as a
// mapping's match. The list starts with the default, so a pin that accepts
// the default gets it.
var matchCharacters = []string{defaultMatch, ">", "=", "<"}

// errNoMatchCharacter reports a C_STRING on a TERM_MAPPING's match that
// accepts none of the characters TERM_MAPPING Match_valid admits.
var errNoMatchCharacter = errors.New("the C_STRING accepts no character TERM_MAPPING Match_valid admits")

// termMappingMatch returns the match the generator writes on a TERM_MAPPING
// whose OPT constrains it with cs, so TERM_MAPPING Match_valid holds: chosen,
// the string the C_STRING path picked, when Match_valid admits it, else the
// first admitted character cs accepts, which is defaultMatch when cs accepts
// it. It returns errNoMatchCharacter when cs accepts no admitted character.
func termMappingMatch(cs constraints.CString, chosen string) (string, error) {
	return narrowString(cs, chosen, matchCharacters, errNoMatchCharacter)
}

// narrowString returns chosen, the string the C_STRING path picked for a
// leaf an RM rule narrows to admitted, when admitted holds it, so a list
// pin keeps its example value and RandomFill its draw; else the first
// member of admitted cs accepts. chosen is "" when the C_STRING path found
// no string. It returns none when cs accepts no member of admitted.
func narrowString(cs constraints.CString, chosen string, admitted []string, none error) (string, error) {
	if slices.Contains(admitted, chosen) {
		return chosen, nil
	}
	for _, s := range admitted {
		if len(cs.Validate(s)) == 0 {
			return s, nil
		}
	}
	return "", none
}

// transitionInto maps each state of the openEHR instruction states group
// that a transition of the instruction state machine leads into to that
// transition, a code of the instruction transitions group. Initial, which
// no transition leads into, is not a key.
var transitionInto = map[string]string{
	"526": "535", // planned: initiate
	"527": "537", // postponed: postpone
	"529": "539", // scheduled: schedule
	"245": "540", // active: start
	"530": "544", // suspended: suspend
	"532": "548", // completed: finish
	"531": "547", // aborted: abort
	"528": "166", // cancelled: cancel
	"533": "549", // expired: time out
}

// defaultTransition is the transition (initiate) the generator writes
// beside the initial state, which no transition leads into, and beside a
// current state outside the instruction states group.
const defaultTransition = "535"

// applyStringLeaf writes a C_STRING leaf onto the String attribute attr
// of rmValue, or onto its main string attribute when attr is "". The
// value is a list member or a pattern match that the constraint accepts,
// or the example under a C_STRING with neither (stringForConstraint),
// except on an ACTIVITY's action_archetype_id, where such a C_STRING gets
// the RM default /.*/ (anyActionArchetype), as where the OPT leaves the
// attribute silent (bmmStringDefault); on a PARTY_REF's type it is also a
// class name PARTY_REF Type_validity admits (see partyRefType), on an
// ACCESS_GROUP_REF's type the one type its Type_validity admits, on a
// TERM_MAPPING's match a character
// TERM_MAPPING Match_valid admits (see termMappingMatch), and on the
// magnitude_status of a quantified value one of the strings DV_QUANTIFIED
// Magnitude_status_valid admits (magnitudeStatuses), the first of them
// the C_STRING accepts, the default = first, where the C_STRING path
// picked none of them. When there is none, it writes nothing and returns
// an error wrapping ErrConstraintUnsatisfiable. An attribute the
// generator has no field for is left alone, like any other unknown
// primitive target.
func applyStringLeaf(leaf *tcimpl.CompiledNode, rmValue any, attr string, cs constraints.CString, ex any) error {
	if attr == "" {
		attr = mainStringAttr(rmValue)
	}
	ref, ok := stringField(rmValue, attr)
	if !ok {
		return nil
	}
	s, err := stringForConstraint(cs, ex)
	switch rmValue.(type) {
	case *rm.PartyRef:
		if attr == "type" {
			// leaf sits on the reference's type, so its parent is the
			// reference's own node.
			var held string
			if leaf != nil {
				held = holdingAttribute(leaf.Parent())
			}
			s, err = partyRefType(cs, s, referenceType(rmValue, held))
		}
	case *rm.AccessGroupRef:
		if attr == "type" {
			s, err = narrowString(cs, s, []string{accessGroupType}, errNoAccessGroupType)
		}
	case *rm.TermMapping:
		if attr == "match" {
			s, err = termMappingMatch(cs, s)
		}
	case *rm.Activity:
		// A C_STRING with neither a pattern nor a list gives no id, so the
		// RM default goes there, as where the OPT leaves the attribute
		// silent (bmmStringDefault).
		if attr == "action_archetype_id" && cs.Pattern == "" && len(cs.List) == 0 {
			s, err = anyActionArchetype, nil
		}
	default:
		if attr == "magnitude_status" && quantified(rmValue) {
			s, err = narrowString(cs, s, magnitudeStatuses, errNoMagnitudeStatus)
		}
	}
	if err != nil {
		return fmt.Errorf("%w: %s.%s at %s: %w", ErrConstraintUnsatisfiable, rmTypeOf(rmValue), attr, leafPath(leaf), err)
	}
	ref.set(s)
	return nil
}

// mainStringAttr names the attribute a C_STRING constrains when the OPT
// puts it on the data value itself rather than on one of its attributes.
func mainStringAttr(rmValue any) string {
	switch rmValue.(type) {
	case *rm.DVIdentifier:
		return "id"
	case *rm.CodePhrase:
		return "code_string"
	case *rm.Activity:
		return "action_archetype_id"
	default:
		return "value"
	}
}

func rmTypeOf(v any) string {
	if t, ok := v.(interface{ BMMName() string }); ok {
		return t.BMMName()
	}
	return fmt.Sprintf("%T", v)
}

func leafPath(leaf *tcimpl.CompiledNode) string {
	if leaf == nil {
		return "?"
	}
	return leaf.AQLPath()
}

// errNoStringValue reports a C_STRING that no string the generator can
// build satisfies: no list member passes the pattern, the pattern matches
// nothing, or the pattern does not compile.
var errNoStringValue = errors.New("no string satisfies the C_STRING constraint")

// stringForConstraint returns a string cs accepts: the example ex when cs
// accepts it, else the first list member cs accepts, else, for a
// pattern-only constraint, the shortest string the pattern's syntax
// builds. Acceptance is cs.Validate, which matches a pattern against the
// whole string, so a value that only contains a match is never
// chosen. It returns errNoStringValue when none of these is accepted.
func stringForConstraint(cs constraints.CString, ex any) (string, error) {
	s, ok := ex.(string)
	if !ok {
		return "", fmt.Errorf("C_STRING example value is %T, want string", ex)
	}
	accepts := func(v string) bool { return len(cs.Validate(v)) == 0 }
	if accepts(s) {
		return s, nil
	}
	for _, member := range cs.List {
		if accepts(member) {
			return member, nil
		}
	}
	if len(cs.List) == 0 && cs.Pattern != "" {
		if m, built := patternExample(cs.Pattern); built && accepts(m) {
			return m, nil
		}
	}
	return "", errNoStringValue
}

// patternExample builds the shortest string the regular expression
// pattern describes: no repetition beyond the minimum, the first
// alternative, and one plain character for a class or a dot. built is
// false when the pattern does not parse or matches nothing.
func patternExample(pattern string) (string, bool) {
	re, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		return "", false
	}
	var b strings.Builder
	if !writeShortest(&b, re.Simplify()) {
		return "", false
	}
	return b.String(), true
}

func writeShortest(b *strings.Builder, re *syntax.Regexp) bool {
	switch re.Op {
	case syntax.OpEmptyMatch, syntax.OpStar, syntax.OpQuest,
		syntax.OpBeginLine, syntax.OpEndLine, syntax.OpBeginText, syntax.OpEndText,
		syntax.OpWordBoundary, syntax.OpNoWordBoundary:
		// Zero width, or zero repetitions. The caller checks the whole
		// string against the pattern, so a boundary that does not hold
		// there is caught.
		return true
	case syntax.OpLiteral:
		for _, r := range re.Rune {
			b.WriteRune(r)
		}
		return true
	case syntax.OpCharClass:
		r, ok := classRune(re.Rune)
		if ok {
			b.WriteRune(r)
		}
		return ok
	case syntax.OpAnyChar, syntax.OpAnyCharNotNL:
		b.WriteByte('a')
		return true
	case syntax.OpCapture, syntax.OpPlus:
		return writeShortest(b, re.Sub[0])
	case syntax.OpRepeat:
		for range re.Min {
			if !writeShortest(b, re.Sub[0]) {
				return false
			}
		}
		return true
	case syntax.OpConcat:
		for _, sub := range re.Sub {
			if !writeShortest(b, sub) {
				return false
			}
		}
		return true
	case syntax.OpAlternate:
		for _, sub := range re.Sub {
			var alt strings.Builder
			if writeShortest(&alt, sub) {
				b.WriteString(alt.String())
				return true
			}
		}
		return false
	case syntax.OpNoMatch:
		// The pattern admits no string.
		return false
	default:
		return false
	}
}

// classRune picks one character of a character class given as rune
// ranges: a letter or digit when the class has one, else the first
// printable ASCII character, else the class's first character.
func classRune(ranges []rune) (rune, bool) {
	if len(ranges) < 2 {
		return 0, false
	}
	in := func(r rune) bool {
		for i := 0; i+1 < len(ranges); i += 2 {
			if ranges[i] <= r && r <= ranges[i+1] {
				return true
			}
		}
		return false
	}
	for _, r := range "aA0" {
		if in(r) {
			return r, true
		}
	}
	for r := rune(0x21); r < 0x7f; r++ {
		if in(r) {
			return r, true
		}
	}
	return ranges[0], true
}
