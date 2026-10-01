package rmwrite

import (
	"fmt"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// writeStringValue stores a String child on the value attribute shared by
// DV_URI and DV_EHR_URI.
func writeStringValue(dst *string, attr string, child any) error {
	v, ok := child.(string)
	if !ok {
		return mismatch(attr, child, "String")
	}
	*dst = v
	return nil
}

// writeOptionalString stores a String child on an optional String
// attribute, which the RM type holds as a pointer.
func writeOptionalString(dst **string, attr string, child any) error {
	v, ok := child.(string)
	if !ok {
		return mismatch(attr, child, "String")
	}
	*dst = &v
	return nil
}

func writeDVURISingle(u *rm.DVURI, attr string, child any) error {
	if attr == "value" {
		return writeStringValue(&u.Value, attr, child)
	}
	return fmt.Errorf("%w: *rm.DVURI has no single attr %q", ErrUnknownAttribute, attr)
}

func writeDVEHRURISingle(u *rm.DVEHRURI, attr string, child any) error {
	if attr == "value" {
		return writeStringValue(&u.Value, attr, child)
	}
	return fmt.Errorf("%w: *rm.DVEHRURI has no single attr %q", ErrUnknownAttribute, attr)
}

func writeDVParsableSingle(p *rm.DVParsable, attr string, child any) error {
	switch attr {
	case "value":
		return writeStringValue(&p.Value, attr, child)
	case "formalism":
		return writeStringValue(&p.Formalism, attr, child)
	case "charset", "language":
		v, ok := coerceCodePhrase(child)
		if !ok {
			return mismatch(attr, child, "CODE_PHRASE")
		}
		if attr == "charset" {
			p.Charset = &v
		} else {
			p.Language = &v
		}
		return nil
	}
	return fmt.Errorf("%w: *rm.DVParsable has no single attr %q", ErrUnknownAttribute, attr)
}

func writeDVIdentifierSingle(d *rm.DVIdentifier, attr string, child any) error {
	switch attr {
	case "id":
		return writeStringValue(&d.ID, attr, child)
	case "issuer":
		return writeOptionalString(&d.Issuer, attr, child)
	case "assigner":
		return writeOptionalString(&d.Assigner, attr, child)
	case "type":
		return writeOptionalString(&d.Type, attr, child)
	}
	return fmt.Errorf("%w: *rm.DVIdentifier has no single attr %q", ErrUnknownAttribute, attr)
}
