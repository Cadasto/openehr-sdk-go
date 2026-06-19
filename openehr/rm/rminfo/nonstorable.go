package rminfo

// IsNonStorableAttr reports whether attrName on parentRMType is a BMM
// function (computed) attribute that is not persisted on the wire — so
// the example synthesiser must not emit it and the validator must not
// require it. The generator (instance) and the validator share this
// one predicate so they cannot drift: if one skips an attribute the
// other still demands, a synthesised tree fails its own validation.
//
//   - EVENT.offset (POINT_EVENT/INTERVAL_EVENT) — derived from the
//     event time and the owning HISTORY origin.
//   - DV_QUANTITY/DV_PROPORTION.is_integral — a computed boolean.
func IsNonStorableAttr(parentRMType, attrName string) bool {
	switch parentRMType {
	case "POINT_EVENT", "INTERVAL_EVENT":
		return attrName == "offset"
	case "DV_PROPORTION", "DV_QUANTITY":
		return attrName == "is_integral"
	}
	return false
}
