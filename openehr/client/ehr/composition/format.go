package composition

import (
	"errors"
	"fmt"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
)

// Format names the representation a composition travels in on the wire.
// The zero value is [FormatJSON], canonical JSON, which every call uses
// unless an option names another format.
type Format int

const (
	// FormatJSON is canonical JSON, sent as application/json. It is the
	// zero value and the default.
	FormatJSON Format = iota
	// FormatXML is canonical XML, sent as application/xml.
	FormatXML
	// FormatFlat is the FLAT simplified format, sent as
	// application/openehr.wt.flat+json. It is written and read against
	// the template's Web Template.
	FormatFlat
	// FormatStructured is the STRUCTURED simplified format, sent as
	// application/openehr.wt.structured+json. It is written and read
	// against the template's Web Template.
	FormatStructured
)

const (
	mediaTypeJSON = "application/json"
	mediaTypeXML  = "application/xml"
)

// MediaType returns the media type f is sent as. FLAT and STRUCTURED use
// their current media types, never the retired `.schema` variants. A value
// outside the four formats returns "".
func (f Format) MediaType() string {
	switch f {
	case FormatJSON:
		return mediaTypeJSON
	case FormatXML:
		return mediaTypeXML
	case FormatFlat:
		return simplified.MediaTypeFlat
	case FormatStructured:
		return simplified.MediaTypeStructured
	default:
		return ""
	}
}

// String names the format: "JSON", "XML", "FLAT" or "STRUCTURED". A value
// outside the four formats is named by its number.
func (f Format) String() string {
	// The FLAT and STRUCTURED names double as the values of the `format`
	// query parameter WithFormatQuery and WithGetFormatQuery send (REQ-173).
	switch f {
	case FormatJSON:
		return "JSON"
	case FormatXML:
		return "XML"
	case FormatFlat:
		return "FLAT"
	case FormatStructured:
		return "STRUCTURED"
	default:
		return fmt.Sprintf("Format(%d)", int(f))
	}
}

// valid reports whether f is one of the four formats.
func (f Format) valid() bool {
	switch f {
	case FormatJSON, FormatXML, FormatFlat, FormatStructured:
		return true
	default:
		return false
	}
}

// isSimplified reports whether f is FLAT or STRUCTURED, the two formats
// that need a Web Template.
func (f Format) isSimplified() bool {
	switch f {
	case FormatFlat, FormatStructured:
		return true
	case FormatJSON, FormatXML:
		return false
	default:
		return false
	}
}

// marshal serialises comp in f. Only FLAT and STRUCTURED read wt. Callers
// refuse a format outside the four with valid first, so the default arm is
// unreachable; it fails loudly rather than send an empty body.
func (f Format) marshal(comp *rm.Composition, wt *webtemplate.WebTemplate) ([]byte, error) {
	switch f {
	case FormatJSON:
		return canjson.Marshal(comp)
	case FormatXML:
		return canxml.Marshal(comp)
	case FormatFlat:
		return simplified.MarshalFlat(comp, wt)
	case FormatStructured:
		return simplified.MarshalStructured(comp, wt)
	default:
		return nil, fmt.Errorf("composition: no codec for %s", f)
	}
}

// unmarshal decodes body in f and returns the codec's own error unwrapped.
// Only FLAT and STRUCTURED read wt. Callers refuse a format outside the four
// with valid first, so the default arm is unreachable; it fails loudly
// rather than return an empty composition.
func (f Format) unmarshal(body []byte, wt *webtemplate.WebTemplate) (*rm.Composition, error) {
	switch f {
	case FormatJSON:
		comp := new(rm.Composition)
		if err := canjson.Unmarshal(body, comp); err != nil {
			return nil, err
		}
		return comp, nil
	case FormatXML:
		comp := new(rm.Composition)
		if err := canxml.Unmarshal(body, comp); err != nil {
			return nil, err
		}
		return comp, nil
	case FormatFlat:
		return simplified.UnmarshalFlat(body, wt)
	case FormatStructured:
		return simplified.UnmarshalStructured(body, wt)
	default:
		return nil, errors.New("no codec for this format")
	}
}

// decoder returns the function that decodes a write's representation body
// in f. Canonical JSON keeps decodeComposition, the decoder every write used
// before the other formats existed; the others wrap their codec's error the
// same way, naming the format.
func (f Format) decoder(wt *webtemplate.WebTemplate) func([]byte) (*rm.Composition, error) {
	if f == FormatJSON {
		return decodeComposition
	}
	return func(body []byte) (*rm.Composition, error) {
		comp, err := f.unmarshal(body, wt)
		if err != nil {
			return nil, fmt.Errorf("composition: decode %s Composition: %w", f, err)
		}
		return comp, nil
	}
}
