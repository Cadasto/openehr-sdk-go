package rminfo

import "slices"

// IsNonStorableAttr reports whether attrName on parentRMType is an
// attribute that no serialization carries, so the example synthesiser
// must not emit it and the validator must not require it. The generator
// (instance) and the validator share this one predicate so they cannot
// drift: if one skips an attribute the other still demands, a
// synthesised tree fails its own validation.
//
// parentRMType is a bare class name, with no generic parameters. The
// predicate names every function of the class in the pinned BMM: one the
// class or an ancestor declares that is a property neither of the class
// nor of any class descending from it. The model computes a function
// rather than storing it, so a template that constrains one, as some
// constrain EVENT.offset or DV_QUANTITY.is_integral, constrains nothing
// an instance holds. A name is answered for the class asked, and holds
// for every instance the class denotes: magnitude is a function of
// DV_DURATION and a property of DV_QUANTITY, which declares it as one,
// and so not a function of DV_AMOUNT, whose descendant DV_QUANTITY
// stores it.
//
// Besides the functions, it names one RM-mandatory back-reference:
//
//   - RESOURCE_DESCRIPTION.parent_resource: the resource that holds the
//     description. It is typed AUTHORED_RESOURCE, a class with no
//     concrete descendant, so no Go value can be held there.
func IsNonStorableAttr(parentRMType, attrName string) bool {
	if parentRMType == "RESOURCE_DESCRIPTION" && attrName == "parent_resource" {
		return true
	}
	_, found := slices.BinarySearch(functionData[parentRMType], attrName)
	return found
}
