package simplified

// REQ-053 — leaf datatype mapping: a concrete RM DataValue becomes one or
// more FLAT entries under a leaf path, keyed by the pipe attribute suffix
// (bare for value-only types). Explicit type switch, no reflection (REQ-024).
// The switch handles both value and pointer forms because a DataValue slot
// may hold either (see openehr/rm on substitution slots).

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// capturedKeys maps a leaf rmType to the canonical top-level attribute keys its
// FLAT suffix form fully represents. A value carrying any canonical key outside
// this set (mappings, normal_range, other_reference_ranges, hyperlink, …) is not
// faithfully expressible as suffixes, so it is emitted as a lossless |raw
// fragment instead. This keeps the codec semantics-preserving (REQ-053) while
// still producing the human-readable suffix form for the common, undecorated case.
var capturedKeys = map[string]map[string]bool{
	"DV_TEXT":       {"value": true, "formatting": true},
	"DV_CODED_TEXT": {"value": true, "defining_code": true, "formatting": true},
	"DV_DATE_TIME":  {"value": true, "magnitude_status": true, "normal_status": true},
	"DV_DATE":       {"value": true, "magnitude_status": true, "normal_status": true},
	"DV_TIME":       {"value": true, "magnitude_status": true, "normal_status": true},
	"DV_DURATION":   {"value": true, "magnitude_status": true, "normal_status": true, "accuracy": true, "accuracy_is_percent": true},
	"DV_URI":        {"value": true},
	"DV_EHR_URI":    {"value": true},
	"DV_QUANTITY": {
		"magnitude": true, "units": true,
		"magnitude_status": true, "normal_status": true,
		"accuracy": true, "accuracy_is_percent": true,
		"precision": true, "units_system": true, "units_display_name": true,
	},
	"DV_COUNT":   {"magnitude": true, "magnitude_status": true, "normal_status": true, "accuracy": true, "accuracy_is_percent": true},
	"DV_BOOLEAN": {"value": true},
	"DV_ORDINAL": {"symbol": true, "value": true},
	"DV_PROPORTION": {
		"numerator": true, "denominator": true, "type": true,
		"magnitude_status": true, "normal_status": true,
		"accuracy": true, "accuracy_is_percent": true, "precision": true,
	},
	"DV_IDENTIFIER": {"id": true, "issuer": true, "assigner": true, "type": true},
	// CODE_PHRASE is not a DataValue, but the reference emits it as a leaf in its
	// own right (ENTRY language / encoding) under the same |code + |terminology
	// pair a DV_CODED_TEXT's defining_code uses. A preferred_term makes it
	// uncaptured and it rides |raw, as everywhere else.
	"CODE_PHRASE": {"code_string": true, "terminology_id": true},
}

// leafToFlat writes the FLAT entries for a single leaf value at flatPath. rmType
// is the Web Template leaf type. A DV_* value whose canonical form is fully
// captured by the suffix mapping (the common case) is emitted as suffixes; a
// decorated DV_* value (extra attributes, incl. nested decorations of the
// composite keys), a substituted subtype, or an unmapped DV_* type is embedded
// losslessly as a |raw canonical fragment; a leaf datatype this codec does not
// map at all (party / context / other RM attribute) is a documented skip (see
// deviations.md). CODE_PHRASE is mapped despite not being a DataValue — the
// reference emits ENTRY language / encoding as leaves in their own right.
//
// DV_COUNT and DV_BOOLEAN carry their value as the bare leaf (mapping to RM
// magnitude / value), not a |suffix — per the STABLE Simplified Formats RM
// mappings.
func leafToFlat(out map[string]any, flatPath string, v any, rmType string, listOpen bool) error {
	// A typed-nil RM pointer carries no value; skip it rather than dereferencing
	// it in the value switch (which would panic). Equivalent to an absent leaf.
	if v == nil || nilRMPointer(v) {
		return nil
	}
	// A substituted subtype (the value's dynamic type differs from the WT leaf
	// type) must not take the suffix form: decode rebuilds from the leaf type
	// and would silently retype it (e.g. a DV_EHR_URI at a DV_URI leaf). It
	// rides |raw, stamped with its dynamic type. The one spec-sanctioned
	// substitution is DV_TEXT at a DV_CODED_TEXT leaf (the |other form).
	dyn := dvTypeName(v)
	textAtCodedLeaf := dyn == "DV_TEXT" && rmType == "DV_CODED_TEXT"
	if dyn != "" && dyn != rmType && !textAtCodedLeaf {
		return emitRaw(out, flatPath, v, dyn)
	}
	if captured, known := capturedKeys[rmType]; known {
		m, err := canonicalMap(v)
		if err != nil {
			return err
		}
		if capturedFully(rmType, m, captured) {
			return emitCoreLeaf(out, flatPath, v, rmType, listOpen)
		}
	}
	// A modelled leaf type whose value the suffix form cannot capture (a
	// preferred_term, a decorated TERMINOLOGY_ID) rides |raw rather than falling
	// through to the non-value skip below, which would lose it silently.
	if isValueLeafType(rmType) {
		return emitRaw(out, flatPath, v, cmp.Or(dyn, rmType))
	}
	return nil
}

// isValueLeafType reports whether a childless Web Template node of this RM type
// carries a value this codec emits.
//
// Every DV_* type qualifies — |raw is the backstop for one with no suffix
// mapping. CODE_PHRASE qualifies too, and cannot be recognised by an
// Inputs-presence test: the reference emits ENTRY `language` / `encoding` as
// in-context leaves with no input descriptors at all (webtemplate.entryIC), and
// PROBE-075 parity locks that shape, so the reference's silence about inputs
// must not be read as "no value here".
func isValueLeafType(rmType string) bool {
	return strings.HasPrefix(rmType, "DV_") || rmType == "CODE_PHRASE"
}

// canonicalMap returns v's canonical JSON parsed as a generic object — the
// input to the captured-form decision.
func canonicalMap(v any) (map[string]any, error) {
	b, err := canjson.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// codePhraseKeys are the CODE_PHRASE attributes the |code/|terminology suffix
// pair captures; anything else (preferred_term, …) forces |raw.
var codePhraseKeys = map[string]bool{"_type": true, "code_string": true, "terminology_id": true}

// capturedFully reports whether the canonical form m is fully representable by
// rmType's FLAT suffix set. Beyond the top-level key check it descends into the
// composite captured keys — DV_CODED_TEXT.defining_code and DV_ORDINAL.symbol —
// whose nested decorations (preferred_term, a non-local ordinal terminology, …)
// the top-level check alone would silently drop or rewrite.
func capturedFully(rmType string, m map[string]any, captured map[string]bool) bool {
	for k := range m {
		if k != "_type" && !captured[k] {
			return false
		}
	}
	// normal_status is a CODE_PHRASE but travels as a bare code (|normal_status:
	// "N"), so decode rebuilds it in the implied openEHR terminology. A value
	// coded elsewhere, or carrying a preferred_term, would be silently rewritten
	// by that rebuild — it rides |raw instead. Checked here rather than per type
	// because seven datatypes inherit the attribute from DV_ORDERED.
	if ns, present := m["normal_status"]; present && !normalStatusCaptured(ns) {
		return false
	}
	switch rmType {
	case "CODE_PHRASE":
		// The standalone form reuses the nested check, which also bounds the
		// TERMINOLOGY_ID shape the |terminology suffix reduces to a string.
		//
		// An empty code_string is the one shape where "captured" depends on the
		// rest of the value: codePhraseToFlat writes *nothing* for it (see there —
		// load-bearing for a zero-valued ENTRY.language / .encoding, which is a
		// non-pointer field with no nil for [leafToFlat] to catch). That skip is
		// lossless only when there is nothing else to write, so a partly-populated
		// value — a terminology with no code — must not count as captured, or it
		// would encode to nothing at all. It rides |raw instead, the same way a
		// preferred_term beside an empty code already does.
		if cs, _ := m["code_string"].(string); cs == "" {
			return codePhraseTerminology(m) == "" && codePhraseCaptured(m)
		}
		return codePhraseCaptured(m)
	case "DV_CODED_TEXT":
		// Absent defining_code is the DV_TEXT-at-coded-leaf (|other) form, and
		// |other carries the value **alone** — the grammar gives it no companion
		// suffix. So `formatting`, capturable at an ordinary DV_TEXT leaf, is not
		// capturable here and the value must ride |raw.
		//
		// This is load-bearing, not defensive: adding `formatting` to
		// capturedKeys["DV_CODED_TEXT"] without this made emitText write |other
		// and silently discard the formatting (caught in PR #86 review).
		if m["defining_code"] == nil {
			return m["formatting"] == nil
		}
		return codePhraseCaptured(m["defining_code"])
	case "DV_ORDINAL":
		sym, ok := m["symbol"].(map[string]any)
		if !ok {
			return false
		}
		for k := range sym {
			if k != "_type" && k != "value" && k != "defining_code" {
				return false
			}
		}
		if !codePhraseCaptured(sym["defining_code"]) {
			return false
		}
		// The ordinal suffix set has no |terminology channel and decode rebuilds
		// the symbol as archetype-local; a non-local terminology would be
		// silently rewritten, so it rides |raw instead.
		if dc, ok := sym["defining_code"].(map[string]any); ok {
			if tv := codePhraseTerminology(dc); tv != "" && tv != "local" {
				return false
			}
		}
	}
	return true
}

// normalStatusTerminology is the openEHR terminology the |normal_status suffix
// implies: the wire carries only the ordinal code (N, H, HH, L, LL, …), which the
// RM defines against the openEHR code set `normal statuses` — hence the
// terminology_id `openehr` decode rebuilds the CODE_PHRASE with.
const normalStatusTerminology = "openehr"

// normalStatusCaptured reports whether a canonical normal_status CODE_PHRASE is
// representable as the bare |normal_status code — that is, it carries nothing
// beyond the code and sits in the terminology decode will rebuild it in.
func normalStatusCaptured(v any) bool {
	if !codePhraseCaptured(v) {
		return false
	}
	cp, ok := v.(map[string]any)
	if !ok {
		return false
	}
	// An absent or empty terminology contradicts nothing, so it survives the
	// implication decode makes.
	tv := codePhraseTerminology(cp)
	return tv == "" || tv == normalStatusTerminology
}

// codePhraseTerminology returns the terminology_id value carried by a canonical
// CODE_PHRASE object, or "" when it has none — the one attribute the
// |terminology suffix reduces to a bare string, and the one a rebuild can
// therefore silently rewrite.
func codePhraseTerminology(cp map[string]any) string {
	tid, _ := cp["terminology_id"].(map[string]any)
	v, _ := tid["value"].(string)
	return v
}

// codePhraseCaptured reports whether a canonical CODE_PHRASE object carries
// only the attributes the |code/|terminology suffixes represent.
func codePhraseCaptured(v any) bool {
	dc, ok := v.(map[string]any)
	if !ok {
		return false
	}
	for k := range dc {
		if !codePhraseKeys[k] {
			return false
		}
	}
	// TERMINOLOGY_ID reduces to its value string on the wire; anything beyond
	// that is uncaptured.
	if tid, ok := dc["terminology_id"].(map[string]any); ok {
		for k := range tid {
			if k != "_type" && k != "value" {
				return false
			}
		}
	}
	return true
}

// as extracts a concrete RM datatype from the value or pointer form a DataValue
// slot may hold. A typed-nil pointer reports false. Type-switching on a type
// parameter keeps the dispatch reflection-free (REQ-024) and halves the
// value/pointer case duplication.
func as[T any](v any) (T, bool) {
	switch x := v.(type) {
	case T:
		return x, true
	case *T:
		if x != nil {
			return *x, true
		}
	}
	var zero T
	return zero, false
}

// emitCoreLeaf writes the suffix form for a fully-captured leaf value. It is
// only reached for values whose canonical form passed capturedFully.
func emitCoreLeaf(out map[string]any, flatPath string, v any, rmType string, listOpen bool) error {
	if dv, ok := as[rm.DVText](v); ok {
		return emitText(out, flatPath, dv, rmType, listOpen)
	}
	if dv, ok := as[rm.DVCodedText](v); ok {
		codedToFlat(out, flatPath, dv)
		return nil
	}
	if dv, ok := as[rm.DVDateTime](v); ok {
		out[flatPath] = dv.Value
		emitOrdered(out, flatPath, ordered{magnitudeStatus: dv.MagnitudeStatus, normalStatus: dv.NormalStatus})
		return nil
	}
	if dv, ok := as[rm.DVDate](v); ok {
		out[flatPath] = dv.Value
		emitOrdered(out, flatPath, ordered{magnitudeStatus: dv.MagnitudeStatus, normalStatus: dv.NormalStatus})
		return nil
	}
	if dv, ok := as[rm.DVTime](v); ok {
		out[flatPath] = dv.Value
		emitOrdered(out, flatPath, ordered{magnitudeStatus: dv.MagnitudeStatus, normalStatus: dv.NormalStatus})
		return nil
	}
	if dv, ok := as[rm.DVQuantity](v); ok {
		quantityToFlat(out, flatPath, dv)
		return nil
	}
	if dv, ok := as[rm.DVCount](v); ok {
		out[flatPath] = dv.Magnitude
		emitOrdered(out, flatPath, ordered{
			magnitudeStatus: dv.MagnitudeStatus, normalStatus: dv.NormalStatus,
			accuracy: dv.Accuracy, accuracyIsPercent: dv.AccuracyIsPercent,
		})
		return nil
	}
	if dv, ok := as[rm.DVBoolean](v); ok {
		out[flatPath] = dv.Value
		return nil
	}
	if dv, ok := as[rm.DVDuration](v); ok {
		out[flatPath] = dv.Value
		emitOrdered(out, flatPath, ordered{
			magnitudeStatus: dv.MagnitudeStatus, normalStatus: dv.NormalStatus,
			accuracy: dv.Accuracy, accuracyIsPercent: dv.AccuracyIsPercent,
		})
		return nil
	}
	if dv, ok := as[rm.DVURI](v); ok {
		out[flatPath] = dv.Value
		return nil
	}
	if dv, ok := as[rm.DVEHRURI](v); ok {
		out[flatPath] = dv.Value
		return nil
	}
	if dv, ok := as[rm.DVOrdinal](v); ok {
		ordinalToFlat(out, flatPath, dv)
		return nil
	}
	if dv, ok := as[rm.DVProportion](v); ok {
		proportionToFlat(out, flatPath, dv)
		return nil
	}
	if dv, ok := as[rm.DVIdentifier](v); ok {
		identifierToFlat(out, flatPath, dv)
		return nil
	}
	if cp, ok := as[rm.CodePhrase](v); ok {
		codePhraseToFlat(out, flatPath, cp)
		return nil
	}
	return nil
}

// emitText writes a DV_TEXT value: a bare leaf normally, or under the |other
// suffix at a DV_CODED_TEXT leaf constraining an open value-set — a free-text
// entry stored as DV_TEXT (spec §Open Value-Sets and |other). A DV_TEXT at a
// *closed* coded leaf has no FLAT representation the decoder accepts, so encode
// fails loudly instead of emitting an undecodable payload.
func emitText(out map[string]any, flatPath string, dv rm.DVText, rmType string, listOpen bool) error {
	if rmType == "DV_CODED_TEXT" {
		if !listOpen {
			return fmt.Errorf("%w: DV_TEXT at closed DV_CODED_TEXT leaf %q (|other requires an open value-set)", ErrUnsupportedDatatype, flatPath)
		}
		// |other is the free-text escape at a coded leaf and carries the value
		// alone. Any DV_TEXT decoration — `formatting` included — makes the value
		// uncapturable in this form, and [capturedFully] has already routed it to
		// |raw before we get here; writing |other unconditionally would drop the
		// decoration silently.
		out[flatPath+"|other"] = dv.Value
		return nil
	}
	out[flatPath] = dv.Value
	emitFormatting(out, flatPath, dv.Formatting)
	return nil
}

// emitFormatting writes DV_TEXT's optional |formatting suffix, shared by the
// plain and coded forms.
func emitFormatting(out map[string]any, flatPath string, formatting *string) {
	if formatting != nil {
		out[flatPath+"|formatting"] = *formatting
	}
}

// emitRaw embeds v as a |raw canonical fragment stamped with the given _type. A
// fragment that serialises to JSON null (a typed-nil pointer of a non-core
// datatype) is treated as an absent leaf, not emitted — the decoder rejects
// null |raw values.
func emitRaw(out map[string]any, flatPath string, v any, stamp string) error {
	raw, err := rawFragment(v, stamp)
	if err != nil {
		return err
	}
	if raw == nil {
		return nil
	}
	out[flatPath+"|raw"] = raw
	return nil
}

// dvTypeName returns the canonical RM type name of a first-class datatype value
// (value or pointer form), or "" for anything outside that set. No reflection
// (REQ-024).
func dvTypeName(v any) string {
	switch v.(type) {
	case rm.DVText, *rm.DVText:
		return "DV_TEXT"
	case rm.DVCodedText, *rm.DVCodedText:
		return "DV_CODED_TEXT"
	case rm.DVDateTime, *rm.DVDateTime:
		return "DV_DATE_TIME"
	case rm.DVDate, *rm.DVDate:
		return "DV_DATE"
	case rm.DVTime, *rm.DVTime:
		return "DV_TIME"
	case rm.DVQuantity, *rm.DVQuantity:
		return "DV_QUANTITY"
	case rm.DVCount, *rm.DVCount:
		return "DV_COUNT"
	case rm.DVBoolean, *rm.DVBoolean:
		return "DV_BOOLEAN"
	case rm.DVDuration, *rm.DVDuration:
		return "DV_DURATION"
	case rm.DVEHRURI, *rm.DVEHRURI:
		return "DV_EHR_URI"
	case rm.DVURI, *rm.DVURI:
		return "DV_URI"
	case rm.DVOrdinal, *rm.DVOrdinal:
		return "DV_ORDINAL"
	case rm.DVProportion, *rm.DVProportion:
		return "DV_PROPORTION"
	case rm.DVIdentifier, *rm.DVIdentifier:
		return "DV_IDENTIFIER"
	}
	return ""
}

// rawFragment serialises v to its openEHR canonical JSON (via canjson) and
// re-parses it as a generic value, so it nests inside the FLAT/STRUCTURED map
// under a |raw key. Numbers are re-parsed with json.Number so a large integer
// (e.g. a decorated DV_COUNT magnitude above 2^53) is preserved rather than
// rounded through float64. canjson emits _type only for pointer/polymorphic
// forms, so the fragment is stamped with rmType (the Web Template leaf type)
// when the value form omits it — decode requires _type on a |raw fragment.
func rawFragment(v any, rmType string) (any, error) {
	b, err := canjson.Marshal(v)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var frag any
	if err := dec.Decode(&frag); err != nil {
		return nil, err
	}
	if m, ok := frag.(map[string]any); ok {
		if _, has := m["_type"]; !has {
			m["_type"] = rmType
		}
	}
	return frag, nil
}

// nilRMPointer reports whether v is a typed-nil pointer to a first-class RM
// datatype — a value that would panic on dereference in emitCoreLeaf. Explicit
// type switch, no reflection (REQ-024).
func nilRMPointer(v any) bool {
	switch p := v.(type) {
	case *rm.DVText:
		return p == nil
	case *rm.DVCodedText:
		return p == nil
	case *rm.DVDateTime:
		return p == nil
	case *rm.DVDate:
		return p == nil
	case *rm.DVTime:
		return p == nil
	case *rm.DVQuantity:
		return p == nil
	case *rm.DVCount:
		return p == nil
	case *rm.DVBoolean:
		return p == nil
	case *rm.DVDuration:
		return p == nil
	case *rm.DVURI:
		return p == nil
	case *rm.DVEHRURI:
		return p == nil
	case *rm.DVOrdinal:
		return p == nil
	case *rm.DVProportion:
		return p == nil
	case *rm.DVIdentifier:
		return p == nil
	}
	return false
}

// codedToFlat emits the |code, |value and (external only) |terminology suffix
// entries for a DV_CODED_TEXT leaf.
func codedToFlat(out map[string]any, flatPath string, dv rm.DVCodedText) {
	out[flatPath+"|code"] = dv.DefiningCode.CodeString
	out[flatPath+"|value"] = dv.Value
	if term := dv.DefiningCode.TerminologyID.Value; term != "" {
		out[flatPath+"|terminology"] = term
	}
	emitFormatting(out, flatPath, dv.Formatting)
}

// ordered carries the optional attributes a value leaf inherits from DV_ORDERED
// (`normal_status`), DV_QUANTIFIED (`magnitude_status`) and DV_AMOUNT
// (`accuracy`, `accuracy_is_percent`), plus DV_QUANTITY / DV_PROPORTION's
// `precision`. The generated RM types flatten the DataValue hierarchy — each
// struct carries its own copy of the inherited fields with no shared interface —
// so the attributes are gathered here rather than reached through a supertype.
//
// A nil field is an absent attribute and writes no suffix, which is what keeps
// an undecorated value's FLAT form byte-identical to before these suffixes were
// modelled.
//
// `accuracy` is deliberately absent for DV_DATE / DV_DATE_TIME / DV_TIME: those
// inherit from DV_TEMPORAL, which redefines accuracy as a DV_DURATION object
// (DV_ABSOLUTE_QUANTITY, its own parent, declares it as a DV_AMOUNT) rather than
// the Real DV_AMOUNT carries, so it has no scalar suffix form and stays on |raw.
type ordered struct {
	magnitudeStatus   *string
	normalStatus      *rm.CodePhrase
	accuracy          *rm.Real
	accuracyIsPercent *bool
	precision         *rm.Integer
}

// emitOrdered writes the present optional suffixes beside a value leaf. These are
// the suffixes the reference emits alongside the core value; before they were
// modelled, a value carrying any of them rode |raw as a whole.
func emitOrdered(out map[string]any, flatPath string, o ordered) {
	if o.magnitudeStatus != nil {
		out[flatPath+"|magnitude_status"] = *o.magnitudeStatus
	}
	if o.normalStatus != nil {
		// The bare ordinal code; the terminology is implied (see
		// normalStatusTerminology) and normalStatusCaptured has already refused
		// any value that would not survive that implication.
		out[flatPath+"|normal_status"] = o.normalStatus.CodeString
	}
	if o.accuracy != nil {
		out[flatPath+"|accuracy"] = float64(*o.accuracy)
	}
	if o.accuracyIsPercent != nil {
		out[flatPath+"|accuracy_is_percent"] = *o.accuracyIsPercent
	}
	if o.precision != nil {
		out[flatPath+"|precision"] = int64(*o.precision)
	}
}

// codePhraseToFlat emits the |code and |terminology suffixes for a standalone
// CODE_PHRASE leaf (ENTRY language / encoding).
//
// An empty code_string writes nothing. CODE_PHRASE sits in non-pointer fields
// (ENTRY.language, ENTRY.encoding), so "absent" and "zero" are the same value
// here and there is no pointer for [leafToFlat]'s nil check to catch. Emitting
// unconditionally would put empty |code / |terminology leaves on every
// composition whose metadata arrived through the ctx/ forms — the hazard
// EVENT_CONTEXT.setting documents in deviations.md.
//
// The skip is only safe because [capturedFully] holds back the values it would
// lose: a code-less CODE_PHRASE that still carries a terminology rides |raw and
// never reaches here.
func codePhraseToFlat(out map[string]any, flatPath string, cp rm.CodePhrase) {
	if cp.CodeString == "" {
		return
	}
	out[flatPath+"|code"] = cp.CodeString
	if term := cp.TerminologyID.Value; term != "" {
		out[flatPath+"|terminology"] = term
	}
}

// quantityToFlat emits the |magnitude and |unit suffix entries for a
// DV_QUANTITY leaf.
func quantityToFlat(out map[string]any, flatPath string, dv rm.DVQuantity) {
	out[flatPath+"|magnitude"] = float64(dv.Magnitude)
	out[flatPath+"|unit"] = dv.Units
	if dv.UnitsSystem != nil {
		out[flatPath+"|units_system"] = *dv.UnitsSystem
	}
	if dv.UnitsDisplayName != nil {
		out[flatPath+"|units_display_name"] = *dv.UnitsDisplayName
	}
	emitOrdered(out, flatPath, ordered{
		magnitudeStatus: dv.MagnitudeStatus, normalStatus: dv.NormalStatus,
		accuracy: dv.Accuracy, accuracyIsPercent: dv.AccuracyIsPercent,
		precision: dv.Precision,
	})
}

// ordinalToFlat emits the |code, |value and |ordinal suffixes for a DV_ORDINAL
// leaf (symbol coded text + the integer position).
func ordinalToFlat(out map[string]any, flatPath string, dv rm.DVOrdinal) {
	out[flatPath+"|code"] = dv.Symbol.DefiningCode.CodeString
	out[flatPath+"|value"] = dv.Symbol.Value
	out[flatPath+"|ordinal"] = int64(dv.Value)
}

// proportionToFlat emits the |numerator, |denominator and |type suffixes for a
// DV_PROPORTION leaf. The derived bare magnitude and the status suffixes are
// not emitted (they are recomputed from numerator/denominator) — see
// deviations.md.
func proportionToFlat(out map[string]any, flatPath string, dv rm.DVProportion) {
	out[flatPath+"|numerator"] = float64(dv.Numerator)
	out[flatPath+"|denominator"] = float64(dv.Denominator)
	out[flatPath+"|type"] = int64(dv.Type)
	emitOrdered(out, flatPath, ordered{
		magnitudeStatus: dv.MagnitudeStatus, normalStatus: dv.NormalStatus,
		accuracy: dv.Accuracy, accuracyIsPercent: dv.AccuracyIsPercent,
		precision: dv.Precision,
	})
}

// identifierToFlat emits the |id and optional |issuer, |assigner, |type
// suffixes for a DV_IDENTIFIER leaf.
func identifierToFlat(out map[string]any, flatPath string, dv rm.DVIdentifier) {
	out[flatPath+"|id"] = dv.ID
	if dv.Issuer != nil {
		out[flatPath+"|issuer"] = *dv.Issuer
	}
	if dv.Assigner != nil {
		out[flatPath+"|assigner"] = *dv.Assigner
	}
	if dv.Type != nil {
		out[flatPath+"|type"] = *dv.Type
	}
}
