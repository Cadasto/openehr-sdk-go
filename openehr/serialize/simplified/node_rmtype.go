package simplified

import (
	"strings"
	"unicode"

	"github.com/cadasto/openehr-sdk-go/internal/bmmtype"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
)

// nodeRMType returns the RM type a Web Template node declares, in the one
// spelling every classification in this codec compares against: the value,
// interval, party and composite leaf predicates, the datatype dispatch in both
// directions, and the container types decode materialises.
//
// It is the only place the codec reads [webtemplate.Node.RMType]. The
// webtemplate package has no parse step of its own (a caller decodes a Web
// Template straight into the struct), so a template exported by another tool
// can carry " DV_INTERVAL<DV_QUANTITY>" or "DV_QUANTITY " exactly as written.
// Compared as written, such a name misses every predicate, and encode read the
// leaf as structure and dropped every key under it without an error.
func nodeRMType(n *webtemplate.Node) string {
	return canonicalRMType(n.RMType)
}

// canonicalRMType removes the white space a well-formed BMM type name carries
// around its class name and its generic parameters, so
// " DV_INTERVAL< DV_QUANTITY >" reads as "DV_INTERVAL<DV_QUANTITY>".
//
// [bmmtype.Split] decides whether the name is well formed, and a well-formed
// name has no white space inside a class name, so every space in it is padding.
// A name that is not well formed ("DV INTERVAL<DV_QUANTITY>") comes back
// unchanged: it matches no predicate, and the encode backstop names it as
// written.
func canonicalRMType(name string) string {
	if !strings.ContainsFunc(name, unicode.IsSpace) {
		return name
	}
	if _, _, ok := bmmtype.Split(name); !ok {
		return name
	}
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, name)
}
