package xmlschema_test

// REQ-176 § Shapes and § Round trip, REQ-056: the registrations of the names a
// generated package's classes take beyond their own read back as registered,
// and refuse what a generated package would only get wrong.

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/internal/xmlschema"
)

// TestREQ176_REQ056_RegisteredNamesReadBack pins the lookups the canonical-XML
// codec reads the registrations through: an alias reads as the name it was
// registered for and a root element as the element registered for its class,
// and a name registered as neither reads as nothing.
func TestREQ176_REQ056_RegisteredNamesReadBack(t *testing.T) {
	xmlschema.RegisterTypeAlias("TEST_READ_BACK_ALIAS", "PARTY_SELF")
	xmlschema.RegisterRootElement("TEST_READ_BACK_CLASS", "read_back")
	if got, ok := xmlschema.AliasOf("TEST_READ_BACK_ALIAS"); !ok || got != "PARTY_SELF" {
		t.Errorf("AliasOf(TEST_READ_BACK_ALIAS) = %q, %v; want PARTY_SELF, true", got, ok)
	}
	if got, ok := xmlschema.RootElement("TEST_READ_BACK_CLASS"); !ok || got != "read_back" {
		t.Errorf("RootElement(TEST_READ_BACK_CLASS) = %q, %v; want read_back, true", got, ok)
	}
	if got, ok := xmlschema.AliasOf("TEST_READ_BACK_CLASS"); ok {
		t.Errorf("AliasOf(a class registered for its root element only) = %q, true; want false", got)
	}
	if got, ok := xmlschema.RootElement("TEST_READ_BACK_ALIAS"); ok {
		t.Errorf("RootElement(an alias) = %q, true; want false", got)
	}
}

// TestRegisterTypeAliasAndRootElementRefuse pins the programming errors the two
// registrations refuse, by panicking: an empty name, and a name registered
// twice.
func TestRegisterTypeAliasAndRootElementRefuse(t *testing.T) {
	xmlschema.RegisterTypeAlias("TEST_ALIAS_TWICE", "PARTY_SELF")
	for name, register := range map[string]func(){
		"an empty alias":        func() { xmlschema.RegisterTypeAlias("", "PARTY_SELF") },
		"an empty type name":    func() { xmlschema.RegisterTypeAlias("TEST_EMPTY_TARGET", "") },
		"an alias twice":        func() { xmlschema.RegisterTypeAlias("TEST_ALIAS_TWICE", "PARTY_SELF") },
		"an empty class name":   func() { xmlschema.RegisterRootElement("", "root") },
		"an empty element name": func() { xmlschema.RegisterRootElement("TEST_CLASS", "") },
		"a root element twice": func() {
			xmlschema.RegisterRootElement("TEST_ROOT_TWICE", "a")
			xmlschema.RegisterRootElement("TEST_ROOT_TWICE", "b")
		},
		"an empty alias and name": func() { xmlschema.RegisterTypeAlias("", "") },
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Errorf("registering %s did not panic", name)
				}
			}()
			register()
		})
	}
}
