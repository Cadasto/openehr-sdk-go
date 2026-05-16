package bmm

import (
	"errors"
	"fmt"
)

// Sentinel errors returned by the loader. They are wrapped with extra
// context (offending name, schema id, etc.) via fmt.Errorf("...: %w", err)
// at call sites; consumers compare with errors.Is.
var (
	// ErrUnknownType is returned when a polymorphic JSON object carries a
	// _type discriminator that the loader's decoder registry does not
	// recognise.
	ErrUnknownType = errors.New("bmm: unknown _type discriminator")

	// ErrMissingField is returned when a required BMM field is absent.
	ErrMissingField = errors.New("bmm: missing required field")

	// ErrSchemaConflict is returned when two schemas (root + an included
	// ancestor) both define the same class.
	ErrSchemaConflict = errors.New("bmm: schema conflict")

	// ErrCircularIncludes is returned when LoadAll detects an include
	// cycle, i.e. schema A includes B which (transitively) includes A.
	ErrCircularIncludes = errors.New("bmm: circular includes")

	// ErrSchemaNotFound is returned when a Resolver cannot produce bytes
	// for a requested schema id.
	ErrSchemaNotFound = errors.New("bmm: schema not found")

	// ErrInvalidShape is returned when a BMM object has an internally
	// inconsistent shape (e.g. a P_BMM_CONTAINER_TYPE with neither
	// type_def nor type set).
	ErrInvalidShape = errors.New("bmm: invalid object shape")
)

// unknownTypeError wraps ErrUnknownType with the offending discriminator
// and the JSON path where it was encountered, so loaders' error messages
// are actionable.
type unknownTypeError struct {
	Discriminator string
	Path          string
}

func (e *unknownTypeError) Error() string {
	return fmt.Sprintf("%s: %q at %s", ErrUnknownType.Error(), e.Discriminator, e.Path)
}

func (e *unknownTypeError) Unwrap() error { return ErrUnknownType }

// schemaConflictError carries the conflicting class name and the two
// schema ids that contributed it.
type schemaConflictError struct {
	ClassName string
	SchemaA   string
	SchemaB   string
}

func (e *schemaConflictError) Error() string {
	return fmt.Sprintf("%s: class %q defined by both %q and %q",
		ErrSchemaConflict.Error(), e.ClassName, e.SchemaA, e.SchemaB)
}

func (e *schemaConflictError) Unwrap() error { return ErrSchemaConflict }

// circularIncludesError records the schema id on which the cycle closes
// and the include chain that led to it.
type circularIncludesError struct {
	SchemaID string
	Chain    []string
}

func (e *circularIncludesError) Error() string {
	return fmt.Sprintf("%s: %q (chain: %v)", ErrCircularIncludes.Error(), e.SchemaID, e.Chain)
}

func (e *circularIncludesError) Unwrap() error { return ErrCircularIncludes }
