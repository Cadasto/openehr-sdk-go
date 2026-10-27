package rmwrite

import (
	"fmt"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

func writeDVMultimediaSingle(m *rm.DVMultimedia, attr string, child any) error {
	switch attr {
	case "media_type":
		v, ok := coerceCodePhrase(child)
		if !ok {
			return mismatch(attr, child, "CODE_PHRASE")
		}
		if v.TerminologyID.Value == "" {
			// media_type codes are drawn from the IANA media-types
			// code set, not the openEHR terminology (REQ-107).
			v.TerminologyID = rm.TerminologyID{Value: "IANA_media-types"}
		}
		m.MediaType = v
		return nil
	case "size":
		v, err := asInteger(attr, child)
		if err != nil {
			return err
		}
		m.Size = v
		return nil
	case "charset":
		return assignVia(child, func(v rm.CodePhrase) { m.Charset = &v }, attr, "CODE_PHRASE")
	case "language":
		return assignVia(child, func(v rm.CodePhrase) { m.Language = &v }, attr, "CODE_PHRASE")
	case "compression_algorithm":
		return assignVia(child, func(v rm.CodePhrase) { m.CompressionAlgorithm = &v }, attr, "CODE_PHRASE")
	case "integrity_check_algorithm":
		return assignVia(child, func(v rm.CodePhrase) { m.IntegrityCheckAlgorithm = &v }, attr, "CODE_PHRASE")
	case "alternate_text":
		return writeOptionalString(&m.AlternateText, attr, child)
	case "uri":
		// A DV_URI or its descendant DV_EHR_URI, stored as given.
		return assignURI(child, func(v rm.DVURILike) { m.URI = v }, attr)
	case "thumbnail":
		return assignVia(child, func(v rm.DVMultimedia) { m.Thumbnail = &v }, attr, "DV_MULTIMEDIA")
	}
	return fmt.Errorf("%w: *rm.DVMultimedia has no single attr %q", ErrUnknownAttribute, attr)
}
