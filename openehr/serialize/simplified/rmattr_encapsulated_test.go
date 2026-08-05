package simplified

// REQ-140 / REQ-053 — the encapsulated and interval leaf closures: DV_PARSABLE,
// DV_MULTIMEDIA and DV_INTERVAL<T> as first-class Web Template leaves, plus the
// CODE_PHRASE-member families they and DV_TEXT carry (`_charset`, `_language`,
// `_encoding`), the nested `_thumbnail`, and the DV_TEMPORAL `_accuracy`.
//
// Every fixture below is copied from the pinned PROBE-086 corpus bodies
// (`ehrbase_conformance_data_types_dv_multimedia.json`, `…_dv_parsable.json`,
// `…_interval_dv_quantity.json`, `…_dv_text.json`, `…_dv_date.json`), so the
// spellings under test are the reference implementation's (ADR 0014, design
// constraint 6).

import (
	"errors"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// FLAT path fragments for the leaves this phase closes.
const (
	rmattrMultimedia = rmattrEvent + "/dv_multimedia"
	rmattrParsable   = rmattrEvent + "/dv_parsable"
	rmattrDate       = rmattrEvent + "/dv_date"
	rmattrTime       = rmattrEvent + "/dv_time"
	// The interval leaves sit under their own OBSERVATION in the corpus template.
	rmattrIntervalObs   = rmattrSection + "/conformance_interval"
	rmattrIntervalEvent = rmattrIntervalObs + "/any_event:0"
	rmattrInterval      = rmattrIntervalEvent + "/interval_dv_quantity"
	rmattrIntervalCount = rmattrIntervalEvent + "/interval_dv_count"
)

// --- DV_PARSABLE --------------------------------------------------------

// TestDVParsableLeafRoundTrip — REQ-053/REQ-140. The DV_PARSABLE leaf is
// `ehrbase_conformance_data_types_dv_parsable.json` exactly: the bare value,
// `|formalism`, and the two DV_ENCAPSULATED CODE_PHRASE members riding the
// underscore router.
func TestDVParsableLeafRoundTrip(t *testing.T) {
	wt, _ := conformanceWT(t)
	comp := assertRMAttrRoundTrip(t, wt, map[string]any{
		rmattrParsable:                            "Formal instructions on carrying out the procedure...",
		rmattrParsable + "|formalism":             "GLIF 1.0",
		rmattrParsable + "/_charset|code":         "UTF-8",
		rmattrParsable + "/_charset|terminology":  "IANA_character-sets",
		rmattrParsable + "/_language|code":        "en",
		rmattrParsable + "/_language|terminology": "ISO_639-1",
	})
	p := elementValueAt[rm.DVParsable](t, comp, "dv_parsable")
	if p.Value != "Formal instructions on carrying out the procedure..." || p.Formalism != "GLIF 1.0" {
		t.Errorf("DV_PARSABLE = %+v", p)
	}
	if p.Charset == nil || p.Charset.CodeString != "UTF-8" || p.Charset.TerminologyID.Value != "IANA_character-sets" {
		t.Errorf("charset = %+v", p.Charset)
	}
	if p.Language == nil || p.Language.CodeString != "en" {
		t.Errorf("language = %+v", p.Language)
	}
}

// TestDVParsableHalfSpelledRefused — REQ-140. `value` and `formalism` are both
// RM-mandatory on DV_PARSABLE, so half a leaf must not decode to a coerced
// empty string.
func TestDVParsableHalfSpelledRefused(t *testing.T) {
	wt, _ := conformanceWT(t)
	for name, body := range map[string]map[string]any{
		"bare value alone": {rmattrParsable: "x"},
		"formalism alone":  {rmattrParsable + "|formalism": "GLIF 1.0"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := decodeRMAttrErr(t, wt, rmattrBody(body)); !errors.Is(err, ErrUnsupportedDatatype) {
				t.Errorf("err = %v, want ErrUnsupportedDatatype", err)
			}
		})
	}
}

// TestActivityTimingRidesParsableSuffixes — REQ-053. ACTIVITY.timing is the one
// DV_PARSABLE leaf outside the datatype fixture, and it rode `|raw` until this
// phase: a fully-captured DV_PARSABLE now rides the suffix form there too.
func TestActivityTimingRidesParsableSuffixes(t *testing.T) {
	out := map[string]any{}
	timing := rm.DVParsable{Value: "R1/2021-12-21T00:00:00", Formalism: "timing"}
	if err := leafToFlat(out, "p/timing", timing, "DV_PARSABLE", false); err != nil {
		t.Fatalf("leafToFlat: %v", err)
	}
	if _, raw := out["p/timing|raw"]; raw {
		t.Errorf("a fully-captured DV_PARSABLE still rode |raw: %#v", out)
	}
	if out["p/timing"] != timing.Value || out["p/timing|formalism"] != timing.Formalism {
		t.Errorf("suffix form = %#v", out)
	}
}

// --- DV_MULTIMEDIA ------------------------------------------------------

// TestDVMultimediaLeafRoundTrip — REQ-053/REQ-140.
// `ehrbase_conformance_data_types_dv_multimedia.json` exactly: the bare key is
// the **uri**, `|mediatype` and `|size` are RM-mandatory, and `_charset`,
// `_language` and `_thumbnail` ride the underscore router.
func TestDVMultimediaLeafRoundTrip(t *testing.T) {
	wt, _ := conformanceWT(t)
	comp := assertRMAttrRoundTrip(t, wt, map[string]any{
		rmattrMultimedia:                                "http://med.tube.com/sample",
		rmattrMultimedia + "|mediatype":                 "video/H261",
		rmattrMultimedia + "|size":                      504903212,
		rmattrMultimedia + "|alternatetext":             "alternate text",
		rmattrMultimedia + "|compression_algorithm":     "zlib",
		rmattrMultimedia + "|integrity_check":           "b90360558e5420cef47015b1afbd70a156f940afa470b0515f95eacc2edcef6a",
		rmattrMultimedia + "|integrity_check_algorithm": "SHA-256",
		rmattrMultimedia + "/_charset|code":             "UTF-8",
		rmattrMultimedia + "/_charset|terminology":      "IANA_character-sets",
		rmattrMultimedia + "/_language|code":            "en",
		rmattrMultimedia + "/_language|terminology":     "ISO_639-1",
		rmattrMultimedia + "/_thumbnail|data":           "Z2hnZ2pnamdnag==",
		rmattrMultimedia + "/_thumbnail|mediatype":      "image/png",
		rmattrMultimedia + "/_thumbnail|size":           504,
	})
	m := elementValueAt[rm.DVMultimedia](t, comp, "dv_multimedia")
	uri, ok := m.URI.(*rm.DVURI)
	if !ok || uri.Value != "http://med.tube.com/sample" {
		t.Errorf("uri = %#v, want a *rm.DVURI carrying the bare value", m.URI)
	}
	if m.MediaType.CodeString != "video/H261" || m.MediaType.TerminologyID.Value != mediaTypeTerminology {
		t.Errorf("media_type = %+v, want %q implied", m.MediaType, mediaTypeTerminology)
	}
	if m.Size != 504903212 {
		t.Errorf("size = %d", m.Size)
	}
	if m.AlternateText == nil || *m.AlternateText != "alternate text" {
		t.Errorf("alternate_text = %v", m.AlternateText)
	}
	if len(m.IntegrityCheck) == 0 {
		t.Error("integrity_check did not decode as octets")
	}
	if m.IntegrityCheckAlgorithm == nil || m.IntegrityCheckAlgorithm.CodeString != "SHA-256" {
		t.Errorf("integrity_check_algorithm = %+v", m.IntegrityCheckAlgorithm)
	}
	if m.CompressionAlgorithm == nil || m.CompressionAlgorithm.CodeString != "zlib" {
		t.Errorf("compression_algorithm = %+v", m.CompressionAlgorithm)
	}
	if m.Charset == nil || m.Charset.CodeString != "UTF-8" {
		t.Errorf("charset = %+v", m.Charset)
	}
	if m.Thumbnail == nil || string(m.Thumbnail.Data) != "ghggjgjggj" || m.Thumbnail.Size != 504 {
		t.Errorf("thumbnail = %+v", m.Thumbnail)
	}
}

// TestDVMultimediaMandatorySuffixesRequired — REQ-140. `media_type` and `size`
// are RM-mandatory on DV_MULTIMEDIA; a body spelling neither must not decode to
// a zero-valued one.
func TestDVMultimediaMandatorySuffixesRequired(t *testing.T) {
	wt, _ := conformanceWT(t)
	for name, body := range map[string]map[string]any{
		"uri alone":      {rmattrMultimedia: "http://x"},
		"no size":        {rmattrMultimedia + "|mediatype": "image/png"},
		"no mediatype":   {rmattrMultimedia + "|size": 1},
		"unknown suffix": {rmattrMultimedia + "|mediatype": "image/png", rmattrMultimedia + "|size": 1, rmattrMultimedia + "|typo": "x"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := decodeRMAttrErr(t, wt, rmattrBody(body)); !errors.Is(err, ErrUnsupportedDatatype) {
				t.Errorf("err = %v, want ErrUnsupportedDatatype", err)
			}
		})
	}
}

// TestDVMultimediaRawBoundary — REQ-140. `|mediatype` carries the code alone, so
// the terminology is implied. A media_type coded anywhere else would be silently
// re-terminologised by that rebuild, so the whole value rides `|raw` instead —
// the `|normal_status` precedent, one level up.
func TestDVMultimediaRawBoundary(t *testing.T) {
	out := map[string]any{}
	m := rm.DVMultimedia{
		MediaType: rm.CodePhrase{CodeString: "425", TerminologyID: rm.TerminologyID{Value: "openEHR"}},
		Size:      31,
	}
	if err := leafToFlat(out, "p/mm", m, "DV_MULTIMEDIA", false); err != nil {
		t.Fatalf("leafToFlat: %v", err)
	}
	if _, raw := out["p/mm|raw"]; !raw {
		t.Errorf("a media_type coded outside %q should ride |raw, got %#v", mediaTypeTerminology, out)
	}
}

// TestDVMultimediaEmptyMediaTypeTerminologyCaptured — REQ-140. An absent
// terminology contradicts nothing the implication asserts, so it survives the
// suffix form (the [normalStatusCaptured] rule).
func TestDVMultimediaEmptyMediaTypeTerminologyCaptured(t *testing.T) {
	out := map[string]any{}
	m := rm.DVMultimedia{MediaType: rm.CodePhrase{CodeString: "image/png"}, Size: 3}
	if err := leafToFlat(out, "p/mm", m, "DV_MULTIMEDIA", false); err != nil {
		t.Fatalf("leafToFlat: %v", err)
	}
	if _, raw := out["p/mm|raw"]; raw {
		t.Errorf("an untermed media_type should ride suffixes, got %#v", out)
	}
	if out["p/mm|mediatype"] != "image/png" {
		t.Errorf("|mediatype = %#v", out["p/mm|mediatype"])
	}
}

// TestDVMultimediaThumbnailBeyondGrammarRidesRaw — REQ-140. The `_thumbnail`
// family carries the DV_MULTIMEDIA suffix set and nothing nested of its own, so a
// thumbnail whose own charset the suffixes cannot spell rides `_thumbnail|raw` —
// lossless, and the shape the corpus never writes stays unwritten.
func TestDVMultimediaThumbnailBeyondGrammarRidesRaw(t *testing.T) {
	out := map[string]any{}
	m := rm.DVMultimedia{
		MediaType: rm.CodePhrase{CodeString: "video/H261", TerminologyID: rm.TerminologyID{Value: mediaTypeTerminology}},
		Size:      4,
		Thumbnail: &rm.DVMultimedia{
			MediaType: rm.CodePhrase{CodeString: "image/png", TerminologyID: rm.TerminologyID{Value: mediaTypeTerminology}},
			Size:      2,
			Charset:   &rm.CodePhrase{CodeString: "UTF-8"},
		},
	}
	if err := leafToFlat(out, "p/mm", m, "DV_MULTIMEDIA", false); err != nil {
		t.Fatalf("leafToFlat: %v", err)
	}
	if _, raw := out["p/mm/_thumbnail|raw"]; !raw {
		t.Errorf("a decorated thumbnail should ride _thumbnail|raw, got %#v", out)
	}
}

// --- DV_INTERVAL<T> -----------------------------------------------------

// TestDVIntervalLeafRoundTrip — REQ-053/REQ-140. The DV_INTERVAL leaf reuses the
// C1 interval grammar at Web Template leaf position, anchored on the bound type
// the Web Template spells inside the angle brackets. Both corpus shapes:
// `any_event:0` bounds the lower end and leaves the upper unbounded,
// `any_event:1` the reverse.
func TestDVIntervalLeafRoundTrip(t *testing.T) {
	wt, _ := conformanceWT(t)
	comp := assertRMAttrRoundTrip(t, wt, map[string]any{
		rmattrIntervalEvent + "/time":       "2022-01-12T09:00:11.7842493+01:00",
		rmattrInterval + "/lower|magnitude": 72.83,
		rmattrInterval + "/lower|unit":      "Unit",
		rmattrInterval + "|lower_included":  false,
		rmattrInterval + "|upper_included":  false,
		rmattrInterval + "|upper_unbounded": true,
	})
	iv := intervalLeafOf(t, comp)
	if iv.UpperUnbounded != true || iv.LowerUnbounded != false {
		t.Errorf("boundary flags = %+v", iv.Interval)
	}
	if iv.LowerIncluded || iv.UpperIncluded {
		t.Errorf("explicit |*_included false did not survive: %+v", iv.Interval)
	}
	lower, ok := iv.Lower.(*rm.DVQuantity)
	if !ok || lower.Magnitude != 72.83 || lower.Units != "Unit" {
		t.Errorf("lower = %#v", iv.Lower)
	}
}

// TestDVIntervalLeafBareBound — REQ-140. The bound is decoded and emitted by the
// anchor datatype's own machinery, so a DV_COUNT interval carries bare bounds.
func TestDVIntervalLeafBareBound(t *testing.T) {
	wt, _ := conformanceWT(t)
	assertRMAttrRoundTrip(t, wt, map[string]any{
		rmattrIntervalEvent + "/time":  "2022-01-12T09:00:11.7842493+01:00",
		rmattrIntervalCount + "/lower": 3,
		rmattrIntervalCount + "/upper": 9,
	})
}

// TestDVIntervalLeafBogusSubPathRefused — REQ-140. The interval leaf's grammar is
// closed: a sub-path that is neither `/lower` nor `/upper` is refused naming the
// key rather than resolving as a Web Template child that does not exist.
func TestDVIntervalLeafBogusSubPathRefused(t *testing.T) {
	wt, _ := conformanceWT(t)
	key := rmattrInterval + "/middle|magnitude"
	err := decodeRMAttrErr(t, wt, rmattrBody(map[string]any{key: 1.0}))
	if !errors.Is(err, ErrUnsupportedDatatype) {
		t.Fatalf("err = %v, want ErrUnsupportedDatatype", err)
	}
	if !strings.Contains(err.Error(), key) {
		t.Errorf("err = %v, want it to name %q", err, key)
	}
}

// --- DV_TEXT's CODE_PHRASE members --------------------------------------

// TestDVTextLanguageEncodingRoundTrip — REQ-140. `_language` and `_encoding` are
// the same CODE_PHRASE-member families DV_MULTIMEDIA / DV_PARSABLE carry, on a
// third owner: the corpus spells DV_TEXT's language with a `|preferred_term`
// (`ehrbase_conformance_data_types_dv_text.json`), which the CODE_PHRASE suffix
// grammar therefore has to carry.
func TestDVTextLanguageEncodingRoundTrip(t *testing.T) {
	wt, _ := conformanceWT(t)
	comp := assertRMAttrRoundTrip(t, wt, map[string]any{
		rmattrText:                               "text value",
		rmattrText + "/_language|code":           "en",
		rmattrText + "/_language|preferred_term": "English",
		rmattrText + "/_language|terminology":    "ISO_639-1",
		rmattrText + "/_encoding|code":           "UTF-8",
		rmattrText + "/_encoding|terminology":    "IANA_character-sets",
	})
	txt := elementValueAt[rm.DVText](t, comp, "dv_text")
	if txt.Language == nil || txt.Language.CodeString != "en" {
		t.Fatalf("language = %+v", txt.Language)
	}
	if txt.Language.PreferredTerm == nil || *txt.Language.PreferredTerm != "English" {
		t.Errorf("language.preferred_term = %v", txt.Language.PreferredTerm)
	}
	if txt.Encoding == nil || txt.Encoding.CodeString != "UTF-8" {
		t.Errorf("encoding = %+v", txt.Encoding)
	}
}

// TestCodePhraseMemberOwnerAdmission — REQ-140. Which owner admits which member
// is read off the RM (rminfo), not listed: `charset` reaches the two
// DV_ENCAPSULATED subtypes, `encoding` only DV_TEXT and its coded subtype,
// `thumbnail` only DV_MULTIMEDIA.
func TestCodePhraseMemberOwnerAdmission(t *testing.T) {
	wt, _ := conformanceWT(t)
	for _, key := range []string{
		rmattrElement + "/_charset|code",     // DV_QUANTITY declares no charset
		rmattrElement + "/_language|code",    // nor a language
		rmattrMultimedia + "/_encoding|code", // DV_MULTIMEDIA declares no encoding
		rmattrParsable + "/_thumbnail|size",  // nor DV_PARSABLE a thumbnail
		rmattrText + "/_charset|code",        // nor DV_TEXT a charset
	} {
		t.Run(key, func(t *testing.T) {
			if err := decodeRMAttrErr(t, wt, rmattrBody(map[string]any{key: "x"})); !errors.Is(err, ErrUnknownPath) {
				t.Errorf("err = %v, want ErrUnknownPath", err)
			}
		})
	}
}

// --- DV_TEMPORAL `_accuracy` --------------------------------------------

// TestTemporalAccuracyRoundTrip — REQ-140. DV_DATE / DV_DATE_TIME / DV_TIME
// inherit `accuracy` from DV_TEMPORAL, which redefines it as a **DV_DURATION
// object** rather than the Real DV_AMOUNT carries — so it has no scalar
// `|accuracy` suffix and the reference spells it as the bare `_accuracy` family
// (`ehrbase_conformance_data_types_dv_{date,date_time,time}.json`).
func TestTemporalAccuracyRoundTrip(t *testing.T) {
	wt, _ := conformanceWT(t)
	comp := assertRMAttrRoundTrip(t, wt, map[string]any{
		rmattrDate:                    "2022-01-12",
		rmattrDate + "/_accuracy":     "P2D",
		rmattrDateTime:                "2022-01-12T13:22:34.000868+01:00",
		rmattrDateTime + "/_accuracy": "P2DT9H52M",
		rmattrTime:                    "09:52:00",
		rmattrTime + "/_accuracy":     "PT9H52M",
	})
	d := elementValueAt[rm.DVDate](t, comp, "dv_date")
	if d.Accuracy == nil || d.Accuracy.Value != "P2D" {
		t.Errorf("dv_date accuracy = %+v", d.Accuracy)
	}
}

// TestScalarAccuracyStaysASuffix — REQ-140. DV_QUANTITY / COUNT / DURATION /
// PROPORTION declare `accuracy` as a Real, which the `|accuracy` suffix already
// carries — so the `_accuracy` family must not be admitted there, or one
// attribute would have two spellings.
func TestScalarAccuracyStaysASuffix(t *testing.T) {
	wt, _ := conformanceWT(t)
	for _, key := range []string{
		rmattrElement + "/_accuracy",
		rmattrCount + "/_accuracy",
		rmattrProportion + "/_accuracy",
	} {
		t.Run(key, func(t *testing.T) {
			if err := decodeRMAttrErr(t, wt, rmattrBody(map[string]any{key: "P1D"})); !errors.Is(err, ErrUnknownPath) {
				t.Errorf("err = %v, want ErrUnknownPath", err)
			}
		})
	}
}

// --- helpers ------------------------------------------------------------

// elementValueAt returns the first decoded value of type T held by a collapsed
// ELEMENT anywhere under the composition's observations. leafID names the leaf in
// the failure message; the corpus template gives each datatype its own ELEMENT, so
// the type is the selector. Unlike [elementValue] it does not assume the leaf sits
// under the *first* observation — the interval leaves live under a second one.
func elementValueAt[T any](t *testing.T, comp *rm.Composition, leafID string) T {
	t.Helper()
	var zero T
	v, ok := findElementValue[T](comp)
	if !ok {
		t.Fatalf("no ELEMENT carrying the %s leaf's %T found", leafID, zero)
	}
	return v
}

// findElementValue walks every OBSERVATION → EVENT → ITEM_TREE → ELEMENT in comp
// looking for a value of type T.
func findElementValue[T any](comp *rm.Composition) (T, bool) {
	var zero T
	sec, ok := comp.Content[0].(*rm.Section)
	if !ok {
		return zero, false
	}
	for _, item := range sec.Items {
		obs, ok := item.(*rm.Observation)
		if !ok {
			continue
		}
		for _, ev := range obs.Data.Events {
			pe, ok := ev.(*rm.PointEvent[rm.ItemStructure])
			if !ok {
				continue
			}
			tree, ok := pe.Data.(*rm.ItemTree)
			if !ok {
				continue
			}
			for _, it := range tree.Items {
				el, ok := it.(*rm.Element)
				if !ok {
					continue
				}
				if v, ok := as[T](el.Value); ok {
					return v, true
				}
			}
		}
	}
	return zero, false
}

// intervalLeafOf digs the decoded DV_INTERVAL out of the interval OBSERVATION's
// collapsed ELEMENT. Canonical `_type: DV_INTERVAL` instantiates the generic over
// the abstract bound (typereg), so the decoded value is a DVInterval[DVOrdered].
func intervalLeafOf(t *testing.T, comp *rm.Composition) rm.DVInterval[rm.DVOrdered] {
	t.Helper()
	iv, ok := findElementValue[rm.DVInterval[rm.DVOrdered]](comp)
	if !ok {
		t.Fatal("no ELEMENT carrying a DV_INTERVAL found")
	}
	return iv
}
