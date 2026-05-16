// Package internal hosts implementation helpers that are explicitly
// excluded from the module's backwards-compatibility promises (per Go
// convention: anything under internal/ is invisible to external
// consumers). Add to this tree when a helper has no public surface
// rationale — but document the rationale in docs/architecture.md.
package internal
