// Package template holds what the SDK's ADL 1.4 operational template
// (OPT) tooling shares: the openEHR path grammar and the small value
// types a compiled template hands out.
//
// The package keeps:
//
//   - [Path] and the path grammar, read by [ParsePath];
//   - [Cardinality] with [Single] and [Multiple], and [Multiplicity]
//     with [NewMultiplicity], which closes an [IntervalBounds];
//   - [ArchetypeTerm], [TermBinding] and [CodedTermRef], the term records
//     of an archetype;
//   - the errors [ErrPathSyntax], [ErrPathNotFound] and [ErrAmbiguousPath];
//   - the constraints subpackage, the typed primitive constraints and
//     archetype-slot rules, and the webtemplate subpackage, the Web
//     Template export of a compiled template.
//
// To read an OPT, call aom14.ReadOperationalTemplate (package
// openehr/aom/aom14), which returns the generated operational template
// model, then compile it with templatecompile.Compile (package
// openehr/templatecompile). Look a path up in the compiled template with
// Compiled.Resolve or Compiled.ValidatePath, passing the [Path] that
// [ParsePath] returns and, to refuse an ambiguous segment,
// templatecompile.WithStrictPaths.
//
// The OPT parser ([ParseOPT], [ParseOPTStrict], [ParseFile] and
// [ParseFileStrict]), the node tree it builds ([OperationalTemplate],
// [Node], [ObjectNode], [ComplexObject], [ArchetypeRoot], [Attribute],
// [Slot], [Annotation] and [Description]), [OperationalTemplate.NodeAt],
// [OperationalTemplate.ValidatePath], [OperationalTemplate.ParsePath],
// [ResolveOption] and [WithStrictPaths] are deprecated. They keep
// working, with the behaviour documented on each, until a later release
// removes them. So do the parser's errors, [ErrInvalidOPT],
// [ErrNotOPTFile] and [ErrUnsupportedNode].
//
// An OPT is an XML artifact with root element <template> in namespace
// http://schemas.openehr.org/v1 (the Ocean Template Designer XSD form),
// typically with the .opt filename suffix. Authoring-time templates
// (.oet) are out of scope. ADL 2 operational templates, archetype-slot
// linkage against a remote repository, and terminology expansion are
// not supported.
//
// In openEHR terminology, "template" without qualification often means
// the authoring OET; in this SDK, "template" means operational
// template (OPT) unless stated otherwise. The package name template
// aligns with the openEHR REST Definition API "template" resource;
// this package operates on deployment artifacts locally without HTTP.
// Upload to a CDR uses openehr/client/definition.
//
// This package is stdlib-only. It does not depend on transport/, auth/,
// openehr/client/*, openehr/rm/, or openehr/aom/aom14/.
package template
