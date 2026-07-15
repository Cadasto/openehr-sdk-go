package simplified

// REQ-053 — FLAT encode: project an *rm.Composition into the FLAT map by
// walking the Web Template tree and resolving each node's value against the
// composition via openehr/rm/rmpath. The Web Template already carries the
// level-removed id-chain (FLAT path) and each node's canonical aqlPath, so
// the walk needs no separate flattening engine.

import (
	"encoding/json"
	"errors"
	"fmt"
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
	emitContext(out, comp)
	root := wt.Tree
	for _, ch := range root.Children {
		if err := emitNode(out, ch, root.ID, comp, root.AQLPath); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// emitContext writes composition-level metadata under the ctx/ prefix (REQ-053):
// the mandatory language and territory code strings, the composer, and the
// context start time. Setting, category, participations, health-care facility,
// workflow ids, and the composer external reference are deferred (they are
// platform defaults or need terminology resolution) — see deviations.md.
func emitContext(out map[string]any, comp *rm.Composition) {
	if comp.Language.CodeString != "" {
		out["ctx/language"] = comp.Language.CodeString
	}
	if comp.Territory.CodeString != "" {
		out["ctx/territory"] = comp.Territory.CodeString
	}
	switch c := comp.Composer.(type) {
	case *rm.PartySelf:
		if c != nil {
			out["ctx/composer_self"] = true
		}
	case rm.PartySelf:
		out["ctx/composer_self"] = true
	case *rm.PartyIdentified:
		if c != nil && c.Name != nil && *c.Name != "" {
			out["ctx/composer_name"] = *c.Name
		}
	case rm.PartyIdentified:
		if c.Name != nil && *c.Name != "" {
			out["ctx/composer_name"] = *c.Name
		}
	}
	if comp.Context != nil && comp.Context.StartTime.Value != "" {
		out["ctx/time"] = comp.Context.StartTime.Value
	}
}

// emitNode resolves node against resolveRoot (whose canonical path is
// resolveRootAql) and writes FLAT entries under flatPrefix. A repeating node
// enumerates its instances and stamps a zero-based :index; a container
// recurses into its children; a value leaf maps its datatype to suffix keys.
// Absent optional nodes resolve to nothing and are silently skipped.
func emitNode(out map[string]any, node *webtemplate.Node, flatPrefix string, resolveRoot rm.Locatable, resolveRootAql string) error {
	isContainer := len(node.Children) > 0
	// A value leaf normally carries input descriptors, but the Web Template emits
	// none for some datatypes (DV_URI, DV_MULTIMEDIA, DV_PARSABLE, …); any childless
	// DV_* node is still a value leaf and must be emitted (bare/suffixed or |raw),
	// not dropped.
	isLeaf := !isContainer && (len(node.Inputs) > 0 || strings.HasPrefix(node.RMType, "DV_"))
	if !isContainer && !isLeaf {
		return nil // structural node carrying neither children nor value inputs
	}
	relPath := strings.TrimPrefix(node.AQLPath, resolveRootAql)

	if node.Max != 1 {
		vals, err := rmpath.ItemsAtPath(resolveRoot, relPath)
		if err != nil {
			return skipNotFound(err, relPath)
		}
		for i, v := range vals {
			if err := emitValue(out, node, flatPrefix+"/"+node.ID+":"+strconv.Itoa(i), v, isContainer, resolveRoot, resolveRootAql); err != nil {
				return err
			}
		}
		return nil
	}
	v, err := rmpath.ItemAtPath(resolveRoot, relPath)
	if err != nil {
		return skipNotFound(err, relPath)
	}
	return emitValue(out, node, flatPrefix+"/"+node.ID, v, isContainer, resolveRoot, resolveRootAql)
}

// skipNotFound treats an absent optional node (ErrPathNotFound) as a no-op,
// but surfaces real faults — a malformed path (ErrPathSyntax) or a Max==1
// node that resolves to multiple items (ErrPathAmbiguous) — rather than
// silently dropping data.
func skipNotFound(err error, relPath string) error {
	if errors.Is(err, rmpath.ErrPathNotFound) {
		return nil
	}
	return fmt.Errorf("simplified: resolve %q: %w", relPath, err)
}

// emitValue recurses into a container instance or maps a leaf value.
// ancestorRoot/ancestorAql are the enclosing Locatable resolution root against
// which node was resolved; they carry through when a container is not itself
// Locatable (e.g. EVENT_CONTEXT), so its children resolve from that ancestor by
// their full relative paths rather than being dropped.
func emitValue(out map[string]any, node *webtemplate.Node, flatPath string, v any, isContainer bool, ancestorRoot rm.Locatable, ancestorAql string) error {
	if isContainer {
		root, rootAql := ancestorRoot, ancestorAql
		if loc, ok := v.(rm.Locatable); ok {
			// A Locatable container becomes the new resolution root for its
			// children (each repeatable instance is resolved independently).
			root, rootAql = loc, node.AQLPath
		}
		for _, ch := range node.Children {
			if err := emitNode(out, ch, flatPath, root, rootAql); err != nil {
				return err
			}
		}
		return nil
	}
	return leafToFlat(out, flatPath, v, node.RMType, leafListOpen(node))
}
