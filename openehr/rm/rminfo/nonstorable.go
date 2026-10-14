package rminfo

// IsNonStorableAttr reports whether attrName on parentRMType is an
// attribute that no serialization carries, so the example synthesiser
// must not emit it and the validator must not require it. The generator
// (instance) and the validator share this one predicate so they cannot
// drift: if one skips an attribute the other still demands, a
// synthesised tree fails its own validation.
//
// It names the BMM function (computed) attributes, which are not
// persisted on the wire:
//
//   - EVENT.offset (POINT_EVENT/INTERVAL_EVENT): derived from the
//     event time and the owning HISTORY origin.
//   - DV_QUANTITY/DV_PROPORTION.is_integral: a computed boolean.
//
// Besides those, it names one RM-mandatory back-reference:
//
//   - RESOURCE_DESCRIPTION.parent_resource: the resource that holds the
//     description. It is typed AUTHORED_RESOURCE, a class with no
//     concrete descendant, so no Go value can be held there.
func IsNonStorableAttr(parentRMType, attrName string) bool {
	switch parentRMType {
	case "POINT_EVENT", "INTERVAL_EVENT":
		return attrName == "offset"
	case "DV_PROPORTION", "DV_QUANTITY":
		return attrName == "is_integral"
	case "RESOURCE_DESCRIPTION":
		return attrName == "parent_resource"
	}
	return false
}
