package validation

// inventory_dispatch.go: REQ-171 *Where the RM rules come from*. The
// invariant inventory (inventory_gen.go) is generated from the pinned RM and
// BASE BMMs and classified in invariants.json; this file declares the kinds
// that classification gives an invariant.

// invariantKind is how invariants.json classifies an invariant of the
// inventory.
type invariantKind uint8

const (
	// invariantEvaluated is an invariant the floor evaluates, with the Go
	// function that evaluates it.
	invariantEvaluated invariantKind = iota + 1
	// invariantNotMechanical is an invariant no check of the instance alone
	// can decide: it needs a terminology service, stored history, or a
	// comparison the RM leaves undefined.
	invariantNotMechanical
	// invariantOutOfScope is an invariant the SDK does not check on an
	// instance, for the reason the classification gives.
	invariantOutOfScope
)

// inventoryEntry is the classification of one invariant: its kind and, for
// an evaluated one, the name of the Go function that evaluates it.
type inventoryEntry struct {
	kind     invariantKind
	function string
}
