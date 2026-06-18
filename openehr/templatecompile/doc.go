// Package templatecompile is the public bridge that turns a parsed ADL
// 1.4 operational template into the compiled form consumed by the
// composition builder, the RM instance synthesiser, the validator, and
// the AQL static lint — REQ-111.
//
// It exists because those entry points
// ([github.com/cadasto/openehr-sdk-go/openehr/composition.NewBuilder],
// [github.com/cadasto/openehr-sdk-go/openehr/instance.Generate],
// [github.com/cadasto/openehr-sdk-go/openehr/validation.Validate] and
// siblings) take a compiled template that, before REQ-111, was only
// constructable from inside this module (it lived in an internal/
// package). This package re-exports the constructor so a different
// module can drive the whole pipeline from a public API:
//
//	opt, _ := template.ParseFile("encounter.opt")   // openehr/template
//	c, _ := templatecompile.Compile(opt)            // this package
//	b, _ := composition.NewBuilder(ctx, c,          // openehr/composition
//	    composition.WithTerritory("NL"),
//	    composition.WithComposer(composer))
//	comp, _ := b.Build()
//	res := validation.ValidateComposition(comp, c)  // openehr/validation
//
// # Public surface
//
// The committed surface is [Compile], the [Compiled] handle, its
// introspection tree ([CompiledNode] / [CompiledAttribute]), the
// functional [Option]s, and the [ErrInvalidInput] / [ErrPathNotFound]
// sentinels. All three types are aliases of the engine's compiled form,
// so values returned by [Compile] are accepted as-is by the consuming
// packages with no conversion, and the tree is fully navigable by
// downstream code (form generation, path discovery, custom mapping —
// walk [Compiled.Root] / [Compiled.NodeAt] → [CompiledNode.Attributes]
// → [CompiledAttribute.Children]).
//
// Pre-1.0, the one area expected to change is multi-language term
// resolution: [CompiledNode.Term]'s lang parameter is accepted but
// currently ignored (single-document-language OPTs, REQ-105).
//
// # Why not openehr/template
//
// The natural home would be openehr/template (next to [template.ParseFile]),
// but two constraints rule it out and ADR 0010 records the decision:
//   - the compile engine imports openehr/template, so hosting Compile there
//     would create an import cycle; and
//   - REQ-100 mandates openehr/template stay stdlib-only, whereas
//     compilation needs openehr/rm/rminfo for implicit-attribute injection.
//
// # REQ-013 building-block independence
//
// This package imports openehr/template and openehr/rm/rminfo only (plus
// the internal compile engine). It does NOT import transport/, auth/,
// openehr/client/*, or openehr/serialize/.
package templatecompile
