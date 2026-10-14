package validation

import (
	"fmt"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// Severity is the typed severity attached to every [Issue]. [ValidateComposition]
// emits only [Error]. [ValidateAQL] may also emit [Warning] for lint
// advisories (e.g. aql_from_archetype, aql_select_star; the lint codes pass
// through verbatim, so the full set is the openehr/aql/lint advisory catalogue).
type Severity int

const (
	// Error means the composition violates a constraint.
	// Callers should treat any Error-severity issue as a hard fail.
	Error Severity = iota

	// Warning is an advisory issue that does not make a [Result]
	// not-OK. [ValidateAQL] emits it for lint advisories (e.g.
	// aql_from_archetype, aql_select_star; the full set is the
	// openehr/aql/lint catalogue); [ValidateComposition] emits only [Error].
	Warning
)

// String returns "error" / "warning"; out-of-range values render as
// `severity(N)` with the numeric form, so callers logging issues see
// both the name and the wire value.
func (s Severity) String() string {
	switch s {
	case Error:
		return "error"
	case Warning:
		return "warning"
	}
	return fmt.Sprintf("severity(%d)", int(s))
}

// Layer names the body of rules an [Issue] comes from: the operational
// template, or the openEHR Reference Model. The zero value, [LayerNone], is
// the layer of an issue that judges the call or the walk rather than the
// instance.
type Layer uint8

const (
	// LayerNone marks an issue that judges the call or the walk rather than
	// the instance: a guard, such as nil_composition or invalid_shape, a
	// walk condition, such as max_depth or cyclic_value, and every issue
	// from [ValidateAQL] and [ValidateAQLWithTypeRelation]. Its Rule is
	// empty.
	LayerNone Layer = iota

	// LayerTemplate marks an issue that a clause of the operational
	// template produces.
	LayerTemplate

	// LayerRM marks an issue that a rule of the openEHR Reference Model
	// produces, an attribute the Reference Model makes mandatory and the
	// instance lacks included.
	LayerRM
)

// String returns "" for [LayerNone], "template" for [LayerTemplate] and
// "rm" for [LayerRM]; out-of-range values render as `layer(N)` with the
// numeric form, so callers logging issues see both the name and the value.
func (l Layer) String() string {
	switch l {
	case LayerNone:
		return ""
	case LayerTemplate:
		return "template"
	case LayerRM:
		return "rm"
	}
	return fmt.Sprintf("layer(%d)", uint8(l))
}

// Issue is one failing clause from a validator. Validators emit one
// Issue per failure (collect-all, not fail-fast). The zero value is
// not useful; the validators in this package build issues themselves.
//
// The doc of each field ends with its class, value-free or value-bearing.
// The class of Path and Detail depends on the entry point that returned
// the issue.
//
// The instance validators are [Validator.Validate], [ValidateComposition],
// [Validate], [ValidateDemographic], [ValidateFolder] and
// [ValidateEHRStatus], and the template-less floor [ValidateRM],
// [ValidateRMFolder], [ValidateRMEHRStatus], [ValidateRMEHRAccess],
// [ValidateRMDemographic] and [ValidateRMEHRStatusBytes]. On their issues
// Path, Code, Layer, Rule, Detail and Severity never contain a value taken
// from the instance: not the content of a data value (a node's own name
// included), a code, units, a precision, an interval bound, nor the text of
// an error raised while decoding it. They may name RM types and attributes,
// archetype and node ids, the names the template gives its nodes, and the
// template's own constraint, so such an issue can be logged or returned to
// a client as it is. The value the failed check read is kept only in Value.
//
// On an issue from [ValidateAQL] or [ValidateAQLWithTypeRelation], Code,
// Layer, Rule and Severity never contain text from the query, but Detail
// and Path carry the lint text and may quote the query, its literals
// included. Layer is [LayerNone], and Rule and Value are empty.
//
// Two issues compare equal under == when their paths, codes, layers,
// rules, details and severities are equal and their held values are equal
// under ==, and the comparison never panics. A value that == cannot
// compare, such as a slice or a map, is held by reference, so it equals
// only copies of the same [constraints.Redacted].
type Issue struct {
	// Path is the openEHR path of the offending node, from the validated
	// root: RM attribute names joined by "/", with "/" alone for the root.
	// Each item of a multi-valued attribute carries a predicate in square
	// brackets, and an attribute that holds a single value carries none.
	// The predicate is the item's archetype node id (an archetype id on an
	// archetype root) when no other item of that attribute carries it. When
	// several items share the id, it is the id followed by the name the
	// template gives the item's node, as in [at0004, 'Systolic'], provided
	// that name tells the item apart; otherwise it is the item's position,
	// counted from 1. An item with no id is always written by its position.
	// A node the template requires and the instance lacks is written with
	// the template's own predicate for it, since there is no item to count.
	// The template-driven validators and [ValidateRM] write one node's path
	// the same way, although [ValidateRM] alone has no template names, so it
	// writes a position where a template-driven validator may write a name.
	//
	// A path with no position in it is also a valid AQL path. AQL has no
	// positional predicate, so a path with one, such as /items[2], is an
	// openEHR path but not an AQL path.
	//
	// Empty for global issues that do not localise to a specific node,
	// such as nil_composition or nil_template for a nil argument. On an
	// issue from [ValidateAQL] or [ValidateAQLWithTypeRelation] it is the
	// lint path.
	//
	// Value-free on issues from the instance validators: never contains a
	// value from the instance. Value-bearing on issues from [ValidateAQL]
	// and [ValidateAQLWithTypeRelation]: may quote the query, literals
	// included.
	Path string

	// Code is a stable programmatic identifier (e.g. "required",
	// "cardinality", "rm_type_mismatch", "slot_fill",
	// "primitive_out_of_range"). Consumers should dispatch on Code
	// rather than parse Detail.
	//
	// Value-free: never carries a value from the instance or text from
	// the query.
	Code string

	// Layer says which rules the issue comes from: [LayerTemplate] for a
	// clause of the operational template, [LayerRM] for a rule of the
	// openEHR Reference Model. An absent attribute is on the template layer
	// when the template's own existence constraint requires it, and on the
	// RM layer when only the Reference Model makes it mandatory. [LayerNone]
	// marks an issue that judges the call or the walk rather than the
	// instance: a guard (nil_composition, nil_template, nil_root and the
	// other nil_ codes, rm_type_unknown, and invalid_shape on input that
	// does not decode), a walk condition (max_depth, max_nodes,
	// cyclic_value), and every issue from [ValidateAQL] and
	// [ValidateAQLWithTypeRelation].
	//
	// Value-free: names a layer, never a value from the instance or text
	// from the query.
	Layer Layer

	// Rule names the clause the issue evaluates, so that issues sharing a
	// Code, such as the rm_invariant issues, are told apart without reading
	// Detail. It is empty exactly when Layer is [LayerNone].
	//
	// On [LayerRM], a rule the openEHR Reference Model or Base types name as
	// a class invariant is written CLASS.Invariant: the class that declares
	// the invariant and the invariant's name, both spelt as the published
	// model spells them, as in "DV_INTERVAL.Limits_consistent",
	// "ELEMENT.Inv_null_flavour_indicated", "ENTRY.Language_valid" on any
	// entry, or "Interval.Lower_included_valid" on any interval. A rule the
	// model states in an attribute's definition rather than as a named
	// invariant is a lower-case word: "mandatory" for a mandatory attribute
	// the instance lacks, or a mandatory string left empty; "cardinality"
	// for a mandatory list that is present and empty; "precision_range" for
	// a precision below -1.
	//
	// On [LayerTemplate], it is "existence" (an attribute the template
	// requires and the instance lacks, or one it prohibits and the instance
	// carries), "cardinality" (the number of items an attribute holds),
	// "occurrences" (the number of items bound to one node of the
	// template), "slot" (an item that fits no node or slot), "type" (the RM
	// type of a value, or the alternatives it may take) or "identity" (an
	// archetype id or a node id); at a constrained leaf, it is the code of
	// the [constraints.Violation] the leaf's constraint reported, such as
	// "out_of_range" or "pattern_mismatch".
	//
	// Value-free: names a rule, never a value from the instance or text
	// from the query.
	Rule string

	// Detail is a human-readable message describing the failure; it is
	// not localised. From an instance validator it names the check that
	// failed; from [ValidateAQL] or [ValidateAQLWithTypeRelation] it is
	// the lint text.
	//
	// Value-free on issues from the instance validators: never contains a
	// value from the instance, such as the value the check read.
	// Value-bearing on issues from [ValidateAQL] and
	// [ValidateAQLWithTypeRelation]: may quote the query, literals
	// included.
	Detail string

	// Severity classifies the issue. [ValidateComposition] emits [Error] only;
	// [ValidateAQL] may emit [Warning] advisories that do not flip [Result.OK].
	//
	// Value-free: never carries a value from the instance or text from
	// the query.
	Severity Severity

	// Value holds the value from the instance that the failed check
	// read. Value.Reveal() returns it with this Go type:
	//
	//   - on a primitive_* issue, the value of the
	//     [constraints.Violation] it reports, with the type that
	//     violation holds;
	//   - on an rm_invariant issue for a precision below -1, the
	//     precision, an rm.Integer;
	//   - on an rm_invariant issue for a precision of 0 with a
	//     fractional operand, the numerator and the denominator, a
	//     [2]float64;
	//   - on an rm_invariant issue for interval bounds out of order, the
	//     two compared magnitudes, lower first, a [2]float64;
	//   - on an rm_invariant issue for a date, time or duration that is
	//     not valid ISO 8601, its value, a string;
	//   - on a term_mapping_match issue, the match, an rm.Character;
	//   - on an invalid_shape issue from a failed decode, the decode
	//     error, an error whose text may quote the input.
	//
	// Each call that fails to decode creates a new error value, so two
	// calls over the same bytes give invalid_shape issues that are not
	// equal under ==.
	//
	// Value is empty on every other issue (something absent or prohibited,
	// a count, a type or identity mismatch, a guard).
	//
	// Value-bearing: holds a value from the instance. It prints as
	// "[redacted]", as [constraints.Redacted] describes, and is left out
	// of JSON and gob output. Value.Reveal() returns the value itself, to
	// be called only where showing it is safe. Value is empty on every
	// issue from [ValidateAQL] and [ValidateAQLWithTypeRelation].
	Value constraints.Redacted `json:"-"`
}

// Err returns the typed sentinel matching this Issue's Code, or
// nil when no sentinel maps. The mapping is:
//
//   - "required"                                          → [ErrRequired]
//   - "prohibited"                                        → [ErrProhibited]
//   - "cardinality"                                       → [ErrCardinality]
//   - "rm_type_mismatch" / "alternative_mismatch" /
//     "archetype_id_mismatch" / "node_id_mismatch"        → [ErrTypeMismatch]
//   - "primitive_*" (any primitive_-prefixed code)        → [ErrPrimitive]
//   - "slot_fill"                                         → [ErrSlotFill]
//   - "aql_syntax" / "aql_empty"                          → [ErrAQLSyntax]
//
// Every other code returns nil, although most of them are errors. That
// covers the guards (`nil_composition`, `nil_template`, `nil_root`,
// `nil_party`, `nil_folder`, `nil_ehr_status`, `nil_ehr_access`,
// `rm_type_unknown` and `invalid_shape`), which report an argument the
// validator could not take; the walk conditions (`max_depth`, `max_nodes`
// and `cyclic_value`), which report where the walk stopped or turned back;
// and the codes of the Reference Model rules (`rm_invariant`,
// `mappings_valid`, `term_mapping_match`, `lower_included_valid`,
// `upper_included_valid`, `code_not_in_value_set`, `is_archetype_root` and
// `rm_version_valid`). The template-driven validators report those rule
// codes as well as [ValidateRM], since they include the Reference Model
// rules. A nil Err therefore does not mean the issue is harmless: read
// [Result.OK], or the issue's Severity, for the verdict, and [Issue.Rule]
// to tell the Reference Model rules apart. Callers wanting `errors.Is`
// dispatch go through this method:
//
//	for _, i := range r.Issues {
//	    if errors.Is(i.Err(), validation.ErrRequired) { ... }
//	}
func (i Issue) Err() error {
	switch i.Code {
	case "required":
		return ErrRequired
	case "prohibited":
		return ErrProhibited
	case "cardinality":
		return ErrCardinality
	case "rm_type_mismatch",
		"alternative_mismatch",
		"archetype_id_mismatch",
		"node_id_mismatch":
		return ErrTypeMismatch
	case "slot_fill":
		return ErrSlotFill
	case "aql_syntax", "aql_empty":
		// Codes minted by openehr/aql/lint (REQ-109) and carried verbatim
		// through ValidateAQL; keep this in sync with the lint code strings.
		return ErrAQLSyntax
	}
	if strings.HasPrefix(i.Code, "primitive_") {
		return ErrPrimitive
	}
	return nil
}

// Result aggregates every [Issue] from a single validator call.
// OK is the convenience boolean: true exactly when no Error-severity
// issue is present.
type Result struct {
	// OK is true when the validator found no [Error]-severity issue.
	// [Warning]-severity advisories (emitted by [ValidateAQL], e.g.
	// aql_from_archetype) do not make OK false. Treat OK as the
	// pass/fail result and inspect Severity for advisories. For
	// [ValidateComposition], which emits only Error issues, OK is
	// equivalent to len(r.Issues) == 0.
	OK bool

	// Issues is the full list of issues in a stable per-validator order
	// (composition walk order for [ValidateComposition]; lint layer order
	// for [ValidateAQL]). Empty when a validator found nothing; never nil
	// after a validator call (zero-length allocation is acceptable). May be
	// non-empty while OK is true when it holds only Warning-severity issues.
	Issues []Issue

	// Coverage says what the validator did not check, so a caller can tell
	// "no issue found" from "everything checked", and which Reference Model
	// invariants of the classes it visited it does not evaluate. Its zero
	// value, which a guard and [ValidateAQL] return, is complete.
	Coverage Coverage
}

// Coverage lists the parts of the instance a validator call did not check.
// OK only reports that no error was found in what was checked; Coverage says
// how much that was. Each path is written as [Issue.Path] describes and holds
// no value from the instance.
type Coverage struct {
	// Stopped lists the path of each place where a limit stopped the walk:
	// the path of the max_depth issue, the first place the walk reached its
	// depth limit (it walked nothing deeper, there or on any other branch),
	// and the path of the max_nodes issue, where it reached its visit limit
	// and ended. It holds at most one of each, in the order the walk met
	// them. The limits are those of the [Validator] that ran the call
	// ([Limits]).
	Stopped []string

	// External lists, in walk order, the path of each node the template
	// binds to a CONSTRAINT_REF: a constraint whose value set is bound
	// outside the template, which the validator cannot consult, so the code
	// at that path was not checked against it; the node's other attributes
	// were. A node bound to a C_CODE_PHRASE that names an external
	// terminology without listing codes is not listed: the validator checks
	// it for that terminology, and whether a code exists in an external
	// terminology is outside every check it makes.
	External []string

	// Unevaluated lists, sorted and once each, the Reference Model
	// invariants the validator does not evaluate on the classes the walk
	// visited, those their ancestors declare included, each written
	// CLASS.Invariant as [Issue.Rule] writes a rule, as in
	// "DV_AMOUNT.Accuracy_validity" on a DV_QUANTITY. An invariant is listed
	// whatever the instance holds: it is a rule the validator evaluates on
	// no instance. Most are rules it does not evaluate yet; others need a
	// record the instance does not carry, such as the versions stored for a
	// versioned object, hold by definition on the SDK's types, or are read
	// otherwise by a documented deviation of the ISO 8601 parse. The SDK
	// records the reason beside each. A call that visits no value, such as a
	// guard, lists none. [Complete] does not read it.
	Unevaluated []string
}

// Complete reports whether the call checked everything it reached: no limit
// stopped the walk, and no node was bound to a CONSTRAINT_REF, whose value
// set the validator cannot consult. It is true exactly when Stopped and
// External are both empty, so the zero Coverage is complete. It says nothing
// of Unevaluated, which lists rules the validator evaluates on no instance.
func (c Coverage) Complete() bool {
	return len(c.Stopped) == 0 && len(c.External) == 0
}

// resultFromIssues builds a [Result] from a slice of issues. Used by
// validators that accumulate into a local slice and return at the
// end of the walk. OK is true unless an Error-severity issue is
// present (Warnings do not flip it). The Issues field is never nil —
// callers can range over it without a nil guard, matching the doc on
// [Result.Issues].
func resultFromIssues(issues []Issue) Result {
	if issues == nil {
		issues = []Issue{}
	}
	ok := true
	for _, i := range issues {
		if i.Severity == Error {
			ok = false
			break
		}
	}
	return Result{
		OK:     ok,
		Issues: issues,
	}
}
