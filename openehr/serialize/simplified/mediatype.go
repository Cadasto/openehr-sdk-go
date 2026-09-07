package simplified

import (
	"errors"
	"fmt"
	"mime"
)

// Format identifies which of the two Simplified Formats a media type names
// (REQ-053).
type Format int

const (
	// FormatUnknown is the zero value: the media type names neither format.
	FormatUnknown Format = iota
	// FormatFlat is the FLAT composition format, application/openehr.wt.flat+json.
	FormatFlat
	// FormatStructured is the STRUCTURED composition format,
	// application/openehr.wt.structured+json.
	FormatStructured
	// formatSentinel is one past the last real format. It is the upper bound
	// the internal completeness test walks, so a member added without an arm
	// in String and MediaType fails a test rather than shipping silently
	// (the exhaustive linter is not enabled in this repo). It is not itself a
	// format, stays unexported, and must remain the last constant here.
	formatSentinel
)

// ErrUnknownMediaType is returned by [ParseMediaType] for a value that names
// neither Simplified Format — a different media type, an unparseable header
// value, or the WebTemplate resource type, which is not a composition format.
var ErrUnknownMediaType = errors.New("simplified: unknown media type")

// String names the format for diagnostics.
func (f Format) String() string {
	switch f {
	case FormatFlat:
		return "FLAT"
	case FormatStructured:
		return "STRUCTURED"
	case FormatUnknown:
		return "unknown"
	default:
		return fmt.Sprintf("Format(%d)", int(f))
	}
}

// MediaType returns the canonical media type for f (REQ-053): the SDK emits
// the two Simplified Formats strings only, never the deprecated
// `.schema`-suffixed variants (retired from the specification, still served by
// EHRbase). FormatUnknown and any out-of-range value yield "".
func (f Format) MediaType() string {
	switch f {
	case FormatFlat:
		return MediaTypeFlat
	case FormatStructured:
		return MediaTypeStructured
	case FormatUnknown:
		return ""
	default:
		return ""
	}
}

// acceptedMediaTypes is the input-side vocabulary: the two canonical strings
// plus the deprecated `.schema`-suffixed variants (retired from the
// specification, still served by EHRbase), which REQ-053 says the SDK SHOULD
// accept on input for interoperability while never emitting them. Keys are
// lower-case; mime.ParseMediaType lower-cases the type it returns.
var acceptedMediaTypes = map[string]Format{
	MediaTypeFlat:       FormatFlat,
	MediaTypeStructured: FormatStructured,
	"application/openehr.wt.flat.schema+json":       FormatFlat,
	"application/openehr.wt.structured.schema+json": FormatStructured,
}

// ParseMediaType classifies one media-type token as FLAT or STRUCTURED — a
// Content-Type value, or a single media range already picked out of an Accept
// list. A comma-separated Accept list is not accepted: split it upstream and
// call this once per range. The type is matched case-insensitively (RFC 2045)
// and every parameter is ignored — `q` included, so a `q=0` range still
// classifies, and a malformed parameter included too: a broken parameter
// beside an otherwise unambiguous type still classifies on that type, because
// the type part says which format the body is and the codec validates the
// bytes regardless (REQ-053 is liberal on input). Only a value whose type
// part itself does not parse is refused. Anything naming neither format —
// including the WebTemplate resource type `application/openehr.wt+json`,
// which is a template projection rather than a composition format — fails
// with [ErrUnknownMediaType]. It never panics on any input (REQ-025).
//
// The codecs themselves take bytes; this is the one call a consumer makes
// before them to decide which codec a negotiated body belongs to.
func ParseMediaType(s string) (Format, error) {
	mt, _, err := mime.ParseMediaType(s)
	if err != nil {
		// A malformed parameter still yields the type it followed, so classify
		// on that type instead of refusing an unambiguous body. Every other
		// parse error means the type part never parsed and there is nothing to
		// classify. The empty-type check is defensive: no current stdlib input
		// pairs ErrInvalidMediaParameter with an empty type (a duplicate
		// parameter name yields an empty type under a different error), and it
		// keeps the cause wrapped rather than falling through to the
		// value-only refusal below.
		if !errors.Is(err, mime.ErrInvalidMediaParameter) || mt == "" {
			return FormatUnknown, fmt.Errorf("%w: %w", ErrUnknownMediaType, err)
		}
	}
	if f, ok := acceptedMediaTypes[mt]; ok {
		return f, nil
	}
	return FormatUnknown, fmt.Errorf("%w: %q", ErrUnknownMediaType, mt)
}
