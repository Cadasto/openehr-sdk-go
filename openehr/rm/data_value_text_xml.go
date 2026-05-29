// Hand-written XML decode helper for [DataValueText] slots (REQ-058).
// Mirrors the JSON-side [DecodeDataValueText] convention: when the
// wire payload omits the `xsi:type` discriminator, default to DV_TEXT
// (the supertype). Used by the generated XML unmarshal code at every
// field site whose BMM type promotes to [DataValueText].

package rm

import (
	"encoding/xml"
	"fmt"

	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// DecodeDataValueTextXML decodes a canonical-XML element into a
// concrete [DataValueText]. Falls back to DV_TEXT when `xsi:type` is
// absent.
func DecodeDataValueTextXML(dec *xml.Decoder, start xml.StartElement) (DataValueText, error) {
	typeName, err := canxml.XSITypeOf(start)
	if err != nil {
		return nil, fmt.Errorf("canxml: decode DataValueText: %w", err)
	}
	if typeName == "" {
		// Canonical-XML shorthand: bare `<name>` ≡ DV_TEXT.
		var dvt DVText
		if err := dec.DecodeElement(&dvt, &start); err != nil {
			return nil, fmt.Errorf("canxml: decode DV_TEXT (default): %w", err)
		}
		return &dvt, nil
	}
	return canxml.DecodeAs[DataValueText](dec, start)
}
