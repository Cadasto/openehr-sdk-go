package simplified

// REQ-053 — the shared engine both codecs use: walk the Web Template and map
// each node's FLAT id-path to its canonical aqlPath, RM type, and
// repeat/context flags. FLAT and STRUCTURED share this identifier grammar.

import "github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"

// wtEntry is one Web Template node indexed by its FLAT id-path.
type wtEntry struct {
	flatPath   string // id-chain from the tree root, joined by "/"
	aqlPath    string // canonical RM path (openehr/rm/rmpath grammar)
	rmType     string
	repeatable bool // Max != 1 — instances take a :index in FLAT paths
	isContext  bool // composition/context metadata (ctx/ prefix)
	node       *webtemplate.Node
}

// contextID is the set of Web Template ids that carry composition-level
// metadata under the ctx/ prefix in the simplified formats (spec §Context).
// Task 6 refines this against the reference; the static set is sufficient for
// the index engine and covers the COMPOSITION / EVENT_CONTEXT / EVENT
// inContext leaves EHRbase emits.
var contextID = map[string]bool{
	"language": true, "territory": true, "composer": true,
	"setting": true, "start_time": true, "time": true,
	"subject": true, "encoding": true,
}

// indexTemplate walks wt and returns entries keyed by FLAT id-path (without
// any :index — indices are matched at value time). Sibling ids are unique
// (webtemplate.Build rejects collisions), so the keys are unique too.
func indexTemplate(wt *webtemplate.WebTemplate) (map[string]*wtEntry, error) {
	if wt == nil || wt.Tree == nil {
		return nil, ErrNoTemplate
	}
	idx := make(map[string]*wtEntry)
	var walk func(n *webtemplate.Node, parentPath string, inCtx bool)
	walk = func(n *webtemplate.Node, parentPath string, inCtx bool) {
		flatPath := n.ID
		if parentPath != "" {
			flatPath = parentPath + "/" + n.ID
		}
		ctx := inCtx || contextID[n.ID]
		idx[flatPath] = &wtEntry{
			flatPath:   flatPath,
			aqlPath:    n.AQLPath,
			rmType:     n.RMType,
			repeatable: n.Max != 1,
			isContext:  ctx,
			node:       n,
		}
		for _, ch := range n.Children {
			walk(ch, flatPath, ctx)
		}
	}
	walk(wt.Tree, "", false)
	return idx, nil
}
