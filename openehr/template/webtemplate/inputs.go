package webtemplate

import "github.com/cadasto/openehr-sdk-go/openehr/templatecompile"

// inputsFor maps a compiled value node's primitive constraint to the
// WebTemplate inputs for the core clinical datatype subset (REQ-106).
//
// TODO(Task 7): implement the per-datatype mapping. Returns nil for now so
// the tree structure can be validated independently of inputs.
func inputsFor(v *templatecompile.CompiledNode) []Input {
	_ = v
	return nil
}
