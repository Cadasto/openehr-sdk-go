package simplified

// REQ-053 § Leaf datatypes, the STRING row: the Web Template gives one RM
// String attribute a leaf of its own, the in-context ACTIVITY
// `action_archetype_id`, and FLAT carries it as a bare JSON string at the
// leaf's key, with no suffix.

import (
	"errors"
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
// rmpath resolves an RM String attribute it knows to the string itself, held
// by value, so whenever the owner is there the attribute resolves (to "" when
// unset, which writes nothing). The Web Template builder makes one STRING leaf,
// ACTIVITY `action_archetype_id`. A STRING leaf on an attribute rmpath does not
// resolve to an RM String is therefore refused whenever its owner is there,
// because whether it holds a value cannot be told; with no owner there is
// nothing to lose, and it is skipped.
func emitStringLeaf(out map[string]any, flatPath string, root rm.Locatable, relPath string) error {
	v, err := rmpath.ItemAtPath(root, relPath)
	if err == nil {
		return leafToFlat(out, flatPath, v, stringLeafType, false)
	}
	if !errors.Is(err, rmpath.ErrPathNotFound) {
		return fmt.Errorf("simplified: resolve %q: %w", relPath, err)
	}
	ownerRel, attr := "", relPath
	if before, after, ok := strings.CutLast(relPath, "/"); ok {
		ownerRel, attr = before, after
	}
	if ownerRel != "" {
		if _, err := rmpath.ItemAtPath(root, ownerRel); err != nil {
			return skipNotFound(err, ownerRel)
		}
	}
	return fmt.Errorf("%w: %q is a %s leaf on the attribute %q, which is no RM String attribute this codec reads",
		ErrUnsupportedDatatype, flatPath, stringLeafType, attr)
}

// leafFromSuffixes builds the canonical-JSON value of a Web Template leaf from
// its FLAT suffix->value map: the RM String itself for a STRING leaf, and the
// DataValue [dvFromSuffixes] builds for every other leaf type. keyOf names the
// FLAT key each suffix arrived under.
func leafFromSuffixes(rmType string, listOpen bool, sfx map[string]any, keyOf func(suffix string) string) (any, error) {
	if rmType == stringLeafType {
		return stringFromSuffixes(sfx, keyOf)
	}
	return dvFromSuffixes(rmType, listOpen, sfx, keyOf)
}

// stringFromSuffixes rebuilds a STRING leaf's RM String from its bare value.
// Any suffix, |raw included, is ErrUnsupportedDatatype: the leaf carries a bare
// value only. A null bare value never gets here: [leafSuffixes] reads it as
// absent (REQ-053), and a leaf left with nothing is not built. A bare value
// that is not a JSON string does not fit the RM String, and is refused with
// ErrUnsupportedDatatype naming the key (REQ-053). An empty string (REQ-053
// § Leaf datatypes) fits the String but breaks an RM invariant: a malformed
// body, not a datatype this codec declines to model, so it carries no
// sentinel.
func stringFromSuffixes(sfx map[string]any, keyOf func(suffix string) string) (any, error) {
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
	if err := stringLeafFit.check(keyOf, "", v); err != nil {
		return nil, err
	}
	s, _ := v.(string)
	// ACTIVITY's Action_archetype_id_valid invariant forbids an empty String,
	// and encode writes nothing for one, so an empty bare value is refused
	// rather than rebuilt into an invalid ACTIVITY.
	if s == "" {
		return nil, fmt.Errorf("an empty %s, which the RM invariant forbids", stringLeafType)
	}
	return s, nil
}
