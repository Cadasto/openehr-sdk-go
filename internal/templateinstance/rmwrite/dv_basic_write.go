package rmwrite

import (
	"fmt"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// writeDVStateSingle writes a DV_STATE's coded value and its is_terminal
// flag.
func writeDVStateSingle(s *rm.DVState, attr string, child any) error {
	switch attr {
	case "value":
		// Value field. The synthesiser passes *DVCodedText.
		return assignVia(child, func(v rm.DVCodedText) { s.Value = v }, attr, "DV_CODED_TEXT")
	case "is_terminal":
		v, ok := child.(bool)
		if !ok {
			return mismatch(attr, child, "Boolean")
		}
		s.IsTerminal = v
		return nil
	}
	return fmt.Errorf("%w: *rm.DVState has no single attr %q", ErrUnknownAttribute, attr)
}

// writeDVParagraphMultiple appends a text to a DV_PARAGRAPH's items. A
// DV_CODED_TEXT is taken as well as a DV_TEXT.
func writeDVParagraphMultiple(p *rm.DVParagraph, attr string, child any) error {
	if attr == "items" {
		return assignDVTextLike(child, func(v rm.DVTextLike) { p.Items = append(p.Items, v) }, attr)
	}
	return fmt.Errorf("%w: *rm.DVParagraph has no multiple attr %q", ErrUnknownAttribute, attr)
}

// writeTimeSpecificationSingle writes the DV_PARSABLE value a
// DV_GENERAL_TIME_SPECIFICATION or a DV_PERIODIC_TIME_SPECIFICATION holds,
// its one attribute. rmType names the class for the error detail.
func writeTimeSpecificationSingle(rmType string, value *rm.DVParsable, attr string, child any) error {
	if attr == "value" {
		// Value field. The synthesiser passes *DVParsable.
		return assignVia(child, func(v rm.DVParsable) { *value = v }, attr, "DV_PARSABLE")
	}
	return fmt.Errorf("%w: %s has no single attr %q", ErrUnknownAttribute, rmType, attr)
}
