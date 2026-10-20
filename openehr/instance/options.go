package instance

import (
	"errors"
	"fmt"
	mrand "math/rand/v2"
	"strings"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// Policy controls which attributes the generator visits. Under every
// policy it skips an attribute the OPT prohibits (an existence of 0..0),
// unless an RM rule needs it, such as an attribute the BMM marks
// mandatory, and never visits one the RM computes rather than stores
// (offset on POINT_EVENT and INTERVAL_EVENT, is_integral on DV_QUANTITY
// and DV_PROPORTION) or a locatable's uid, which the identity rule
// decides. Under every policy an RM rule can need more than the policy
// visits, such as an ELEMENT's value or null_flavour, and a visited
// multi-valued attribute is topped up to its lower bound. [ValueFill],
// not the policy, decides how a primitive leaf is valued. Medium,
// Required and Large are the only levels, and Medium, the zero value, is
// the default; Minimal and Complete are other names for Required and
// Large. [Generate] refuses any other value with [ErrUnsupportedPolicy].
// [ParsePolicy] reads a policy from its name.
type Policy int

const (
	// Medium builds at most one optional archetype under an attribute,
	// what the ITS-REST template example calls detail_level=medium. It
	// visits the required attributes, and those with an allowed OPT
	// child: an attribute the BMM marks mandatory, one whose existence or
	// cardinality lower bound is bounded and at least 1, and one with an
	// OPT child whose occurrences upper bound is not 0. A visited
	// multi-valued attribute gets its members as under Large, except that
	// an optional archetype root after the first optional archetype root
	// of the attribute, in OPT order, that holds more than content the
	// template does not describe (see the package documentation) gets
	// none of its own, whatever its archetype. Medium is the zero value,
	// so Options with no Policy builds it.
	Medium Policy = iota
	// Required builds only the data points the template makes mandatory
	// and a composition's context, what the ITS-REST template example
	// calls detail_level=required. It visits only the required
	// attributes: an attribute the BMM marks mandatory, or one whose
	// existence or cardinality lower bound is bounded and at least 1; it
	// does not visit an attribute merely because it has an allowed OPT
	// child, except a COMPOSITION's context, which it visits as Medium
	// does, so the context every policy writes is built under the OPT's
	// constraints. A visited multi-valued attribute gets
	// occurrences.lower members of each allowed OPT child that is not a
	// slot, so an optional child gets none of its own. The top-up to the
	// attribute's lower bound and the RM rules, such as an ELEMENT's
	// value or null_flavour, still apply. An RM rule can add a node, such
	// as the null_flavour of an ELEMENT whose value the template leaves
	// optional, so its tree is not always smaller than Medium's.
	Required
	// Large builds a member of every optional archetype the template
	// names that holds more than content the template does not describe
	// (see the package documentation). It visits every attribute the
	// visit rule allows: every attribute the compiled node carries, the
	// optional ones the OPT names included. A visited multi-valued
	// attribute gets at least one member of each allowed OPT child that
	// is not a slot, except an optional child that holds only such
	// content, or whose identity a required sibling, or an earlier
	// optional one, carries; optional archetype roots of different
	// archetypes each get theirs. It stops short of the ITS-REST template
	// example's detail_level=complete (every data point the template
	// allows), which no level of its own builds yet.
	Large

	// Minimal is another name for Required.
	Minimal = Required
	// Complete is another name for Large, until a level of its own
	// builds the ITS-REST template example's detail_level=complete.
	Complete = Large
)

// String returns "medium" / "required" / "large" / "unknown" for
// diagnostics. Minimal and Complete print the name of the level they
// stand for.
func (p Policy) String() string {
	switch p {
	case Medium:
		return "medium"
	case Required:
		return "required"
	case Large:
		return "large"
	}
	return "unknown"
}

// ParsePolicy returns the Policy that name names: "required", "minimal",
// "medium", "large" or "complete", in any mix of upper and lower case.
// "minimal" gives Required and "complete" gives Large. Any other text,
// the empty string and a name with a space around it included, returns
// an error wrapping [ErrUnsupportedPolicy] that quotes the text.
func ParsePolicy(name string) (Policy, error) {
	for _, n := range policyNames {
		if strings.EqualFold(name, n.name) {
			return n.policy, nil
		}
	}
	return 0, fmt.Errorf("%w %q", ErrUnsupportedPolicy, name)
}

// policyNames are the names ParsePolicy reads, with the policy each names.
var policyNames = [...]struct {
	name   string
	policy Policy
}{
	{name: "required", policy: Required},
	{name: "minimal", policy: Minimal},
	{name: "medium", policy: Medium},
	{name: "large", policy: Large},
	{name: "complete", policy: Complete},
}

// ValueFill controls how primitive leaves are valued, orthogonally to
// [Policy] (which controls which nodes are materialised).
type ValueFill int

const (
	// ExampleFill is the default: each primitive leaf gets its
	// PrimitiveConstraint.ExampleValue, a single representative value
	// that is byte-identical across calls for one OPT.
	ExampleFill ValueFill = iota
	// RandomFill draws each leaf from within its constraint (in-range
	// magnitudes, value-set-member codes, enumeration entries), valid by
	// construction and varying between calls. Use [Options.ValueSource]
	// to make a run reproducible.
	RandomFill
)

// Any ValueFill other than RandomFill behaves as ExampleFill (the
// RandomFill check in Generate is exact), so an out-of-range value
// degrades to the deterministic example fill rather than erroring.

// String returns "example" / "random" / "unknown" for diagnostics.
func (f ValueFill) String() string {
	switch f {
	case ExampleFill:
		return "example"
	case RandomFill:
		return "random"
	}
	return "unknown"
}

// Options configures one [Generate] invocation. Zero value is valid
// for non-COMPOSITION roots: the Medium policy, the compiled template's
// own language (English when it names none), time.Now().UTC() clock.
type Options struct {
	// Policy is Medium, Required or Large, or Minimal or Complete, their
	// other names. The zero value is Medium. Generate refuses any other
	// value with ErrUnsupportedPolicy.
	Policy Policy

	// Language is the ISO 639-1 code of the COMPOSITION's and every
	// ENTRY's language default. Empty is read as the compiled template's
	// Language(), then as "en".
	Language string

	// Territory is the ISO 3166-1 alpha-2 code of a COMPOSITION's
	// territory default, required for a COMPOSITION root: empty there
	// returns ErrTerritoryRequired. Any other root ignores it.
	Territory string

	// Composer is the COMPOSITION's composer, required for a COMPOSITION
	// root: nil there returns ErrComposerRequired. It is written as given,
	// whatever the OPT constrains on composer. Any other root ignores it.
	Composer rm.PartyProxy

	// Now is the clock for unconstrained date-times: every DV_DATE_TIME
	// whose OPT node carries no primitive constraint and gets no OPT
	// value, such as HISTORY.origin, EVENT.time, ACTION.time and
	// EVENT_CONTEXT.start_time. Zero is read as the current time. Generate
	// writes every date-time it takes from Now in UTC, whatever zone Now
	// carries; tests pin it for determinism.
	Now time.Time

	// UIDSource supplies the uids the generator writes: the uid of every
	// ENTRY, GENERIC_ENTRY, PARTY and PARTY_RELATIONSHIP, of any other
	// locatable whose OPT requires its uid, and the id of a
	// PARTY_RELATIONSHIP's source or target that carries none. A
	// COMPOSITION gets a uid only where its OPT requires one, so the
	// server that stores it gives it its version id. Nil falls back to a
	// random RFC 9562 v4 UUID (stdlib uuid.NewV4). Tests pin a counter or
	// named-seed source for deterministic uids.
	UIDSource func() *rm.HierObjectID

	// ValueFill selects how primitive leaves are valued. Zero value =
	// ExampleFill (the constraint's representative example value).
	// RandomFill draws in-constraint values that vary between calls.
	ValueFill ValueFill

	// ValueSource seeds the in-constraint sampler used when ValueFill is
	// RandomFill; ignored otherwise. A fixed source (e.g.
	// rand.NewPCG(seed, seed)) makes the leaf values byte-reproducible;
	// nil draws from the package-global, auto-seeded generator so
	// successive calls differ. Mirrors the UIDSource seam.
	//
	// A math/rand/v2.Source is not safe for concurrent use: do not share
	// one Source across concurrent Generate calls. Give each goroutine its
	// own source (or leave it nil to use the concurrency-safe global).
	ValueSource mrand.Source
}

// ErrTerritoryRequired signals that a COMPOSITION-root template
// invocation omitted the Territory option.
var ErrTerritoryRequired = errors.New("instance.Generate: Territory is required for COMPOSITION roots")

// ErrComposerRequired signals that a COMPOSITION-root template
// invocation omitted the Composer option.
var ErrComposerRequired = errors.New("instance.Generate: Composer is required for COMPOSITION roots")

// ErrUnsupportedPolicy signals a policy the package does not define.
// Generate returns it, naming the value, for a Policy option that is none
// of Medium, Required and Large, before it builds anything, and
// ParsePolicy returns it, quoting the text, for a name it does not read.
var ErrUnsupportedPolicy = errors.New("instance: unsupported policy")

// ErrTypeMismatch is returned by AsComposition / AsObservation /
// etc. when the produced root is not the requested concrete type.
var ErrTypeMismatch = errors.New("instance: root type does not match requested accessor")

// ErrNilCompiled signals a nil compiled-template argument.
var ErrNilCompiled = errors.New("instance.Generate: nil compiled template")

// ErrSlotFillUnsupported signals that a required ARCHETYPE_SLOT fill
// could not be synthesized safely from the parsed slot assertions.
var ErrSlotFillUnsupported = errors.New("instance.Generate: required slot fill cannot be synthesized")

// ErrConstraintUnsatisfiable signals that no value the generator can
// derive satisfies a primitive leaf's constraint, for example a C_STRING
// whose pattern and list admit no string. The error names the RM type,
// the attribute and the OPT path of the leaf, and Generate returns no
// root. It is never wrapped together with [ErrSlotFillUnsupported], which
// concerns slots only, nor with [ErrArchetypeIDMissing], which concerns
// archetype roots only.
var ErrConstraintUnsatisfiable = errors.New("instance.Generate: no value satisfies the primitive constraint")

// ErrArchetypeIDMissing signals that the template asks for an object of an
// RM class that is always an archetype root (an ENTRY, a COMPOSITION, a
// PARTY, EHR_STATUS or EHR_ACCESS) but names no archetype for it. Such an
// object could not carry valid archetype_details, so the RM floor would
// reject it, and the generator does not invent an archetype id to avoid
// that. A slot is not such an object: the slot-fill rule stamps it with an
// archetype id, or refuses it with [ErrSlotFillUnsupported]. An archetype
// id the template carries under an xsi:type the parser does not recognise
// is dropped by lenient parsing, so this error covers that case too. The
// error names the RM type and the OPT path, and Generate returns no root.
// It wraps neither [ErrSlotFillUnsupported] nor
// [ErrConstraintUnsatisfiable], and ErrConstraintUnsatisfiable never wraps
// it.
var ErrArchetypeIDMissing = errors.New("instance.Generate: the template names no archetype for an archetype root")
