package simplified

// REQ-053 § Leaf datatypes, the STRING row: the Web Template gives one RM
// String attribute a leaf of its own, the in-context ACTIVITY
// `action_archetype_id`, and FLAT carries it as a bare JSON string at the
// leaf's key, with no suffix.

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rmpath"
)

// stringLeafType is the Web Template's RM type for a leaf that holds an RM
// String attribute rather than a DataValue.
const stringLeafType = "STRING"

// emitStringLeaf resolves a STRING leaf and writes it at flatPath through
// [leafToFlat], which spells it as its bare value. relPath is the leaf's
// canonical path relative to root.
//
// rmpath resolves the LOCATABLE tree and the DataValue slots in it, not RM
// String attributes, so the value is read off the object that owns the
// attribute. A STRING leaf on an attribute this codec does not read is refused
// rather than skipped, because whether it holds a value cannot be told.
func emitStringLeaf(out map[string]any, flatPath string, root rm.Locatable, relPath string) error {
	ownerRel, attr := "", relPath
	if i := strings.LastIndexByte(relPath, '/'); i >= 0 {
		ownerRel, attr = relPath[:i], relPath[i+1:]
	}
	var owner any = root
	if ownerRel != "" {
		o, err := rmpath.ItemAtPath(root, ownerRel)
		if err != nil {
			return skipNotFound(err, ownerRel)
		}
		owner = o
	}
	s, known := rmStringAttr(owner, attr)
	if !known {
		return fmt.Errorf("%w: %q is a %s leaf on %T.%s, which is no RM String attribute this codec reads",
			ErrUnsupportedDatatype, flatPath, stringLeafType, owner, attr)
	}
	return leafToFlat(out, flatPath, s, stringLeafType, false)
}

// rmStringAttr reads the RM String attribute attr off owner. known is false
// when owner has no String attribute of that name this codec reads. The Web
// Template gives exactly one String attribute a leaf, ACTIVITY
// `action_archetype_id`, so that is the one read here.
func rmStringAttr(owner any, attr string) (s string, known bool) {
	if a, ok := as[rm.Activity](owner); ok && attr == "action_archetype_id" {
		return a.ActionArchetypeID, true
	}
	return "", false
}

// leafFromSuffixes builds the canonical-JSON value of a Web Template leaf from
// its FLAT suffix->value map: the RM String itself for a STRING leaf, and the
// DataValue [dvFromSuffixes] builds for every other leaf type.
func leafFromSuffixes(rmType string, listOpen bool, sfx map[string]any) (any, error) {
	if rmType == stringLeafType {
		return stringFromSuffixes(sfx)
	}
	return dvFromSuffixes(rmType, listOpen, sfx)
}

// stringFromSuffixes rebuilds a STRING leaf's RM String from its bare value.
// Any suffix, |raw included, is ErrUnsupportedDatatype: the leaf carries a bare
// value only. A bare value that is not a JSON string is a malformed body, not a
// datatype this codec declines to model, so it carries no gap sentinel (the
// [applyOrderedSuffixes] rule).
func stringFromSuffixes(sfx map[string]any) (any, error) {
	for _, k := range slices.Sorted(maps.Keys(sfx)) {
		if k != "" {
			return nil, fmt.Errorf("%w: unexpected |%s for %s, which carries a bare value only",
				ErrUnsupportedDatatype, k, stringLeafType)
		}
	}
	v, err := requireSuffix(stringLeafType, sfx, "")
	if err != nil {
		return nil, err
	}
	s, ok := v.(string)
	if !ok {
		return nil, fmt.Errorf("the %s bare value must be a string, got %T", stringLeafType, v)
	}
	return s, nil
}
