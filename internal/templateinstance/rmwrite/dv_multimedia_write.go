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
		n, ok := rm.AsInt64(child)
		if !ok {
			return mismatch(attr, child, "Integer")
		}
		m.Size = rm.Integer(n)
		return nil
	case "charset", "language", "compression_algorithm", "integrity_check_algorithm":
		v, ok := child.(*rm.CodePhrase)
		if !ok {
			return mismatch(attr, child, "CODE_PHRASE")
		}
		switch attr {
		case "charset":
			m.Charset = v
		case "language":
			m.Language = v
		case "compression_algorithm":
			m.CompressionAlgorithm = v
		case "integrity_check_algorithm":
			m.IntegrityCheckAlgorithm = v
		}
		return nil
	case "alternate_text":
		v, ok := child.(string)
		if !ok {
			return mismatch(attr, child, "String")
		}
		m.AlternateText = &v
		return nil
	}
	return fmt.Errorf("%w: *rm.DVMultimedia has no single attr %q", ErrUnknownAttribute, attr)
}
