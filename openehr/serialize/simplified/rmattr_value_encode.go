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
	"strconv"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// valueDecorationAttrs maps each canonical DataValue attribute the underscore
// grammar carries to the FLAT family that spells it. It is the source of the
// `|raw` boundary: [capturedKeysDecorated] widens a datatype's captured-key set
// by whichever of these the RM says that datatype declares, so a value whose
// only extras are listed here rides suffixes plus `_` keys where it used to ride
// one `|raw` fragment (REQ-140; deviations.md § |raw boundary).
//
// A decoration the grammar does *not* yet carry — DV_TEXT's `language` and
// `encoding`, which the corpus spells `_language` / `_encoding` and Phase C3
// owns — is deliberately absent, so a value carrying one still rides `|raw`
// whole and loses nothing.
var valueDecorationAttrs = map[string]string{
	"normal_range":           "_normal_range",
	"other_reference_ranges": "_other_reference_ranges",
	"mappings":               "_mapping",
}

// valueRMAttrs writes the underscore-carried decorations of a leaf value at its
// FLAT path. anchor is the suffix type the value was emitted as, which is also
// the type its interval bounds are spelled with.
//
// The final refusal is a guard on [capturedKeysDecorated], not a datatype gap: a
// type whose captured set was widened by a decoration but which no branch here
// reads would have its decoration silently dropped, so it fails loudly instead.
func valueRMAttrs(out map[string]any, base string, v any, anchor string) error {
	switch {
	case anchorCarries(anchor, "normal_range"):
		// DV_ORDERED: `normal_range` + `other_reference_ranges`, whose generic
		// parameter differs per datatype (the generated RM narrows the interval to
		// the anchor type for three of them and leaves it abstract for the rest).
		if dv, ok := as[rm.DVQuantity](v); ok {
			return orderedRMAttrs(out, base, anchor, dv.NormalRange, dv.OtherReferenceRanges)
		}
		if dv, ok := as[rm.DVCount](v); ok {
			return orderedRMAttrs(out, base, anchor, dv.NormalRange, dv.OtherReferenceRanges)
		}
		if dv, ok := as[rm.DVProportion](v); ok {
			return orderedRMAttrs(out, base, anchor, dv.NormalRange, dv.OtherReferenceRanges)
		}
		if dv, ok := as[rm.DVOrdinal](v); ok {
			return orderedRMAttrs(out, base, anchor, dv.NormalRange, dv.OtherReferenceRanges)
		}
		if dv, ok := as[rm.DVDateTime](v); ok {
			return orderedRMAttrs(out, base, anchor, dv.NormalRange, dv.OtherReferenceRanges)
		}
		if dv, ok := as[rm.DVDate](v); ok {
			return orderedRMAttrs(out, base, anchor, dv.NormalRange, dv.OtherReferenceRanges)
		}
		if dv, ok := as[rm.DVTime](v); ok {
			return orderedRMAttrs(out, base, anchor, dv.NormalRange, dv.OtherReferenceRanges)
		}
		if dv, ok := as[rm.DVDuration](v); ok {
			return orderedRMAttrs(out, base, anchor, dv.NormalRange, dv.OtherReferenceRanges)
		}
	case anchorCarries(anchor, "mappings"):
		// DV_TEXT and its coded subtype: `mappings`. Both cases are needed — [as]
		// matches the exact type, and DV_CODED_TEXT reaches the attribute through
		// its embedded DV_TEXT.
		if dv, ok := as[rm.DVCodedText](v); ok {
			return mappingsRMAttr(out, base, dv.Mappings)
		}
		if dv, ok := as[rm.DVText](v); ok {
			return mappingsRMAttr(out, base, dv.Mappings)
		}
	}
	if !decoratedAnchor(anchor) {
		return nil // a datatype the grammar decorates with nothing
	}
	return fmt.Errorf("%w: %q holds a %T whose underscore-carried RM attributes this codec cannot read (REQ-140)",
		ErrUnsupportedDatatype, base, v)
}

// anchorCarries reports whether the underscore grammar carries attr for anchor —
// the same rminfo-derived rule the decode router applies, read off the widened
// captured set so the two cannot disagree.
func anchorCarries(anchor, attr string) bool {
	return capturedKeysDecorated[anchor][attr] && !capturedKeys[anchor][attr]
}

// decoratedAnchor reports whether anchor's captured set was widened at all, i.e.
// whether [valueRMAttrs] owes the value any `_` key.
func decoratedAnchor(anchor string) bool {
	return len(capturedKeysDecorated[anchor]) > len(capturedKeys[anchor])
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
// An unbounded end writes no bound: the RM's bound value is meaningless there
// (and for the three datatypes whose interval is narrowed to a *concrete* bound
// type it cannot be nil at all, so the flag is the only channel that can say
// "no bound here").
func intervalToFlat[T any](out map[string]any, base, anchor string, iv rm.Interval[T]) error {
	if !iv.LowerUnbounded {
		if _, err := emitLeafValue(out, base+"/lower", any(iv.Lower), anchor, false, false); err != nil {
			return err
		}
	}
	if !iv.UpperUnbounded {
		if _, err := emitLeafValue(out, base+"/upper", any(iv.Upper), anchor, false, false); err != nil {
			return err
		}
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
