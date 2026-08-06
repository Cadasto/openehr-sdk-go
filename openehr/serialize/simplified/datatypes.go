package simplified

// REQ-053 — leaf datatype mapping: a concrete RM DataValue becomes one or
// more FLAT entries under a leaf path, keyed by the pipe attribute suffix
// (bare for value-only types). Explicit type switch, no reflection (REQ-024).
// The switch handles both value and pointer forms because a DataValue slot
// may hold either (see openehr/rm on substitution slots).

import (
	"bytes"
	"cmp"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"maps"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// capturedKeys maps a leaf rmType to the canonical top-level attribute keys its
// FLAT suffix form fully represents. A value carrying any canonical key outside
// this set (hyperlink, language, encoding, …) is not faithfully expressible as
// suffixes, so it is emitted as a lossless |raw fragment instead. This keeps the
// codec semantics-preserving (REQ-053) while still producing the human-readable
// suffix form for the common, undecorated case.
//
// The suffix set is not the whole FLAT vocabulary: at a Web Template leaf the
// REQ-140 underscore families carry three further attributes beside the suffixes
// (normal_range, other_reference_ranges, mappings), so the decision there is
// taken against [capturedKeysDecorated] — this table stays the *suffix-only*
// answer, which is what a nested position with no `_` keys of its own (an
// interval bound, a REFERENCE_RANGE meaning) is judged by.
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
	// DV_SCALE is DV_ORDINAL with a Real `value`; same symbol, same captured set.
	"DV_SCALE": {"symbol": true, "value": true},
	"DV_PROPORTION": {
		"numerator": true, "denominator": true, "type": true,
		"magnitude_status": true, "normal_status": true,
		"accuracy": true, "accuracy_is_percent": true, "precision": true,
	},
	"DV_IDENTIFIER": {"id": true, "issuer": true, "assigner": true, "type": true},
	"DV_PARSABLE":   {"value": true, "formalism": true},
	// DV_MULTIMEDIA's bare key is the **uri**, not the inline data: the corpus
	// writes `dv_multimedia: "http://med.tube.com/sample"` beside a `|data`
	// suffix for the octets (`ehrbase_conformance_data_types_dv_multimedia`,
	// whose `_thumbnail` spells `|data` and no bare key at all). `media_type`
	// and `size` are RM-mandatory. The three CODE_PHRASE-valued attributes
	// reachable here — media_type and the two algorithms — carry the **code
	// alone**, which is what [capturedFully] bounds below.
	"DV_MULTIMEDIA": {
		"uri": true, "media_type": true, "size": true, "data": true,
		"alternate_text": true, "integrity_check": true,
		"integrity_check_algorithm": true, "compression_algorithm": true,
	},
	// CODE_PHRASE is not a DataValue, but the reference emits it as a leaf in its
	// own right (ENTRY language / encoding, and REQ-140's `_charset` / `_language`
	// / `_encoding` members) under the same |code + |terminology pair a
	// DV_CODED_TEXT's defining_code uses, plus a |preferred_term the nested
	// spelling has no channel for (the corpus writes it at
	// `dv_text/_language|preferred_term`).
	"CODE_PHRASE": {"code_string": true, "terminology_id": true, "preferred_term": true},
}

// capturedKeysDecorated is [capturedKeys] widened, per datatype, by whichever
// [valueDecorationAttrs] the RM says that datatype declares — the set that
// decides the |raw boundary at a Web Template leaf, where [valueRMAttrs] writes
// those attributes as REQ-140 `_` keys beside the suffixes.
//
// Derived from rminfo rather than listed, so it is the same rule the decode
// router applies (`normal_range` reaches every DV_ORDERED, `mappings` DV_TEXT and
// its coded subtype) and cannot drift from the BMM. A datatype that has no
// captured suffix form at all is absent here too: it rides |raw whole, and no
// widening could change that.
var capturedKeysDecorated = decoratedCapturedKeys()

func decoratedCapturedKeys() map[string]map[string]bool {
	out := make(map[string]map[string]bool, len(capturedKeys))
	for rmType, keys := range capturedKeys {
		wide := maps.Clone(keys)
		for attr := range valueDecorationAttrs {
			if _, declared := rminfo.Default.AttributeRMType(rmType, attr); declared {
				wide[attr] = true
			}
		}
		out[rmType] = wide
	}
	return out
}

// leafToFlat writes the FLAT entries for a single leaf value at flatPath. rmType
// is the Web Template leaf type. A DV_* value whose canonical form is fully
// captured by the suffix mapping (the common case) is emitted as suffixes; a
// decorated DV_* value (extra attributes, incl. nested decorations of the
// composite keys), a substituted subtype (bar the two spec-sanctioned forms
// noted below), or an unmapped DV_* type is embedded losslessly as a |raw
// canonical fragment; a leaf datatype this codec does not
// map at all (party / context / other RM attribute) is a documented skip (see
// deviations.md). CODE_PHRASE is mapped despite not being a DataValue — the
// reference emits ENTRY language / encoding as leaves in their own right.
//
// DV_COUNT and DV_BOOLEAN carry their value as the bare leaf (mapping to RM
// magnitude / value), not a |suffix — per the STABLE Simplified Formats RM
// mappings.
//
// A value emitted in suffix form then gets its REQ-140 underscore-carried
// attributes ([valueRMAttrs]) beside those suffixes; one that rode |raw does not,
// because the fragment already carries them.
func leafToFlat(out map[string]any, flatPath string, v any, rmType string, listOpen bool) error {
	anchor, err := emitLeafValue(out, flatPath, v, rmType, listOpen, true)
	if err != nil || anchor == "" {
		return err
	}
	return valueRMAttrs(out, flatPath, v, anchor)
}

// emitLeafValue writes the leaf value itself — the suffix form or the |raw
// fallback — and returns the suffix type it used, or "" when the value was
// absent or rode |raw. That return is what tells [leafToFlat] whether the
// underscore decorations are still owed, and it doubles as the anchor type they
// are spelled with.
//
// decorated says whether this position has the REQ-140 `_` keys available: true
// at a Web Template leaf, false at a nested position inside the underscore
// grammar (an interval bound, a REFERENCE_RANGE meaning), where a decorated
// value has to ride |raw because there is nowhere else for its extras to go.
func emitLeafValue(out map[string]any, flatPath string, v any, rmType string, listOpen, decorated bool) (string, error) {
	// A typed-nil RM pointer carries no value; skip it rather than dereferencing
	// it in the value switch (which would panic). Equivalent to an absent leaf.
	if v == nil || nilRMPointer(v) {
		return "", nil
	}
	// A party leaf — the ENTRY `subject` in-context leaf (REQ-053) — is not a
	// DataValue and has no suffix *set*: it decomposes through REQ-140's party
	// grammar, which owns the same spelling at every other position it reaches a
	// party (rmattr_party_encode.go). The returned anchor is "" because a party
	// carries no value decoration, so the [valueRMAttrs] walk is not owed.
	//
	// The composition's `composer` is the same datatype and is deliberately *not*
	// reached here: emitNode holds the ctx/-owned metadata leaves back by name
	// (ADR 0015), rather than relying on this arm's absence as it used to.
	if isPartyLeafType(rmType) {
		return "", partyRMAttr(out, flatPath, v)
	}
	// A DV_INTERVAL<T> leaf is not a suffix set either: it decomposes into the
	// `/lower` + `/upper` sub-objects and the four boundary Booleans REQ-140's
	// interval grammar owns (rmattr_value_encode.go), which the `_normal_range`
	// family already spells at every other position. The anchor returned is ""
	// because DV_INTERVAL declares none of the underscore-carried decorations, so
	// the [valueRMAttrs] walk is not owed.
	if isIntervalLeafType(rmType) {
		return "", intervalLeafToFlat(out, flatPath, rmType, v)
	}
	// A substituted subtype (the value's dynamic type differs from the WT leaf
	// type) must not take the suffix form: decode rebuilds from the leaf type
	// and would silently retype it (e.g. a DV_EHR_URI at a DV_URI leaf). It
	// rides |raw, stamped with its dynamic type. Two substitutions are
	// spec-sanctioned in non-|raw form (REQ-053): DV_TEXT at a DV_CODED_TEXT
	// leaf (the |other form) and DV_CODED_TEXT at a DV_TEXT leaf, whose |code
	// suffix lets decode re-select the coded builder — no retyping either way.
	dyn := dvTypeName(v)
	textAtCodedLeaf := dyn == "DV_TEXT" && rmType == "DV_CODED_TEXT"
	codedAtTextLeaf := dyn == "DV_CODED_TEXT" && rmType == "DV_TEXT"
	if dyn != "" && dyn != rmType && !textAtCodedLeaf && !codedAtTextLeaf {
		return "", emitRaw(out, flatPath, v, dyn)
	}
	// The coded-at-text carve-out is judged (and emitted) by the value's own
	// DV_CODED_TEXT suffix set, not the leaf's DV_TEXT one: a decorated coded
	// text whose extras neither the suffixes nor the underscore families can
	// carry (a preferred_term, …) still rides |raw below, stamped with its
	// dynamic type.
	sfxType := rmType
	if codedAtTextLeaf {
		sfxType = "DV_CODED_TEXT"
	}
	table := capturedKeys
	if decorated {
		table = capturedKeysDecorated
	}
	if captured, known := table[sfxType]; known {
		m, err := canonicalMap(v)
		if err != nil {
			return "", err
		}
		if capturedFully(sfxType, m, captured) {
			return sfxType, emitCoreLeaf(out, flatPath, v, sfxType, listOpen)
		}
	}
	// A modelled leaf type whose value the suffix form cannot capture (a
	// preferred_term, a decorated TERMINOLOGY_ID) rides |raw rather than falling
	// through to the non-value skip below, which would lose it silently.
	if isValueLeafType(rmType) {
		return "", emitRaw(out, flatPath, v, cmp.Or(dyn, rmType))
	}
	return "", nil
}

// isValueLeafType reports whether a childless Web Template node of this RM type
// carries a value this codec emits.
//
// Every DV_* type qualifies — |raw is the backstop for one with no suffix
// mapping. CODE_PHRASE qualifies too, and cannot be recognised by an
// Inputs-presence test: the reference emits ENTRY `language` / `encoding` as
// in-context leaves with no input descriptors at all (webtemplate.entryIC), and
// PROBE-075 parity locks that shape, so the reference's silence about inputs
// must not be read as "no value here". A party leaf joined on the same terms with
// REQ-140's party grammar.
func isValueLeafType(rmType string) bool {
	return strings.HasPrefix(rmType, "DV_") || rmType == "CODE_PHRASE" || isPartyLeafType(rmType)
}

// isIntervalLeafType reports whether a Web Template leaf's RM type is a
// DV_INTERVAL, whose FLAT form is the REQ-140 interval grammar rather than a
// suffix set. The Web Template spells the bound datatype inside the angle
// brackets (`DV_INTERVAL<DV_QUANTITY>`, `DV_INTERVAL<DV_COUNT>`, … — verified
// against the vendored PROBE-086 corpus template, whose `conformance_interval`
// OBSERVATION carries six of them), which is the only place the anchor the bounds
// are spelled with can come from.
func isIntervalLeafType(rmType string) bool {
	_, ok := intervalLeafAnchor(rmType)
	return ok
}

// intervalLeafAnchor extracts the bound datatype from a `DV_INTERVAL<T>` Web
// Template leaf type. A bare, unparameterised `DV_INTERVAL` names no bound
// datatype, so it is not an interval leaf here: the bounds would have no suffix
// form and the value rides |raw instead of being silently mis-spelled.
func intervalLeafAnchor(rmType string) (string, bool) {
	inner, ok := strings.CutPrefix(rmType, "DV_INTERVAL<")
	if !ok {
		return "", false
	}
	inner, ok = strings.CutSuffix(inner, ">")
	if !ok || inner == "" {
		return "", false
	}
	return inner, true
}

// isPartyLeafType reports whether a Web Template leaf's RM type is a party — the
// ENTRY `subject` and COMPOSITION `composer` in-context leaves, which the
// reference declares as the abstract PARTY_PROXY. The concrete subtypes are
// admitted too, because a template is free to narrow the slot and the FLAT
// spelling is the same either way (the party grammar reads the subtype off the
// keys, not off the leaf type).
func isPartyLeafType(rmType string) bool {
	switch rmType {
	case "PARTY_PROXY", "PARTY_IDENTIFIED", "PARTY_RELATED", "PARTY_SELF":
		return true
	}
	return false
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

// codePhraseKeys are the CODE_PHRASE attributes the **nested** |code/|terminology
// suffix pair captures — a DV_CODED_TEXT's defining_code, a DV_ORDINAL symbol's,
// a |normal_status. Anything else (preferred_term, …) forces |raw, because those
// positions spell a code and a terminology and nothing more.
var codePhraseKeys = map[string]bool{"_type": true, "code_string": true, "terminology_id": true}

// codePhraseLeafKeys is [codePhraseKeys] plus `preferred_term`: a **standalone**
// CODE_PHRASE leaf (ENTRY language / encoding, REQ-140's `_charset` / `_language`
// / `_encoding` members) has a `|preferred_term` suffix of its own, which the
// corpus writes at `dv_text/_language|preferred_term`.
var codePhraseLeafKeys = map[string]bool{
	"_type": true, "code_string": true, "terminology_id": true, "preferred_term": true,
}

// mediaTypeTerminology is the openEHR code set DV_MULTIMEDIA.media_type is drawn
// from — the IANA media types, whose external identifier the RM names and this
// SDK already pins in the template-instance writer (REQ-107).
//
// The `|mediatype` suffix carries the **code alone**, so the terminology has to be
// implied on decode, and the implication is only safe because encode refuses to
// take the suffix form for a media_type coded anywhere else: that value rides
// |raw whole instead of being silently re-terminologised. It is the
// |normal_status rule one attribute up, and it is corpus-verified in both
// directions — the vendored `Test_dv_multimedia_open_constraint.v0` and
// `Demonstration.v1` canonical compositions both carry exactly this terminology
// against a bare-code FLAT spelling.
const mediaTypeTerminology = "IANA_media-types"

// multimediaBareCodeAttrs are the DV_MULTIMEDIA attributes whose CODE_PHRASE the
// suffix set reduces to a bare code with **no** terminology to imply: the openEHR
// code sets behind `compression algorithms` and `integrity check algorithms` have
// no identifier this codec can source the way media_type's is sourced above, so
// decode writes the code alone and encode refuses the suffix form for a value that
// carries a terminology (it rides |raw). Recorded in deviations.md.
var multimediaBareCodeAttrs = []string{"integrity_check_algorithm", "compression_algorithm"}

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
		// The standalone form admits a |preferred_term the nested spelling has no
		// channel for, and bounds the TERMINOLOGY_ID shape the |terminology suffix
		// reduces to a string.
		//
		// An empty code_string is the one shape where "captured" depends on the
		// rest of the value: codePhraseToFlat writes *nothing* for it (see there —
		// load-bearing for a zero-valued ENTRY.language / .encoding, which is a
		// non-pointer field with no nil for [leafToFlat] to catch). That skip is
		// lossless only when there is nothing else to write, so a partly-populated
		// value — a terminology with no code — must not count as captured, or it
		// would encode to nothing at all. It rides |raw instead.
		if cs, _ := m["code_string"].(string); cs == "" {
			return codePhraseTerminology(m) == "" && m["preferred_term"] == nil &&
				codePhraseCapturedIn(m, codePhraseLeafKeys)
		}
		return codePhraseCapturedIn(m, codePhraseLeafKeys)
	case "DV_MULTIMEDIA":
		// media_type is RM-mandatory and travels as a bare code in the implied
		// [mediaTypeTerminology]; the two algorithm codes travel with no implied
		// terminology at all. A value the rebuild would re-terminologise, or one
		// carrying a preferred_term the suffix has no room for, rides |raw whole.
		mt, ok := m["media_type"].(map[string]any)
		if !ok || !codePhraseCaptured(mt) {
			return false
		}
		if cs, _ := mt["code_string"].(string); cs == "" {
			return false // nothing to write; |raw keeps the rest of the value
		}
		if tv := codePhraseTerminology(mt); tv != "" && tv != mediaTypeTerminology {
			return false
		}
		for _, attr := range multimediaBareCodeAttrs {
			cp, present := m[attr].(map[string]any)
			if !present {
				continue
			}
			if !codePhraseCaptured(cp) || codePhraseTerminology(cp) != "" {
				return false
			}
		}
		// The bare key is a plain DV_URI; a DV_EHR_URI at `uri` would come back
		// retyped, so it rides |raw (the substitution rule, one level down).
		if uri, present := m["uri"].(map[string]any); present {
			if t, _ := uri["_type"].(string); t != "DV_URI" {
				return false
			}
			for k := range uri {
				if k != "_type" && k != "value" {
					return false
				}
			}
		}
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
	case "DV_ORDINAL", "DV_SCALE":
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
// only the attributes the nested |code/|terminology suffixes represent.
func codePhraseCaptured(v any) bool {
	return codePhraseCapturedIn(v, codePhraseKeys)
}

// codePhraseCapturedIn is [codePhraseCaptured] against an explicit key set, so the
// nested and standalone spellings — which differ by |preferred_term alone — share
// one implementation.
func codePhraseCapturedIn(v any, keys map[string]bool) bool {
	dc, ok := v.(map[string]any)
	if !ok {
		return false
	}
	for k := range dc {
		if !keys[k] {
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
	if dv, ok := as[rm.DVScale](v); ok {
		scaleToFlat(out, flatPath, dv)
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
	if dv, ok := as[rm.DVParsable](v); ok {
		out[flatPath] = dv.Value
		out[flatPath+"|formalism"] = dv.Formalism
		return nil
	}
	if dv, ok := as[rm.DVMultimedia](v); ok {
		multimediaToFlat(out, flatPath, dv)
		return nil
	}
	return nil
}

// multimediaToFlat emits the DV_MULTIMEDIA suffix set: the bare `uri`, the
// RM-mandatory `|mediatype` + `|size`, and the optional octet / text / coded
// attributes. Only reached for values [capturedFully] has already bounded, so the
// three CODE_PHRASEs are code-only and the uri a plain DV_URI.
//
// The two Byte[] attributes are written as the base64 their canonical JSON form
// carries (encoding/json's []byte convention), which is exactly what the corpus
// spells at `_thumbnail|data`.
func multimediaToFlat(out map[string]any, flatPath string, dv rm.DVMultimedia) {
	if uri, ok := as[rm.DVURI](dv.URI); ok {
		out[flatPath] = uri.Value
	}
	out[flatPath+"|mediatype"] = dv.MediaType.CodeString
	out[flatPath+"|size"] = int64(dv.Size)
	if len(dv.Data) > 0 {
		out[flatPath+"|data"] = base64.StdEncoding.EncodeToString(dv.Data)
	}
	if len(dv.IntegrityCheck) > 0 {
		out[flatPath+"|integrity_check"] = base64.StdEncoding.EncodeToString(dv.IntegrityCheck)
	}
	if dv.AlternateText != nil {
		out[flatPath+"|alternatetext"] = *dv.AlternateText
	}
	if dv.IntegrityCheckAlgorithm != nil {
		out[flatPath+"|integrity_check_algorithm"] = dv.IntegrityCheckAlgorithm.CodeString
	}
	if dv.CompressionAlgorithm != nil {
		out[flatPath+"|compression_algorithm"] = dv.CompressionAlgorithm.CodeString
	}
}

// intervalLeafToFlat writes a DV_INTERVAL Web Template leaf through REQ-140's
// interval grammar ([intervalToFlat]), with the bound datatype the leaf type names
// as the anchor its `/lower` and `/upper` are spelled with.
//
// The generated `DVInterval[T]` is generic and reflection is forbidden (REQ-024),
// so the instantiations are enumerated — the same set `rm.RMTypeName` switches
// over. `DVInterval[DVOrdered]`, the abstract instantiation, is what a canonical
// decode produces (typereg registers `DV_INTERVAL` as that), and the concrete ones
// are what a hand-built or datatype-narrowed composition carries.
func intervalLeafToFlat(out map[string]any, flatPath, rmType string, v any) error {
	anchor, named := intervalLeafAnchor(rmType)
	if !named {
		return fmt.Errorf("%w: %q is typed %q, which names no interval bound datatype to spell its bounds with",
			ErrUnsupportedDatatype, flatPath, rmType)
	}
	if iv, ok := as[rm.DVInterval[rm.DVOrdered]](v); ok {
		return intervalToFlat(out, flatPath, anchor, iv.Interval)
	}
	if iv, ok := as[rm.DVInterval[rm.DVQuantity]](v); ok {
		return intervalToFlat(out, flatPath, anchor, iv.Interval)
	}
	if iv, ok := as[rm.DVInterval[rm.DVCount]](v); ok {
		return intervalToFlat(out, flatPath, anchor, iv.Interval)
	}
	if iv, ok := as[rm.DVInterval[rm.DVDateTime]](v); ok {
		return intervalToFlat(out, flatPath, anchor, iv.Interval)
	}
	if iv, ok := as[rm.DVInterval[rm.DVDate]](v); ok {
		return intervalToFlat(out, flatPath, anchor, iv.Interval)
	}
	if iv, ok := as[rm.DVInterval[rm.DVTime]](v); ok {
		return intervalToFlat(out, flatPath, anchor, iv.Interval)
	}
	if iv, ok := as[rm.DVInterval[rm.DVDuration]](v); ok {
		return intervalToFlat(out, flatPath, anchor, iv.Interval)
	}
	if iv, ok := as[rm.DVInterval[rm.DVOrdinal]](v); ok {
		return intervalToFlat(out, flatPath, anchor, iv.Interval)
	}
	if iv, ok := as[rm.DVInterval[rm.DVProportion]](v); ok {
		return intervalToFlat(out, flatPath, anchor, iv.Interval)
	}
	if iv, ok := as[rm.DVInterval[rm.DVScale]](v); ok {
		return intervalToFlat(out, flatPath, anchor, iv.Interval)
	}
	return fmt.Errorf("%w: %q is a %s leaf but holds a %T, which is no DV_INTERVAL this codec spells",
		ErrUnsupportedDatatype, flatPath, rmType, v)
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
	case rm.DVScale, *rm.DVScale:
		return "DV_SCALE"
	case rm.DVProportion, *rm.DVProportion:
		return "DV_PROPORTION"
	case rm.DVIdentifier, *rm.DVIdentifier:
		return "DV_IDENTIFIER"
	case rm.DVParsable, *rm.DVParsable:
		return "DV_PARSABLE"
	case rm.DVMultimedia, *rm.DVMultimedia:
		return "DV_MULTIMEDIA"
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
	case *rm.DVParsable:
		return p == nil
	case *rm.DVMultimedia:
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
	if cp.PreferredTerm != nil {
		out[flatPath+"|preferred_term"] = *cp.PreferredTerm
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

// scaleToFlat emits the |code, |value and |ordinal suffixes for a DV_SCALE leaf.
//
// DV_SCALE is DV_ORDINAL with a Real `value` — same `symbol: DV_CODED_TEXT`, same
// RM attribute names — so it takes the same three suffixes, `|ordinal` carrying
// the Real. **Not corpus-pinned:** no vendored body and no reference sample
// spells a DV_SCALE, so this is this codec's choice within the format's own
// conventions, on the same footing as the STRUCTURED `"|"` member, and both
// wire.md § REQ-053 and deviations.md record it as such. Reusing the sibling's spelling for the sibling's
// attribute is the same move the party `|type` suffix makes; should a reference
// sample later spell it otherwise, ADR 0014 means the reference wins.
func scaleToFlat(out map[string]any, flatPath string, dv rm.DVScale) {
	out[flatPath+"|code"] = dv.Symbol.DefiningCode.CodeString
	out[flatPath+"|value"] = dv.Symbol.Value
	out[flatPath+"|ordinal"] = float64(dv.Value)
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
