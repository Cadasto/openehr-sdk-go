// Package templatecompile turns an ADL 1.4 operational template, read into
// the operational template model of
// [github.com/cadasto/openehr-sdk-go/openehr/aom/aom14], into the compiled
// form consumed by the composition builder, the RM instance synthesiser, the
// validator, and the AQL static lint.
//
// Those entry points
// ([github.com/cadasto/openehr-sdk-go/openehr/composition.NewBuilder],
// [github.com/cadasto/openehr-sdk-go/openehr/instance.Generate],
// [github.com/cadasto/openehr-sdk-go/openehr/validation.Validate] and
// siblings) take a compiled template. The compile engine lives in an
// internal package, so this package exposes its constructor and lets any
// module drive the whole pipeline from a public API:
//
//	f, _ := os.Open("encounter.opt")
//	opt, _ := aom14.ReadOperationalTemplate(f)     // openehr/aom/aom14
//	c, _ := templatecompile.Compile(opt)           // this package
//	b, _ := composition.NewBuilder(ctx, c,         // openehr/composition
//	    composition.WithTerritory("NL"),
//	    composition.WithComposer(composer))
//	comp, _ := b.Build()
//	res := validation.ValidateComposition(comp, c) // openehr/validation
//	valid := res.OK                                // template and RM floor
//
// [github.com/cadasto/openehr-sdk-go/openehr/aom/aom14.ReadOperationalTemplate]
// reads an untrusted document within size and depth limits; Compile holds a
// model read or built some other way to the same depth.
//
// ValidateComposition checks the template constraints and the Reference
// Model floor in one call, reporting a defect both find once.
//
// # Public surface
//
// The public surface is [Compile], the [Compiled] handle, its
// introspection tree ([CompiledNode] / [CompiledAttribute]), the path
// lookup ([Compiled.Resolve] / [Compiled.ValidatePath], which take a path
// that [github.com/cadasto/openehr-sdk-go/openehr/template.ParsePath] reads
// and are tuned by a [ResolveOption] such as [WithStrictPaths]), the
// functional [Option]s, and the [ErrInvalidInput] / [ErrPathNotFound]
// sentinels. The types are aliases of the engine's compiled form, so values
// returned by [Compile] are accepted as-is by the consuming packages with no
// conversion. Downstream code (form generation, path discovery, custom
// mapping) can walk the whole tree: [Compiled.Root] / [Compiled.NodeAt] /
// [Compiled.Resolve] → [CompiledNode.Attributes] →
// [CompiledAttribute.Children].
//
// Before 1.0, the one area expected to change is multi-language term
// resolution. [CompiledNode.Term] accepts a lang parameter for forward
// compatibility, but OPTs currently carry a single document language and
// every lang resolves to the document-language term. Multi-language
// selection, with document-language fallback, is planned before 1.0.
//
// # Why not openehr/template
//
// Compile does not live in openehr/template for two reasons:
//   - the compile engine imports openehr/template, so hosting Compile there
//     would create an import cycle; and
//   - openehr/template stays stdlib-only, whereas compilation needs
//     openehr/rm/rminfo for implicit-attribute injection and the model in
//     openehr/aom/aom14.
//
// # Dependencies
//
// This package imports openehr/aom/aom14 and openehr/rm/rminfo only (plus
// the internal compile engine, which also imports openehr/template for the
// types the compiled tree hands out). It does not import transport/, auth/
// or openehr/client/*, and its own files do not import openehr/serialize/.
package templatecompile
