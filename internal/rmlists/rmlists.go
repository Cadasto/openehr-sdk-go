// Package rmlists names the optional openEHR Reference Model lists that
// canonical JSON writes as an empty array when they are present and empty.
//
// Canonical JSON leaves out any other optional list when it is empty, nil or
// not. The lists named here are the item-structure lists an archetype can make
// mandatory by existence: an AOM 1.4 C_MULTIPLE_ATTRIBUTE's existence says
// whether the list is there and its cardinality how many members it holds, so
// a structure with no members still carries its list, and a server may refuse
// one that leaves it out. The BMM code generator reads this list to tag those
// fields, and the instance generator in openehr/instance reads it to write a
// present empty list where canonical JSON carries one. The package imports
// nothing, so both can depend on it.
package rmlists

// KeepsEmpty reports whether the list attribute attr of the RM class class is
// written to canonical JSON as [] when it is present and empty, rather than
// left out. It is a closed list: ITEM_LIST.items, ITEM_TREE.items and
// ITEM_TABLE.rows. Each is an optional list its own concrete class declares,
// with no class invariant that keeps it non-empty, and a test pins the list to
// the vendored BMM.
//
// class is a bare BMM class name and attr a BMM attribute name, both matched
// exactly. Any other pair gives false.
func KeepsEmpty(class, attr string) bool {
	switch class {
	case "ITEM_LIST", "ITEM_TREE":
		return attr == "items"
	case "ITEM_TABLE":
		return attr == "rows"
	}
	return false
}
