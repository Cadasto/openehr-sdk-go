package simplified

// REQ-053 — what the codecs derive from a Web Template alone is built once per
// Web Template and kept for every later call with it. REQ-106 makes the tree
// webtemplate.Build returns read-only, which is what lets calls share it.

import (
	"runtime"
	"sync"
	"weak"

	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
)

// webTemplateTables holds what the codecs derive from one Web Template's tree
// alone. Each table is written while it is built and only read after, so the
// calls that share a Web Template, on any goroutine, read the same tables. It
// holds the tree's nodes but no pointer to the Web Template itself, so the
// weak key of [webTemplateTablesCache] can die.
type webTemplateTables struct {
	// tree is the tree the tables were built from.
	tree *webtemplate.Node
	// sib is the tree's reused siblings ([readReusedSiblings]), which encode
	// and decode both read.
	sib *reusedSiblings
	// ranks ([templateRanks]), ism ([ismTransitionNodes]) and archetypeIDs
	// ([readArchetypeRootIDs]) are read by decode alone, so each is built on
	// the first decode that reads it, and an encode does not pay for it.
	ranks        func() map[string]int
	ism          func() ismTables
	archetypeIDs func() map[string]bool
}

// ismTables is the result of [ismTransitionNodes]: the tree's ISM_TRANSITION
// nodes, and whether it has any.
type ismTables struct {
	nodes ismNodes
	found bool
}

// newWebTemplateTables builds the tables of tree, which may be nil.
func newWebTemplateTables(tree *webtemplate.Node) *webTemplateTables {
	sib := readReusedSiblings(tree)
	return &webTemplateTables{
		tree:  tree,
		sib:   sib,
		ranks: sync.OnceValue(func() map[string]int { return templateRanks(tree, sib) }),
		ism: sync.OnceValue(func() ismTables {
			nodes, found := ismTransitionNodes(tree, sib)
			return ismTables{nodes: nodes, found: found}
		}),
		archetypeIDs: sync.OnceValue(func() map[string]bool { return readArchetypeRootIDs(tree) }),
	}
}

// webTemplateTablesCache holds the tables [newWebTemplateTables] built for each
// Web Template, for as long as the Web Template lives. It is keyed by a weak
// pointer to the Web Template, so the cache does not keep one alive, and the
// entry is dropped once it is collected.
var webTemplateTablesCache sync.Map // weak.Pointer[webtemplate.WebTemplate] → *webTemplateTables

// tablesFor returns the tables of wt, building them on the first call with wt
// ([webTemplateTablesCache]). Two first calls at once may both build them; the
// one stored first wins, and the other is discarded.
//
// A built Web Template is read-only. Where wt's tree is no longer the one its
// tables were built from, the tree was replaced against that rule: the tables
// are built again for this call alone and not stored, so the call still reads
// the tree it is given.
func tablesFor(wt *webtemplate.WebTemplate) *webTemplateTables {
	if wt == nil {
		return newWebTemplateTables(nil)
	}
	key := weak.Make(wt)
	x, ok := webTemplateTablesCache.Load(key)
	if !ok {
		var loaded bool
		x, loaded = webTemplateTablesCache.LoadOrStore(key, newWebTemplateTables(wt.Tree))
		if !loaded {
			runtime.AddCleanup(wt, func(key weak.Pointer[webtemplate.WebTemplate]) { webTemplateTablesCache.Delete(key) }, key)
		}
	}
	tables := x.(*webTemplateTables)
	if tables.tree != wt.Tree {
		return newWebTemplateTables(wt.Tree)
	}
	return tables
}
