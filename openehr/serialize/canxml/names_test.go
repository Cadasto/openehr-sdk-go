package canxml_test

// REQ-176 § Shapes and § Round trip, REQ-056: canonical-XML dispatch reads a
// registered type alias as the class it names, at an abstract slot and a
// narrow one, while a name the type registry holds reads as itself.

import (
	"errors"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/internal/xmlschema"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// The aliases the tests below read, registered once for the test binary, as a
// generated package registers its own from its init function.
func init() {
	xmlschema.RegisterTypeAlias("TEST_LEGACY_PARTY_SELF", "PARTY_SELF")
	xmlschema.RegisterTypeAlias("TEST_LEGACY_DV_CODED_TEXT", "DV_CODED_TEXT")
	xmlschema.RegisterTypeAlias("PARTY_IDENTIFIED", "PARTY_SELF")
	xmlschema.RegisterTypeAlias("TEST_ALIAS_OF_NOTHING", "NO_SUCH_TYPE")
}

// compositionWith returns a COMPOSITION whose name, a coded text, carries the
// xsi:type nameType, and whose composer carries composerType.
func compositionWith(nameType, composerType string) []byte {
	name := `<name xsi:type="` + nameType + `">`
	return []byte(`<composition xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">` +
		name + `<value>x</value><defining_code><terminology_id><value>local</value></terminology_id><code_string>at1</code_string></defining_code></name>` +
		`<archetype_node_id>x</archetype_node_id><language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>` +
		`<territory><terminology_id><value>ISO_3166-1</value></terminology_id><code_string>GB</code_string></territory>` +
		`<category><value>event</value><defining_code><terminology_id><value>openehr</value></terminology_id><code_string>433</code_string></defining_code></category>` +
		`<composer xsi:type="` + composerType + `"/></composition>`)
}

// TestREQ176_REQ056_TypeAliasDispatch pins how dispatch reads a registered
// alias: as the class it names, at an abstract slot (COMPOSITION.composer,
// through DecodeAs) and at a narrow one (LOCATABLE.name, through
// DecodeAsOrDefault), in every mode with nothing reported; a name the type
// registry holds reads as itself although it is registered as an alias too;
// and an alias of a name the registry does not hold is an unknown type.
func TestREQ176_REQ056_TypeAliasDispatch(t *testing.T) {
	modes := map[string][]canxml.DecoderOption{
		"default": nil,
		"report":  {canxml.WithUnmatched(func(p canxml.Path) { t.Errorf("reported %s; want nothing", p.String()) })},
		"strict":  {canxml.WithStrict()},
	}
	for mode, opts := range modes {
		var c rm.Composition
		if err := canxml.Unmarshal(compositionWith("TEST_LEGACY_DV_CODED_TEXT", "TEST_LEGACY_PARTY_SELF"), &c, opts...); err != nil {
			t.Fatalf("canxml.Unmarshal(aliases, %s) = %v; want nil", mode, err)
		}
		if _, ok := c.Composer.(*rm.PartySelf); !ok {
			t.Errorf("canxml.Unmarshal(%s): composer = %T; want *rm.PartySelf, the class the alias names", mode, c.Composer)
		}
		if _, ok := c.Name.(*rm.DVCodedText); !ok {
			t.Errorf("canxml.Unmarshal(%s): name = %T; want *rm.DVCodedText, the class the alias names", mode, c.Name)
		}

		var registered rm.Composition
		if err := canxml.Unmarshal(compositionWith("DV_CODED_TEXT", "PARTY_IDENTIFIED"), &registered, opts...); err != nil {
			t.Fatalf("canxml.Unmarshal(PARTY_IDENTIFIED, %s) = %v; want nil", mode, err)
		}
		if _, ok := registered.Composer.(*rm.PartyIdentified); !ok {
			t.Errorf("canxml.Unmarshal(PARTY_IDENTIFIED, %s): composer = %T; want *rm.PartyIdentified, the name the registry holds", mode, registered.Composer)
		}

		err := canxml.Unmarshal(compositionWith("DV_CODED_TEXT", "TEST_ALIAS_OF_NOTHING"), new(rm.Composition), opts...)
		if !errors.Is(err, typereg.ErrUnknownType) {
			t.Errorf("canxml.Unmarshal(an alias of nothing, %s) = %v; want typereg.ErrUnknownType", mode, err)
		}
	}
}
