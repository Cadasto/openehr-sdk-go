package rminfo

import "slices"

// NonEmptyList is a list attribute that an invariant of the pinned RM keeps
// from being present and empty: once the list is there, it holds an item.
// The template-driven generator and the validator both read these lists, so
// that what one builds the other accepts.
type NonEmptyList struct {
	// Class is the class that declares the invariant, a bare class name.
	Class string
	// Invariant is the invariant's name, as the pinned BMM declares it on
	// Class.
	Invariant string
	// List is the list attribute of Class that the invariant reads.
	List string
	// Alternative is the attribute of Class whose presence keeps the
	// invariant while List holds no item, or "" where nothing but an item
	// of List keeps it.
	Alternative string
}

// nonEmptyLists is the table NonEmptyLists copies, sorted by class and
// invariant.
var nonEmptyLists = []NonEmptyList{
	{Class: "ATTESTATION", Invariant: "Items_valid", List: "items"},
	{Class: "COMPOSITION", Invariant: "Content_valid", List: "content"},
	{Class: "EVENT_CONTEXT", Invariant: "Participations_validity", List: "participations"},
	{Class: "HISTORY", Invariant: "Events_valid", List: "events", Alternative: "summary"},
	{Class: "INSTRUCTION", Invariant: "Activities_valid", List: "activities"},
	{Class: "SECTION", Invariant: "Items_valid", List: "items"},
}

// NonEmptyLists returns the lists the RM keeps non-empty once present, one
// entry per invariant, sorted by class and invariant. Five of the six
// invariants read `x /= Void implies not x.is_empty` for their list x:
// ATTESTATION.Items_valid (items), COMPOSITION.Content_valid (content),
// EVENT_CONTEXT.Participations_validity (participations),
// INSTRUCTION.Activities_valid (activities) and SECTION.Items_valid
// (items), so a list that is present holds an item and an absent one keeps
// the invariant. HISTORY.Events_valid reads `(events /= Void and then not
// events.is_empty) or summary /= Void`, so a HISTORY carries an event or a
// summary: its entry names summary as the Alternative.
//
// Each call returns a fresh slice, which the caller may change.
func NonEmptyLists() []NonEmptyList { return slices.Clone(nonEmptyLists) }

// NonEmptyListOf returns the entry of [NonEmptyLists] whose Class is class
// and whose List is attrName, and reports whether there is one. class is a
// bare class name, matched as written: a class that only inherits from
// one of the six has no entry.
func NonEmptyListOf(class, attrName string) (NonEmptyList, bool) {
	for _, l := range nonEmptyLists {
		if l.Class == class && l.List == attrName {
			return l, true
		}
	}
	return NonEmptyList{}, false
}
