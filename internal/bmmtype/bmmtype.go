// Package bmmtype reads the type names that openEHR BMM schemas and
// operational templates write as text, such as "DV_INTERVAL<DV_QUANTITY>".
//
// A generic type name is a class name followed, in angle brackets, by the
// actual generic parameters. Lookups keyed by class name need the bare class
// ("DV_INTERVAL"), and an attribute that its class types with a formal
// parameter ("T") needs the actual parameter the name supplies
// ("DV_QUANTITY"). This package answers both from the text alone.
package bmmtype

import (
	"slices"
	"strings"
	"unicode"
)

// Split parses a BMM type name into its class name and its actual generic
// parameters, in order. "DV_INTERVAL<DV_QUANTITY>" gives "DV_INTERVAL" and
// ["DV_QUANTITY"]; "Hash<String, String>" gives "Hash" and two "String"
// parameters; a nested parameter stays whole, so "A<B<C>>" gives "A" and
// ["B<C>"]. A name without parameters gives itself and nil. White space
// (any Unicode space) around the name and around each part is ignored.
//
// ok is false when name is not a well-formed type name: it is empty, a class
// name or a parameter is missing or blank, a class name has white space
// inside it ("DV INTERVAL"), the angle brackets do not balance, or text
// follows the closing bracket. Split runs in linear time in the length of
// the name, however deeply the parameters nest.
func Split(name string) (class string, params []string, ok bool) {
	name = strings.TrimSpace(name)
	if !wellFormed(name) {
		return "", nil, false
	}
	open := strings.IndexByte(name, '<')
	if open < 0 {
		return name, nil, true
	}
	// wellFormed guarantees the name ends with the bracket that closes open.
	inner := name[open+1 : len(name)-1]
	depth, start := 0, 0
	for i := range len(inner) {
		switch inner[i] {
		case '<':
			depth++
		case '>':
			depth--
		case ',':
			if depth == 0 {
				params = append(params, strings.TrimSpace(inner[start:i]))
				start = i + 1
			}
		}
	}
	params = append(params, strings.TrimSpace(inner[start:]))
	return strings.TrimSpace(name[:open]), params, true
}

// Class returns the class name of a BMM type name, without its generic
// parameters: "DV_INTERVAL<DV_QUANTITY>" gives "DV_INTERVAL", and a name
// without parameters comes back with surrounding white space removed. A name
// that is not well formed (see [Split]) is returned unchanged, so a lookup
// keyed by class name misses it rather than matching a guess.
func Class(name string) string {
	class, _, ok := Split(name)
	if !ok {
		return name
	}
	return class
}

// Substitute returns the type an attribute has on a value of the type named
// owner, given the type that the attribute's class declares. When declared is
// one of the class's formal generic parameters and owner supplies it, the
// supplied type is returned: on owner "DV_INTERVAL<DV_QUANTITY>", the lower
// bound that DV_INTERVAL declares as "T" is a "DV_QUANTITY". Otherwise
// declared is returned unchanged, including when owner names the class
// without parameters, since no actual type is then known.
func Substitute(owner, declared string) string {
	class, params, ok := Split(owner)
	if !ok || len(params) == 0 {
		return declared
	}
	formals := formalParameters[class]
	if len(formals) != len(params) {
		return declared
	}
	if i := slices.Index(formals, declared); i >= 0 {
		return params[i]
	}
	return declared
}

// formalParameters lists, in declaration order, the formal generic parameters
// of every generic class in the class universe of openehr/rm/rminfo, the
// lookup whose attribute types Substitute resolves. rminfo records an
// attribute typed by a parameter under the parameter's name ("T") and does not
// carry the parameter list itself, so it is kept here. The classes and names
// come from the RM 1.2.0 and BASE 1.3.0 schemas under resources/bmm, and a
// test holds this table to them.
var formalParameters = map[string][]string{
	"DV_INTERVAL":      {"T"},
	"EVENT":            {"T"},
	"HISTORY":          {"T"},
	"IMPORTED_VERSION": {"T"},
	"INTERVAL_EVENT":   {"T"},
	"ORIGINAL_VERSION": {"T"},
	"POINT_EVENT":      {"T"},
	"Point_interval":   {"T"},
	"Proper_interval":  {"T"},
	"REFERENCE_RANGE":  {"T"},
	"VERSION":          {"T"},
	"VERSIONED_OBJECT": {"T"},
}

// wellFormed reports whether s is exactly one type name: a class name,
// optionally followed by a bracketed, comma-separated list of type names. White
// space is what [unicode.IsSpace] reports, the same set [strings.TrimSpace]
// strips, so a part that is blank once trimmed is refused here. It makes a
// single pass over s.
func wellFormed(s string) bool {
	depth := 0
	named := false  // the type being read has a class name
	spaced := false // white space followed that class name
	closed := false // the type being read has had its parameter list closed
	for _, r := range s {
		switch {
		case r == '<':
			if !named || closed {
				return false
			}
			depth++
			named, spaced = false, false
		case r == ',':
			if !named || depth == 0 {
				return false
			}
			named, spaced, closed = false, false, false
		case r == '>':
			if !named || depth == 0 {
				return false
			}
			// Back in the enclosing type, which already has its name and has
			// now had its parameter list closed.
			depth--
			closed = true
		case unicode.IsSpace(r):
			spaced = named
		default:
			// A class name is one unbroken run: nothing may follow a closed
			// parameter list, and no white space may split the name.
			if closed || spaced {
				return false
			}
			named = true
		}
	}
	return named && depth == 0
}
