// Package instance synthesises an RM object graph from a compiled
// operational template. It is the inverse of openehr/validation:
// where validation walks an OPT and an existing RM tree in lockstep
// emitting issues, this package walks an OPT and constructs the RM
// tree, valuing each primitive leaf from the template's primitive
// constraint the way [Options.ValueFill] says.
//
// The public entry point is [Generate]:
//
//	name := "Test"
//	out, err := instance.Generate(ctx, compiled, instance.Options{
//	    Policy:    instance.Large,
//	    Territory: "NL",
//	    Composer:  &rm.PartyIdentified{Name: &name},
//	})
//
// Typed accessors ([AsComposition], [AsObservation], …) cast the
// returned `any` into a concrete RM root for downstream code that
// knows the template's root type.
//
// # Policies
//
// Under every policy the generator skips an attribute the template
// prohibits (an existence of 0..0), unless an RM rule needs it, such as
// an attribute the BMM marks mandatory, which it then writes as if the
// template allowed it; and it never visits one the RM computes rather
// than stores (offset on an event, is_integral on DV_QUANTITY and
// DV_PROPORTION), nor a locatable's uid, which the identity rule
// decides. Under every policy an RM rule can need more than the policy
// visits, such as an ELEMENT's value or null flavour, which the generator
// then builds as a visit does, from the attribute's first allowed
// template child where it has one; and a visited multi-valued attribute
// is topped up to its lower bound.
//
// The three policies follow the levels of the ITS-REST template
// example's detail_level:
//
//   - Medium: the required attributes (the BMM marks them mandatory, or
//     their existence or cardinality lower bound is at least 1), and those
//     with an allowed template child (one whose occurrences upper bound is
//     not 0). A visited multi-valued attribute gets its members as under
//     Large, except that an optional archetype after the first optional
//     archetype of the attribute that holds more than content the
//     template does not describe (below) gets none, whatever its
//     archetype. So Medium builds at most one optional archetype under an
//     attribute, the medium level. It is the zero value, and so the
//     default.
//   - Required: the required attributes only, not one merely because it
//     has an allowed template child, except a composition's context,
//     which it visits as Medium does, so the context every policy writes
//     is built under the template's constraints. A visited multi-valued
//     attribute gets each allowed child that is not a slot as many times
//     as its occurrences lower bound says, so an optional child gets none.
//     It builds the data points the template makes mandatory and a
//     composition's context, the required level. An RM rule can still add
//     a node, such as the null flavour of an element whose value the
//     template leaves optional, so its tree is not always smaller than
//     Medium's.
//   - Large: every attribute the visit rule allows, the optional ones the
//     template names included. A visited multi-valued attribute gets at
//     least one member of each allowed child that is not a slot, except
//     an optional child whose identity a required or earlier sibling
//     carries. So Large builds a member of every optional archetype the
//     template names, except one that holds only content the template does
//     not describe (below). It stops short of the complete level, every
//     data point the template allows, which no level of its own builds yet.
//
// Under Large and Medium the generator builds an optional node only where
// it holds more than content the template does not describe: a slot fill,
// or a LOCATABLE built from the BMM type of an attribute the template gives
// no allowed child, such as the placeholder element of a list the template
// leaves empty where the RM or the template's cardinality asks for a member.
// An optional child gets no member of its own when
// it is a slot, or when an attribute of its own that is required can only
// be given such members: the attribute has no allowed child and its member
// would be a LOCATABLE, every allowed child of it is a slot, or it is
// single-valued and the child that builds its member is a slot. An
// attribute that is not required is not visited when every member it
// would get is such content or such a child. A composition's context is
// the exception: every policy writes one, so the walk visits it whatever
// its members need. The first optional archetype Medium keeps is the first
// that needs none. The check reads a node's own attributes only: below a
// node the template or the RM requires, the walk builds what it always
// builds, placeholders and slot fills included where something asks for a
// member, so the described content around them is kept.
//
// Under every policy, an attribute left with fewer members than it needs
// is topped up with members of its first allowed child that needs no
// such content and whose identity no earlier sibling carries, else of its
// first allowed child that is not a slot, else with fills of its first
// slot. Where only the attribute's existence asks for a member, because
// neither the RM, nor its cardinality, nor the occurrences of a child it
// allows does, the top-up takes only the first of these, and the attribute
// is left with no member where it has no such child: a list with no member
// meets its existence. A child's identity is its archetype id when it is
// an archetype root, and its node id otherwise, and a sibling that is a
// slot carries none.
//
// [Minimal] is another name for Required, and [Complete] for Large until
// the complete level lands. [ParsePolicy] reads a policy from the names
// required, minimal, medium, large and complete, in any case. Generate
// refuses any other Policy value with [ErrUnsupportedPolicy], before it
// builds anything.
//
// Under every policy, every primitive leaf the walk reaches is valued as
// [Options.ValueFill] says: the constraint's example value under
// [ExampleFill], an in-constraint draw under [RandomFill]. Where the
// template gives an RM attribute no value, the generator writes an
// RM-valid default, unless the template's own constraint on that
// attribute rejects it. A DV_PROPORTION is then settled to the rules of
// its kind, inside the template's constraints on its parts: a unitary
// proportion over 1, a percent over 100, a fraction or an integer
// fraction over whole numbers, any proportion of precision 0 over whole
// numbers, and every kind over a denominator that is not 0. Where the
// drawn type cannot meet its rule there, another type the template allows
// is taken, and where none can, the value stays as the walk wrote it. A
// coded text whose code is in the openEHR terminology, on an attribute the
// RM ties to a group of that terminology (a COMPOSITION's category, an
// INTERVAL_EVENT's math function, a PARTY_RELATED's relationship, …),
// takes as its text that code's rubric in the group. A coded text whose
// code is in any other terminology takes the text its own archetype, the
// nearest enclosing archetype root, gives that code, in
// [Options.Language]: under its code string for a local code, and under
// <terminology id>::<code string>, such as SNOMED-CT::248152002, for a code
// of an external terminology. Either way the walk's text stays where the
// template's constraint on the text rejects the new one, and where the
// group has no rubric for the code or its own archetype does not define
// it, even where an enclosing archetype does. A DV_MULTIMEDIA carries inline
// data, the seven bytes of the text "example" under both value fills,
// with that byte count as its size, unless the template prohibits data or
// its constraint on size rejects that count.
//
// # Trust model
//
// The OPT walk is authoritative. RM children are created from the
// openehr/rm/typereg registry (typereg.Default.Lookup), directly or
// through [internal/templateinstance/rmwrite.NewRM], attached via
// rmwrite EnsureSingle / AppendMultiple, and decorated
// with LOCATABLE bookkeeping (archetype_node_id, name, uid,
// archetype_details) here in the instance package. rmwrite stays
// focused on the attribute setter contract, the inverse of the
// generated attribute access the validators read through.
//
// # Dependencies
//
// The exported Generate signature takes the public compiled template
// openehr/templatecompile.Compiled; this package also imports
// openehr/rm, openehr/rm/typereg, openehr/rm/rminfo, openehr/template,
// openehr/template/constraints, openehr/terminology,
// internal/templatecompile (engine node types),
// internal/templateinstance/rmwrite, internal/bmmtype and
// internal/rmroots. It does not import openehr/serialize,
// openehr/client, transport, auth, openehr/composition (which uses
// this package, not the reverse), or openehr/validation.
package instance
