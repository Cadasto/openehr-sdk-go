package aom14

// like_interfaces.go: hand-written declarations of the narrow polymorphic
// interfaces of the AOM 1.4 package, as openehr/rm/like_interfaces.go holds
// the RM's. The generator types a member `<Parent>Like` where its BMM type is
// a concrete class with concrete subclasses, so a value of any of them decodes
// into the member and encodes back with its own type; the interface and its
// marker methods live here.
//
// When the operational template classes subclass a class, or a BMM bump adds
// a concrete subclass, the subclass gains a marker method here, or the
// generated code that assigns it to the member does not compile.

// CComplexObjectLike is the narrow polymorphic interface for
// C_COMPLEX_OBJECT: a member typed C_COMPLEX_OBJECT, such as
// ARCHETYPE.definition, holds a C_COMPLEX_OBJECT or one of the operational
// template classes that extend it, C_ARCHETYPE_ROOT and T_COMPLEX_OBJECT.
// Members: [CComplexObject], [CArchetypeRoot], [TComplexObject].
type CComplexObjectLike interface {
	isCComplexObjectLike()
}

func (CComplexObject) isCComplexObjectLike() {}

func (CArchetypeRoot) isCComplexObjectLike() {}

func (TComplexObject) isCComplexObjectLike() {}

// ArchetypeOntologyLike is the narrow polymorphic interface for
// ARCHETYPE_ONTOLOGY: a member typed ARCHETYPE_ONTOLOGY, such as
// ARCHETYPE.ontology, holds an ARCHETYPE_ONTOLOGY or the
// FLAT_ARCHETYPE_ONTOLOGY of an operational template, which extends it.
// Members: [ArchetypeOntology], [FlatArchetypeOntology].
type ArchetypeOntologyLike interface {
	isArchetypeOntologyLike()
}

func (ArchetypeOntology) isArchetypeOntologyLike() {}

func (FlatArchetypeOntology) isArchetypeOntologyLike() {}
