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
// call this once per range. The type is matched case-insensitively (RFC 2045),
// and well-formed parameters are ignored whether or not this package
// recognises them — `q` included, so a `q=0` range still classifies, because
// this call does no Accept negotiation. A value whose type part or parameters
// do not parse is refused; a comma-separated list is one such value, and
// refusing it is what stops a whole Accept list from being read as its first
// range. Anything naming neither format — including the WebTemplate resource
// type `application/openehr.wt+json`, which is a template projection rather
// than a composition format — fails with [ErrUnknownMediaType]. It never
// panics on any input (REQ-025).
//
// The codecs themselves take bytes; this is the one call a consumer makes
// before them to decide which codec a negotiated body belongs to.
func ParseMediaType(s string) (Format, error) {
	mt, _, err := mime.ParseMediaType(s)
	if err != nil {
		return FormatUnknown, fmt.Errorf("%w: %w", ErrUnknownMediaType, err)
	}
	if f, ok := acceptedMediaTypes[mt]; ok {
		return f, nil
	}
	return FormatUnknown, fmt.Errorf("%w: %q", ErrUnknownMediaType, mt)
}
