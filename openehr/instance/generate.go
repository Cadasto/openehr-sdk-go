package instance

import (
	"context"
	"errors"
	"fmt"
	"regexp/syntax"
	"slices"
	"strings"
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
// When the template asks for an object of a class that is always an
// archetype root but names no archetype for it, Generate returns an
// error wrapping [ErrArchetypeIDMissing] and no root. A slot is not
// such an object: the slot-fill rule gives a required slot an
// archetype id from its includes, or the RM-type-prefix example id
// when it has none, or refuses it with [ErrSlotFillUnsupported].
func Generate(ctx context.Context, c *templatecompile.Compiled, opts Options) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c == nil || c.Root() == nil {
		return nil, ErrNilCompiled
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
	switch rootType {
	case "COMPOSITION":
		if err := g.applyCompositionDefaults(root.(*rm.Composition)); err != nil {
			return nil, err
		}
	case "CODE_PHRASE":
		// The implicit terminology_id attribute replaces the terminology
		// the primitive default wrote with an empty TERMINOLOGY_ID, whose
		// value the generator cannot write. A nested code phrase gets its
		// terminology when rmwrite attaches it (local on a coded text,
		// IANA_media-types on a media type); a root is not attached.
		if cp := root.(*rm.CodePhrase); cp.TerminologyID.Value == "" {
			cp.TerminologyID = rm.TerminologyID{Value: "local"}
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
	// either.
	if pc := optNode.PrimitiveConstraint(); pc != nil {
		if g.opts.Policy == Example {
			return g.applyPrimitiveExample(optNode, rmValue, "", pc)
		}
		// Under Minimal we still populate the leaf so the resulting
		// tree is valid (bounded constraints require a value); leaving
		// a zero RM value would surface as primitive_wrong_type or
		// out_of_range at validation. Cheap and aligned with the
		// "structurally complete" Minimal contract.
		return g.applyPrimitiveExample(optNode, rmValue, "", pc)
	}

	if g.opts.ValueFill == RandomFill {
		if unit, ok := sharedQuantityUnit(optNode, g.valueSampler); ok {
			saved := g.valueSampler.quantityUnit
			g.valueSampler.quantityUnit = unit
			defer func() { g.valueSampler.quantityUnit = saved }()
		}
	}

	for _, attr := range optNode.Attributes() {
		if !g.visits(optNode, attr) {
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
	orderIntervalBounds(optNode, rmValue)
	settleIntervalEndpoints(optNode, rmValue)
	g.finishNode(optNode, rmValue)
	return nil
}

// visits decides whether the walk descends into attr of optNode. Under
// either policy it never visits an attribute the OPT prohibits (an
// existence of 0..0), whatever children the OPT names under it, nor one
// the RM computes rather than stores (offset on POINT_EVENT and
// INTERVAL_EVENT, is_integral on DV_QUANTITY and DV_PROPORTION): the
// generator has nothing to write there. Any other attribute is visited
// when the policy says so (shouldVisit).
func (g *generator) visits(optNode *tcimpl.CompiledNode, attr *tcimpl.CompiledAttribute) bool {
	if attrProhibited(attr) {
		return false
	}
	// rminfo knows each class by its bare BMM name; the OPT may declare
	// a generic instantiation (DV_INTERVAL<DV_QUANTITY>).
	if rminfo.IsNonStorableAttr(bmmtype.Class(optNode.RMTypeName()), attr.Name()) {
		return false
	}
	return g.shouldVisit(attr)
}

// attrProhibited reports whether the OPT prohibits attr: its existence
// upper bound is bounded and 0.
func attrProhibited(attr *tcimpl.CompiledAttribute) bool {
	e := attr.Existence()
	return e != nil && !e.UpperUnbounded() && e.Upper() == 0
}

// shouldVisit decides whether an attribute is in scope under the
// current policy. Under Example: every attribute. Under Minimal:
// every attribute that is required (BMM-mandatory OR existence ≥ 1),
// whose cardinality lower bound is 1 or more, OR that has OPT-pinned
// children. The "has OPT children" arm captures the case where the
// OPT explicitly constrains a structurally optional attribute (e.g.
// COMPOSITION.content with archetype-root pins) — the act of pinning
// is itself a signal that the resulting tree should carry those
// children even under the smallest viable build.
func (g *generator) shouldVisit(attr *tcimpl.CompiledAttribute) bool {
	if g.opts.Policy == Example {
		return true
	}
	if isRequired(attr) {
		return true
	}
	// Cardinality lower and existence are separate. A 1..* list is
	// still required when existence does not say so.
	if cm := attr.ChildMultiplicity(); cm != nil && !cm.LowerUnbounded() && cm.Lower() > 0 {
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
			return g.applyPrimitiveExample(child, parentRM, attr.Name(), pc)
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
	if g.fillEntryCode(optNode, parentRM, optNode.RMTypeName(), attr.Name()) {
		return nil
	}
	rmType := attr.RMTypeName()
	if rmType == "" {
		return nil
	}
	if rmType == "String" {
		// BMM String. Write only when the field is still empty, so a
		// clock or code already stored on the parent is left alone.
		g.writeBMMString(parentRM, optNode.RMTypeName(), attr.Name())
		return nil
	}
	rmChild, err := newRMForOPTType(rmType)
	if err != nil {
		// Unknown RM type — silently skip; the OPT is mis-modelled or
		// the attribute is outside the current registry, both of
		// which the validator will flag.
		return nil //nolint:nilerr // intentional: defer to validator
	}
	// A default built from the BMM alone has no archetype to name, so it
	// must not be an archetype root: an optional attribute gets nothing,
	// and a required one is refused, as in materialiseImplicitMultiple.
	// No attribute the pinned RM declares single-valued has such a
	// default; a template reaches this by writing a multi-valued one,
	// such as COMPOSITION.content, as a single attribute.
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
		if g.fillEntryCode(nil, parent, parentRMType, attrName) {
			continue
		}
		isContainer, _ := rminfo.Default.IsContainer(parentRMType, attrName)
		if rmType == "String" {
			g.writeBMMString(parent, parentRMType, attrName)
			continue
		}
		concrete := concreteFor(rmType)
		rmChild, err := rmwrite.NewRM(concrete)
		if err != nil {
			continue
		}
		g.populatePrimitiveDefault(rmChild)
		g.stampIfLocatable(rmChild, concrete)
		if rel, ok := rmChild.(*rm.PartyRelationship); ok {
			g.fillPartyRelationship(rel)
		}
		// Recurse so nested BMM-required attrs (e.g. CODE_PHRASE
		// inside DV_CODED_TEXT) get filled.
		g.populateBMMRequiredAttrs(rmChild, concrete, depth+1)
		// Best-effort attach: a default the slot rejects (a polymorphic
		// attribute the BMM cannot narrow) is left to the validator, as in
		// materialiseImplicitSingle.
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
// DV_DATE_TIME values. It matches the clock applyCompositionDefaults
// uses for EventContext.start_time.
func (g *generator) dateTimeDefault() string {
	return g.opts.Now.Format(time.RFC3339)
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

// writeBMMString stores a BMM String attribute. A field that already
// holds a value is left alone: populatePrimitiveDefault may have set
// a clock or a code before this pass. An empty value of a temporal
// data value takes its temporal sentinel, so it stays a valid ISO 8601
// value; every other empty string keeps the open-string example
// sentinel.
func (g *generator) writeBMMString(parent any, parentType, attr string) {
	cur, known := stringAttr(parent, attr)
	if known && cur != "" {
		return
	}
	val := "example"
	if s := g.temporalSentinel(parent); attr == "value" && s != "" {
		val = s
	}
	// Best-effort, on purpose: the write is refused for a String
	// attribute rmwrite does not address (TERMINOLOGY_ID.value, a
	// locatable's archetype_node_id), and those are filled by another
	// default or reported by the validator. Returning the error would
	// fail Generate on every OPT.
	_ = rmwrite.EnsureSingle(parent, parentType, attr, val)
}

// stringAttr reads a BMM String field the generator itself writes.
// ok is false when parent has no such field under attr.
func stringAttr(parent any, attr string) (string, bool) {
	get, _, ok := stringField(parent, attr)
	if !ok {
		return "", false
	}
	return get(), true
}

// stringField returns a reader and a writer for the BMM String attribute
// attr of parent. It covers every String attribute of the data values the
// generator builds, plus ACTIVITY.action_archetype_id, TERMINOLOGY_ID.value
// and a PARTY_REF's namespace and type, so the walk writes a C_STRING the
// OPT pins on any of them. A pin on a PARTY_REF holds where the reference
// can be attached and no later default replaces it: a ROLE's performer
// keeps it, because fillPerformer fills only the empty parts. A template
// that names a PARTY_RELATIONSHIP's source or target makes Generate fail
// today, because rmwrite cannot attach either; once it can, a pin there
// would still be lost, because fillPartyRelationship replaces a reference
// with any empty part whole, and the walk cannot build the id. An optional
// attribute reads as "" while unset, and its writer sets it. ok is false
// when parent has no such field; when ok is true, get and set are both
// non-nil.
func stringField(parent any, attr string) (get func() string, set func(string), ok bool) {
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
		return valueField(&p.Value, attr)
	case *rm.DVTime:
		return valueField(&p.Value, attr)
	case *rm.DVDateTime:
		return valueField(&p.Value, attr)
	case *rm.DVDuration:
		return valueField(&p.Value, attr)
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
		}
	case *rm.Activity:
		if attr == "action_archetype_id" {
			return requiredString(&p.ActionArchetypeID)
		}
	case *rm.TerminologyID:
		return valueField(&p.Value, attr)
	case *rm.PartyRef:
		switch attr {
		case "namespace":
			return requiredString(&p.Namespace)
		case "type":
			return requiredString(&p.Type)
		}
	}
	return nil, nil, false
}

func textField(value *string, formatting **string, attr string) (func() string, func(string), bool) {
	switch attr {
	case "value":
		return requiredString(value)
	case "formatting":
		return optionalString(formatting)
	}
	return nil, nil, false
}

func valueField(value *string, attr string) (func() string, func(string), bool) {
	if attr != "value" {
		return nil, nil, false
	}
	return requiredString(value)
}

func requiredString(f *string) (func() string, func(string), bool) {
	return func() string { return *f }, func(s string) { *f = s }, true
}

// optionalString reads and writes an optional String attribute. It reads
// as "" while unset; the writer sets the attribute, so a C_STRING
// constraint on it is honoured like any other String leaf.
func optionalString(f **string) (func() string, func(string), bool) {
	get := func() string {
		if *f == nil {
			return ""
		}
		return **f
	}
	return get, func(s string) { *f = &s }, true
}

// fillEntryCode sets ENTRY.language from Options.Language and
// ENTRY.encoding to UTF-8 when that field has no code (noCode) and the
// OPT's constraint on it admits the default (codeAdmitted); opt is the
// OPT node of parent, or nil for an entry built from the BMM alone. It
// reports whether attr is one of those two fields, so the caller does
// not also build a generic code phrase.
func (g *generator) fillEntryCode(opt *tcimpl.CompiledNode, parent any, parentType, attr string) bool {
	phrase, ok := g.entryCodePhrase(parentType, attr)
	if !ok {
		return false
	}
	if !entryCodeEmpty(parent, attr) || !codeAdmitted(opt, attr, phrase) {
		return true
	}
	// Best-effort: every ENTRY parent type is addressed by rmwrite, and an
	// entry the walk builds is checked by the validator afterwards.
	_ = rmwrite.EnsureSingle(parent, parentType, attr, phrase)
	return true
}

func (g *generator) entryCodePhrase(parentType, attr string) (rm.CodePhrase, bool) {
	switch parentType {
	case "OBSERVATION", "EVALUATION", "INSTRUCTION", "ACTION", "ADMIN_ENTRY", "CARE_ENTRY", "ENTRY":
	default:
		return rm.CodePhrase{}, false
	}
	switch attr {
	case "language":
		return rm.CodePhrase{
			CodeString:    g.opts.Language,
			TerminologyID: rm.TerminologyID{Value: "ISO_639-1"},
		}, true
	case "encoding":
		return rm.CodePhrase{
			CodeString:    "UTF-8",
			TerminologyID: rm.TerminologyID{Value: "IANA_character-sets"},
		}, true
	default:
		return rm.CodePhrase{}, false
	}
}

func entryCodeEmpty(parent any, attr string) bool {
	lang, enc, ok := entryCodes(parent)
	if !ok {
		return true
	}
	switch attr {
	case "language":
		return noCode(lang.CodeString)
	case "encoding":
		return noCode(enc.CodeString)
	default:
		return true
	}
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

func entryCodes(parent any) (language, encoding rm.CodePhrase, ok bool) {
	switch p := parent.(type) {
	case *rm.Observation:
		return p.Language, p.Encoding, true
	case *rm.Evaluation:
		return p.Language, p.Encoding, true
	case *rm.Instruction:
		return p.Language, p.Encoding, true
	case *rm.Action:
		return p.Language, p.Encoding, true
	case *rm.AdminEntry:
		return p.Language, p.Encoding, true
	default:
		return rm.CodePhrase{}, rm.CodePhrase{}, false
	}
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
// lower_included or upper_included: the example value of its C_BOOLEAN,
// which is true whenever the constraint admits true. Without a constraint
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
	for _, child := range attr.Children() {
		if c, ok := child.PrimitiveConstraint().(constraints.CBoolean); ok {
			included, _ := c.ExampleValue().(bool)
			return included
		}
	}
	return true
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
		// Implicit / OPT-silent multi-valued attribute. A required one
		// (BMM-mandatory, or existence or cardinality lower of 1 or more)
		// gets one default child of the BMM-resolved element type so the
		// validator's required-attribute / cardinality.lower check
		// passes; downstream consumers (REQ-101 Builder) overwrite. An
		// optional one gets no child.
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
		for range childCount {
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
		RMVersion:   rm.Release,
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
// will flag it. An attribute that is optional (neither BMM-mandatory,
// nor existence or cardinality lower ≥ 1) gets no child. A required
// one whose child would be an archetype root is refused with
// [ErrArchetypeIDMissing].
func (g *generator) materialiseImplicitMultiple(
	optNode *tcimpl.CompiledNode,
	attr *tcimpl.CompiledAttribute,
	parentRM any,
) error {
	rmType := attr.RMTypeName()
	if rmType == "" {
		return nil
	}
	// An optional attribute the OPT leaves empty stays empty. A child
	// built from the BMM alone has no archetype to name, so an
	// archetype-rooted one (COMPOSITION.content) would break the RM
	// floor's archetype_details rule; the RM rule needs no such child.
	if remainingLowerNeeded(attr, 0) == 0 {
		return nil
	}
	rmChild, err := newRMForOPTType(rmType)
	if err != nil {
		return nil //nolint:nilerr // intentional: defer to validator
	}
	// A required attribute whose child would be an archetype root is
	// refused instead, for the same reason: the generator does not
	// invent an archetype id.
	if built := rmTypeOf(rmChild); rmroots.IsArchetypeRoot(built) {
		return fmt.Errorf("%w: %s for %s.%s at %s (required, but the template names no child)",
			ErrArchetypeIDMissing, built, optNode.RMTypeName(), attr.Name(), optNode.AQLPath())
	}
	g.populatePrimitiveDefault(rmChild)
	g.stampIfLocatable(rmChild, concreteFor(rmType))
	if rel, ok := rmChild.(*rm.PartyRelationship); ok {
		g.fillPartyRelationship(rel)
	}
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
	if rel, ok := rmChild.(*rm.PartyRelationship); ok {
		g.fillPartyRelationship(rel)
	}
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
	// PARTY_SELF, not PARTY_IDENTIFIED: the generator fills an unconstrained
	// PARTY_PROXY with no attributes, and PARTY_SELF is the only subtype that
	// is RM-valid empty. PARTY_IDENTIFIED requires at least one of `name`,
	// `identifiers` or `external_ref` (invariant `Basic_validity`), so the
	// empty one this used to build was invalid the moment it was generated —
	// and PARTY_SELF is in any case the conventional default for a
	// self-referencing ENTRY `subject`.
	case "PARTY_PROXY":
		return "PARTY_SELF"
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
		// Data values are not locatable. A locatable the OPT left
		// without a node id still needs one: the RM requires it.
		if _, ok := rmValue.(rm.MutableLocatable); !ok || rm.IsTypedNil(rmValue) {
			return
		}
		id = "at0000"
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

	applyLocatableIdentity(rmValue, id, name, archetypeDetails, g.nextUID)
}

// applyPrimitiveExample materialises a primitive leaf's ExampleValue
// against the RM value bound at this OPT node. Closed switch on the
// constraint type because the value shape differs per primitive
// (REQ-103 closed set). leaf is the OPT node that carries pc; attr is
// the attribute of rmValue it constrains, or "" when pc constrains
// rmValue itself.
func (g *generator) applyPrimitiveExample(leaf *tcimpl.CompiledNode, rmValue any, attr string, pc constraints.PrimitiveConstraint) error {
	ex := pc.ExampleValue()
	if g.opts.ValueFill == RandomFill {
		// In-constraint sampled value (valid by construction); same Go
		// shape as ExampleValue so the switch below is unchanged. REQ-107.
		ex = sampleValue(pc, g.valueSampler)
	}
	if cs, ok := pc.(constraints.CString); ok {
		return applyStringLeaf(leaf, rmValue, attr, cs, ex)
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
		v.Magnitude = n
		return nil
	case *rm.DVOrdinal:
		return applyOrdinal(v, pc, ex)
	case *rm.DVProportion:
		return applyProportionPrimitive(v, attr, ex)
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

// applyProportionPrimitive writes one sampled primitive onto the
// proportion field the OPT named. A C_REAL or C_INTEGER on numerator,
// denominator or type is otherwise discarded: the proportion node itself
// has no primitive constraint, so the value stayed at the sentinel.
func applyProportionPrimitive(v *rm.DVProportion, attr string, ex any) error {
	switch attr {
	case "numerator", "denominator", "accuracy":
		f, ok := float64Value(ex)
		if !ok {
			return fmt.Errorf("DV_PROPORTION %s example value is %T, want real", attr, ex)
		}
		switch attr {
		case "numerator":
			v.Numerator = rm.Real(f)
		case "denominator":
			v.Denominator = rm.Real(f)
		default:
			a := rm.Real(f)
			v.Accuracy = &a
		}
		return nil
	case "type":
		n, ok := rm.AsInt64(ex)
		if !ok {
			return fmt.Errorf("DV_PROPORTION type example value is %T, want integer", ex)
		}
		v.Type = rm.Integer(n)
		return nil
	case "precision":
		n, ok := rm.AsInt64(ex)
		if !ok {
			return fmt.Errorf("DV_PROPORTION precision example value is %T, want integer", ex)
		}
		p := rm.Integer(n)
		v.Precision = &p
		return nil
	default:
		return nil
	}
}

func float64Value(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	default:
		i, ok := rm.AsInt64(v)
		return float64(i), ok
	}
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
// per REQ-107: category 433|event|, language, territory, composer,
// and an EVENT_CONTEXT with start_time and setting. Called once after
// the OPT-driven walk so the values land regardless of whether the OPT
// pinned them. Each coded default yields to the OPT: it is written only
// where the OPT's own constraint on that attribute admits it
// (codeAdmitted), and no EVENT_CONTEXT is created where the OPT
// prohibits context. The OPT node of c is the template root.
func (g *generator) applyCompositionDefaults(c *rm.Composition) error {
	root := g.compiled.Root()
	event := rm.CodePhrase{CodeString: "433", TerminologyID: rm.TerminologyID{Value: terminology.ID}}
	if noCode(c.Category.DefiningCode.CodeString) {
		if codeAdmitted(root, "category", event) {
			// The rubric comes from the pinned `composition category`
			// group, never typed beside the code (REQ-034).
			value, _ := terminology.CompositionCategory.Rubric(event.CodeString)
			c.Category = rm.DVCodedText{Value: value, DefiningCode: event}
		}
	} else {
		// The OPT pinned the code, and the walk left the synthesiser's text
		// beside it.
		useGroupRubric(&c.Category, terminology.CompositionCategory)
	}
	language := rm.CodePhrase{CodeString: g.opts.Language, TerminologyID: rm.TerminologyID{Value: "ISO_639-1"}}
	if noCode(c.Language.CodeString) && codeAdmitted(root, "language", language) {
		c.Language = language
	}
	territory := rm.CodePhrase{CodeString: g.opts.Territory, TerminologyID: rm.TerminologyID{Value: "ISO_3166-1"}}
	if noCode(c.Territory.CodeString) && codeAdmitted(root, "territory", territory) {
		c.Territory = territory
	}
	if c.Composer == nil {
		c.Composer = g.opts.Composer
	}
	if c.Context == nil {
		if prohibited(root, "context") {
			return nil
		}
		c.Context = &rm.EventContext{}
	}
	if c.Context.StartTime.Value == "" {
		c.Context.StartTime = rm.DVDateTime{Value: g.opts.Now.Format(time.RFC3339)}
	}
	// EventContext.Setting is BMM-mandatory and carries the RM invariant
	// Setting_valid — its defining code MUST be a member of the openEHR
	// terminology's `setting` group (RM composition package, EVENT_CONTEXT).
	// "Populated" is therefore not enough: a template that leaves setting
	// unconstrained lets the generic example synthesiser invent an
	// archetype-local code (`local`/`example`), which reads as populated and
	// still violates the invariant. Nor is "openehr-coded" enough: a template
	// can pin an `openehr` code that is not in the group. All three cases
	// take the documented default where the template's own constraint on
	// setting admits it, because the pinned terminology tables (REQ-034)
	// answer membership directly (REQ-107). Where that constraint rejects
	// the default, such as a pin of one non-member code or of another
	// terminology, the walk's value stays and Setting_valid can fail.
	// Checking the invariant on a composition the generator did not build
	// stays a REQ-112 RM-floor job.
	otherCare := rm.CodePhrase{CodeString: "238", TerminologyID: rm.TerminologyID{Value: terminology.ID}}
	if (c.Context.Setting.DefiningCode.CodeString == "" ||
		c.Context.Setting.DefiningCode.TerminologyID.Value != terminology.ID ||
		!terminology.Setting.Has(c.Context.Setting.DefiningCode.CodeString)) &&
		codeAdmitted(firstChild(root, "context"), "setting", otherCare) {
		rubric, _ := terminology.Setting.Rubric(otherCare.CodeString)
		c.Context.Setting = rm.DVCodedText{Value: rubric, DefiningCode: otherCare}
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

// newHierObjectID generates a HierObjectID with a random RFC 9562
// version-4 UUID. When Options.UIDSource is nil it gives the uid of each
// locatable stampsUID names (a Composition, an Entry, a Party), and a
// PARTY_RELATIONSHIP's uid and empty source or target id. Returns a
// pointer so
// canjson's polymorphic dispatch on the UIDBasedID interface emits
// the `_type:"HIER_OBJECT_ID"` discriminator the decoder needs to
// round-trip the field. uuid.NewV4 has no error path — it draws from
// crypto/rand, which aborts the process rather than returning an error
// if the operating system has no entropy — so the timestamp fallback
// this replaced was already unreachable.
func newHierObjectID() *rm.HierObjectID {
	return &rm.HierObjectID{Value: uuid.NewV4().String()}
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

// finishNode fills RM-mandatory fields the OPT walk left empty.
// REQ-107: generated output has to pass the template-less floor.
func (g *generator) finishNode(opt *tcimpl.CompiledNode, rmValue any) {
	// An OPT can name an ENTRY's language or encoding without a code; the
	// walk then leaves the placeholder there for the RM default to replace,
	// where the OPT's constraint on that attribute admits the default.
	if _, _, ok := entryCodes(rmValue); ok {
		g.fillEntryCode(opt, rmValue, opt.RMTypeName(), "language")
		g.fillEntryCode(opt, rmValue, opt.RMTypeName(), "encoding")
	}
	switch v := rmValue.(type) {
	case *rm.Action:
		if v.Time.Value == "" {
			v.Time = rm.DVDateTime{Value: g.dateTimeDefault()}
		}
	case *rm.IsmTransition:
		fillCurrentState(opt, v)
	case *rm.IntervalEvent[rm.ItemStructure]:
		// typereg builds every INTERVAL_EVENT with this instantiation.
		fillMathFunction(opt, &v.MathFunction)
	case *rm.DVMultimedia:
		settleMultimedia(opt, v)
	case *rm.Cluster:
		// CLUSTER.items is RM-mandatory. ITEM_TREE.items and ITEM_LIST.items
		// are optional, so they get no member here: the walk gives them one
		// when the OPT requires it, and none when the OPT leaves them
		// optional.
		g.ensureItems(opt, &v.Items)
	case *rm.PartyRelationship:
		g.fillPartyRelationship(v)
	case *rm.Role:
		fillPerformer(&v.Performer)
	case *rm.Element:
		settleElement(opt, v)
	case *rm.Activity:
		if v.ActionArchetypeID == "" {
			v.ActionArchetypeID = "openEHR-EHR-ACTION.example.v1"
		}
	case *rm.ItemSingle:
		if v.Item.GetArchetypeNodeID() == "" && (v.Item.Value == nil || rm.IsTypedNil(v.Item.Value)) {
			v.Item = *g.placeholderElement()
		}
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

// fillMathFunction gives an INTERVAL_EVENT's math function the code 146
// (mean) of the openEHR event math function group, with that code's
// rubric, when the OPT gave it no code (noCode) and the OPT's constraint
// on it admits that code (codeAdmitted), so RM Math_function_validity
// holds. A code the OPT gave is kept. opt is the OPT node of the event.
func fillMathFunction(opt *tcimpl.CompiledNode, mf *rm.DVCodedText) {
	const code = "146"
	mean := rm.CodePhrase{CodeString: code, TerminologyID: rm.TerminologyID{Value: terminology.ID}}
	if !noCode(mf.DefiningCode.CodeString) || !codeAdmitted(opt, "math_function", mean) {
		return
	}
	rubric, _ := terminology.EventMathFunction.Rubric(code)
	*mf = rm.DVCodedText{Value: rubric, DefiningCode: mean}
}

// settleMultimedia gives a DV_MULTIMEDIA the RM defaults the OPT left it
// without, where the OPT's own constraint admits them. A media type with
// no code (noCode) becomes text/plain in IANA_media-types, so RM
// Media_type_valid holds, unless the OPT's constraint on media_type
// rejects that, such as a C_CODE_PHRASE that names the terminology
// openEHR (codeAdmitted); a code the OPT gave is kept. A value with
// neither uri nor data gets the uri http://example.com, so RM Not_empty
// holds, unless the OPT prohibits uri. opt is the OPT node of m.
func settleMultimedia(opt *tcimpl.CompiledNode, m *rm.DVMultimedia) {
	textPlain := rm.CodePhrase{
		CodeString:    "text/plain",
		TerminologyID: rm.TerminologyID{Value: "IANA_media-types"},
	}
	if noCode(m.MediaType.CodeString) && codeAdmitted(opt, "media_type", textPlain) {
		m.MediaType = textPlain
	}
	if (m.URI == nil || rm.IsTypedNil(m.URI)) && len(m.Data) == 0 && !prohibited(opt, "uri") {
		m.URI = &rm.DVURI{Value: "http://example.com"}
	}
}

// prohibited reports whether the OPT prohibits attrName of opt with an
// existence of 0..0. opt is nil for a value built from the BMM alone,
// which no OPT constrains.
func prohibited(opt *tcimpl.CompiledNode, attrName string) bool {
	if opt == nil {
		return false
	}
	attr := opt.Attribute(attrName)
	return attr != nil && attrProhibited(attr)
}

// codeAdmitted reports whether the OPT's own constraint on attrName of opt
// admits phrase as the attribute's code, so an RM default may be written
// there. An attribute the OPT prohibits admits nothing. Otherwise the
// first OPT child is read, the one the walk builds the attribute from
// (phraseAdmitted). opt is nil for a value built from the BMM alone, and
// an attribute the OPT does not name, or names with no child, admits any
// phrase.
func codeAdmitted(opt *tcimpl.CompiledNode, attrName string, phrase rm.CodePhrase) bool {
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
	if len(attr.Children()) == 0 {
		return true
	}
	return phraseAdmitted(attr.Children()[0], phrase)
}

// phraseAdmitted reports whether the OPT node that constrains a code
// phrase, or a coded text through its defining_code, admits phrase. It
// reads the two shapes an OPT gives that constraint, as the template
// validator does: a C_CODE_PHRASE, and a CODE_PHRASE node whose
// code_string, or whose terminology_id's value, carries a C_STRING.
func phraseAdmitted(node *tcimpl.CompiledNode, phrase rm.CodePhrase) bool {
	if cp, ok := node.PrimitiveConstraint().(constraints.CodePhrase); ok {
		ref := constraints.CodedTermRef{Terminology: phrase.TerminologyID.Value, CodeString: phrase.CodeString}
		return len(cp.Validate(ref)) == 0
	}
	if dc := node.Attribute("defining_code"); dc != nil && len(dc.Children()) > 0 {
		return phraseAdmitted(dc.Children()[0], phrase)
	}
	if !stringAdmitted(node.Attribute("code_string"), phrase.CodeString) {
		return false
	}
	if tid := node.Attribute("terminology_id"); tid != nil && len(tid.Children()) > 0 {
		return stringAdmitted(tid.Children()[0].Attribute("value"), phrase.TerminologyID.Value)
	}
	return true
}

// stringAdmitted reports whether the C_STRING the OPT puts on attr, its
// first child, accepts s. An attribute the OPT does not name, or
// constrains with no C_STRING, accepts any string.
func stringAdmitted(attr *tcimpl.CompiledAttribute, s string) bool {
	if attr == nil || len(attr.Children()) == 0 {
		return true
	}
	cs, ok := attr.Children()[0].PrimitiveConstraint().(constraints.CString)
	return !ok || len(cs.Validate(s)) == 0
}

// ensureItems puts one member in an RM-mandatory items list. When the
// OPT names a child, that child is used so the template's RM type is
// kept. A list the OPT does not describe gets one ELEMENT.
func (g *generator) ensureItems(opt *tcimpl.CompiledNode, items *[]rm.Item) {
	if len(*items) > 0 {
		return
	}
	if opt != nil {
		if attr := opt.Attribute("items"); attr != nil && len(attr.Children()) > 0 {
			for _, child := range attr.Children() {
				made, err := g.makeChild(child)
				if err != nil {
					continue
				}
				// A slot fill takes the walk's slot branch, like the fills
				// materialiseSingle and materialiseMultiple make.
				if child.IsSlot() && !g.stampSlotFill(made, child) {
					continue
				}
				if err := g.walkNode(child, made); err != nil {
					continue
				}
				item, ok := made.(rm.Item)
				if !ok {
					continue
				}
				*items = append(*items, item)
				return
			}
			return
		}
	}
	*items = append(*items, g.placeholderElement())
}

// placeholderElement is the one member the generator adds to an RM-mandatory
// items list that the OPT does not describe. It has no value constraint to
// fill, so it carries a null flavour (RM Inv_null_flavour_indicated).
func (g *generator) placeholderElement() *rm.Element {
	el := &rm.Element{}
	applyLocatableIdentity(el, "at0000", "element", nil, g.nextUID)
	settleElement(nil, el)
	return el
}

// settleElement makes an ELEMENT carry exactly one of value and null_flavour
// (RM Inv_null_flavour_indicated), and a null_reason only while it is null
// (RM Inv_null_reason_valid). A value wins: when the OPT constrains the value
// and either null attribute, the null flavour and the null reason are both
// dropped. An ELEMENT with no value, because the OPT constrains none or none
// could be generated, keeps any null reason and gets the null flavour
// "no information" when it has none, unless the OPT's constraint on
// null_flavour rejects that code, as a prohibited null_flavour does
// (codeAdmitted); it then has neither. A null flavour the OPT filled keeps
// its code, and takes the pinned rubric of that code when the code is in
// the openEHR null flavours group. opt is the OPT node of e, or nil for an
// ELEMENT built from the BMM alone.
func settleElement(opt *tcimpl.CompiledNode, e *rm.Element) {
	if e.Value != nil && !rm.IsTypedNil(e.Value) {
		e.NullFlavour = nil
		e.NullReason = nil
		return
	}
	if e.NullFlavour == nil {
		if nf := noInformation(); codeAdmitted(opt, "null_flavour", nf.DefiningCode) {
			e.NullFlavour = nf
		}
		return
	}
	useGroupRubric(e.NullFlavour, terminology.NullFlavours)
}

// useGroupRubric sets the text of a coded text to the pinned rubric of its
// code, when that code is an `openehr` code in group. The walk fills a code
// the OPT pins but leaves the synthesiser's placeholder text beside it. A
// code in another terminology, or outside group, keeps its text, so the
// generator invents no rubric for it.
func useGroupRubric(v *rm.DVCodedText, group *terminology.Group) {
	if v.DefiningCode.TerminologyID.Value != terminology.ID {
		return
	}
	if rubric, ok := group.Rubric(v.DefiningCode.CodeString); ok {
		v.Value = rubric
	}
}

// noInformation is the "no information" code (271) of the openEHR null
// flavours group.
func noInformation() *rm.DVCodedText {
	const code = "271"
	rubric, _ := terminology.NullFlavours.Rubric(code)
	return &rm.DVCodedText{
		Value: rubric,
		DefiningCode: rm.CodePhrase{
			CodeString:    code,
			TerminologyID: rm.TerminologyID{Value: terminology.ID},
		},
	}
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
	applyLocatableIdentity(rmValue, "at0000", name, nil, g.nextUID)
	if el, ok := rmValue.(*rm.Element); ok {
		settleElement(nil, el)
	}
}

func (g *generator) fillPartyRelationship(rel *rm.PartyRelationship) {
	if rel.GetArchetypeNodeID() == "" {
		applyLocatableIdentity(rel, "at0000", "relationship", nil, g.nextUID)
	}
	if rel.GetUID() == nil {
		rel.SetUID(g.nextUID())
	}
	if rel.Source.Namespace == "" || rel.Source.Type == "" || rel.Source.ID == nil {
		rel.Source = partyRef(g.nextUID())
	}
	if rel.Target.Namespace == "" || rel.Target.Type == "" || rel.Target.ID == nil {
		rel.Target = partyRef(g.nextUID())
	}
}

// fillPerformer gives a ROLE the performer the RM requires, a reference
// to the actor that plays the role, with partyRef's namespace and type:
// the generator cannot build the abstract OBJECT_ID a PARTY_REF needs.
// Only the parts the template left empty are filled. The id is a fixed
// one, where a PARTY_RELATIONSHIP's source and target ids are drawn from
// Options.UIDSource.
func fillPerformer(ref *rm.PartyRef) {
	def := partyRef(&rm.HierObjectID{Value: "00000000-0000-0000-0000-000000000001"})
	if ref.ID == nil || rm.IsTypedNil(ref.ID) {
		ref.ID = def.ID
	}
	if ref.Namespace == "" {
		ref.Namespace = def.Namespace
	}
	if ref.Type == "" {
		ref.Type = def.Type
	}
}

func partyRef(id *rm.HierObjectID) rm.PartyRef {
	return rm.PartyRef{
		ID:        id,
		Namespace: "local",
		Type:      defaultPartyRefType,
	}
}

// defaultPartyRefType is the type of the references the generator builds
// itself: a PARTY_RELATIONSHIP's source and target, a ROLE's performer.
const defaultPartyRefType = "PERSON"

// partyRefTypes are the class names BASE PARTY_REF Type_validity admits as
// a reference's type, in the invariant's order, which starts with the
// default type, so a pin that accepts the default gets it.
var partyRefTypes = []string{defaultPartyRefType, "ORGANISATION", "GROUP", "AGENT", "ROLE", "PARTY", "ACTOR"}

// errNoPartyRefType reports a C_STRING on a PARTY_REF's type that accepts
// none of the class names PARTY_REF Type_validity admits.
var errNoPartyRefType = errors.New("the C_STRING accepts no class name PARTY_REF Type_validity admits")

// partyRefType returns the type the generator writes on a PARTY_REF whose
// OPT constrains it with cs, so BASE PARTY_REF Type_validity holds, which
// neither validator evaluates. It is chosen, the string the C_STRING path
// picked, when that is an admitted class name, so a list pin keeps its
// example value and RandomFill its draw; else the first admitted class
// name cs accepts, which is the default type when cs accepts it. chosen is
// "" when the C_STRING path found no string. It returns errNoPartyRefType
// when cs accepts no admitted class name.
func partyRefType(cs constraints.CString, chosen string) (string, error) {
	if slices.Contains(partyRefTypes, chosen) {
		return chosen, nil
	}
	for _, class := range partyRefTypes {
		if len(cs.Validate(class)) == 0 {
			return class, nil
		}
	}
	return "", errNoPartyRefType
}

// fillCurrentState gives an ISM_TRANSITION whose current state has no
// code (noCode) the first code the OPT gives under current_state, or else
// the code 524 (initial) of the openEHR instruction states group where the
// OPT's constraint on current_state admits it (codeAdmitted). opt is the
// OPT node of iv.
func fillCurrentState(opt *tcimpl.CompiledNode, iv *rm.IsmTransition) {
	if !noCode(iv.CurrentState.DefiningCode.CodeString) {
		return
	}
	ref, ok := firstCodedExample(opt, "current_state")
	if !ok {
		ref = constraints.CodedTermRef{Terminology: terminology.ID, CodeString: "524"}
		initial := rm.CodePhrase{CodeString: ref.CodeString, TerminologyID: rm.TerminologyID{Value: ref.Terminology}}
		if !codeAdmitted(opt, "current_state", initial) {
			return
		}
	}
	rubric := ref.CodeString
	if text, found := terminology.InstructionStates.Rubric(ref.CodeString); found {
		rubric = text
	}
	iv.CurrentState = rm.DVCodedText{
		Value: rubric,
		DefiningCode: rm.CodePhrase{
			CodeString:    ref.CodeString,
			TerminologyID: rm.TerminologyID{Value: ref.Terminology},
		},
	}
}

func firstCodedExample(opt *tcimpl.CompiledNode, attrName string) (constraints.CodedTermRef, bool) {
	if opt == nil {
		return constraints.CodedTermRef{}, false
	}
	attr := opt.Attribute(attrName)
	if attr == nil {
		return constraints.CodedTermRef{}, false
	}
	var found constraints.CodedTermRef
	var ok bool
	var walk func(*tcimpl.CompiledNode)
	walk = func(n *tcimpl.CompiledNode) {
		if n == nil || ok {
			return
		}
		if pc := n.PrimitiveConstraint(); pc != nil {
			if phrase, is := pc.(constraints.CodePhrase); is {
				if ref, isRef := phrase.ExampleValue().(constraints.CodedTermRef); isRef && !noCode(ref.CodeString) {
					found = ref
					ok = true
					return
				}
			}
		}
		for _, a := range n.Attributes() {
			for _, child := range a.Children() {
				walk(child)
			}
		}
	}
	for _, child := range attr.Children() {
		walk(child)
	}
	return found, ok
}

// applyStringLeaf writes a C_STRING leaf onto the String attribute attr
// of rmValue, or onto its main string attribute when attr is "". The
// value is a list member or a pattern match that the constraint accepts;
// on a PARTY_REF's type it is also a class name PARTY_REF Type_validity
// admits (see partyRefType). When there is none, it writes nothing and
// returns an error wrapping ErrConstraintUnsatisfiable. An attribute the
// generator has no field for is left alone, like any other unknown
// primitive target.
func applyStringLeaf(leaf *tcimpl.CompiledNode, rmValue any, attr string, cs constraints.CString, ex any) error {
	if attr == "" {
		attr = mainStringAttr(rmValue)
	}
	_, set, ok := stringField(rmValue, attr)
	if !ok {
		return nil
	}
	s, err := stringForConstraint(cs, ex)
	if _, isRef := rmValue.(*rm.PartyRef); isRef && attr == "type" {
		s, err = partyRefType(cs, s)
	}
	if err != nil {
		return fmt.Errorf("%w: %s.%s at %s: %w", ErrConstraintUnsatisfiable, rmTypeOf(rmValue), attr, leafPath(leaf), err)
	}
	set(s)
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
