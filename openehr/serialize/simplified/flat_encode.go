package simplified

// REQ-053 — FLAT encode: project an *rm.Composition into the FLAT map by
// walking the Web Template tree and resolving each node's value against the
// composition via openehr/rm/rmpath. The Web Template already carries the
// level-removed id-chain (FLAT path) and each node's canonical aqlPath, so
// the walk needs no separate flattening engine.

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rmpath"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
)

// MarshalFlat encodes comp as FLAT JSON using wt (REQ-053).
func MarshalFlat(comp *rm.Composition, wt *webtemplate.WebTemplate) ([]byte, error) {
	m, err := encodeFlat(comp, wt)
	if err != nil {
		return nil, err
	}
	return json.Marshal(m)
}

// encodeFlat builds the FLAT path->value map. The root COMPOSITION is the
// resolution root; its FLAT path is the tree id.
func encodeFlat(comp *rm.Composition, wt *webtemplate.WebTemplate) (map[string]any, error) {
	if wt == nil || wt.Tree == nil {
		return nil, ErrNoTemplate
	}
	if comp == nil {
		return nil, ErrNilComposition
	}
	out := make(map[string]any)
	root := wt.Tree
	for _, ch := range root.Children {
		emitNode(out, ch, root.ID, comp, root.AQLPath)
	}
	return out, nil
}

// emitNode resolves node against resolveRoot (whose canonical path is
// resolveRootAql) and writes FLAT entries under flatPrefix. A repeating node
// enumerates its instances and stamps a zero-based :index; a container
// recurses into its children; a value leaf maps its datatype to suffix keys.
// Absent optional nodes resolve to nothing and are silently skipped.
func emitNode(out map[string]any, node *webtemplate.Node, flatPrefix string, resolveRoot rm.Locatable, resolveRootAql string) {
	isContainer := len(node.Children) > 0
	isLeaf := !isContainer && len(node.Inputs) > 0
	if !isContainer && !isLeaf {
		return // structural node carrying neither children nor value inputs
	}
	relPath := strings.TrimPrefix(node.AQLPath, resolveRootAql)

	if node.Max != 1 {
		vals, err := rmpath.ItemsAtPath(resolveRoot, relPath)
		if err != nil {
			return
		}
		for i, v := range vals {
			emitValue(out, node, flatPrefix+"/"+node.ID+":"+strconv.Itoa(i), v, isContainer)
		}
		return
	}
	v, err := rmpath.ItemAtPath(resolveRoot, relPath)
	if err != nil {
		return
	}
	emitValue(out, node, flatPrefix+"/"+node.ID, v, isContainer)
}

// emitValue recurses into a container instance or maps a leaf value.
func emitValue(out map[string]any, node *webtemplate.Node, flatPath string, v any, isContainer bool) {
	if isContainer {
		loc, ok := v.(rm.Locatable)
		if !ok {
			return
		}
		for _, ch := range node.Children {
			emitNode(out, ch, flatPath, loc, node.AQLPath)
		}
		return
	}
	leafToFlat(out, flatPath, v)
}
