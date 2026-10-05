// Package rmroots names the openEHR Reference Model classes whose objects
// are always archetype roots.
//
// An object of such a class must carry archetype_details: the class
// invariant Is_archetype_root fixes is_archetype_root true, and LOCATABLE's
// Archetyped_valid invariant then requires the details. The RM floor in
// openehr/validation reports an object that lacks them, and the instance
// generator in openehr/instance refuses to build one the template names no
// archetype for. Both read the class list from here, so they cannot drift
// apart. The package uses the standard library only, so the generator can
// import it without depending on the validator.
package rmroots

// IsArchetypeRoot reports whether rmType is a concrete RM class whose
// objects are always archetype roots. It is a closed list of the classes
// whose BMM definition declares the Is_archetype_root invariant
// (COMPOSITION, EHR_ACCESS, EHR_STATUS) or inherits it from a declaring
// abstract class (PARTY: PERSON, ORGANISATION, GROUP, AGENT, ROLE; ENTRY:
// ADMIN_ENTRY, OBSERVATION, EVALUATION, INSTRUCTION, ACTION). The rminfo
// package does not expose invariants, so the list is written out and a test
// pins it to the vendored BMM: a BMM bump that adds a root class fails that
// test until the class is added here.
//
// rmType is a bare BMM class name, matched exactly. An abstract class
// (PARTY, ENTRY), a generic instantiation or any other name gives false.
func IsArchetypeRoot(rmType string) bool {
	switch rmType {
	case "COMPOSITION", "EHR_ACCESS", "EHR_STATUS",
		"PERSON", "ORGANISATION", "GROUP", "AGENT", "ROLE",
		"ADMIN_ENTRY", "OBSERVATION", "EVALUATION", "INSTRUCTION", "ACTION":
		return true
	}
	return false
}
