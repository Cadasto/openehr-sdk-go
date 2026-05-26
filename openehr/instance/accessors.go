package instance

import (
	"fmt"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// AsComposition asserts that v is a *rm.Composition (the typical
// COMPOSITION-root result from [Generate]). Returns ErrTypeMismatch
// when v is any other concrete type.
func AsComposition(v any) (*rm.Composition, error) {
	c, ok := v.(*rm.Composition)
	if !ok {
		return nil, fmt.Errorf("%w: got %T, want *rm.Composition", ErrTypeMismatch, v)
	}
	return c, nil
}

// AsObservation asserts that v is a *rm.Observation.
func AsObservation(v any) (*rm.Observation, error) {
	o, ok := v.(*rm.Observation)
	if !ok {
		return nil, fmt.Errorf("%w: got %T, want *rm.Observation", ErrTypeMismatch, v)
	}
	return o, nil
}

// AsEvaluation asserts that v is a *rm.Evaluation.
func AsEvaluation(v any) (*rm.Evaluation, error) {
	e, ok := v.(*rm.Evaluation)
	if !ok {
		return nil, fmt.Errorf("%w: got %T, want *rm.Evaluation", ErrTypeMismatch, v)
	}
	return e, nil
}

// AsInstruction asserts that v is a *rm.Instruction.
func AsInstruction(v any) (*rm.Instruction, error) {
	i, ok := v.(*rm.Instruction)
	if !ok {
		return nil, fmt.Errorf("%w: got %T, want *rm.Instruction", ErrTypeMismatch, v)
	}
	return i, nil
}

// AsAction asserts that v is a *rm.Action.
func AsAction(v any) (*rm.Action, error) {
	a, ok := v.(*rm.Action)
	if !ok {
		return nil, fmt.Errorf("%w: got %T, want *rm.Action", ErrTypeMismatch, v)
	}
	return a, nil
}

// AsAdminEntry asserts that v is a *rm.AdminEntry.
func AsAdminEntry(v any) (*rm.AdminEntry, error) {
	a, ok := v.(*rm.AdminEntry)
	if !ok {
		return nil, fmt.Errorf("%w: got %T, want *rm.AdminEntry", ErrTypeMismatch, v)
	}
	return a, nil
}

// AsCluster asserts that v is a *rm.Cluster.
func AsCluster(v any) (*rm.Cluster, error) {
	c, ok := v.(*rm.Cluster)
	if !ok {
		return nil, fmt.Errorf("%w: got %T, want *rm.Cluster", ErrTypeMismatch, v)
	}
	return c, nil
}

// AsSection asserts that v is a *rm.Section.
func AsSection(v any) (*rm.Section, error) {
	s, ok := v.(*rm.Section)
	if !ok {
		return nil, fmt.Errorf("%w: got %T, want *rm.Section", ErrTypeMismatch, v)
	}
	return s, nil
}

// AsGenericEntry asserts that v is a *rm.GenericEntry.
func AsGenericEntry(v any) (*rm.GenericEntry, error) {
	g, ok := v.(*rm.GenericEntry)
	if !ok {
		return nil, fmt.Errorf("%w: got %T, want *rm.GenericEntry", ErrTypeMismatch, v)
	}
	return g, nil
}

// AsElement asserts that v is a *rm.Element.
func AsElement(v any) (*rm.Element, error) {
	e, ok := v.(*rm.Element)
	if !ok {
		return nil, fmt.Errorf("%w: got %T, want *rm.Element", ErrTypeMismatch, v)
	}
	return e, nil
}
