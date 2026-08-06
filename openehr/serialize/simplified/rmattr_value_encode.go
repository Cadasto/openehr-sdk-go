package simplified

// REQ-140 — the value-decoration underscore families, encode side: the inverse
// of rmattr_value.go.
//
// [valueRMAttrs] is called from [leafToFlat] once a leaf value has been written
// in **suffix** form, and only then: a value that rode `|raw` already carries
// its decorations inside the fragment, so writing the `_` keys too would spell
// one attribute twice.
//
// Every nested value goes back out through [emitLeafValue] — the same function
// that writes a Web Template leaf — so a bound, a `/meaning` and (from the
// `_mapping` family) a `/target` are emitted by their own datatype's rules,
// including the `|raw` carrier for one the suffix set cannot capture. That makes
// these emitters total: no representable RM value is refused here for want of a
// channel, which is what lets [capturedKeysDecorated] promise that a decorated
// value can leave the `|raw` path at all.

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// valueDecorationAttrs maps each canonical DataValue attribute the underscore
// grammar carries to the FLAT family that spells it. It is the source of the
// `|raw` boundary: [capturedKeysDecorated] widens a datatype's captured-key set
// by whichever of these the RM says that datatype declares, so a value whose
// only extras are listed here rides suffixes plus `_` keys where it used to ride
// one `|raw` fragment (REQ-140; deviations.md § |raw boundary).
//
// Every entry has a reader in [valueRMAttrs]; a widening with no reader would
// silently drop the attribute, which the final refusal there guards against.
var valueDecorationAttrs = map[string]string{
	"normal_range":           "_normal_range",
	"other_reference_ranges": "_other_reference_ranges",
	"mappings":               "_mapping",
	"charset":                "_charset",
	"language":               "_language",
	"encoding":               "_encoding",
	"thumbnail":              "_thumbnail",
	// `accuracy` widens only the three DV_TEMPORAL types: DV_QUANTITY, DV_COUNT,
	// DV_DURATION and DV_PROPORTION declare it as a Real their `|accuracy` suffix
	// already captures, so the widening is a no-op there (see
	// [decodeRMAttrTemporalAccuracy] for the two-spellings rule).
	"accuracy": "_accuracy",
}

// valueRMAttrs writes the underscore-carried decorations of a leaf value at its
// FLAT path. anchor is the suffix type the value was emitted as, which is also
// the type its interval bounds are spelled with.
//
// The final refusal is a guard on [capturedKeysDecorated], not a datatype gap: a
// type whose captured set was widened by a decoration but which no branch here
// reads would have its decoration silently dropped, so it fails loudly instead.
func valueRMAttrs(out map[string]any, base string, v any, anchor string) error {
	if !decoratedAnchor(anchor) {
		return nil // a datatype the grammar decorates with nothing
	}
	// The dispatch is on the value's **dynamic** type, which is the anchor itself
	// or one of REQ-053's two sanctioned substitutions — a value whose type differs
	// otherwise rode `|raw` and never reaches here — so each branch emits exactly
	// the decorations the RM declares on that type. The generic parameter of the
	// two DV_ORDERED intervals differs per datatype (the generated RM narrows it to
	// the anchor type for some and leaves it abstract for others), which is why
	// they cannot share one branch.
	switch {
	case isTyped[rm.DVQuantity](v):
		dv, _ := as[rm.DVQuantity](v)
		return orderedRMAttrs(out, base, anchor, dv.NormalRange, dv.OtherReferenceRanges)
	case isTyped[rm.DVCount](v):
		dv, _ := as[rm.DVCount](v)
		return orderedRMAttrs(out, base, anchor, dv.NormalRange, dv.OtherReferenceRanges)
	case isTyped[rm.DVProportion](v):
		dv, _ := as[rm.DVProportion](v)
		return orderedRMAttrs(out, base, anchor, dv.NormalRange, dv.OtherReferenceRanges)
	case isTyped[rm.DVOrdinal](v):
		dv, _ := as[rm.DVOrdinal](v)
		return orderedRMAttrs(out, base, anchor, dv.NormalRange, dv.OtherReferenceRanges)
	case isTyped[rm.DVDuration](v):
		dv, _ := as[rm.DVDuration](v)
		return orderedRMAttrs(out, base, anchor, dv.NormalRange, dv.OtherReferenceRanges)
	// DV_TEMPORAL adds the DV_DURATION-typed `accuracy`, which has no scalar suffix.
	case isTyped[rm.DVDateTime](v):
		dv, _ := as[rm.DVDateTime](v)
		return temporalRMAttrs(out, base, anchor, dv.NormalRange, dv.OtherReferenceRanges, dv.Accuracy)
	case isTyped[rm.DVDate](v):
		dv, _ := as[rm.DVDate](v)
		return temporalRMAttrs(out, base, anchor, dv.NormalRange, dv.OtherReferenceRanges, dv.Accuracy)
	case isTyped[rm.DVTime](v):
		dv, _ := as[rm.DVTime](v)
		return temporalRMAttrs(out, base, anchor, dv.NormalRange, dv.OtherReferenceRanges, dv.Accuracy)
	// DV_TEXT and its coded subtype: `mappings` plus the two CODE_PHRASE members.
	// Both cases are needed — [as] matches the exact type, and DV_CODED_TEXT
	// reaches the attributes through its embedded DV_TEXT.
	case isTyped[rm.DVCodedText](v):
		dv, _ := as[rm.DVCodedText](v)
		return textRMAttrs(out, base, dv.DVText)
	case isTyped[rm.DVText](v):
		dv, _ := as[rm.DVText](v)
		return textRMAttrs(out, base, dv)
	// DV_ENCAPSULATED: the same two CODE_PHRASE members, plus DV_MULTIMEDIA's
	// nested thumbnail.
	case isTyped[rm.DVMultimedia](v):
		dv, _ := as[rm.DVMultimedia](v)
		if err := codePhraseMembersRMAttr(out, base,
			codePhraseMember{"_charset", dv.Charset}, codePhraseMember{"_language", dv.Language}); err != nil {
			return err
		}
		if dv.Thumbnail == nil {
			return nil
		}
		_, err := emitLeafValue(out, base+"/_thumbnail", *dv.Thumbnail, "DV_MULTIMEDIA", false, false)
		return err
	case isTyped[rm.DVParsable](v):
		dv, _ := as[rm.DVParsable](v)
		return codePhraseMembersRMAttr(out, base,
			codePhraseMember{"_charset", dv.Charset}, codePhraseMember{"_language", dv.Language})
	}
	return fmt.Errorf("%w: %q holds a %T whose underscore-carried RM attributes this codec cannot read (REQ-140)",
		ErrUnsupportedDatatype, base, v)
}

// isTyped reports whether v holds a T in either the value or the pointer form —
// [as] without the extraction, so a switch can dispatch on the type and then bind
// the value in its own case.
func isTyped[T any](v any) bool {
	_, ok := as[T](v)
	return ok
}

// decoratedAnchor reports whether anchor's captured set was widened at all, i.e.
// whether [valueRMAttrs] owes the value any `_` key.
func decoratedAnchor(anchor string) bool {
	return len(capturedKeysDecorated[anchor]) > len(capturedKeys[anchor])
}

// temporalRMAttrs writes a DV_DATE / DV_DATE_TIME / DV_TIME's decorations: the
// two DV_ORDERED ones plus `_accuracy`.
//
// DV_TEMPORAL redefines `accuracy` as a DV_DURATION object where DV_AMOUNT
// declares a Real, so it has no scalar `|accuracy` suffix and rides its own family
// (the reference's spelling — `ehrbase_conformance_data_types_dv_date` and its two
// siblings). It goes out through the DV_DURATION leaf emitter, so a decorated
// accuracy rides `_accuracy|raw` rather than being refused.
func temporalRMAttrs[T rm.DVOrdered](out map[string]any, base, anchor string,
	normal *rm.DVInterval[T], others []rm.ReferenceRange[T], accuracy *rm.DVDuration,
) error {
	if err := orderedRMAttrs(out, base, anchor, normal, others); err != nil {
		return err
	}
	if accuracy == nil {
		return nil
	}
	_, err := emitLeafValue(out, base+"/_accuracy", *accuracy, "DV_DURATION", false, false)
	return err
}

// textRMAttrs writes a DV_TEXT's (and, through the embedded struct, a
// DV_CODED_TEXT's) decorations: `_mapping:N` plus the two CODE_PHRASE members the
// corpus spells `_language` and `_encoding`.
func textRMAttrs(out map[string]any, base string, dv rm.DVText) error {
	if err := mappingsRMAttr(out, base, dv.Mappings); err != nil {
		return err
	}
	return codePhraseMembersRMAttr(out, base,
		codePhraseMember{"_language", dv.Language}, codePhraseMember{"_encoding", dv.Encoding})
}

// codePhraseMember pairs an underscore family with the CODE_PHRASE it spells.
type codePhraseMember struct {
	family string
	cp     *rm.CodePhrase
}

// codePhraseMembersRMAttr writes the CODE_PHRASE-valued members a value carries —
// `_charset`, `_language`, `_encoding` — in argument order, so a refusal names the
// same member on every run.
//
// Each goes out through the CODE_PHRASE **leaf** emitter, which carries `|code`,
// `|terminology` and `|preferred_term` and rides `|raw` for anything beyond them
// (a decorated TERMINOLOGY_ID), so no representable member is refused for want of a
// channel. A member whose code is empty *is* refused: [codePhraseToFlat] writes
// nothing for it, and these are pointer fields where "present but blank" is
// distinguishable from absent — silently collapsing the two would drop the
// attribute.
func codePhraseMembersRMAttr(out map[string]any, base string, ms ...codePhraseMember) error {
	for _, m := range ms {
		if m.cp == nil {
			continue
		}
		path := base + "/" + m.family
		if m.cp.CodeString == "" {
			return fmt.Errorf("%w: %q carries a CODE_PHRASE with no code, which the suffix set writes as nothing at all",
				ErrUnsupportedDatatype, path)
		}
		if _, err := emitLeafValue(out, path, *m.cp, "CODE_PHRASE", false, false); err != nil {
			return err
		}
	}
	return nil
}

// orderedRMAttrs writes the two DV_ORDERED decorations: `_normal_range` and one
// `_other_reference_ranges:N` per REFERENCE_RANGE, indexed by list position.
func orderedRMAttrs[T rm.DVOrdered](out map[string]any, base, anchor string,
	normal *rm.DVInterval[T], others []rm.ReferenceRange[T],
) error {
	if normal != nil {
		if err := intervalToFlat(out, base+"/_normal_range", anchor, normal.Interval); err != nil {
			return err
		}
	}
	for i, rr := range others {
		prefix := base + "/_other_reference_ranges:" + strconv.Itoa(i)
		// REFERENCE_RANGE.range is elided: its bounds and boundary Booleans sit
		// directly under the family instance (the reference's spelling — see
		// decodeRMAttrReferenceRange).
		if err := intervalToFlat(out, prefix, anchor, rr.Range.Interval); err != nil {
			return err
		}
		if err := meaningToFlat(out, prefix+"/meaning", rr.Meaning); err != nil {
			return err
		}
	}
	return nil
}

// intervalToFlat writes the DV_INTERVAL grammar under base: each bound through
// the anchor datatype's own suffix form, then the boundary Booleans.
//
// A Boolean is written only when it **contradicts** the default decode applies
// ([intervalSuffixes]): `|*_unbounded` when true, `|*_included` when false. That
// is what keeps an undecorated interval's FLAT form as short as the reference
// writes it, and it reproduces both corpus shapes exactly — the flags omitted on
// `dv_count`'s `_normal_range`, spelled `false` on `dv_quantity`'s.
//
// An unbounded end writes no bound: the RM's bound value is meaningless there.
// For the nine *concrete* instantiations that reach here — the three narrowed
// `normal_range` fields plus every arm of [intervalLeafToFlat] — the bound
// cannot be nil at all, so the flag is the only channel that can say "no bound
// here", and only a bound differing from the Go zero contradicts it
// ([boundContradictsUnbounded]).
func intervalToFlat[T any](out map[string]any, base, anchor string, iv rm.Interval[T]) error {
	// Whether the instantiation can express Void at all is knowledge only this
	// generic frame has — boxing into `any` erases it — and it changes what
	// counts as a contradiction. Read it off a zero T rather than the bound.
	var zero T
	voidRepresentable := any(zero) == nil || rm.IsTypedNil(any(zero))
	if err := intervalBoundToFlat(out, base, anchor, "lower", any(iv.Lower), iv.LowerUnbounded, voidRepresentable); err != nil {
		return err
	}
	if err := intervalBoundToFlat(out, base, anchor, "upper", any(iv.Upper), iv.UpperUnbounded, voidRepresentable); err != nil {
		return err
	}
	if iv.LowerUnbounded {
		out[base+"|lower_unbounded"] = true
	}
	if iv.UpperUnbounded {
		out[base+"|upper_unbounded"] = true
	}
	if !iv.LowerIncluded {
		out[base+"|lower_included"] = false
	}
	if !iv.UpperIncluded {
		out[base+"|upper_included"] = false
	}
	return nil
}

// intervalBoundToFlat writes one end of a DV_INTERVAL, or nothing when that end
// is unbounded.
//
// A *bounded* end must carry a bound the wire can actually spell. The RM ties
// the flag to the value directly — `lower_unbounded = (lower = Void)` — so an
// end that claims to be bounded and holds no usable bound is an invalid
// interval, and neither way of papering over it is acceptable: with an
// interface-typed bound (`DVInterval[DVOrdered]`, what a canonical decode
// produces) a Void bound emits nothing at all, leaving decode to read the
// opposite of what the flags say, while a concrete-typed bound cannot be nil
// and its Go zero value puts a *fabricated* bound on the wire (`|magnitude` 0
// under an empty, RM-mandatory `|unit`). Refuse both (REQ-140).
//
// An empty **RM-mandatory** suffix is what separates the fabricated zero from a
// legitimate one: a genuine zero magnitude (`DV_COUNT`'s `0` lower bound) leaves
// no mandatory suffix empty and passes, while the Go zero `DV_QUANTITY` spells
// its mandatory `|unit` as "".
func intervalBoundToFlat(out map[string]any, base, anchor, end string, bound any, unbounded, voidRepresentable bool) error {
	if unbounded {
		// The mirror of decode's refusal (intervalSuffixes): a bound standing
		// beside its `|*_unbounded: true` contradicts the RM equivalence, and
		// dropping it here would lose a populated clinical value silently while
		// the same pair on the way in is a typed error.
		if boundContradictsUnbounded(bound, voidRepresentable) {
			return fmt.Errorf("%w: %q carries a %s bound and is also marked `|%s_unbounded`; the RM ties the two (`%s_unbounded = (%s = Void)`), so the pair contradicts itself",
				ErrUnsupportedDatatype, base+"/"+end, end, end, end, end)
		}
		return nil
	}
	path := base + "/" + end
	sub := make(map[string]any)
	if _, err := emitLeafValue(sub, path, bound, anchor, false, false); err != nil {
		return err
	}
	if len(sub) == 0 {
		return fmt.Errorf("%w: %q carries no %s bound but is not marked `|%s_unbounded`; the RM ties the two (`%s_unbounded = (%s = Void)`), so decode would read a bounded end that no key spells",
			ErrUnsupportedDatatype, path, end, end, end, end)
	}
	if key, empty := emptyMandatorySuffix(sub, path, anchor); empty {
		return fmt.Errorf("%w: %q is RM-mandatory on a %s bound but empty — an unpopulated bound is not an unbounded end, which is spelled `%s|%s_unbounded`",
			ErrUnsupportedDatatype, key, anchor, base, end)
	}
	// The one mandatory suffix emptyMandatorySuffix cannot reach that the RM
	// nonetheless forbids outright: DV_PROPORTION spells its mandatory suffixes
	// numerically, so a Go-zero bound leaves none of them *empty* and would ride
	// out as a fabricated `0/0`. The RM states the invariant by name —
	// `Valid_denominator: denominator /= 0.0` (DV_PROPORTION, RM 1.2.0 BMM
	// `resources/bmm/openehr_rm_1.2.0.bmm.json`) — so refusing it is the same
	// move, on the same grounding, as the `Basic_validity` / `Name_valid`
	// refusals in the party grammar and `Setting_valid` at the `ctx/` boundary,
	// not a rule this codec invents. Scoped to a *bound*, where the zero is
	// fabricated by absence; a producer-supplied `0/0` at a plain Web Template
	// leaf is that producer's defect and is carried as written. Encode-only for
	// the same reason: a wire `|denominator: 0` is a value some producer wrote,
	// not one absence fabricated here, so refusing it on decode would be RM
	// validation rather than a codec concern (the validation package owns it).
	if dv, isProportion := as[rm.DVProportion](bound); isProportion && dv.Denominator == 0 {
		return fmt.Errorf("%w: %q is 0, which the RM forbids (`DV_PROPORTION` invariant `Valid_denominator: denominator /= 0.0`); an unpopulated bound is not an unbounded end, which is spelled `%s|%s_unbounded`",
			ErrUnsupportedDatatype, path+"|denominator", base, end)
	}
	maps.Copy(out, sub)
	return nil
}

// boundContradictsUnbounded reports whether an interval end holds a bound a
// reader could tell apart from absence, and so contradicts its `|*_unbounded`
// flag rather than merely restating it.
//
// The two instantiations spell absence differently, and the rule differs with
// them. An **interface** bound (`DVInterval[DVOrdered]` — every REFERENCE_RANGE's
// `range`, and what a polymorphic `DV_INTERVAL` site decodes to) says Void with
// nil, so *anything* non-nil beside the flag is a contradiction, including a
// value-boxed zero: the producer put something in a slot that could have held
// Void. A **concrete** bound (`DVInterval[DVQuantity]` and the eight other
// narrowed instantiations, which is what a canonical decode of a DV_QUANTITY's
// `normal_range` and what `instance.Generate` both produce) cannot be nil at
// all, so absence has exactly one spelling — the Go zero — and only a bound
// differing from it can be reported.
//
// That last limit is real and unclosable, not an oversight: for a concrete `T`,
// `Interval[DVCount]{LowerUnbounded: true}` and the same value with an explicit
// `Lower: DVCount{0}` are the *same bits*. Refusing the pair would refuse every
// half-open concrete count range, so a legitimate clinical zero beside the flag
// stays silent. Only the interface instantiation can carry that distinction, and
// there it is enforced.
//
// A non-nil *pointer* counts as a bound even when it addresses a zero: the
// pointer itself is distinguishable from Void.
func boundContradictsUnbounded(bound any, voidRepresentable bool) bool {
	if bound == nil || rm.IsTypedNil(bound) {
		return false
	}
	if voidRepresentable {
		return true
	}
	v := reflect.ValueOf(bound)
	if v.Kind() == reflect.Pointer {
		return true
	}
	return !v.IsZero()
}

// emptyMandatorySuffix reports the lexically first suffix a bound leaves empty
// that its datatype declares RM-mandatory. A bound can leave more than one
// empty — a Go-zero DV_SCALE spells both `|code` and `|value` — so the scan is
// ordered rather than left to map iteration, which would make the diagnostic
// (and any test pinning it) differ run to run.
//
// Which suffixes those are is read off the datatype rather than enumerated
// here: an RM-optional field is a pointer, nil in a freshly constructed
// instance, and emits no key at all — so precisely the keys a **zero** instance
// of the anchor spells as an empty string are the mandatory string-valued ones
// (`DV_QUANTITY`'s `|unit`, a bare-keyed temporal's own value). An *optional*
// suffix explicitly set to "" is odd but not invalid, and it encodes at a plain
// Web Template leaf, so it must encode at a bound too.
//
// The reach is therefore **string-valued mandatory suffixes only**, measured
// rather than assumed: it fires for DV_QUANTITY (`|unit`), DV_ORDINAL (`|code`)
// and the four temporals (the bare key), and cannot fire for the two anchors
// whose mandatory suffixes are all numeric — DV_COUNT, where a zero magnitude is
// a legitimate bound and must pass, and DV_PROPORTION, whose Go-zero `0/0` the
// RM forbids by name and which [intervalBoundToFlat] therefore refuses on the
// `Valid_denominator` invariant directly, one guard down.
//
// It fires for DV_SCALE too, since 2026-08-06: DV_SCALE gained a [capturedKeys]
// entry and the DV_ORDINAL suffix set, so a Go-zero bound now spells an empty
// mandatory `|code` and is refused here rather than riding out as a fabricated
// `|raw` fragment. Only one anchor stays outside the check — the abstract
// `DV_INTERVAL<DV_ORDERED>`, which has no [capturedKeys] entry and whose bound
// rides `|raw` whole, losslessly, with nothing to fabricate.
func emptyMandatorySuffix(sub map[string]any, path, anchor string) (string, bool) {
	ctor, known := typereg.Default.Lookup(anchor)
	if !known {
		return "", false
	}
	zero := make(map[string]any)
	if _, err := emitLeafValue(zero, path, ctor(), anchor, false, false); err != nil {
		return "", false
	}
	for _, key := range slices.Sorted(maps.Keys(zero)) {
		if s, isString := zero[key].(string); !isString || s != "" {
			continue
		}
		if s, isString := sub[key].(string); isString && s == "" {
			return key, true
		}
	}
	return "", false
}

// mappingsRMAttr writes one `_mapping:N` per TERM_MAPPING, indexed by list
// position: `|match` as the one-character string the wire carries, `/target` as
// a CODE_PHRASE and the optional `/purpose` as a DV_CODED_TEXT, both through
// their own leaf emitters.
//
// A `match` outside TERM_MAPPING's set is a typed error, not a narrowed emit: the
// RM types it as a bare Character with no invariant the compiler can hold, so a
// zero or stray rune would otherwise be written as an unreadable `|match` (decode
// refuses it — [rmattrTails.matchCode]) or, worse, as a mapping claiming a
// relation nobody asserted.
func mappingsRMAttr(out map[string]any, base string, mappings []rm.TermMapping) error {
	for i, tm := range mappings {
		prefix := base + "/_mapping:" + strconv.Itoa(i)
		if !strings.ContainsRune(matchCodes, tm.Match) {
			return fmt.Errorf("%w: %q cannot carry TERM_MAPPING.match %q; it is one of %s (REQ-140)",
				ErrUnsupportedDatatype, prefix+"|match", tm.Match, matchCodes)
		}
		out[prefix+"|match"] = string(tm.Match)
		if _, err := emitLeafValue(out, prefix+"/target", tm.Target, "CODE_PHRASE", false, false); err != nil {
			return err
		}
		if tm.Target.CodeString == "" {
			return fmt.Errorf("%w: %q is RM-mandatory on TERM_MAPPING but carries no code",
				ErrUnsupportedDatatype, prefix+"/target|code")
		}
		if tm.Purpose == nil {
			continue
		}
		if _, err := emitLeafValue(out, prefix+"/purpose", *tm.Purpose, "DV_CODED_TEXT", false, false); err != nil {
			return err
		}
	}
	return nil
}

// meaningToFlat writes REFERENCE_RANGE.meaning at path — bare for a DV_TEXT,
// the `|code`+`|value`(+`|terminology`) form for a DV_CODED_TEXT, via the Phase
// A substitution carve-out the DV_TEXT leaf already carries. Absent is a typed
// error: `meaning` is RM-mandatory and decode requires it, so emitting a range
// without one would produce a payload this codec cannot read back.
func meaningToFlat(out map[string]any, path string, meaning rm.DVTextLike) error {
	if meaning == nil || rm.IsTypedNil(meaning) {
		return fmt.Errorf("%w: %q is RM-mandatory on REFERENCE_RANGE but absent", ErrUnsupportedDatatype, path)
	}
	_, err := emitLeafValue(out, path, meaning, "DV_TEXT", false, false)
	return err
}
